package retry

import (
	"context"
	"errors"
	"time"
)

// ErrExhausted — все попытки израсходованы. Итоговая ошибка Do должна
// удовлетворять и errors.Is(err, ErrExhausted), и errors.Is(err, <последняя ошибка>).
var ErrExhausted = errors.New("retry: attempts exhausted")

// Заголовки протокола идемпотентности.
const (
	HeaderIdempotencyKey = "Idempotency-Key"
	HeaderReplayed       = "Idempotent-Replayed"
)

// Policy описывает стратегию повторов. Нулевые значения — значения по умолчанию.
type Policy struct {
	// MaxAttempts — общее число попыток, включая первую (<= 0 → 1).
	MaxAttempts int
	// BaseDelay — пауза после первой неудачной попытки.
	BaseDelay time.Duration
	// MaxDelay — верхняя граница паузы (0 — без ограничения).
	MaxDelay time.Duration
	// Multiplier — множитель экспоненты (<= 0 → 2).
	Multiplier float64
	// Jitter преобразует рассчитанную паузу (например, full jitter: rand в [0, d)).
	// nil — без джиттера (детерминированно).
	Jitter func(d time.Duration) time.Duration
	// Retryable решает, стоит ли повторять ошибку. nil — повторяем всё,
	// кроме context.Canceled / context.DeadlineExceeded.
	Retryable func(err error) bool
	// Sleep ждёт d или отмены ctx (для тестов). nil — настоящий таймер.
	Sleep func(ctx context.Context, d time.Duration) error
}

// RetryAfterer может реализовать ошибка, если сервер подсказал, сколько
// ждать (например, из заголовка Retry-After у ответа 429/503).
type RetryAfterer interface {
	RetryAfter() time.Duration
}
