package graph

import (
	"fmt"
	"math/bits"
)

// BitMatrix est la matrice d'adjacence dense, un bit par couple de sommets.
// HasEdge y est en O(1) sans indirection, mais le coût mémoire est en O(n²)
// indépendamment du nombre d'arêtes : 12,5 Go pour un million de sommets.
// Elle n'a de sens que sur de petits graphes denses — le runner la saute
// au-delà de MaxVertices.
type BitMatrix struct {
	bits        []uint64
	n           int
	m           int
	wordsPerRow int
}

// NewBitMatrix alloue la matrice, ou échoue si elle dépasse la limite fixée.
func NewBitMatrix(n int, edges []Edge) (Graph, error) {
	const maxBytes = 1 << 30
	wordsPerRow := (n + 63) / 64
	if total := uint64(n) * uint64(wordsPerRow) * 8; total > maxBytes {
		return nil, fmt.Errorf("bitmatrix: %d sommets demanderaient %.1f Gio", n, float64(total)/(1<<30))
	}
	g := &BitMatrix{bits: make([]uint64, n*wordsPerRow), n: n, m: len(edges), wordsPerRow: wordsPerRow}
	for _, e := range edges {
		g.bits[int(e.From)*wordsPerRow+int(e.To>>6)] |= 1 << uint(e.To&63)
	}
	return g, nil
}

func (g *BitMatrix) Name() string     { return "bitmatrix" }
func (g *BitMatrix) NumVertices() int { return g.n }
func (g *BitMatrix) NumEdges() int    { return g.m }

func (g *BitMatrix) row(u uint32) []uint64 {
	base := int(u) * g.wordsPerRow
	return g.bits[base : base+g.wordsPerRow]
}

func (g *BitMatrix) Degree(u uint32) int {
	d := 0
	for _, w := range g.row(u) {
		d += bits.OnesCount64(w)
	}
	return d
}

func (g *BitMatrix) ForEachNeighbor(u uint32, fn func(uint32) bool) {
	for i, w := range g.row(u) {
		for w != 0 {
			b := bits.TrailingZeros64(w)
			if !fn(uint32(i*64 + b)) {
				return
			}
			w &= w - 1
		}
	}
}

func (g *BitMatrix) HasEdge(u, v uint32) bool {
	return g.bits[int(u)*g.wordsPerRow+int(v>>6)]&(1<<uint(v&63)) != 0
}

func (g *BitMatrix) MemoryBytes() uint64 { return sliceBytes(g.bits, 8) }
