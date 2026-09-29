//go:build !solution

// Package orders — два микросервиса (orders → inventory) в одном процессе,
// общающиеся по JSON-RPC 2.0 с таймаутами, ретраями и circuit breaker.
package orders

import (
	"context"
	"net/http"
)

// ============================================================ inventory

// Inventory — in-memory склад с идемпотентным резервированием.
type Inventory struct {
	// TODO: поля (mutex, остатки, выполненные резервы по OrderID)
}

// NewInventory создаёт склад с копией начальных остатков.
func NewInventory(stock map[string]int) *Inventory {
	// TODO: реализуйте
	return &Inventory{}
}

// Stock возвращает текущий остаток SKU (0 для неизвестного).
func (inv *Inventory) Stock(sku string) int {
	// TODO: реализуйте
	return -1
}

// Reserve списывает req.Qty единиц req.SKU.
//   - пустой OrderID/SKU или Qty <= 0 → ErrInvalid;
//   - неизвестный SKU → ErrUnknownSKU; не хватает → ErrOutOfStock (остаток не меняется);
//   - повтор с тем же OrderID и теми же параметрами → прежний результат без
//     повторного списания; тот же OrderID с другими параметрами → ErrInvalid.
func (inv *Inventory) Reserve(ctx context.Context, req ReserveRequest) (ReserveResult, error) {
	// TODO: реализуйте
	panic("TODO")
}

// NewInventoryHandler — HTTP-интерфейс склада:
//   - GET /healthz → 200;
//   - POST /rpc — JSON-RPC 2.0 (одиночные запросы), метод MethodReserve с
//     params = ReserveRequest; ошибки: ErrOutOfStock → CodeOutOfStock,
//     ErrUnknownSKU → CodeUnknownSKU, ErrInvalid и плохие params → CodeInvalidParams,
//     неизвестный метод → CodeMethodNotFound, плохой JSON → CodeParseError,
//     прочее → CodeInternal. Ответы — HTTP 200.
func NewInventoryHandler(inv *Inventory) http.Handler {
	// TODO: реализуйте
	return http.NotFoundHandler()
}

// ============================================================ client

// InventoryClient — устойчивый клиент inventory. Реализует InventoryAPI.
type InventoryClient struct {
	// TODO: поля (URL, конфиг с подставленными умолчаниями, breaker, счётчик id)
}

// NewInventoryClient создаёт клиента к inventory по базовому URL (без /rpc).
func NewInventoryClient(baseURL string, cfg ClientConfig) *InventoryClient {
	// TODO: реализуйте
	return &InventoryClient{}
}

// Reserve вызывает MethodReserve.
//
//   - каждая попытка — с таймаутом cfg.AttemptTimeout (context.WithTimeout от ctx);
//   - повторяются: транспортные ошибки, таймаут попытки, HTTP != 200, CodeInternal
//     и прочие неизвестные коды; пауза BaseDelay, 2·BaseDelay, ...;
//   - бизнес-ошибки (CodeOutOfStock/CodeUnknownSKU/CodeInvalidParams) НЕ повторяются,
//     НЕ считаются неудачей breaker и возвращаются как ErrOutOfStock/ErrUnknownSKU/ErrInvalid;
//   - каждая попытка проходит через circuit breaker (FailureThreshold неудач подряд →
//     open на OpenTimeout по часам cfg.Now → одна пробная попытка);
//     breaker разомкнут → сразу ошибка Is(ErrUnavailable), без сетевого запроса;
//   - попытки кончились → ошибка Is(ErrUnavailable);
//   - ctx вызывающего истёк/отменён → ошибка Is(ctx.Err()), паузы прерываются.
func (c *InventoryClient) Reserve(ctx context.Context, req ReserveRequest) (ReserveResult, error) {
	// TODO: реализуйте
	panic("TODO")
}

// ============================================================ orders

// NewOrdersHandler — HTTP API сервиса заказов.
//
//	POST /orders {"sku": "A", "qty": 2}
//	  201 + Order{Status: "reserved"} + заголовок Location: /orders/{id}
//	  400 — плохой JSON / пустой sku / qty <= 0 / ErrInvalid
//	  409 — ErrOutOfStock
//	  422 — ErrUnknownSKU
//	  503 — ErrUnavailable (+ заголовок Retry-After)
//	  504 — context.DeadlineExceeded (не ErrUnavailable)
//	  500 — прочее
//	GET /orders/{id} → 200 Order | 404
//
// ID заказа генерируется сервисом и передаётся в inventory как OrderID
// (ключ идемпотентности). Ответы с ошибкой — JSON {"error": "..."}.
func NewOrdersHandler(api InventoryAPI) http.Handler {
	// TODO: реализуйте
	return http.NotFoundHandler()
}
