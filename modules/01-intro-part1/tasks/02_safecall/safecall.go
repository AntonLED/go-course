//go:build !solution

// Package safecall — задача к уроку «Основы синтаксиса»:
// defer/panic/recover, именованные результаты, замыкания, variadic.
package safecall

// SafeCall вызывает f и превращает панику в ошибку *PanicError.
//   - f == nil         → ErrNilFunc (без вызова);
//   - f не паникует    → nil;
//   - f паникует с v   → &PanicError{Value: v}.
//
// Подсказка: recover работает только в отложенной функции, а изменить
// возвращаемое значение из defer можно только через именованный результат.
func SafeCall(f func()) (err error) {
	// TODO: реализуйте
	panic("TODO")
}

// Counter возвращает две функции, разделяющие одно состояние:
// next() возвращает start, start+step, start+2*step, ...;
// reset() возвращает счётчик к началу (следующий next() снова вернёт start).
// Разные вызовы Counter создают независимые счётчики.
func Counter(start, step int) (next func() int, reset func()) {
	// TODO: реализуйте
	panic("TODO")
}

// Compose возвращает функцию x → fs[len-1](...fs[1](fs[0](x))),
// то есть функции применяются слева направо. Без аргументов — тождественная.
// Изменение исходного среза fs после вызова Compose не должно влиять на результат.
func Compose(fs ...func(int) int) func(int) int {
	// TODO: реализуйте
	panic("TODO")
}
