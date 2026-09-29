# 06. Задание 7: orders → inventory

Финальная задача модуля «Основы микросервисов в Go».

Здесь сходится всё, что было в модуле. Два «микросервиса» живут в одном процессе, но общаются только по сети (в тестах — через `httptest.Server`): сервис заказов резервирует товар на складе по JSON-RPC 2.0. Написать сам резерв несложно. Вся соль в том, чтобы система вела себя честно, когда склад тормозит, падает или теряет ответы: нужны таймауты, ретраи, идемпотентность, circuit breaker и аккуратный маппинг ошибок в HTTP-коды.

```
клиент ──HTTP/REST──▶ orders ──JSON-RPC 2.0 (POST /rpc)──▶ inventory
                     │  InventoryClient:                   │  Inventory:
                     │  timeout/attempt, retry, breaker    │  идемпотентный Reserve(OrderID)
```

## Что сделать

Код пишется в `orders.go`. Типы, ошибки, коды и `ClientConfig` уже есть в `types.go`.

```go
// inventory
func NewInventory(stock map[string]int) *Inventory
func (inv *Inventory) Stock(sku string) int
func (inv *Inventory) Reserve(ctx context.Context, req ReserveRequest) (ReserveResult, error)
func NewInventoryHandler(inv *Inventory) http.Handler            // POST /rpc, GET /healthz

// клиент (реализует InventoryAPI)
func NewInventoryClient(baseURL string, cfg ClientConfig) *InventoryClient
func (c *InventoryClient) Reserve(ctx context.Context, req ReserveRequest) (ReserveResult, error)

// orders
func NewOrdersHandler(api InventoryAPI) http.Handler             // POST /orders, GET /orders/{id}
```

### Inventory

Склад должен быть потокобезопасным, а `Reserve` — идемпотентным по `OrderID`: повторный вызов с тем же `OrderID` возвращает прежний результат и не списывает товар ещё раз. Без этого ретраи и «потерянные ответы» будут списывать товар дважды. Если тот же `OrderID` пришёл с другими параметрами, `Reserve` возвращает `ErrInvalid`.

Наружу склад торчит JSON-RPC-методом `Inventory.Reserve`, в `params` которого лежит `ReserveRequest`. Ошибки отображаются в коды `CodeOutOfStock`, `CodeUnknownSKU`, `CodeInvalidParams`, `CodeMethodNotFound`, `CodeParseError`, `CodeInternal`.

### InventoryClient

Главное в клиенте — отличать ошибки, которые стоит повторить, от тех, что повторять бессмысленно:

| Ситуация | Повтор? | Неудача для breaker? | Результат |
|---|---|---|---|
| успех | — | нет (сброс) | `ReserveResult` |
| `CodeOutOfStock` / `CodeUnknownSKU` / `CodeInvalidParams` | нет | нет | `ErrOutOfStock` / `ErrUnknownSKU` / `ErrInvalid` |
| сетевая ошибка, таймаут попытки, HTTP ≠ 200, `CodeInternal` | да | да | после исчерпания — `Is(ErrUnavailable)` |
| breaker разомкнут | нет | — | сразу `Is(ErrUnavailable)`, без запроса |
| ctx вызывающего истёк/отменён | нет | нет | `Is(ctx.Err())` |

Каждая попытка выполняется под `context.WithTimeout(ctx, AttemptTimeout)`. Пауза после неудачной попытки $n$ равна $\text{BaseDelay} \cdot 2^{n-1}$ и прерывается отменой ctx.

Breaker считает неудачные попытки, а не вызовы: после `FailureThreshold` неудачных попыток подряд он размыкается на `OpenTimeout` (время берётся только из `cfg.Now`). Затем пропускается одна пробная попытка: успех замыкает цепь, неудача снова её размыкает.

### Orders

`POST /orders {"sku":"A","qty":2}` генерирует уникальный ID заказа и вызывает `Reserve` с `OrderID = ID` и контекстом входящего запроса. Коды ответов:

- `201` при успехе, с заголовком `Location`;
- `400` для некорректного запроса (и когда склад ответил `ErrInvalid`);
- `409`, если товара нет (out of stock);
- `422` для неизвестного SKU;
- `503`, если склад недоступен, с заголовком `Retry-After`;
- `504`, если истёк дедлайн;
- `500` во всех прочих случаях.

Тело ошибки — `{"error": "..."}`. `GET /orders/{id}` отвечает `200` для известного заказа и `404` для неизвестного.

## Подвохи

Классический «потерянный ответ»: склад выполнил резерв, а клиент получил 502 и повторил запрос. Без идемпотентности по `OrderID` товар спишется дважды. Похожая история с зависшей попыткой: она может выполниться на сервере уже после того, как клиент по таймауту ушёл на вторую попытку.

Бизнес-отказы вроде «нет товара» не должны размыкать breaker. Зависимость исправна, она просто честно сказала «нет».

И обратите внимание на порядок проверок при выборе HTTP-кода. `errors.Is(err, ErrUnavailable)` важнее, чем `DeadlineExceeded` внутри этой ошибки: если все попытки исчерпаны по таймаутам, это 503, а не 504.

## Запуск проверки

```bash
go test -race ./modules/07-microservices/tasks/06_project_orders/
go test -race -tags solution ./modules/07-microservices/tasks/06_project_orders/   # эталон
```
