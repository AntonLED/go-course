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

// step — ответ фейкового транспорта на очередную попытку.
type step struct {
	code int
	hdr  map[string]string
	err  error
	body string
}

type trackBody struct {
	io.Reader
	closed *atomic.Int32
}

func (b trackBody) Close() error { b.closed.Add(1); return nil }

type fakeRT struct {
	mu     sync.Mutex
	steps  []step
	calls  int
	bodies []string
	closed atomic.Int32
	opened int
}

func (f *fakeRT) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if req.Body != nil {
		b, _ := io.ReadAll(req.Body)
		req.Body.Close()
		f.bodies = append(f.bodies, string(b))
	}
	i := f.calls
	f.calls++
	if i >= len(f.steps) {
		i = len(f.steps) - 1
	}
	s := f.steps[i]
	if s.err != nil {
		return nil, s.err
	}
	h := http.Header{}
	for k, v := range s.hdr {
		h.Set(k, v)
	}
	f.opened++
	return &http.Response{
		StatusCode: s.code,
		Header:     h,
		Body:       trackBody{strings.NewReader(s.body + strings.Repeat(".", 100)), &f.closed},
		Request:    req,
	}, nil
}

type sleeps struct {
	mu sync.Mutex
	d  []time.Duration
}

func (s *sleeps) sleep(ctx context.Context, d time.Duration) error {
	s.mu.Lock()
	s.d = append(s.d, d)
	s.mu.Unlock()
	return ctx.Err()
}

func newClient(rt *fakeRT, sl *sleeps) *Client {
	return &Client{
		HTTP:        &http.Client{Transport: rt},
		MaxAttempts: 4,
		BaseDelay:   100 * time.Millisecond,
		MaxDelay:    time.Second,
		Sleep:       sl.sleep,
		Now:         func() time.Time { return time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC) },
	}
}

var netErr = errors.New("connection reset by peer")

