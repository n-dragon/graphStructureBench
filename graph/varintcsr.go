package graph

import "encoding/binary"

// VarintCSR compresse chaque liste d'adjacence triée : on encode le degré,
// puis le premier voisin, puis les écarts successifs, en varint LEB128.
// Sur un graphe dense en écarts faibles (grille, graphe renuméroté par
// localité) la plupart des voisins tiennent sur 1 ou 2 octets au lieu de 4.
// Le décodage coûte du CPU mais divise le volume lu en mémoire : sur les
// charges limitées par la bande passante, le compromis peut être gagnant.
//
// Les bornes sont des uint32 : la limite est de 4 Gio de données encodées,
// soit de l'ordre du milliard d'arêtes.
type VarintCSR struct {
	offsets []uint32 // n+1 bornes dans data
	data    []byte
	m       int
}

// NewVarintCSR construit l'index compressé.
func NewVarintCSR(n int, edges []Edge) *VarintCSR {
	off, tgt := buildCSRArrays(n, edges)
	// 5 octets suffisent au pire pour un uint32 en LEB128, + le degré.
	buf := make([]byte, 0, len(tgt)*2+n*2)
	offsets := make([]uint32, n+1)
	var tmp [binary.MaxVarintLen64]byte
	putUvarint := func(x uint64) {
		k := binary.PutUvarint(tmp[:], x)
		buf = append(buf, tmp[:k]...)
	}
	for u := 0; u < n; u++ {
		offsets[u] = uint32(len(buf))
		block := tgt[off[u]:off[u+1]]
		putUvarint(uint64(len(block)))
		var prev uint32
		for i, v := range block {
			if i == 0 {
				putUvarint(uint64(v))
			} else {
				putUvarint(uint64(v - prev))
			}
			prev = v
		}
	}
	offsets[n] = uint32(len(buf))
	return &VarintCSR{offsets: offsets, data: buf, m: len(tgt)}
}

func (g *VarintCSR) Name() string     { return "varint-csr" }
func (g *VarintCSR) NumVertices() int { return len(g.offsets) - 1 }
func (g *VarintCSR) NumEdges() int    { return g.m }

func (g *VarintCSR) Degree(u uint32) int {
	deg, _ := binary.Uvarint(g.data[g.offsets[u]:g.offsets[u+1]])
	return int(deg)
}

func (g *VarintCSR) ForEachNeighbor(u uint32, fn func(uint32) bool) {
	block := g.data[g.offsets[u]:g.offsets[u+1]]
	deg, k := binary.Uvarint(block)
	block = block[k:]
	var cur uint32
	for i := uint64(0); i < deg; i++ {
		d, k := binary.Uvarint(block)
		block = block[k:]
		if i == 0 {
			cur = uint32(d)
		} else {
			cur += uint32(d)
		}
		if !fn(cur) {
			return
		}
	}
}

func (g *VarintCSR) HasEdge(u, v uint32) bool {
	block := g.data[g.offsets[u]:g.offsets[u+1]]
	deg, k := binary.Uvarint(block)
	block = block[k:]
	var cur uint32
	for i := uint64(0); i < deg; i++ {
		d, k := binary.Uvarint(block)
		block = block[k:]
		if i == 0 {
			cur = uint32(d)
		} else {
			cur += uint32(d)
		}
		if cur == v {
			return true
		}
		if cur > v { // liste triée : inutile d'aller plus loin
			return false
		}
	}
	return false
}

func (g *VarintCSR) MemoryBytes() uint64 {
	return sliceBytes(g.offsets, 4) + sliceBytes(g.data, 1)
}
