//go:build !solution

// Package parmap — параллельный Map с сохранением порядка.
package parmap

// Map применяет f к каждому элементу in, используя не более workers
// горутин одновременно (workers <= 0 означает runtime.GOMAXPROCS(0)).
// Результат out[i] == f(in[i]) — порядок сохраняется.
//
// Если f паникует, Map дожидается завершения всех воркеров и
// паникует в вызывающей горутине с тем же значением.
func Map[T, R any](in []T, workers int, f func(T) R) []R {
	// TODO
	panic("TODO")
}
