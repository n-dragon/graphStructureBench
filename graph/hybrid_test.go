package graph

import (
	"slices"
	"testing"
)

// starFrom construit une étoile sortante depuis center vers deg sommets.
func starFrom(center uint32, deg int) [][2]uint32 {
	var pairs [][2]uint32
	for v := 1; v <= deg; v++ {
		pairs = append(pairs, [2]uint32{center, uint32(v) * 3 % 1000})
	}
	return pairs
}

// TestHybridThresholdRule : la règle de promotion est dérivée du point
// d'équilibre mémoire (4·deg contre n/8 octets), soit deg > n/32. Un sommet
// juste en dessous ne doit pas être promu, un sommet juste au-dessus doit
// l'être.
func TestHybridThresholdRule(t *testing.T) {
	const n = 1024 // seuil = 32
	if got := hubThreshold(n); got != 32 {
		t.Fatalf("hubThreshold(%d) = %d, attendu 32", n, got)
	}

	below := NewHybrid(n, Dedup(edgesOf(starFrom(0, 32))))
	if below.NumHubs() != 0 {
		t.Errorf("degré 32 (= seuil) : %d hub(s), attendu 0", below.NumHubs())
	}
	above := NewHybrid(n, Dedup(edgesOf(starFrom(0, 33))))
	if above.NumHubs() != 1 {
		t.Errorf("degré 33 (> seuil) : %d hub(s), attendu 1", above.NumHubs())
	}
	if !above.hub(0) {
		t.Error("le sommet 0 devrait être marqué comme hub")
	}
	if above.hub(1) {
		t.Error("le sommet 1 ne devrait pas être marqué comme hub")
	}
}

// TestHybridHubAnswersFromBitmap : un hub répond depuis son bitmap, les
// voisins sortent triés et son bloc CSR est vide (pas de double stockage).
func TestHybridHubAnswersFromBitmap(t *testing.T) {
	const n = 1024
	pairs := starFrom(0, 40)
	pairs = append(pairs, [2]uint32{500, 1}, [2]uint32{500, 2}) // un non-hub
	edges := Dedup(edgesOf(pairs))
	g := NewHybrid(n, edges)

	if g.NumHubs() != 1 {
		t.Fatalf("%d hub(s), attendu 1", g.NumHubs())
	}
	if lo, hi := g.offsets[0], g.offsets[1]; lo != hi {
		t.Errorf("le bloc CSR du hub contient %d cibles, il devrait être vide", hi-lo)
	}

	var want []uint32
	for _, e := range edges {
		if e.From == 0 {
			want = append(want, e.To)
		}
	}
	got := g.AppendNeighbors(nil, 0)
	if !slices.Equal(got, want) {
		t.Errorf("voisins du hub = %v, attendu %v", got, want)
	}
	if !slices.IsSorted(got) {
		t.Error("le balayage de bitmap doit rendre les voisins triés")
	}
	if !g.hub(0) || g.hub(500) {
		t.Error("classement hub/non-hub incorrect")
	}
	if !g.HasEdge(500, 1) || g.HasEdge(500, 999) {
		t.Error("le chemin CSR des non-hubs répond faux")
	}
}

// TestHybridNoHubsOnUniformDegrees : sur des degrés homogènes, aucun sommet ne
// franchit le seuil. La structure se réduit alors à un CSR plus un bitset —
// c'est le comportement attendu, pas une défaillance.
func TestHybridNoHubsOnUniformDegrees(t *testing.T) {
	const n = 2048
	var pairs [][2]uint32
	for u := uint32(0); u < n; u++ {
		for k := uint32(1); k <= 8; k++ {
			pairs = append(pairs, [2]uint32{u, (u + k) % n})
		}
	}
	g := NewHybrid(n, Dedup(edgesOf(pairs)))
	if g.NumHubs() != 0 {
		t.Errorf("%d hub(s) sur des degrés uniformes à 8, attendu 0", g.NumHubs())
	}
	if len(g.hubBits) != 0 {
		t.Errorf("%d mots de bitmap alloués sans aucun hub", len(g.hubBits))
	}
}

// TestHybridBitmapsNeverCostMoreThanTheyReplace : c'est l'invariant qui rend
// la structure sûre en mémoire. Il découle du seuil (promouvoir n'arrive que
// si le bitmap est plus compact que les cibles remplacées), et le plafond de
// construction n'est qu'une ceinture de sécurité.
func TestHybridBitmapsNeverCostMoreThanTheyReplace(t *testing.T) {
	cases := []struct {
		name  string
		n     int
		pairs [][2]uint32
	}{
		{"un gros hub", 1024, starFrom(0, 400)},
		{"plusieurs hubs", 512, append(append(starFrom(0, 100), starFrom(1, 100)...), starFrom(2, 100)...)},
		{"hubs au seuil", 256, append(starFrom(0, 9), starFrom(1, 9)...)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			edges := Dedup(edgesOf(c.pairs))
			g := NewHybrid(c.n, edges)
			bitmapBytes := uint64(len(g.hubBits)) * 8
			replaced := uint64(len(edges)-len(g.targets)) * 4
			if bitmapBytes > replaced {
				t.Errorf("%d o de bitmaps pour %d o de cibles remplacées (%d hubs)",
					bitmapBytes, replaced, g.NumHubs())
			}
		})
	}
}

// TestHybridDegreeMatchesPopcount : le degré d'un hub se lit dans le bitmap.
func TestHybridDegreeMatchesPopcount(t *testing.T) {
	const n = 1024
	pairs := starFrom(0, 50)
	edges := Dedup(edgesOf(pairs))
	g := NewHybrid(n, edges)
	var want int
	for _, e := range edges {
		if e.From == 0 {
			want++
		}
	}
	if got := g.Degree(0); got != want {
		t.Errorf("Degree du hub = %d, attendu %d", got, want)
	}
}

func edgesOf(pairs [][2]uint32) []Edge {
	out := make([]Edge, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, Edge{From: p[0], To: p[1]})
	}
	return out
}
