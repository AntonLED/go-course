//go:build !solution

// Package retry — повторы с экспоненциальной задержкой и таймауты с учётом context.
package retry

import (
	"context"
	"time"
)

// Backoff возвращает задержку перед попыткой номер failed+1 после failed
// неудачных попыток (failed >= 1): BaseDelay * Multiplier^(failed-1),
// но не больше MaxDelay (если MaxDelay > 0). Переполнение недопустимо.
func Backoff(p Policy, failed int) time.Duration {
	// TODO
	panic("TODO")
}

// Do вызывает f, пока она не вернёт nil, но не более p.Attempts раз,
// выдерживая между попытками паузу Backoff(p, i).
//
//   - Если ctx уже отменён — f не вызывается, возвращается ctx.Err().
//   - Успех — nil.
//   - f вернула Permanent(err) — повторов нет, возвращается сама err (без обёртки).
//   - Попытки кончились — ошибка, для которой errors.Is верно и для
//     ErrExhausted, и для последней ошибки f.
//   - ctx отменён во время паузы (или во время f) — возврат немедленно;
//     errors.Is верно и для ctx.Err(), и для последней ошибки f.
func Do(ctx context.Context, p Policy, f func(ctx context.Context) error) error {
	// TODO
	panic("TODO")
}

// DoWithTimeout выполняет f с контекстом, ограниченным таймаутом d.
// Возвращается не позже, чем истечёт d или отменится ctx, даже если f
// игнорирует контекст; в этом случае результат — нулевое T и ошибка контекста
// (context.DeadlineExceeded или context.Canceled). Горутина с f не должна
// утечь навсегда: когда f всё-таки завершится, она должна выйти.
func DoWithTimeout[T any](ctx context.Context, d time.Duration, f func(ctx context.Context) (T, error)) (T, error) {
	// TODO
	panic("TODO")
}
