//go:build solution

// Package pipeline — конвейер Generate → Square → Merge с отменой по контексту.
package pipeline

import (
	"context"
	"sync"
)

// Generate — генератор: владелец канала пишет в него и сам же закрывает.
func Generate(ctx context.Context, vals ...int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out) // закрывает тот, кто пишет
		for _, v := range vals {
			select {
			case out <- v:
			case <-ctx.Done():
				// Без этого select горутина навсегда зависла бы на отправке,
				// если потребитель перестал читать, — утечка.
				return
			}
		}
	}()
	return out
}

// Square — стадия конвейера.
func Square(ctx context.Context, in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for {
			// Ждём и вход, и отмену: если upstream завис, мы всё равно выйдем.
			var v int
			var ok bool
			select {
			case v, ok = <-in:
				if !ok {
					return
				}
			case <-ctx.Done():
				return
			}
			select {
			case out <- v * v:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// Merge — fan-in: по горутине на вход + одна горутина, закрывающая out
// после того, как все пересылающие горутины завершились.
func Merge(ctx context.Context, ins ...<-chan int) <-chan int {
	out := make(chan int)
	var wg sync.WaitGroup
	wg.Add(len(ins))
	for _, in := range ins {
		go func() {
			defer wg.Done()
			for {
				select {
				case v, ok := <-in:
					if !ok {
						return
					}
					select {
					case out <- v:
					case <-ctx.Done():
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	// Закрыть out можно только когда никто больше в него не пишет,
	// иначе — panic: send on closed channel.
	go func() {
		wg.Wait()
		close(out)
	}()
	return out
}

// SquareParallel — fan-out на workers стадий Square + fan-in через Merge.
func SquareParallel(ctx context.Context, in <-chan int, workers int) <-chan int {
	workers = max(workers, 1)
	outs := make([]<-chan int, workers)
	for i := range outs {
		outs[i] = Square(ctx, in) // все читают из одного канала: конкурентное чтение безопасно
	}
	return Merge(ctx, outs...)
}
