# 03. Исправить гонку: потокобезопасные счётчики (`counter`)

Урок: [Синхронизация данных](../../lessons/03-sync.md)

Коллега написал счётчик запросов, и на проде цифры «плывут», а иногда процесс падает с
`fatal error: concurrent map writes`:

```go
type Stats struct {
	total int64
	byKey map[string]int64
}

func (s Stats) Hit(key string) {   // (1) value-receiver: изменения теряются
	s.total++                      // (2) data race: неатомарный read-modify-write
	s.byKey[key]++                 // (3) конкурентная запись в map -> fatal error
}

func (s *Stats) All() map[string]int64 { return s.byKey } // (4) отдаёт внутреннюю map наружу
```

Найдите в этом коде все четыре проблемы (запустите его с `go run -race`) и напишите правильную версию:

```go
type Counter struct{ /* ... */ }        // atomic.Int64 внутри
func (c *Counter) Inc()
func (c *Counter) Add(n int64) int64     // возвращает новое значение
func (c *Counter) Value() int64

type KeyCounter struct{ /* ... */ }     // sync.RWMutex + map
func (k *KeyCounter) Inc(key string) int64
func (k *KeyCounter) Get(key string) int64
func (k *KeyCounter) Len() int
func (k *KeyCounter) Snapshot() map[string]int64 // копия
func (k *KeyCounter) Reset() map[string]int64    // вернуть старые значения и обнулить
```

Требования:

- Нулевое значение обоих типов готово к использованию: `var k KeyCounter; k.Inc("a")` должно работать. `Snapshot` и `Reset` на пустом счётчике возвращают пустую не-nil карту.
- `Snapshot` возвращает копию. Вызывающий может её менять, и это не влияет на счётчик и не приводит к гонкам.
- `Reset` атомарен: ни один инкремент не теряется между «прочитать» и «обнулить». Тест это проверяет, складывая результаты всех вызовов `Reset` и остаток.
- Все тесты проходят под `-race`.

## Подсказки

Читающие методы `Get`, `Len` и `Snapshot` берут `RLock`, а изменяющие `Inc` и `Reset` — `Lock`.

Типы, внутри которых лежит мьютекс или атомик, нельзя копировать. Если передать `KeyCounter` по значению, `go vet` (анализатор `copylocks`) об этом предупредит, так что все методы объявляйте на указателе.

Когда тесты пройдут, запустите `go test -bench . -tags solution` и сравните атомик с мьютексом, а заодно посмотрите, как ведёт себя `RLock` под конкуренцией.

## Запуск проверки

```
go test -race ./modules/03-concurrency/tasks/03_counter/
```
