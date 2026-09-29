//go:build !solution

package seq

import "iter"

// Fibonacci возвращает бесконечную последовательность 0, 1, 1, 2, 3, 5, ...
// Обязательно прекращай генерацию, как только yield вернул false.
func Fibonacci() iter.Seq[int] {
	panic("TODO")
}

// Filter пропускает только элементы, для которых keep == true.
func Filter[T any](s iter.Seq[T], keep func(T) bool) iter.Seq[T] {
	panic("TODO")
}

// Map применяет f к каждому элементу.
func Map[T, U any](s iter.Seq[T], f func(T) U) iter.Seq[U] {
	panic("TODO")
}

// Take отдаёт не более n первых элементов. Важно: не запрашивать у s
// лишних элементов (при n <= 0 вообще не вызывать s; после n-го — сразу
// остановить источник).
func Take[T any](s iter.Seq[T], n int) iter.Seq[T] {
	panic("TODO")
}

// Enumerate нумерует элементы с нуля: (0, a), (1, b), ...
func Enumerate[T any](s iter.Seq[T]) iter.Seq2[int, T] {
	panic("TODO")
}

// Zip идёт по двум последовательностям параллельно и заканчивается, когда
// закончится любая из них. Используй iter.Pull и НЕ забудь вызвать stop()
// для обеих (иначе горутины/ресурсы источников не освободятся).
func Zip[A, B any](a iter.Seq[A], b iter.Seq[B]) iter.Seq2[A, B] {
	panic("TODO")
}

// Chunk группирует элементы в срезы по n (последний может быть короче).
// Каждый чанк — новый срез (потребитель может его сохранить).
// При n <= 0 — panic.
func Chunk[T any](s iter.Seq[T], n int) iter.Seq[[]T] {
	panic("TODO")
}
