package orders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// ------------------------------------------------------------ Inventory

func TestInventoryReserve(t *testing.T) {
	initial := map[string]int{"A": 5, "B": 0}
	inv := NewInventory(initial)
	initial["A"] = 100 // склад должен хранить копию
	ctx := context.Background()

	res, err := inv.Reserve(ctx, ReserveRequest{OrderID: "o1", SKU: "A", Qty: 2})
	if err != nil || res.Remaining != 3 || inv.Stock("A") != 3 {
		t.Fatalf("Reserve = %+v, %v; остаток %d; ожидалось Remaining=3", res, err, inv.Stock("A"))
	}
	// идемпотентность
	res, err = inv.Reserve(ctx, ReserveRequest{OrderID: "o1", SKU: "A", Qty: 2})
	if err != nil || res.Remaining != 3 || inv.Stock("A") != 3 {
		t.Errorf("повтор o1 = %+v, %v; остаток %d; ожидалось тот же результат без списания", res, err, inv.Stock("A"))
	}
	if _, err := inv.Reserve(ctx, ReserveRequest{OrderID: "o1", SKU: "A", Qty: 1}); !errors.Is(err, ErrInvalid) {
		t.Errorf("o1 с другими параметрами: %v, ожидался ErrInvalid", err)
	}
	cases := []struct {
		req  ReserveRequest
		want error
	}{
		{ReserveRequest{OrderID: "o2", SKU: "A", Qty: 4}, ErrOutOfStock},
		{ReserveRequest{OrderID: "o3", SKU: "B", Qty: 1}, ErrOutOfStock},
		{ReserveRequest{OrderID: "o4", SKU: "Z", Qty: 1}, ErrUnknownSKU},
		{ReserveRequest{OrderID: "", SKU: "A", Qty: 1}, ErrInvalid},
		{ReserveRequest{OrderID: "o5", SKU: "", Qty: 1}, ErrInvalid},
		{ReserveRequest{OrderID: "o6", SKU: "A", Qty: 0}, ErrInvalid},
		{ReserveRequest{OrderID: "o7", SKU: "A", Qty: -1}, ErrInvalid},
	}
	for _, c := range cases {
		if _, err := inv.Reserve(ctx, c.req); !errors.Is(err, c.want) {
			t.Errorf("Reserve(%+v) = %v, ожидалось %v", c.req, err, c.want)
		}
	}
	if inv.Stock("A") != 3 {
		t.Errorf("неудачные резервы изменили остаток: %d", inv.Stock("A"))
	}
	// после ErrOutOfStock тот же OrderID можно использовать, когда товара хватает
	if _, err := inv.Reserve(ctx, ReserveRequest{OrderID: "o2", SKU: "A", Qty: 3}); err != nil {
		t.Errorf("o2 после отказа: %v", err)
	}
}

func TestInventoryConcurrent(t *testing.T) {
	inv := NewInventory(map[string]int{"A": 50})
	var ok atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := inv.Reserve(context.Background(), ReserveRequest{OrderID: fmt.Sprint(i), SKU: "A", Qty: 1}); err == nil {
				ok.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if ok.Load() != 50 || inv.Stock("A") != 0 {
		t.Errorf("успешных резервов %d, остаток %d; ожидалось 50 и 0", ok.Load(), inv.Stock("A"))
	}
}

func rpcPost(t *testing.T, url, body string) map[string]any {
	t.Helper()
	resp, err := http.Post(url+"/rpc", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /rpc: HTTP %d, ожидалось 200", resp.StatusCode)
	}
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("ответ не JSON: %v", err)
	}
	return m
}

func errCode(m map[string]any) int {
	e, _ := m["error"].(map[string]any)
	c, _ := e["code"].(float64)
	return int(c)
}

