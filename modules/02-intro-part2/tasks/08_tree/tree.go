//go:build !solution

package tree

import (
	"cmp"
	"iter"
)

// Put добавляет пару или заменяет значение существующего ключа.
func (t *Tree[K, V]) Put(k K, v V) { panic("TODO") }

// Get возвращает значение по ключу.
func (t *Tree[K, V]) Get(k K) (V, bool) { panic("TODO") }

// Len — количество ключей.
func (t *Tree[K, V]) Len() int { panic("TODO") }

// All обходит дерево in-order (по возрастанию ключей). После того как yield
// вернул false, обход должен немедленно прекратиться на любой глубине
// рекурсии.
func (t *Tree[K, V]) All() iter.Seq2[K, V] { panic("TODO") }

// Backward — обход по убыванию ключей.
func (t *Tree[K, V]) Backward() iter.Seq2[K, V] { panic("TODO") }

// Keys — только ключи по возрастанию.
func (t *Tree[K, V]) Keys() iter.Seq[K] { panic("TODO") }

// Range — пары с lo <= key < hi по возрастанию. Не заходи в поддеревья,
// которые заведомо вне диапазона.
func (t *Tree[K, V]) Range(lo, hi K) iter.Seq2[K, V] { panic("TODO") }

// MergeSorted сливает две отсортированные по возрастанию последовательности
// в одну отсортированную (дубликаты сохраняются; при равенстве первым идёт
// элемент из a). Используй iter.Pull для обеих и не забудь stop.
func MergeSorted[T cmp.Ordered](a, b iter.Seq[T]) iter.Seq[T] { panic("TODO") }
