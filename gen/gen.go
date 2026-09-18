// Package gen produit des graphes synthétiques aux topologies contrastées :
// la structure d'index gagnante dépend fortement de la forme du graphe.
package gen

import (
	"fmt"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/n-dragon/graphstructurebench/graph"
)

// Spec décrit le graphe à engendrer.
type Spec struct {
	Kind       string // "er", "rmat", "grid"
	N          int    // nombre de sommets demandé (ajusté pour rmat et grid)
	AvgDegree  int    // degré sortant moyen visé
	Undirected bool   // ajoute l'arête inverse
	Seed       uint64
}

// Graph est un graphe engendré, prêt à être indexé.
type Graph struct {
	Spec  Spec
	N     int
	Edges []graph.Edge
}

// Label décrit le graphe en une ligne.
func (g Graph) Label() string {
	return fmt.Sprintf("%s n=%d m=%d", g.Spec.Kind, g.N, len(g.Edges))
}

// Generate construit le graphe décrit par spec. Les arêtes sont triées et
// dédupliquées : toutes les structures voient exactement le même ensemble.
func Generate(s Spec) (Graph, error) {
	if s.AvgDegree <= 0 {
		s.AvgDegree = 8
	}
	r := rand.New(rand.NewPCG(s.Seed, s.Seed^0x9e3779b97f4a7c15))

	var n int
	var edges []graph.Edge
	switch s.Kind {
	case "er":
		n = s.N
		edges = erdosRenyi(r, n, s.AvgDegree)
	case "rmat":
		n, edges = rmat(r, s.N, s.AvgDegree)
	case "grid":
		n, edges = grid(s.N)
	default:
		return Graph{}, fmt.Errorf("topologie inconnue: %q (er, rmat, grid)", s.Kind)
	}

	if s.Undirected {
		rev := make([]graph.Edge, 0, len(edges)*2)
		rev = append(rev, edges...)
		for _, e := range edges {
			rev = append(rev, graph.Edge{From: e.To, To: e.From})
		}
		edges = rev
	}
	edges = dedup(edges)
	return Graph{Spec: s, N: n, Edges: edges}, nil
}

// erdosRenyi : arêtes uniformes. Degrés homogènes, aucune localité — le pire
// cas pour les caches, la référence pour comparer les structures.
func erdosRenyi(r *rand.Rand, n, avgDeg int) []graph.Edge {
	m := n * avgDeg
	edges := make([]graph.Edge, 0, m)
	for i := 0; i < m; i++ {
		u := uint32(r.IntN(n))
		v := uint32(r.IntN(n))
		if u == v {
			continue
		}
		edges = append(edges, graph.Edge{From: u, To: v})
	}
	return edges
}

// rmat : générateur récursif (paramètres Graph500) donnant une distribution
// des degrés en loi de puissance — quelques hubs énormes, une majorité de
// sommets de degré 1 ou 2. C'est la forme des graphes web et sociaux.
// n est arrondi à la puissance de deux supérieure.
func rmat(r *rand.Rand, nWanted, avgDeg int) (int, []graph.Edge) {
	scale := 1
	for 1<<scale < nWanted {
		scale++
	}
	n := 1 << scale
	m := n * avgDeg
	const a, b, c = 0.57, 0.19, 0.19 // d = 0.05
	edges := make([]graph.Edge, 0, m)
	for i := 0; i < m; i++ {
		var u, v uint32
		for step := 0; step < scale; step++ {
			bit := uint32(1) << uint(scale-step-1)
			switch p := r.Float64(); {
			case p < a:
			case p < a+b:
				v |= bit
			case p < a+b+c:
				u |= bit
			default:
				u |= bit
				v |= bit
			}
		}
		if u == v {
			continue
		}
		edges = append(edges, graph.Edge{From: u, To: v})
	}
	return n, edges
}

// grid : grille 2D à 4 voisins. Degrés constants et forte localité : les
// voisins d'un sommet ont des identifiants proches, ce qui avantage les
// représentations compressées par écarts.
func grid(nWanted int) (int, []graph.Edge) {
	k := int(math.Round(math.Sqrt(float64(nWanted))))
	if k < 2 {
		k = 2
	}
	n := k * k
	edges := make([]graph.Edge, 0, 4*n)
	id := func(x, y int) uint32 { return uint32(y*k + x) }
	for y := 0; y < k; y++ {
		for x := 0; x < k; x++ {
			u := id(x, y)
			if x+1 < k {
				edges = append(edges, graph.Edge{From: u, To: id(x+1, y)}, graph.Edge{From: id(x+1, y), To: u})
			}
			if y+1 < k {
				edges = append(edges, graph.Edge{From: u, To: id(x, y+1)}, graph.Edge{From: id(x, y+1), To: u})
			}
		}
	}
	return n, edges
}

// dedup trie les arêtes et supprime les doublons.
func dedup(edges []graph.Edge) []graph.Edge {
	keys := make([]uint64, len(edges))
	for i, e := range edges {
		keys[i] = uint64(e.From)<<32 | uint64(e.To)
	}
	slices.Sort(keys)
	keys = slices.Compact(keys)
	out := make([]graph.Edge, len(keys))
	for i, k := range keys {
		out[i] = graph.Edge{From: uint32(k >> 32), To: uint32(k)}
	}
	return out
}
