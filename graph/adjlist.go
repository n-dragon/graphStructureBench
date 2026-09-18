package graph

import "slices"

// AdjList est la liste d'adjacence classique : une slice par sommet.
// Deux constructions sont comparées car elles n'ont pas le même coût mémoire :
// la croissance par append laisse jusqu'à ~2x de capacité inutilisée, le
// dimensionnement exact non — au prix d'une passe de comptage supplémentaire.
type AdjList struct {
	name string
	adj  [][]uint32
	m    int
}

// NewAdjListAppend construit la liste d'adjacence à coups d'append, comme on
// l'écrit spontanément.
func NewAdjListAppend(n int, edges []Edge) *AdjList {
	adj := make([][]uint32, n)
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	for u := range adj {
		if len(adj[u]) > 1 && !slices.IsSorted(adj[u]) {
			slices.Sort(adj[u])
		}
	}
	return &AdjList{name: "adjlist", adj: adj, m: len(edges)}
}

// NewAdjListExact alloue chaque liste à sa taille exacte après comptage des
// degrés.
func NewAdjListExact(n int, edges []Edge) *AdjList {
	deg := make([]int, n)
	for _, e := range edges {
		deg[e.From]++
	}
	adj := make([][]uint32, n)
	for u := 0; u < n; u++ {
		if deg[u] > 0 {
			adj[u] = make([]uint32, 0, deg[u])
		}
	}
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	for u := range adj {
		if len(adj[u]) > 1 && !slices.IsSorted(adj[u]) {
			slices.Sort(adj[u])
		}
	}
	return &AdjList{name: "adjlist-exact", adj: adj, m: len(edges)}
}

func (g *AdjList) Name() string     { return g.name }
func (g *AdjList) NumVertices() int { return len(g.adj) }
func (g *AdjList) NumEdges() int    { return g.m }

func (g *AdjList) Degree(u uint32) int         { return len(g.adj[u]) }
func (g *AdjList) Neighbors(u uint32) []uint32 { return g.adj[u] }

func (g *AdjList) ForEachNeighbor(u uint32, fn func(uint32) bool) {
	for _, v := range g.adj[u] {
		if !fn(v) {
			return
		}
	}
}

func (g *AdjList) AppendNeighbors(dst []uint32, u uint32) []uint32 {
	return append(dst, g.adj[u]...)
}

func (g *AdjList) HasEdge(u, v uint32) bool {
	_, ok := slices.BinarySearch(g.adj[u], v)
	return ok
}

func (g *AdjList) MemoryBytes() uint64 {
	total := sliceBytes(g.adj, sliceHeaderBytes)
	for _, nb := range g.adj {
		if cap(nb) > 0 {
			// L'en-tête est déjà compté dans le tableau de slices ; ici seul
			// le bloc de données alloué compte.
			total += uint64(cap(nb)) * 4
		}
	}
	return total
}
