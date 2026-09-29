package counter

import (
	"fmt"
	"sync"
	"testing"
)

func TestCounterConcurrent(t *testing.T) {
	var c Counter // нулевое значение должно работать
	const g, n = 50, 1000
	var wg sync.WaitGroup
	for i := 0; i < g; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < n; j++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()
	if got := c.Value(); got != g*n {
		t.Errorf("Value() = %d после %d инкрементов, ожидалось %d", got, g*n, g*n)
	}
	if got := c.Add(-g * n); got != 0 {
		t.Errorf("Add(-%d) = %d, ожидалось 0", g*n, got)
	}
	if got := c.Add(5); got != 5 {
		t.Errorf("Add(5) = %d, ожидалось 5", got)
	}
}

func TestKeyCounterZeroValue(t *testing.T) {
	var k KeyCounter
	if got := k.Get("x"); got != 0 {
		t.Errorf("Get на пустом = %d, ожидалось 0", got)
	}
	if got := k.Len(); got != 0 {
		t.Errorf("Len на пустом = %d, ожидалось 0", got)
	}
	if s := k.Snapshot(); s == nil || len(s) != 0 {
		t.Errorf("Snapshot на пустом = %#v, ожидалась пустая не-nil map", s)
	}
	if r := k.Reset(); r == nil || len(r) != 0 {
		t.Errorf("Reset на пустом = %#v, ожидалась пустая не-nil map", r)
	}
	if got := k.Inc("a"); got != 1 {
		t.Errorf("первый Inc(a) = %d, ожидалось 1", got)
	}
}

func TestKeyCounterConcurrent(t *testing.T) {
	var k KeyCounter
	const g, n, keys = 20, 500, 5
	var wg sync.WaitGroup
	for i := 0; i < g; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < n; j++ {
				k.Inc(fmt.Sprintf("k%d", j%keys))
			}
		}()
		// параллельные читатели: под -race поймают возврат внутренней map
		go func() {
			defer wg.Done()
			for j := 0; j < n/10; j++ {
				s := k.Snapshot()
				for key := range s {
					s[key]++ // модифицируем копию
				}
				_ = k.Get("k0")
				_ = k.Len()
			}
		}()
	}
	wg.Wait()

	snap := k.Snapshot()
	if len(snap) != keys {
		t.Fatalf("число ключей = %d, ожидалось %d", len(snap), keys)
	}
	for i := 0; i < keys; i++ {
		key := fmt.Sprintf("k%d", i)
		if snap[key] != g*n/keys {
			t.Errorf("счётчик %s = %d, ожидалось %d (модификация Snapshot не должна влиять)", key, snap[key], g*n/keys)
		}
	}
}

func TestKeyCounterReset(t *testing.T) {
	var k KeyCounter
	k.Inc("a")
	k.Inc("a")
	k.Inc("b")
	old := k.Reset()
	if old["a"] != 2 || old["b"] != 1 || len(old) != 2 {
		t.Errorf("Reset() = %v, ожидалось map[a:2 b:1]", old)
	}
	if k.Len() != 0 || k.Get("a") != 0 {
		t.Errorf("после Reset Len=%d Get(a)=%d, ожидалось 0 и 0", k.Len(), k.Get("a"))
	}
	k.Inc("a")
	if old["a"] != 2 {
		t.Errorf("Inc после Reset изменил возвращённую map: old[a]=%d", old["a"])
	}
}

func TestResetConcurrentNoLoss(t *testing.T) {
	// Сумма всех Reset + остаток должна равняться числу инкрементов:
	// Reset не должен «терять» инкременты между чтением и обнулением.
	var k KeyCounter
	const g, n = 8, 2000
	var wg sync.WaitGroup
	var mu sync.Mutex
	var collected int64
	for i := 0; i < g; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < n; j++ {
				k.Inc("x")
				if j%100 == 0 {
					r := k.Reset()
					mu.Lock()
					collected += r["x"]
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	total := collected + k.Get("x")
	if total != g*n {
		t.Errorf("сумма по Reset + остаток = %d, ожидалось %d — Reset теряет инкременты", total, g*n)
	}
}

func BenchmarkCounterInc(b *testing.B) {
	var c Counter
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			c.Inc()
		}
	})
}

func BenchmarkKeyCounterGet(b *testing.B) {
	var k KeyCounter
	k.Inc("hot")
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			k.Get("hot")
		}
	})
}
