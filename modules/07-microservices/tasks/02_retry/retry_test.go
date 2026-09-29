package retry

import (
	"context"
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

var errTemp = errors.New("temporary")

// recordSleep запоминает запрошенные паузы и не спит.
type recordSleep struct {
	mu     sync.Mutex
	delays []time.Duration
}

func (r *recordSleep) Sleep(ctx context.Context, d time.Duration) error {
	r.mu.Lock()
	r.delays = append(r.delays, d)
	r.mu.Unlock()
	return ctx.Err()
}

func TestBackoff(t *testing.T) {
	p := Policy{BaseDelay: 100 * time.Millisecond, MaxDelay: time.Second}
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 0},
		{1, 100 * time.Millisecond},
		{2, 200 * time.Millisecond},
		{3, 400 * time.Millisecond},
		{4, 800 * time.Millisecond},
		{5, time.Second},
		{100, time.Second}, // без переполнения
	}
	for _, c := range cases {
		if got := Backoff(p, c.attempt); got != c.want {
			t.Errorf("Backoff(attempt=%d) = %v, ожидалось %v", c.attempt, got, c.want)
		}
	}
	p3 := Policy{BaseDelay: time.Millisecond, Multiplier: 3}
	if got := Backoff(p3, 3); got != 9*time.Millisecond {
		t.Errorf("Backoff(mult=3, attempt=3) = %v, ожидалось 9ms", got)
	}
	huge := Backoff(Policy{BaseDelay: time.Hour}, 200)
	if huge <= 0 {
		t.Errorf("Backoff без MaxDelay переполнился: %v", huge)
	}
}

func TestDoSucceedsAfterRetries(t *testing.T) {
	rs := &recordSleep{}
	p := Policy{MaxAttempts: 5, BaseDelay: 10 * time.Millisecond, Sleep: rs.Sleep}
	var attempts []int
	err := Do(context.Background(), p, func(ctx context.Context, attempt int) error {
		attempts = append(attempts, attempt)
		if attempt < 3 {
			return errTemp
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Do вернул %v, ожидался nil", err)
	}
	if fmt.Sprint(attempts) != "[1 2 3]" {
		t.Errorf("номера попыток %v, ожидалось [1 2 3]", attempts)
	}
	if fmt.Sprint(rs.delays) != "[10ms 20ms]" {
		t.Errorf("паузы %v, ожидалось [10ms 20ms]", rs.delays)
	}
}

func TestDoExhausted(t *testing.T) {
	rs := &recordSleep{}
	calls := 0
	err := Do(context.Background(), Policy{MaxAttempts: 3, Sleep: rs.Sleep}, func(context.Context, int) error {
		calls++
		return fmt.Errorf("call %d: %w", calls, errTemp)
	})
	if calls != 3 {
		t.Errorf("fn вызвана %d раз, ожидалось 3", calls)
	}
	if !errors.Is(err, ErrExhausted) || !errors.Is(err, errTemp) {
		t.Errorf("ошибка %v должна быть Is(ErrExhausted) и Is(последняя ошибка)", err)
	}
	if !strings.Contains(err.Error(), "call 3") {
		t.Errorf("ошибка %q должна содержать ПОСЛЕДНЮЮ ошибку", err)
	}
}

func TestDoZeroAttemptsMeansOne(t *testing.T) {
	calls := 0
	_ = Do(context.Background(), Policy{}, func(context.Context, int) error { calls++; return errTemp })
	if calls != 1 {
		t.Errorf("MaxAttempts=0: fn вызвана %d раз, ожидалось 1", calls)
	}
}

func TestDoPermanent(t *testing.T) {
	errBad := errors.New("bad request")
	calls := 0
	err := Do(context.Background(), Policy{MaxAttempts: 5, Sleep: (&recordSleep{}).Sleep}, func(context.Context, int) error {
		calls++
		return Permanent(fmt.Errorf("wrap: %w", errBad))
	})
	if calls != 1 {
		t.Errorf("Permanent: fn вызвана %d раз, ожидалось 1", calls)
	}
	if !errors.Is(err, errBad) || errors.Is(err, ErrExhausted) {
		t.Errorf("Permanent: ошибка %v, ожидалась исходная без ErrExhausted", err)
	}
	if err.Error() != "wrap: bad request" {
		t.Errorf("Permanent: текст ошибки %q, ожидалось %q", err, "wrap: bad request")
	}
	if Permanent(nil) != nil {
		t.Error("Permanent(nil) должен быть nil")
	}
}

func TestDoNotRetryable(t *testing.T) {
	calls := 0
	p := Policy{MaxAttempts: 5, Retryable: func(err error) bool { return !errors.Is(err, errTemp) }, Sleep: (&recordSleep{}).Sleep}
	err := Do(context.Background(), p, func(context.Context, int) error { calls++; return errTemp })
	if calls != 1 || !errors.Is(err, errTemp) {
		t.Errorf("неповторяемая ошибка: calls=%d err=%v", calls, err)
	}
	// по умолчанию ошибки контекста не повторяются
	calls = 0
	err = Do(context.Background(), Policy{MaxAttempts: 5, Sleep: (&recordSleep{}).Sleep}, func(context.Context, int) error {
		calls++
		return context.DeadlineExceeded
	})
	if calls != 1 || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("DeadlineExceeded по умолчанию не повторяется: calls=%d err=%v", calls, err)
	}
}

func TestDoContextCanceledBeforeStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := Do(ctx, Policy{MaxAttempts: 3}, func(context.Context, int) error { called = true; return nil })
	if called || !errors.Is(err, context.Canceled) {
		t.Errorf("отменённый ctx: called=%v err=%v", called, err)
	}
}

