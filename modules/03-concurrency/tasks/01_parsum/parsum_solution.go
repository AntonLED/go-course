//go:build solution

// Package parsum — параллельная обработка среза по чанкам.
package parsum

import (
	"runtime"
	"sync"
)

// Workers возвращает фактическое число воркеров:
// n, если n > 0, иначе runtime.GOMAXPROCS(0).
func Workers(n int) int {
	if n > 0 {
		return n
	}
	// GOMAXPROCS(0) не меняет настройку, а только читает её:
	// это число P, т.е. сколько горутин реально исполняются одновременно.
	return runtime.GOMAXPROCS(0)
}

// chunks делит [0, n) на k почти равных непрерывных отрезков.
// Первые n%k отрезков на один элемент длиннее.
func chunks(n, k int) [][2]int {
	k = min(k, n)
	if k <= 0 {
		return nil
	}
	res := make([][2]int, 0, k)
	size, rest := n/k, n%k
	lo := 0
	for i := 0; i < k; i++ {
		hi := lo + size
		if i < rest {
			hi++
		}
		res = append(res, [2]int{lo, hi})
		lo = hi
	}
	return res
}

// Sum параллельно суммирует nums по чанкам.
func Sum(nums []int64, workers int) int64 {
	parts := chunks(len(nums), Workers(workers))
	// Каждая горутина пишет только в свою ячейку: гонки нет,
	// а wg.Wait() даёт happens-before между записями и чтением ниже.
	partial := make([]int64, len(parts))
	var wg sync.WaitGroup
	for i, p := range parts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var s int64
			for _, v := range nums[p[0]:p[1]] {
				s += v // локальная переменная — никакого false sharing в цикле
			}
			partial[i] = s
		}()
	}
	wg.Wait()

	var total int64
	for _, s := range partial {
		total += s
	}
	return total
}

// Count параллельно считает элементы, удовлетворяющие pred.
func Count[T any](items []T, workers int, pred func(T) bool) int {
	parts := chunks(len(items), Workers(workers))
	partial := make([]int, len(parts))
	var wg sync.WaitGroup
	wg.Add(len(parts))
	for i, p := range parts {
		go func() {
			defer wg.Done()
			c := 0
			for _, it := range items[p[0]:p[1]] {
				if pred(it) {
					c++
				}
			}
			partial[i] = c
		}()
	}
	wg.Wait()

	total := 0
	for _, c := range partial {
		total += c
	}
	return total
}
