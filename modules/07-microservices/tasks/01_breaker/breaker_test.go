package breaker

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var errBoom = errors.New("boom")

// fakeClock — управляемые часы.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newClock() *fakeClock { return &fakeClock{now: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func fail() error { return errBoom }
func ok() error   { return nil }

func TestDefaults(t *testing.T) {
	b := New(Settings{})
	if got := b.State(); got != StateClosed {
		t.Fatalf("новый breaker в состоянии %v, ожидалось closed", got)
	}
	// по умолчанию FailureThreshold = 5
	for i := 0; i < 4; i++ {
		if err := b.Execute(fail); !errors.Is(err, errBoom) {
			t.Fatalf("Execute вернул %v, ожидалась исходная ошибка", err)
		}
	}
	if got := b.State(); got != StateClosed {
		t.Fatalf("после 4 неудач состояние %v, ожидалось closed (порог по умолчанию 5)", got)
	}
	_ = b.Execute(fail)
	if got := b.State(); got != StateOpen {
		t.Fatalf("после 5 неудач состояние %v, ожидалось open", got)
	}
}

func TestOpensAfterConsecutiveFailures(t *testing.T) {
	clk := newClock()
	b := New(Settings{FailureThreshold: 3, OpenTimeout: time.Second, Now: clk.Now})

	_ = b.Execute(fail)
	_ = b.Execute(fail)
	_ = b.Execute(ok) // успех сбрасывает счётчик ПОДРЯД идущих неудач
	_ = b.Execute(fail)
	_ = b.Execute(fail)
	if got := b.State(); got != StateClosed {
		t.Fatalf("состояние %v, ожидалось closed: успех должен сбрасывать счётчик", got)
	}
	_ = b.Execute(fail)
	if got := b.State(); got != StateOpen {
		t.Fatalf("состояние %v, ожидалось open после 3 неудач подряд", got)
	}

	called := false
	err := b.Execute(func() error { called = true; return nil })
	if !errors.Is(err, ErrOpen) {
		t.Fatalf("в open Execute вернул %v, ожидался ErrOpen", err)
	}
	if called {
		t.Fatal("в open функция не должна вызываться")
	}
}

func TestHalfOpenTransitions(t *testing.T) {
	clk := newClock()
	b := New(Settings{FailureThreshold: 1, OpenTimeout: 10 * time.Second, Now: clk.Now})

	_ = b.Execute(fail)
	clk.Advance(9 * time.Second)
	if got := b.State(); got != StateOpen {
		t.Fatalf("через 9s состояние %v, ожидалось open", got)
	}
	clk.Advance(time.Second)
	if got := b.State(); got != StateHalfOpen {
		t.Fatalf("через 10s состояние %v, ожидалось half-open", got)
	}

	// неудачный пробный вызов снова размыкает цепь и перезапускает таймер
	_ = b.Execute(fail)
	if got := b.State(); got != StateOpen {
		t.Fatalf("после неудачи в half-open состояние %v, ожидалось open", got)
	}
	clk.Advance(5 * time.Second)
	if got := b.State(); got != StateOpen {
		t.Fatalf("таймер open должен перезапуститься, а состояние %v", got)
	}
	clk.Advance(5 * time.Second)
	if err := b.Execute(ok); err != nil {
		t.Fatalf("пробный вызов вернул %v", err)
	}
	if got := b.State(); got != StateClosed {
		t.Fatalf("после успешной пробы состояние %v, ожидалось closed", got)
	}
}

func TestSuccessThreshold(t *testing.T) {
	clk := newClock()
	b := New(Settings{FailureThreshold: 1, OpenTimeout: time.Second, SuccessThreshold: 3, Now: clk.Now})
	_ = b.Execute(fail)
	clk.Advance(time.Second)
	for i := 1; i <= 2; i++ {
		_ = b.Execute(ok)
		if got := b.State(); got != StateHalfOpen {
			t.Fatalf("после %d успехов состояние %v, ожидалось half-open", i, got)
		}
	}
	_ = b.Execute(ok)
	if got := b.State(); got != StateClosed {
		t.Fatalf("после 3 успехов состояние %v, ожидалось closed", got)
	}
	// после замыкания счётчик неудач начинается с нуля
	_ = b.Execute(ok)
	if got := b.State(); got != StateClosed {
		t.Fatalf("состояние %v, ожидалось closed", got)
	}
}

