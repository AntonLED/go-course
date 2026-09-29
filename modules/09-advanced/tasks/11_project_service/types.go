package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"time"
)

// Config — конфигурация сервиса (см. LoadConfig).
type Config struct {
	Addr            string        // SVC_ADDR / -addr, по умолчанию ":8080"
	LogLevel        slog.Level    // SVC_LOG_LEVEL / -log-level, по умолчанию info
	ShutdownTimeout time.Duration // SVC_SHUTDOWN_TIMEOUT / -shutdown-timeout, по умолчанию 10s
	DrainDelay      time.Duration // SVC_DRAIN_DELAY / -drain-delay, по умолчанию 0
}

// DefaultConfig — значения по умолчанию.
func DefaultConfig() Config {
	return Config{Addr: ":8080", LogLevel: slog.LevelInfo, ShutdownTimeout: 10 * time.Second}
}

// ErrNotFound возвращает Store, если ключа нет.
var ErrNotFound = errors.New("service: ключ не найден")

// Store — хранилище. Интерфейс объявлен у потребителя (HTTP-слоя):
// реализация может быть в памяти, в Redis или в Postgres.
type Store interface {
	Get(ctx context.Context, key string) (string, error)
	Put(ctx context.Context, key, value string) error
	Len() int
}

// RequestIDHeader — заголовок идентификатора запроса.
const RequestIDHeader = "X-Request-ID"

// MaxBodyBytes — лимит тела PUT-запроса.
const MaxBodyBytes = 1 << 20

// Option — функциональная опция для New.
type Option func(*options)

type options struct {
	logOut io.Writer
	store  Store
}

// WithLogWriter направляет JSON-логи в w (по умолчанию os.Stdout).
func WithLogWriter(w io.Writer) Option { return func(o *options) { o.logOut = w } }

// WithStore подменяет хранилище (по умолчанию — in-memory).
func WithStore(s Store) Option { return func(o *options) { o.store = s } }
