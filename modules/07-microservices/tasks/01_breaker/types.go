package breaker

import (
	"errors"
	"time"
)

// State — состояние автомата circuit breaker.
type State int

const (
	StateClosed   State = iota // запросы проходят, считаем подряд идущие ошибки
	StateOpen                  // запросы отбиваются сразу с ErrOpen
	StateHalfOpen              // пропускаем ограниченное число пробных запросов
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

var (
	// ErrOpen возвращается, когда цепь разомкнута и вызов не выполнялся.
	ErrOpen = errors.New("breaker: circuit is open")
	// ErrTooManyRequests возвращается в half-open, когда лимит пробных вызовов исчерпан.
	ErrTooManyRequests = errors.New("breaker: too many requests in half-open state")
)

// Settings — настройки Breaker. Нулевые значения заменяются значениями по умолчанию.
type Settings struct {
	// FailureThreshold — сколько подряд неудач в closed размыкает цепь (по умолчанию 5).
	FailureThreshold int
	// OpenTimeout — сколько цепь остаётся open перед переходом в half-open (по умолчанию 30s).
	OpenTimeout time.Duration
	// HalfOpenMaxCalls — максимум одновременных пробных вызовов в half-open (по умолчанию 1).
	HalfOpenMaxCalls int
	// SuccessThreshold — сколько успешных пробных вызовов подряд замыкает цепь (по умолчанию 1).
	SuccessThreshold int
	// IsFailure решает, считать ли ошибку неудачей. По умолчанию — любая err != nil.
	// Бизнес-ошибки (например, «не найдено») обычно НЕ должны размыкать цепь.
	IsFailure func(err error) bool
	// Now — источник времени (для тестов). По умолчанию time.Now.
	Now func() time.Time
	// OnStateChange вызывается при каждой смене состояния (может быть nil).
	// Вызывается под внутренней блокировкой — не вызывайте из него методы Breaker.
	OnStateChange func(from, to State)
}
