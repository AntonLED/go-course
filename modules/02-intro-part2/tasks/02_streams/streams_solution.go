//go:build solution

package streams

import "io"

type rot13Reader struct{ r io.Reader }

func rot13(b byte) byte {
	switch {
	case b >= 'a' && b <= 'z':
		return 'a' + (b-'a'+13)%26
	case b >= 'A' && b <= 'Z':
		return 'A' + (b-'A'+13)%26
	}
	return b
}

// Read сначала читает из вложенного reader'а, затем преобразует ровно n
// прочитанных байт. Важно обработать данные ДО проверки err: контракт
// io.Reader допускает n > 0 вместе с err != nil (в том числе io.EOF).
func (r rot13Reader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	for i := range p[:n] {
		p[i] = rot13(p[i])
	}
	return n, err
}

func NewRot13Reader(r io.Reader) io.Reader { return rot13Reader{r} }

func NewCountingWriter(w io.Writer) *CountingWriter { return &CountingWriter{w: w} }

func (c *CountingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

func (c *CountingWriter) Count() int64 { return c.n }

type limitReader struct {
	r io.Reader
	n int64 // сколько ещё можно прочитать
}

func (l *limitReader) Read(p []byte) (int, error) {
	if l.n <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > l.n {
		p = p[:l.n] // обрезаем буфер, чтобы не прочитать лишнего из l.r
	}
	n, err := l.r.Read(p)
	l.n -= int64(n)
	return n, err
}

func LimitReader(r io.Reader, n int64) io.Reader { return &limitReader{r: r, n: n} }

type multiWriter []io.Writer

func (m multiWriter) Write(p []byte) (int, error) {
	for _, w := range m {
		n, err := w.Write(p)
		if err != nil {
			return n, err
		}
		if n != len(p) {
			return n, io.ErrShortWrite
		}
	}
	return len(p), nil
}

func MultiWriter(ws ...io.Writer) io.Writer {
	// Копируем срез: вызывающий может потом изменить свой массив.
	return multiWriter(append([]io.Writer(nil), ws...))
}
