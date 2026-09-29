//go:build !solution

// Package retry — повторы с экспоненциальной задержкой и HTTP-идемпотентность.
package retry

import (
	"context"
	"net/http"
	"time"
)

// Permanent оборачивает err так, что Do прекращает повторы и возвращает
// исходную err (не обёртку). Permanent(nil) == nil.
func Permanent(err error) error {
	// TODO: реализуйте (свой тип с методом Unwrap)
	return err
}

// Backoff возвращает паузу после неудачной попытки номер attempt (с 1):
// BaseDelay * Multiplier^(attempt-1), не больше MaxDelay (если он > 0).
// Multiplier <= 0 означает 2. Осторожно с переполнением.
func Backoff(p Policy, attempt int) time.Duration {
	// TODO: реализуйте
	return 0
}

// Do вызывает fn до p.MaxAttempts раз (attempt начинается с 1).
//
//   - ctx уже отменён до старта → вернуть ctx.Err(), fn не вызывать;
//   - успех → nil;
//   - Permanent(err) → вернуть err без повторов;
//   - ctx отменён или ошибка не Retryable → вернуть её без повторов;
//   - попытки кончились → ошибка, для которой errors.Is верно и для ErrExhausted,
//     и для последней ошибки (подсказка: fmt.Errorf с двумя %w);
//   - пауза = Backoff (+ Jitter); если ошибка реализует RetryAfterer с
//     положительным значением — ждать ровно столько;
//   - ctx отменён во время паузы → вернуть ошибку, которая Is(ctx.Err()) и Is(последняя ошибка).
func Do(ctx context.Context, p Policy, fn func(ctx context.Context, attempt int) error) error {
	// TODO: реализуйте
	panic("TODO")
}

// NewKey генерирует случайный ключ идемпотентности: 16 байт из crypto/rand в hex.
func NewKey() string {
	// TODO: реализуйте
	return ""
}

// Idempotency — middleware с поддержкой заголовка Idempotency-Key.
//
//   - нет заголовка → просто next;
//   - первый запрос с ключом: выполнить next, запомнить статус/заголовки/тело
//     (но НЕ запоминать ответы 5xx — клиент должен иметь возможность повторить);
//   - повтор с тем же ключом и тем же запросом (метод + URI + тело) →
//     отдать сохранённый ответ + заголовок Idempotent-Replayed: true, next не вызывать;
//   - тот же ключ, но другой запрос → 422;
//   - тот же ключ, пока первый запрос ещё выполняется → 409.
func Idempotency(next http.Handler) http.Handler {
	// TODO: реализуйте
	return next
}
