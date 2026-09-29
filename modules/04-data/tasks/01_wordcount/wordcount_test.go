package wordcount

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestCount(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want Counts
	}{
		{"пусто", "", Counts{}},
		{"одна строка без \\n", "hello world", Counts{Lines: 1, Words: 2, Bytes: 11, Runes: 11, MaxLineLen: 11}},
		{"одна пустая строка", "\n", Counts{Lines: 1, Bytes: 1, Runes: 1}},
		{"две строки", "a b\nccc\n", Counts{Lines: 2, Words: 3, Bytes: 8, Runes: 8, MaxLineLen: 3}},
		{"кириллица", "привет, мир\n", Counts{Lines: 1, Words: 2, Bytes: 21, Runes: 12, MaxLineLen: 11}},
		{"CRLF", "ab\r\ncd\r\n", Counts{Lines: 2, Words: 2, Bytes: 8, Runes: 8, MaxLineLen: 2}},
		{"\\r в середине", "a\rb\n", Counts{Lines: 1, Words: 2, Bytes: 4, Runes: 4, MaxLineLen: 3}},
		{"два \\r перед \\n", "a\r\r\n", Counts{Lines: 1, Words: 1, Bytes: 4, Runes: 4, MaxLineLen: 2}},
		{"пробелы разных видов", " \t a b　c  \n\n", Counts{Lines: 2, Words: 3, Bytes: 15, Runes: 12, MaxLineLen: 10}},
		{"невалидный UTF-8", "a\xffb\n", Counts{Lines: 1, Words: 1, Bytes: 4, Runes: 4, MaxLineLen: 3}},
		{"хвост без \\n", "x\ny", Counts{Lines: 2, Words: 2, Bytes: 3, Runes: 3, MaxLineLen: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// OneByteReader проверяет, что слова/руны не «рвутся» на границах чтения.
			for _, r := range []io.Reader{strings.NewReader(tt.in), iotest.OneByteReader(strings.NewReader(tt.in))} {
				got, err := Count(r)
				if err != nil {
					t.Fatalf("Count(%q): неожиданная ошибка %v", tt.in, err)
				}
				if got != tt.want {
					t.Errorf("Count(%q) = %+v, ожидалось %+v", tt.in, got, tt.want)
				}
			}
		})
	}
}

func TestCountLongLine(t *testing.T) {
	long := strings.Repeat("ю", 300_000) // ~600 КиБ в одной строке — больше лимита Scanner
	in := "short\n" + long + " end\nlast"
	got, err := Count(strings.NewReader(in))
	if err != nil {
		t.Fatalf("длинная строка: неожиданная ошибка %v (bufio.Scanner с буфером по умолчанию?)", err)
	}
	want := Counts{Lines: 3, Words: 4, Bytes: len(in), Runes: 6 + 300_000 + 5 + 4, MaxLineLen: 300_004}
	if got != want {
		t.Errorf("Count = %+v, ожидалось %+v", got, want)
	}
}

func TestCountReadError(t *testing.T) {
	boom := errors.New("диск отвалился")
	r := io.MultiReader(strings.NewReader("one two\nthree"), iotest.ErrReader(boom))
	got, err := Count(r)
	if !errors.Is(err, boom) {
		t.Fatalf("ожидалась ошибка %v, получено %v", boom, err)
	}
	if got.Words != 3 || got.Bytes != 13 {
		t.Errorf("при ошибке нужно вернуть то, что успели посчитать: %+v", got)
	}
}

func BenchmarkCount(b *testing.B) {
	in := strings.Repeat("the quick brown fox jumps over the lazy dog\n", 10_000)
	b.SetBytes(int64(len(in)))
	for i := 0; i < b.N; i++ {
		if _, err := Count(strings.NewReader(in)); err != nil {
			b.Fatal(err)
		}
	}
}
