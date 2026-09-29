package parmap

import (
	"fmt"
	"runtime"
	"slices"
	"strconv"
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

func TestMapOrder(t *testing.T) {
	tests := []struct {
		name    string
		n       int
		workers int
	}{
		{"пустой", 0, 4},
		{"один", 1, 4},
		{"воркеров больше элементов", 5, 100},
		{"много элементов", 1000, 8},
		{"GOMAXPROCS", 300, 0},
		{"отрицательное число воркеров", 50, -3},
		{"один воркер", 77, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			in := make([]int, tc.n)
			want := make([]string, tc.n)
			for i := range in {
				in[i] = i
				want[i] = strconv.Itoa(i * i)
			}
			base := runtime.NumGoroutine()
			got := Map(in, tc.workers, func(x int) string {
				if x%7 == 0 {
					runtime.Gosched() // перемешиваем порядок завершения
				}
				return strconv.Itoa(x * x)
			})
			if !slices.Equal(got, want) {
				t.Errorf("Map вернул %v, ожидалось %v", got, want)
			}
			checkNoLeak(t, base)
		})
	}
}

func TestMapNilInput(t *testing.T) {
	got := Map[int, int](nil, 4, func(x int) int { return x })
	if got == nil || len(got) != 0 {
		t.Errorf("Map(nil) = %#v, ожидался пустой не-nil срез", got)
	}
}

func TestMapParallelAndLimited(t *testing.T) {
	const workers = 3
	in := make([]int, 30)
	var active, maxActive atomic.Int64
	var timedOut atomic.Bool
	reached := make(chan struct{})
	var once sync.Once

	base := runtime.NumGoroutine()
	Map(in, workers, func(int) int {
		cur := active.Add(1)
		for {
			m := maxActive.Load()
			if cur <= m || maxActive.CompareAndSwap(m, cur) {
				break
			}
		}
		if cur >= workers {
			once.Do(func() { close(reached) })
		}
		if !timedOut.Load() {
			select {
			case <-reached:
			case <-time.After(time.Second):
				timedOut.Store(true)
			}
		}
		active.Add(-1)
		return 0
	})
	if timedOut.Load() {
		t.Errorf("не набралось %d одновременных вызовов f — Map не параллелен", workers)
	}
	if m := maxActive.Load(); m > workers {
		t.Errorf("одновременно выполнялось %d вызовов f, лимит %d", m, workers)
	}
	checkNoLeak(t, base)
}

func TestMapPanicPropagates(t *testing.T) {
	base := runtime.NumGoroutine()
	var calls atomic.Int64
	func() {
		defer func() {
			r := recover()
			if r != "boom 13" {
				t.Errorf("recover() = %v, ожидалось значение паники %q", r, "boom 13")
			}
		}()
		in := make([]int, 100)
		for i := range in {
			in[i] = i
		}
		Map(in, 4, func(x int) int {
			calls.Add(1)
			if x == 13 {
				panic(fmt.Sprintf("boom %d", x))
			}
			return x
		})
		t.Errorf("Map должен был запаниковать")
	}()
	// Все воркеры должны завершиться ещё до возврата паники.
	checkNoLeak(t, base)
}

// Элементы раздаются динамически: пока один воркер «застрял» на медленном
// элементе, остальные должны разобрать ВСЕ прочие элементы (при статическом
// делении на чанки элементы 1..4 ждали бы, пока освободится воркер с элементом 0).
func TestMapDynamicBalancing(t *testing.T) {
	const n = 10
	in := make([]int, n)
	for i := range in {
		in[i] = i
	}
	var others sync.WaitGroup
	others.Add(n - 1)
	var balanced atomic.Bool
	base := runtime.NumGoroutine()
	Map(in, 2, func(x int) int {
		if x != 0 {
			others.Done()
			return x
		}
		done := make(chan struct{})
		go func() { others.Wait(); close(done) }()
		select {
		case <-done:
			balanced.Store(true)
		case <-time.After(2 * time.Second):
		}
		return x
	})
	if !balanced.Load() {
		t.Errorf("пока f(in[0]) выполнялась, второй воркер не обработал остальные %d элементов: "+
			"элементы должны раздаваться динамически, а не чанками", n-1)
	}
	checkNoLeak(t, base)
}

// При панике Map дожидается завершения всех воркеров и только потом паникует:
// ни один вызов f не должен продолжаться после того, как паника дошла до вызывающего.
func TestMapPanicWaitsForWorkers(t *testing.T) {
	base := runtime.NumGoroutine()
	var returned atomic.Bool // паника уже дошла до вызывающего
	var late atomic.Int64    // вызовы f, закончившиеся после этого
	var started sync.WaitGroup
	started.Add(3)
	allStarted := make(chan struct{})
	go func() { started.Wait(); close(allStarted) }()

	func() {
		defer func() {
			recover()
			returned.Store(true)
		}()
		Map([]int{0, 1, 2, 3}, 4, func(x int) int {
			if x == 0 {
				select { // даём остальным элементам начаться
				case <-allStarted:
				case <-time.After(time.Second):
				}
				panic("boom")
			}
			started.Done()
			time.Sleep(50 * time.Millisecond)
			if returned.Load() {
				late.Add(1)
			}
			return x
		})
	}()
	checkNoLeak(t, base)
	if l := late.Load(); l > 0 {
		t.Errorf("%d вызовов f завершились уже после того, как Map запаниковал: "+
			"нужно дождаться всех воркеров (wg.Wait) перед повторной паникой", l)
	}
}
