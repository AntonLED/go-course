# 02. Middleware: Chain, Recover, RequestID, Logging

Пакет `middleware`. Урок: [Роутинг и middleware](../../lessons/02-routing-middleware.md).

Есть вещи, которые нужны каждому HTTP-обработчику: не падать от паники, помечать запрос идентификатором, чтобы по нему потом искать логи, и записывать строку в журнал доступа. Писать это в каждом хендлере никто не хочет, поэтому такой код выносят в middleware — обёртки над `http.Handler`. Сейчас мы напишем четыре самых ходовых.

## Что сделать

```go
type Middleware func(http.Handler) http.Handler          // в types.go

func Chain(h http.Handler, mws ...Middleware) http.Handler
type StatusRecorder struct{ http.ResponseWriter; Status, Bytes int }
func NewStatusRecorder(w http.ResponseWriter) *StatusRecorder
func Recover(logger *slog.Logger) Middleware
func RequestID(next http.Handler) http.Handler
func RequestIDFrom(ctx context.Context) string
func Logging(logger *slog.Logger) Middleware
```

`Chain` собирает цепочку так, что `mws[0]` оказывается самым внешним: `Chain(h, A, B)` эквивалентно `A(B(h))`.

`StatusRecorder` запоминает первый записанный код ответа (по умолчанию 200) и число байт тела. Повторный `WriteHeader` он игнорирует. Метод `Unwrap() http.ResponseWriter` обязателен: без него `http.NewResponseController(w).Flush()` вернёт `ErrNotSupported`.

`Recover` перехватывает панику, пишет в лог запись уровня `ERROR` с сообщением `"panic"` и отвечает `500 {"error":"internal server error"}`. Исключение — паника со значением `http.ErrAbortHandler`: её нужно пробросить дальше, потому что это штатный способ оборвать ответ.

`RequestID` принимает входящий заголовок `X-Request-ID`, только если это от 1 до 64 символов из `[A-Za-z0-9_-]`. Всё остальное открывает дорогу инъекциям в логи и мусору, поэтому в таком случае, как и при отсутствии заголовка, мы генерируем свой ID из 16 hex-символов через `crypto/rand`. ID кладётся в контекст (ключом служит неэкспортируемый тип) и в заголовок ответа.

`Logging` делает на каждый запрос одну запись уровня `INFO` с сообщением `"request"` и атрибутами `method, path, status, bytes, duration, request_id`.

Типичная цепочка выглядит так: `Chain(mux, RequestID, Logging(l), Recover(l))`. Logging стоит снаружи Recover и поэтому видит код 500 после паники, а RequestID стоит снаружи всех, чтобы ID попал в логи.

## Подвохи

Если хендлер вызвал `Write`, не вызвав перед этим `WriteHeader`, код ответа неявно становится `200`, и recorder должен это учитывать.

Обёртка над `ResponseWriter` «прячет» реализованные исходным writer'ом `http.Flusher` и `Hijacker`. Именно поэтому нужен `Unwrap`.

Ключ контекста типа `string` — антипаттерн из-за возможных коллизий, и линтеры (staticcheck SA1029) на это ругаются.

## Запуск проверки

```
go test -race ./modules/05-web/tasks/02_middleware/
```
