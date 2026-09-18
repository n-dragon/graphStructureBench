package graph

import "slices"

// buildCSRArrays construit la forme canonique CSR (tri par comptage en O(n+m))
// avec les listes d'adjacence triées, ce qui autorise la recherche binaire.
func buildCSRArrays(n int, edges []Edge) (offsets []uint32, targets []uint32) {
	offsets = make([]uint32, n+1)
	for _, e := range edges {
		offsets[e.From+1]++
	}
	for i := 0; i < n; i++ {
		offsets[i+1] += offsets[i]
	}
	targets = make([]uint32, len(edges))
	cursor := make([]uint32, n)
	copy(cursor, offsets[:n])
	for _, e := range edges {
		targets[cursor[e.From]] = e.To
		cursor[e.From]++
	}
	for u := 0; u < n; u++ {
		block := targets[offsets[u]:offsets[u+1]]
		if len(block) > 1 && !slices.IsSorted(block) {
			slices.Sort(block)
		}
	}
	return offsets, targets
}

// CSR (Compressed Sparse Row) : deux tableaux plats, aucun pointeur, donc
// aucun travail pour le GC et une localité parfaite en parcours séquentiel.
type CSR struct {
	offsets []uint32 // n+1 bornes
	targets []uint32 // m cibles, groupées par source et triées
}

// NewCSR indexe les arêtes au format CSR.
func NewCSR(n int, edges []Edge) *CSR {
	off, tgt := buildCSRArrays(n, edges)
	return &CSR{offsets: off, targets: tgt}
}

func (g *CSR) Name() string     { return "csr" }
func (g *CSR) NumVertices() int { return len(g.offsets) - 1 }
func (g *CSR) NumEdges() int    { return len(g.targets) }

func (g *CSR) Degree(u uint32) int { return int(g.offsets[u+1] - g.offsets[u]) }

// Neighbors rend la tranche d'adjacence sans copie.
func (g *CSR) Neighbors(u uint32) []uint32 { return g.targets[g.offsets[u]:g.offsets[u+1]] }

func (g *CSR) ForEachNeighbor(u uint32, fn func(uint32) bool) {
	for _, v := range g.targets[g.offsets[u]:g.offsets[u+1]] {
		if !fn(v) {
			return
		}
	}
}

func (g *CSR) HasEdge(u, v uint32) bool {
	block := g.targets[g.offsets[u]:g.offsets[u+1]]
	_, ok := slices.BinarySearch(block, v)
	return ok
}

func (g *CSR) MemoryBytes() uint64 {
	return sliceBytes(g.offsets, 4) + sliceBytes(g.targets, 4)
}

// CSRSlices garde l'arène plate du CSR mais remplace le tableau d'offsets par
// un en-tête de slice par sommet : +24 octets par sommet contre un
// déréférencement de moins et une adjacence directement utilisable en slice.
type CSRSlices struct {
	arena []uint32
	adj   [][]uint32
}

// NewCSRSlices indexe les arêtes dans une arène unique découpée en slices.
func NewCSRSlices(n int, edges []Edge) *CSRSlices {
	off, tgt := buildCSRArrays(n, edges)
	adj := make([][]uint32, n)
	for u := 0; u < n; u++ {
		lo, hi := off[u], off[u+1]
		adj[u] = tgt[lo:hi:hi]
	}
	return &CSRSlices{arena: tgt, adj: adj}
}

func (g *CSRSlices) Name() string     { return "csr-slices" }
func (g *CSRSlices) NumVertices() int { return len(g.adj) }
func (g *CSRSlices) NumEdges() int    { return len(g.arena) }

func (g *CSRSlices) Degree(u uint32) int         { return len(g.adj[u]) }
func (g *CSRSlices) Neighbors(u uint32) []uint32 { return g.adj[u] }

func (g *CSRSlices) ForEachNeighbor(u uint32, fn func(uint32) bool) {
	for _, v := range g.adj[u] {
		if !fn(v) {
			return
		}
	}
}

func (g *CSRSlices) HasEdge(u, v uint32) bool {
	_, ok := slices.BinarySearch(g.adj[u], v)
	return ok
}

func (g *CSRSlices) MemoryBytes() uint64 {
	return sliceBytes(g.arena, 4) + sliceBytes(g.adj, sliceHeaderBytes)
}
