//go:build solution

package service

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

const dockerfilePath = "Dockerfile.solution"

// ======================= конфигурация =======================

// LoadConfig: DefaultConfig() < SVC_* < явно заданные флаги, затем валидация.
func LoadConfig(args []string, lookup func(string) (string, bool)) (Config, error) {
	if lookup == nil {
		lookup = func(string) (string, bool) { return "", false }
	}
	cfg := DefaultConfig()

	// Слой окружения.
	if v, ok := lookup("SVC_ADDR"); ok {
		cfg.Addr = v
	}
	if v, ok := lookup("SVC_LOG_LEVEL"); ok {
		if err := cfg.LogLevel.UnmarshalText([]byte(v)); err != nil {
			return Config{}, fmt.Errorf("SVC_LOG_LEVEL: %w", err)
		}
	}
	for key, dst := range map[string]*time.Duration{
		"SVC_SHUTDOWN_TIMEOUT": &cfg.ShutdownTimeout,
		"SVC_DRAIN_DELAY":      &cfg.DrainDelay,
	} {
		if v, ok := lookup(key); ok {
			d, err := time.ParseDuration(v)
			if err != nil {
				return Config{}, fmt.Errorf("%s: %w", key, err)
			}
			*dst = d
		}
	}

	// Слой флагов: значения по умолчанию флагов = текущие значения cfg,
	// поэтому незаданный флаг ничего не меняет.
	fs := flag.NewFlagSet("kvservice", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "адрес HTTP-сервера")
	fs.TextVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "уровень логов (debug|info|warn|error)")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", cfg.ShutdownTimeout, "таймаут graceful shutdown")
	fs.DurationVar(&cfg.DrainDelay, "drain-delay", cfg.DrainDelay, "пауза между readyz=503 и Shutdown")
	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	// Валидация.
	var errs []error
	if _, port, err := net.SplitHostPort(cfg.Addr); err != nil {
		errs = append(errs, fmt.Errorf("addr %q: %w", cfg.Addr, err))
	} else if p, err := strconv.Atoi(port); err != nil || p < 0 || p > 65535 {
		errs = append(errs, fmt.Errorf("addr %q: некорректный порт", cfg.Addr))
	}
	if cfg.ShutdownTimeout <= 0 {
		errs = append(errs, errors.New("shutdown-timeout должен быть > 0"))
	}
	if cfg.DrainDelay < 0 {
		errs = append(errs, errors.New("drain-delay не может быть отрицательным"))
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// ======================= хранилище =======================

type memoryStore struct {
	mu sync.RWMutex
	m  map[string]string
}

func newMemoryStore() *memoryStore { return &memoryStore{m: map[string]string{}} }

func (s *memoryStore) Get(_ context.Context, key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (s *memoryStore) Put(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
	return nil
}

func (s *memoryStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.m)
}

// ======================= HTTP-слой =======================

// kvHandler зависит только от интерфейса Store и логгера — всё приходит
// через конструктор, глобальных переменных нет.
type kvHandler struct {
	store Store
	log   *slog.Logger
}

func newKVHandler(store Store, log *slog.Logger) *kvHandler {
	return &kvHandler{store: store, log: log}
}

func (h *kvHandler) get(w http.ResponseWriter, r *http.Request) {
	v, err := h.store.Get(r.Context(), r.PathValue("key"))
	switch {
	case errors.Is(err, ErrNotFound):
		http.Error(w, "not found", http.StatusNotFound)
	case err != nil:
		h.log.ErrorContext(r.Context(), "store get", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	default:
		_, _ = io.WriteString(w, v)
	}
}

func (h *kvHandler) put(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, MaxBodyBytes))
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
			return
		}
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}
	if err := h.store.Put(r.Context(), r.PathValue("key"), string(body)); err != nil {
		h.log.ErrorContext(r.Context(), "store put", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ======================= сборка приложения =======================

// App — собранное приложение.
type App struct {
	cfg     Config
	log     *slog.Logger
	ready   atomic.Bool
	handler http.Handler
}

// New — composition root: здесь (и только здесь) создаются конкретные
// реализации и связываются через конструкторы.
func New(cfg Config, opts ...Option) (*App, error) {
	o := options{logOut: os.Stdout}
	for _, opt := range opts {
		opt(&o)
	}
	if o.store == nil {
		o.store = newMemoryStore()
	}
	if cfg.ShutdownTimeout <= 0 {
		return nil, errors.New("service: ShutdownTimeout должен быть > 0")
	}

	log := newLogger(o.logOut, cfg.LogLevel)
	m := newMetrics(o.store.Len)
	kv := newKVHandler(o.store, log)
	a := &App{cfg: cfg, log: log}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /kv/{key}", kv.get)
	mux.HandleFunc("PUT /kv/{key}", kv.put)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok\n")
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, _ *http.Request) {
		if !a.ready.Load() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		_, _ = io.WriteString(w, "ready\n")
	})
	mux.Handle("GET /metrics", m)

	// Порядок: request-id → трейсинг → access-лог/recover → метрики → mux.
	// Внешние слои кладут данные в контекст, внутренние ими пользуются.
	a.handler = requestID(tracing(accessLog(log, instrument(m, mux))))
	a.ready.Store(true)
	return a, nil
}

// Handler возвращает корневой обработчик со всеми middleware.
func (a *App) Handler() http.Handler { return a.handler }

// Run обслуживает ln до отмены ctx, затем корректно останавливается.
func (a *App) Run(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           a.handler,
		ReadHeaderTimeout: 5 * time.Second,
		ErrorLog:          slog.NewLogLogger(a.log.Handler(), slog.LevelWarn),
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	a.log.Info("сервер запущен", "addr", ln.Addr().String())

	select {
	case err := <-errc:
		return err // Serve упал сам (ErrServerClosed здесь быть не может)
	case <-ctx.Done():
	}

	// 1. Сообщаем балансировщику, что трафик больше слать не надо,
	// и ждём, пока он это заметит (в Kubernetes — несколько периодов readiness probe).
	a.ready.Store(false)
	a.log.Info("остановка: readyz=503", "drain_delay", a.cfg.DrainDelay)
	time.Sleep(a.cfg.DrainDelay)

	// 2. Graceful shutdown: закрываем listener, дожидаемся активных запросов.
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), a.cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		_ = srv.Close()
		a.log.Error("shutdown", "err", err)
		return fmt.Errorf("shutdown: %w", err)
	}
	<-errc // Serve вернул ErrServerClosed
	a.log.Info("shutdown complete")
	return nil
}
