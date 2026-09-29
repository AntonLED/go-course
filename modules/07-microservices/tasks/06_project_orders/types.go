package orders

import (
	"context"
	"errors"
	"net/http"
	"time"
)

// Бизнес- и инфраструктурные ошибки. Клиент inventory обязан возвращать
// ошибки, для которых errors.Is работает с этими значениями.
var (
	ErrOutOfStock  = errors.New("out of stock")
	ErrUnknownSKU  = errors.New("unknown sku")
	ErrInvalid     = errors.New("invalid request")
	ErrUnavailable = errors.New("inventory unavailable")
)

// Протокол inventory: JSON-RPC 2.0, POST /rpc.
const (
	MethodReserve = "Inventory.Reserve"

	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602 // ↔ ErrInvalid
	CodeInternal       = -32603
	CodeOutOfStock     = -32010 // ↔ ErrOutOfStock
	CodeUnknownSKU     = -32011 // ↔ ErrUnknownSKU
)

// ReserveRequest — параметры Inventory.Reserve. OrderID — ключ идемпотентности:
// повторный Reserve с тем же OrderID не списывает товар второй раз.
type ReserveRequest struct {
	OrderID string `json:"order_id"`
	SKU     string `json:"sku"`
	Qty     int    `json:"qty"`
}

// ReserveResult — результат Inventory.Reserve.
type ReserveResult struct {
	Remaining int `json:"remaining"` // остаток SKU после резерва
}

// InventoryAPI — то, что нужно сервису заказов от склада.
type InventoryAPI interface {
	Reserve(ctx context.Context, req ReserveRequest) (ReserveResult, error)
}

// Order — заказ в ответах сервиса orders.
type Order struct {
	ID     string `json:"id"`
	SKU    string `json:"sku"`
	Qty    int    `json:"qty"`
	Status string `json:"status"` // "reserved"
}

// ClientConfig — настройки устойчивого клиента inventory.
// Нулевые значения заменяются значениями по умолчанию.
type ClientConfig struct {
	HTTPClient       *http.Client     // по умолчанию http.DefaultClient
	AttemptTimeout   time.Duration    // таймаут одной попытки (по умолчанию 1s)
	MaxAttempts      int              // попыток на один вызов (по умолчанию 3)
	BaseDelay        time.Duration    // пауза перед 2-й попыткой, далее ×2 (по умолчанию 10ms)
	FailureThreshold int              // неудач подряд до размыкания breaker (по умолчанию 5)
	OpenTimeout      time.Duration    // сколько breaker разомкнут (по умолчанию 5s)
	Now              func() time.Time // часы breaker (по умолчанию time.Now)
}
