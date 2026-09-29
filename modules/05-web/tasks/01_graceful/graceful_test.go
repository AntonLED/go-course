package graceful

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewServerTimeouts(t *testing.T) {
	h := http.NotFoundHandler()
	srv := NewServer(h)
	if srv.Handler == nil {
		t.Fatal("Handler не установлен")
	}
	if srv.ReadHeaderTimeout <= 0 || srv.ReadHeaderTimeout > 10*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, ожидалось (0, 10s]", srv.ReadHeaderTimeout)
	}
	for name, d := range map[string]time.Duration{
		"ReadTimeout":  srv.ReadTimeout,
		"WriteTimeout": srv.WriteTimeout,
		"IdleTimeout":  srv.IdleTimeout,
	} {
		if d <= 0 {
			t.Errorf("%s = %v, ожидалось > 0", name, d)
		}
	}
	if srv.MaxHeaderBytes <= 0 {
		t.Errorf("MaxHeaderBytes = %d, ожидалось > 0", srv.MaxHeaderBytes)
	}
}

func TestHealth(t *testing.T) {
	var ready atomic.Bool
	h := Health(&ready)
	tests := []struct {
		method, path string
		ready        bool
		code         int
	}{
		{"GET", "/healthz", false, 200},
		{"GET", "/readyz", false, 503},
		{"GET", "/readyz", true, 200},
		{"POST", "/healthz", true, 405},
		{"GET", "/nope", true, 404},
	}
	for _, tt := range tests {
		ready.Store(tt.ready)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(tt.method, tt.path, nil))
		if rec.Code != tt.code {
			t.Errorf("%s %s (ready=%v): код %d, ожидалось %d", tt.method, tt.path, tt.ready, rec.Code, tt.code)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Body.String() != "ok" {
		t.Errorf("тело /healthz = %q, ожидалось \"ok\"", rec.Body.String())
	}
	if got := Health(nil); got == nil {
		t.Error("Health(nil) вернул nil")
	} else {
		rec := httptest.NewRecorder()
		got.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
		if rec.Code != 503 {
			t.Errorf("Health(nil) /readyz: %d, ожидалось 503", rec.Code)
		}
	}
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return ln
}

func TestServeGracefulFinishesInFlight(t *testing.T) {
	started := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		close(started)
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, "done")
	})
	ln := listen(t)
	addr := ln.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serveErr := make(chan error, 1)
	go func() { serveErr <- Serve(ctx, NewServer(mux), ln, 2*time.Second) }()

	type result struct {
		body string
		err  error
	}
	resCh := make(chan result, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/slow")
		if err != nil {
			resCh <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		resCh <- result{string(b), err}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("запрос не дошёл до обработчика — сервер не запущен?")
	}
	cancel()

	select {
	case r := <-resCh:
		if r.err != nil || r.body != "done" {
			t.Fatalf("активный запрос должен завершиться успешно: body=%q err=%v", r.body, r.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("активный запрос не завершился")
	}
	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("Serve вернул %v, ожидался nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve не вернулся после отмены контекста")
	}
	c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err == nil {
		c.Close()
		t.Error("после shutdown листенер всё ещё принимает соединения")
	}
}

func TestServeShutdownTimeout(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	mux := http.NewServeMux()
	mux.HandleFunc("/hang", func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-release:
		case <-r.Context().Done(): // после Close соединение рвётся
		}
	})
	ln := listen(t)
	addr := ln.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	serveErr := make(chan error, 1)
	go func() { serveErr <- Serve(ctx, NewServer(mux), ln, 100*time.Millisecond) }()
	go func() {
		resp, err := http.Get("http://" + addr + "/hang")
		if err == nil {
			resp.Body.Close()
		}
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("запрос не дошёл до обработчика")
	}
	begin := time.Now()
	cancel()
	select {
	case err := <-serveErr:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Serve вернул %v, ожидалась ошибка с context.DeadlineExceeded", err)
		}
		if el := time.Since(begin); el > 2*time.Second {
			t.Errorf("shutdown занял %v, ожидалось около 100ms", el)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve не уважает shutdownTimeout")
	}
}

func TestServeReturnsServeError(t *testing.T) {
	ln := listen(t)
	ln.Close()
	done := make(chan error, 1)
	go func() { done <- Serve(context.Background(), NewServer(http.NotFoundHandler()), ln, time.Second) }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("Serve на закрытом листенере вернул nil")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve должен сразу вернуть ошибку srv.Serve, не дожидаясь ctx")
	}
}

func TestRunBadAddr(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Run(ctx, "256.0.0.1:bad", http.NotFoundHandler()); err == nil {
		t.Fatal("Run с некорректным адресом должен вернуть ошибку")
	}
}

func TestRunServes(t *testing.T) {
	ln := listen(t)
	addr := ln.Addr().String()
	ln.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, addr, Health(nil)) }()
	var resp *http.Response
	var err error
	for i := 0; i < 50; i++ {
		resp, err = http.Get("http://" + addr + "/healthz")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("Run не поднял сервер: %v", err)
	}
	resp.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run вернул %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run не завершился после отмены ctx")
	}
}
