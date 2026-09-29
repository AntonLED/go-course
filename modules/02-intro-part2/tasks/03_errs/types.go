package errs

import "errors"

// ErrNotFound — sentinel-ошибка «не найдено». Сравнивать через errors.Is.
var ErrNotFound = errors.New("not found")

// User — проверяемая сущность.
type User struct {
	Name  string
	Age   int
	Email string
}

// ValidationError — ошибка валидации одного поля.
type ValidationError struct {
	Field string
	Msg   string
}

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

// PanicError — паника, превращённая в ошибку функцией SafeCall.
type PanicError struct {
	Value any // значение, переданное в panic
}
