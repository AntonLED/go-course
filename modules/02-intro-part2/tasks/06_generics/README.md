# Задача 06. Дженерики: утилиты, Set, Stack

Пакет `gen`. До Go 1.18 функции вроде `Map` или `Filter` приходилось писать заново для каждого типа или прятать всё за `interface{}`. В этой задаче вы напишете небольшую библиотеку обобщённых утилит и две структуры данных — примерно то, что в реальных проектах лежит во внутреннем пакете утилит. Ограничение `Number` и типы `Set[T comparable]`, `Stack[T any]` уже объявлены в `types.go`. Реализуйте в `gen.go`:

```go
func Map[T, U any](s []T, f func(T) U) []U
func Filter[T any](s []T, keep func(T) bool) []T
func Reduce[T, A any](s []T, init A, f func(A, T) A) A
func Sum[T Number](s []T) T
func MinMax[T cmp.Ordered](s []T) (lo, hi T, ok bool)
func GroupBy[T any, K comparable](s []T, key func(T) K) map[K][]T
func Uniq[T comparable](s []T) []T

func NewSet[T comparable](items ...T) *Set[T]
func (s *Set[T]) Add(items ...T) / Has / Remove / Len
func (s *Set[T]) Union(o *Set[T]) *Set[T] / Intersect / Difference
func Sorted[T cmp.Ordered](s *Set[T]) []T

func (s *Stack[T]) Push(x T) / Pop() (T, bool) / Peek() (T, bool) / Len() int
```

Пример:

```go
strs := Map([]int{1, 2, 3}, strconv.Itoa)          // []string{"1","2","3"} — типы выведены
type Celsius float64
Sum([]Celsius{20.5, 1.5})                           // 22 — работает благодаря ~float64
Sorted(NewSet(3, 1, 2).Union(NewSet(4)))            // [1 2 3 4]
```

## Подвохи

`Filter` не должен портить исходный срез и не должен разделять с ним массив, так что привычный трюк `out := s[:0]` здесь запрещён.

Дженерик не спасает от переполнения: `Sum` для `[]uint8{200, 100}` даёт `44`.

Больше всего сюрпризов у `MinMax` для `float64` с `NaN`. Оператор `<` с NaN всегда возвращает `false`, а встроенные `min` и `max` возвращают NaN, если он встретился хоть где-то. Нам же нужна семантика `cmp.Compare`, в которой NaN меньше всех остальных значений, поэтому `MinMax({3, NaN, 1}) == (NaN, 3)`. Используйте `cmp.Less`.

Нулевые значения `Set` и `Stack` должны быть рабочими, а значит, карту внутри `Set` нужно создавать лениво. Чтение и `delete` из `nil`-карты безопасны, а запись вызывает панику.

`Union`, `Intersect` и `Difference` возвращают новые множества, которые не разделяют карту с аргументами. Осторожнее с `maps.Clone(nil)`: он вернёт `nil`.

`Sorted` сделана функцией, а не методом, потому что методы не могут вводить собственные type parameters и сужать ограничение типа.

И последнее: `Pop` должен обнулять освободившийся слот. Иначе стек указателей будет удерживать объекты, которые сборщик мусора мог бы освободить.

## Проверка

```
go test ./modules/02-intro-part2/tasks/06_generics/
```
