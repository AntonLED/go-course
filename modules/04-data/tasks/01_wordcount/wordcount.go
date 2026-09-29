//go:build !solution

package wordcount

import "io"

// Count потоково считает статистику по r.
//
// Подсказки: bufio.Scanner по умолчанию падает на строках длиннее 64 КиБ
// (bufio.ErrTooLong) — либо увеличьте буфер, либо используйте bufio.Reader.
// Ошибку чтения (кроме io.EOF) нужно вернуть вместе с тем, что успели насчитать.
func Count(r io.Reader) (Counts, error) {
	// TODO: реализуйте
	return Counts{}, nil
}
