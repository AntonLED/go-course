//go:build solution

package retry

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

func (c *Client) defaults() (hc *http.Client, attempts int, base, maxD time.Duration,
	sleep func(context.Context, time.Duration) error, now func() time.Time) {
	hc = c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	attempts = c.MaxAttempts
	if attempts <= 0 {
		attempts = 3
	}
	base = c.BaseDelay
	if base <= 0 {
		base = 100 * time.Millisecond
	}
	maxD = c.MaxDelay
	if maxD <= 0 {
		maxD = 5 * time.Second
	}
	sleep = c.Sleep
	if sleep == nil {
		sleep = sleepCtx
	}
	now = c.Now
	if now == nil {
		now = time.Now
	}
	return
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// idempotent — можно ли безопасно повторить запрос.
func idempotent(req *http.Request) bool {
	switch req.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace,
		http.MethodPut, http.MethodDelete:
		return true
	}
	// POST можно повторять, если сервер дедуплицирует по ключу.
	return req.Header.Get("Idempotency-Key") != ""
}

func retryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// retryAfter разбирает Retry-After: секунды или HTTP-дата.
func retryAfter(h string, now time.Time) (time.Duration, bool) {
	if h == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(h); err == nil {
		if secs < 0 {
			return 0, false
		}
		return time.Duration(secs) * time.Second, true
	}
	if t, err := http.ParseTime(h); err == nil {
		d := t.Sub(now)
		if d < 0 {
			d = 0
		}
		return d, true
	}
	return 0, false
}

// backoff возвращает base·2^n, но не больше maxD. Удваиваем в цикле, а не сдвигом
// base<<n: при большом n сдвиг переполнит int64 и даст отрицательную задержку.
func backoff(base, maxD time.Duration, n int) time.Duration {
	d := base
	for i := 0; i < n && d < maxD; i++ {
		if d > maxD/2 {
			return maxD
		}
		d *= 2
	}
	return min(d, maxD)
}

// drain дочитывает (немного) и закрывает тело — иначе соединение не вернётся в пул.
func drain(resp *http.Response) {
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	_ = resp.Body.Close()
}

// Do выполняет запрос с повторами.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	hc, attempts, base, maxD, sleep, now := c.defaults()
	ctx := req.Context()

	canRetry := idempotent(req) &&
		(req.Body == nil || req.Body == http.NoBody || req.GetBody != nil)
	if !canRetry {
		attempts = 1
	}

	var lastErr error
	for attempt := 1; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r := req
		if attempt > 1 && req.GetBody != nil {
			// Тело — это поток, после первой попытки он вычитан. Берём свежую копию.
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			r = req.Clone(ctx)
			r.Body = body
		}

		resp, err := hc.Do(r)
		delay := time.Duration(-1) // -1 — «Retry-After не задан»
		switch {
		case err != nil:
			if ctx.Err() != nil { // отмену не ретраим
				return nil, ctx.Err()
			}
			lastErr = err
		case retryableStatus(resp.StatusCode):
			if attempt == attempts {
				return resp, nil // отдаём последний ответ как есть — пусть вызывающий решает
			}
			if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable {
				if d, ok := retryAfter(resp.Header.Get("Retry-After"), now()); ok {
					delay = min(d, maxD)
				}
			}
			drain(resp)
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		default:
			return resp, nil
		}

		if attempt == attempts {
			return nil, fmt.Errorf("после %d попыток: %w", attempt, lastErr)
		}
		if delay < 0 {
			// Экспоненциальный backoff: base, 2·base, 4·base… не больше maxD.
			// В проде добавляют jitter, чтобы клиенты не били синхронно.
			delay = backoff(base, maxD, attempt-1)
		}
		if err := sleep(ctx, delay); err != nil {
			return nil, err
		}
	}
}

// RoundTrip добавляет заголовки и логирует.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	// Контракт RoundTripper: исходный запрос менять нельзя — клонируем.
	r := req.Clone(req.Context())
	for k, vs := range t.Header {
		if r.Header.Get(k) == "" {
			for _, v := range vs {
				r.Header.Add(k, v)
			}
		}
	}
	start := time.Now()
	resp, err := base.RoundTrip(r)
	if t.Logf != nil {
		if err != nil {
			t.Logf("%s %s -> error: %v", req.Method, req.URL, err)
		} else {
			t.Logf("%s %s -> %d (%s)", req.Method, req.URL, resp.StatusCode, time.Since(start).Round(time.Millisecond))
		}
	}
	return resp, err
}
