package singleflight

import (
	"bytes"
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

// waitInDo ждёт, пока в стеках горутин наберётся не меньше n кадров Group.Do.
// Горутина, уже вошедшая в Do, пока fn ведущего вызова заблокирована,
// обязана присоединиться к нему, — так тест не зависит от time.Sleep.
func waitInDo(t *testing.T, n int) {
	t.Helper()
	buf := make([]byte, 1<<20)
	deadline := time.Now().Add(5 * time.Second)
	for {
		m := runtime.Stack(buf, true)
		if bytes.Count(buf[:m], []byte("(*Group[...]).Do(")) >= n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("за 5с в Group.Do не вошли %d горутин", n)
		}
		time.Sleep(time.Millisecond)
	}
}

func recv[V any](t *testing.T, ch <-chan Result[V]) Result[V] {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("результат DoChan не пришёл за 2с")
		return Result[V]{}
	}
}

func TestDoSimple(t *testing.T) {
	var g Group[string, int] // нулевое значение
	v, err, shared := g.Do("k", func() (int, error) { return 42, nil })
	if v != 42 || err != nil || shared {
		t.Errorf("Do = (%d, %v, %v), ожидалось (42, nil, false)", v, err, shared)
	}
	errX := errors.New("x")
	_, err, _ = g.Do("k", func() (int, error) { return 0, errX })
	if err != errX {
		t.Errorf("Do вернул ошибку %v, ожидалось %v — результаты не должны кэшироваться", err, errX)
	}
}

// 100 горутин с одним ключом → ровно один вызов fn, всем shared == true.
func TestDoDedup(t *testing.T) {
	base := runtime.NumGoroutine()
	var g Group[string, string]
	const n = 100
	var calls atomic.Int64
	started := make(chan struct{})
	release := make(chan struct{})
	fn := func() (string, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		return "value", nil
	}

	var ready, done sync.WaitGroup
	ready.Add(n)
	done.Add(n)
	type res struct {
		v      string
		err    error
		shared bool
	}
	results := make([]res, n)
	for i := 0; i < n; i++ {
		go func() {
			defer done.Done()
			ready.Done()
			v, err, shared := g.Do("key", fn)
			results[i] = res{v, err, shared}
		}()
	}
	ready.Wait()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("fn так и не была вызвана")
	}
	waitInDo(t, n) // все n горутин внутри Do: ведущая — в fn, остальные ждут её результата
	close(release)
	done.Wait()

	if c := calls.Load(); c != 1 {
		t.Errorf("fn вызвана %d раз, ожидалось 1", c)
	}
	for i, r := range results {
		if r.v != "value" || r.err != nil || !r.shared {
			t.Fatalf("горутина %d получила (%q, %v, shared=%v), ожидалось (\"value\", nil, true)", i, r.v, r.err, r.shared)
		}
	}
	checkNoLeak(t, base)
}

// Детерминированная версия: DoChan регистрируется синхронно.
func TestDoChanJoinsInFlight(t *testing.T) {
	base := runtime.NumGoroutine()
	var g Group[int, int]
	var calls atomic.Int64
	release := make(chan struct{})
	fn := func() (int, error) {
		calls.Add(1)
		<-release
		return 7, nil
	}
	chans := make([]<-chan Result[int], 10)
	for i := range chans {
		chans[i] = g.DoChan(1, fn)
	}
	other := g.DoChan(2, func() (int, error) { return 8, nil }) // другой ключ — не ждёт
	if r := recv(t, other); r.Val != 8 || r.Shared {
		t.Errorf("DoChan(2) = %+v, ожидалось {Val:8 Shared:false}", r)
	}
	close(release)
	for i, ch := range chans {
		if r := recv(t, ch); r.Val != 7 || r.Err != nil || !r.Shared {
			t.Errorf("DoChan #%d = %+v, ожидалось {Val:7 Err:nil Shared:true}", i, r)
		}
	}
	if c := calls.Load(); c != 1 {
		t.Errorf("fn для ключа 1 вызвана %d раз, ожидалось 1", c)
	}
	// Одиночный вызов — не shared.
	if r := recv(t, g.DoChan(1, func() (int, error) { return 1, nil })); r.Shared {
		t.Errorf("одиночный DoChan вернул Shared=true")
	}
	checkNoLeak(t, base)
}

