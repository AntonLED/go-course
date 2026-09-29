package barrier

import (
	"runtime"
	"sync"
	"sync/atomic"
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

func waitTimeout(t *testing.T, wg *sync.WaitGroup, d time.Duration, msg string) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatalf("таймаут: %s", msg)
	}
}

func TestNewPanics(t *testing.T) {
	for _, n := range []int{0, -1} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("New(%d) должен паниковать", n)
				}
			}()
			New(n)
		}()
	}
}

func TestSingleParticipant(t *testing.T) {
	b := New(1)
	for i := 0; i < 3; i++ {
		if !b.Wait() {
			t.Errorf("New(1).Wait() = false, единственный участник — всегда последний")
		}
	}
}

func TestBlocksUntilAll(t *testing.T) {
	const n = 5
	b := New(n)
	base := runtime.NumGoroutine()
	var passed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < n-1; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Wait()
			passed.Add(1)
		}()
	}
	time.Sleep(50 * time.Millisecond)
	if p := passed.Load(); p != 0 {
		t.Fatalf("%d горутин прошли барьер, хотя пришло только %d из %d", p, n-1, n)
	}
	// Последний участник — из тестовой горутины.
	if !b.Wait() {
		t.Errorf("последний пришедший должен получить true")
	}
	waitTimeout(t, &wg, 2*time.Second, "участники не освободились после прихода последнего")
	if p := passed.Load(); p != n-1 {
		t.Errorf("прошли %d, ожидалось %d", p, n-1)
	}
	checkNoLeak(t, base)
}

func TestCyclicPhases(t *testing.T) {
	const n, rounds = 8, 200
	b := New(n)
	var arrived [rounds]atomic.Int64
	var leaders [rounds]atomic.Int64
	var errs atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 0; r < rounds; r++ {
				arrived[r].Add(1)
				if b.Wait() {
					leaders[r].Add(1)
				}
				// После барьера все n участников фазы r уже пришли.
				if arrived[r].Load() != n {
					errs.Add(1)
				}
			}
		}()
	}
	waitTimeout(t, &wg, 5*time.Second, "барьер завис (дедлок при повторном использовании?)")
	if e := errs.Load(); e > 0 {
		t.Errorf("%d раз горутина прошла барьер раньше, чем пришли все участники фазы", e)
	}
	for r := 0; r < rounds; r++ {
		if l := leaders[r].Load(); l != 1 {
			t.Fatalf("в фазе %d Wait вернул true %d раз, ожидалось ровно 1", r, l)
		}
	}
}
