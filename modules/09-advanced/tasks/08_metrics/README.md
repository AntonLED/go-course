# 08. Мини-реестр метрик в формате Prometheus

Урок: [Логгирование, трейсинг, метрики](../../lessons/06-observability.md)

Prometheus раз в несколько секунд приходит на `/metrics` каждого сервиса и забирает оттуда текст в простом построчном формате. Чтобы этот текст появился, в Go обычно подключают `prometheus/client_golang`. Здесь вы напишете его упрощённую версию: Counter, Gauge и Histogram с лейблами и вывод в **text exposition format 0.0.4**.

```go
reg := metrics.NewRegistry()
reqs, _ := reg.NewCounter("http_requests_total", "Всего запросов.", "method", "code")
lat, _ := reg.NewHistogram("http_duration_seconds", "Латентность.", nil, "method")
reqs.WithLabelValues("GET", "200").Inc()
lat.WithLabelValues("GET").Observe(0.042)
mux.Handle("GET /metrics", reg.Handler())
```

## Формат вывода (`WriteText`)
```
# HELP http_requests_total Всего запросов.
# TYPE http_requests_total counter
http_requests_total{method="GET",code="200"} 3
http_requests_total{method="POST",code="500"} 1
# HELP http_duration_seconds Латентность.
# TYPE http_duration_seconds histogram
http_duration_seconds_bucket{method="GET",le="0.1"} 1
http_duration_seconds_bucket{method="GET",le="+Inf"} 3
http_duration_seconds_sum{method="GET"} 7.4
http_duration_seconds_count{method="GET"} 3
```

- Метрики сортируются по имени, а серии внутри метрики — по значениям лейблов, лексикографически и в порядке лейблов. Сами лейблы выводятся в порядке объявления.
- Числа форматируются через `strconv.FormatFloat(v, 'g', -1, 64)`; бесконечности выводятся как `+Inf` и `-Inf`, нечисло — как `NaN`.
- В значениях лейблов экранируются `\` (превращается в `\\`), `"` (в `\"`) и перевод строки (в `\n`). В HELP экранируются только `\` и перевод строки.
- Метрика без лейблов видна сразу после регистрации со значением 0; у гистограммы все бакеты тоже 0. Метрика с лейблами, у которой ещё нет ни одной серии, выводит только `# HELP` и `# TYPE`.
- Бакеты гистограммы кумулятивные, а граница включительная: наблюдение $v$ попадает в бакет $\mathit{le}$, если $v \le \mathit{le}$ (`v <= le`). Бакет `+Inf` добавляется всегда, но если пользователь сам передал `+Inf`, дублировать его не нужно. `_count` равен значению бакета `+Inf`. При `buckets == nil` используются `DefBuckets`.
- `Handler()` ставит заголовок `Content-Type`, равный константе `ContentType`.

## Валидация

- Имя метрики должно соответствовать `[a-zA-Z_:][a-zA-Z0-9_:]*`, имя лейбла — `[a-zA-Z_][a-zA-Z0-9_]*`. Лейбл не может начинаться с `__`, повторяться или (у гистограммы) называться `le`. Во всех этих случаях возвращается `ErrInvalidName`.
- Повторная регистрация имени, неважно с каким типом, даёт `ErrDuplicate`.
- Бакеты должны строго возрастать, иначе — `ErrBuckets`.
- Неверное число значений в `WithLabelValues` и `Counter.Add` с отрицательным `v` вызывают panic. Это ошибка программиста, и client_golang поступает так же.

## Конкурентность

Всё должно быть потокобезопасным, а экспорт может идти параллельно с записью. На горячем пути — `WithLabelValues` для уже существующей серии и `Inc` — не берите эксклюзивную блокировку, иначе каждый HTTP-запрос будет стоять в очереди за мьютексом метрик. Хорошо работает `RWMutex` с double-checked созданием серии, а сами значения храните в `atomic`; `float64` — через `math.Float64bits` и CAS.

## Подвох: кардинальность

Каждая уникальная комбинация значений лейблов — это отдельная серия, которая живёт в памяти процесса (и в Prometheus) навсегда. Поэтому никогда не кладите в лейблы user_id, полный URL с параметрами или текст ошибки: миллион пользователей превратится в миллион серий.

## Проверка
```
go test ./modules/09-advanced/tasks/08_metrics/
```
