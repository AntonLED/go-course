//go:build !solution

package clock

import (
	"context"
	"errors"
	"time"
)

// Fake — управляемые часы для тестов. Безопасны для конкурентного использования.
type Fake struct {
	// TODO
}

// NewFake создаёт часы, показывающие start.
func NewFake(start time.Time) *Fake {
	// TODO
	return &Fake{}
}

// Now возвращает текущее «время» часов.
func (f *Fake) Now() time.Time {
	// TODO
	return time.Time{}
}

// After возвращает канал (буфер 1), в который придёт момент срабатывания
// (Now()+d), когда часы будут переведены на d или дальше. d <= 0 — срабатывает сразу.
func (f *Fake) After(d time.Duration) <-chan time.Time {
	// TODO
	return nil
}

// Advance переводит часы на d вперёд и срабатывает все таймеры с дедлайном <= нового Now,
// в порядке возрастания дедлайнов.
func (f *Fake) Advance(d time.Duration) {
	// TODO
}

// Waiters — сколько таймеров After ещё ждут срабатывания.
func (f *Fake) Waiters() int {
	// TODO
	return 0
}

// BlockUntil блокируется, пока число ожидающих таймеров не станет >= n.
// Нужен, чтобы тест дождался, пока тестируемая горутина «уснёт» на After.
func (f *Fake) BlockUntil(n int) {
	// TODO
}

// TTLCache — кэш, записи которого живут ttl по часам clock.
type TTLCache[K comparable, V any] struct {
	// TODO
}

// NewTTLCache создаёт кэш.
func NewTTLCache[K comparable, V any](c Clock, ttl time.Duration) *TTLCache[K, V] {
	// TODO
	return &TTLCache[K, V]{}
}

// Set кладёт значение (перезапись продлевает жизнь).
func (c *TTLCache[K, V]) Set(k K, v V) {
	// TODO
}

// Get возвращает значение, если оно не протухло. Запись протухает, когда
// Now() >= время_записи + ttl; протухшая запись удаляется.
func (c *TTLCache[K, V]) Get(k K) (V, bool) {
	// TODO
	var zero V
	return zero, false
}

// Len — число непротухших записей.
func (c *TTLCache[K, V]) Len() int {
	// TODO
	return 0
}

// Poll вызывает cond сразу, затем каждые interval (по часам c), пока cond не вернёт
// true (→ nil) или ошибку (→ эта ошибка), или пока не отменят ctx (→ ctx.Err()).
func Poll(ctx context.Context, c Clock, interval time.Duration, cond func() (bool, error)) error {
	// TODO
	return errors.New("TODO")
}
