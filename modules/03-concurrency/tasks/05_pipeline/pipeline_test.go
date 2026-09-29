package pipeline

import (
	"context"
	"runtime"
	"slices"
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

// collect читает канал до закрытия, но не дольше 2 секунд.
func collect(t *testing.T, ch <-chan int) []int {
	t.Helper()
	var res []int
	timeout := time.After(2 * time.Second)
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				return res
			}
			res = append(res, v)
		case <-timeout:
			t.Fatalf("канал не закрылся за 2с (прочитано %d значений)", len(res))
			return nil
		}
	}
}

func seq(n int) []int {
	s := make([]int, n)
	for i := range s {
		s[i] = i + 1
	}
	return s
}

func TestGenerate(t *testing.T) {
	base := runtime.NumGoroutine()
	ctx := context.Background()
	if got := collect(t, Generate(ctx, 3, 1, 2)); !slices.Equal(got, []int{3, 1, 2}) {
		t.Errorf("Generate(3,1,2) = %v, ожидалось [3 1 2]", got)
	}
	if got := collect(t, Generate(ctx)); len(got) != 0 {
		t.Errorf("Generate() = %v, ожидался пустой закрытый канал", got)
	}
	checkNoLeak(t, base)
}

func TestSquareKeepsOrder(t *testing.T) {
	base := runtime.NumGoroutine()
	ctx := context.Background()
	got := collect(t, Square(ctx, Square(ctx, Generate(ctx, 1, 2, 3, -4))))
	if want := []int{1, 16, 81, 256}; !slices.Equal(got, want) {
		t.Errorf("Square(Square(1,2,3,-4)) = %v, ожидалось %v", got, want)
	}
	checkNoLeak(t, base)
}

func TestMerge(t *testing.T) {
	base := runtime.NumGoroutine()
	ctx := context.Background()

	t.Run("без входов", func(t *testing.T) {
		if got := collect(t, Merge(ctx)); len(got) != 0 {
			t.Errorf("Merge() = %v, ожидался пустой закрытый канал", got)
		}
	})
	t.Run("несколько входов", func(t *testing.T) {
		got := collect(t, Merge(ctx, Generate(ctx, 1, 2, 3), Generate(ctx), Generate(ctx, 10, 20)))
		slices.Sort(got)
		if want := []int{1, 2, 3, 10, 20}; !slices.Equal(got, want) {
			t.Errorf("Merge = %v, ожидалось (в любом порядке) %v", got, want)
		}
	})
	t.Run("медленный вход не теряется", func(t *testing.T) {
		slow := make(chan int)
		go func() {
			defer close(slow)
			time.Sleep(30 * time.Millisecond)
			slow <- 99
		}()
		got := collect(t, Merge(ctx, Generate(ctx, 1), slow))
		slices.Sort(got)
		if !slices.Equal(got, []int{1, 99}) {
			t.Errorf("Merge = %v, ожидалось [1 99] — выход закрыт раньше, чем все входы?", got)
		}
	})
	checkNoLeak(t, base)
}

func TestSquareParallel(t *testing.T) {
	base := runtime.NumGoroutine()
	ctx := context.Background()
	for _, w := range []int{-1, 1, 4, 16} {
		got := collect(t, SquareParallel(ctx, Generate(ctx, seq(200)...), w))
		slices.Sort(got)
		want := make([]int, 200)
		for i := range want {
			want[i] = (i + 1) * (i + 1)
		}
		if !slices.Equal(got, want) {
			t.Errorf("SquareParallel(workers=%d): получено %d значений, ожидалось %d квадратов 1..200", w, len(got), len(want))
		}
	}
	checkNoLeak(t, base)
}

// Потребитель читает пару значений и уходит, отменив контекст.
// Все стадии должны завершиться и закрыть свои каналы.
func TestCancelStopsPipeline(t *testing.T) {
	base := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	out := SquareParallel(ctx, Square(ctx, Generate(ctx, seq(100000)...)), 4)
	for i := 0; i < 3; i++ {
		select {
		case <-out:
		case <-time.After(2 * time.Second):
			t.Fatal("конвейер не выдал значения за 2с")
		}
	}
	cancel()
	// Выход должен закрыться (дочитываем то, что уже в полёте).
	collect(t, out)
	checkNoLeak(t, base)
}

// Даже если никто не читает выход, отмена должна освободить горутины.
func TestCancelWithoutReader(t *testing.T) {
	base := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	_ = Merge(ctx, Square(ctx, Generate(ctx, seq(1000)...)), Generate(ctx, 1, 2, 3))
	time.Sleep(10 * time.Millisecond)
	cancel()
	checkNoLeak(t, base)
}

// Вход, который никогда не закрывается: Square/Merge должны выйти по ctx.
func TestCancelStuckUpstream(t *testing.T) {
	base := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	stuck := make(chan int) // никто не пишет и не закрывает
	out := Merge(ctx, Square(ctx, stuck))
	cancel()
	collect(t, out)
	checkNoLeak(t, base)
}

// SquareParallel действительно делает fan-out: запускает workers стадий
// (не меньше workers горутин), а не одну стадию Square.
func TestSquareParallelFanOut(t *testing.T) {
	const workers = 8
	base := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	stuck := make(chan int) // вход без данных: все стадии ждут на чтении
	out := SquareParallel(ctx, stuck, workers)
	if got := runtime.NumGoroutine() - base; got < workers {
		t.Errorf("SquareParallel(workers=%d) запустил %d горутин, ожидалось не меньше %d (fan-out)", workers, got, workers)
	}
	cancel()
	collect(t, out)
	checkNoLeak(t, base)
}