func TestInventoryHandler(t *testing.T) {
	srv := httptest.NewServer(NewInventoryHandler(NewInventory(map[string]int{"A": 1})))
	defer srv.Close()

	m := rpcPost(t, srv.URL, `{"jsonrpc":"2.0","method":"Inventory.Reserve","params":{"order_id":"x","sku":"A","qty":1},"id":1}`)
	if res, _ := m["result"].(map[string]any); res == nil || res["remaining"] != float64(0) || m["id"] != float64(1) {
		t.Errorf("успешный резерв: %v", m)
	}
	cases := []struct {
		body string
		code int
	}{
		{`{"jsonrpc":"2.0","method":"Inventory.Reserve","params":{"order_id":"y","sku":"A","qty":1},"id":2}`, CodeOutOfStock},
		{`{"jsonrpc":"2.0","method":"Inventory.Reserve","params":{"order_id":"y","sku":"Q","qty":1},"id":3}`, CodeUnknownSKU},
		{`{"jsonrpc":"2.0","method":"Inventory.Reserve","params":{"order_id":"y","sku":"A","qty":0},"id":4}`, CodeInvalidParams},
		{`{"jsonrpc":"2.0","method":"Inventory.Reserve","params":[1,2],"id":5}`, CodeInvalidParams},
		{`{"jsonrpc":"2.0","method":"Inventory.Drop","id":6}`, CodeMethodNotFound},
		{`{"jsonrpc":`, CodeParseError},
	}
	for _, c := range cases {
		if got := errCode(rpcPost(t, srv.URL, c.body)); got != c.code {
			t.Errorf("%s → код %d, ожидался %d", c.body, got, c.code)
		}
	}
	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil || resp.StatusCode != 200 {
		t.Errorf("GET /healthz: %v %v", resp, err)
	}
	if resp != nil {
		resp.Body.Close()
	}
}

// ------------------------------------------------------------ Client

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// chaos — middleware перед inventory, считающее запросы и умеющее портить ответы.
type chaos struct {
	next  http.Handler
	hits  atomic.Int64
	fault func(n int64, w http.ResponseWriter, r *http.Request) bool // true — ответ уже испорчен
}

func (c *chaos) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	n := c.hits.Add(1)
	if r.URL.Path == "/rpc" && c.fault != nil && c.fault(n, w, r) {
		return
	}
	c.next.ServeHTTP(w, r)
}

func setup(t *testing.T, stock map[string]int, fault func(int64, http.ResponseWriter, *http.Request) bool, cfg ClientConfig) (*Inventory, *chaos, *InventoryClient) {
	t.Helper()
	inv := NewInventory(stock)
	ch := &chaos{next: NewInventoryHandler(inv), fault: fault}
	srv := httptest.NewServer(ch)
	t.Cleanup(srv.Close)
	if cfg.BaseDelay == 0 {
		cfg.BaseDelay = time.Millisecond
	}
	cfg.HTTPClient = srv.Client()
	return inv, ch, NewInventoryClient(srv.URL, cfg)
}

func TestClientHappyAndBusinessErrors(t *testing.T) {
	inv, ch, c := setup(t, map[string]int{"A": 3}, nil, ClientConfig{FailureThreshold: 1})
	ctx := context.Background()
	res, err := c.Reserve(ctx, ReserveRequest{OrderID: "1", SKU: "A", Qty: 2})
	if err != nil || res.Remaining != 1 {
		t.Fatalf("Reserve = %+v, %v", res, err)
	}
	for i := 0; i < 5; i++ {
		if _, err := c.Reserve(ctx, ReserveRequest{OrderID: fmt.Sprint("x", i), SKU: "A", Qty: 5}); !errors.Is(err, ErrOutOfStock) {
			t.Fatalf("нехватка: %v, ожидался ErrOutOfStock", err)
		}
	}
	if _, err := c.Reserve(ctx, ReserveRequest{OrderID: "u", SKU: "nope", Qty: 1}); !errors.Is(err, ErrUnknownSKU) {
		t.Errorf("неизвестный SKU: %v, ожидался ErrUnknownSKU", err)
	}
	if _, err := c.Reserve(ctx, ReserveRequest{OrderID: "i", SKU: "A", Qty: 0}); !errors.Is(err, ErrInvalid) {
		t.Errorf("qty=0: %v, ожидался ErrInvalid", err)
	}
	if got := ch.hits.Load(); got != 8 {
		t.Errorf("запросов к inventory %d, ожидалось 8: бизнес-ошибки не ретраятся", got)
	}
	// бизнес-ошибки не размыкают breaker даже при FailureThreshold=1
	if _, err := c.Reserve(ctx, ReserveRequest{OrderID: "2", SKU: "A", Qty: 1}); err != nil {
		t.Errorf("после бизнес-ошибок breaker не должен быть разомкнут: %v", err)
	}
	if inv.Stock("A") != 0 {
		t.Errorf("остаток %d, ожидался 0", inv.Stock("A"))
	}
}

