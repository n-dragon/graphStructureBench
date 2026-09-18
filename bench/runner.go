package bench

import (
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/n-dragon/graphstructurebench/gen"
	"github.com/n-dragon/graphstructurebench/graph"
	"github.com/n-dragon/graphstructurebench/traverse"
)

// BuildInfo rassemble ce qu'on apprend à la construction de l'index.
type BuildInfo struct {
	Duration time.Duration
	// HeapBytes est le delta de tas après GC : la mesure de référence pour le
	// critère mémoire, elle inclut tout ce que la structure retient réellement.
	HeapBytes uint64
	// AnalyticBytes est le compte déclaré par la structure (somme des tableaux).
	AnalyticBytes uint64
	Detail        string
}

// Build construit l'index et mesure son temps de construction et son
// empreinte mémoire. La liste d'arêtes source est allouée avant la mesure :
// elle n'entre donc pas dans le delta.
func Build(b graph.Builder, g gen.Graph) (graph.Graph, BuildInfo, error) {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	start := time.Now()
	gr, err := b.Build(g.N, g.Edges)
	elapsed := time.Since(start)
	if err != nil {
		return nil, BuildInfo{}, err
	}

	runtime.GC()
	runtime.ReadMemStats(&after)
	var heap uint64
	if after.HeapAlloc > before.HeapAlloc {
		heap = after.HeapAlloc - before.HeapAlloc
	}
	runtime.KeepAlive(gr)

	info := BuildInfo{Duration: elapsed, HeapBytes: heap, AnalyticBytes: gr.MemoryBytes()}
	if h, ok := gr.(*graph.Hybrid); ok {
		info.Detail = "hubs=" + itoa(h.NumHubs())
	}
	return gr, info, nil
}

// RunStats est le résultat chronométré d'un lot de requêtes.
type RunStats struct {
	Wall         time.Duration
	Throughput   float64 // requêtes par seconde
	P50, P99     time.Duration
	ChunkSize    int
	ScratchBytes uint64 // cumul sur tous les threads
	Checksum     uint64
}

// Run exécute `queries` requêtes réparties sur `threads` goroutines, qui se
// servent dans un compteur atomique commun : c'est le modèle d'un service qui
// encaisse des requêtes concurrentes sur un index partagé en lecture seule.
//
// GOMAXPROCS est fixé à threads le temps de la mesure, pour que l'axe
// « nombre de threads » corresponde à un vrai parallélisme matériel borné.
func Run(g graph.Graph, wl Workload, queries []Query, threads int) RunStats {
	if threads < 1 {
		threads = 1
	}
	prev := runtime.GOMAXPROCS(threads)
	defer runtime.GOMAXPROCS(prev)

	chunk := wl.Chunk
	if chunk < 1 {
		chunk = 1
	}
	n := g.NumVertices()

	type workerOut struct {
		samples  []time.Duration
		checksum uint64
		scratch  uint64
	}
	outs := make([]workerOut, threads)

	var next atomic.Int64
	var wg sync.WaitGroup
	start := time.Now()
	for w := 0; w < threads; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			s := traverse.NewScratch(n)
			var sum uint64
			samples := make([]time.Duration, 0, len(queries)/chunk/threads+1)
			for {
				lo := int(next.Add(int64(chunk))) - chunk
				if lo >= len(queries) {
					break
				}
				hi := min(lo+chunk, len(queries))
				t0 := time.Now()
				for _, q := range queries[lo:hi] {
					sum += wl.Run(g, q, s)
				}
				samples = append(samples, time.Since(t0))
			}
			outs[w] = workerOut{samples: samples, checksum: sum, scratch: s.Bytes()}
		}(w)
	}
	wg.Wait()
	wall := time.Since(start)

	var all []time.Duration
	var checksum, scratch uint64
	for _, o := range outs {
		all = append(all, o.samples...)
		checksum += o.checksum // somme commutative : indépendante de l'ordonnancement
		scratch += o.scratch
	}
	slices.Sort(all)

	st := RunStats{
		Wall: wall, ChunkSize: chunk, Checksum: checksum, ScratchBytes: scratch,
		P50: percentile(all, 0.50), P99: percentile(all, 0.99),
	}
	if wall > 0 {
		st.Throughput = float64(len(queries)) / wall.Seconds()
	}
	return st
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := int(p * float64(len(sorted)))
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}
