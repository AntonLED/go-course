package kv

import "errors"

// Ошибки пакета. Возвращайте их обёрнутыми (fmt.Errorf("...: %w", ErrSyntax)),
// чтобы вызывающий мог проверить errors.Is.
var (
	ErrInvalidKey   = errors.New("kv: invalid key")
	ErrSyntax       = errors.New("kv: syntax error")
	ErrDuplicateKey = errors.New("kv: duplicate key")
)
