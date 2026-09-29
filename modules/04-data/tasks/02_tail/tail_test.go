package tail

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// countingRS считает, сколько байт реально прочитано.
type countingRS struct {
	rs   io.ReadSeeker
	read int64
}

func (c *countingRS) Read(p []byte) (int, error) {
	n, err := c.rs.Read(p)
	c.read += int64(n)
	return n, err
}

func (c *countingRS) Seek(off int64, whence int) (int64, error) { return c.rs.Seek(off, whence) }

func TestTail(t *testing.T) {
	long := strings.Repeat("ж", 5000) // 10000 байт — строка длиннее нескольких блоков
	tests := []struct {
		name string
		in   string
		n    int
		want []string
	}{
		{"пусто", "", 3, nil},
		{"n=0", "a\nb\n", 0, nil},
		{"меньше строк чем n", "a\nb\n", 5, []string{"a", "b"}},
		{"ровно n", "a\nb\nc", 3, []string{"a", "b", "c"}},
		{"последние 2", "1\n2\n3\n4\n", 2, []string{"3", "4"}},
		{"без финального \\n", "1\n2\n3", 1, []string{"3"}},
		{"только \\n", "\n", 2, []string{""}},
		{"пустые строки", "a\n\n\n", 2, []string{"", ""}},
		{"CRLF", "a\r\nb\r\nc\r\n", 2, []string{"b", "c"}},
		{"длинная строка", "x\n" + long + "\ny\n", 2, []string{long, "y"}},
		{"длинная строка целиком", long, 1, []string{long}},
		{"граница блока", strings.Repeat("a", ChunkSize-1) + "\nb\n", 2, []string{strings.Repeat("a", ChunkSize-1), "b"}},
		{"граница блока 2", strings.Repeat("a", ChunkSize) + "\nb", 1, []string{"b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := strings.NewReader(tt.in)
			r.Seek(1, io.SeekStart) // текущая позиция не должна влиять
			got, err := Tail(r, tt.n)
			if err != nil {
				t.Fatalf("неожиданная ошибка: %v", err)
			}
			if len(got) != len(tt.want) || (len(got) > 0 && !slices.Equal(got, tt.want)) {
				t.Errorf("Tail(n=%d) = %q, ожидалось %q", tt.n, short(got), short(tt.want))
			}
		})
	}
}

func short(s []string) []string {
	out := make([]string, len(s))
	for i, x := range s {
		if len(x) > 20 {
			x = fmt.Sprintf("%s…(%d байт)", x[:10], len(x))
		}
		out[i] = x
	}
	return out
}

func TestTailReadsOnlyTheEnd(t *testing.T) {
	var sb strings.Builder
	for i := range 100_000 {
		fmt.Fprintf(&sb, "line %d\n", i)
	}
	in := sb.String() // ~1.2 МБ
	c := &countingRS{rs: strings.NewReader(in)}
	got, err := Tail(c, 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{}
	for i := 99_990; i < 100_000; i++ {
		want = append(want, fmt.Sprintf("line %d", i))
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Tail = %q, ожидалось %q", got, want)
	}
	if c.read > 4*ChunkSize {
		t.Errorf("прочитано %d байт из %d — Tail должен читать только хвост", c.read, len(in))
	}
}

func TestTailFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "log.txt")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := Tail(f, 2)
	if err != nil || !slices.Equal(got, []string{"two", "three"}) {
		t.Errorf("Tail(file) = %q, %v", got, err)
	}
}

func TestTailNegative(t *testing.T) {
	if _, err := Tail(strings.NewReader("x"), -1); !errors.Is(err, ErrNegative) {
		t.Errorf("n=-1: ожидалось ErrNegative, получено %v", err)
	}
}
