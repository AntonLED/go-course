//go:build solution

// Package orders — два микросервиса (orders → inventory) в одном процессе,
// общающиеся по JSON-RPC 2.0 с таймаутами, ретраями и circuit breaker.
package orders

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ============================================================ inventory

// Inventory — in-memory склад с идемпотентным резервированием.
type Inventory struct {
	mu       sync.Mutex
	stock    map[string]int
	reserved map[string]reservation // OrderID → выполненный резерв
}

type reservation struct {
	req ReserveRequest
	res ReserveResult
}

// NewInventory создаёт склад с копией начальных остатков.
func NewInventory(stock map[string]int) *Inventory {
	s := make(map[string]int, len(stock))
	for k, v := range stock {
		s[k] = v
	}
	return &Inventory{stock: s, reserved: map[string]reservation{}}
}

// Stock возвращает текущий остаток SKU.
func (inv *Inventory) Stock(sku string) int {
	inv.mu.Lock()
	defer inv.mu.Unlock()
	return inv.stock[sku]
}

// Reserve списывает Qty единиц SKU. Повтор с тем же OrderID возвращает
// прежний результат без повторного списания.
func (inv *Inventory) Reserve(_ context.Context, req ReserveRequest) (ReserveResult, error) {
	if req.OrderID == "" || req.SKU == "" || req.Qty <= 0 {
		return ReserveResult{}, ErrInvalid
	}
	inv.mu.Lock()
	defer inv.mu.Unlock()
	if prev, ok := inv.reserved[req.OrderID]; ok {
		if prev.req != req {
			return ReserveResult{}, fmt.Errorf("%w: order %s reused with different params", ErrInvalid, req.OrderID)
		}
		return prev.res, nil
	}
	have, ok := inv.stock[req.SKU]
	if !ok {
		return ReserveResult{}, ErrUnknownSKU
	}
	if have < req.Qty {
		return ReserveResult{}, ErrOutOfStock
	}
	inv.stock[req.SKU] = have - req.Qty
	res := ReserveResult{Remaining: have - req.Qty}
	inv.reserved[req.OrderID] = reservation{req: req, res: res}
	return res, nil
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      json.RawMessage `json:"id,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
	ID      json.RawMessage `json:"id"`
}

// NewInventoryHandler — HTTP-интерфейс склада: POST /rpc (JSON-RPC 2.0) и GET /healthz.
func NewInventoryHandler(inv *Inventory) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /rpc", func(w http.ResponseWriter, r *http.Request) {
		reply := func(id json.RawMessage, result any, rerr *rpcError) {
			if id == nil {
				id = json.RawMessage("null")
			}
			resp := rpcResponse{JSONRPC: "2.0", ID: id, Error: rerr}
			if rerr == nil {
				resp.Result, _ = json.Marshal(result)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
		}
		var req rpcRequest
		if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
			reply(nil, nil, &rpcError{CodeParseError, "Parse error"})
			return
		}
		if req.JSONRPC != "2.0" || req.Method == "" {
			reply(req.ID, nil, &rpcError{CodeInvalidRequest, "Invalid Request"})
			return
		}
		if req.Method != MethodReserve {
			reply(req.ID, nil, &rpcError{CodeMethodNotFound, "Method not found"})
			return
		}
		var p ReserveRequest
		if err := json.Unmarshal(req.Params, &p); err != nil {
			reply(req.ID, nil, &rpcError{CodeInvalidParams, "Invalid params"})
			return
		}
		res, err := inv.Reserve(r.Context(), p)
		switch {
		case err == nil:
			reply(req.ID, res, nil)
		case errors.Is(err, ErrOutOfStock):
			reply(req.ID, nil, &rpcError{CodeOutOfStock, err.Error()})
		case errors.Is(err, ErrUnknownSKU):
			reply(req.ID, nil, &rpcError{CodeUnknownSKU, err.Error()})
		case errors.Is(err, ErrInvalid):
			reply(req.ID, nil, &rpcError{CodeInvalidParams, err.Error()})
		default:
			reply(req.ID, nil, &rpcError{CodeInternal, "Internal error"})
		}
	})
	return mux
}

// ============================================================ breaker

type breakerState int

const (
	closed breakerState = iota
	open
	halfOpen
)

var errBreakerOpen = errors.New("circuit breaker is open")

// breaker — минимальный circuit breaker (одна пробная попытка в half-open).
type breaker struct {
	threshold int
	timeout   time.Duration
	now       func() time.Time

	mu       sync.Mutex
	state    breakerState
	failures int
	openedAt time.Time
	probing  bool
}

func (b *breaker) allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.state == open && !b.now().Before(b.openedAt.Add(b.timeout)) {
		b.state, b.probing = halfOpen, false
	}
	switch b.state {
	case open:
		return errBreakerOpen
	case halfOpen:
		if b.probing {
			return errBreakerOpen
		}
		b.probing = true
	}
	return nil
}

// release снимает пробную попытку без вердикта (вызов отменён вызывающим).
func (b *breaker) release() {
	b.mu.Lock()
	b.probing = false
	b.mu.Unlock()
}

func (b *breaker) record(failed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case !failed:
		b.state, b.failures, b.probing = closed, 0, false
	case b.state == halfOpen:
		b.state, b.openedAt, b.probing = open, b.now(), false
	default:
		b.failures++
		if b.failures >= b.threshold {
			b.state, b.openedAt = open, b.now()
		}
	}
}

// ============================================================ client

// InventoryClient — устойчивый клиент inventory: таймаут на попытку,
// ретраи с экспоненциальной паузой, circuit breaker, маппинг ошибок.
type InventoryClient struct {
	url string
	cfg ClientConfig
	br  *breaker

	mu     sync.Mutex
	nextID int
}

// NewInventoryClient создаёт клиента к inventory по базовому URL.
func NewInventoryClient(baseURL string, cfg ClientConfig) *InventoryClient {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = http.DefaultClient
	}
	if cfg.AttemptTimeout <= 0 {
		cfg.AttemptTimeout = time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = 3
	}
	if cfg.BaseDelay <= 0 {
		cfg.BaseDelay = 10 * time.Millisecond
	}
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.OpenTimeout <= 0 {
		cfg.OpenTimeout = 5 * time.Second
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &InventoryClient{
		url: strings.TrimRight(baseURL, "/"),
		cfg: cfg,
		br:  &breaker{threshold: cfg.FailureThreshold, timeout: cfg.OpenTimeout, now: cfg.Now},
	}
}

// businessError — ошибка, которую нельзя ретраить и которая не говорит о
// неисправности зависимости.
type businessError struct{ err error }

func (b businessError) Error() string { return b.err.Error() }
func (b businessError) Unwrap() error { return b.err }

// Reserve вызывает Inventory.Reserve с ретраями и breaker.
func (c *InventoryClient) Reserve(ctx context.Context, req ReserveRequest) (ReserveResult, error) {
	var lastErr error
	delay := c.cfg.BaseDelay
	for attempt := 1; attempt <= c.cfg.MaxAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return ReserveResult{}, fmt.Errorf("inventory: %w", err)
		}
		if err := c.br.allow(); err != nil {
			return ReserveResult{}, fmt.Errorf("%w: %w", ErrUnavailable, err)
		}
		res, err := c.attempt(ctx, req)
		var be businessError
		switch {
		case err == nil:
			c.br.record(false)
			return res, nil
		case errors.As(err, &be):
			// Зависимость исправна — она просто сказала «нет».
			c.br.record(false)
			return ReserveResult{}, be.err
		case ctx.Err() != nil:
			// Отменили нас — зависимость не виновата, breaker не трогаем.
			c.br.release()
			return ReserveResult{}, fmt.Errorf("inventory: %w", ctx.Err())
		}
		c.br.record(true)
		lastErr = err
		if attempt == c.cfg.MaxAttempts {
			break
		}
		t := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			t.Stop()
			return ReserveResult{}, fmt.Errorf("inventory: %w (last error: %v)", ctx.Err(), lastErr)
		case <-t.C:
		}
		delay *= 2
	}
	return ReserveResult{}, fmt.Errorf("%w: %w", ErrUnavailable, lastErr)
}

// attempt — одна попытка с собственным таймаутом.
func (c *InventoryClient) attempt(ctx context.Context, req ReserveRequest) (ReserveResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.cfg.AttemptTimeout)
	defer cancel()

	c.mu.Lock()
	c.nextID++
	id := c.nextID
	c.mu.Unlock()
	params, _ := json.Marshal(req)
	body, _ := json.Marshal(rpcRequest{JSONRPC: "2.0", Method: MethodReserve, Params: params,
		ID: json.RawMessage(fmt.Sprint(id))})

	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/rpc", bytes.NewReader(body))
	if err != nil {
		return ReserveResult{}, businessError{fmt.Errorf("%w: %v", ErrInvalid, err)}
	}
	hreq.Header.Set("Content-Type", "application/json")
	resp, err := c.cfg.HTTPClient.Do(hreq)
	if err != nil {
		return ReserveResult{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ReserveResult{}, fmt.Errorf("inventory: HTTP %d", resp.StatusCode)
	}
	var rr rpcResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rr); err != nil {
		return ReserveResult{}, fmt.Errorf("inventory: bad response: %w", err)
	}
	if rr.Error != nil {
		switch rr.Error.Code {
		case CodeOutOfStock:
			return ReserveResult{}, businessError{ErrOutOfStock}
		case CodeUnknownSKU:
			return ReserveResult{}, businessError{ErrUnknownSKU}
		case CodeInvalidParams:
			return ReserveResult{}, businessError{fmt.Errorf("%w: %s", ErrInvalid, rr.Error.Message)}
		default:
			return ReserveResult{}, fmt.Errorf("inventory: rpc error %d: %s", rr.Error.Code, rr.Error.Message)
		}
	}
	var res ReserveResult
	if err := json.Unmarshal(rr.Result, &res); err != nil {
		return ReserveResult{}, fmt.Errorf("inventory: bad result: %w", err)
	}
	return res, nil
}

// ============================================================ orders

type ordersService struct {
	api InventoryAPI

	mu     sync.RWMutex
	orders map[string]Order
}

func newOrderID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "ord-" + hex.EncodeToString(b[:])
}

// NewOrdersHandler — HTTP API сервиса заказов: POST /orders, GET /orders/{id}.
func NewOrdersHandler(api InventoryAPI) http.Handler {
	s := &ordersService{api: api, orders: map[string]Order{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders", s.create)
	mux.HandleFunc("GET /orders/{id}", s.get)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *ordersService) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		SKU string `json:"sku"`
		Qty int    `json:"qty"`
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<16))
	if err := dec.Decode(&in); err != nil || in.SKU == "" || in.Qty <= 0 {
		writeErr(w, http.StatusBadRequest, "body must be {\"sku\": string, \"qty\": positive int}")
		return
	}
	id := newOrderID()
	// ID заказа — ключ идемпотентности для inventory: ретраи не спишут товар дважды.
	_, err := s.api.Reserve(r.Context(), ReserveRequest{OrderID: id, SKU: in.SKU, Qty: in.Qty})
	switch {
	case err == nil:
	case errors.Is(err, ErrOutOfStock):
		writeErr(w, http.StatusConflict, "out of stock")
		return
	case errors.Is(err, ErrUnknownSKU):
		writeErr(w, http.StatusUnprocessableEntity, "unknown sku")
		return
	case errors.Is(err, ErrInvalid):
		writeErr(w, http.StatusBadRequest, "invalid request")
		return
	case errors.Is(err, ErrUnavailable):
		w.Header().Set("Retry-After", "1")
		writeErr(w, http.StatusServiceUnavailable, "inventory unavailable")
		return
	case errors.Is(err, context.DeadlineExceeded):
		writeErr(w, http.StatusGatewayTimeout, "inventory timeout")
		return
	default:
		writeErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	o := Order{ID: id, SKU: in.SKU, Qty: in.Qty, Status: "reserved"}
	s.mu.Lock()
	s.orders[id] = o
	s.mu.Unlock()
	w.Header().Set("Location", "/orders/"+id)
	writeJSON(w, http.StatusCreated, o)
}

func (s *ordersService) get(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	o, ok := s.orders[r.PathValue("id")]
	s.mu.RUnlock()
	if !ok {
		writeErr(w, http.StatusNotFound, "order not found")
		return
	}
	writeJSON(w, http.StatusOK, o)
}
