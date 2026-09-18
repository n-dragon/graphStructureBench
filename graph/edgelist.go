package graph

import (
	"slices"
	"sort"
)

// SortedEdgeList stocke les arêtes packées dans un seul uint64
// (source << 32 | cible) et trié. Aucune structure par sommet : le coût
// mémoire est strictement proportionnel au nombre d'arêtes, ce qui en fait la
// représentation la plus compacte sur les graphes très creux avec beaucoup de
// sommets isolés. En contrepartie chaque accès à l'adjacence commence par une
// recherche binaire en O(log m).
type SortedEdgeList struct {
	keys []uint64
	n    int
}

// NewSortedEdgeList trie la liste d'arêtes packées.
func NewSortedEdgeList(n int, edges []Edge) *SortedEdgeList {
	keys := make([]uint64, len(edges))
	for i, e := range edges {
		keys[i] = uint64(e.From)<<32 | uint64(e.To)
	}
	if !slices.IsSorted(keys) {
		slices.Sort(keys)
	}
	return &SortedEdgeList{keys: keys, n: n}
}

func (g *SortedEdgeList) Name() string     { return "edgelist" }
func (g *SortedEdgeList) NumVertices() int { return g.n }
func (g *SortedEdgeList) NumEdges() int    { return len(g.keys) }

// lowerBound renvoie l'indice de la première arête de source u.
func (g *SortedEdgeList) lowerBound(u uint32) int {
	target := uint64(u) << 32
	return sort.Search(len(g.keys), func(i int) bool { return g.keys[i] >= target })
}

func (g *SortedEdgeList) Degree(u uint32) int {
	lo := g.lowerBound(u)
	limit := uint64(u+1) << 32
	hi := lo
	for hi < len(g.keys) && g.keys[hi] < limit {
		hi++
	}
	return hi - lo
}

func (g *SortedEdgeList) ForEachNeighbor(u uint32, fn func(uint32) bool) {
	limit := uint64(u+1) << 32
	for i := g.lowerBound(u); i < len(g.keys); i++ {
		k := g.keys[i]
		if k >= limit {
			return
		}
		if !fn(uint32(k)) {
			return
		}
	}
}

func (g *SortedEdgeList) HasEdge(u, v uint32) bool {
	_, ok := slices.BinarySearch(g.keys, uint64(u)<<32|uint64(v))
	return ok
}

func (g *SortedEdgeList) MemoryBytes() uint64 { return sliceBytes(g.keys, 8) }
