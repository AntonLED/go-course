# Задание 9. Production-ready каркас сервиса

Итоговая задача модуля. Бизнес-логика здесь нарочно тривиальная — это key-value хранилище, — потому что вся суть в обвязке: конфигурация из нескольких источников, структурные логи с request ID, трейсинг, метрики, пробы и корректная остановка. Вам нужно собрать сервис со всем, что от него ждут в проде. Используйте знания всех уроков модуля, а свои решения задач 02–10 — как образец, но без импорта их пакетов: проект самодостаточен.

Типы `Config`, `Store`, `Option`, `WithLogWriter`, `WithStore` и константы уже есть в `types.go`. Точка входа — [`cmd/kvservice/main.go`](cmd/kvservice/main.go), она уже написана.

```go
func LoadConfig(args []string, lookup func(string) (string, bool)) (Config, error)
func New(cfg Config, opts ...Option) (*App, error)
func (a *App) Handler() http.Handler
func (a *App) Run(ctx context.Context, ln net.Listener) error
```

## 1. Конфигурация (`LoadConfig`)

Слои накладываются в порядке `DefaultConfig()` < окружение < явно заданные флаги:

| Поле | env | флаг |
|---|---|---|
| `Addr` | `SVC_ADDR` | `-addr` |
| `LogLevel` (`slog.Level`, `debug/info/warn/error`, регистр не важен) | `SVC_LOG_LEVEL` | `-log-level` |
| `ShutdownTimeout` | `SVC_SHUTDOWN_TIMEOUT` | `-shutdown-timeout` |
| `DrainDelay` | `SVC_DRAIN_DELAY` | `-drain-delay` |

`LoadConfig` возвращает ошибку, если значение не разбирается, флаг неизвестен, в `Addr` нет порта или `ShutdownTimeout <= 0`. Подсказка: у `flag.FlagSet` есть `TextVar`, а `slog.Level` реализует `encoding.TextUnmarshaler`.

## 2. Сборка (`New`) — composition root

Все зависимости создаются в `New` и передаются дальше через конструкторы в таком порядке: логгер, хранилище, метрики, HTTP-обработчики, middleware. Хранилище `Store` по умолчанию in-memory, а опция `WithStore` позволяет его подменить — так тесты проверяют обработку ошибок хранилища. Никаких глобальных переменных.

## 3. HTTP API

| Маршрут | Ответ |
|---|---|
| `PUT /kv/{key}` | тело — значение; `204`; тело > `MaxBodyBytes` → `413` (`http.MaxBytesReader`) |
| `GET /kv/{key}` | `200` + значение; `ErrNotFound` → `404`; другая ошибка Store → `500` + лог ERROR |
| `GET /healthz` | `200` всегда (liveness) |
| `GET /readyz` | `200`, а после начала остановки — `503` (readiness) |
| `GET /metrics` | Prometheus text format |

На прочие методы сервис отвечает `405`, на неизвестные пути — `404`.

## 4. Middleware

Порядок middleware важен:

`requestID → tracing → accessLog(+recover) → metrics → ServeMux`

**Request ID.** Если во входящем запросе есть `X-Request-ID` и он соответствует `[A-Za-z0-9_-]{1,64}`, используйте его, иначе сгенерируйте новый. Проверка обязательна: без неё клиент может вписать в заголовок перевод строки и подделать записи в ваших логах (log injection). ID возвращается в заголовке ответа и кладётся в `context`.

**Трейсинг.** Если пришёл валидный `traceparent` (W3C, версия `00`), трасса продолжается с тем же trace-id, иначе начинается новая. Span-id генерируется заново, всё это кладётся в контекст.

**Логи.** Используется JSON (`slog.NewJSONHandler`) с уровнем `cfg.LogLevel`. Обёртка над `slog.Handler` добавляет `request_id`, `trace_id` и `span_id` из контекста к любой записи, сделанной через `*Context`-методы вроде `log.ErrorContext(ctx, ...)`. Запись access-лога содержит `msg="http request"`, `method`, `path`, `status` (числом) и `duration_ms` (числом).

**Recover.** Паника в обработчике превращается в ответ `500` и запись в лог с текстом паники, а сервис продолжает работать.

**Метрики.** Нужны три метрики:

- `http_requests_total{method,route,code}` — counter;
- `http_request_duration_seconds{method,route}` — histogram с бакетами как `DefBuckets` в задаче 08;
- `kv_keys` — gauge, значение берётся из `Store.Len()`.

Лейбл `route` — это шаблон маршрута из `r.Pattern`, например `"GET /kv/{key}"`, а для запросов, не совпавших ни с одним маршрутом, — `"unmatched"`. Шаблон, а не реальный путь, нужен, чтобы каждый новый ключ не порождал новую серию. Тут есть тонкость: `ServeMux` записывает `Pattern` в тот `*http.Request`, который получил сам, поэтому metrics-middleware должен стоять вплотную к mux. Паника тоже учитывается в метриках как `500`.

## 5. Graceful shutdown (`Run`)

При отмене `ctx` сервис останавливается в три шага. Сначала `readyz` начинает отвечать `503`. Затем выдерживается пауза `DrainDelay`: сервер всё ещё принимает запросы, а балансировщик тем временем успевает заметить 503 и убрать под из ротации. Наконец вызывается `Shutdown` с таймаутом `ShutdownTimeout`, и активные запросы дорабатывают. При успехе `Run` возвращает `nil` и пишет в лог `"shutdown complete"`, а по таймауту возвращает ошибку.

## 6. Dockerfile

Замените заготовку `Dockerfile` на production-ready, как в задаче 10. Нужны multi-stage сборка, фиксированные теги, `CGO_ENABLED=0`, `-trimpath`, `-ldflags "-s -w -X main.version=..."`, финальный образ distroless или scratch, `USER` не root, `EXPOSE`, `ENTRYPOINT ["/kvservice"]` и никакого `COPY . .` в финальной стадии.

## Запуск
```
go run ./modules/09-advanced/tasks/11_project_service/cmd/kvservice -log-level=debug
curl -X PUT -d hello localhost:8080/kv/greeting
curl -H 'traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01' localhost:8080/kv/greeting
curl localhost:8080/metrics
```

## Проверка
```
go test -race ./modules/09-advanced/tasks/11_project_service/
```
