//go:build solution

// Package singleflight — собственная обобщённая реализация
// golang.org/x/sync/singleflight.
package singleflight

import (
	"fmt"
	"sync"
)

// call — один выполняющийся (или завершённый) вызов fn.
type call[V any] struct {
	wg sync.WaitGroup // Done после записи val/err: даёт happens-before для ожидающих

	val V
	err error

	dups   int                // число присоединившихся Do (под Group.mu)
	chans  []chan<- Result[V] // получатели DoChan (под Group.mu)
	shared bool               // итоговый флаг, вычисляется под Group.mu
}

// Group подавляет дублирующиеся конкурентные вызовы с одинаковым ключом.
type Group[K comparable, V any] struct {
	mu sync.Mutex
	m  map[K]*call[V] // ленивая инициализация
}

// Do выполняет fn один раз на все конкурентные вызовы с одним key.
func (g *Group[K, V]) Do(key K, fn func() (V, error)) (v V, err error, shared bool) {
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[K]*call[V])
	}
	if c, ok := g.m[key]; ok {
		// Вызов уже идёт — присоединяемся и ждём.
		c.dups++
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, c.err, true
	}
	c := new(call[V])
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	g.doCall(c, key, fn, true)
	return c.val, c.err, c.shared
}

// DoChan — асинхронный вариант Do.
func (g *Group[K, V]) DoChan(key K, fn func() (V, error)) <-chan Result[V] {
	ch := make(chan Result[V], 1) // буфер: отправка в doCall никогда не блокирует
	g.mu.Lock()
	if g.m == nil {
		g.m = make(map[K]*call[V])
	}
	if c, ok := g.m[key]; ok {
		c.dups++
		c.chans = append(c.chans, ch)
		g.mu.Unlock()
		return ch
	}
	c := &call[V]{chans: []chan<- Result[V]{ch}}
	c.wg.Add(1)
	g.m[key] = c
	g.mu.Unlock()

	go g.doCall(c, key, fn, false)
	return ch
}

// doCall выполняет fn и раздаёт результат. rethrow — повторно паниковать
// в вызывающей горутине (для Do); в DoChan паника в отдельной горутине
// уронила бы весь процесс, поэтому там она превращается в ошибку.
func (g *Group[K, V]) doCall(c *call[V], key K, fn func() (V, error), rethrow bool) {
	normal := false
	defer func() {
		var r any
		if !normal {
			r = recover() // nil при runtime.Goexit
			var zero V
			if r != nil {
				c.val, c.err = zero, fmt.Errorf("%w: %v", ErrPanicked, r)
			} else {
				// fn вызвала runtime.Goexit: ожидающие не должны зависнуть,
				// поэтому они тоже получают ошибку.
				c.val, c.err = zero, fmt.Errorf("%w: fn called runtime.Goexit", ErrPanicked)
			}
		}

		g.mu.Lock()
		// Ключ мог быть забыт через Forget и занят новым вызовом —
		// удаляем только «свою» запись.
		if g.m[key] == c {
			delete(g.m, key)
		}
		c.shared = c.dups > 0
		c.wg.Done()
		for _, ch := range c.chans {
			ch <- Result[V]{Val: c.val, Err: c.err, Shared: c.shared}
		}
		g.mu.Unlock()

		if r != nil && rethrow {
			panic(r)
		}
	}()

	c.val, c.err = fn()
	normal = true
}

// Forget забывает key.
func (g *Group[K, V]) Forget(key K) {
	g.mu.Lock()
	delete(g.m, key) // delete из nil-map — no-op
	g.mu.Unlock()
}
