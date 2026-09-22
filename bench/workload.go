// Package bench exécute les charges de travail sur chaque structure et mesure
// temps, débit, latence et mémoire.
package bench

import (
	"fmt"
	"math/rand/v2"

	"github.com/n-dragon/graphstructurebench/gen"
	"github.com/n-dragon/graphstructurebench/graph"
	"github.com/n-dragon/graphstructurebench/traverse"
)

// Query est une requête : A seul pour un parcours ou une lecture d'adjacence,
// (A,B) pour un test d'existence d'arête.
type Query struct{ A, B uint32 }

// Workload décrit une charge de travail.
type Workload struct {
	Name string
	Desc string
	// Chunk : nombre de requêtes par unité de temps chronométrée. Pour les
	// requêtes de quelques dizaines de nanosecondes, chronométrer chaque
	// requête coûterait plus cher que la requête elle-même. Les tailles sont
	// choisies pour qu'un lot dure de l'ordre de 10 µs : en deçà, le coût des
	// deux appels à time.Now() entre dans la mesure et gonfle sa variance.
	Chunk int
	// DefaultQueries : tailles de lot par défaut si l'utilisateur n'en impose pas.
	DefaultQueries []int
	Gen            func(r *rand.Rand, g gen.Graph, count int) []Query
	Run            func(g graph.Graph, q Query, s *traverse.Scratch) uint64
}

// Workloads liste les charges disponibles.
var Workloads = []Workload{
	{
		Name: "bfs", Desc: "parcours en largeur complet depuis une source",
		Chunk: 1, DefaultQueries: []int{8, 64},
		Gen: randomSources,
		Run: func(g graph.Graph, q Query, s *traverse.Scratch) uint64 {
			_, sum := traverse.BFS(g, q.A, s)
			return sum
		},
	},
	{
		Name: "bfs-batch", Desc: "parcours en largeur lisant l'adjacence par blocs",
		Chunk: 1, DefaultQueries: []int{8, 64},
		Gen: randomSources,
		Run: func(g graph.Graph, q Query, s *traverse.Scratch) uint64 {
			_, sum := traverse.BFSBatch(g, q.A, s)
			return sum
		},
	},
	{
		Name: "dfs", Desc: "parcours en profondeur itératif depuis une source",
		Chunk: 1, DefaultQueries: []int{8, 64},
		Gen: randomSources,
		Run: func(g graph.Graph, q Query, s *traverse.Scratch) uint64 {
			_, sum := traverse.DFS(g, q.A, s)
			return sum
		},
	},
	{
		Name: "neighbors", Desc: "lecture de l'adjacence d'un sommet tiré au hasard",
		Chunk: 512, DefaultQueries: []int{100_000, 1_000_000},
		Gen: randomSources,
		Run: func(g graph.Graph, q Query, s *traverse.Scratch) uint64 {
			return traverse.Neighbors(g, q.A, s)
		},
	},
	{
		Name: "neighbors-batch", Desc: "lecture de l'adjacence par bloc, sans appel indirect",
		Chunk: 512, DefaultQueries: []int{100_000, 1_000_000},
		Gen: randomSources,
		Run: func(g graph.Graph, q Query, s *traverse.Scratch) uint64 {
			return traverse.NeighborsBatch(g, q.A, s)
		},
	},
	{
		Name: "hasedge", Desc: "test d'existence d'arête, moitié présentes moitié absentes",
		Chunk: 2048, DefaultQueries: []int{100_000, 1_000_000},
		Gen: edgeProbes,
		Run: func(g graph.Graph, q Query, _ *traverse.Scratch) uint64 {
			if g.HasEdge(q.A, q.B) {
				return 1
			}
			return 0
		},
	},
}

// WorkloadByName retourne la charge portant ce nom.
func WorkloadByName(name string) (Workload, error) {
	for _, w := range Workloads {
		if w.Name == name {
			return w, nil
		}
	}
	return Workload{}, fmt.Errorf("charge inconnue: %q", name)
}

// randomSources tire des sommets au hasard. Pour les parcours, on ne retient
// que des sommets de degré non nul : partir d'un sommet isolé mesurerait le
// coût de la boucle du runner, pas celui de la structure.
func randomSources(r *rand.Rand, g gen.Graph, count int) []Query {
	deg := make([]int32, g.N)
	for _, e := range g.Edges {
		deg[e.From]++
	}
	candidates := make([]uint32, 0, g.N)
	for u := 0; u < g.N; u++ {
		if deg[u] > 0 {
			candidates = append(candidates, uint32(u))
		}
	}
	if len(candidates) == 0 {
		candidates = append(candidates, 0)
	}
	qs := make([]Query, count)
	for i := range qs {
		qs[i] = Query{A: candidates[r.IntN(len(candidates))]}
	}
	return qs
}

// edgeProbes construit un mélange 50/50 d'arêtes existantes et de couples
// tirés au hasard : un test d'existence qui ne sonderait que des arêtes
// présentes flatterait les structures qui sortent tôt de leur boucle.
func edgeProbes(r *rand.Rand, g gen.Graph, count int) []Query {
	qs := make([]Query, count)
	for i := range qs {
		if i%2 == 0 && len(g.Edges) > 0 {
			e := g.Edges[r.IntN(len(g.Edges))]
			qs[i] = Query{A: e.From, B: e.To}
		} else {
			qs[i] = Query{A: uint32(r.IntN(g.N)), B: uint32(r.IntN(g.N))}
		}
	}
	return qs
}
