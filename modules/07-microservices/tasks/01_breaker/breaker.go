//go:build !solution

// Package breaker — circuit breaker (closed / open / half-open) с инжектируемыми часами.
package breaker

// Breaker — потокобезопасный circuit breaker.
//
// Подсказка по полям: mutex, текущее состояние, «поколение» (счётчик смен
// состояния), счётчики неудач/успехов, число активных пробных вызовов,
// момент размыкания.
type Breaker struct {
	s Settings
	// TODO: добавьте поля
}

// New создаёт Breaker в состоянии closed, подставляя значения по умолчанию
// для нулевых полей Settings.
func New(s Settings) *Breaker {
	// TODO: реализуйте
	return &Breaker{s: s}
}

// State возвращает текущее состояние. Если цепь open и OpenTimeout истёк
// (по часам Settings.Now), переход в half-open должен произойти прямо здесь.
func (b *Breaker) State() State {
	// TODO: реализуйте
	return StateOpen
}

// Execute выполняет fn, если цепь это позволяет, и учитывает результат:
//   - open → ErrOpen без вызова fn;
//   - half-open и лимит пробных вызовов занят → ErrTooManyRequests;
//   - результат вызова, начатого в другом «поколении» состояния, игнорируется;
//   - паника в fn — это неудача, панику нужно пробросить дальше.
func (b *Breaker) Execute(fn func() error) error {
	// TODO: реализуйте
	panic("TODO")
}
