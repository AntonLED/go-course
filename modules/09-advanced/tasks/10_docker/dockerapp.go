//go:build !solution

package dockerapp

import (
	"context"
	"net"
	"net/http"
	"time"
)

// dockerfilePath — какой Dockerfile проверяет тест-линтер. НЕ меняйте:
// ваша задача — исправить файл Dockerfile в этой директории.
const dockerfilePath = "Dockerfile"

// ReadInfo собирает Info из переменных, заданных через -ldflags -X, и
// runtime/debug.ReadBuildInfo (см. README).
func ReadInfo(version, commit string) Info {
	panic("TODO")
}

// NewHandler: GET /healthz, GET /readyz, GET /version. ready == nil — всегда готов.
func NewHandler(info Info, ready func() bool) http.Handler {
	panic("TODO")
}

// Run обслуживает HTTP на ln, пока не отменён ctx, затем делает graceful
// shutdown, ожидая активные запросы не дольше shutdownTimeout.
func Run(ctx context.Context, ln net.Listener, h http.Handler, shutdownTimeout time.Duration) error {
	panic("TODO")
}

// Probe делает GET url и возвращает ошибку, если ответ не 200.
// Используется в HEALTHCHECK distroless-образа, где нет curl/wget.
func Probe(ctx context.Context, url string) error {
	panic("TODO")
}

// MaxProcsFromCgroup вычисляет GOMAXPROCS по содержимому cgroup v2 файла
// cpu.max ("<quota> <period>" или "max <period>"). 0 — лимита нет.
func MaxProcsFromCgroup(cpuMax string) (int, error) {
	panic("TODO")
}
