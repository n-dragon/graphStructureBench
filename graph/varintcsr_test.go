package graph

import (
	"slices"
	"testing"
)

// TestVarintCSRRoundTripLargeIDs : les écarts multi-octets sont le cas
// intéressant. Avec des identifiants espacés, l'encodage LEB128 passe à 2 puis
// 3 octets, et une erreur de curseur se verrait immédiatement.
func TestVarintCSRRoundTripLargeIDs(t *testing.T) {
	// Écarts choisis de part et d'autre des seuils de 1, 2 et 3 octets
	// (127, 16 383, 2 097 151).
	targets := []uint32{1, 5, 130, 200, 16_500, 20_000, 2_100_000, 2_100_001, 4_000_000}
	var pairs [][2]uint32
	for _, v := range targets {
		pairs = append(pairs, [2]uint32{7, v})
	}
	n := 4_000_001
	g := NewVarintCSR(n, mkEdges(pairs...))

	if got := g.Degree(7); got != len(targets) {
		t.Errorf("Degree = %d, attendu %d", got, len(targets))
	}
	if nb := g.AppendNeighbors(nil, 7); !slices.Equal(nb, targets) {
		t.Errorf("voisins décodés = %v, attendu %v", nb, targets)
	}
	for _, v := range targets {
		if !g.HasEdge(7, v) {
			t.Errorf("HasEdge(7,%d) = false", v)
		}
	}
	for _, v := range []uint32{0, 2, 129, 131, 16_499, 2_100_002, 4_000_000 - 1} {
		if g.HasEdge(7, v) && slices.Contains(targets, v) == false {
			t.Errorf("HasEdge(7,%d) = true, cette arête n'existe pas", v)
		}
	}
}

// TestVarintCSRHasEdgeStopsEarly : la liste étant triée, la recherche doit
// s'arrêter dès qu'elle dépasse la valeur cherchée — c'est ce qui rend le
// test d'arête acceptable malgré le décodage séquentiel.
func TestVarintCSRHasEdgeStopsEarly(t *testing.T) {
	g := NewVarintCSR(100, mkEdges(
		[2]uint32{0, 10}, [2]uint32{0, 20}, [2]uint32{0, 30}))
	cases := []struct {
		v    uint32
		want bool
	}{
		{5, false},  // avant le premier
		{10, true},  // le premier
		{15, false}, // entre deux
		{20, true},
		{30, true},  // le dernier
		{99, false}, // au-delà du dernier
	}
	for _, c := range cases {
		if got := g.HasEdge(0, c.v); got != c.want {
			t.Errorf("HasEdge(0,%d) = %v, attendu %v", c.v, got, c.want)
		}
	}
}

// TestVarintCSRCompressesLocality : la promesse de la structure. Sur une
// adjacence à écarts faibles, un voisin doit tenir sur un octet au lieu de
// quatre.
func TestVarintCSRCompressesLocality(t *testing.T) {
	// 200 sommets, chacun relié à ses 4 voisins immédiats : écarts de 1 ou 2.
	var pairs [][2]uint32
	const n = 200
	for u := uint32(2); u < n-2; u++ {
		pairs = append(pairs, [2]uint32{u, u - 2}, [2]uint32{u, u - 1},
			[2]uint32{u, u + 1}, [2]uint32{u, u + 2})
	}
	edges := mkEdges(pairs...)
	varint := NewVarintCSR(n, edges)
	csr := NewCSR(n, edges)

	if varint.MemoryBytes() >= csr.MemoryBytes() {
		t.Errorf("varint = %d o, csr = %d o : aucune compression sur un graphe local",
			varint.MemoryBytes(), csr.MemoryBytes())
	}
	perEdge := float64(len(varint.data)) / float64(len(edges))
	if perEdge > 2 {
		t.Errorf("%.2f octets par arête, attendu ~1 sur des écarts unitaires", perEdge)
	}
	t.Logf("écarts unitaires : %.2f octet par arête contre 4 en CSR", perEdge)
}

// TestVarintCSRWorstCase : sur des écarts énormes, l'encodage doit rester
// correct même s'il devient plus coûteux que le CSR.
//
// La taille reste modérée à dessein : le tableau d'offsets est dimensionné sur
// n, un test à 2³⁰ sommets allouerait 4 Gio pour éprouver le même chemin de
// code que 2²², où l'écart tient déjà sur quatre octets de varint.
func TestVarintCSRWorstCase(t *testing.T) {
	const n = 1 << 22
	g := NewVarintCSR(n, mkEdges([2]uint32{0, 1}, [2]uint32{0, n - 1}))
	if nb := g.AppendNeighbors(nil, 0); !slices.Equal(nb, []uint32{1, n - 1}) {
		t.Errorf("voisins = %v, attendu [1 %d]", nb, uint32(n-1))
	}
	if !g.HasEdge(0, n-1) {
		t.Error("HasEdge sur l'écart maximal = false")
	}
}

// TestVarintCSROffsetsCoverData : chaque sommet doit occuper un bloc contigu,
// et le dernier offset fermer exactement le tampon.
func TestVarintCSROffsetsCoverData(t *testing.T) {
	n, edges := sampleEdges()
	g := NewVarintCSR(n, edges)
	if len(g.offsets) != n+1 {
		t.Fatalf("%d offsets, attendu %d", len(g.offsets), n+1)
	}
	for u := 0; u < n; u++ {
		if g.offsets[u] > g.offsets[u+1] {
			t.Fatalf("offsets non croissants en %d", u)
		}
	}
	if int(g.offsets[n]) != len(g.data) {
		t.Errorf("dernier offset = %d, tampon de %d octets", g.offsets[n], len(g.data))
	}
}
