//go:build !solution

// Package pipeline — конвейер Generate → Square → Merge с отменой по контексту.
package pipeline

import "context"

// Generate отправляет vals в возвращаемый канал по порядку и закрывает его.
// При отмене ctx прекращает отправку и закрывает канал.
func Generate(ctx context.Context, vals ...int) <-chan int {
	// TODO
	panic("TODO")
}

// Square читает из in и отправляет квадраты чисел, сохраняя порядок.
// Выходной канал закрывается, когда in закрыт или ctx отменён.
func Square(ctx context.Context, in <-chan int) <-chan int {
	// TODO
	panic("TODO")
}

// Merge (fan-in) объединяет все входные каналы в один. Выходной канал
// закрывается, когда закрыты все входы или отменён ctx.
func Merge(ctx context.Context, ins ...<-chan int) <-chan int {
	// TODO
	panic("TODO")
}

// SquareParallel (fan-out + fan-in) запускает workers стадий Square,
// читающих из одного in, и сливает результаты через Merge.
// Порядок результатов не гарантируется. workers <= 0 означает 1.
func SquareParallel(ctx context.Context, in <-chan int, workers int) <-chan int {
	// TODO
	panic("TODO")
}
