# Задача 07. Итераторы: комбинаторы над iter.Seq

Пакет `seq`. В Go 1.23 появился range-over-func: теперь можно писать `for v := range seq`, где `seq` — функция вида `func(yield func(T) bool)`. Это позволяет строить ленивые цепочки обработки, которые не создают промежуточных срезов и спокойно работают даже с бесконечными последовательностями. Реализуйте в `seq.go` набор таких комбинаторов:

```go
func Fibonacci() iter.Seq[int]                                  // 0 1 1 2 3 5 ... (бесконечная)
func Filter[T any](s iter.Seq[T], keep func(T) bool) iter.Seq[T]
func Map[T, U any](s iter.Seq[T], f func(T) U) iter.Seq[U]
func Take[T any](s iter.Seq[T], n int) iter.Seq[T]
func Enumerate[T any](s iter.Seq[T]) iter.Seq2[int, T]
func Zip[A, B any](a iter.Seq[A], b iter.Seq[B]) iter.Seq2[A, B] // через iter.Pull
func Chunk[T any](s iter.Seq[T], n int) iter.Seq[[]T]
```

Пример:

```go
even := Filter(Fibonacci(), func(x int) bool { return x%2 == 0 })
fmt.Println(slices.Collect(Take(even, 5))) // [0 2 8 34 144]
```

## Главное правило

Если `yield` вернул `false` — значит, потребитель сделал `break` или `return`, — итератор обязан прекратить работу и больше ни разу не вызывать `yield`. Иначе рантайм запаникует с сообщением `range function continued iteration after function for loop body returned false`. Тесты проверяют это правило для каждого комбинатора.

## Подвохи

`Take(s, n)` не должен запрашивать у источника `(n+1)`-й элемент, а при `n <= 0` не должен запускать источник вообще. Тест считает, сколько элементов источник успел выдать.

`Zip` использует `iter.Pull`, который запускает источник как корутину. Если не вызвать `stop()`, корутина «зависнет», а `defer` внутри источника так и не выполнится. Поэтому вызывайте `stop` для обоих источников через `defer`.

В `Chunk` каждый чанк должен быть новым срезом. Если переиспользовать буфер, `slices.Collect` вернёт N ссылок на один и тот же массив. При `n <= 0` `Chunk` паникует.

Все комбинаторы должны работать с бесконечным источником.

## Проверка

```
go test ./modules/02-intro-part2/tasks/07_seq/
```
