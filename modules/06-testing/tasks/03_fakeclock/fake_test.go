package clock

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var t0 = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

// Компилятор проверит, что Fake и Real реализуют Clock.
var (
	_ Clock = (*Fake)(nil)
	_ Clock = Real{}
)

// recv ждёт значение из канала не дольше реальной секунды — чтобы сломанная реализация
// давала понятную ошибку, а не зависший тест.
func recv(t *testing.T, ch <-chan time.Time) (time.Time, bool) {
	t.Helper()
	if ch == nil {
		t.Fatal("After вернул nil-канал")
	}
	select {
	case v := <-ch:
		return v, true
	case <-time.After(time.Second):
		return time.Time{}, false
	}
}

func fired(ch <-chan time.Time) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestFakeNowAdvance(t *testing.T) {
	f := NewFake(t0)
	if !f.Now().Equal(t0) {
		t.Fatalf("Now() = %v, ожидалось %v", f.Now(), t0)
	}
	f.Advance(90 * time.Minute)
	if want := t0.Add(90 * time.Minute); !f.Now().Equal(want) {
		t.Errorf("после Advance Now() = %v, ожидалось %v", f.Now(), want)
	}
}

func TestFakeAfter(t *testing.T) {
	f := NewFake(t0)
	if v, ok := recv(t, f.After(0)); !ok || !v.Equal(t0) {
		t.Errorf("After(0) должен сработать сразу со временем Now: %v %v", v, ok)
	}
	if _, ok := recv(t, f.After(-time.Second)); !ok {
		t.Error("After(<0) должен сработать сразу")
	}

	ch := f.After(5 * time.Second)
	if f.Waiters() != 1 {
		t.Errorf("Waiters() = %d, ожидалось 1", f.Waiters())
	}
	f.Advance(4 * time.Second)
	if fired(ch) {
		t.Fatal("таймер на 5s сработал через 4s")
	}
	f.Advance(time.Second)
	v, ok := recv(t, ch)
	if !ok {
		t.Fatal("таймер на 5s не сработал через 5s")
	}
	if want := t0.Add(5 * time.Second); !v.Equal(want) {
		t.Errorf("в канал пришло %v, ожидался момент срабатывания %v", v, want)
	}
	if f.Waiters() != 0 {
		t.Errorf("после срабатывания Waiters() = %d", f.Waiters())
	}
}

func TestFakeManyTimersOneAdvance(t *testing.T) {
	f := NewFake(t0)
	c3 := f.After(3 * time.Second)
	c1 := f.After(1 * time.Second)
	c2 := f.After(2 * time.Second)
	c9 := f.After(9 * time.Second)
	f.Advance(5 * time.Second) // «проскочили» три таймера за раз
	for i, ch := range []<-chan time.Time{c1, c2, c3} {
		v, ok := recv(t, ch)
		if want := t0.Add(time.Duration(i+1) * time.Second); !ok || !v.Equal(want) {
			t.Errorf("таймер %ds: %v %v, ожидалось %v", i+1, v, ok, want)
		}
	}
	if fired(c9) {
		t.Error("таймер 9s сработал раньше времени")
	}
	if f.Waiters() != 1 {
		t.Errorf("Waiters() = %d, ожидалось 1", f.Waiters())
	}
	f.Advance(10 * time.Second)
	if _, ok := recv(t, c9); !ok {
		t.Error("таймер 9s не сработал")
	}
	// Повторный Advance не должен повторно слать в сработавшие каналы (и не блокироваться).
	done := make(chan struct{})
	go func() { f.Advance(time.Hour); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Advance заблокировался")
	}
	if fired(c1) {
		t.Error("сработавший таймер получил второе значение")
	}
}

func TestFakeAdvanceDoesNotBlockOnAbandonedTimers(t *testing.T) {
	f := NewFake(t0)
	for i := 0; i < 100; i++ {
		f.After(time.Second) // никто не читает
	}
	done := make(chan struct{})
	go func() { f.Advance(time.Minute); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Advance блокируется, если из канала никто не читает (нужен буфер 1)")
	}
}

func TestFakeBlockUntilAndConcurrency(t *testing.T) {
	f := NewFake(t0)
	const n = 20
	var got atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-f.After(time.Minute)
			got.Add(1)
		}()
	}
	blocked := make(chan struct{})
	go func() { f.BlockUntil(n); close(blocked) }()
	select {
	case <-blocked:
	case <-time.After(2 * time.Second):
		t.Fatalf("BlockUntil(%d) не вернулся, Waiters() = %d", n, f.Waiters())
	}
	if f.Waiters() != n {
		t.Fatalf("Waiters() = %d, ожидалось %d", f.Waiters(), n)
	}
	f.Advance(time.Minute)
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("проснулось %d из %d горутин", got.Load(), n)
	}
}

