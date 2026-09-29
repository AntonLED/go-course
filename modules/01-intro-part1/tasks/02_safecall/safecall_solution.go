//go:build solution

// Package safecall — задача к уроку «Основы синтаксиса»:
// defer/panic/recover, именованные результаты, замыкания, variadic.
package safecall

import "slices"

// SafeCall вызывает f и превращает панику в ошибку *PanicError.
func SafeCall(f func()) (err error) {
	if f == nil {
		return ErrNilFunc
	}
	defer func() {
		// recover != nil — была паника. Начиная с Go 1.21, panic(nil)
		// превращается в *runtime.PanicNilError, так что recover()
		// никогда не вернёт nil при настоящей панике.
		if r := recover(); r != nil {
			err = &PanicError{Value: r} // пишем в именованный результат
		}
	}()
	f()
	return nil
}

// Counter возвращает замыкания над общей переменной cur.
func Counter(start, step int) (next func() int, reset func()) {
	cur := start
	next = func() int {
		v := cur
		cur += step
		return v
	}
	reset = func() { cur = start }
	return next, reset
}

// Compose применяет функции слева направо.
func Compose(fs ...func(int) int) func(int) int {
	// variadic-параметр — это обычный срез; если вызвали Compose(list...),
	// он разделяет backing array с list. Копируем, чтобы защититься от изменений.
	fs = slices.Clone(fs)
	return func(x int) int {
		for _, f := range fs {
			x = f(x)
		}
		return x
	}
}
