package parsum

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

func TestWorkers(t *testing.T) {
	if got := Workers(3); got != 3 {
		t.Errorf("Workers(3) = %d, ожидалось 3", got)
	}
	want := runtime.GOMAXPROCS(0)
	for _, n := range []int{0, -5} {
		if got := Workers(n); got != want {
			t.Errorf("Workers(%d) = %d, ожидалось GOMAXPROCS=%d", n, got, want)
		}
	}
}

func TestSum(t *testing.T) {
	mk := func(n int) []int64 {
		s := make([]int64, n)
		for i := range s {
			s[i] = int64(i*7 - 3)
		}
		return s
	}
	seq := func(s []int64) int64 {
		var r int64
		for _, v := range s {
			r += v
		}
		return r
	}
	tests := []struct {
		name    string
		nums    []int64
		workers int
	}{
		{"nil", nil, 4},
		{"пустой", []int64{}, 4},
		{"один элемент", []int64{42}, 8},
		{"воркеров больше элементов", mk(3), 16},
		{"неровное деление", mk(1001), 7},
		{"по умолчанию GOMAXPROCS", mk(10000), 0},
		{"один воркер", mk(100), 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := runtime.NumGoroutine()
			if got, want := Sum(tc.nums, tc.workers), seq(tc.nums); got != want {
				t.Errorf("Sum(len=%d, workers=%d) = %d, ожидалось %d", len(tc.nums), tc.workers, got, want)
			}
			checkNoLeak(t, base)
		})
	}
}

func TestCountCorrect(t *testing.T) {
	items := make([]int, 12345)
	for i := range items {
		items[i] = i
	}
	even := func(x int) bool { return x%2 == 0 }
	for _, w := range []int{-1, 0, 1, 3, 64, 100000} {
		if got := Count(items, w, even); got != 6173 {
			t.Errorf("Count(чётные, workers=%d) = %d, ожидалось 6173", w, got)
		}
	}
	if got := Count([]string{}, 4, func(string) bool { return true }); got != 0 {
		t.Errorf("Count(пустой) = %d, ожидалось 0", got)
	}
}

// TestCountParallel проверяет, что вызовы pred действительно идут параллельно
// (одновременно активны ровно workers вызовов), что лимит не превышается и
// что срез поделён на непрерывные чанки (по горутине на чанк).
func TestCountParallel(t *testing.T) {
	const workers = 4
	const n = 40 // 4 чанка по 10 элементов: [0,10) [10,20) [20,30) [30,40)
	items := make([]int, n)
	for i := range items {
		items[i] = i
	}

	var (
		mu        sync.Mutex
		active    = map[int]bool{} // элементы, для которых pred сейчас выполняется
		maxActive int
		snapshot  []int // активные элементы в момент, когда их стало workers
	)
	var timedOut atomic.Bool
	reached := make(chan struct{})

	pred := func(x int) bool {
		mu.Lock()
		active[x] = true
		maxActive = max(maxActive, len(active))
		if len(active) >= workers && snapshot == nil {
			for k := range active {
				snapshot = append(snapshot, k)
			}
			close(reached)
		}
		mu.Unlock()

		if !timedOut.Load() {
			select {
			case <-reached:
			case <-time.After(time.Second):
				timedOut.Store(true)
			}
		}

		mu.Lock()
		delete(active, x)
		mu.Unlock()
		return true
	}

	base := runtime.NumGoroutine()
	if got := Count(items, workers, pred); got != len(items) {
		t.Errorf("Count = %d, ожидалось %d", got, len(items))
	}
	if timedOut.Load() {
		t.Errorf("за 1с не набралось %d одновременных вызовов pred: обработка не параллельна", workers)
	}
	mu.Lock()
	defer mu.Unlock()
	if maxActive > workers {
		t.Errorf("одновременно выполнялось %d вызовов pred, лимит %d", maxActive, workers)
	}
	if snapshot != nil {
		chunkSeen := map[int]bool{}
		for _, x := range snapshot {
			chunkSeen[x/(n/workers)] = true
		}
		if len(chunkSeen) != workers {
			t.Errorf("одновременно обрабатывались элементы %v: ожидалось по одному из каждого из %d "+
				"непрерывных чанков по %d элементов (по горутине на чанк)", snapshot, workers, n/workers)
		}
	}
	checkNoLeak(t, base)
}

func BenchmarkSum(b *testing.B) {
	nums := make([]int64, 1<<20)
	for i := range nums {
		nums[i] = int64(i)
	}
	for _, w := range []int{1, 0} {
		name := "workers=1"
		if w == 0 {
			name = "workers=GOMAXPROCS"
		}
		b.Run(name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				Sum(nums, w)
			}
		})
	}
}
