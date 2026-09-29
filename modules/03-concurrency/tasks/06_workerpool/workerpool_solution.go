//go:build solution

// Package workerpool — пул воркеров с результатами и отменой.
package workerpool

import (
	"context"
	"sync"
)

// Run — пул из фиксированного числа воркеров. Число горутин ограничено
// workers независимо от количества задач (в отличие от «горутина на задачу»).
func Run[T, R any](ctx context.Context, workers int, in <-chan T, f func(context.Context, T) (R, error)) <-chan Result[T, R] {
	workers = max(workers, 1)
	out := make(chan Result[T, R])

	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for {
				// Проверяем отмену с приоритетом: select выбирает случайно
				// среди готовых веток, и без этой проверки воркер мог бы
				// продолжать брать задачи из заполненного in после cancel().
				if ctx.Err() != nil {
					return
				}
				var v T
				select {
				case <-ctx.Done():
					return
				case x, ok := <-in:
					if !ok {
						return
					}
					v = x
				}
				r, err := f(ctx, v)
				select {
				case out <- Result[T, R]{In: v, Out: r, Err: err}:
				case <-ctx.Done():
					return // потребитель мог уйти — не блокируемся навсегда
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}
