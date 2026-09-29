//go:build !solution

package streams

import "io"

// NewRot13Reader возвращает io.Reader, который читает из r и применяет ROT13
// к латинским буквам (A–Z, a–z). Остальные байты (цифры, пробелы, UTF-8
// кириллица) проходят без изменений.
func NewRot13Reader(r io.Reader) io.Reader {
	panic("TODO")
}

// NewCountingWriter создаёт CountingWriter поверх w.
func NewCountingWriter(w io.Writer) *CountingWriter {
	panic("TODO")
}

// Write пишет p во вложенный writer и прибавляет к счётчику фактически
// записанное число байт (даже если вернулась ошибка).
func (c *CountingWriter) Write(p []byte) (int, error) {
	panic("TODO")
}

// Count возвращает количество записанных байт.
func (c *CountingWriter) Count() int64 {
	panic("TODO")
}

// LimitReader возвращает Reader, который отдаёт не больше n байт из r,
// после чего возвращает io.EOF. Нельзя читать из r больше n байт!
// Не используй io.LimitReader — напиши сам.
func LimitReader(r io.Reader, n int64) io.Reader {
	panic("TODO")
}

// MultiWriter возвращает Writer, дублирующий каждую запись во все ws по
// очереди. Если какой-то writer вернул ошибку — вернуть её сразу (дальше не
// писать). Если writer записал меньше len(p) без ошибки — вернуть
// io.ErrShortWrite. При успехе возвращается len(p), nil.
// Без аргументов ведёт себя как io.Discard.
func MultiWriter(ws ...io.Writer) io.Writer {
	panic("TODO")
}
