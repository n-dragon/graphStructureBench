package graph

import (
	"slices"
	"strings"
	"testing"
)

// TestBitMatrixRefusesOversizedGraphs : le coût est en n², la structure doit
// refuser plutôt que tenter une allocation démesurée.
func TestBitMatrixRefusesOversizedGraphs(t *testing.T) {
	if _, err := NewBitMatrix(1<<20, nil); err == nil {
		t.Fatal("un million de sommets (128 Gio) devrait être refusé")
	} else if !strings.Contains(err.Error(), "bitmatrix") {
		t.Errorf("message d'erreur peu explicite: %v", err)
	}
	if _, err := NewBitMatrix(1024, nil); err != nil {
		t.Errorf("1024 sommets devrait passer: %v", err)
	}
}

// TestBitMatrixWordBoundaries : les voisins aux frontières de mots de 64 bits
// sont l'endroit où un décalage mal écrit se voit.
func TestBitMatrixWordBoundaries(t *testing.T) {
	const n = 200
	targets := []uint32{0, 1, 62, 63, 64, 65, 127, 128, 191, 199}
	var pairs [][2]uint32
	for _, v := range targets {
		pairs = append(pairs, [2]uint32{5, v})
	}
	gi, err := NewBitMatrix(n, Dedup(edgesOf(pairs)))
	if err != nil {
		t.Fatal(err)
	}
	g := gi.(*BitMatrix)

	if nb := g.AppendNeighbors(nil, 5); !slices.Equal(nb, targets) {
		t.Errorf("voisins = %v, attendu %v", nb, targets)
	}
	if got := g.Degree(5); got != len(targets) {
		t.Errorf("Degree = %d, attendu %d", got, len(targets))
	}
	for v := uint32(0); v < n; v++ {
		want := slices.Contains(targets, v)
		if got := g.HasEdge(5, v); got != want {
			t.Errorf("HasEdge(5,%d) = %v, attendu %v", v, got, want)
		}
	}
}

// TestBitMatrixRowIsolation : écrire la ligne d'un sommet ne doit pas déborder
// sur la suivante, y compris quand n n'est pas un multiple de 64.
func TestBitMatrixRowIsolation(t *testing.T) {
	const n = 100 // 2 mots par ligne, 28 bits de remplissage
	gi, err := NewBitMatrix(n, Dedup(edgesOf([][2]uint32{
		{3, 99}, {4, 0}, {5, 99},
	})))
	if err != nil {
		t.Fatal(err)
	}
	g := gi.(*BitMatrix)

	if nb := g.AppendNeighbors(nil, 3); !slices.Equal(nb, []uint32{99}) {
		t.Errorf("voisins de 3 = %v, attendu [99]", nb)
	}
	if nb := g.AppendNeighbors(nil, 4); !slices.Equal(nb, []uint32{0}) {
		t.Errorf("voisins de 4 = %v, attendu [0] (débordement depuis la ligne 3 ?)", nb)
	}
	if g.HasEdge(4, 99) {
		t.Error("HasEdge(4,99) = true : la ligne 3 déborde sur la ligne 4")
	}
	for u := uint32(0); u < n; u++ {
		if u == 3 || u == 4 || u == 5 {
			continue
		}
		if d := g.Degree(u); d != 0 {
			t.Errorf("Degree(%d) = %d, attendu 0", u, d)
		}
	}
}

// TestBitMatrixMemoryIsQuadratic : la structure paie n² bits quel que soit le
// nombre d'arêtes. C'est son intérêt (accès en O(1)) et sa limite.
func TestBitMatrixMemoryIsQuadratic(t *testing.T) {
	const n = 640 // 10 mots par ligne
	empty, err := NewBitMatrix(n, nil)
	if err != nil {
		t.Fatal(err)
	}
	full, err := NewBitMatrix(n, Dedup(edgesOf([][2]uint32{{1, 2}, {3, 4}, {5, 6}})))
	if err != nil {
		t.Fatal(err)
	}
	if empty.MemoryBytes() != full.MemoryBytes() {
		t.Errorf("mémoire dépendante des arêtes: %d o à vide, %d o avec 3 arêtes",
			empty.MemoryBytes(), full.MemoryBytes())
	}
	want := uint64(sliceHeaderBytes + n*(n/64)*8)
	if got := empty.MemoryBytes(); got != want {
		t.Errorf("MemoryBytes = %d, attendu %d (n²/8 octets)", got, want)
	}
}

// TestBitMatrixCollapsesDuplicates : illustration de la précondition des
// constructeurs. Sur une liste comportant des doublons, une représentation
// ensembliste n'en garde qu'un exemplaire alors que NumEdges en compte deux —
// d'où l'obligation de passer par Dedup.
func TestBitMatrixCollapsesDuplicates(t *testing.T) {
	withDup := []Edge{{From: 1, To: 2}, {From: 1, To: 2}} // volontairement non dédupliquée
	gi, err := NewBitMatrix(10, withDup)
	if err != nil {
		t.Fatal(err)
	}
	g := gi.(*BitMatrix)

	if got := g.NumEdges(); got != 2 {
		t.Errorf("NumEdges = %d : le compte reflète la liste fournie", got)
	}
	if got := g.Degree(1); got != 1 {
		t.Errorf("Degree = %d : le bitmap ne peut porter qu'un exemplaire", got)
	}
	// La même liste dédupliquée est cohérente de bout en bout.
	gi2, err := NewBitMatrix(10, Dedup(withDup))
	if err != nil {
		t.Fatal(err)
	}
	if gi2.NumEdges() != gi2.Degree(1) {
		t.Errorf("après Dedup: NumEdges = %d, Degree = %d", gi2.NumEdges(), gi2.Degree(1))
	}
}
