//go:build !solution

package service

import (
	"context"
	"net"
	"net/http"
)

// dockerfilePath — какой Dockerfile проверяет тест. НЕ меняйте.
const dockerfilePath = "Dockerfile"

// LoadConfig: DefaultConfig() < переменные окружения SVC_* < явно заданные флаги.
// args — без имени программы; lookup — как os.LookupEnv.
func LoadConfig(args []string, lookup func(string) (string, bool)) (Config, error) {
	panic("TODO")
}

// App — собранное приложение (composition root).
type App struct {
	// TODO: логгер, метрики, хранилище, флаг готовности, http.Handler...
}

// New собирает приложение из компонентов (constructor injection).
func New(cfg Config, opts ...Option) (*App, error) {
	panic("TODO")
}

// Handler возвращает корневой обработчик со всеми middleware.
func (a *App) Handler() http.Handler {
	panic("TODO")
}

// Run обслуживает ln до отмены ctx, затем: readyz → 503, пауза DrainDelay,
// graceful shutdown с ShutdownTimeout.
func (a *App) Run(ctx context.Context, ln net.Listener) error {
	panic("TODO")
}
