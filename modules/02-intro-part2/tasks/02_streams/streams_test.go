package streams

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

var _ io.Writer = (*CountingWriter)(nil)

func TestRot13(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"Hello, World!", "Uryyb, Jbeyq!"},
		{"abcxyzABCXYZ", "nopklmNOPKLM"},
		{"Go 1.23 — Гофер", "Tb 1.23 — Гофер"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := io.ReadAll(NewRot13Reader(strings.NewReader(tt.in)))
			if err != nil || string(got) != tt.want {
				t.Fatalf("rot13(%q) = %q, %v; ожидалось %q, nil", tt.in, got, err, tt.want)
			}
			// iotest.TestReader проверяет контракт io.Reader (маленькие буферы и т.п.).
			if err := iotest.TestReader(NewRot13Reader(strings.NewReader(tt.in)), []byte(tt.want)); err != nil {
				t.Errorf("нарушен контракт io.Reader: %v", err)
			}
			// Данные вместе с EOF в одном вызове Read.
			r := NewRot13Reader(iotest.DataErrReader(iotest.OneByteReader(strings.NewReader(tt.in))))
			got, err = io.ReadAll(r)
			if err != nil || string(got) != tt.want {
				t.Errorf("rot13 с DataErrReader = %q, %v; ожидалось %q", got, err, tt.want)
			}
		})
	}
	// Двойной ROT13 — тождественное преобразование.
	s := "The Quick Brown Fox"
	got, _ := io.ReadAll(NewRot13Reader(NewRot13Reader(strings.NewReader(s))))
	if string(got) != s {
		t.Errorf("rot13(rot13(%q)) = %q", s, got)
	}
}

func TestRot13PropagatesError(t *testing.T) {
	boom := errors.New("boom")
	r := NewRot13Reader(io.MultiReader(strings.NewReader("abc"), iotest.ErrReader(boom)))
	got, err := io.ReadAll(r)
	if string(got) != "nop" || !errors.Is(err, boom) {
		t.Errorf("ReadAll = %q, %v; ожидалось \"nop\", boom", got, err)
	}
}

type failingWriter struct {
	limit int // сколько байт примет до ошибки
	err   error
	buf   bytes.Buffer
}

func (f *failingWriter) Write(p []byte) (int, error) {
	if len(p) <= f.limit {
		f.limit -= len(p)
		return f.buf.Write(p)
	}
	n := f.limit
	f.buf.Write(p[:n])
	f.limit = 0
	return n, f.err
}

func TestCountingWriter(t *testing.T) {
	var buf bytes.Buffer
	cw := NewCountingWriter(&buf)
	if cw.Count() != 0 {
		t.Fatalf("новый CountingWriter: Count() = %d, ожидалось 0", cw.Count())
	}
	io.WriteString(cw, "hello, ")
	io.WriteString(cw, "мир")
	if cw.Count() != int64(len("hello, мир")) || buf.String() != "hello, мир" {
		t.Errorf("Count() = %d, buf = %q; ожидалось %d, %q", cw.Count(), buf.String(), len("hello, мир"), "hello, мир")
	}

	boom := errors.New("диск переполнен")
	fw := &failingWriter{limit: 3, err: boom}
	cw = NewCountingWriter(fw)
	n, err := cw.Write([]byte("abcdef"))
	if n != 3 || !errors.Is(err, boom) || cw.Count() != 3 {
		t.Errorf("Write при ошибке: n=%d err=%v Count=%d; ожидалось 3, boom, 3", n, err, cw.Count())
	}
}

type countingReader struct {
	r    io.Reader
	read int
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.read += n
	return n, err
}

func TestLimitReader(t *testing.T) {
	tests := []struct {
		in   string
		n    int64
		want string
	}{
		{"hello world", 5, "hello"},
		{"hi", 10, "hi"},
		{"hello", 0, ""},
		{"hello", -3, ""},
		{"", 3, ""},
	}
	for _, tt := range tests {
		src := &countingReader{r: strings.NewReader(tt.in)}
		got, err := io.ReadAll(LimitReader(src, tt.n))
		if err != nil || string(got) != tt.want {
			t.Errorf("LimitReader(%q, %d) = %q, %v; ожидалось %q", tt.in, tt.n, got, err, tt.want)
		}
		if src.read > len(tt.want) {
			t.Errorf("LimitReader(%q, %d) прочитал из источника %d байт — больше лимита", tt.in, tt.n, src.read)
		}
		if err := iotest.TestReader(LimitReader(strings.NewReader(tt.in), tt.n), []byte(tt.want)); err != nil {
			t.Errorf("LimitReader(%q, %d): нарушен контракт io.Reader: %v", tt.in, tt.n, err)
		}
	}
}

func TestMultiWriter(t *testing.T) {
	var a, b bytes.Buffer
	w := MultiWriter(&a, &b)
	n, err := io.WriteString(w, "дубль")
	if n != len("дубль") || err != nil || a.String() != "дубль" || b.String() != "дубль" {
		t.Errorf("MultiWriter: n=%d err=%v a=%q b=%q", n, err, a.String(), b.String())
	}

	// Без писателей — как io.Discard.
	if n, err := MultiWriter().Write([]byte("abc")); n != 3 || err != nil {
		t.Errorf("MultiWriter().Write = %d, %v; ожидалось 3, nil", n, err)
	}

	// Ошибка первого писателя останавливает запись.
	boom := errors.New("boom")
	var c bytes.Buffer
	_, err = MultiWriter(&failingWriter{limit: 1, err: boom}, &c).Write([]byte("xyz"))
	if !errors.Is(err, boom) || c.Len() != 0 {
		t.Errorf("ошибка: err=%v, во второй writer записано %q; ожидалось boom и пусто", err, c.String())
	}

	// Короткая запись без ошибки -> io.ErrShortWrite.
	_, err = MultiWriter(shortWriter{}).Write([]byte("xyz"))
	if !errors.Is(err, io.ErrShortWrite) {
		t.Errorf("короткая запись: err=%v, ожидалось io.ErrShortWrite", err)
	}

	// Композиция: считаем байты, одновременно пишем в буфер.
	var out bytes.Buffer
	cw := NewCountingWriter(io.Discard)
	io.Copy(MultiWriter(&out, cw), NewRot13Reader(strings.NewReader("abc")))
	if out.String() != "nop" || cw.Count() != 3 {
		t.Errorf("композиция: out=%q count=%d", out.String(), cw.Count())
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) / 2, nil }
