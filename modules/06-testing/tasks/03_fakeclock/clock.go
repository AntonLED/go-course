package clock

import "time"

// Clock — всё, что коду нужно от времени. Объявлен там, где используется,
// и достаточно мал, чтобы его было легко подменить в тестах.
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// Real — настоящие часы для продакшена.
type Real struct{}

func (Real) Now() time.Time                         { return time.Now() }
func (Real) After(d time.Duration) <-chan time.Time { return time.After(d) }
