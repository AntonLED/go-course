//go:build !solution

package gen

import "cmp"

// Map применяет f к каждому элементу. Для пустого/nil входа — пустой срез.
func Map[T, U any](s []T, f func(T) U) []U { panic("TODO") }

// Filter возвращает новый срез из элементов, для которых keep == true.
// Исходный срез менять нельзя!
func Filter[T any](s []T, keep func(T) bool) []T { panic("TODO") }

// Reduce сворачивает срез слева направо: f(f(f(init, s0), s1), s2)...
func Reduce[T, A any](s []T, init A, f func(A, T) A) A { panic("TODO") }

// Sum суммирует числа любого типа, удовлетворяющего Number.
func Sum[T Number](s []T) T { panic("TODO") }

// MinMax возвращает минимум и максимум в смысле cmp.Compare (NaN меньше
// любого числа и равен другому NaN). ok == false для пустого среза.
func MinMax[T cmp.Ordered](s []T) (lo, hi T, ok bool) { panic("TODO") }

// GroupBy раскладывает элементы по ключу, сохраняя порядок внутри групп.
func GroupBy[T any, K comparable](s []T, key func(T) K) map[K][]T { panic("TODO") }

// Uniq удаляет дубликаты, сохраняя порядок первых вхождений.
func Uniq[T comparable](s []T) []T { panic("TODO") }

// NewSet создаёт множество из элементов.
func NewSet[T comparable](items ...T) *Set[T] { panic("TODO") }

// Add добавляет элементы (инициализирует map лениво).
func (s *Set[T]) Add(items ...T) { panic("TODO") }

// Has — принадлежит ли элемент множеству.
func (s *Set[T]) Has(x T) bool { panic("TODO") }

// Remove удаляет элемент (отсутствующий — не ошибка).
func (s *Set[T]) Remove(x T) { panic("TODO") }

// Len — количество элементов.
func (s *Set[T]) Len() int { panic("TODO") }

// Union, Intersect, Difference возвращают новые множества, не меняя s и o.
func (s *Set[T]) Union(o *Set[T]) *Set[T]      { panic("TODO") }
func (s *Set[T]) Intersect(o *Set[T]) *Set[T]  { panic("TODO") }
func (s *Set[T]) Difference(o *Set[T]) *Set[T] { panic("TODO") }

// Sorted возвращает элементы множества по возрастанию. Это функция, а не
// метод: у метода не может быть собственных type parameters или более
// строгого ограничения (cmp.Ordered), чем у типа (comparable).
func Sorted[T cmp.Ordered](s *Set[T]) []T { panic("TODO") }

// Push кладёт элемент на вершину стека.
func (s *Stack[T]) Push(x T) { panic("TODO") }

// Pop снимает элемент с вершины; для пустого стека — (нулевое значение, false).
func (s *Stack[T]) Pop() (T, bool) { panic("TODO") }

// Peek возвращает вершину, не снимая её.
func (s *Stack[T]) Peek() (T, bool) { panic("TODO") }

// Len — количество элементов.
func (s *Stack[T]) Len() int { panic("TODO") }