func TestDoContextCanceledDuringSleep(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := Do(ctx, Policy{MaxAttempts: 10, BaseDelay: 10 * time.Second}, func(context.Context, int) error { return errTemp })
	if el := time.Since(start); el > 2*time.Second {
		t.Fatalf("Do ждал %v — пауза должна прерываться отменой контекста", el)
	}
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, errTemp) {
		t.Errorf("ошибка %v должна быть Is(DeadlineExceeded) и Is(последняя ошибка)", err)
	}
}

type retryAfterErr struct{ d time.Duration }

func (e retryAfterErr) Error() string             { return "slow down" }
func (e retryAfterErr) RetryAfter() time.Duration { return e.d }

func TestDoRetryAfterAndJitter(t *testing.T) {
	rs := &recordSleep{}
	p := Policy{
		MaxAttempts: 3, BaseDelay: time.Second, Sleep: rs.Sleep,
		Jitter: func(d time.Duration) time.Duration { return d / 2 },
	}
	_ = Do(context.Background(), p, func(_ context.Context, attempt int) error {
		if attempt == 1 {
			return fmt.Errorf("429: %w", retryAfterErr{7 * time.Second})
		}
		return errTemp
	})
	if fmt.Sprint(rs.delays) != "[7s 1s]" {
		t.Errorf("паузы %v, ожидалось [7s 1s] (Retry-After, затем 2s с джиттером /2)", rs.delays)
	}
}

func TestNewKey(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		k := NewKey()
		if len(k) != 32 || strings.Trim(k, "0123456789abcdef") != "" {
			t.Fatalf("NewKey() = %q, ожидалось 32 hex-символа", k)
		}
		if seen[k] {
			t.Fatalf("NewKey() повторился: %q", k)
		}
		seen[k] = true
	}
}

// orderHandler — «создание заказа» с побочным эффектом.
func orderHandler(counter *atomic.Int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n := counter.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		fmt.Fprintf(w, `{"order":%d,"body":%q}`, n, body)
	})
}

