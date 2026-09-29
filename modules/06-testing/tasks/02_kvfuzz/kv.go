//go:build !solution

package kv

import "errors"

// Encode сериализует m в строку вида `a=1 b="hello world"` (ключи по возрастанию).
// Подробный формат — в README.
func Encode(m map[string]string) (string, error) {
	// TODO
	return "", errors.New("TODO")
}

// Decode разбирает строку, созданную Encode (и чуть более свободную: лишние пробелы).
// Для пустой строки возвращает пустую (не nil) карту.
func Decode(s string) (map[string]string, error) {
	// TODO
	return nil, errors.New("TODO")
}
