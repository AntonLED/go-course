//go:build solution

// Package parmap — параллельный Map с сохранением порядка.
package parmap

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// Map применяет f к каждому элементу in, используя не более workers
// горутин одновременно. Порядок результатов сохраняется.
func Map[T, R any](in []T, workers int, f func(T) R) []R {
	out := make([]R, len(in))
	if len(in) == 0 {
		return out
	}
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	workers = min(workers, len(in))

	var (
		next     atomic.Int64 // индекс следующего необработанного элемента
		stop     atomic.Bool  // после паники новые элементы не берём
		wg       sync.WaitGroup
		panicked bool
		panicVal any
		once     sync.Once
	)

	worker := func() {
		defer wg.Done()
		// Паника в горутине, которую никто не перехватил, роняет весь процесс:
		// recover в main её не поймает. Поэтому ловим здесь и передаём наверх.
		defer func() {
			if r := recover(); r != nil {
				once.Do(func() {
					panicked, panicVal = true, r
				})
				stop.Store(true)
			}
		}()
		for !stop.Load() {
			i := int(next.Add(1) - 1)
			if i >= len(in) {
				return
			}
			// Каждая горутина пишет в свой out[i] — синхронизация не нужна.
			out[i] = f(in[i])
		}
	}

	wg.Add(workers)
	for range workers {
		go worker()
	}
	wg.Wait() // happens-before: все записи в out/panicVal видны ниже

	if panicked {
		panic(panicVal)
	}
	return out
}
