//go:build solution

package clock

import (
	"context"
	"sort"
	"sync"
	"time"
)

type waiter struct {
	deadline time.Time
	ch       chan time.Time
}

// Fake — управляемые часы для тестов.
type Fake struct {
	mu      sync.Mutex
	cond    *sync.Cond // сигналит об изменении числа ожидающих (для BlockUntil)
	now     time.Time
	waiters []waiter
}

// NewFake создаёт часы, показывающие start.
func NewFake(start time.Time) *Fake {
	f := &Fake{now: start}
	f.cond = sync.NewCond(&f.mu)
	return f
}

func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *Fake) After(d time.Duration) <-chan time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	// Буфер 1: Advance никогда не блокируется, даже если получатель ушёл.
	ch := make(chan time.Time, 1)
	if d <= 0 {
		ch <- f.now
		return ch
	}
	f.waiters = append(f.waiters, waiter{deadline: f.now.Add(d), ch: ch})
	f.cond.Broadcast()
	return ch
}

func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
	// Стабильная сортировка: равные дедлайны — в порядке создания.
	sort.SliceStable(f.waiters, func(i, j int) bool { return f.waiters[i].deadline.Before(f.waiters[j].deadline) })
	rest := f.waiters[:0]
	for _, w := range f.waiters {
		if !w.deadline.After(f.now) {
			w.ch <- w.deadline
		} else {
			rest = append(rest, w)
		}
	}
	clear(f.waiters[len(rest):]) // не держим ссылки на каналы
	f.waiters = rest
	f.cond.Broadcast()
}

func (f *Fake) Waiters() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.waiters)
}

func (f *Fake) BlockUntil(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for len(f.waiters) < n {
		f.cond.Wait()
	}
}

// ---------- TTLCache ----------

type entry[V any] struct {
	v       V
	expires time.Time
}

type TTLCache[K comparable, V any] struct {
	mu    sync.Mutex
	clock Clock
	ttl   time.Duration
	m     map[K]entry[V]
}

func NewTTLCache[K comparable, V any](c Clock, ttl time.Duration) *TTLCache[K, V] {
	return &TTLCache[K, V]{clock: c, ttl: ttl, m: make(map[K]entry[V])}
}

func (c *TTLCache[K, V]) Set(k K, v V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[k] = entry[V]{v: v, expires: c.clock.Now().Add(c.ttl)}
}

func (c *TTLCache[K, V]) Get(k K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[k]
	if !ok {
		var zero V
		return zero, false
	}
	if !c.clock.Now().Before(e.expires) {
		delete(c.m, k)
		var zero V
		return zero, false
	}
	return e.v, true
}

func (c *TTLCache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.clock.Now()
	for k, e := range c.m { // удаление во время range по map разрешено
		if !now.Before(e.expires) {
			delete(c.m, k)
		}
	}
	return len(c.m)
}

// ---------- Poll ----------

func Poll(ctx context.Context, c Clock, interval time.Duration, cond func() (bool, error)) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		ok, err := cond()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		select {
		case <-c.After(interval):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
