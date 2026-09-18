package graph

import (
	"slices"
	"testing"
)

// slackEdges : des degrés volontairement hors puissances de deux (5, 3, 9),
// pour que la croissance géométrique d'append laisse de la capacité inutilisée.
func slackEdges() (int, []Edge) {
	var pairs [][2]uint32
	add := func(u uint32, deg int) {
		for i := 1; i <= deg; i++ {
			pairs = append(pairs, [2]uint32{u, uint32(i)})
		}
	}
	add(0, 5)
	add(1, 3)
	add(2, 9)
	return 16, mkEdges(pairs...)
}

// TestAdjListExactHasNoSlack : c'est la promesse de la variante « exact ».
func TestAdjListExactHasNoSlack(t *testing.T) {
	n, edges := slackEdges()
	g := NewAdjListExact(n, edges)
	for u := 0; u < n; u++ {
		if cap(g.adj[u]) != len(g.adj[u]) {
			t.Errorf("sommet %d: cap = %d pour len = %d, aucune marge n'est attendue",
				u, cap(g.adj[u]), len(g.adj[u]))
		}
	}
}

// TestAdjListAppendLeavesSlack : et voici ce que la variante « exact » évite.
func TestAdjListAppendLeavesSlack(t *testing.T) {
	n, edges := slackEdges()
	g := NewAdjListAppend(n, edges)
	var totalLen, totalCap int
	for u := 0; u < n; u++ {
		totalLen += len(g.adj[u])
		totalCap += cap(g.adj[u])
	}
	if totalCap <= totalLen {
		t.Fatalf("capacité totale %d pour %d éléments : aucune marge, le scénario ne teste rien",
			totalCap, totalLen)
	}
	t.Logf("croissance par append : %d éléments logés dans %d emplacements (%.0f %% de marge)",
		totalLen, totalCap, 100*float64(totalCap-totalLen)/float64(totalLen))
}

// TestAdjListExactIsLighterThanAppend : l'écart mémoire entre les deux
// variantes est précisément ce que la paire est censée mesurer dans le banc.
func TestAdjListExactIsLighterThanAppend(t *testing.T) {
	n, edges := slackEdges()
	exact := NewAdjListExact(n, edges).MemoryBytes()
	grown := NewAdjListAppend(n, edges).MemoryBytes()
	if exact >= grown {
		t.Errorf("exact = %d o, append = %d o : la variante exacte devrait être la plus légère",
			exact, grown)
	}
}

// TestAdjListVariantsAgree : même contenu malgré des capacités différentes.
func TestAdjListVariantsAgree(t *testing.T) {
	n, edges := slackEdges()
	exact, grown := NewAdjListExact(n, edges), NewAdjListAppend(n, edges)
	for u := 0; u < n; u++ {
		if !slices.Equal(exact.adj[u], grown.adj[u]) {
			t.Errorf("sommet %d: exact = %v, append = %v", u, exact.adj[u], grown.adj[u])
		}
	}
	if exact.Name() == grown.Name() {
		t.Errorf("les deux variantes portent le même nom %q, les rapports les confondraient", exact.Name())
	}
}

// TestAdjListIsolatedVerticesCostNothing : aucune allocation pour un sommet
// sans arête sortante, seulement l'en-tête de slice du tableau.
func TestAdjListIsolatedVerticesCostNothing(t *testing.T) {
	edges := mkEdges([2]uint32{3, 1})
	for name, g := range map[string]*AdjList{
		"exact":  NewAdjListExact(10, edges),
		"append": NewAdjListAppend(10, edges),
	} {
		for _, u := range []uint32{0, 5, 9} {
			if g.adj[u] != nil {
				t.Errorf("%s: le sommet isolé %d a une adjacence allouée (%v)", name, u, g.adj[u])
			}
		}
	}
}

func TestAdjListMemoryBytesFormula(t *testing.T) {
	n, edges := slackEdges()
	g := NewAdjListExact(n, edges)
	want := uint64(sliceHeaderBytes + sliceHeaderBytes*n + 4*len(edges))
	if got := g.MemoryBytes(); got != want {
		t.Errorf("MemoryBytes = %d, attendu %d (24 o/sommet + 4 o/arête sans marge)", got, want)
	}
}