func TestClientRetriesTransientFailures(t *testing.T) {
	fault := func(n int64, w http.ResponseWriter, r *http.Request) bool {
		if n <= 2 {
			http.Error(w, "overloaded", http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	inv, ch, c := setup(t, map[string]int{"A": 3}, fault, ClientConfig{MaxAttempts: 3})
	if _, err := c.Reserve(context.Background(), ReserveRequest{OrderID: "1", SKU: "A", Qty: 1}); err != nil {
		t.Fatalf("после двух 503 третья попытка должна пройти: %v", err)
	}
	if ch.hits.Load() != 3 || inv.Stock("A") != 2 {
		t.Errorf("попыток %d, остаток %d; ожидалось 3 и 2", ch.hits.Load(), inv.Stock("A"))
	}
}

func TestClientInternalRPCErrorRetried(t *testing.T) {
	fault := func(n int64, w http.ResponseWriter, r *http.Request) bool {
		if n == 1 {
			io.WriteString(w, `{"jsonrpc":"2.0","error":{"code":-32603,"message":"Internal error"},"id":1}`)
			return true
		}
		return false
	}
	_, ch, c := setup(t, map[string]int{"A": 3}, fault, ClientConfig{})
	if _, err := c.Reserve(context.Background(), ReserveRequest{OrderID: "1", SKU: "A", Qty: 1}); err != nil || ch.hits.Load() != 2 {
		t.Errorf("-32603 должна ретраиться: err=%v, попыток %d", err, ch.hits.Load())
	}
}

func TestClientLostResponseIsIdempotent(t *testing.T) {
	var inv *Inventory
	var next http.Handler
	fault := func(n int64, w http.ResponseWriter, r *http.Request) bool {
		if n == 1 {
			// запрос ВЫПОЛНЕН, но ответ «потерялся»
			next.ServeHTTP(httptest.NewRecorder(), r)
			http.Error(w, "gateway error", http.StatusBadGateway)
			return true
		}
		return false
	}
	inv, ch, c := setup(t, map[string]int{"A": 3}, fault, ClientConfig{})
	next = ch.next
	res, err := c.Reserve(context.Background(), ReserveRequest{OrderID: "1", SKU: "A", Qty: 2})
	if err != nil || res.Remaining != 1 {
		t.Fatalf("Reserve = %+v, %v", res, err)
	}
	if inv.Stock("A") != 1 {
		t.Errorf("остаток %d, ожидался 1: повтор с тем же OrderID не должен списывать дважды", inv.Stock("A"))
	}
}

func TestClientAttemptTimeout(t *testing.T) {
	var next http.Handler
	slowDone := make(chan struct{})
	fault := func(n int64, w http.ResponseWriter, r *http.Request) bool {
		if n == 1 {
			defer close(slowDone)
			time.Sleep(time.Second) // первая попытка «зависла»...
			next.ServeHTTP(w, r)    // ...но всё же выполнилась
			return true
		}
		return false
	}
	inv, ch, c := setup(t, map[string]int{"A": 3}, fault, ClientConfig{AttemptTimeout: 50 * time.Millisecond})
	next = ch.next
	start := time.Now()
	if _, err := c.Reserve(context.Background(), ReserveRequest{OrderID: "1", SKU: "A", Qty: 1}); err != nil {
		t.Fatalf("вторая попытка должна пройти: %v", err)
	}
	if el := time.Since(start); el > 700*time.Millisecond {
		t.Errorf("Reserve занял %v: таймаут попытки 50ms не сработал", el)
	}
	<-slowDone // даём зависшей попытке завершиться
	if inv.Stock("A") != 2 {
		t.Errorf("остаток %d, ожидался 2: зависшая и повторная попытки не должны списать дважды", inv.Stock("A"))
	}
}

func TestClientCallerDeadline(t *testing.T) {
	block := make(chan struct{})
	defer close(block)
	fault := func(n int64, w http.ResponseWriter, r *http.Request) bool {
		select {
		case <-block:
		case <-r.Context().Done():
		}
		return true
	}
	_, _, c := setup(t, map[string]int{"A": 3}, fault, ClientConfig{AttemptTimeout: 10 * time.Second, MaxAttempts: 5})
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := c.Reserve(ctx, ReserveRequest{OrderID: "1", SKU: "A", Qty: 1})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("истёк ctx вызывающего: %v, ожидалось Is(context.DeadlineExceeded)", err)
	}
	if el := time.Since(start); el > time.Second {
		t.Errorf("Reserve занял %v при дедлайне 80ms", el)
	}
}

func TestClientCircuitBreaker(t *testing.T) {
	var broken atomic.Bool
	broken.Store(true)
	fault := func(n int64, w http.ResponseWriter, r *http.Request) bool {
		if broken.Load() {
			http.Error(w, "down", http.StatusInternalServerError)
			return true
		}
		return false
	}
	clk := &fakeClock{now: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)}
	inv, ch, c := setup(t, map[string]int{"A": 3}, fault, ClientConfig{
		MaxAttempts: 2, FailureThreshold: 3, OpenTimeout: time.Minute, Now: clk.Now,
	})
	ctx := context.Background()
	req := ReserveRequest{OrderID: "1", SKU: "A", Qty: 1}

	if _, err := c.Reserve(ctx, req); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("исчерпаны попытки: %v, ожидался ErrUnavailable", err)
	}
	if ch.hits.Load() != 2 {
		t.Fatalf("попыток %d, ожидалось 2", ch.hits.Load())
	}
	// 3-я неудача размыкает цепь, 4-я попытка уже не уходит в сеть
	if _, err := c.Reserve(ctx, req); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("размыкание: %v, ожидался ErrUnavailable", err)
	}
	if ch.hits.Load() != 3 {
		t.Fatalf("запросов %d, ожидалось 3: после размыкания запросы не отправляются", ch.hits.Load())
	}
	for i := 0; i < 5; i++ {
		if _, err := c.Reserve(ctx, req); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("open: %v, ожидался ErrUnavailable", err)
		}
	}
	if ch.hits.Load() != 3 {
		t.Fatalf("в open ушло %d запросов, ожидалось 0 новых", ch.hits.Load()-3)
	}

	broken.Store(false)
	clk.Advance(time.Minute)
	if _, err := c.Reserve(ctx, req); err != nil {
		t.Fatalf("после OpenTimeout пробный запрос должен пройти: %v", err)
	}
	if _, err := c.Reserve(ctx, ReserveRequest{OrderID: "2", SKU: "A", Qty: 1}); err != nil {
		t.Fatalf("после успешной пробы цепь замкнута: %v", err)
	}
	if inv.Stock("A") != 1 {
		t.Errorf("остаток %d, ожидался 1", inv.Stock("A"))
	}
}

func TestClientServerDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // никого нет дома
	c := NewInventoryClient(url, ClientConfig{BaseDelay: time.Millisecond})
	if _, err := c.Reserve(context.Background(), ReserveRequest{OrderID: "1", SKU: "A", Qty: 1}); !errors.Is(err, ErrUnavailable) {
		t.Errorf("сервер недоступен: %v, ожидался ErrUnavailable", err)
	}
}

// ------------------------------------------------------------ Orders

type fakeAPI struct {
	err  error
	last ReserveRequest
	mu   sync.Mutex
}

func (f *fakeAPI) Reserve(ctx context.Context, req ReserveRequest) (ReserveResult, error) {
	f.mu.Lock()
	f.last = req
	f.mu.Unlock()
	return ReserveResult{Remaining: 1}, f.err
}

func post(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(body)))
	return rec
}

func TestOrdersErrorMapping(t *testing.T) {
	cases := []struct {
		name string
		err  error
		body string
		want int
	}{
		{"ok", nil, `{"sku":"A","qty":1}`, 201},
		{"плохой JSON", nil, `{"sku":`, 400},
		{"qty=0", nil, `{"sku":"A","qty":0}`, 400},
		{"пустой sku", nil, `{"qty":1}`, 400},
		{"out of stock", fmt.Errorf("wrap: %w", ErrOutOfStock), `{"sku":"A","qty":1}`, 409},
		{"unknown sku", ErrUnknownSKU, `{"sku":"A","qty":1}`, 422},
		{"invalid", ErrInvalid, `{"sku":"A","qty":1}`, 400},
		{"unavailable", fmt.Errorf("%w: breaker open", ErrUnavailable), `{"sku":"A","qty":1}`, 503},
		{"unavailable after timeouts", fmt.Errorf("%w: %w", ErrUnavailable, context.DeadlineExceeded), `{"sku":"A","qty":1}`, 503},
		{"deadline", fmt.Errorf("inventory: %w", context.DeadlineExceeded), `{"sku":"A","qty":1}`, 504},
		{"other", errors.New("???"), `{"sku":"A","qty":1}`, 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := post(t, NewOrdersHandler(&fakeAPI{err: c.err}), c.body)
			if rec.Code != c.want {
				t.Fatalf("код %d, ожидался %d (тело %s)", rec.Code, c.want, rec.Body)
			}
			if c.want >= 400 {
				var m map[string]string
				if json.Unmarshal(rec.Body.Bytes(), &m) != nil || m["error"] == "" {
					t.Errorf("тело ошибки %q, ожидался JSON {\"error\": ...}", rec.Body)
				}
			}
			if c.want == 503 && rec.Header().Get("Retry-After") == "" {
				t.Error("503 без заголовка Retry-After")
			}
		})
	}
}

