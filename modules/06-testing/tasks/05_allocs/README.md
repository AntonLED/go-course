# 05. Убрать аллокации: JoinInts, SumCSV, AppendRecord, Render

Пакет `allocs`. Урок: [Бенчмарки](../../lessons/03-benchmarks.md).

В горячем коде каждая лишняя аллокация — это работа для сборщика мусора и лишние микросекунды на запрос, а на миллионах запросов они складываются в заметную нагрузку. Эта задача учит видеть, откуда аллокации берутся, и убирать их, не меняя поведения.

Четыре функции в заготовке `allocs.go` уже работают правильно, и тесты на корректность проходят. Падают только проверки через `testing.AllocsPerRun`: функции аллоцируют слишком много. Перепишите их так, чтобы результат совпадал с эталоном байт в байт, а число аллокаций не превышало лимит.

| Функция | Что делает | Лимит аллокаций | Сейчас |
|---|---|---|---|
| `JoinInts(xs []int, sep string) string` | `"1, -2, 3"` | не больше 1 (0 для пустого) | $N+1$ |
| `SumCSV(s string) (int64, error)` | сумма чисел через запятую | 0 на успешном пути | 1 |
| `AppendRecord(dst []byte, r Record) []byte` | `id=42 name="Ann" score=3.5 active=true tags=a\|b\n` | 0 при достаточной ёмкости `dst` | 7 |
| `Render(w io.Writer, user string, items []string) error` | `"user: a, b\n"` одним `Write` | 0 | 3 |

## Как измерять

```
go test -run XXX -bench . -benchmem ./modules/06-testing/tasks/05_allocs/
go test -gcflags=-m ./modules/06-testing/tasks/05_allocs/ 2>&1 | grep escapes   # что утекает в кучу
```

Чтобы сравнить «до» и «после» статистически корректно, снимите `go test -bench . -count=10 > old.txt`, внесите правку, снимите то же самое в `> new.txt` и запустите `benchstat old.txt new.txt` (`golang.org/x/perf/cmd/benchstat`).

## Подсказки

Функции `strconv.AppendInt/AppendQuote/AppendFloat/AppendBool` пишут прямо в `[]byte`, без промежуточных строк.

`strings.Builder` с предварительным `Grow(n)` делает ровно одну аллокацию, а его `String()` не копирует буфер.

Подстроки `s[i:j]` не аллоцируют, в отличие от `strings.Split`.

`fmt.Sprintf` аллоцирует результат и вдобавок упаковывает аргументы в `interface{}`.

Если решите использовать `sync.Pool`, кладите туда указатель (`*bytes.Buffer`) и не возвращайте в пул гигантские буферы.

Под `-race` лимиты аллокаций не проверяются: детектор гонок добавляет свои аллокации, а `sync.Pool` намеренно «теряет» объекты. Поэтому лимиты проверяйте без `-race`.

## Запуск проверки

```
go test ./modules/06-testing/tasks/05_allocs/          # лимиты аллокаций
go test -race ./modules/06-testing/tasks/05_allocs/    # корректность и гонки
```
