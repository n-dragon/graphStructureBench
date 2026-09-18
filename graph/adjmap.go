package graph

import "slices"

// AdjMap indexe l'adjacence dans une table de hachage. C'est la structure la
// plus souple (sommets non denses, ajout/suppression faciles) et la plus
// coûteuse : hachage à chaque accès, entrées dispersées en mémoire, et un
// pointeur par entrée à tracer pour le GC.
type AdjMap struct {
	adj map[uint32][]uint32
	n   int
	m   int
}

// NewAdjMap construit l'index sous forme de map.
func NewAdjMap(n int, edges []Edge) *AdjMap {
	adj := make(map[uint32][]uint32)
	for _, e := range edges {
		adj[e.From] = append(adj[e.From], e.To)
	}
	for u, nb := range adj {
		if len(nb) > 1 && !slices.IsSorted(nb) {
			slices.Sort(nb)
			adj[u] = nb
		}
	}
	return &AdjMap{adj: adj, n: n, m: len(edges)}
}

func (g *AdjMap) Name() string     { return "adjmap" }
func (g *AdjMap) NumVertices() int { return g.n }
func (g *AdjMap) NumEdges() int    { return g.m }

func (g *AdjMap) Degree(u uint32) int         { return len(g.adj[u]) }
func (g *AdjMap) Neighbors(u uint32) []uint32 { return g.adj[u] }

func (g *AdjMap) ForEachNeighbor(u uint32, fn func(uint32) bool) {
	for _, v := range g.adj[u] {
		if !fn(v) {
			return
		}
	}
}

func (g *AdjMap) AppendNeighbors(dst []uint32, u uint32) []uint32 {
	return append(dst, g.adj[u]...)
}

func (g *AdjMap) HasEdge(u, v uint32) bool {
	nb, ok := g.adj[u]
	if !ok {
		return false
	}
	_, found := slices.BinarySearch(nb, v)
	return found
}

// MemoryBytes ne peut qu'estimer le coût de la table : la structure interne
// des maps Go (groupes de 8 emplacements, octets de contrôle, facteur de
// charge ~0,7) n'est pas exposée. On compte ~48 octets par entrée pour
// l'emplacement clé+en-tête de slice, plus les blocs de voisins.
func (g *AdjMap) MemoryBytes() uint64 {
	const perEntry = 48
	total := uint64(len(g.adj)) * perEntry
	for _, nb := range g.adj {
		total += uint64(cap(nb)) * 4
	}
	return total
}
