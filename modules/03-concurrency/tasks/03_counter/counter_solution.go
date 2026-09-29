//go:build solution

// Package counter — потокобезопасные счётчики.
package counter

import (
	"maps"
	"sync"
	"sync/atomic"
)

// Counter — потокобезопасный счётчик на atomic.Int64.
// atomic.Int64 (в отличие от голого int64 + atomic.AddInt64) гарантирует
// выравнивание на 8 байт даже на 32-битных платформах и не даёт случайно
// обратиться к полю неатомарно. Внутри есть noCopy — go vet ругнётся на копию.
type Counter struct {
	v atomic.Int64
}

// Inc увеличивает счётчик на 1.
func (c *Counter) Inc() { c.v.Add(1) }

// Add прибавляет n и возвращает новое значение.
func (c *Counter) Add(n int64) int64 { return c.v.Add(n) }

// Value возвращает текущее значение.
func (c *Counter) Value() int64 { return c.v.Load() }

// KeyCounter — счётчик по ключам на RWMutex.
// Мьютекс нельзя копировать после первого использования — поэтому
// все методы на указателе, а сам KeyCounter передают по указателю.
type KeyCounter struct {
	mu sync.RWMutex
	m  map[string]int64
}

// Inc увеличивает счётчик key на 1.
func (k *KeyCounter) Inc(key string) int64 {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.m == nil { // ленивая инициализация: нулевое значение пригодно к работе
		k.m = make(map[string]int64)
	}
	k.m[key]++
	return k.m[key]
}

// Get возвращает значение счётчика key. Читатели не блокируют друг друга.
func (k *KeyCounter) Get(key string) int64 {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return k.m[key] // чтение из nil-map безопасно
}

// Len возвращает число ключей.
func (k *KeyCounter) Len() int {
	k.mu.RLock()
	defer k.mu.RUnlock()
	return len(k.m)
}

// Snapshot возвращает копию. Возвращать k.m напрямую нельзя: вызывающий
// код читал бы map без блокировки параллельно с Inc — это data race.
func (k *KeyCounter) Snapshot() map[string]int64 {
	k.mu.RLock()
	defer k.mu.RUnlock()
	res := make(map[string]int64, len(k.m))
	maps.Copy(res, k.m)
	return res
}

// Reset атомарно (под одной блокировкой) забирает старые значения и обнуляет счётчик.
func (k *KeyCounter) Reset() map[string]int64 {
	k.mu.Lock()
	defer k.mu.Unlock()
	old := k.m
	k.m = nil
	if old == nil {
		old = map[string]int64{}
	}
	return old // старую map больше никто не трогает — отдавать её безопасно
}
