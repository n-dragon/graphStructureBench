// Package graph fournit plusieurs structures d'index pour représenter un
// graphe orienté et le parcourir. Toutes exposent la même interface Graph,
// ce qui permet de les comparer sur les mêmes charges de travail.
package graph

import "fmt"

// Edge est une arête orientée.
type Edge struct {
	From, To uint32
}

// Graph est le contrat commun à toutes les structures d'index.
//
// ForEachNeighbor est l'opération centrale d'un parcours : elle est appelée
// une fois par sommet visité. Le callback renvoie false pour interrompre
// l'itération.
type Graph interface {
	Name() string
	NumVertices() int
	NumEdges() int
	Degree(u uint32) int
	ForEachNeighbor(u uint32, fn func(v uint32) bool)
	HasEdge(u, v uint32) bool

	// MemoryBytes est le coût mémoire analytique de la structure (somme des
	// tailles des tableaux sous-jacents). C'est une estimation : la mesure
	// de référence reste le delta de tas relevé par le runner.
	MemoryBytes() uint64
}

// SliceGraph est implémentée par les structures capables de rendre la liste
// d'adjacence sans copie. Le runner s'en sert pour mesurer le surcoût du
// callback de ForEachNeighbor.
type SliceGraph interface {
	Graph
	Neighbors(u uint32) []uint32
}

// Builder décrit une structure constructible depuis une liste d'arêtes.
type Builder struct {
	Name string
	Desc string
	// MaxVertices > 0 signale une structure qui ne passe pas à l'échelle
	// (matrice dense) ; le runner la saute au-delà.
	MaxVertices int
	Build       func(n int, edges []Edge) (Graph, error)
}

// Builders liste les structures dans l'ordre d'affichage des rapports.
var Builders = []Builder{
	{Name: "adjmap", Desc: "map[uint32][]uint32 (le réflexe idiomatique)", Build: func(n int, e []Edge) (Graph, error) { return NewAdjMap(n, e), nil }},
	{Name: "adjlist", Desc: "[][]uint32 construit par append (capacités en excès)", Build: func(n int, e []Edge) (Graph, error) { return NewAdjListAppend(n, e), nil }},
	{Name: "adjlist-exact", Desc: "[][]uint32 dimensionné exactement", Build: func(n int, e []Edge) (Graph, error) { return NewAdjListExact(n, e), nil }},
	{Name: "csr", Desc: "CSR : offsets []uint32 + cibles []uint32", Build: func(n int, e []Edge) (Graph, error) { return NewCSR(n, e), nil }},
	{Name: "csr-slices", Desc: "arène unique + en-têtes de slice par sommet", Build: func(n int, e []Edge) (Graph, error) { return NewCSRSlices(n, e), nil }},
	{Name: "edgelist", Desc: "[]uint64 (src<<32|dst) trié + recherche binaire", Build: func(n int, e []Edge) (Graph, error) { return NewSortedEdgeList(n, e), nil }},
	{Name: "varint-csr", Desc: "CSR compressé : deltas encodés en varint", Build: func(n int, e []Edge) (Graph, error) { return NewVarintCSR(n, e), nil }},
	{Name: "hybrid", Desc: "bitmaps pour les hubs + CSR pour le reste", Build: func(n int, e []Edge) (Graph, error) { return NewHybrid(n, e), nil }},
	{Name: "bitmatrix", Desc: "matrice d'adjacence dense en bitset", MaxVertices: 32768, Build: NewBitMatrix},
}

// BuilderByName retourne le constructeur portant ce nom.
func BuilderByName(name string) (Builder, error) {
	for _, b := range Builders {
		if b.Name == name {
			return b, nil
		}
	}
	return Builder{}, fmt.Errorf("structure inconnue: %q", name)
}

const (
	sliceHeaderBytes = 24 // ptr + len + cap sur 64 bits
	wordBytes        = 8
)

// sliceBytes compte le coût d'une slice : en-tête + capacité réellement allouée.
func sliceBytes[T any](s []T, elemSize uint64) uint64 {
	return sliceHeaderBytes + uint64(cap(s))*elemSize
}
