# 03. JSON-RPC 2.0: сервер и клиент

Задача к уроку [«JSON-RPC»](../../lessons/02-jsonrpc.md).

В стандартной библиотеке есть `net/rpc/jsonrpc`, но он реализует только JSON-RPC 1.0 поверх TCP. Современные сервисы говорят на 2.0 поверх HTTP, и спецификация у этой версии короткая, но придирчивая: отдельно оговорены нотификации, пакеты, `id: null` и коды ошибок. Здесь вы напишете сервер и клиент, строго соблюдающие [спецификацию](https://www.jsonrpc.org/specification).

## Что сделать

Код пишется в `jsonrpc.go`. Коды ошибок, тип `Error` и `BatchElem` уже есть в `types.go`.

```go
func NewServer() *Server
func Register[P, R any](s *Server, method string, fn func(ctx context.Context, params P) (R, error))
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request)

func NewClient(url string, hc *http.Client) *Client
func (c *Client) Call(ctx context.Context, method string, params, result any) error
func (c *Client) Notify(ctx context.Context, method string, params any) error
func (c *Client) Batch(ctx context.Context, elems []*BatchElem) error
```

`Register` — это generic-обёртка. Вы пишете обычный типизированный обработчик, а сервер хранит его в «сырой» форме `func(ctx, json.RawMessage) (any, error)` и сам распаковывает параметры:

```go
s := jsonrpc.NewServer()
jsonrpc.Register(s, "sum", func(ctx context.Context, xs []int) (int, error) { ... })
http.Handle("/rpc", s)
```

## Правила сервера

| Вход | Ответ |
|---|---|
| не POST | HTTP 405 |
| невалидный JSON (в т.ч. битый пакет) | `-32700 Parse error`, `id: null` |
| `jsonrpc != "2.0"`, `method` не строка/пустой, `id` не строка/число/null, `params` не массив/объект | `-32600 Invalid Request` (`id` — исходный, если он валиден, иначе `null`) |
| неизвестный метод | `-32601 Method not found` |
| params не распаковались в `P` | `-32602 Invalid params` |
| обработчик вернул `*Error` | этот объект как есть |
| другая ошибка или паника | `-32603 Internal error` — без текста исходной ошибки |
| нотификация (нет поля `id`) | ответа нет; одиночная → HTTP 204 |
| пакет `[...]` | массив ответов без нотификаций; `[]` → одиночная `-32600`; нечего ответить → 204 |

Все JSON-ответы отдаются с HTTP 200 и `Content-Type: application/json`. В ответе присутствует ровно одно из полей `result` и `error`; при этом `"result": null` — вполне допустимый результат. Обработчик получает контекст запроса `r.Context()`.

## Правила клиента

- В качестве `id` клиент использует уникальные числа. Клиентом пользуются из многих горутин, так что счётчик должен быть атомарным.
- Ошибка из ответа сервера возвращается как `*Error`, чтобы её можно было достать через `errors.As`.
- Статус, отличный от 200, и чужой `id` в ответе считаются ошибкой.
- В `Batch` ответы приходят в произвольном порядке, поэтому сопоставляйте их с запросами по `id`. Если на какой-то вызов ответа не пришло, запишите ошибку в его `elem.Error`.

## Подвохи

Запрос с `"id": null` — это именно запрос, а не нотификация, и на него нужно ответить. Поэтому отличайте отсутствие поля от значения `null`. Удобно читать `id` в `json.RawMessage` без `omitempty`: у отсутствующего поля будет `nil`, а у явного null — `"null"`.

Ошибку «метод не найден» для нотификации тоже не отправляют: нотификация не получает ответа ни при каких обстоятельствах.

Текст внутренней ошибки нельзя отдавать клиенту. Сообщение вроде `pq: password authentication failed…` рассказывает атакующему о вашей инфраструктуре больше, чем нужно.

И паника в одном обработчике не должна ронять весь сервер.

## Запуск проверки

```bash
go test -race ./modules/07-microservices/tasks/03_jsonrpc/
go test -race -tags solution ./modules/07-microservices/tasks/03_jsonrpc/   # эталон
```
