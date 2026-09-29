package singleflight

import "errors"

// Result — результат, который DoChan отправляет в канал.
type Result[V any] struct {
	Val    V
	Err    error
	Shared bool // true, если результат получили несколько вызывающих
}

// ErrPanicked — ошибка, которую получают ожидающие вызывающие (и все
// получатели DoChan), если fn запаниковала. Возвращаемая ошибка оборачивает
// ErrPanicked и содержит значение паники.
var ErrPanicked = errors.New("singleflight: fn panicked")
