//go:build !solution

// Package miniyaml — парсер небольшого подмножества YAML в map[string]any.
package miniyaml

// Parse разбирает документ (подмножество YAML, см. README) и возвращает
// корневое отображение. Значения: map[string]any, []any, string, int,
// float64, bool, nil. Ошибки содержат номер строки: "строка 3: ...".
func Parse(src string) (map[string]any, error) {
	// TODO: 1) разбить на логические строки (отступ, текст, номер), выкинув
	//          пустые строки и комментарии; 2) рекурсивный спуск по отступам.
	panic("TODO")
}
