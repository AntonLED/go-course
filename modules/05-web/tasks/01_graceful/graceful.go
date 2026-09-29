//go:build !solution

package graceful

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// NewServer создаёт http.Server с обработчиком h и разумными таймаутами:
// ReadHeaderTimeout (обязательно! больше 0 и не больше 10 с), ReadTimeout, WriteTimeout,
// IdleTimeout и MaxHeaderBytes.
func NewServer(h http.Handler) *http.Server {
	// TODO
	return &http.Server{}
}

// Health возвращает обработчик:
//
//	GET /healthz -> 200 "ok"
//	GET /readyz  -> 200 "ready", если ready != nil && ready.Load(), иначе 503.
func Health(ready *atomic.Bool) http.Handler {
	// TODO
	return http.NotFoundHandler()
}

// Serve обслуживает соединения из ln, пока не отменят ctx.
// После отмены — graceful shutdown не дольше shutdownTimeout:
//   - активные запросы дорабатывают, новые соединения не принимаются;
//   - если уложились — вернуть nil;
//   - если нет — закрыть сервер принудительно и вернуть ошибку,
//     для которой errors.Is(err, context.DeadlineExceeded) == true.
//
// Если srv.Serve завершился сам (не из-за Shutdown) — вернуть его ошибку сразу.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, shutdownTimeout time.Duration) error {
	// TODO
	return errors.New("TODO")
}

// Run слушает addr и вызывает Serve с NewServer(h) и таймаутом 10 с.
func Run(ctx context.Context, addr string, h http.Handler) error {
	// TODO
	return errors.New("TODO")
}
