package retry

import (
	"context"
	"net/http"
	"time"
)

// Client — HTTP-клиент с повторами. Нулевые поля заменяются значениями по умолчанию.
type Client struct {
	HTTP        *http.Client  // nil → &http.Client{Timeout: 10 * time.Second} (НЕ DefaultClient: у него нет таймаута)
	MaxAttempts int           // всего попыток, включая первую; ≤0 → 3
	BaseDelay   time.Duration // ≤0 → 100ms
	MaxDelay    time.Duration // ≤0 → 5s

	// Sleep ждёт d или отмены ctx (тогда возвращает ctx.Err()).
	// nil → реализация на time.Timer. В тестах подменяется, чтобы не спать по-настоящему.
	Sleep func(ctx context.Context, d time.Duration) error
	// Now нужен для Retry-After в формате HTTP-даты. nil → time.Now.
	Now func() time.Time
}

// Transport — RoundTripper-middleware: добавляет заголовки и логирует запросы.
type Transport struct {
	Base   http.RoundTripper                // nil → http.DefaultTransport
	Header http.Header                      // добавляются, если в запросе такого заголовка ещё нет
	Logf   func(format string, args ...any) // nil → не логировать
}
