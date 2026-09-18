package graph

import (
	"math/bits"
	"slices"
)

// Hybrid applique une représentation différente selon le degré du sommet.
//
// Un sommet coûte 4·deg octets en CSR et n/8 octets en bitmap ; le bitmap
// devient donc plus compact dès que deg > n/32, et il répond à HasEdge en
// O(1) au lieu d'un O(log deg). C'est exactement le régime des « hubs » des
// graphes en loi de puissance (web, réseaux sociaux), où une poignée de
// sommets concentre une grande part des arêtes.
//
// Les autres sommets restent en CSR.
type Hybrid struct {
	offsets     []uint32 // CSR des sommets non-hubs (bloc vide pour un hub)
	targets     []uint32
	isHub       []uint64 // bitset : ce sommet est-il un hub ?
	hubRow      []int32  // -1, ou l'indice de ligne dans hubBits
	hubBits     []uint64 // len(hubs) lignes de wordsPerRow mots
	wordsPerRow int
	numHubs     int
	m           int
}

// hubThreshold est le degré à partir duquel un bitmap coûte moins cher que la
// liste d'adjacence.
func hubThreshold(n int) int { return n / 32 }

// NewHybrid choisit la représentation de chaque sommet selon son degré.
func NewHybrid(n int, edges []Edge) *Hybrid {
	deg := make([]int32, n)
	for _, e := range edges {
		deg[e.From]++
	}
	thr := int32(hubThreshold(n))
	if thr < 1 {
		thr = 1 << 30 // graphe minuscule : aucun hub
	}
	wordsPerRow := (n + 63) / 64
	hubRow := make([]int32, n)
	isHub := make([]uint64, (n+63)/64)
	numHubs := 0
	// Budget de sécurité : les bitmaps ne doivent jamais dépasser la taille du
	// tableau de cibles qu'ils remplacent.
	budgetWords := len(edges)/2 + 1
	usedWords := 0
	for u := 0; u < n; u++ {
		hubRow[u] = -1
		if deg[u] > thr && usedWords+wordsPerRow <= budgetWords {
			hubRow[u] = int32(numHubs)
			isHub[u>>6] |= 1 << uint(u&63)
			numHubs++
			usedWords += wordsPerRow
		}
	}

	hubBits := make([]uint64, numHubs*wordsPerRow)
	// CSR restreint aux non-hubs.
	rest := make([]Edge, 0, len(edges))
	for _, e := range edges {
		if r := hubRow[e.From]; r >= 0 {
			hubBits[int(r)*wordsPerRow+int(e.To>>6)] |= 1 << uint(e.To&63)
		} else {
			rest = append(rest, e)
		}
	}
	off, tgt := buildCSRArrays(n, rest)

	return &Hybrid{
		offsets: off, targets: tgt,
		isHub: isHub, hubRow: hubRow, hubBits: hubBits,
		wordsPerRow: wordsPerRow, numHubs: numHubs, m: len(edges),
	}
}

func (g *Hybrid) Name() string     { return "hybrid" }
func (g *Hybrid) NumVertices() int { return len(g.offsets) - 1 }
func (g *Hybrid) NumEdges() int    { return g.m }

// NumHubs expose le nombre de sommets promus en bitmap (utile au rapport).
func (g *Hybrid) NumHubs() int { return g.numHubs }

func (g *Hybrid) hub(u uint32) bool { return g.isHub[u>>6]&(1<<uint(u&63)) != 0 }

func (g *Hybrid) row(u uint32) []uint64 {
	r := int(g.hubRow[u])
	return g.hubBits[r*g.wordsPerRow : (r+1)*g.wordsPerRow]
}

func (g *Hybrid) Degree(u uint32) int {
	if !g.hub(u) {
		return int(g.offsets[u+1] - g.offsets[u])
	}
	d := 0
	for _, w := range g.row(u) {
		d += bits.OnesCount64(w)
	}
	return d
}

func (g *Hybrid) ForEachNeighbor(u uint32, fn func(uint32) bool) {
	if !g.hub(u) {
		for _, v := range g.targets[g.offsets[u]:g.offsets[u+1]] {
			if !fn(v) {
				return
			}
		}
		return
	}
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

func (g *Hybrid) HasEdge(u, v uint32) bool {
	if g.hub(u) {
		return g.row(u)[v>>6]&(1<<uint(v&63)) != 0
	}
	_, ok := slices.BinarySearch(g.targets[g.offsets[u]:g.offsets[u+1]], v)
	return ok
}

func (g *Hybrid) MemoryBytes() uint64 {
	return sliceBytes(g.offsets, 4) + sliceBytes(g.targets, 4) +
		sliceBytes(g.isHub, 8) + sliceBytes(g.hubRow, 4) + sliceBytes(g.hubBits, 8)
}
