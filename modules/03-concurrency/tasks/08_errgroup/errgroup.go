//go:build !solution

// Package errgroup — собственная реализация golang.org/x/sync/errgroup.
package errgroup

import "context"

// Group — набор горутин, работающих над подзадачами одной задачи.
// Нулевое значение готово к работе: без лимита и без отмены контекста.
type Group struct {
	// TODO: wg sync.WaitGroup, sem chan struct{}, errOnce sync.Once, err error,
	// cancel context.CancelCauseFunc
}

// WithContext возвращает новую Group и производный контекст. Контекст
// отменяется (с причиной — первой ошибкой) при первой ошибке любой из
// горутин или (с причиной context.Canceled) после возврата из Wait.
func WithContext(ctx context.Context) (*Group, context.Context) {
	// TODO
	panic("TODO")
}

// Go запускает f в новой горутине. Если установлен лимит и он достигнут,
// Go блокируется, пока не освободится место.
// Первая ненулевая ошибка запоминается и отменяет контекст группы.
func (g *Group) Go(f func() error) {
	// TODO
	panic("TODO")
}

// TryGo запускает f, только если лимит не достигнут, и сообщает, запущена ли она.
func (g *Group) TryGo(f func() error) bool {
	// TODO
	panic("TODO")
}

// SetLimit ограничивает число одновременно активных горутин n (n < 0 — без
// лимита). Паникует, если n >= 0 и заняты слоты ранее установленного лимита.
func (g *Group) SetLimit(n int) {
	// TODO
	panic("TODO")
}

// Wait ждёт завершения всех горутин, запущенных через Go/TryGo, и
// возвращает первую ненулевую ошибку (или nil).
func (g *Group) Wait() error {
	// TODO
	panic("TODO")
}
