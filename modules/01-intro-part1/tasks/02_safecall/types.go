package safecall

import (
	"errors"
	"fmt"
)

// ErrNilFunc возвращается SafeCall, если передана nil-функция.
var ErrNilFunc = errors.New("safecall: nil func")

// PanicError — ошибка, в которую SafeCall превращает перехваченную панику.
type PanicError struct {
	Value any // значение, переданное в panic (результат recover())
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("panic: %v", e.Value)
}

// Unwrap позволяет errors.Is/errors.As «заглянуть» внутрь,
// если паника была вызвана значением типа error.
func (e *PanicError) Unwrap() error {
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}
