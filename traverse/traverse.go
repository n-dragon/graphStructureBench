// Package traverse implémente les parcours, écrits une seule fois contre
// l'interface graph.Graph : seule la structure d'index change d'une mesure à
// l'autre.
package traverse

import "github.com/n-dragon/graphstructurebench/graph"

// Scratch regroupe les tampons d'un parcours. Un Scratch par goroutine : les
// structures sont en lecture seule, l'état mutable d'un parcours ne doit pas
// être partagé. Les tampons sont réutilisés d'une requête à l'autre pour que
// la mesure porte sur le parcours, pas sur l'allocateur.
type Scratch struct {
	visited []uint64 // bitset des sommets marqués
	list    []uint32 // file BFS, et liste des bits à effacer au reset
	stack   []uint32 // pile DFS
}

// NewScratch alloue les tampons pour un graphe de n sommets.
func NewScratch(n int) *Scratch {
	return &Scratch{
		visited: make([]uint64, (n+63)/64),
		list:    make([]uint32, 0, 1024),
		stack:   make([]uint32, 0, 1024),
	}
}

// Bytes est l'empreinte mémoire courante du Scratch : c'est le coût par
// thread de parcours, à ajouter à l'index lui-même.
func (s *Scratch) Bytes() uint64 {
	return uint64(cap(s.visited))*8 + uint64(cap(s.list))*4 + uint64(cap(s.stack))*4
}

// reset n'efface que les bits réellement posés au parcours précédent : le coût
// est proportionnel aux sommets visités, pas à la taille du graphe.
func (s *Scratch) reset() {
	for _, v := range s.list {
		s.visited[v>>6] &^= 1 << uint(v&63)
	}
	s.list = s.list[:0]
	s.stack = s.stack[:0]
}

func (s *Scratch) test(v uint32) bool { return s.visited[v>>6]&(1<<uint(v&63)) != 0 }

func (s *Scratch) mark(v uint32) {
	s.visited[v>>6] |= 1 << uint(v&63)
	s.list = append(s.list, v)
}

// BFS parcourt en largeur depuis src et renvoie le nombre de sommets atteints
// ainsi qu'une somme de contrôle (somme des identifiants visités), qui doit
// être identique pour toutes les structures.
func BFS(g graph.Graph, src uint32, s *Scratch) (int, uint64) {
	s.reset()
	s.mark(src)
	var sum uint64
	visit := func(v uint32) bool {
		if !s.test(v) {
			s.mark(v)
		}
		return true
	}
	for head := 0; head < len(s.list); head++ {
		u := s.list[head]
		sum += uint64(u)
		g.ForEachNeighbor(u, visit)
	}
	return len(s.list), sum
}

// DFS parcourt en profondeur (version itérative) depuis src.
func DFS(g graph.Graph, src uint32, s *Scratch) (int, uint64) {
	s.reset()
	s.stack = append(s.stack, src)
	var sum uint64
	var push = func(v uint32) bool {
		if !s.test(v) {
			s.stack = append(s.stack, v)
		}
		return true
	}
	for len(s.stack) > 0 {
		u := s.stack[len(s.stack)-1]
		s.stack = s.stack[:len(s.stack)-1]
		if s.test(u) {
			continue
		}
		s.mark(u)
		sum += uint64(u)
		g.ForEachNeighbor(u, push)
	}
	return len(s.list), sum
}

// Neighbors somme les voisins d'un sommet : la requête la plus élémentaire,
// elle isole le coût d'accès à l'adjacence sans le bruit d'un parcours.
func Neighbors(g graph.Graph, u uint32) uint64 {
	var sum uint64
	g.ForEachNeighbor(u, func(v uint32) bool {
		sum += uint64(v)
		return true
	})
	return sum
}

// NeighborsSlice fait la même chose sans callback, pour les structures qui
// exposent leur adjacence en slice : l'écart avec Neighbors mesure le prix de
// l'abstraction ForEachNeighbor.
func NeighborsSlice(g graph.SliceGraph, u uint32) uint64 {
	var sum uint64
	for _, v := range g.Neighbors(u) {
		sum += uint64(v)
	}
	return sum
}
