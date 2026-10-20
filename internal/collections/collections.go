// Package collections tem estruturas de dados genéricas usadas em vários pacotes.
package collections

import (
	"cmp"
	"slices"
)

// Set é um conjunto com busca O(1).
type Set[T comparable] map[T]struct{}

func NewSet[T comparable](items ...T) Set[T] {
	s := make(Set[T], len(items))
	for _, v := range items {
		s.Add(v)
	}
	return s
}

func (s Set[T]) Add(v T) { s[v] = struct{}{} }

func (s Set[T]) Has(v T) bool {
	_, ok := s[v]
	return ok
}

func (s Set[T]) Len() int { return len(s) }

// Counter é um multiconjunto: conta ocorrências por chave.
type Counter[K cmp.Ordered] map[K]int

type Count[K cmp.Ordered] struct {
	Key K
	N   int
}

func (c Counter[K]) Add(k K) { c[k]++ }

// Top devolve as n chaves mais frequentes; empates são ordenados pela chave
// para a saída ser determinística. n <= 0 devolve todas.
func (c Counter[K]) Top(n int) []Count[K] {
	out := make([]Count[K], 0, len(c))
	for k, v := range c {
		out = append(out, Count[K]{k, v})
	}
	slices.SortFunc(out, func(a, b Count[K]) int {
		return cmp.Or(cmp.Compare(b.N, a.N), cmp.Compare(a.Key, b.Key))
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}
