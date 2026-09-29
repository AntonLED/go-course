package workerpool

import (
	"context"
	"errors"
	"runtime"
	"slices"
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

func feed(vals ...int) <-chan int {
	ch := make(chan int, len(vals))
	for _, v := range vals {
		ch <- v
	}
	close(ch)
	return ch
}

func drain[T, R any](t *testing.T, ch <-chan Result[T, R]) []Result[T, R] {
	t.Helper()
	var res []Result[T, R]
	timeout := time.After(2 * time.Second)
	for {
		select {
		case r, ok := <-ch:
			if !ok {
				return res
			}
			res = append(res, r)
		case <-timeout:
			t.Fatalf("выходной канал не закрылся за 2с (получено %d результатов)", len(res))
			return nil
		}
	}
}

var errOdd = errors.New("odd")

func double(_ context.Context, v int) (int, error) {
	if v%2 != 0 {
		return 0, errOdd
	}
	return v * 2, nil
}

func TestRunAllResults(t *testing.T) {
	for _, w := range []int{0, 1, 3, 50} {
		base := runtime.NumGoroutine()
		vals := make([]int, 100)
		for i := range vals {
			vals[i] = i
		}
		res := drain(t, Run(context.Background(), w, feed(vals...), double))
		if len(res) != len(vals) {
			t.Fatalf("workers=%d: получено %d результатов, ожидалось %d", w, len(res), len(vals))
		}
		var ins []int
		for _, r := range res {
			ins = append(ins, r.In)
			if r.In%2 == 0 && (r.Err != nil || r.Out != r.In*2) {
				t.Errorf("результат для %d = (%d, %v), ожидалось (%d, nil)", r.In, r.Out, r.Err, r.In*2)
			}
			if r.In%2 != 0 && !errors.Is(r.Err, errOdd) {
				t.Errorf("для %d ожидалась ошибка errOdd, получено %v", r.In, r.Err)
			}
		}
		slices.Sort(ins)
		if !slices.Equal(ins, vals) {
			t.Errorf("workers=%d: каждая задача должна быть обработана ровно один раз", w)
		}
		checkNoLeak(t, base)
	}
}

func TestRunEmptyInput(t *testing.T) {
	base := runtime.NumGoroutine()
	if res := drain(t, Run(context.Background(), 4, feed(), double)); len(res) != 0 {
		t.Errorf("пустой вход: получено %d результатов", len(res))
	}
	checkNoLeak(t, base)
}

func TestRunConcurrencyLimit(t *testing.T) {
	const workers = 4
	var active, maxActive atomic.Int64
	var timedOut atomic.Bool
	reached := make(chan struct{})
	var once sync.Once
	f := func(_ context.Context, v int) (int, error) {
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
		return v, nil
	}
	base := runtime.NumGoroutine()
	res := drain(t, Run(context.Background(), workers, feed(make([]int, 40)...), f))
	if len(res) != 40 {
		t.Errorf("получено %d результатов, ожидалось 40", len(res))
	}
	if timedOut.Load() {
		t.Errorf("не набралось %d одновременно работающих воркеров", workers)
	}
	if m := maxActive.Load(); m > workers {
		t.Errorf("одновременно работало %d воркеров, лимит %d", m, workers)
	}
	checkNoLeak(t, base)
}

// Потребитель прочитал один результат и отменил контекст, больше не читая.
// Воркеры не должны зависнуть на отправке, выход должен закрыться.
func TestRunCancelConsumerGone(t *testing.T) {
	base := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan int) // бесконечный источник, который тоже слушает ctx
	go func() {
		defer close(in)
		for i := 0; ; i++ {
			select {
			case in <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	var calls atomic.Int64
	out := Run(ctx, 3, in, func(_ context.Context, v int) (int, error) {
		calls.Add(1)
		return v, nil
	})
	select {
	case <-out:
	case <-time.After(2 * time.Second):
		t.Fatal("нет ни одного результата за 2с")
	}
	cancel()
	checkNoLeak(t, base) // выход никто не читает — но горутины всё равно должны уйти
	drain(t, out)
	checkNoLeak(t, base)
}

// После отмены воркеры не берут новые задачи из (заполненного) входа.
// В момент cancel() каждый воркер держит не больше одной задачи, поэтому
// всего вызовов f — не больше workers. Без явной проверки ctx.Err() select
// случайно выбирал бы и заполненный in, поэтому сценарий повторяется.
func TestRunCancelStopsTakingTasks(t *testing.T) {
	const workers = 4
	for round := 0; round < 30; round++ {
		base := runtime.NumGoroutine()
		ctx, cancel := context.WithCancel(context.Background())
		vals := make([]int, 1000)
		var calls atomic.Int64
		started := make(chan struct{})
		release := make(chan struct{})
		out := Run(ctx, workers, feed(vals...), func(ctx context.Context, v int) (int, error) {
			if calls.Add(1) == 1 {
				close(started)
			}
			<-release
			return v, ctx.Err()
		})
		<-started
		cancel()
		close(release)
		drain(t, out)
		if c := calls.Load(); c > workers {
			t.Fatalf("после отмены обработано %d задач при %d воркерах — воркеры должны "+
				"прекращать брать новые (проверяйте ctx.Err() перед приёмом из in)", c, workers)
		}
		checkNoLeak(t, base)
	}
}

// f получает контекст, который отменяется вместе с ctx.
func TestRunPassesContext(t *testing.T) {
	base := runtime.NumGoroutine()
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan error, 1)
	out := Run(ctx, 1, feed(1), func(ctx context.Context, v int) (int, error) {
		cancel()
		select {
		case <-ctx.Done():
			got <- ctx.Err()
		case <-time.After(2 * time.Second):
			got <- errors.New("контекст в f не отменился")
		}
		return v, nil
	})
	drain(t, out)
	if err := <-got; !errors.Is(err, context.Canceled) {
		t.Errorf("ctx.Err() внутри f = %v, ожидалось context.Canceled", err)
	}
	checkNoLeak(t, base)
}
