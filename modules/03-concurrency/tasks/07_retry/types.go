package retry

import (
	"errors"
	"time"
)

// Policy — параметры повторов.
type Policy struct {
	Attempts   int           // максимум попыток (включая первую); <= 0 означает 1
	BaseDelay  time.Duration // задержка перед второй попыткой
	MaxDelay   time.Duration // верхняя граница задержки; 0 — без ограничения
	Multiplier float64       // множитель роста задержки; <= 1 означает 2
}

// permanentError помечает ошибку как неповторяемую.
type permanentError struct{ err error }

func (p *permanentError) Error() string { return p.err.Error() }
func (p *permanentError) Unwrap() error { return p.err }

// Permanent оборачивает err: Do прекратит повторы и вернёт err
// (errors.Is/As по исходной ошибке продолжают работать). Permanent(nil) == nil.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

// ErrExhausted возвращается (обёрнутой вместе с последней ошибкой f),
// когда все попытки исчерпаны.
var ErrExhausted = errors.New("retry: attempts exhausted")
