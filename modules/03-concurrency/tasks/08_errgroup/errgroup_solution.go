//go:build solution

// Package errgroup — собственная реализация golang.org/x/sync/errgroup.
package errgroup

import (
	"context"
	"fmt"
	"sync"
)

// Group — набор горутин, работающих над подзадачами одной задачи.
type Group struct {
	cancel context.CancelCauseFunc // nil для нулевого значения

	wg  sync.WaitGroup
	sem chan struct{} // семафор: буферизованный канал ёмкостью limit; nil — без лимита

	errOnce sync.Once
	err     error
}

// WithContext возвращает новую Group и производный контекст.
func WithContext(ctx context.Context) (*Group, context.Context) {
	ctx, cancel := context.WithCancelCause(ctx)
	return &Group{cancel: cancel}, ctx
}

func (g *Group) done() {
	if g.sem != nil {
		<-g.sem // освобождаем слот
	}
	g.wg.Done()
}

func (g *Group) run(f func() error) {
	go func() {
		defer g.done()
		if err := f(); err != nil {
			// Запоминаем только первую ошибку; остальные отбрасываются.
			g.errOnce.Do(func() {
				g.err = err
				if g.cancel != nil {
					g.cancel(err) // context.Cause(ctx) вернёт именно эту ошибку
				}
			})
		}
	}()
}

// Go запускает f в новой горутине, при необходимости ожидая свободный слот.
func (g *Group) Go(f func() error) {
	if g.sem != nil {
		g.sem <- struct{}{} // блокируемся, пока активных горутин == limit
	}
	g.wg.Add(1) // Add — ДО запуска горутины, иначе Wait может проскочить
	g.run(f)
}

// TryGo запускает f, только если есть свободный слот.
func (g *Group) TryGo(f func() error) bool {
	if g.sem != nil {
		select {
		case g.sem <- struct{}{}:
		default:
			return false
		}
	}
	g.wg.Add(1)
	g.run(f)
	return true
}

// SetLimit ограничивает число одновременно активных горутин.
func (g *Group) SetLimit(n int) {
	if n < 0 {
		g.sem = nil
		return
	}
	if len(g.sem) != 0 {
		panic(fmt.Errorf("errgroup: modify limit while %v goroutines in the group are still active", len(g.sem)))
	}
	g.sem = make(chan struct{}, n)
}

// Wait ждёт все горутины и возвращает первую ошибку.
func (g *Group) Wait() error {
	g.wg.Wait()
	if g.cancel != nil {
		// Отменяем контекст и при успехе: производные ресурсы должны освободиться.
		// Если уже отменён ошибкой, повторный вызов причину не меняет.
		g.cancel(g.err)
	}
	return g.err
}
