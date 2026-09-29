# 07. Профили из кода: CPU, heap и «топ горутин»

Пакет `profiling`. Урок: [Профилирование](../../lessons/04-profiling.md).

`go test -cpuprofile` и `net/http/pprof` удобны, но иногда профиль нужно снять программно: по сигналу, при превышении порога латентности или в CLI-утилите, где HTTP-сервера нет. А разбор профиля горутин — лучший способ найти утечку горутин в работающем сервисе: если одна функция держит десять тысяч горутин, это сразу видно.

## Что сделать

```go
func CPUProfile(w io.Writer, work func()) error
func HeapProfile(w io.Writer) error
func ParseGoroutineProfile(r io.Reader) ([]FuncCount, error)
func GoroutineTop(n int) ([]FuncCount, error)
```

`CPUProfile` вызывает `pprof.StartCPUProfile(w)`, затем `work()`, затем `pprof.StopCPUProfile()`. CPU-профиль в процессе может идти только один, поэтому если он уже запущен, функция возвращает ошибку и не вызывает `work`. Результат записывается в формате gzip-protobuf, который открывает `go tool pprof`.

`HeapProfile` сначала вызывает `runtime.GC()`, потому что статистика heap-профиля обновляется на сборке мусора, а затем `pprof.Lookup("heap").WriteTo(w, 0)`.

`GoroutineTop(n)` снимает профиль горутин текущего процесса, разбирает его и возвращает первые `n` записей. При `n < 0` возвращаются все записи, при `0` — пустой результат.

### ParseGoroutineProfile

Эта функция разбирает текстовый формат, который выдаёт `pprof.Lookup("goroutine").WriteTo(w, 1)`:

```
goroutine profile: total 5
4 @ 0x474cee 0x40ee85 0x5b8805 0x47c221
#	0x5b8804	example.com/app/worker.(*Pool).run+0x24	/app/worker/pool.go:31

1 @ 0x474cee 0x40ee85 0x5b87a5 0x47c221
# labels: {"handler":"upload"}
#	0x40ee84	runtime.chanrecv1+0x14	/usr/local/go/src/runtime/chan.go:489
#	0x5b87a4	example.com/app/worker.(*Pool).run+0x24	/app/worker/pool.go:31
```

Правила разбора:

1. Первая строка должна иметь вид `goroutine profile: total N`, иначе возвращается `ErrMalformed`.
2. Запись начинается строкой `<count> @ <адреса>`, за ней идут кадры вида `#<TAB>адрес<TAB>функция+0xсмещение<TAB>файл:строка`. Табов-разделителей может быть несколько, так что разбивайте строку через `strings.Fields`. Строки `# labels:` пропускаются. Пустая строка завершает запись.
3. Имя функции берётся без `+0x…`. Горутины записи приписываются первой функции, имя которой не начинается с `runtime.` (при этом `runtime/pprof.…` — это уже не рантайм). Если все кадры рантаймовые, горутины приписываются первой функции.
4. Одинаковые функции из разных записей суммируются.
5. Сумма `count` по всем записям должна равняться `N`, иначе возвращается `ErrMalformed`. Любая другая нераспознанная строка — кадр вне записи, кривой `count`, кадр без функции — тоже приводит к `ErrMalformed`.
6. Результат сортируется по убыванию `Count`, а при равенстве — по возрастанию `Func`.

## Попробуйте руками

```
go test -run XXX -bench . -cpuprofile cpu.out -memprofile mem.out ./modules/06-testing/tasks/06_hotspot/
go tool pprof -top cpu.out
go tool pprof -sample_index=alloc_space -top mem.out
```

## Запуск проверки

```
go test -race ./modules/06-testing/tasks/07_pprof/
```
