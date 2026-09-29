//go:build !solution

// Package parsum — параллельная обработка среза по чанкам.
package parsum

// Workers возвращает фактическое число воркеров:
// n, если n > 0, иначе runtime.GOMAXPROCS(0).
func Workers(n int) int {
	// TODO
	panic("TODO")
}

// Sum параллельно суммирует nums, разбивая срез на не более чем Workers(workers)
// примерно равных непрерывных чанков (по горутине на чанк).
func Sum(nums []int64, workers int) int64 {
	// TODO: посчитайте частичные суммы в горутинах, без мьютекса
	// (каждая горутина пишет в свою ячейку среза partial[i]).
	panic("TODO")
}

// Count параллельно считает элементы, для которых pred возвращает true.
// Одновременно выполняется не более Workers(workers) вызовов pred.
func Count[T any](items []T, workers int, pred func(T) bool) int {
	// TODO
	panic("TODO")
}
