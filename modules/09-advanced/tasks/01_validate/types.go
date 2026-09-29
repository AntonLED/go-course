package validate

import (
	"errors"
	"fmt"
	"strings"
)

// Сентинел-ошибки. Проверяются через errors.Is.
var (
	// ErrNotStruct возвращается, если в Validate передана не структура
	// (и не ненулевой указатель на структуру).
	ErrNotStruct = errors.New("validate: ожидается структура или указатель на структуру")
	// ErrBadTag возвращается при неизвестном правиле или некорректном параметре
	// правила (например, min=abc или min на поле типа bool).
	ErrBadTag = errors.New("validate: некорректный тег")
)

// FieldError описывает одно нарушенное правило.
type FieldError struct {
	Field string // путь к полю: "Name", "Address.City", "Items[2].SKU"
	Rule  string // имя правила: "required", "min", "max", "email", "oneof"
	Param string // параметр правила ("3" для min=3), может быть пустым
}

func (e FieldError) Error() string {
	if e.Param == "" {
		return fmt.Sprintf("%s: нарушено правило %s", e.Field, e.Rule)
	}
	return fmt.Sprintf("%s: нарушено правило %s=%s", e.Field, e.Rule, e.Param)
}

// ValidationErrors — все нарушения, найденные в структуре, в порядке обхода полей.
type ValidationErrors []FieldError

func (v ValidationErrors) Error() string {
	parts := make([]string, len(v))
	for i, e := range v {
		parts[i] = e.Error()
	}
	return strings.Join(parts, "; ")
}
