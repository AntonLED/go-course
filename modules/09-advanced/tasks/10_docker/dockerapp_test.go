package dockerapp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestReadInfo(t *testing.T) {
	info := ReadInfo("1.2.3", "abc123")
	if info.Version != "1.2.3" || info.Commit != "abc123" {
		t.Errorf("ReadInfo(ldflags) = %+v", info)
	}
	if info.GoVersion != runtime.Version() {
		t.Errorf("GoVersion = %q, ожидалось %q (debug.ReadBuildInfo)", info.GoVersion, runtime.Version())
	}
	// Module берётся из debug.ReadBuildInfo().Main.Path. В тестовом бинаре до Go 1.24
	// это поле пустое, а с 1.24 там путь модуля ("gocourse"), поэтому сравниваем
	// с тем, что видит сам тест, а не с константой.
	wantModule := ""
	if bi, ok := debug.ReadBuildInfo(); ok {
		wantModule = bi.Main.Path
	}
	if info.Module != wantModule {
		t.Errorf("Module = %q, ожидалось %q (debug.ReadBuildInfo().Main.Path)", info.Module, wantModule)
	}
	def := ReadInfo("", "")
	if def.Version != "dev" {
		t.Errorf("без ldflags Version = %q, ожидалось dev", def.Version)
	}
	if def.Commit == "" {
		t.Error("Commit не должен быть пустым (vcs.revision или unknown)")
	}
	if s := info.String(); s != "1.2.3 (abc123, "+runtime.Version()+")" {
		t.Errorf("String() = %q", s)
	}
}

func TestHandler(t *testing.T) {
	var ready atomic.Bool
	h := NewHandler(Info{Version: "9.9.9", Commit: "c", GoVersion: "go1.23"}, ready.Load)
	do := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}
	if rec := do("GET", "/healthz"); rec.Code != 200 {
		t.Errorf("/healthz = %d, ожидалось 200", rec.Code)
	}
	if rec := do("GET", "/readyz"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("/readyz до готовности = %d, ожидалось 503", rec.Code)
	}
	ready.Store(true)
	if rec := do("GET", "/readyz"); rec.Code != 200 {
		t.Errorf("/readyz после готовности = %d, ожидалось 200", rec.Code)
	}
	rec := do("GET", "/version")
	var got Info
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got.Version != "9.9.9" || got.GoVersion != "go1.23" {
		t.Errorf("/version = %s (%v)", rec.Body.String(), err)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("/version Content-Type = %q", ct)
	}
	if rec := do("POST", "/healthz"); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /healthz = %d, ожидалось 405", rec.Code)
	}
	if rec := do("GET", "/nope"); rec.Code != 404 {
		t.Errorf("/nope = %d, ожидалось 404", rec.Code)
	}
	if rec := httptest.NewRecorder(); true {
		NewHandler(Info{}, nil).ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
		if rec.Code != 200 {
			t.Errorf("ready == nil → всегда готов, получено %d", rec.Code)
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

func TestRunGracefulShutdown(t *testing.T) {
	ln := listen(t)
	url := "http://" + ln.Addr().String()
	started := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			close(started)
			time.Sleep(300 * time.Millisecond)
		}
		_, _ = io.WriteString(w, "done")
	})
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- Run(ctx, ln, h, 5*time.Second) }()

	if err := Probe(context.Background(), url+"/fast"); err != nil {
		t.Fatalf("сервер не отвечает: %v", err)
	}

	type result struct {
		body string
		err  error
	}
	slow := make(chan result, 1)
	go func() {
		resp, err := http.Get(url + "/slow")
		if err != nil {
			slow <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		slow <- result{string(b), err}
	}()
	<-started
	cancel() // «SIGTERM» во время обработки запроса

	select {
	case err := <-runErr:
		t.Fatalf("Run вернулся (%v) до завершения активного запроса", err)
	case <-time.After(100 * time.Millisecond):
	}
	r := <-slow
	if r.err != nil || r.body != "done" {
		t.Errorf("активный запрос должен завершиться успешно: %q, %v", r.body, r.err)
	}
	select {
	case err := <-runErr:
		if err != nil {
			t.Errorf("Run = %v, ожидалось nil при корректной остановке", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run не завершился после отмены контекста")
	}
	c := http.Client{Timeout: time.Second}
	if _, err := c.Get(url + "/fast"); err == nil {
		t.Error("после остановки новые подключения должны отклоняться")
	}
}

func TestRunShutdownTimeout(t *testing.T) {
	ln := listen(t)
	url := "http://" + ln.Addr().String()
	release := make(chan struct{})
	defer close(release)
	started := make(chan struct{})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release // «зависший» обработчик
	})
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- Run(ctx, ln, h, 100*time.Millisecond) }()
	go func() {
		resp, err := http.Get(url)
		if err == nil {
			resp.Body.Close()
		}
	}()
	<-started
	cancel()
	select {
	case err := <-runErr:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Run = %v, ожидалась ошибка с context.DeadlineExceeded", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run должен вернуться через shutdownTimeout, даже если обработчик завис")
	}
}

func TestRunListenerClosed(t *testing.T) {
	ln := listen(t)
	ln.Close()
	done := make(chan error, 1)
	go func() { done <- Run(context.Background(), ln, http.NotFoundHandler(), time.Second) }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("Run на закрытом listener должен вернуть ошибку")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run должен вернуть ошибку Serve, не дожидаясь ctx")
	}
}

func TestProbe(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			w.WriteHeader(500)
		}
	}))
	defer srv.Close()
	if err := Probe(context.Background(), srv.URL+"/ok"); err != nil {
		t.Errorf("Probe(200) = %v", err)
	}
	if err := Probe(context.Background(), srv.URL+"/bad"); err == nil {
		t.Error("Probe(500) должен вернуть ошибку")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Probe(ctx, srv.URL); err == nil {
		t.Error("Probe с отменённым контекстом должен вернуть ошибку")
	}
}

func TestMaxProcsFromCgroup(t *testing.T) {
	tests := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"max 100000\n", 0, false},
		{"max", 0, false},
		{"200000 100000", 2, false},
		{"150000 100000\n", 1, false}, // floor, как automaxprocs
		{"50000 100000", 1, false},    // минимум 1
		{"400000 50000", 8, false},
		{"100000", 1, false}, // период по умолчанию 100000
		{"", 0, true},
		{"abc 100000", 0, true},
		{"100000 0", 0, true},
		{"-5 100000", 0, true},
		{"1 2 3", 0, true},
	}
	for _, tc := range tests {
		got, err := MaxProcsFromCgroup(tc.in)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("MaxProcsFromCgroup(%q) = %d, %v; ожидалось %d, ошибка=%v", tc.in, got, err, tc.want, tc.wantErr)
		}
	}
}
