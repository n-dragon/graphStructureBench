package traverse_test

import (
	"slices"
	"testing"

	"github.com/n-dragon/graphstructurebench/graph"
	"github.com/n-dragon/graphstructurebench/traverse"
)

// chainAndIsland : une chaîne 0-1-2-3 et un îlot 5-6, plus un sommet isolé.
// Deux composantes suffisent à révéler un reset incomplet.
func chainAndIsland() graph.Graph {
	edges := graph.Dedup([]graph.Edge{
		{From: 0, To: 1}, {From: 1, To: 0},
		{From: 1, To: 2}, {From: 2, To: 1},
		{From: 2, To: 3}, {From: 3, To: 2},
		{From: 5, To: 6}, {From: 6, To: 5},
	})
	return graph.NewCSR(8, edges)
}

func TestBFSVisitsOnlyItsComponent(t *testing.T) {
	g := chainAndIsland()
	s := traverse.NewScratch(g.NumVertices())

	n, sum := traverse.BFS(g, 0, s)
	if n != 4 || sum != 0+1+2+3 {
		t.Errorf("BFS depuis 0 = (%d sommets, somme %d), attendu (4, 6)", n, sum)
	}
	n, sum = traverse.BFS(g, 5, s)
	if n != 2 || sum != 5+6 {
		t.Errorf("BFS depuis 5 = (%d sommets, somme %d), attendu (2, 11)", n, sum)
	}
	n, sum = traverse.BFS(g, 7, s)
	if n != 1 || sum != 7 {
		t.Errorf("BFS depuis le sommet isolé 7 = (%d, %d), attendu (1, 7)", n, sum)
	}
}

// TestScratchResetIsComplete : c'est le test qui compte. Le reset n'efface que
// les bits réellement posés au parcours précédent ; s'il en oubliait un, les
// parcours suivants sauteraient des sommets et toutes les mesures du banc
// seraient fausses sans que rien ne plante.
func TestScratchResetIsComplete(t *testing.T) {
	g := chainAndIsland()
	s := traverse.NewScratch(g.NumVertices())

	// Référence : un Scratch neuf par parcours.
	want := make([]uint64, g.NumVertices())
	for u := 0; u < g.NumVertices(); u++ {
		fresh := traverse.NewScratch(g.NumVertices())
		_, want[u] = traverse.BFS(g, uint32(u), fresh)
	}

	// Le même Scratch, réutilisé, dans un ordre entremêlé et plusieurs fois.
	for round := 0; round < 3; round++ {
		for _, u := range []int{5, 0, 7, 2, 6, 1, 3, 4} {
			if _, got := traverse.BFS(g, uint32(u), s); got != want[u] {
				t.Fatalf("tour %d, BFS(%d) = %d sur tampon réutilisé, %d sur tampon neuf",
					round, u, got, want[u])
			}
			if _, got := traverse.DFS(g, uint32(u), s); got != want[u] {
				t.Fatalf("tour %d, DFS(%d) = %d, attendu %d", round, u, got, want[u])
			}
			if _, got := traverse.BFSBatch(g, uint32(u), s); got != want[u] {
				t.Fatalf("tour %d, BFSBatch(%d) = %d, attendu %d", round, u, got, want[u])
			}
		}
	}
}

// TestBFSAndDFSCoverTheSameSet : ordres différents, ensemble identique.
func TestBFSAndDFSCoverTheSameSet(t *testing.T) {
	g := chainAndIsland()
	s := traverse.NewScratch(g.NumVertices())
	for u := 0; u < g.NumVertices(); u++ {
		nb, sumB := traverse.BFS(g, uint32(u), s)
		nd, sumD := traverse.DFS(g, uint32(u), s)
		if nb != nd || sumB != sumD {
			t.Errorf("sommet %d: BFS (%d,%d) et DFS (%d,%d) divergent", u, nb, sumB, nd, sumD)
		}
	}
}

// TestNeighborsMatchesBatch : les deux modes de lecture doivent donner la même
// somme, sinon les charges « neighbors » et « neighbors-batch » du banc ne
// seraient pas comparables.
func TestNeighborsMatchesBatch(t *testing.T) {
	g := chainAndIsland()
	s := traverse.NewScratch(g.NumVertices())
	for u := 0; u < g.NumVertices(); u++ {
		callback := traverse.Neighbors(g, uint32(u), s)
		batch := traverse.NeighborsBatch(g, uint32(u), s)
		if callback != batch {
			t.Errorf("sommet %d: callback = %d, bloc = %d", u, callback, batch)
		}
		var want uint64
		g.ForEachNeighbor(uint32(u), func(v uint32) bool { want += uint64(v); return true })
		if callback != want {
			t.Errorf("sommet %d: somme = %d, attendu %d", u, callback, want)
		}
	}
}

// TestNeighborsResetsAccumulator : l'accumulateur vit dans le Scratch ; s'il
// n'était pas remis à zéro, les sommes s'empileraient d'une requête à l'autre.
func TestNeighborsResetsAccumulator(t *testing.T) {
	g := chainAndIsland()
	s := traverse.NewScratch(g.NumVertices())
	first := traverse.Neighbors(g, 1, s)
	for i := 0; i < 5; i++ {
		if got := traverse.Neighbors(g, 1, s); got != first {
			t.Fatalf("appel %d = %d, premier appel = %d : accumulateur non réinitialisé",
				i+2, got, first)
		}
	}
}

// TestScratchBytesCountsAllBuffers : le coût mémoire par thread rapporté par
// le banc doit inclure le tampon de l'accès par bloc.
func TestScratchBytesCountsAllBuffers(t *testing.T) {
	g := chainAndIsland()
	s := traverse.NewScratch(g.NumVertices())
	before := s.Bytes()
	if before == 0 {
		t.Fatal("Scratch.Bytes = 0 à l'allocation")
	}
	for u := 0; u < g.NumVertices(); u++ {
		traverse.BFSBatch(g, uint32(u), s)
	}
	if after := s.Bytes(); after < before {
		t.Errorf("Bytes = %d après usage, %d avant : le compte diminue", after, before)
	}
}

// TestTraversalsAgreeAcrossStructures : le même parcours sur les neuf
// structures doit rendre exactement le même résultat.
func TestTraversalsAgreeAcrossStructures(t *testing.T) {
	edges := graph.Dedup([]graph.Edge{
		{From: 0, To: 1}, {From: 1, To: 0}, {From: 1, To: 2}, {From: 2, To: 1},
		{From: 2, To: 3}, {From: 3, To: 2}, {From: 5, To: 6}, {From: 6, To: 5},
	})
	const n = 8
	var names []string
	var sums []uint64
	for _, b := range graph.Builders {
		g, err := b.Build(n, edges)
		if err != nil {
			continue
		}
		s := traverse.NewScratch(n)
		var acc uint64
		for u := 0; u < n; u++ {
			_, sum := traverse.BFS(g, uint32(u), s)
			acc += sum
			if _, batch := traverse.BFSBatch(g, uint32(u), s); batch != sum {
				t.Errorf("%s: BFSBatch(%d) diverge de BFS", b.Name, u)
			}
		}
		names = append(names, b.Name)
		sums = append(sums, acc)
	}
	if len(sums) == 0 {
		t.Fatal("aucune structure construite")
	}
	for i := range sums {
		if sums[i] != sums[0] {
			t.Errorf("%s: somme des parcours = %d, %s donne %d",
				names[i], sums[i], names[0], sums[0])
		}
	}
	if !slices.Contains(names, "csr") {
		t.Error("csr absent de la comparaison")
	}
}
