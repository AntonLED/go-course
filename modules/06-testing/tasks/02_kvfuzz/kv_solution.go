//go:build solution

package kv

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

func validKey(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == '-') {
			return false
		}
	}
	return true
}

// bareByte — байт, допустимый в значении без кавычек.
func bareByte(c byte) bool {
	return c > ' ' && c < 0x7f && c != '"' && c != '=' && c != '\\'
}

func bare(v string) bool {
	if v == "" {
		return false
	}
	for i := 0; i < len(v); i++ {
		if !bareByte(v[i]) {
			return false
		}
	}
	return true
}

// Encode сериализует карту детерминированно (ключи отсортированы).
func Encode(m map[string]string) (string, error) {
	keys := make([]string, 0, len(m))
	for k := range m {
		if !validKey(k) {
			return "", fmt.Errorf("%w: %q", ErrInvalidKey, k)
		}
		keys = append(keys, k)
	}
	slices.Sort(keys) // порядок итерации map случаен — без сортировки golden-тесты флакали бы

	var b strings.Builder
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(k)
		b.WriteByte('=')
		if v := m[k]; bare(v) {
			b.WriteString(v)
		} else {
			// strconv.Quote экранирует кавычки, '\', управляющие символы и невалидный UTF-8 (\xff),
			// а Unquote восстанавливает исходные байты — round trip сохраняется.
			b.WriteString(strconv.Quote(v))
		}
	}
	return b.String(), nil
}

// Decode разбирает строку.
func Decode(s string) (map[string]string, error) {
	m := make(map[string]string)
	i := 0
	for {
		for i < len(s) && s[i] == ' ' {
			i++
		}
		if i == len(s) {
			return m, nil
		}
		// ключ
		start := i
		for i < len(s) && s[i] != '=' && s[i] != ' ' {
			i++
		}
		key := s[start:i]
		if i == len(s) || s[i] != '=' {
			return nil, fmt.Errorf("%w: нет '=' после %q (позиция %d)", ErrSyntax, key, start)
		}
		if !validKey(key) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidKey, key)
		}
		i++ // '='

		// значение
		var val string
		if i < len(s) && s[i] == '"' {
			j := i + 1
			for j < len(s) && s[j] != '"' {
				if s[j] == '\\' {
					j++ // пропускаем экранированный символ
				}
				j++
			}
			if j >= len(s) {
				return nil, fmt.Errorf("%w: незакрытая кавычка у ключа %q", ErrSyntax, key)
			}
			v, err := strconv.Unquote(s[i : j+1])
			if err != nil {
				return nil, fmt.Errorf("%w: значение %q: %v", ErrSyntax, key, err)
			}
			val = v
			i = j + 1
			if i < len(s) && s[i] != ' ' {
				return nil, fmt.Errorf("%w: мусор после кавычки у ключа %q", ErrSyntax, key)
			}
		} else {
			vs := i
			for i < len(s) && s[i] != ' ' {
				if !bareByte(s[i]) {
					return nil, fmt.Errorf("%w: недопустимый байт %q в значении %q", ErrSyntax, s[i], key)
				}
				i++
			}
			if i == vs {
				return nil, fmt.Errorf("%w: пустое значение у ключа %q (нужно \"\")", ErrSyntax, key)
			}
			val = s[vs:i]
		}
		if _, dup := m[key]; dup {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateKey, key)
		}
		m[key] = val
	}
}
