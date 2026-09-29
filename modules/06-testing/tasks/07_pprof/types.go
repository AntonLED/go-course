package profiling

import "errors"

// FuncCount — функция и число горутин, «стоящих» в ней.
type FuncCount struct {
	Func  string
	Count int
}

// ErrMalformed — профиль не удалось разобрать.
var ErrMalformed = errors.New("profiling: malformed goroutine profile")
