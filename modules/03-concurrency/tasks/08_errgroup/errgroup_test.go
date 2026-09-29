package errgroup

import (
	"context"
	"errors"
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

// waitErr вызывает g.Wait() с таймаутом, чтобы зависание не вешало тесты.
func waitErr(t *testing.T, g *Group) error {
	t.Helper()
	ch := make(chan error, 1)
	go func() { ch <- g.Wait() }()
	select {
	case err := <-ch:
		return err
	case <-time.After(2 * time.Second):
		t.Fatal("Wait не вернулся за 2с")
		return nil
	}
}

func TestZeroValue(t *testing.T) {
	base := runtime.NumGoroutine()
	var g Group
	var n atomic.Int64
	for i := 0; i < 50; i++ {
		g.Go(func() error {
			time.Sleep(time.Millisecond)
			n.Add(1)
			return nil
		})
	}
	if err := waitErr(t, &g); err != nil {
		t.Errorf("Wait() = %v, ожидалось nil", err)
	}
	if n.Load() != 50 {
		t.Errorf("к возврату из Wait завершилось %d горутин из 50", n.Load())
	}
	checkNoLeak(t, base)
}

func TestZeroValueError(t *testing.T) {
	var g Group
	errA := errors.New("A")
	g.Go(func() error { return nil })
	g.Go(func() error { return errA })
	if err := waitErr(t, &g); err != errA {
		t.Errorf("Wait() = %v, ожидалось %v", err, errA)
	}
}

func TestFirstErrorCancelsContext(t *testing.T) {
	base := runtime.NumGoroutine()
	g, ctx := WithContext(context.Background())
	errFirst := errors.New("first")
	errSecond := errors.New("second")
	sawCancel := make(chan error, 1)

	g.Go(func() error {
		// Ждём отмены, вызванной первой ошибкой, и возвращаем свою ошибку.
		select {
		case <-ctx.Done():
			sawCancel <- context.Cause(ctx)
			return errSecond
		case <-time.After(2 * time.Second):
			sawCancel <- nil
			return errors.New("контекст не отменился после ошибки")
		}
	})
	g.Go(func() error { return errFirst })

	if err := waitErr(t, g); err != errFirst {
		t.Errorf("Wait() = %v, ожидалась первая ошибка %v", err, errFirst)
	}
	if c := <-sawCancel; c != errFirst {
		t.Errorf("context.Cause(ctx) внутри горутины = %v, ожидалось %v", c, errFirst)
	}
	if c := context.Cause(ctx); c != errFirst {
		t.Errorf("после Wait context.Cause(ctx) = %v, ожидалось %v (Wait не должен менять причину)", c, errFirst)
	}
	checkNoLeak(t, base)
}

func TestContextCanceledAfterWait(t *testing.T) {
	g, ctx := WithContext(context.Background())
	g.Go(func() error { return nil })
	if err := waitErr(t, g); err != nil {
		t.Fatalf("Wait() = %v", err)
	}
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Errorf("после успешного Wait ctx.Err() = %v, ожидалось context.Canceled", ctx.Err())
	}
}

func TestParentCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	g, ctx := WithContext(parent)
	g.Go(func() error {
		<-ctx.Done()
		return ctx.Err()
	})
	cancel()
	if err := waitErr(t, g); !errors.Is(err, context.Canceled) {
		t.Errorf("Wait() = %v, ожидалось context.Canceled", err)
	}
}

func TestSetLimit(t *testing.T) {
	const limit = 3
	base := runtime.NumGoroutine()
	var g Group
	g.SetLimit(limit)

	var active, maxActive atomic.Int64
	var timedOut atomic.Bool
	reached := make(chan struct{})
	var once sync.Once
	for i := 0; i < 30; i++ {
		g.Go(func() error { // блокируется, когда слотов нет
			cur := active.Add(1)
			for {
				m := maxActive.Load()
				if cur <= m || maxActive.CompareAndSwap(m, cur) {
					break
				}
			}
			if cur >= limit {
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
			return nil
		})
	}
	if err := waitErr(t, &g); err != nil {
		t.Errorf("Wait() = %v", err)
	}
	if timedOut.Load() {
		t.Errorf("не набралось %d одновременных горутин", limit)
	}
	if m := maxActive.Load(); m > limit {
		t.Errorf("одновременно было активно %d горутин при лимите %d", m, limit)
	}
	checkNoLeak(t, base)
}

func TestTryGo(t *testing.T) {
	var g Group
	g.SetLimit(1)
	release := make(chan struct{})
	started := make(chan struct{})
	if !g.TryGo(func() error { close(started); <-release; return nil }) {
		t.Fatal("TryGo в пустой группе с лимитом 1 должен вернуть true")
	}
	<-started
	ran := false
	if g.TryGo(func() error { ran = true; return nil }) {
		t.Errorf("TryGo при занятом слоте должен вернуть false")
	}
	close(release)
	if err := waitErr(t, &g); err != nil {
		t.Errorf("Wait() = %v", err)
	}
	if ran {
		t.Errorf("функция, для которой TryGo вернул false, не должна выполняться")
	}
	if !g.TryGo(func() error { return nil }) {
		t.Errorf("после Wait слот свободен — TryGo должен вернуть true")
	}
	waitErr(t, &g)

	var unlimited Group
	for i := 0; i < 10; i++ {
		if !unlimited.TryGo(func() error { return nil }) {
			t.Fatalf("TryGo без лимита должен всегда возвращать true")
		}
	}
	waitErr(t, &unlimited)
}

func TestSetLimitWhileActivePanics(t *testing.T) {
	var g Group
	g.SetLimit(2)
	release := make(chan struct{})
	g.Go(func() error { <-release; return nil })
	func() {
		defer func() {
			if recover() == nil {
				t.Errorf("SetLimit при активных горутинах должен паниковать")
			}
		}()
		g.SetLimit(5)
	}()
	close(release)
	waitErr(t, &g)
	g.SetLimit(-1) // после Wait можно, и -1 снимает лимит
	for i := 0; i < 10; i++ {
		if !g.TryGo(func() error { return nil }) {
			t.Fatalf("после SetLimit(-1) TryGo должен всегда возвращать true")
		}
	}
	waitErr(t, &g)
}
