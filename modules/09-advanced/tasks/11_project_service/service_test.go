package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// syncBuffer — потокобезопасный буфер для логов.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) lines(t *testing.T) []map[string]any {
	t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []map[string]any
	for _, l := range bytes.Split(s.b.Bytes(), []byte("\n")) {
		if len(l) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(l, &m); err != nil {
			t.Fatalf("лог не JSON: %q", l)
		}
		out = append(out, m)
	}
	return out
}

func env(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

// ---------------- конфигурация ----------------

func TestLoadConfig(t *testing.T) {
	cfg, err := LoadConfig(nil, env(nil))
	if err != nil || cfg != DefaultConfig() {
		t.Fatalf("LoadConfig(пусто) = %+v, %v; ожидалось DefaultConfig()", cfg, err)
	}
	cfg, err = LoadConfig(
		[]string{"-addr", ":9090", "-drain-delay=2s"},
		env(map[string]string{"SVC_ADDR": ":7070", "SVC_LOG_LEVEL": "debug", "SVC_SHUTDOWN_TIMEOUT": "3s", "SVC_DRAIN_DELAY": "1s"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	want := Config{Addr: ":9090", LogLevel: slog.LevelDebug, ShutdownTimeout: 3 * time.Second, DrainDelay: 2 * time.Second}
	if cfg != want {
		t.Errorf("LoadConfig = %+v, ожидалось %+v (флаги > env > defaults)", cfg, want)
	}
	if cfg, _ := LoadConfig([]string{"-log-level=WARN"}, nil); cfg.LogLevel != slog.LevelWarn {
		t.Errorf("-log-level=WARN → %v", cfg.LogLevel)
	}
	bad := []struct {
		args []string
		env  map[string]string
	}{
		{[]string{"-addr=localhost"}, nil},
		{nil, map[string]string{"SVC_LOG_LEVEL": "loud"}},
		{nil, map[string]string{"SVC_SHUTDOWN_TIMEOUT": "10"}},
		{[]string{"-shutdown-timeout=0s"}, nil},
		{[]string{"-unknown"}, nil},
	}
	for _, tc := range bad {
		if _, err := LoadConfig(tc.args, env(tc.env)); err == nil {
			t.Errorf("LoadConfig(%v, %v): ожидалась ошибка", tc.args, tc.env)
		}
	}
}

// ---------------- HTTP ----------------

func newApp(t *testing.T, opts ...Option) (*App, *syncBuffer) {
	t.Helper()
	logs := &syncBuffer{}
	cfg := DefaultConfig()
	cfg.LogLevel = slog.LevelDebug
	app, err := New(cfg, append([]Option{WithLogWriter(logs)}, opts...)...)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return app, logs
}

func do(h http.Handler, method, path, body string, hdr map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestKV(t *testing.T) {
	app, _ := newApp(t)
	h := app.Handler()
	if rec := do(h, "GET", "/kv/a", "", nil); rec.Code != 404 {
		t.Errorf("GET отсутствующего ключа = %d, ожидалось 404", rec.Code)
	}
	if rec := do(h, "PUT", "/kv/a", "hello", nil); rec.Code != 204 {
		t.Errorf("PUT = %d, ожидалось 204", rec.Code)
	}
	if rec := do(h, "GET", "/kv/a", "", nil); rec.Code != 200 || rec.Body.String() != "hello" {
		t.Errorf("GET = %d %q, ожидалось 200 hello", rec.Code, rec.Body.String())
	}
	if rec := do(h, "PUT", "/kv/big", strings.Repeat("x", MaxBodyBytes+1), nil); rec.Code != 413 {
		t.Errorf("PUT больше лимита = %d, ожидалось 413", rec.Code)
	}
	if rec := do(h, "DELETE", "/kv/a", "", nil); rec.Code != 405 {
		t.Errorf("DELETE = %d, ожидалось 405", rec.Code)
	}
	if rec := do(h, "GET", "/healthz", "", nil); rec.Code != 200 {
		t.Errorf("/healthz = %d", rec.Code)
	}
	if rec := do(h, "GET", "/readyz", "", nil); rec.Code != 200 {
		t.Errorf("/readyz = %d, после New приложение готово", rec.Code)
	}
}

func TestRequestIDAndLogs(t *testing.T) {
	app, logs := newApp(t)
	h := app.Handler()

	rec := do(h, "PUT", "/kv/x", "1", map[string]string{
		RequestIDHeader: "req-123",
		"traceparent":   "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
	})
	if got := rec.Header().Get(RequestIDHeader); got != "req-123" {
		t.Errorf("входящий X-Request-ID должен возвращаться: %q", got)
	}
	rec2 := do(h, "GET", "/kv/x", "", map[string]string{RequestIDHeader: "bad id\nwith newline"})
	gen := rec2.Header().Get(RequestIDHeader)
	if gen == "" || strings.ContainsAny(gen, " \n") {
		t.Errorf("небезопасный X-Request-ID должен заменяться сгенерированным, получено %q", gen)
	}
	rec3 := do(h, "GET", "/kv/x", "", nil)
	if id := rec3.Header().Get(RequestIDHeader); id == "" || id == gen {
		t.Errorf("request id должен генерироваться и быть уникальным: %q / %q", id, gen)
	}

	var access []map[string]any
	for _, l := range logs.lines(t) {
		if l["msg"] == "http request" {
			access = append(access, l)
		}
	}
	if len(access) != 3 {
		t.Fatalf("ожидалось 3 строки access-лога с msg=\"http request\", получено %d", len(access))
	}
	first := access[0]
	checks := map[string]any{
		"request_id": "req-123", "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
		"method": "PUT", "path": "/kv/x", "status": float64(204), "level": "INFO",
	}
	for k, v := range checks {
		if first[k] != v {
			t.Errorf("access-лог[%s] = %v, ожидалось %v (строка %v)", k, first[k], v, first)
		}
	}
	if _, ok := first["duration_ms"].(float64); !ok {
		t.Errorf("access-лог должен содержать числовой duration_ms: %v", first)
	}
	if access[1]["request_id"] != gen {
		t.Errorf("request_id в логе (%v) должен совпадать с заголовком ответа (%s)", access[1]["request_id"], gen)
	}
	tid, _ := access[2]["trace_id"].(string)
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(tid) || tid == "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("без traceparent должна начинаться новая трасса, trace_id=%q", tid)
	}
}

type failingStore struct{ panicOnGet bool }

func (f failingStore) Get(context.Context, string) (string, error) {
	if f.panicOnGet {
		panic("store сломан")
	}
	return "", errors.New("db down")
}
func (failingStore) Put(context.Context, string, string) error { return errors.New("db down") }
func (failingStore) Len() int                                  { return 42 }

func TestStoreErrorsAndPanics(t *testing.T) {
	app, logs := newApp(t, WithStore(failingStore{}))
	if rec := do(app.Handler(), "PUT", "/kv/a", "v", map[string]string{RequestIDHeader: "r-err"}); rec.Code != 500 {
		t.Errorf("ошибка хранилища → %d, ожидалось 500", rec.Code)
	}
	found := false
	for _, l := range logs.lines(t) {
		if l["level"] == "ERROR" && l["request_id"] == "r-err" {
			found = true
		}
	}
	if !found {
		t.Error("ошибка хранилища должна логироваться на уровне ERROR с request_id из контекста")
	}

	app2, logs2 := newApp(t, WithStore(failingStore{panicOnGet: true}))
	h := app2.Handler()
	if rec := do(h, "GET", "/kv/a", "", nil); rec.Code != 500 {
		t.Errorf("паника в обработчике → %d, ожидалось 500", rec.Code)
	}
	if rec := do(h, "GET", "/healthz", "", nil); rec.Code != 200 {
		t.Error("после паники сервис продолжает работать")
	}
	if !strings.Contains(logs2.b.String(), "store сломан") {
		t.Error("паника должна логироваться")
	}
	body := do(h, "GET", "/metrics", "", nil).Body.String()
	if !strings.Contains(body, `http_requests_total{method="GET",route="GET /kv/{key}",code="500"} 1`) {
		t.Errorf("паника должна учитываться в метриках как 500:\n%s", body)
	}
	if !strings.Contains(body, "kv_keys 42\n") {
		t.Errorf("kv_keys должен браться из Store.Len():\n%s", body)
	}
}

func TestMetrics(t *testing.T) {
	app, _ := newApp(t)
	h := app.Handler()
	do(h, "PUT", "/kv/a", "1", nil)
	do(h, "PUT", "/kv/b", "2", nil)
	do(h, "GET", "/kv/a", "", nil)
	do(h, "GET", "/kv/zzz", "", nil)
	do(h, "GET", "/no/such/route", "", nil)

	rec := do(h, "GET", "/metrics", "", nil)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain; version=0.0.4") {
		t.Errorf("Content-Type /metrics = %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"# TYPE http_requests_total counter",
		"# TYPE http_request_duration_seconds histogram",
		"# TYPE kv_keys gauge",
		`http_requests_total{method="PUT",route="PUT /kv/{key}",code="204"} 2`,
		`http_requests_total{method="GET",route="GET /kv/{key}",code="200"} 1`,
		`http_requests_total{method="GET",route="GET /kv/{key}",code="404"} 1`,
		`http_requests_total{method="GET",route="unmatched",code="404"} 1`,
		`http_request_duration_seconds_count{method="PUT",route="PUT /kv/{key}"} 2`,
		`http_request_duration_seconds_bucket{method="PUT",route="PUT /kv/{key}",le="+Inf"} 2`,
		"kv_keys 2",
	} {
		if !strings.Contains(body, want+"\n") {
			t.Errorf("в /metrics нет строки %q", want)
		}
	}
	if strings.Contains(body, `route="/kv/a"`) || strings.Contains(body, "zzz") {
		t.Error("в лейбле route должен быть шаблон маршрута, а не путь (кардинальность!)")
	}
}

// ---------------- graceful shutdown ----------------

// slowStore: Get сигналит о начале и работает 300 мс.
type slowStore struct{ started chan struct{} }

func (s slowStore) Get(context.Context, string) (string, error) {
	close(s.started)
	time.Sleep(300 * time.Millisecond)
	return "slow-value", nil
}
func (slowStore) Put(context.Context, string, string) error { return nil }
func (slowStore) Len() int                                  { return 0 }

func TestRunGraceful(t *testing.T) {
	logs := &syncBuffer{}
	cfg := DefaultConfig()
	cfg.DrainDelay = 300 * time.Millisecond
	cfg.ShutdownTimeout = 5 * time.Second
	store := slowStore{started: make(chan struct{})}
	app, err := New(cfg, WithLogWriter(logs), WithStore(store))
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- app.Run(ctx, ln) }()

	client := &http.Client{Timeout: 3 * time.Second}
	get := func(path string) (int, string, error) {
		resp, err := client.Get(base + path)
		if err != nil {
			return 0, "", err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), nil
	}
	if code, _, err := get("/readyz"); err != nil || code != 200 {
		t.Fatalf("/readyz до остановки = %d, %v", code, err)
	}

	slow := make(chan string, 1)
	go func() {
		_, body, err := get("/kv/k")
		if err != nil {
			body = "ошибка: " + err.Error()
		}
		slow <- body
	}()
	<-store.started
	cancel() // SIGTERM

	time.Sleep(100 * time.Millisecond) // внутри DrainDelay
	if code, _, err := get("/readyz"); err != nil || code != http.StatusServiceUnavailable {
		t.Errorf("/readyz во время drain = %d, %v; ожидалось 503", code, err)
	}
	if body := <-slow; body != "slow-value" {
		t.Errorf("активный запрос должен завершиться: %q", body)
	}
	select {
	case err := <-runErr:
		if err != nil {
			t.Errorf("Run = %v, ожидалось nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run не завершился")
	}
	if _, _, err := get("/healthz"); err == nil {
		t.Error("после остановки сервер не должен принимать подключения")
	}
	if !strings.Contains(logs.b.String(), "shutdown complete") {
		t.Error(`в логе должно быть сообщение "shutdown complete"`)
	}
}

// ---------------- Dockerfile ----------------

func TestDockerfile(t *testing.T) {
	data, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("нет %s: %v", dockerfilePath, err)
	}
	src := regexp.MustCompile(`(?m)^\s*#.*$`).ReplaceAllString(string(data), "")
	src = strings.ReplaceAll(src, "\\\n", " ")
	fromRe := regexp.MustCompile(`(?mi)^\s*FROM\s+(?:--\S+\s+)*(\S+)(?:\s+AS\s+(\S+))?`)
	froms := fromRe.FindAllStringSubmatch(src, -1)
	if len(froms) < 2 {
		t.Fatalf("нужна multi-stage сборка, FROM: %d", len(froms))
	}
	finalIdx := regexp.MustCompile(`(?mi)^\s*FROM\s`).FindAllStringIndex(src, -1)
	final := src[finalIdx[len(finalIdx)-1][0]:]
	finalImage := froms[len(froms)-1][1]
	stages := map[string]bool{}
	for _, f := range froms {
		img := f[1]
		last := img[strings.LastIndex(img, "/")+1:] // тег — после последнего "/" (у registry:5000 двоеточие раньше)
		_, tag, hasTag := strings.Cut(last, ":")
		switch {
		case img == "scratch" || stages[img] || strings.Contains(img, "@sha256:"):
		case !hasTag || tag == "" || tag == "latest":
			t.Errorf("образ %q без фиксированного тега", img)
		}
		if f[2] != "" {
			stages[f[2]] = true
		}
	}
	nonRoot := false
	if m := regexp.MustCompile(`(?mi)^\s*USER\s+(\S+)`).FindAllStringSubmatch(final, -1); len(m) > 0 {
		u, _, _ := strings.Cut(m[len(m)-1][1], ":")
		nonRoot = u != "" && u != "root" && u != "0"
	}
	flagRe := func(f string) *regexp.Regexp { return regexp.MustCompile(`(^|[\s"'=])` + f + `($|[\s"'])`) }
	checks := []struct {
		ok   bool
		what string
	}{
		{regexp.MustCompile(`CGO_ENABLED[= ]0`).MatchString(src), "CGO_ENABLED=0"},
		{strings.Contains(src, "-trimpath"), "-trimpath"},
		{strings.Contains(src, "-ldflags") && flagRe("-s").MatchString(src) && flagRe("-w").MatchString(src), `-ldflags "-s -w ..."`},
		{regexp.MustCompile(`-X[= ]+['"]?main\.version=`).MatchString(src), "-X main.version="},
		{finalImage == "scratch" || strings.HasPrefix(finalImage, "gcr.io/distroless/"), "финальный образ scratch/distroless"},
		{nonRoot, "USER non-root в финальной стадии"},
		{regexp.MustCompile(`(?mi)^\s*EXPOSE\s+\d+`).MatchString(final), "EXPOSE"},
		{regexp.MustCompile(`(?mi)^\s*ENTRYPOINT\s+\[`).MatchString(final), `ENTRYPOINT в exec-форме ["/kvservice"]`},
		{!regexp.MustCompile(`(?mi)^\s*COPY\s+\.\s`).MatchString(final), "нет COPY . . в финальной стадии"},
	}
	for _, c := range checks {
		if !c.ok {
			t.Errorf("Dockerfile: требуется %s", c.what)
		}
	}
}
