//go:build solution

// Package breaker — circuit breaker (closed / open / half-open) с инжектируемыми часами.
package breaker

import (
	"sync"
	"time"
)

// Breaker — потокобезопасный circuit breaker.
type Breaker struct {
	s Settings

	mu         sync.Mutex
	state      State
	generation uint64    // растёт при каждой смене состояния
	failures   int       // подряд идущие неудачи в closed
	successes  int       // подряд идущие успехи в half-open
	inflight   int       // активные пробные вызовы в half-open
	openedAt   time.Time // момент последнего размыкания
}

// New создаёт Breaker в состоянии closed.
func New(s Settings) *Breaker {
	if s.FailureThreshold <= 0 {
		s.FailureThreshold = 5
	}
	if s.OpenTimeout <= 0 {
		s.OpenTimeout = 30 * time.Second
	}
	if s.HalfOpenMaxCalls <= 0 {
		s.HalfOpenMaxCalls = 1
	}
	if s.SuccessThreshold <= 0 {
		s.SuccessThreshold = 1
	}
	if s.IsFailure == nil {
		s.IsFailure = func(err error) bool { return err != nil }
	}
	if s.Now == nil {
		s.Now = time.Now
	}
	return &Breaker{s: s}
}

// State возвращает текущее состояние с учётом истёкшего OpenTimeout.
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refreshLocked(b.s.Now())
	return b.state
}

// Execute выполняет fn, если цепь это позволяет, и учитывает результат.
// Паника в fn считается неудачей и пробрасывается дальше.
func (b *Breaker) Execute(fn func() error) (err error) {
	gen, err := b.before()
	if err != nil {
		return err
	}
	done := false
	defer func() {
		if !done { // fn запаниковала
			b.after(gen, true)
		}
	}()
	err = fn()
	done = true
	b.after(gen, b.s.IsFailure(err))
	return err
}

// before решает, можно ли выполнить вызов, и возвращает поколение,
// в котором вызов стартовал.
func (b *Breaker) before() (uint64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refreshLocked(b.s.Now())
	switch b.state {
	case StateOpen:
		return 0, ErrOpen
	case StateHalfOpen:
		if b.inflight >= b.s.HalfOpenMaxCalls {
			return 0, ErrTooManyRequests
		}
		b.inflight++
	}
	return b.generation, nil
}

// after учитывает результат вызова. Результаты вызовов из «старого»
// поколения игнорируются: пока вызов выполнялся, состояние уже сменилось.
func (b *Breaker) after(gen uint64, failed bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.s.Now()
	if gen != b.generation {
		return
	}
	switch b.state {
	case StateClosed:
		if failed {
			b.failures++
			if b.failures >= b.s.FailureThreshold {
				b.setStateLocked(StateOpen, now)
			}
		} else {
			b.failures = 0
		}
	case StateHalfOpen:
		b.inflight--
		if failed {
			b.setStateLocked(StateOpen, now)
			return
		}
		b.successes++
		if b.successes >= b.s.SuccessThreshold {
			b.setStateLocked(StateClosed, now)
		}
	}
}

// refreshLocked выполняет «ленивый» переход open → half-open по времени.
func (b *Breaker) refreshLocked(now time.Time) {
	if b.state == StateOpen && !now.Before(b.openedAt.Add(b.s.OpenTimeout)) {
		b.setStateLocked(StateHalfOpen, now)
	}
}

func (b *Breaker) setStateLocked(to State, now time.Time) {
	from := b.state
	if from == to {
		return
	}
	b.state = to
	b.generation++
	b.failures, b.successes, b.inflight = 0, 0, 0
	if to == StateOpen {
		b.openedAt = now
	}
	if b.s.OnStateChange != nil {
		b.s.OnStateChange(from, to)
	}
}
