# 01. Graceful shutdown и health-чеки

Пакет `graceful`. Урок: [Основы HTTP и запуск сервера](../../lessons/01-http-server.md).

Почти каждый боевой сервис на Go начинается с одного и того же каркаса. Это сервер с таймаутами, эндпоинты `/healthz` и `/readyz`, по которым оркестратор понимает, жив ли сервис и можно ли слать ему трафик, и корректная остановка по SIGTERM, при которой запросы, уже попавшие в работу, успевают завершиться. Этот каркас мы и напишем.

## Что сделать

```go
func NewServer(h http.Handler) *http.Server
func Health(ready *atomic.Bool) http.Handler
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, shutdownTimeout time.Duration) error
func Run(ctx context.Context, addr string, h http.Handler) error
```

`NewServer` возвращает `http.Server` с `Handler: h` и всеми таймаутами. `ReadHeaderTimeout` должен быть больше нуля и не больше 10s, а `ReadTimeout`, `WriteTimeout`, `IdleTimeout` и `MaxHeaderBytes` — просто больше нуля.

`Health` обслуживает два пути. `GET /healthz` отвечает `200 ok`. `GET /readyz` отвечает `200 ready`, если `ready != nil && ready.Load()`, и `503` в остальных случаях. На другие методы нужно ответить 405, на другие пути — 404; и то и другое `ServeMux` сделает сам.

`Serve` запускает `srv.Serve(ln)` и ждёт отмены `ctx`, после чего вызывает `srv.Shutdown` с таймаутом `shutdownTimeout`. Дальше возможны три исхода:

- активные запросы успели доработать, и `Serve` возвращает `nil`;
- запросы не уложились в таймаут, тогда нужно вызвать `srv.Close()` и вернуть ошибку, оборачивающую `context.DeadlineExceeded`;
- `srv.Serve` завершился сам, не дожидаясь отмены (например, закрыли листенер), и его ошибку надо вернуть сразу.

`Run` просто собирает всё вместе: `net.Listen("tcp", addr)`, а затем `Serve(ctx, NewServer(h), ln, 10*time.Second)`.

В `main` это используется так:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
if err := graceful.Run(ctx, ":8080", mux); err != nil { log.Fatal(err) }
```

## Подвохи

После `Shutdown` метод `srv.Serve` возвращает `http.ErrServerClosed`, и это не ошибка, а штатное завершение.

Контекст для `Shutdown` нельзя строить из `ctx`: он к этому моменту уже отменён, и shutdown завершится мгновенно, не дождавшись запросов.

`Shutdown` не ждёт hijacked-соединения, например WebSocket. Для них предусмотрен `RegisterOnShutdown`.

## Запуск проверки

```
go test -race ./modules/05-web/tasks/01_graceful/
```
