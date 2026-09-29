//go:build solution

package graceful

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

// NewServer создаёт http.Server с «боевыми» таймаутами.
// Сервер без таймаутов уязвим к Slowloris: клиент открывает соединение и
// присылает заголовки по байту в минуту, держа горутину и дескриптор.
func NewServer(h http.Handler) *http.Server {
	return &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,  // главное — защита от медленных заголовков
		ReadTimeout:       15 * time.Second, // заголовки + тело
		WriteTimeout:      15 * time.Second, // от конца чтения заголовков до конца записи ответа
		IdleTimeout:       60 * time.Second, // keep-alive простой
		MaxHeaderBytes:    1 << 20,
	}
}

// Health возвращает обработчик /healthz (liveness) и /readyz (readiness).
func Health(ready *atomic.Bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if ready == nil || !ready.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return
		}
		_, _ = w.Write([]byte("ready"))
	})
	return mux
}

// Serve обслуживает ln, пока не отменят ctx, затем делает graceful shutdown.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, shutdownTimeout time.Duration) error {
	errCh := make(chan error, 1)
	go func() {
		// Serve всегда возвращает не-nil ошибку; после Shutdown — ErrServerClosed.
		errCh <- srv.Serve(ln)
	}()

	select {
	case err := <-errCh:
		// Сервер упал сам (например, листенер закрыт) — ctx тут ни при чём.
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	// Контекст shutdown НЕ должен наследоваться от ctx — тот уже отменён.
	shCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shCtx); err != nil {
		// Не дождались активных запросов — рвём соединения принудительно.
		_ = srv.Close()
		<-errCh
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Run — удобная обёртка для main: слушает addr и вызывает Serve.
func Run(ctx context.Context, addr string, h http.Handler) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return Serve(ctx, NewServer(h), ln, 10*time.Second)
}
