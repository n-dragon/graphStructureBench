package graph_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/n-dragon/graphstructurebench/gen"
	"github.com/n-dragon/graphstructurebench/graph"
	"github.com/n-dragon/graphstructurebench/traverse"
)

// testGraphs fournit des graphes petits mais variés : le comportement des
// structures diverge surtout sur les sommets isolés, les hubs et les degrés 1.
func testGraphs(t *testing.T) []gen.Graph {
	t.Helper()
	var out []gen.Graph
	for _, s := range []gen.Spec{
		{Kind: "er", N: 500, AvgDegree: 4, Seed: 1},
		{Kind: "er", N: 2000, AvgDegree: 1, Seed: 2, Undirected: true},
		{Kind: "rmat", N: 1024, AvgDegree: 8, Seed: 3, Undirected: true},
		{Kind: "grid", N: 900, Seed: 4},
	} {
		g, err := gen.Generate(s)
		if err != nil {
			t.Fatalf("génération %v: %v", s, err)
		}
		out = append(out, g)
	}
	return out
}

// reference construit l'adjacence de référence, naïvement.
func reference(g gen.Graph) [][]uint32 {
	adj := make([][]uint32, g.N)
	for _, e := range g.Edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	for _, nb := range adj {
		slices.Sort(nb)
	}
	return adj
}

func TestStructuresAgreeWithReference(t *testing.T) {
	for _, gg := range testGraphs(t) {
		ref := reference(gg)
		for _, b := range graph.Builders {
			if b.MaxVertices > 0 && gg.N > b.MaxVertices {
				continue
			}
			t.Run(fmt.Sprintf("%s/%s", gg.Spec.Kind, b.Name), func(t *testing.T) {
				idx, err := b.Build(gg.N, gg.Edges)
				if err != nil {
					t.Skipf("non construite: %v", err)
				}
				if got := idx.NumVertices(); got != gg.N {
					t.Errorf("NumVertices = %d, attendu %d", got, gg.N)
				}
				if got := idx.NumEdges(); got != len(gg.Edges) {
					t.Errorf("NumEdges = %d, attendu %d", got, len(gg.Edges))
				}
				var buf []uint32
				for u := 0; u < gg.N; u++ {
					var got []uint32
					idx.ForEachNeighbor(uint32(u), func(v uint32) bool {
						got = append(got, v)
						return true
					})
					if !slices.Equal(got, ref[u]) {
						t.Fatalf("voisins de %d = %v, attendu %v", u, got, ref[u])
					}
					if d := idx.Degree(uint32(u)); d != len(ref[u]) {
						t.Fatalf("Degree(%d) = %d, attendu %d", u, d, len(ref[u]))
					}
					// L'accès par lot doit rendre exactement la même chose que
					// l'accès par callback, y compris sur un tampon réutilisé.
					buf = idx.AppendNeighbors(buf[:0], uint32(u))
					if !slices.Equal(buf, ref[u]) {
						t.Fatalf("AppendNeighbors(%d) = %v, attendu %v", u, buf, ref[u])
					}
				}
				if idx.MemoryBytes() == 0 {
					t.Error("MemoryBytes = 0")
				}
			})
		}
	}
}

func TestHasEdge(t *testing.T) {
	for _, gg := range testGraphs(t) {
		ref := reference(gg)
		r := rand.New(rand.NewPCG(7, 8))
		for _, b := range graph.Builders {
			if b.MaxVertices > 0 && gg.N > b.MaxVertices {
				continue
			}
			t.Run(fmt.Sprintf("%s/%s", gg.Spec.Kind, b.Name), func(t *testing.T) {
				idx, err := b.Build(gg.N, gg.Edges)
				if err != nil {
					t.Skipf("non construite: %v", err)
				}
				// arêtes présentes
				for i := 0; i < 500 && i < len(gg.Edges); i++ {
					e := gg.Edges[r.IntN(len(gg.Edges))]
					if !idx.HasEdge(e.From, e.To) {
						t.Fatalf("HasEdge(%d,%d) = false, arête pourtant présente", e.From, e.To)
					}
				}
				// couples quelconques, comparés à la référence
				for i := 0; i < 2000; i++ {
					u, v := uint32(r.IntN(gg.N)), uint32(r.IntN(gg.N))
					want := slices.Contains(ref[u], v)
					if got := idx.HasEdge(u, v); got != want {
						t.Fatalf("HasEdge(%d,%d) = %v, attendu %v", u, v, got, want)
					}
				}
			})
		}
	}
}

// TestTraversalsAgree vérifie que le parcours donne le même résultat quelle
// que soit la structure : c'est l'invariant sur lequel repose la comparaison
// des débits du banc.
func TestTraversalsAgree(t *testing.T) {
	for _, gg := range testGraphs(t) {
		var wantBFS, wantDFS uint64
		var wantCount int
		first := true
		for _, b := range graph.Builders {
			if b.MaxVertices > 0 && gg.N > b.MaxVertices {
				continue
			}
			idx, err := b.Build(gg.N, gg.Edges)
			if err != nil {
				continue
			}
			s := traverse.NewScratch(gg.N)
			nb, sumB := traverse.BFS(idx, 0, s)
			nd, sumD := traverse.DFS(idx, 0, s)
			if nbb, sumBB := traverse.BFSBatch(idx, 0, s); nbb != nb || sumBB != sumB {
				t.Errorf("%s/%s: BFSBatch (%d,%d) diverge de BFS (%d,%d)",
					gg.Spec.Kind, b.Name, nbb, sumBB, nb, sumB)
			}
			if nb != nd || sumB != sumD {
				t.Errorf("%s/%s: BFS (%d,%d) et DFS (%d,%d) ne couvrent pas le même ensemble",
					gg.Spec.Kind, b.Name, nb, sumB, nd, sumD)
			}
			if first {
				wantBFS, wantDFS, wantCount, first = sumB, sumD, nb, false
				continue
			}
			if sumB != wantBFS || sumD != wantDFS || nb != wantCount {
				t.Errorf("%s/%s: parcours divergent (bfs=%d/%d dfs=%d/%d, attendu %d/%d)",
					gg.Spec.Kind, b.Name, nb, sumB, nd, sumD, wantCount, wantBFS)
			}
		}
	}
}

// TestConcurrentReadsAreSafe : les index sont immuables après construction,
// plusieurs goroutines doivent pouvoir les lire sans synchronisation.
func TestConcurrentReadsAreSafe(t *testing.T) {
	gg, err := gen.Generate(gen.Spec{Kind: "rmat", N: 4096, AvgDegree: 8, Seed: 11, Undirected: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range graph.Builders {
		idx, err := b.Build(gg.N, gg.Edges)
		if err != nil {
			continue
		}
		t.Run(b.Name, func(t *testing.T) {
			t.Parallel()
			want := make([]uint64, 8)
			done := make(chan struct{})
			for w := 0; w < 8; w++ {
				go func(w int) {
					defer func() { done <- struct{}{} }()
					s := traverse.NewScratch(gg.N)
					_, sum := traverse.BFS(idx, uint32(w), s)
					want[w] = sum
				}(w)
			}
			for i := 0; i < 8; i++ {
				<-done
			}
			s := traverse.NewScratch(gg.N)
			for w := 0; w < 8; w++ {
				if _, sum := traverse.BFS(idx, uint32(w), s); sum != want[w] {
					t.Fatalf("BFS(%d) concurrent = %d, séquentiel = %d", w, want[w], sum)
				}
			}
		})
	}
}
