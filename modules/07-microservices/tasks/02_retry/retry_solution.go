//go:build solution

// Package retry — повторы с экспоненциальной задержкой и HTTP-идемпотентность.
package retry

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"
)

// permanentError помечает ошибку как неповторяемую.
type permanentError struct{ err error }

func (p *permanentError) Error() string { return p.err.Error() }
func (p *permanentError) Unwrap() error { return p.err }

// Permanent оборачивает err так, что Do прекращает повторы и возвращает err.
// Permanent(nil) == nil.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// Backoff возвращает паузу после неудачной попытки номер attempt (с 1):
// BaseDelay * Multiplier^(attempt-1), но не больше MaxDelay (если задан).
func Backoff(p Policy, attempt int) time.Duration {
	if attempt < 1 || p.BaseDelay <= 0 {
		return 0
	}
	mult := p.Multiplier
	if mult <= 0 {
		mult = 2
	}
	// Считаем во float64: целочисленное умножение быстро переполнится.
	d := float64(p.BaseDelay) * math.Pow(mult, float64(attempt-1))
	if p.MaxDelay > 0 && d > float64(p.MaxDelay) {
		return p.MaxDelay
	}
	if d >= math.MaxInt64 {
		return time.Duration(math.MaxInt64)
	}
	return time.Duration(d)
}

func defaultRetryable(err error) bool {
	return !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)
}

func defaultSleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Do вызывает fn до p.MaxAttempts раз (attempt начинается с 1).
func Do(ctx context.Context, p Policy, fn func(ctx context.Context, attempt int) error) error {
	maxAttempts := max(p.MaxAttempts, 1)
	retryable := p.Retryable
	if retryable == nil {
		retryable = defaultRetryable
	}
	sleep := p.Sleep
	if sleep == nil {
		sleep = defaultSleep
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	for attempt := 1; ; attempt++ {
		err := fn(ctx, attempt)
		if err == nil {
			return nil
		}
		var perm *permanentError
		if errors.As(err, &perm) {
			return perm.err
		}
		if ctx.Err() != nil || !retryable(err) {
			return err
		}
		if attempt >= maxAttempts {
			return fmt.Errorf("%w after %d attempts: %w", ErrExhausted, attempt, err)
		}

		delay := Backoff(p, attempt)
		if p.Jitter != nil {
			delay = p.Jitter(delay)
		}
		// Сервер лучше знает, когда ему полегчает.
		var ra RetryAfterer
		if errors.As(err, &ra) && ra.RetryAfter() > 0 {
			delay = ra.RetryAfter()
		}
		if serr := sleep(ctx, delay); serr != nil {
			return fmt.Errorf("retry: %w (last error: %w)", serr, err)
		}
	}
}

// NewKey генерирует случайный ключ идемпотентности (32 hex-символа).
func NewKey() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err) // crypto/rand не должен падать
	}
	return hex.EncodeToString(b[:])
}

// storedResponse — сохранённый ответ для повторной выдачи.
type storedResponse struct {
	fingerprint string
	done        bool
	status      int
	header      http.Header
	body        []byte
}

// recorder буферизует ответ обработчика.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header { return r.header }
func (r *recorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}
func (r *recorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(p)
}

// Idempotency — middleware, которое по заголовку Idempotency-Key гарантирует,
// что повтор запроса не выполнит обработчик второй раз.
func Idempotency(next http.Handler) http.Handler {
	var (
		mu    sync.Mutex
		store = map[string]*storedResponse{}
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get(HeaderIdempotencyKey)
		if key == "" {
			next.ServeHTTP(w, r)
			return
		}
		// Тело нужно и для отпечатка, и обработчику — читаем и подменяем.
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "cannot read body", http.StatusBadRequest)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		sum := sha256.Sum256(body)
		fp := r.Method + " " + r.URL.RequestURI() + " " + hex.EncodeToString(sum[:])

		mu.Lock()
		if s, ok := store[key]; ok {
			// Снимок читаем под mutex: поле done (и сохранённый ответ)
			// параллельно дописывает горутина первого запроса.
			snap := *s
			mu.Unlock()
			switch {
			case snap.fingerprint != fp:
				http.Error(w, "idempotency key reused with different request", http.StatusUnprocessableEntity)
			case !snap.done:
				http.Error(w, "request with this idempotency key is in progress", http.StatusConflict)
			default:
				for k, v := range snap.header {
					w.Header()[k] = append([]string(nil), v...)
				}
				w.Header().Set(HeaderReplayed, "true")
				w.WriteHeader(snap.status)
				_, _ = w.Write(snap.body)
			}
			return
		}
		entry := &storedResponse{fingerprint: fp}
		store[key] = entry // резервируем ключ: параллельный дубль получит 409
		mu.Unlock()

		rec := &recorder{header: http.Header{}}
		func() {
			defer func() {
				if p := recover(); p != nil {
					mu.Lock()
					delete(store, key)
					mu.Unlock()
					panic(p)
				}
			}()
			next.ServeHTTP(rec, r)
		}()
		if rec.status == 0 {
			rec.status = http.StatusOK
		}

		mu.Lock()
		if rec.status >= 500 {
			// Сбой сервера не запоминаем — клиент должен иметь шанс повторить.
			delete(store, key)
		} else {
			entry.done = true
			entry.status = rec.status
			entry.header = rec.header.Clone()
			entry.body = bytes.Clone(rec.body.Bytes())
		}
		mu.Unlock()

		for k, v := range rec.header {
			w.Header()[k] = v
		}
		w.WriteHeader(rec.status)
		_, _ = w.Write(rec.body.Bytes())
	})
}
