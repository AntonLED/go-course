package workerpool

// Result — результат обработки одного входного значения.
type Result[T, R any] struct {
	In  T     // исходная задача
	Out R     // результат f (нулевое значение при ошибке)
	Err error // ошибка f
}
