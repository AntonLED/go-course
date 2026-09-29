//go:build solution

// Package retry — повторы с экспоненциальной задержкой и таймауты с учётом context.
package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

// Backoff возвращает задержку после failed неудачных попыток.
func Backoff(p Policy, failed int) time.Duration {
	if failed < 1 || p.BaseDelay <= 0 {
		return 0
	}
	mult := p.Multiplier
	if mult <= 1 {
		mult = 2
	}
	// Считаем во float64: так нет целочисленного переполнения,
	// а результат больше MaxInt64 просто упирается в потолок.
	d := float64(p.BaseDelay) * math.Pow(mult, float64(failed-1))
	limit := float64(math.MaxInt64)
	if p.MaxDelay > 0 {
		limit = float64(p.MaxDelay)
	}
	if d >= limit || math.IsInf(d, 0) || math.IsNaN(d) {
		if p.MaxDelay > 0 {
			return p.MaxDelay
		}
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(d)
}

// Do повторяет f с экспоненциальной задержкой.
func Do(ctx context.Context, p Policy, f func(ctx context.Context) error) error {
	attempts := max(p.Attempts, 1)
	if err := ctx.Err(); err != nil {
		return err
	}

	var last error
	for i := 1; ; i++ {
		last = f(ctx)
		if last == nil {
			return nil
		}
		var perm *permanentError
		if errors.As(last, &perm) {
			return perm.err
		}
		if i >= attempts {
			return fmt.Errorf("%w after %d attempts: %w", ErrExhausted, attempts, last)
		}
		if err := sleep(ctx, Backoff(p, i)); err != nil {
			// Два %w (Go 1.20+): errors.Is сработает и для ctx.Err(), и для last.
			return fmt.Errorf("%w; last error: %w", err, last)
		}
	}
}

// sleep ждёт d или отмены ctx — в отличие от time.Sleep, прерываемо.
func sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// DoWithTimeout выполняет f с таймаутом d.
func DoWithTimeout[T any](ctx context.Context, d time.Duration, f func(ctx context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(ctx, d)
	defer cancel() // освобождаем таймер и ресурсы контекста в любом случае

	type result struct {
		v   T
		err error
	}
	// Буфер 1 обязателен: если мы уйдём по таймауту, горутина всё равно
	// сможет положить результат и завершиться, а не висеть вечно на отправке.
	ch := make(chan result, 1)
	go func() {
		v, err := f(ctx)
		ch <- result{v, err}
	}()

	select {
	case r := <-ch:
		return r.v, r.err
	case <-ctx.Done():
		var zero T
		return zero, ctx.Err()
	}
}