func TestRetryScenarios(t *testing.T) {
	ms := time.Millisecond
	tests := []struct {
		name       string
		method     string
		idemKey    bool
		steps      []step
		wantCalls  int
		wantCode   int // 0 — ожидается ошибка
		wantSleeps []time.Duration
	}{
		{"okFirst", "GET", false, []step{{code: 200}}, 1, 200, nil},
		{"404noRetry", "GET", false, []step{{code: 404}}, 1, 404, nil},
		{"501noRetry", "GET", false, []step{{code: 501}}, 1, 501, nil},
		{"503then200", "GET", false, []step{{code: 503}, {code: 503}, {code: 200}}, 3, 200, []time.Duration{100 * ms, 200 * ms}},
		{"allFail", "GET", false, []step{{code: 502}}, 4, 502, []time.Duration{100 * ms, 200 * ms, 400 * ms}},
		{"netErrThenOK", "PUT", false, []step{{err: netErr}, {code: 500}, {code: 204}}, 3, 204, []time.Duration{100 * ms, 200 * ms}},
		{"netErrAll", "DELETE", false, []step{{err: netErr}}, 4, 0, []time.Duration{100 * ms, 200 * ms, 400 * ms}},
		{"postNoRetry", "POST", false, []step{{code: 503}, {code: 200}}, 1, 503, nil},
		{"postNetErrNoRetry", "POST", false, []step{{err: netErr}, {code: 200}}, 1, 0, nil},
		{"postIdemKey", "POST", true, []step{{code: 503}, {code: 201}}, 2, 201, []time.Duration{100 * ms}},
		{"retryAfterSecs", "GET", false, []step{{code: 429, hdr: map[string]string{"Retry-After": "0"}}, {code: 200}}, 2, 200, []time.Duration{0}},
		{"retryAfter1", "GET", false, []step{{code: 503, hdr: map[string]string{"Retry-After": "1"}}, {code: 200}}, 2, 200, []time.Duration{time.Second}},
		{"retryAfterCap", "GET", false, []step{{code: 429, hdr: map[string]string{"Retry-After": "120"}}, {code: 200}}, 2, 200, []time.Duration{time.Second}},
		{"retryAfterDate", "GET", false, []step{{code: 503, hdr: map[string]string{"Retry-After": "Mon, 01 Jan 2024 12:00:01 GMT"}}, {code: 200}}, 2, 200, []time.Duration{time.Second}},
		{"retryAfterPastDate", "GET", false, []step{{code: 503, hdr: map[string]string{"Retry-After": "Mon, 01 Jan 2024 11:59:00 GMT"}}, {code: 200}}, 2, 200, []time.Duration{0}},
		{"retryAfterGarbage", "GET", false, []step{{code: 503, hdr: map[string]string{"Retry-After": "soon"}}, {code: 200}}, 2, 200, []time.Duration{100 * ms}},
		{"retryAfterIgnoredOn500", "GET", false, []step{{code: 500, hdr: map[string]string{"Retry-After": "1"}}, {code: 200}}, 2, 200, []time.Duration{100 * ms}},
		{"maxDelayCap", "GET", false, []step{{code: 500}, {code: 500}, {code: 500}, {code: 500}}, 4, 500, []time.Duration{100 * ms, 200 * ms, 400 * ms}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := &fakeRT{steps: tt.steps}
			sl := &sleeps{}
			c := newClient(rt, sl)
			req, _ := http.NewRequest(tt.method, "http://api.local/x", nil)
			if tt.idemKey {
				req.Header.Set("Idempotency-Key", "k1")
			}
			resp, err := c.Do(req)
			if tt.wantCode == 0 {
				if err == nil {
					resp.Body.Close()
					t.Fatalf("ожидалась ошибка, получен код %d", resp.StatusCode)
				}
				if !errors.Is(err, netErr) {
					t.Errorf("ошибка должна оборачивать исходную сетевую: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("неожиданная ошибка: %v", err)
				}
				if resp.StatusCode != tt.wantCode {
					t.Errorf("код %d, ожидалось %d", resp.StatusCode, tt.wantCode)
				}
				resp.Body.Close()
			}
			if rt.calls != tt.wantCalls {
				t.Errorf("попыток %d, ожидалось %d", rt.calls, tt.wantCalls)
			}
			if fmt.Sprint(sl.d) != fmt.Sprint(tt.wantSleeps) {
				t.Errorf("задержки %v, ожидалось %v", sl.d, tt.wantSleeps)
			}
			if int(rt.closed.Load()) != rt.opened {
				t.Errorf("закрыто тел %d из %d — утечка соединений", rt.closed.Load(), rt.opened)
			}
		})
	}
}

