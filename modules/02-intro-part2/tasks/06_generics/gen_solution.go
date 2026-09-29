//go:build solution

package gen

import (
	"cmp"
	"maps"
	"slices"
)

func Map[T, U any](s []T, f func(T) U) []U {
	out := make([]U, 0, len(s))
	for _, v := range s {
		out = append(out, f(v))
	}
	return out
}

func Filter[T any](s []T, keep func(T) bool) []T {
	// Популярный трюк out := s[:0] переиспользует массив исходного среза и
	// портит данные вызывающего. Здесь это запрещено условием.
	var out []T
	for _, v := range s {
		if keep(v) {
			out = append(out, v)
		}
	}
	return out
}

func Reduce[T, A any](s []T, init A, f func(A, T) A) A {
	acc := init
	for _, v := range s {
		acc = f(acc, v)
	}
	return acc
}

func Sum[T Number](s []T) T {
	var total T
	for _, v := range s {
		total += v
	}
	return total
}

func MinMax[T cmp.Ordered](s []T) (lo, hi T, ok bool) {
	if len(s) == 0 {
		return lo, hi, false
	}
	lo, hi = s[0], s[0]
	for _, v := range s[1:] {
		// Встроенные min/max «заражаются» NaN, а оператор < с NaN всегда
		// false. cmp.Less даёт полный порядок: NaN < всех чисел.
		if cmp.Less(v, lo) {
			lo = v
		}
		if cmp.Less(hi, v) {
			hi = v
		}
	}
	return lo, hi, true
}

func GroupBy[T any, K comparable](s []T, key func(T) K) map[K][]T {
	out := make(map[K][]T)
	for _, v := range s {
		k := key(v)
		out[k] = append(out[k], v)
	}
	return out
}

func Uniq[T comparable](s []T) []T {
	seen := make(map[T]struct{}, len(s))
	out := make([]T, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func NewSet[T comparable](items ...T) *Set[T] {
	s := &Set[T]{}
	s.Add(items...)
	return s
}

func (s *Set[T]) Add(items ...T) {
	if s.m == nil {
		s.m = make(map[T]struct{}, len(items))
	}
	for _, x := range items {
		s.m[x] = struct{}{}
	}
}

func (s *Set[T]) Has(x T) bool {
	_, ok := s.m[x] // чтение из nil-map безопасно
	return ok
}

func (s *Set[T]) Remove(x T) { delete(s.m, x) } // delete из nil-map — no-op
func (s *Set[T]) Len() int   { return len(s.m) }

func (s *Set[T]) Union(o *Set[T]) *Set[T] {
	out := &Set[T]{m: maps.Clone(s.m)}
	if out.m == nil {
		out.m = make(map[T]struct{})
	}
	maps.Copy(out.m, o.m)
	return out
}

func (s *Set[T]) Intersect(o *Set[T]) *Set[T] {
	out := NewSet[T]()
	// Итерируемся по меньшему множеству.
	small, big := s, o
	if small.Len() > big.Len() {
		small, big = big, small
	}
	for x := range small.m {
		if big.Has(x) {
			out.m[x] = struct{}{}
		}
	}
	return out
}

func (s *Set[T]) Difference(o *Set[T]) *Set[T] {
	out := NewSet[T]()
	for x := range s.m {
		if !o.Has(x) {
			out.m[x] = struct{}{}
		}
	}
	return out
}

func Sorted[T cmp.Ordered](s *Set[T]) []T {
	return slices.Sorted(maps.Keys(s.m))
}

func (s *Stack[T]) Push(x T) { s.items = append(s.items, x) }

func (s *Stack[T]) Pop() (T, bool) {
	var zero T
	n := len(s.items)
	if n == 0 {
		return zero, false
	}
	x := s.items[n-1]
	s.items[n-1] = zero // не держим ссылку — иначе утечка памяти для указателей
	s.items = s.items[:n-1]
	return x, true
}

func (s *Stack[T]) Peek() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	return s.items[len(s.items)-1], true
}

func (s *Stack[T]) Len() int { return len(s.items) }
