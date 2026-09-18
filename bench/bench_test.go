package bench_test

// Benchmarks au format standard `go test -bench`, complémentaires de la
// commande gsbench : l'axe « threads » se pilote ici avec -cpu.
//
//	go test ./bench -bench . -cpu=1,2,4 -benchtime=2s
//	go test ./bench -bench BFS -cpu=1,4 -run '^$'
//
// L'axe « sommets » est balayé par les sous-benchmarks, l'axe « requêtes
// concurrentes » par b.N (go test l'ajuste tout seul).

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/n-dragon/graphstructurebench/gen"
	"github.com/n-dragon/graphstructurebench/graph"
	"github.com/n-dragon/graphstructurebench/traverse"
)

var benchSpecs = []gen.Spec{
	{Kind: "er", N: 20_000, AvgDegree: 8, Undirected: true, Seed: 42},
	{Kind: "er", N: 500_000, AvgDegree: 8, Undirected: true, Seed: 42},
	{Kind: "rmat", N: 500_000, AvgDegree: 8, Undirected: true, Seed: 42},
	{Kind: "grid", N: 500_000, Undirected: true, Seed: 42},
}

type built struct {
	spec gen.Spec
	gg   gen.Graph
	idx  graph.Graph
	name string
}

// forEachIndex construit chaque structure sur chaque graphe et exécute fn.
func forEachIndex(b *testing.B, fn func(b *testing.B, bi built)) {
	b.Helper()
	for _, spec := range benchSpecs {
		gg, err := gen.Generate(spec)
		if err != nil {
			b.Fatal(err)
		}
		for _, builder := range graph.Builders {
			if builder.MaxVertices > 0 && gg.N > builder.MaxVertices {
				continue
			}
			idx, err := builder.Build(gg.N, gg.Edges)
			if err != nil {
				continue
			}
			bi := built{spec: spec, gg: gg, idx: idx, name: builder.Name}
			b.Run(fmt.Sprintf("%s-n%d/%s", spec.Kind, gg.N, builder.Name), func(b *testing.B) {
				b.ReportMetric(float64(idx.MemoryBytes())/(1<<20), "Mio_index")
				b.ReportMetric(float64(idx.MemoryBytes())/float64(len(gg.Edges)), "o/arête")
				fn(b, bi)
			})
		}
	}
}

// BenchmarkBuild mesure la construction de l'index depuis la liste d'arêtes.
func BenchmarkBuild(b *testing.B) {
	for _, spec := range benchSpecs {
		gg, err := gen.Generate(spec)
		if err != nil {
			b.Fatal(err)
		}
		for _, builder := range graph.Builders {
			if builder.MaxVertices > 0 && gg.N > builder.MaxVertices {
				continue
			}
			b.Run(fmt.Sprintf("%s-n%d/%s", spec.Kind, gg.N, builder.Name), func(b *testing.B) {
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					if _, err := builder.Build(gg.N, gg.Edges); err != nil {
						b.Skip(err)
					}
				}
			})
		}
	}
}

// BenchmarkNeighbors : lecture d'adjacence concurrente, le cœur d'un parcours.
func BenchmarkNeighbors(b *testing.B) {
	forEachIndex(b, func(b *testing.B, bi built) {
		b.RunParallel(func(pb *testing.PB) {
			r := rand.New(rand.NewPCG(1, 2))
			var sink uint64
			for pb.Next() {
				sink += traverse.Neighbors(bi.idx, uint32(r.IntN(bi.gg.N)))
			}
			_ = sink
		})
	})
}

// BenchmarkHasEdge : test d'existence d'arête, moitié présentes moitié absentes.
func BenchmarkHasEdge(b *testing.B) {
	forEachIndex(b, func(b *testing.B, bi built) {
		b.RunParallel(func(pb *testing.PB) {
			r := rand.New(rand.NewPCG(3, 4))
			var hits int
			for i := 0; pb.Next(); i++ {
				var u, v uint32
				if i%2 == 0 {
					e := bi.gg.Edges[r.IntN(len(bi.gg.Edges))]
					u, v = e.From, e.To
				} else {
					u, v = uint32(r.IntN(bi.gg.N)), uint32(r.IntN(bi.gg.N))
				}
				if bi.idx.HasEdge(u, v) {
					hits++
				}
			}
			_ = hits
		})
	})
}

// BenchmarkBFS : parcours complets concurrents, un Scratch par goroutine.
func BenchmarkBFS(b *testing.B) {
	forEachIndex(b, func(b *testing.B, bi built) {
		b.RunParallel(func(pb *testing.PB) {
			r := rand.New(rand.NewPCG(5, 6))
			s := traverse.NewScratch(bi.gg.N)
			var sink uint64
			for pb.Next() {
				_, sum := traverse.BFS(bi.idx, uint32(r.IntN(bi.gg.N)), s)
				sink += sum
			}
			_ = sink
		})
	})
}

// BenchmarkCallbackOverhead compare l'itération par callback à l'itération
// directe sur la slice d'adjacence, pour chiffrer le prix de l'abstraction
// commune à toutes les structures.
func BenchmarkCallbackOverhead(b *testing.B) {
	gg, err := gen.Generate(gen.Spec{Kind: "er", N: 500_000, AvgDegree: 8, Undirected: true, Seed: 42})
	if err != nil {
		b.Fatal(err)
	}
	for _, builder := range graph.Builders {
		idx, err := builder.Build(gg.N, gg.Edges)
		if err != nil {
			continue
		}
		sg, ok := idx.(graph.SliceGraph)
		if !ok {
			continue
		}
		b.Run(builder.Name+"/callback", func(b *testing.B) {
			r := rand.New(rand.NewPCG(1, 2))
			var sink uint64
			for i := 0; i < b.N; i++ {
				sink += traverse.Neighbors(idx, uint32(r.IntN(gg.N)))
			}
			_ = sink
		})
		b.Run(builder.Name+"/slice", func(b *testing.B) {
			r := rand.New(rand.NewPCG(1, 2))
			var sink uint64
			for i := 0; i < b.N; i++ {
				sink += traverse.NeighborsSlice(sg, uint32(r.IntN(gg.N)))
			}
			_ = sink
		})
	}
}
