package graph_test

import (
	"slices"
	"testing"

	"github.com/n-dragon/graphstructurebench/graph"
)

// Ce fichier vérifie le contrat commun aux neuf structures sur un corpus de
// petits graphes choisis pour leurs cas limites. Les graphes sont assez petits
// pour que HasEdge soit vérifié **exhaustivement**, sur les n² couples.

type scenario struct {
	name  string
	n     int
	edges []graph.Edge
}

func e(from, to uint32) graph.Edge { return graph.Edge{From: from, To: to} }

func scenarios() []scenario {
	star := func(center uint32, n int, outgoing bool) []graph.Edge {
		var out []graph.Edge
		for v := 0; v < n; v++ {
			if uint32(v) == center {
				continue
			}
			if outgoing {
				out = append(out, e(center, uint32(v)))
			} else {
				out = append(out, e(uint32(v), center))
			}
		}
		return out
	}
	chain := func(n int) []graph.Edge {
		var out []graph.Edge
		for v := 0; v+1 < n; v++ {
			out = append(out, e(uint32(v), uint32(v+1)), e(uint32(v+1), uint32(v)))
		}
		return out
	}
	complete := func(n int) []graph.Edge {
		var out []graph.Edge
		for u := 0; u < n; u++ {
			for v := 0; v < n; v++ {
				if u != v {
					out = append(out, e(uint32(u), uint32(v)))
				}
			}
		}
		return out
	}
	hubAndLeaves := func(n int) []graph.Edge {
		var out []graph.Edge
		for v := 1; v < n/2; v++ { // sommet 0 très connecté
			out = append(out, e(0, uint32(v)), e(uint32(v), 0))
		}
		out = append(out, e(uint32(n-2), uint32(n-1))) // et une feuille à l'écart
		return out
	}

	scs := []scenario{
		{"graphe vide", 5, nil},
		{"sommet unique sans arête", 1, nil},
		{"boucle sur soi", 3, []graph.Edge{e(0, 0)}},
		{"tous les sommets bouclés", 4, []graph.Edge{e(0, 0), e(1, 1), e(2, 2), e(3, 3)}},
		{"isolés aux deux bouts", 6, []graph.Edge{e(2, 3), e(3, 2)}},
		{"dernier sommet source", 4, []graph.Edge{e(3, 0), e(3, 3)}},
		{"premier sommet seul actif", 4, []graph.Edge{e(0, 3)}},
		{"étoile sortante", 8, star(0, 8, true)},
		{"étoile entrante", 8, star(0, 8, false)},
		{"chaîne", 6, chain(6)},
		{"graphe complet", 5, complete(5)},
		{"hub et feuilles", 64, hubAndLeaves(64)},
	}
	// Les constructeurs supposent une liste dédupliquée et triée.
	for i := range scs {
		scs[i].edges = graph.Dedup(scs[i].edges)
	}
	return scs
}

// refAdj est l'adjacence de référence, construite naïvement.
func refAdj(sc scenario) [][]uint32 {
	adj := make([][]uint32, sc.n)
	for _, ed := range sc.edges {
		adj[ed.From] = append(adj[ed.From], ed.To)
	}
	for _, nb := range adj {
		slices.Sort(nb)
	}
	return adj
}

// forEachBuilder exécute fn pour chaque structure constructible sur ce scénario.
func forEachBuilder(t *testing.T, sc scenario, fn func(t *testing.T, g graph.Graph, ref [][]uint32)) {
	t.Helper()
	ref := refAdj(sc)
	for _, b := range graph.Builders {
		if b.MaxVertices > 0 && sc.n > b.MaxVertices {
			continue
		}
		g, err := b.Build(sc.n, sc.edges)
		if err != nil {
			t.Fatalf("%s: construction impossible: %v", b.Name, err)
		}
		t.Run(b.Name, func(t *testing.T) { fn(t, g, ref) })
	}
}

func collect(g graph.Graph, u uint32) []uint32 {
	var got []uint32
	g.ForEachNeighbor(u, func(v uint32) bool {
		got = append(got, v)
		return true
	})
	return got
}

func TestContractNeighbors(t *testing.T) {
	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			forEachBuilder(t, sc, func(t *testing.T, g graph.Graph, ref [][]uint32) {
				var buf []uint32
				for u := 0; u < sc.n; u++ {
					if got := collect(g, uint32(u)); !slices.Equal(got, ref[u]) {
						t.Errorf("ForEachNeighbor(%d) = %v, attendu %v", u, got, ref[u])
					}
					buf = g.AppendNeighbors(buf[:0], uint32(u))
					if !slices.Equal(buf, ref[u]) {
						t.Errorf("AppendNeighbors(%d) = %v, attendu %v", u, buf, ref[u])
					}
					if d := g.Degree(uint32(u)); d != len(ref[u]) {
						t.Errorf("Degree(%d) = %d, attendu %d", u, d, len(ref[u]))
					}
				}
				if g.NumVertices() != sc.n {
					t.Errorf("NumVertices = %d, attendu %d", g.NumVertices(), sc.n)
				}
				if g.NumEdges() != len(sc.edges) {
					t.Errorf("NumEdges = %d, attendu %d", g.NumEdges(), len(sc.edges))
				}
			})
		})
	}
}

