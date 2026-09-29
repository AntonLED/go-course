//go:build solution

package seq

import "iter"

func Fibonacci() iter.Seq[int] {
	return func(yield func(int) bool) {
		a, b := 0, 1
		for {
			if !yield(a) {
				return // потребитель сделал break — выходим
			}
			a, b = b, a+b
		}
	}
}

func Filter[T any](s iter.Seq[T], keep func(T) bool) iter.Seq[T] {
	return func(yield func(T) bool) {
		for v := range s {
			// return внутри range-over-func корректно останавливает s:
			// рантайм заставит yield источника вернуть false.
			if keep(v) && !yield(v) {
				return
			}
		}
	}
}

func Map[T, U any](s iter.Seq[T], f func(T) U) iter.Seq[U] {
	return func(yield func(U) bool) {
		for v := range s {
			if !yield(f(v)) {
				return
			}
		}
	}
}

func Take[T any](s iter.Seq[T], n int) iter.Seq[T] {
	return func(yield func(T) bool) {
		if n <= 0 {
			return
		}
		i := 0
		for v := range s {
			if !yield(v) {
				return
			}
			i++
			if i == n { // останавливаемся сразу, не дожидаясь (n+1)-го элемента
				return
			}
		}
	}
}

func Enumerate[T any](s iter.Seq[T]) iter.Seq2[int, T] {
	return func(yield func(int, T) bool) {
		i := 0
		for v := range s {
			if !yield(i, v) {
				return
			}
			i++
		}
	}
}

func Zip[A, B any](a iter.Seq[A], b iter.Seq[B]) iter.Seq2[A, B] {
	return func(yield func(A, B) bool) {
		nextA, stopA := iter.Pull(a)
		defer stopA()
		nextB, stopB := iter.Pull(b)
		defer stopB()
		for {
			va, ok := nextA()
			if !ok {
				return
			}
			vb, ok := nextB()
			if !ok {
				return
			}
			if !yield(va, vb) {
				return
			}
		}
	}
}

func Chunk[T any](s iter.Seq[T], n int) iter.Seq[[]T] {
	if n <= 0 {
		panic("seq.Chunk: n должно быть > 0")
	}
	return func(yield func([]T) bool) {
		buf := make([]T, 0, n)
		for v := range s {
			buf = append(buf, v)
			if len(buf) == n {
				if !yield(buf) {
					return
				}
				buf = make([]T, 0, n) // новый массив: старый чанк отдан потребителю
			}
		}
		if len(buf) > 0 {
			yield(buf)
		}
	}
}
