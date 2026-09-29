package unsafex

import "errors"

// Ошибки Reinterpret.
var (
	ErrNotPOD = errors.New("unsafex: разрешены только числовые типы без указателей")
	ErrSize   = errors.New("unsafex: размер данных не кратен размеру целевого типа")
	ErrAlign  = errors.New("unsafex: адрес не выровнен для целевого типа")
)
