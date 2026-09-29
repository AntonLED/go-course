# Задача 09. Репозиторий на `database/sql`: CRUD, NULL, транзакции

Слой доступа к данным есть почти в каждом бэкенде, и именно в нём чаще всего прячутся неприятные ошибки: SQL-инъекции, незакрытые `rows`, потерянные `NULL`, переводы денег, которые списали с одного счёта и не зачислили на другой. В этой задаче мы напишем небольшой репозиторий пользователей и счетов и пройдём через все эти ловушки на практике.

Пакет `userrepo`. Тип `User`, ошибки, `Schema`, `Repo`, `New` и `Migrate` уже есть в `types.go`. Реализуйте методы в `userrepo.go`:

```go
func (r *Repo) Create(ctx context.Context, u *User) error            // пишет u.ID
func (r *Repo) Get(ctx context.Context, id int64) (User, error)
func (r *Repo) List(ctx context.Context, limit, offset int) ([]User, error)
func (r *Repo) Update(ctx context.Context, u User) error
func (r *Repo) Delete(ctx context.Context, id int64) error
func (r *Repo) OpenAccount(ctx context.Context, userID, initial int64) (int64, error)
func (r *Repo) Balance(ctx context.Context, accountID int64) (int64, error)
func (r *Repo) Transfer(ctx context.Context, from, to, amount int64) error
```

В роли базы выступает учебный драйвер [`memsql`](../../memsql), открывается он через `sql.Open("memsql", dsn)`. Плейсхолдеры в нём записываются как `?`. Столбец `INTEGER PRIMARY KEY` автоинкрементируется, и новый ID отдаётся через `Result.LastInsertId()`. Нарушение `UNIQUE` возвращает ошибку, для которой `errors.Is(err, memsql.ErrUniqueViolation)`.

## Требования

- Наружу отдаются только доменные ошибки: `ErrNotFound` (вместо `sql.ErrNoRows` и вместо «0 затронутых строк»), `ErrDuplicateEmail`, `ErrInvalid`, `ErrInsufficientFunds`. Тесты проверяют их через `errors.Is`, так что оборачивать через `%w` можно.
- `ErrInvalid` возвращается для пустого или состоящего из пробелов имени, email без `@`, `Age < 0`, отрицательного начального баланса в `OpenAccount`, а в `List` — для `offset < 0`. Если `limit <= 0`, используется значение 100.
- `Phone sql.NullString` и `Age *int` отображаются на `NULL` в обе стороны: пустое значение записывается как `NULL`, а `NULL` читается обратно как пустое значение. Тест проверяет, что в базе лежит именно `NULL`.
- `List` возвращает записи по возрастанию ID. Для пустой базы результат — пустой, но не nil, срез.
- `Transfer` выполняется в одной транзакции: `BeginTx`, затем `defer tx.Rollback()`, запросы через `tx` и в конце `Commit`. Если что-то пошло не так (нет получателя, не хватает денег), баланс отправителя не меняется. При `amount <= 0` или `from == to` возвращается `ErrInvalid`.
- Все запросы выполняются с `ctx`: `QueryRowContext`, `ExecContext` и так далее.

## Подвохи

В тестах стоит `db.SetMaxOpenConns(1)`. Если внутри транзакции обратиться к `r.db` вместо `tx`, запрос будет ждать единственное соединение, которое держит сама транзакция. Получится дедлок, и тест упадёт по таймауту контекста. То же самое произойдёт, если оставить `rows` незакрытыми.

После цикла по `rows` обязательно проверяйте `rows.Err()`: `Next()` возвращает `false` не только в конце данных, но и при ошибке.

Никакой конкатенации значений в SQL, только плейсхолдеры. В тестах есть значение `x'); DELETE FROM users; --`.

`RowsAffected()` у `UPDATE`, который ничего не нашёл, равен 0, и это и есть признак «не найдено». (В MySQL по умолчанию `UPDATE` без фактических изменений тоже даёт 0, и там нужен флаг `clientFoundRows`.)

Сканировать `NULL` в обычный `string` или `int` нельзя, получится ошибка. Используйте `sql.Null*` или указатели: `&u.Age` имеет тип `**int`, и это работает.

## Проверка

```
cd go-course  # корень курса
go test ./modules/04-data/tasks/09_userrepo/
go test -tags solution ./modules/04-data/tasks/09_userrepo/   # эталон
```
