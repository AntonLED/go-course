//go:build !solution

// Package counter — потокобезопасные счётчики.
package counter

// Counter — потокобезопасный счётчик. Нулевое значение готово к работе.
// Подсказка: atomic.Int64.
type Counter struct {
	// TODO
}

// Inc увеличивает счётчик на 1.
func (c *Counter) Inc() { panic("TODO") }

// Add прибавляет n (n может быть отрицательным) и возвращает новое значение.
func (c *Counter) Add(n int64) int64 { panic("TODO") }

// Value возвращает текущее значение.
func (c *Counter) Value() int64 { panic("TODO") }

// KeyCounter — потокобезопасный счётчик по строковым ключам
// (например, число запросов на эндпоинт). Нулевое значение готово к работе.
// Подсказка: sync.RWMutex + map[string]int64 с ленивой инициализацией.
type KeyCounter struct {
	// TODO
}

// Inc увеличивает счётчик key на 1 и возвращает новое значение.
func (k *KeyCounter) Inc(key string) int64 { panic("TODO") }

// Get возвращает значение счётчика key (0, если ключа нет).
func (k *KeyCounter) Get(key string) int64 { panic("TODO") }

// Len возвращает число ключей.
func (k *KeyCounter) Len() int { panic("TODO") }

// Snapshot возвращает копию текущих значений. Изменение копии
// не должно влиять на счётчик и не должно вызывать гонок.
func (k *KeyCounter) Snapshot() map[string]int64 { panic("TODO") }

// Reset обнуляет все счётчики и возвращает значения до обнуления.
func (k *KeyCounter) Reset() map[string]int64 { panic("TODO") }