// TestContractHasEdgeExhaustive vérifie HasEdge sur tous les couples possibles :
// sur ces tailles, l'exhaustivité coûte moins cher qu'un tirage aléatoire et
// ne laisse aucun angle mort.
func TestContractHasEdgeExhaustive(t *testing.T) {
	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			forEachBuilder(t, sc, func(t *testing.T, g graph.Graph, ref [][]uint32) {
				for u := 0; u < sc.n; u++ {
					for v := 0; v < sc.n; v++ {
						want := slices.Contains(ref[u], uint32(v))
						if got := g.HasEdge(uint32(u), uint32(v)); got != want {
							t.Fatalf("HasEdge(%d,%d) = %v, attendu %v", u, v, got, want)
						}
					}
				}
			})
		})
	}
}

// TestContractEarlyStop : un callback qui renvoie false doit interrompre
// l'itération immédiatement. C'est ce qui permet à un parcours de sortir tôt,
// et chaque structure doit le respecter, y compris celles qui itèrent sur des
// bits ou décodent un flux.
func TestContractEarlyStop(t *testing.T) {
	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			forEachBuilder(t, sc, func(t *testing.T, g graph.Graph, ref [][]uint32) {
				for u := 0; u < sc.n; u++ {
					for _, stopAfter := range []int{1, 2, 3} {
						if len(ref[u]) < stopAfter {
							continue
						}
						var seen []uint32
						g.ForEachNeighbor(uint32(u), func(v uint32) bool {
							seen = append(seen, v)
							return len(seen) < stopAfter
						})
						if len(seen) != stopAfter {
							t.Fatalf("arrêt après %d voisins de %d : %d appels reçus",
								stopAfter, u, len(seen))
						}
						if !slices.Equal(seen, ref[u][:stopAfter]) {
							t.Fatalf("arrêt après %d voisins de %d = %v, attendu %v",
								stopAfter, u, seen, ref[u][:stopAfter])
						}
					}
				}
			})
		})
	}
}

// TestContractAppendPreservesPrefix : AppendNeighbors ajoute, il ne tronque
// pas. Un appelant doit pouvoir accumuler plusieurs adjacences dans le même
// tampon.
func TestContractAppendPreservesPrefix(t *testing.T) {
	for _, sc := range scenarios() {
		t.Run(sc.name, func(t *testing.T) {
			forEachBuilder(t, sc, func(t *testing.T, g graph.Graph, ref [][]uint32) {
				sentinel := []uint32{111, 222}
				var want []uint32
				got := slices.Clone(sentinel)
				want = append(want, sentinel...)
				for u := 0; u < sc.n; u++ {
					got = g.AppendNeighbors(got, uint32(u))
					want = append(want, ref[u]...)
				}
				if !slices.Equal(got, want) {
					t.Errorf("accumulation dans un tampon commun = %v, attendu %v", got, want)
				}
			})
		})
	}
}

// TestContractMemoryReported : une structure qui contient des arêtes doit
// déclarer une empreinte non nulle, faute de quoi le critère mémoire du banc
// ne veut rien dire.
//
// Le contrat s'arrête là : aucune borne inférieure par arête n'est exigible,
// puisque c'est justement ce que les représentations compactes (bitmap,
// varint) descendent en dessous. L'exactitude du compte se vérifie structure
// par structure, contre la formule de chacune.
func TestContractMemoryReported(t *testing.T) {
	for _, sc := range scenarios() {
		if len(sc.edges) == 0 {
			continue
		}
		t.Run(sc.name, func(t *testing.T) {
			forEachBuilder(t, sc, func(t *testing.T, g graph.Graph, ref [][]uint32) {
				if g.MemoryBytes() == 0 {
					t.Errorf("MemoryBytes = 0 alors que la structure porte %d arêtes", len(sc.edges))
				}
			})
		})
	}
}

// TestDedup : la précondition des constructeurs.
func TestDedup(t *testing.T) {
	in := []graph.Edge{e(3, 1), e(0, 2), e(3, 1), e(0, 2), e(1, 0), e(0, 2)}
	want := []graph.Edge{e(0, 2), e(1, 0), e(3, 1)}
	if got := graph.Dedup(in); !slices.Equal(got, want) {
		t.Errorf("Dedup = %v, attendu %v", got, want)
	}
	if got := graph.Dedup(nil); len(got) != 0 {
		t.Errorf("Dedup(nil) = %v, attendu vide", got)
	}
}
