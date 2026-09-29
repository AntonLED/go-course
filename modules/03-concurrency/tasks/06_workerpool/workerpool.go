//go:build !solution

// Package workerpool — пул воркеров с результатами и отменой.
package workerpool

import "context"

// Run запускает workers воркеров (workers <= 0 означает 1), которые читают
// задачи из in и вызывают f(ctx, v). Результаты (в произвольном порядке)
// отправляются в возвращаемый канал.
//
// Выходной канал закрывается, когда:
//   - in закрыт и все задачи обработаны, или
//   - ctx отменён (тогда оставшиеся задачи не обрабатываются,
//     а воркеры не блокируются на отправке результата).
func Run[T, R any](ctx context.Context, workers int, in <-chan T, f func(context.Context, T) (R, error)) <-chan Result[T, R] {
	// TODO
	panic("TODO")
}
