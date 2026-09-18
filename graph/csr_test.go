package graph

import (
	"slices"
	"testing"
)

// mkEdges construit une liste d'arêtes à partir de couples, dédupliquée et
// triée comme l'exigent les constructeurs.
func mkEdges(pairs ...[2]uint32) []Edge {
	out := make([]Edge, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, Edge{From: p[0], To: p[1]})
	}
	return Dedup(out)
}

// sampleEdges : un petit graphe aux degrés inégaux, avec sommets isolés.
func sampleEdges() (int, []Edge) {
	return 8, mkEdges(
		[2]uint32{0, 3}, [2]uint32{0, 1}, [2]uint32{0, 7},
		[2]uint32{2, 2},
		[2]uint32{5, 0}, [2]uint32{5, 4},
		[2]uint32{7, 6},
	)
}

func TestBuildCSRArraysInvariants(t *testing.T) {
	n, edges := sampleEdges()
	off, tgt := buildCSRArrays(n, edges)

	if len(off) != n+1 {
		t.Fatalf("offsets de longueur %d, attendu %d", len(off), n+1)
	}
	if off[0] != 0 {
		t.Errorf("offsets[0] = %d, attendu 0", off[0])
	}
	if int(off[n]) != len(edges) {
		t.Errorf("offsets[n] = %d, attendu %d arêtes", off[n], len(edges))
	}
	if len(tgt) != len(edges) {
		t.Errorf("cibles de longueur %d, attendu %d", len(tgt), len(edges))
	}
	for u := 0; u < n; u++ {
		if off[u] > off[u+1] {
			t.Fatalf("offsets non croissants en %d: %d > %d", u, off[u], off[u+1])
		}
		block := tgt[off[u]:off[u+1]]
		if !slices.IsSorted(block) {
			t.Errorf("adjacence de %d non triée: %v", u, block)
		}
	}
}

// TestBuildCSRArraysSortsUnsortedInput : le tri intra-sommet ne doit pas
// dépendre de l'ordre d'entrée, sans quoi la recherche binaire de HasEdge
// répondrait faux sur une liste d'arêtes non triée.
func TestBuildCSRArraysSortsUnsortedInput(t *testing.T) {
	edges := []Edge{{0, 9}, {0, 2}, {0, 5}, {0, 1}} // volontairement non triées
	off, tgt := buildCSRArrays(10, edges)
	block := tgt[off[0]:off[1]]
	want := []uint32{1, 2, 5, 9}
	if !slices.Equal(block, want) {
		t.Errorf("adjacence = %v, attendu %v", block, want)
	}
}

// TestCSRNeighborsAliasesArena : Neighbors doit être une vue sur le tableau de
// cibles, pas une copie — c'est toute la raison d'être du CSR.
func TestCSRNeighborsAliasesArena(t *testing.T) {
	n, edges := sampleEdges()
	g := NewCSR(n, edges)
	nb := g.Neighbors(0)
	if len(nb) == 0 {
		t.Fatal("le sommet 0 devrait avoir des voisins")
	}
	g.targets[g.offsets[0]] = 42 // écriture dans l'arène
	if nb[0] != 42 {
		t.Error("Neighbors a copié l'adjacence au lieu d'en rendre une vue")
	}
}

func TestCSRMemoryBytesFormula(t *testing.T) {
	n, edges := sampleEdges()
	g := NewCSR(n, edges)
	want := uint64(sliceHeaderBytes + 4*(n+1) + sliceHeaderBytes + 4*len(edges))
	if got := g.MemoryBytes(); got != want {
		t.Errorf("MemoryBytes = %d, attendu %d (4 o/sommet + 4 o/arête + 2 en-têtes)", got, want)
	}
}

// TestCSRSlicesCappedSlices : chaque adjacence est découpée avec une capacité
// bornée à sa longueur. Sans cela, un appelant qui ferait append sur
// l'adjacence rendue écraserait silencieusement celle du sommet suivant.
func TestCSRSlicesCappedSlices(t *testing.T) {
	n, edges := sampleEdges()
	g := NewCSRSlices(n, edges)

	for u := 0; u < n; u++ {
		nb := g.adj[u]
		if cap(nb) != len(nb) {
			t.Errorf("adjacence de %d: cap = %d, len = %d (découpage à trois indices attendu)",
				u, cap(nb), len(nb))
		}
	}

	// Mise en situation : append sur l'adjacence du sommet 0.
	before := slices.Clone(g.Neighbors(2))
	victim := g.Neighbors(0)
	_ = append(victim, 999)
	if after := g.Neighbors(2); !slices.Equal(after, before) {
		t.Errorf("append sur l'adjacence de 0 a corrompu celle de 2: %v puis %v", before, after)
	}
}

func TestCSRSlicesArenaIsContiguous(t *testing.T) {
	n, edges := sampleEdges()
	g := NewCSRSlices(n, edges)
	total := 0
	for u := 0; u < n; u++ {
		total += len(g.adj[u])
	}
	if total != len(g.arena) {
		t.Errorf("somme des adjacences = %d, arène de %d éléments", total, len(g.arena))
	}
	if len(g.arena) != len(edges) {
		t.Errorf("arène de %d éléments, attendu %d arêtes", len(g.arena), len(edges))
	}
}

func TestCSRSlicesMemoryBytesFormula(t *testing.T) {
	n, edges := sampleEdges()
	g := NewCSRSlices(n, edges)
	want := uint64(sliceHeaderBytes + 4*len(edges) + sliceHeaderBytes + sliceHeaderBytes*n)
	if got := g.MemoryBytes(); got != want {
		t.Errorf("MemoryBytes = %d, attendu %d (24 o/sommet + 4 o/arête)", got, want)
	}
}
