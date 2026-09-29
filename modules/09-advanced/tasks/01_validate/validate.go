//go:build !solution

package validate

// Validate проверяет структуру (или указатель на структуру) по тегам `validate:"..."`.
//
// Поддерживаемые правила (через запятую): required, omitempty, min=N, max=N,
// email, oneof=a b c. Тег "-" — пропустить поле.
//
// Возвращает:
//   - nil, если всё корректно;
//   - ValidationErrors со всеми нарушениями (порядок — порядок обхода полей);
//   - ошибку, оборачивающую ErrBadTag / ErrNotStruct, при ошибке программиста.
//
// Подсказки: reflect.Indirect, reflect.Type.Field(i).Tag.Get("validate"),
// StructField.IsExported(), reflect.Value.IsZero, utf8.RuneCountInString.
func Validate(v any) error {
	// TODO: реализуйте обход через reflect.
	panic("TODO")
}