func do(t *testing.T, h http.Handler, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(body))
	if key != "" {
		req.Header.Set(HeaderIdempotencyKey, key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestIdempotencyReplay(t *testing.T) {
	var counter atomic.Int64
	h := Idempotency(orderHandler(&counter))

	r1 := do(t, h, "k1", `{"sku":"A"}`)
	r2 := do(t, h, "k1", `{"sku":"A"}`)
	if counter.Load() != 1 {
		t.Fatalf("обработчик выполнен %d раз, ожидалось 1", counter.Load())
	}
	if r1.Code != http.StatusCreated || r2.Code != http.StatusCreated {
		t.Fatalf("коды %d и %d, ожидалось 201 и 201", r1.Code, r2.Code)
	}
	if r1.Body.String() != r2.Body.String() {
		t.Errorf("повтор вернул %q, ожидалось %q", r2.Body, r1.Body)
	}
	if !strings.Contains(r1.Body.String(), `\"sku\"`) {
		t.Errorf("обработчик не увидел тело запроса: %q (тело нужно вернуть в r.Body после чтения)", r1.Body)
	}
	if r2.Header().Get("Content-Type") != "application/json" {
		t.Errorf("повтор должен восстанавливать заголовки, Content-Type = %q", r2.Header().Get("Content-Type"))
	}
	if r2.Header().Get(HeaderReplayed) != "true" || r1.Header().Get(HeaderReplayed) != "" {
		t.Errorf("заголовок %s: первый %q, повтор %q", HeaderReplayed, r1.Header().Get(HeaderReplayed), r2.Header().Get(HeaderReplayed))
	}

	// без ключа — каждый раз заново
	do(t, h, "", `{}`)
	do(t, h, "", `{}`)
	if counter.Load() != 3 {
		t.Errorf("без ключа обработчик должен вызываться всегда, счётчик = %d", counter.Load())
	}
	// другой ключ — новый заказ
	do(t, h, "k2", `{"sku":"A"}`)
	if counter.Load() != 4 {
		t.Errorf("новый ключ должен выполнить обработчик, счётчик = %d", counter.Load())
	}
}

func TestIdempotencyMismatch(t *testing.T) {
	var counter atomic.Int64
	h := Idempotency(orderHandler(&counter))
	do(t, h, "k", `{"sku":"A"}`)
	r := do(t, h, "k", `{"sku":"B"}`)
	if r.Code != http.StatusUnprocessableEntity {
		t.Errorf("тот же ключ с другим телом: код %d, ожидалось 422", r.Code)
	}
	if counter.Load() != 1 {
		t.Errorf("обработчик выполнен %d раз, ожидалось 1", counter.Load())
	}
}

func TestIdempotencyDoesNotStore5xx(t *testing.T) {
	var calls atomic.Int64
	h := Idempotency(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "db down", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	r1 := do(t, h, "k", "x")
	r2 := do(t, h, "k", "x")
	if r1.Code != 503 || r2.Code != 201 || calls.Load() != 2 {
		t.Errorf("5xx не должен запоминаться: коды %d, %d, вызовов %d", r1.Code, r2.Code, calls.Load())
	}
}

func TestIdempotencyInProgress(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	h := Idempotency(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusCreated)
	}))
	done := make(chan int)
	go func() { done <- do(t, h, "k", "x").Code }()
	<-started
	if r := do(t, h, "k", "x"); r.Code != http.StatusConflict {
		t.Errorf("параллельный дубль: код %d, ожидалось 409", r.Code)
	}
	close(release)
	if c := <-done; c != http.StatusCreated {
		t.Errorf("первый запрос: код %d, ожидалось 201", c)
	}
}

func TestIdempotencyConcurrentDuplicates(t *testing.T) {
	var counter atomic.Int64
	h := Idempotency(orderHandler(&counter))
	var wg sync.WaitGroup
	codes := make([]int, 32)
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			codes[i] = do(t, h, "same", `{"sku":"A"}`).Code
		}(i)
	}
	wg.Wait()
	if counter.Load() != 1 {
		t.Errorf("параллельные дубли выполнили обработчик %d раз, ожидалось 1", counter.Load())
	}
	for i, c := range codes {
		if c != http.StatusCreated && c != http.StatusConflict {
			t.Errorf("дубль %d: код %d, ожидалось 201 (повтор) или 409 (ещё выполняется)", i, c)
		}
	}
}

// lossyTransport выполняет запрос, но «теряет» первый ответ — как при обрыве сети.
type lossyTransport struct {
	lost atomic.Bool
}

func (l *lossyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultTransport.RoundTrip(req)
	if err == nil && l.lost.CompareAndSwap(false, true) {
		resp.Body.Close()
		return nil, errors.New("connection reset by peer")
	}
	return resp, err
}

func TestRetryWithIdempotencyEndToEnd(t *testing.T) {
	var counter atomic.Int64
	srv := httptest.NewServer(Idempotency(orderHandler(&counter)))
	defer srv.Close()
	client := &http.Client{Transport: &lossyTransport{}}

	key := NewKey() // ОДИН ключ на все попытки одной операции
	var status int
	var replayed string
	err := Do(context.Background(), Policy{MaxAttempts: 3, BaseDelay: time.Millisecond}, func(ctx context.Context, _ int) error {
		// тело нужно создавать заново на каждую попытку
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/orders", strings.NewReader(`{"sku":"A"}`))
		req.Header.Set(HeaderIdempotencyKey, key)
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		status, replayed = resp.StatusCode, resp.Header.Get(HeaderReplayed)
		return nil
	})
	if err != nil {
		t.Fatalf("Do вернул %v", err)
	}
	if counter.Load() != 1 {
		t.Errorf("заказ создан %d раз, ожидалось ровно 1", counter.Load())
	}
	if status != http.StatusCreated || replayed != "true" {
		t.Errorf("второй ответ: код %d, %s=%q; ожидался повтор сохранённого 201", status, HeaderReplayed, replayed)
	}
}
