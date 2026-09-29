//go:build solution

// Package barrier — циклический барьер на sync.Cond.
package barrier

import "sync"

// Barrier — многоразовый барьер на n участников.
type Barrier struct {
	mu    sync.Mutex
	cond  *sync.Cond
	n     int
	count int    // сколько пришло в текущей фазе
	gen   uint64 // номер фазы
}

// New создаёт барьер на n участников.
func New(n int) *Barrier {
	if n <= 0 {
		panic("barrier: n must be positive")
	}
	b := &Barrier{n: n}
	b.cond = sync.NewCond(&b.mu)
	return b
}

// Wait ждёт, пока соберутся все n участников.
func (b *Barrier) Wait() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	gen := b.gen
	b.count++
	if b.count == b.n {
		// Последний: открываем следующую фазу и будим всех.
		b.count = 0
		b.gen++
		b.cond.Broadcast()
		return true
	}
	// Ждём смены поколения, а не условия count == 0: иначе быстрый участник
	// следующей фазы может снова увеличить count до того, как «медленный»
	// проснётся, и тот уснёт навсегда. Wait всегда в цикле — возможны
	// ложные пробуждения и Broadcast «не для нас».
	for gen == b.gen {
		b.cond.Wait()
	}
	return false
}
