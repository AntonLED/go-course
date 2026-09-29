package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"testing"
	"time"
)

func checkNoLeak(t *testing.T, base int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > base {
		if time.Now().After(deadline) {
			buf := make([]byte, 1<<16)
			n := runtime.Stack(buf, true)
			t.Fatalf("утечка горутин: было %d, стало %d\n%s", base, runtime.NumGoroutine(), buf[:n])
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestBackoff(t *testing.T) {
	ms := time.Millisecond
	tests := []struct {
		p      Policy
		failed int
		want   time.Duration
	}{
		{Policy{BaseDelay: 10 * ms, Multiplier: 2}, 1, 10 * ms},
		{Policy{BaseDelay: 10 * ms, Multiplier: 2}, 2, 20 * ms},
		{Policy{BaseDelay: 10 * ms, Multiplier: 2}, 4, 80 * ms},
		{Policy{BaseDelay: 10 * ms, Multiplier: 3}, 3, 90 * ms},
		{Policy{BaseDelay: 10 * ms}, 3, 40 * ms},                      // Multiplier 0 → 2
		{Policy{BaseDelay: 10 * ms, Multiplier: 0.5}, 3, 40 * ms},     // <= 1 → 2
		{Policy{BaseDelay: 10 * ms, MaxDelay: 50 * ms}, 4, 50 * ms},   // потолок
		{Policy{BaseDelay: 10 * ms, MaxDelay: 50 * ms}, 500, 50 * ms}, // без переполнения
		{Policy{BaseDelay: time.Second}, 200, time.Duration(math.MaxInt64)},
		{Policy{BaseDelay: 0}, 3, 0},
		{Policy{BaseDelay: 10 * ms}, 0, 0},
	}
	for _, tc := range tests {
		if got := Backoff(tc.p, tc.failed); got != tc.want {
			t.Errorf("Backoff(%+v, %d) = %v, ожидалось %v", tc.p, tc.failed, got, tc.want)
		}
	}
}

var errTemp = errors.New("temporary")

func fast(attempts int) Policy {
	return Policy{Attempts: attempts, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}
}

func TestDoSucceedsAfterRetries(t *testing.T) {
	calls := 0
	err := Do(context.Background(), fast(5), func(context.Context) error {
		calls++
		if calls < 3 {
			return errTemp
		}
		return nil
	})
	if err != nil || calls != 3 {
		t.Errorf("Do = %v после %d вызовов, ожидалось nil после 3", err, calls)
	}
}

func TestDoExhausted(t *testing.T) {
	for _, attempts := range []int{-1, 0, 1, 4} {
		calls := 0
		err := Do(context.Background(), fast(attempts), func(context.Context) error {
			calls++
			return fmt.Errorf("call %d: %w", calls, errTemp)
		})
		want := max(attempts, 1)
		if calls != want {
			t.Errorf("Attempts=%d: f вызвана %d раз, ожидалось %d", attempts, calls, want)
		}
		if !errors.Is(err, ErrExhausted) || !errors.Is(err, errTemp) {
			t.Errorf("Attempts=%d: err = %v, ожидалась обёртка ErrExhausted и последней ошибки", attempts, err)
		}
	}
}

func TestDoPermanent(t *testing.T) {
	if Permanent(nil) != nil {
		t.Errorf("Permanent(nil) должен быть nil")
	}
	base := errors.New("bad request")
	calls := 0
	err := Do(context.Background(), fast(10), func(context.Context) error {
		calls++
		return Permanent(base)
	})
	if calls != 1 {
		t.Errorf("при Permanent-ошибке f вызвана %d раз, ожидалось 1", calls)
	}
	if err != base {
		t.Errorf("Do вернул %v (%T), ожидалась исходная ошибка без обёртки", err, err)
	}
}

func TestDoAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := Do(ctx, fast(3), func(context.Context) error { called = true; return nil })
	if called {
		t.Errorf("f не должна вызываться с уже отменённым контекстом")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Do = %v, ожидалось context.Canceled", err)
	}
}

// Пауза длиной в час должна прерываться отменой контекста.
func TestDoCancelDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := Policy{Attempts: 5, BaseDelay: time.Hour}
	calls := 0
	done := make(chan error, 1)
	go func() {
		done <- Do(ctx, p, func(context.Context) error {
			calls++
			cancel() // отмена сразу после первой неудачи
			return errTemp
		})
	}()
	select {
	case err := <-done:
		if calls != 1 {
			t.Errorf("f вызвана %d раз, ожидалось 1", calls)
		}
		if !errors.Is(err, context.Canceled) || !errors.Is(err, errTemp) {
			t.Errorf("Do = %v, ожидалось, что errors.Is сработает и для context.Canceled, и для последней ошибки", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Do не прервался по отмене контекста во время паузы (используете time.Sleep?)")
	}
}

func TestDoDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := Do(ctx, Policy{Attempts: 100, BaseDelay: 20 * time.Millisecond}, func(context.Context) error {
		return errTemp
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Do = %v, ожидалось context.DeadlineExceeded", err)
	}
	if el := time.Since(start); el > 2*time.Second {
		t.Errorf("Do работал %v при дедлайне 50мс", el)
	}
}

func TestDoWithTimeoutSuccess(t *testing.T) {
	base := runtime.NumGoroutine()
	v, err := DoWithTimeout(context.Background(), time.Second, func(ctx context.Context) (string, error) {
		if _, ok := ctx.Deadline(); !ok {
			return "", errors.New("у контекста f нет дедлайна")
		}
		return "ok", nil
	})
	if v != "ok" || err != nil {
		t.Errorf("DoWithTimeout = (%q, %v), ожидалось (\"ok\", nil)", v, err)
	}
	wantErr := errors.New("f failed")
	if _, err := DoWithTimeout(context.Background(), time.Second, func(context.Context) (int, error) {
		return 0, wantErr
	}); err != wantErr {
		t.Errorf("DoWithTimeout вернул %v, ожидалась ошибка f", err)
	}
	checkNoLeak(t, base)
}

// f игнорирует контекст: DoWithTimeout всё равно обязан вернуться по таймауту.
func TestDoWithTimeoutIgnoringF(t *testing.T) {
	base := runtime.NumGoroutine()
	release := make(chan struct{})
	start := time.Now()
	v, err := DoWithTimeout(context.Background(), 30*time.Millisecond, func(context.Context) (int, error) {
		<-release
		return 42, nil
	})
	if el := time.Since(start); el > 2*time.Second {
		t.Errorf("DoWithTimeout вернулся через %v при таймауте 30мс", el)
	}
	if v != 0 || !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("DoWithTimeout = (%d, %v), ожидалось (0, context.DeadlineExceeded)", v, err)
	}
	close(release) // f завершается — её горутина не должна зависнуть на отправке
	checkNoLeak(t, base)
}

func TestDoWithTimeoutParentCanceled(t *testing.T) {
	base := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	go func() { <-started; cancel() }()
	_, err := DoWithTimeout(ctx, time.Hour, func(ctx context.Context) (int, error) {
		close(started)
		<-ctx.Done() // f уважает контекст
		return 0, ctx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("DoWithTimeout = %v, ожидалось context.Canceled", err)
	}
	checkNoLeak(t, base)
}