func TestOrdersCreateAndGet(t *testing.T) {
	api := &fakeAPI{}
	h := NewOrdersHandler(api)
	rec := post(t, h, `{"sku":"A","qty":2}`)
	if rec.Code != 201 {
		t.Fatalf("POST /orders: %d", rec.Code)
	}
	var o Order
	if err := json.Unmarshal(rec.Body.Bytes(), &o); err != nil || o.ID == "" || o.SKU != "A" || o.Qty != 2 || o.Status != "reserved" {
		t.Fatalf("созданный заказ %s", rec.Body)
	}
	if rec.Header().Get("Location") != "/orders/"+o.ID {
		t.Errorf("Location = %q", rec.Header().Get("Location"))
	}
	if api.last.OrderID != o.ID {
		t.Errorf("в inventory ушёл OrderID %q, ожидался ID заказа %q", api.last.OrderID, o.ID)
	}
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/orders/"+o.ID, nil))
	if get.Code != 200 || !strings.Contains(get.Body.String(), o.ID) {
		t.Errorf("GET /orders/%s: %d %s", o.ID, get.Code, get.Body)
	}
	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/orders/nope", nil))
	if missing.Code != 404 {
		t.Errorf("GET несуществующего заказа: %d", missing.Code)
	}
	// неуспешные заказы не сохраняются, ID уникальны
	rec2 := post(t, h, `{"sku":"A","qty":1}`)
	var o2 Order
	_ = json.Unmarshal(rec2.Body.Bytes(), &o2)
	if o2.ID == o.ID {
		t.Error("ID заказов должны быть уникальны")
	}
}

func TestEndToEnd(t *testing.T) {
	inv := NewInventory(map[string]int{"book": 2})
	invSrv := httptest.NewServer(NewInventoryHandler(inv))
	defer invSrv.Close()
	client := NewInventoryClient(invSrv.URL, ClientConfig{HTTPClient: invSrv.Client()})
	ordSrv := httptest.NewServer(NewOrdersHandler(client))
	defer ordSrv.Close()

	codes := []int{}
	for i := 0; i < 3; i++ {
		resp, err := http.Post(ordSrv.URL+"/orders", "application/json", strings.NewReader(`{"sku":"book","qty":1}`))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		codes = append(codes, resp.StatusCode)
	}
	if fmt.Sprint(codes) != "[201 201 409]" {
		t.Errorf("коды %v, ожидалось [201 201 409]", codes)
	}
	resp, _ := http.Post(ordSrv.URL+"/orders", "application/json", strings.NewReader(`{"sku":"pen","qty":1}`))
	if resp != nil {
		resp.Body.Close()
		if resp.StatusCode != 422 {
			t.Errorf("неизвестный товар: %d, ожидалось 422", resp.StatusCode)
		}
	}
	if inv.Stock("book") != 0 {
		t.Errorf("остаток %d, ожидался 0", inv.Stock("book"))
	}
}