func TestRetryMaxDelay(t *testing.T) {
	rt := &fakeRT{steps: []step{{code: 500}}}
	sl := &sleeps{}
	c := newClient(rt, sl)
	c.MaxAttempts = 6
	c.BaseDelay = 300 * time.Millisecond
	req, _ := http.NewRequest("GET", "http://api.local/x", nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	want := "[300ms 600ms 1s 1s 1s]"
	if fmt.Sprint(sl.d) != want {
		t.Errorf("задержки %v, ожидалось %s", sl.d, want)
	}
}

func TestRetryResendsBody(t *testing.T) {
	rt := &fakeRT{steps: []step{{code: 503}, {code: 503}, {code: 200}}}
	c := newClient(rt, &sleeps{})
	req, _ := http.NewRequest("PUT", "http://api.local/x", strings.NewReader(`{"v":1}`))
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if fmt.Sprint(rt.bodies) != `[{"v":1} {"v":1} {"v":1}]` {
		t.Errorf("тело на каждой попытке: %q — надо использовать req.GetBody", rt.bodies)
	}

	// Тело без GetBody повторить нельзя.
	rt = &fakeRT{steps: []step{{code: 503}, {code: 200}}}
	c = newClient(rt, &sleeps{})
	req, _ = http.NewRequest("PUT", "http://api.local/x", io.NopCloser(strings.NewReader("x")))
	resp, err = c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if rt.calls != 1 {
		t.Errorf("тело без GetBody: попыток %d, ожидалась 1", rt.calls)
	}
}

func TestRetryContextCancelDuringSleep(t *testing.T) {
	rt := &fakeRT{steps: []step{{code: 503}}}
	ctx, cancel := context.WithCancel(context.Background())
	c := newClient(rt, nil)
	c.Sleep = func(ctx context.Context, d time.Duration) error {
		cancel()
		<-ctx.Done()
		return ctx.Err()
	}
	req, _ := http.NewRequestWithContext(ctx, "GET", "http://api.local/x", nil)
	resp, err := c.Do(req)
	if !errors.Is(err, context.Canceled) {
		if resp != nil {
			resp.Body.Close()
		}
		t.Fatalf("ожидалась context.Canceled, получено %v", err)
	}
	if rt.calls != 1 {
		t.Errorf("после отмены не должно быть попыток: %d", rt.calls)
	}
	if int(rt.closed.Load()) != rt.opened {
		t.Error("тело ответа не закрыто")
	}
}

func TestRetryRealSleepAndServer(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) < 3 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), BaseDelay: 5 * time.Millisecond}
	req, _ := http.NewRequest("GET", srv.URL, nil)
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if string(b) != "ok" || n.Load() != 3 {
		t.Errorf("тело %q, запросов %d; ожидалось ok и 3 (MaxAttempts по умолчанию 3)", b, n.Load())
	}

	// Реальный Sleep должен уважать отмену контекста.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	slow := &Client{HTTP: &http.Client{Transport: &fakeRT{steps: []step{{code: 503}}}}, BaseDelay: time.Hour, MaxDelay: time.Hour}
	req, _ = http.NewRequestWithContext(ctx, "GET", "http://x/", nil)
	start := time.Now()
	_, err = slow.Do(req)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("ожидался DeadlineExceeded, получено %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("Sleep по умолчанию не прерывается отменой контекста")
	}
}

func TestTransport(t *testing.T) {
	var gotUA, gotKey, gotReq string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA, gotKey, gotReq = r.Header.Get("User-Agent"), r.Header.Get("X-Api-Key"), r.Header.Get("X-Request-Id")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	var logs []string
	tr := &Transport{
		Header: http.Header{"User-Agent": {"course-bot/1.0"}, "X-Api-Key": {"secret"}},
		Logf:   func(f string, a ...any) { logs = append(logs, fmt.Sprintf(f, a...)) },
	}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	req, _ := http.NewRequest("GET", srv.URL+"/items?id=1", nil)
	req.Header.Set("X-Api-Key", "override")
	req.Header.Set("X-Request-Id", "r1")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotUA != "course-bot/1.0" || gotKey != "override" || gotReq != "r1" {
		t.Errorf("сервер увидел UA=%q key=%q req=%q", gotUA, gotKey, gotReq)
	}
	if req.Header.Get("User-Agent") != "" {
		t.Error("RoundTripper не должен менять исходный запрос — нужен req.Clone")
	}
	wantPrefix := "GET " + srv.URL + "/items?id=1 -> 202"
	if len(logs) != 1 || !strings.HasPrefix(logs[0], wantPrefix) {
		t.Errorf("лог %q, ожидалось начало %q", logs, wantPrefix)
	}

	logs = nil
	tr.Base = &fakeRT{steps: []step{{err: netErr}}}
	req, _ = http.NewRequest("DELETE", "http://down.local/x", nil)
	if _, err := client.Do(req); !errors.Is(err, netErr) {
		t.Errorf("ошибка базового транспорта должна вернуться: %v", err)
	}
	if len(logs) != 1 || !strings.HasPrefix(logs[0], "DELETE http://down.local/x -> error: ") {
		t.Errorf("лог ошибки %q", logs)
	}

	// Без Logf — без паники.
	tr.Logf = nil
	tr.Base = nil
	req, _ = http.NewRequest("GET", srv.URL, nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}