func TestHalfOpenLimitsConcurrentProbes(t *testing.T) {
	clk := newClock()
	b := New(Settings{FailureThreshold: 1, OpenTimeout: time.Second, HalfOpenMaxCalls: 1, Now: clk.Now})
	_ = b.Execute(fail)
	clk.Advance(time.Second)

	var innerErr error
	err := b.Execute(func() error {
		// пока идёт пробный вызов, второй должен быть отбит
		innerErr = b.Execute(ok)
		return nil
	})
	if err != nil {
		t.Fatalf("пробный вызов вернул %v", err)
	}
	if !errors.Is(innerErr, ErrTooManyRequests) {
		t.Fatalf("второй вызов в half-open вернул %v, ожидался ErrTooManyRequests", innerErr)
	}
	if got := b.State(); got != StateClosed {
		t.Fatalf("состояние %v, ожидалось closed", got)
	}
}

func TestStaleGenerationIgnored(t *testing.T) {
	clk := newClock()
	b := New(Settings{FailureThreshold: 2, OpenTimeout: time.Minute, Now: clk.Now})

	// Долгий вызов стартует в closed; пока он идёт, другие вызовы размыкают цепь.
	// Его запоздалый успех НЕ должен замкнуть цепь обратно.
	err := b.Execute(func() error {
		_ = b.Execute(fail)
		_ = b.Execute(fail)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute вернул %v", err)
	}
	if got := b.State(); got != StateOpen {
		t.Fatalf("состояние %v, ожидалось open: результат вызова из старого поколения должен игнорироваться", got)
	}

	// Долгий вызов стартует в closed; пока он идёт, цепь размыкается и
	// переходит в half-open. Его запоздалый успех НЕ должен засчитываться
	// как успешная проба и замыкать цепь.
	clk2 := newClock()
	b2 := New(Settings{FailureThreshold: 1, OpenTimeout: time.Minute, Now: clk2.Now})
	err = b2.Execute(func() error {
		_ = b2.Execute(fail) // closed → open
		clk2.Advance(time.Minute)
		if got := b2.State(); got != StateHalfOpen { // open → half-open
			t.Errorf("внутри долгого вызова состояние %v, ожидалось half-open", got)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Execute вернул %v", err)
	}
	if got := b2.State(); got != StateHalfOpen {
		t.Fatalf("состояние %v, ожидалось half-open: успех вызова, начатого в closed, не должен замыкать цепь", got)
	}
	// настоящая проба по-прежнему доступна
	if err := b2.Execute(ok); err != nil {
		t.Fatalf("пробный вызов вернул %v", err)
	}
	if got := b2.State(); got != StateClosed {
		t.Fatalf("после успешной пробы состояние %v, ожидалось closed", got)
	}
}

func TestIsFailure(t *testing.T) {
	errNotFound := errors.New("not found")
	b := New(Settings{
		FailureThreshold: 1,
		IsFailure:        func(err error) bool { return err != nil && !errors.Is(err, errNotFound) },
	})
	for i := 0; i < 10; i++ {
		if err := b.Execute(func() error { return errNotFound }); !errors.Is(err, errNotFound) {
			t.Fatalf("Execute вернул %v, ожидалась бизнес-ошибка как есть", err)
		}
	}
	if got := b.State(); got != StateClosed {
		t.Fatalf("бизнес-ошибки не должны размыкать цепь, состояние %v", got)
	}
}

func TestPanicCountsAsFailure(t *testing.T) {
	b := New(Settings{FailureThreshold: 1})
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("паника из fn должна пробрасываться наружу")
			}
		}()
		_ = b.Execute(func() error { panic("oops") })
	}()
	if got := b.State(); got != StateOpen {
		t.Fatalf("паника должна считаться неудачей, состояние %v", got)
	}
}

func TestOnStateChange(t *testing.T) {
	clk := newClock()
	var log []string
	b := New(Settings{
		FailureThreshold: 1, OpenTimeout: time.Second, Now: clk.Now,
		OnStateChange: func(from, to State) { log = append(log, from.String()+"->"+to.String()) },
	})
	_ = b.Execute(fail)
	clk.Advance(time.Second)
	_ = b.Execute(ok)
	want := []string{"closed->open", "open->half-open", "half-open->closed"}
	if len(log) != len(want) {
		t.Fatalf("переходы %v, ожидалось %v", log, want)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("переходы %v, ожидалось %v", log, want)
		}
	}
}

func TestConcurrentUse(t *testing.T) {
	clk := newClock()
	b := New(Settings{FailureThreshold: 1000000, Now: clk.Now})
	var wg sync.WaitGroup
	var calls atomic.Int64
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_ = b.Execute(func() error {
					calls.Add(1)
					if (g+i)%2 == 0 {
						return errBoom
					}
					return nil
				})
				_ = b.State()
			}
		}(g)
	}
	wg.Wait()
	if calls.Load() != 16*200 {
		t.Fatalf("выполнено %d вызовов, ожидалось %d", calls.Load(), 16*200)
	}
}

func BenchmarkExecute(b *testing.B) {
	br := New(Settings{})
	for i := 0; i < b.N; i++ {
		_ = br.Execute(ok)
	}
}
