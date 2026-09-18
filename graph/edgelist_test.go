package graph

import (
	"math"
	"slices"
	"testing"
)

func TestSortedEdgeListKeysAreSorted(t *testing.T) {
	n, edges := sampleEdges()
	g := NewSortedEdgeList(n, edges)
	if !slices.IsSorted(g.keys) {
		t.Errorf("clés non triées: %v", g.keys)
	}
	if len(g.keys) != len(edges) {
		t.Errorf("%d clés pour %d arêtes", len(g.keys), len(edges))
	}
}

// TestSortedEdgeListLowerBound : la recherche binaire doit tomber sur la
// première arête du sommet, y compris quand le sommet n'en a aucune.
func TestSortedEdgeListLowerBound(t *testing.T) {
	n, edges := sampleEdges()
	g := NewSortedEdgeList(n, edges)
	for u := 0; u < n; u++ {
		lo := g.lowerBound(uint32(u))
		if lo > 0 && g.keys[lo-1]>>32 >= uint64(u) {
			t.Errorf("lowerBound(%d) = %d : la clé précédente appartient déjà à %d",
				u, lo, g.keys[lo-1]>>32)
		}
		if lo < len(g.keys) && g.keys[lo]>>32 < uint64(u) {
			t.Errorf("lowerBound(%d) = %d : la clé pointée appartient à %d",
				u, lo, g.keys[lo]>>32)
		}
	}
}

// TestSortedEdgeListBoundaries : premier sommet, dernier sommet, sommet isolé
// en fin de tableau — les trois endroits où une recherche de borne se trompe.
func TestSortedEdgeListBoundaries(t *testing.T) {
	// Le sommet 0 émet, le sommet 9 (dernier) émet, les autres non.
	g := NewSortedEdgeList(10, mkEdges(
		[2]uint32{0, 1}, [2]uint32{0, 9}, [2]uint32{9, 0}, [2]uint32{9, 9}))

	if got, want := g.Degree(0), 2; got != want {
		t.Errorf("Degree(0) = %d, attendu %d", got, want)
	}
	if got, want := g.Degree(9), 2; got != want {
		t.Errorf("Degree(9) = %d, attendu %d (dernier sommet)", got, want)
	}
	if got := g.Degree(8); got != 0 {
		t.Errorf("Degree(8) = %d, attendu 0 (sommet isolé juste avant le dernier)", got)
	}
	if nb := g.AppendNeighbors(nil, 9); !slices.Equal(nb, []uint32{0, 9}) {
		t.Errorf("voisins du dernier sommet = %v, attendu [0 9]", nb)
	}
}

// TestSortedEdgeListMaxVertexID : régression sur un débordement d'entier.
// La borne haute s'écrivait uint64(u+1)<<32 ; sur u = MaxUint32, l'addition
// débordait en uint32 et donnait une borne nulle, donc un sommet sans voisin.
// Cette structure ne dimensionne rien sur n, on peut donc tester le sommet
// extrême sans rien allouer.
func TestSortedEdgeListMaxVertexID(t *testing.T) {
	const maxID = uint32(math.MaxUint32)
	g := NewSortedEdgeList(1<<32, mkEdges(
		[2]uint32{maxID, 3}, [2]uint32{maxID, 7}, [2]uint32{maxID - 1, 1}))

	if got := g.Degree(maxID); got != 2 {
		t.Errorf("Degree(MaxUint32) = %d, attendu 2", got)
	}
	if nb := g.AppendNeighbors(nil, maxID); !slices.Equal(nb, []uint32{3, 7}) {
		t.Errorf("voisins de MaxUint32 = %v, attendu [3 7]", nb)
	}
	if !g.HasEdge(maxID, 7) {
		t.Error("HasEdge(MaxUint32, 7) = false")
	}
	var seen []uint32
	g.ForEachNeighbor(maxID, func(v uint32) bool { seen = append(seen, v); return true })
	if !slices.Equal(seen, []uint32{3, 7}) {
		t.Errorf("ForEachNeighbor(MaxUint32) = %v, attendu [3 7]", seen)
	}
}

// TestSortedEdgeListCostsNothingPerVertex : l'argument de la structure est de
// ne rien payer pour les sommets, seulement 8 octets par arête.
func TestSortedEdgeListCostsNothingPerVertex(t *testing.T) {
	edges := mkEdges([2]uint32{5, 1})
	small := NewSortedEdgeList(10, edges).MemoryBytes()
	huge := NewSortedEdgeList(10_000_000, edges).MemoryBytes()
	if small != huge {
		t.Errorf("mémoire dépendante de n: %d o pour 10 sommets, %d o pour 10 millions", small, huge)
	}
	if want := uint64(sliceHeaderBytes + 8*len(edges)); small != want {
		t.Errorf("MemoryBytes = %d, attendu %d (8 o/arête)", small, want)
	}
}