func TestTTLCache(t *testing.T) {
	f := NewFake(t0)
	c := NewTTLCache[string, int](f, 10*time.Second)
	c.Set("a", 1)
	f.Advance(4 * time.Second)
	c.Set("b", 2)
	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("Get(a) = %v %v", v, ok)
	}
	if c.Len() != 2 {
		t.Errorf("Len() = %d, ожидалось 2", c.Len())
	}
	f.Advance(6*time.Second - time.Nanosecond)
	if _, ok := c.Get("a"); !ok {
		t.Error("a протухла на наносекунду раньше срока")
	}
	f.Advance(time.Nanosecond)
	if _, ok := c.Get("a"); ok {
		t.Error("a должна протухнуть ровно через ttl")
	}
	if c.Len() != 1 {
		t.Errorf("Len() = %d, ожидалось 1", c.Len())
	}
	c.Set("b", 20) // перезапись продлевает
	f.Advance(9 * time.Second)
	if v, ok := c.Get("b"); !ok || v != 20 {
		t.Errorf("перезаписанная b: %v %v", v, ok)
	}
	f.Advance(time.Second)
	if c.Len() != 0 {
		t.Errorf("Len() = %d, ожидалось 0", c.Len())
	}
	if _, ok := c.Get("missing"); ok {
		t.Error("Get несуществующего ключа")
	}
}

func TestTTLCacheConcurrent(t *testing.T) {
	f := NewFake(t0)
	c := NewTTLCache[int, int](f, time.Second)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				c.Set(i%10, g)
				c.Get(i % 7)
				c.Len()
				if i%50 == 0 {
					f.Advance(100 * time.Millisecond)
				}
			}
		}(g)
	}
	wg.Wait()
}

func TestPollWithFakeClock(t *testing.T) {
	f := NewFake(t0)
	var calls atomic.Int32
	errCh := make(chan error, 1)
	go func() {
		errCh <- Poll(context.Background(), f, time.Minute, func() (bool, error) {
			return calls.Add(1) == 3, nil
		})
	}()
	for i := 0; i < 2; i++ {
		waitWaiters(t, f, 1)
		f.Advance(time.Minute)
	}
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Poll вернул %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Poll не завершился после третьей проверки")
	}
	if calls.Load() != 3 {
		t.Errorf("cond вызван %d раз, ожидалось 3", calls.Load())
	}
	if el := f.Now().Sub(t0); el != 2*time.Minute {
		t.Errorf("прошло %v фейкового времени, ожидалось 2m", el)
	}
}

func waitWaiters(t *testing.T, f *Fake, n int) {
	t.Helper()
	done := make(chan struct{})
	go func() { f.BlockUntil(n); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("никто не ждёт на After (Waiters() = %d) — Poll должен использовать c.After", f.Waiters())
	}
}

func TestPollErrorsAndCancel(t *testing.T) {
	f := NewFake(t0)
	boom := errors.New("boom")
	if err := Poll(context.Background(), f, time.Second, func() (bool, error) { return false, boom }); !errors.Is(err, boom) {
		t.Errorf("ошибка cond: получено %v", err)
	}
	if err := Poll(context.Background(), f, time.Second, func() (bool, error) { return true, nil }); err != nil {
		t.Errorf("cond сразу true: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- Poll(ctx, f, time.Hour, func() (bool, error) { return false, nil })
	}()
	waitWaiters(t, f, 1)
	cancel()
	select {
	case err := <-errCh:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("после отмены ctx: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Poll не реагирует на отмену контекста")
	}

	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if err := Poll(ctx, f, time.Second, func() (bool, error) { return false, nil }); !errors.Is(err, context.Canceled) {
		t.Errorf("уже отменённый ctx: %v", err)
	}
}

func TestPollRealClock(t *testing.T) {
	var n atomic.Int32
	start := time.Now()
	err := Poll(context.Background(), Real{}, 5*time.Millisecond, func() (bool, error) { return n.Add(1) == 3, nil })
	if err != nil || n.Load() != 3 {
		t.Fatalf("Poll(Real): %v, вызовов %d", err, n.Load())
	}
	if time.Since(start) < 10*time.Millisecond {
		t.Error("Poll не ждал interval между проверками")
	}
}
