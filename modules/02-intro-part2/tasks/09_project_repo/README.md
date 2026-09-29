# Задание 2. Generic in-memory репозиторий с индексами

Пакет `repo`. Почти в каждом сервисе есть слой хранения, и для тестов или прототипа его часто делают в памяти: карта по ID плюс пара индексов для поиска по email или городу. В этой финальной задаче модуля вы напишете такой репозиторий в обобщённом виде, и в нём сойдётся всё, что было в уроках: интерфейсы (ограничение `Entity`), ошибки (sentinel'ы, собственный тип `ConflictError` с методом `Is`, обёртки с двумя `%w`), дженерики (`Repo[ID cmp.Ordered, E Entity[ID]]`) и итераторы (`iter.Seq` и `iter.Seq2` по снимку данных).

Типы, ошибки и структура `Repo` объявлены в `types.go`. Реализуйте в `repo.go`:

```go
func New[ID cmp.Ordered, E Entity[ID]]() *Repo[ID, E]

func (r *Repo[ID, E]) AddIndex(name string, key func(E) string, unique bool) error
func (r *Repo[ID, E]) Insert(e E) error
func (r *Repo[ID, E]) Update(e E) error
func (r *Repo[ID, E]) Delete(id ID) error
func (r *Repo[ID, E]) Get(id ID) (E, error)
func (r *Repo[ID, E]) Len() int

func (r *Repo[ID, E]) All() iter.Seq2[ID, E]                          // по возрастанию ID
func (r *Repo[ID, E]) Where(pred func(E) bool) iter.Seq[E]
func (r *Repo[ID, E]) Find(index, key string) (iter.Seq[E], error)
func (r *Repo[ID, E]) Lookup(index, key string) (E, error)

func Paginate[T any](s iter.Seq[T], offset, limit int) iter.Seq[T]
```

## Пример

```go
type User struct{ ID int; Email, City string }
func (u User) GetID() int      { return u.ID }
func (u User) Validate() error { /* ... */ }

r := repo.New[int, User]()
r.AddIndex("email", func(u User) string { return strings.ToLower(u.Email) }, true)
r.AddIndex("city", func(u User) string { return u.City }, false)

err := r.Insert(User{ID: 1, Email: "a@x.io", City: "Москва"})
err = r.Insert(User{ID: 2, Email: "A@X.IO"})
// errors.Is(err, repo.ErrConflict) == true
// errors.As(err, &ce) -> ce.Index == "email", ce.Key == "a@x.io"

moscow, _ := r.Find("city", "Москва")
page := slices.Collect(repo.Paginate(moscow, 20, 10))
```

## Требования к ошибкам

| Ситуация | Ошибка (через `errors.Is`) |
|---|---|
| `Validate() != nil` | и `ErrInvalid`, и исходная ошибка (`fmt.Errorf("...: %w: %w", ErrInvalid, err)`) |
| ID уже есть, индекс уже есть или пустое имя индекса | `ErrDuplicate` |
| нарушение уникального индекса | `*ConflictError` (а через его метод `Is` — ещё и `ErrConflict`) |
| нет сущности | `ErrNotFound` |
| неизвестный индекс | `ErrUnknownIndex` |

Ошибка всегда возвращается с контекстом (`repo: insert 5: ...`), «голый» sentinel — никогда.

## Подвохи

Начнём с атомарности. Сначала выполняются все проверки, включая все уникальные индексы, и только потом все изменения. Неудачный `Insert` не должен оставить следов ни в одном индексе. `Update` не конфликтует сам с собой, а старые ключи обновлённой сущности из индексов удаляются.

Вторая ловушка — итераторы и блокировки. `sync.RWMutex` не реентерабелен: если держать `RLock` во время вызова `yield`, а в теле цикла вызвать `Insert`, получится дедлок. Поэтому делайте снимок данных под блокировкой и отпускайте её до начала обхода. Тест проверяет это с таймаутом.

Порядок выдачи должен быть детерминированным — по возрастанию ID (например, через `slices.Sorted(maps.Keys(...))`), хотя сами данные лежат в карте.

Если уникальный индекс добавляют поверх данных, которые ему противоречат, `AddIndex` возвращает `ConflictError`, и индекс не создаётся.

`Paginate` не должен запрашивать у источника элементы после позиции `offset+limit`.

И наконец, всё должно корректно работать под `-race` при конкурентных вставках и чтениях.

## Проверка

```
go test -race ./modules/02-intro-part2/tasks/09_project_repo/
```