func TestForget(t *testing.T) {
	base := runtime.NumGoroutine()
	var g Group[string, int]
	var calls atomic.Int64
	rel1, rel2 := make(chan struct{}), make(chan struct{})
	started2 := make(chan struct{})

	// Вызов №1 (DoChan — регистрация синхронная).
	first := g.DoChan("k", func() (int, error) { calls.Add(1); <-rel1; return 1, nil })
	follower := g.DoChan("k", func() (int, error) { calls.Add(1); return -1, nil })

	g.Forget("k")

	// Вызов №2 после Forget должен стартовать, не дожидаясь №1.
	second := g.DoChan("k", func() (int, error) {
		calls.Add(1)
		close(started2)
		<-rel2
		return 2, nil
	})
	select {
	case <-started2:
	case <-time.After(2 * time.Second):
		t.Fatal("после Forget новый вызов не запустился")
	}

	// Завершаем №1: его очистка НЕ должна удалить запись вызова №2.
	close(rel1)
	if r := recv(t, first); r.Val != 1 {
		t.Errorf("первый вызов вернул %d, ожидалось 1", r.Val)
	}
	if r := recv(t, follower); r.Val != 1 || !r.Shared {
		t.Errorf("присоединившийся до Forget получил %+v, ожидался результат первого вызова (1, shared)", r)
	}
	joiner := g.DoChan("k", func() (int, error) { calls.Add(1); return 3, nil })
	close(rel2)
	if r := recv(t, second); r.Val != 2 {
		t.Errorf("второй вызов вернул %d, ожидалось 2", r.Val)
	}
	if r := recv(t, joiner); r.Val != 2 || !r.Shared {
		t.Errorf("DoChan во время вызова №2 получил %+v, ожидалось присоединение к нему (2, shared)", r)
	}
	if c := calls.Load(); c != 2 {
		t.Errorf("всего вызовов fn: %d, ожидалось 2", c)
	}
	g.Forget("нет такого ключа") // не паникует
	checkNoLeak(t, base)
}

func TestDoPanic(t *testing.T) {
	base := runtime.NumGoroutine()
	var g Group[string, int]
	started := make(chan struct{})
	release := make(chan struct{})
	leaderDone := make(chan any, 1)
	go func() {
		defer func() { leaderDone <- recover() }()
		g.Do("k", func() (int, error) {
			close(started)
			<-release
			panic("boom")
		})
	}()
	<-started
	follower := g.DoChan("k", func() (int, error) { return 0, nil })
	doFollower := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				doFollower <- errors.New("паника дошла до ожидающего Do")
			}
		}()
		_, err, _ := g.Do("k", func() (int, error) { return 0, nil })
		doFollower <- err
	}()
	waitInDo(t, 2) // ведущий и ожидающий Do
	close(release)

	select {
	case r := <-leaderDone:
		if r != "boom" {
			t.Errorf("паника в fn должна распространиться в вызвавшую Do горутину, recover() = %v", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Do не завершился после паники fn")
	}
	if r := recv(t, follower); !errors.Is(r.Err, ErrPanicked) {
		t.Errorf("ожидающий получил %+v, ожидалась ошибка ErrPanicked", r)
	}
	select {
	case err := <-doFollower:
		if !errors.Is(err, ErrPanicked) {
			t.Errorf("ожидающий Do получил ошибку %v, ожидалась обёртка ErrPanicked", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ожидающий Do завис после паники fn")
	}
	// Ключ освобождён — новый вызов работает.
	done := make(chan int, 1)
	go func() {
		v, _, _ := g.Do("k", func() (int, error) { return 5, nil })
		done <- v
	}()
	select {
	case v := <-done:
		if v != 5 {
			t.Errorf("Do после паники = %d, ожидалось 5", v)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("после паники ключ не освобождён: Do завис")
	}

	// Паника в DoChan не роняет процесс.
	if r := recv(t, g.DoChan("p", func() (int, error) { panic("oops") })); !errors.Is(r.Err, ErrPanicked) {
		t.Errorf("DoChan с паникующей fn вернул %+v, ожидалась ошибка ErrPanicked", r)
	}
	checkNoLeak(t, base)
}

func BenchmarkDo(b *testing.B) {
	var g Group[string, int]
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			g.Do("k", func() (int, error) { return 1, nil })
		}
	})
}
