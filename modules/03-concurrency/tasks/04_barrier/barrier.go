//go:build !solution

// Package barrier — циклический барьер на sync.Cond.
package barrier

// Barrier — многоразовый (циклический) барьер на n участников.
type Barrier struct {
	// TODO: mu sync.Mutex, cond *sync.Cond, n, count, generation...
}

// New создаёт барьер на n участников. Паникует, если n <= 0.
func New(n int) *Barrier {
	// TODO
	panic("TODO")
}

// Wait блокирует вызывающую горутину, пока Wait не вызовут n горутин.
// Затем все n освобождаются, и барьер готов к следующей «фазе».
// Ровно одна горутина в каждой фазе (последняя пришедшая) получает true.
func (b *Barrier) Wait() bool {
	// TODO
	panic("TODO")
}
