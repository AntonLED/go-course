package envconfig

import (
	"errors"
	"fmt"
)

// LookupFunc совпадает по сигнатуре с os.LookupEnv — в тестах подменяется картой.
type LookupFunc func(key string) (string, bool)

var (
	// ErrInvalidTarget — dst не является ненулевым указателем на структуру.
	ErrInvalidTarget = errors.New("envconfig: ожидается ненулевой указатель на структуру")
	// ErrRequired — обязательная переменная не задана или пуста.
	ErrRequired = errors.New("обязательная переменная не задана")
	// ErrUnsupported — тип поля не поддерживается.
	ErrUnsupported = errors.New("неподдерживаемый тип поля")
)

// FieldError — ошибка заполнения конкретного поля.
type FieldError struct {
	Field string // путь к полю: "DB.Port"
	Key   string // имя переменной окружения: "DB_PORT"
	Err   error
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("envconfig: поле %s (%s): %v", e.Field, e.Key, e.Err)
}

func (e *FieldError) Unwrap() error { return e.Err }
