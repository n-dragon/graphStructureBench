package graph

import "testing"

// TestAdjMapSkipsIsolatedVertices : une map ne paie que les sommets qui ont au
// moins une arête sortante. C'est son seul avantage mémoire sur un tableau, et
// il ne se manifeste que sur un espace d'identifiants très creux.
func TestAdjMapSkipsIsolatedVertices(t *testing.T) {
	edges := mkEdges([2]uint32{5, 1}, [2]uint32{5, 2}, [2]uint32{900, 3})
	g := NewAdjMap(1000, edges)

	if len(g.adj) != 2 {
		t.Errorf("%d entrées dans la map, attendu 2 (seuls 5 et 900 émettent)", len(g.adj))
	}
	if g.NumVertices() != 1000 {
		t.Errorf("NumVertices = %d, attendu 1000 : la map est creuse, le graphe non", g.NumVertices())
	}
}

// TestAdjMapUnknownVertex : un sommet absent de la map doit se comporter comme
// un sommet sans voisin, pas provoquer un accès à une entrée nulle.
func TestAdjMapUnknownVertex(t *testing.T) {
	g := NewAdjMap(100, mkEdges([2]uint32{5, 1}))
	for _, u := range []uint32{0, 42, 99} {
		if d := g.Degree(u); d != 0 {
			t.Errorf("Degree(%d) = %d, attendu 0", u, d)
		}
		if g.HasEdge(u, 1) {
			t.Errorf("HasEdge(%d,1) = true, attendu false", u)
		}
		called := false
		g.ForEachNeighbor(u, func(uint32) bool { called = true; return true })
		if called {
			t.Errorf("ForEachNeighbor(%d) a appelé le callback", u)
		}
		if nb := g.AppendNeighbors(nil, u); len(nb) != 0 {
			t.Errorf("AppendNeighbors(%d) = %v, attendu vide", u, nb)
		}
	}
}

// TestAdjMapMemoryCountsEntriesAndBlocks : le compte est une estimation (la
// structure interne des maps Go n'est pas exposée), mais il doit au moins
// suivre le nombre d'entrées et la taille des blocs de voisins.
func TestAdjMapMemoryCountsEntriesAndBlocks(t *testing.T) {
	small := NewAdjMap(1000, mkEdges([2]uint32{5, 1}))
	large := NewAdjMap(1000, mkEdges(
		[2]uint32{5, 1}, [2]uint32{6, 1}, [2]uint32{7, 1}, [2]uint32{8, 1}))
	if large.MemoryBytes() <= small.MemoryBytes() {
		t.Errorf("4 entrées comptées %d o, 1 entrée %d o : le compte ne suit pas",
			large.MemoryBytes(), small.MemoryBytes())
	}
}
