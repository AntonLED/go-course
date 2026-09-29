package gen

// Number — ограничение для числовых типов. Тильда (~) означает «любой тип,
// у которого базовый (underlying) тип такой»: подойдёт и `type Celsius float64`.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64
}

// Set — множество элементов comparable-типа. Нулевое значение готово к
// использованию: var s Set[string]; s.Add("a").
type Set[T comparable] struct {
	m map[T]struct{}
}

// Stack — LIFO-стек. Нулевое значение готово к использованию.
type Stack[T any] struct {
	items []T
}
