// Package traverse implémente les parcours, écrits une seule fois contre
// l'interface graph.Graph : seule la structure d'index change d'une mesure à
// l'autre.
package traverse

import "github.com/n-dragon/graphstructurebench/graph"

// Scratch regroupe les tampons d'un parcours. Un Scratch par goroutine : les
// structures sont en lecture seule, l'état mutable d'un parcours ne doit pas
// être partagé. Les tampons sont réutilisés d'une requête à l'autre pour que
// la mesure porte sur le parcours, pas sur l'allocateur.
//
// Les fermetures passées à ForEachNeighbor sont construites ici, une fois pour
// toutes. Écrites à l'intérieur des fonctions de parcours, elles s'échappaient
// sur le tas à chaque appel (l'analyse d'échappement ne traverse pas un appel
// de méthode d'interface) : la charge « neighbors » mesurait alors deux
// allocations par requête autant que la structure d'index.
type Scratch struct {
	visited []uint64 // bitset des sommets marqués
	list    []uint32 // file BFS, et liste des bits à effacer au reset
	stack   []uint32 // pile DFS
	buf     []uint32 // tampon de l'accès par lot

	sum     uint64            // accumulateur des fermetures
	visitFn func(uint32) bool // marque et enfile (BFS)
	pushFn  func(uint32) bool // empile si non marqué (DFS)
	sumFn   func(uint32) bool // somme les voisins
}

// NewScratch alloue les tampons pour un graphe de n sommets.
func NewScratch(n int) *Scratch {
	s := &Scratch{
		visited: make([]uint64, (n+63)/64),
		list:    make([]uint32, 0, 1024),
		stack:   make([]uint32, 0, 1024),
		buf:     make([]uint32, 0, 256),
	}
	s.visitFn = func(v uint32) bool {
		if !s.test(v) {
			s.mark(v)
		}
		return true
	}
	s.pushFn = func(v uint32) bool {
		if !s.test(v) {
			s.stack = append(s.stack, v)
		}
		return true
	}
	s.sumFn = func(v uint32) bool {
		s.sum += uint64(v)
		return true
	}
	return s
}

// Bytes est l'empreinte mémoire courante du Scratch : c'est le coût par
// thread de parcours, à ajouter à l'index lui-même.
func (s *Scratch) Bytes() uint64 {
	return uint64(cap(s.visited))*8 + uint64(cap(s.list))*4 +
		uint64(cap(s.stack))*4 + uint64(cap(s.buf))*4
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
	for head := 0; head < len(s.list); head++ {
		u := s.list[head]
		sum += uint64(u)
		g.ForEachNeighbor(u, s.visitFn)
	}
	return len(s.list), sum
}

// DFS parcourt en profondeur (version itérative) depuis src.
func DFS(g graph.Graph, src uint32, s *Scratch) (int, uint64) {
	s.reset()
	s.stack = append(s.stack, src)
	var sum uint64
	for len(s.stack) > 0 {
		u := s.stack[len(s.stack)-1]
		s.stack = s.stack[:len(s.stack)-1]
		if s.test(u) {
			continue
		}
		s.mark(u)
		sum += uint64(u)
		g.ForEachNeighbor(u, s.pushFn)
	}
	return len(s.list), sum
}

// BFSBatch est le même parcours en largeur, mais en lisant l'adjacence par
// blocs plutôt que par appels indirects : la structure remplit un tampon que
// la boucle parcourt ensuite sans indirection. L'écart avec BFS chiffre ce
// que coûte l'abstraction par callback sur un parcours réel.
func BFSBatch(g graph.Graph, src uint32, s *Scratch) (int, uint64) {
	s.reset()
	s.mark(src)
	var sum uint64
	for head := 0; head < len(s.list); head++ {
		u := s.list[head]
		sum += uint64(u)
		s.buf = g.AppendNeighbors(s.buf[:0], u)
		for _, v := range s.buf {
			if !s.test(v) {
				s.mark(v)
			}
		}
	}
	return len(s.list), sum
}

// NeighborsBatch somme les voisins d'un sommet par accès en bloc.
func NeighborsBatch(g graph.Graph, u uint32, s *Scratch) uint64 {
	s.buf = g.AppendNeighbors(s.buf[:0], u)
	var sum uint64
	for _, v := range s.buf {
		sum += uint64(v)
	}
	return sum
}

// Neighbors somme les voisins d'un sommet : la requête la plus élémentaire,
// elle isole le coût d'accès à l'adjacence sans le bruit d'un parcours. Le
// Scratch ne sert ici qu'à porter la fermeture et son accumulateur, pour que
// la mesure ne contienne aucune allocation.
func Neighbors(g graph.Graph, u uint32, s *Scratch) uint64 {
	s.sum = 0
	g.ForEachNeighbor(u, s.sumFn)
	return s.sum
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
