# Задача 03. Ошибки: typed nil, errors.Join/Is/As, дерево ошибок, recover

Пакет `errs`. Представьте форму регистрации: пользователь ввёл пустое имя, возраст 200 и адрес без `@`. Хорошая валидация сообщит обо всех трёх проблемах сразу, а не заставит исправлять их по одной. В этой задаче вы соберёте такие ошибки в дерево, научитесь его обходить, добавлять контекст при обёртке и превращать панику в ошибку. Заодно встретитесь с самой знаменитой ловушкой Go — typed nil.

Общие типы (`ErrNotFound`, `User`, `ValidationError`, `PanicError`) лежат в `types.go`. Реализуйте в `errs.go`:

```go
func ValidateUser(u User) error                          // все нарушения через errors.Join, или nil
func FieldErrors(err error) []*ValidationError           // обход дерева ошибок
func FindUser(db map[int]User, id int) (User, error)     // "find user 42: not found", обёртка ErrNotFound
func ProfileName(db map[int]User, id int) (string, error) // "profile: find user 42: not found"
func (e *PanicError) Error() string                      // "panic: <value>"
func (e *PanicError) Unwrap() error                      // Value, если это error
func SafeCall(f func()) (err error)                      // panic -> *PanicError
```

## Правила валидации

Поля проверяются в том порядке, в каком перечислены в таблице, и порядок ошибок в результате важен.

| Поле | Условие | Ошибка |
|---|---|---|
| `name` | непустое после `TrimSpace` | `обязательное поле` |
| `age` | `0 <= Age <= 150` | `вне диапазона 0..150` |
| `email` | ровно один `@`, обе части непустые | `некорректный адрес` |

## Подвохи

Начните с того, что в заготовке `ValidateUser` уже есть баг: она возвращает `*ValidationError(nil)` как `error`, а такой интерфейс не равен `nil`. Исправьте её так, чтобы для валидного пользователя возвращался настоящий `nil`. Подсказка: `errors.Join()` без аргументов как раз возвращает `nil`.

`FieldErrors` должна уметь спускаться по обоим видам обёрток: и по `Unwrap() error` (так устроены ошибки из `fmt.Errorf("...%w")`), и по `Unwrap() []error` (так устроены `errors.Join` и `fmt.Errorf` с несколькими `%w`). Обход идёт в глубину: сначала сам узел, потом его дети слева направо.

Оборачивайте ошибки с контекстом через `%w`, не возвращайте sentinel «голым» и не логируйте ошибку там же, где её возвращаете.

В `SafeCall` помните, что `recover()` работает только в отложенной функции, а результат выставляется через именованное возвращаемое значение. Если паниковали ошибкой, она должна быть видна через `errors.Is` и `errors.As` — для этого и нужен метод `Unwrap`. Runtime-паники реализуют `runtime.Error`, а `panic(nil)` начиная с Go 1.21 даёт `*runtime.PanicNilError`.

## Проверка

```
go test ./modules/02-intro-part2/tasks/03_errs/
```
