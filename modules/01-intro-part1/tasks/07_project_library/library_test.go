package library

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func fixture(t *testing.T) *Library {
	t.Helper()
	l := New()
	books := []Book{
		{ISBN: "978-5-17-090335-2", Title: "Война и мир", Author: "Лев Толстой", Year: 1869, Tags: []string{"Роман", "классика"}},
		{ISBN: "978 5 389 07435 4", Title: "Анна Каренина", Author: "Лев Толстой", Year: 1877, Tags: []string{"роман", " классика ", "КЛАССИКА"}},
		{ISBN: "0-306-40615-X", Title: "The Go Programming Language", Author: "Alan Donovan", Year: 2015, Tags: []string{"go", "programming"}},
		{ISBN: "1234567890", Title: "  Преступление и наказание ", Author: "Фёдор Достоевский", Year: 1866, Tags: []string{"роман", ""}},
		{ISBN: "111", Title: "go in action", Author: "William Kennedy", Year: 2015, Tags: []string{"Go"}},
	}
	for _, b := range books {
		if err := l.Add(b); err != nil {
			t.Fatalf("Add(%q): неожиданная ошибка %v", b.Title, err)
		}
	}
	return l
}

func titles(bs []Book) []string {
	res := make([]string, len(bs))
	for i, b := range bs {
		res[i] = b.Title
	}
	return res
}

func TestAddValidation(t *testing.T) {
	l := New()
	bad := []Book{
		{ISBN: "", Title: "T", Author: "A"},
		{ISBN: "- -", Title: "T", Author: "A"},
		{ISBN: "12a4", Title: "T", Author: "A"},
		{ISBN: "12X4", Title: "T", Author: "A"},
		{ISBN: "123", Title: "   ", Author: "A"},
		{ISBN: "123", Title: "T", Author: ""},
	}
	for _, b := range bad {
		if err := l.Add(b); !errors.Is(err, ErrInvalidBook) {
			t.Errorf("Add(%+v) = %v, ожидалось ErrInvalidBook", b, err)
		}
	}
	if err := l.Add(Book{ISBN: "0-306-40615-x", Title: "T", Author: "A"}); err != nil {
		t.Fatalf("ISBN с 'x' в конце должен приниматься: %v", err)
	}
	if err := l.Add(Book{ISBN: "030640615X", Title: "Другая", Author: "B"}); !errors.Is(err, ErrDuplicate) {
		t.Errorf("повтор ISBN после нормализации: err = %v, ожидалось ErrDuplicate", err)
	}
	b, ok := l.Get("0306 40615 X")
	if !ok || b.ISBN != "030640615X" {
		t.Errorf("Get по ненормализованному ISBN: %+v, %v", b, ok)
	}
}

func TestAddNormalizes(t *testing.T) {
	l := fixture(t)
	b, ok := l.Get("978-5-389-07435-4")
	if !ok {
		t.Fatal("книга не найдена по ISBN с дефисами")
	}
	if b.ISBN != "9785389074354" {
		t.Errorf("ISBN = %q, ожидалось нормализованное %q", b.ISBN, "9785389074354")
	}
	if want := []string{"роман", "классика"}; !slices.Equal(b.Tags, want) {
		t.Errorf("Tags = %q, ожидалось %q (trim, lower, без повторов)", b.Tags, want)
	}
	p, _ := l.Get("1234567890")
	if p.Title != "Преступление и наказание" {
		t.Errorf("Title не обрезан: %q", p.Title)
	}
	if want := []string{"роман"}; !slices.Equal(p.Tags, want) {
		t.Errorf("пустые теги должны выбрасываться: %q", p.Tags)
	}
	if _, ok := l.Get("nope"); ok {
		t.Error("Get несуществующей книги вернул ok=true")
	}
}

func TestNoAliasing(t *testing.T) {
	l := New()
	tags := make([]string, 2, 10)
	tags[0], tags[1] = "a", "b"
	if err := l.Add(Book{ISBN: "1", Title: "T", Author: "A", Tags: tags}); err != nil {
		t.Fatal(err)
	}
	tags[0] = "HACKED"
	_ = append(tags, "extra")

	b, _ := l.Get("1")
	if !slices.Equal(b.Tags, []string{"a", "b"}) {
		t.Fatalf("библиотека разделяет Tags с вызывающим: %q", b.Tags)
	}
	b.Tags[0] = "HACKED"
	b.Title = "changed"
	if again, _ := l.Get("1"); again.Tags[0] != "a" || again.Title != "T" {
		t.Errorf("Get вернул не копию: %+v", again)
	}
	found := l.Search("")
	found[0].Tags[1] = "HACKED"
	if again, _ := l.Get("1"); again.Tags[1] != "b" {
		t.Errorf("Search вернул книги, разделяющие Tags с библиотекой: %+v", again)
	}
}

func TestCheckoutReturn(t *testing.T) {
	l := fixture(t)
	if err := l.Checkout("111", "  "); !errors.Is(err, ErrInvalidReader) {
		t.Errorf("пустой читатель: %v", err)
	}
	if err := l.Checkout("999", "Иван"); !errors.Is(err, ErrNotFound) {
		t.Errorf("несуществующая книга: %v", err)
	}
	if err := l.Checkout("1-1-1", "Иван"); err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if err := l.Checkout("111", "Пётр"); !errors.Is(err, ErrCheckedOut) {
		t.Errorf("повторная выдача: %v, ожидалось ErrCheckedOut", err)
	}
	if err := l.Return("111"); err != nil {
		t.Errorf("Return: %v", err)
	}
	if err := l.Return("111"); !errors.Is(err, ErrNotCheckedOut) {
		t.Errorf("повторный Return: %v, ожидалось ErrNotCheckedOut", err)
	}
	if err := l.Return("000"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Return несуществующей: %v", err)
	}
	if err := l.Checkout("111", "Пётр"); err != nil {
		t.Errorf("выдача после возврата: %v", err)
	}
}

func TestSearch(t *testing.T) {
	l := fixture(t)
	tests := []struct {
		q    string
		want []string
	}{
		{"", []string{"go in action", "The Go Programming Language", "Анна Каренина", "Война и мир", "Преступление и наказание"}},
		{"ТОЛСТОЙ", []string{"Анна Каренина", "Война и мир"}},
		{"  go ", []string{"go in action", "The Go Programming Language"}},
		{"ёдор", []string{"Преступление и наказание"}},
		{"и", []string{"Анна Каренина", "Война и мир", "Преступление и наказание"}},
		{"нет такого", []string{}},
	}
	for _, tt := range tests {
		for range 5 { // порядок не должен зависеть от обхода map
			got := l.Search(tt.q)
			if got == nil && len(tt.want) == 0 {
				continue
			}
			if !slices.Equal(titles(got), tt.want) {
				t.Fatalf("Search(%q) = %q, ожидалось %q", tt.q, titles(got), tt.want)
			}
		}
	}
}

func TestSearchTieByISBN(t *testing.T) {
	l := New()
	_ = l.Add(Book{ISBN: "30", Title: "Same", Author: "A"})
	_ = l.Add(Book{ISBN: "10", Title: "same", Author: "B"})
	_ = l.Add(Book{ISBN: "20", Title: "SAME", Author: "C"})
	got := l.Search("")
	var isbns []string
	for _, b := range got {
		isbns = append(isbns, b.ISBN)
	}
	if want := []string{"10", "20", "30"}; !slices.Equal(isbns, want) {
		t.Errorf("при равных названиях порядок по ISBN: %v, ожидалось %v", isbns, want)
	}
}

func TestAuthorStats(t *testing.T) {
	l := fixture(t)
	_ = l.Checkout("9785170903352", "Анна")
	got := l.AuthorStats()
	want := []AuthorStat{
		{"Лев Толстой", 2, 1},
		{"Alan Donovan", 1, 0},
		{"William Kennedy", 1, 0},
		{"Фёдор Достоевский", 1, 0},
	}
	if !slices.Equal(got, want) {
		t.Errorf("AuthorStats = %+v\nожидалось %+v", got, want)
	}
	if got := New().AuthorStats(); len(got) != 0 {
		t.Errorf("AuthorStats пустой библиотеки = %v", got)
	}
}

func TestTopTags(t *testing.T) {
	l := fixture(t)
	want := []TagCount{{"роман", 3}, {"go", 2}, {"классика", 2}, {"programming", 1}}
	if got := l.TopTags(10); !slices.Equal(got, want) {
		t.Errorf("TopTags(10) = %v, ожидалось %v", got, want)
	}
	if got := l.TopTags(2); !slices.Equal(got, want[:2]) {
		t.Errorf("TopTags(2) = %v, ожидалось %v", got, want[:2])
	}
	for _, n := range []int{0, -1} {
		if got := l.TopTags(n); len(got) != 0 {
			t.Errorf("TopTags(%d) = %v, ожидался пустой результат", n, got)
		}
	}
}

func TestReport(t *testing.T) {
	l := fixture(t)
	_ = l.Checkout("0-306-40615-X", "Мария")
	want := strings.Join([]string{
		"ISBN          | Title                       | Author            | Year | Status",
		"--------------+-----------------------------+-------------------+------+-----------",
		"111           | go in action                | William Kennedy   | 2015 | available",
		"030640615X    | The Go Programming Language | Alan Donovan      | 2015 | out: Мария",
		"9785389074354 | Анна Каренина               | Лев Толстой       | 1877 | available",
		"9785170903352 | Война и мир                 | Лев Толстой       | 1869 | available",
		"1234567890    | Преступление и наказание    | Фёдор Достоевский | 1866 | available",
		"",
	}, "\n")
	if got := l.Report(); got != want {
		t.Errorf("Report() =\n%s\nожидалось:\n%s", got, want)
	}
}

func TestReportEmpty(t *testing.T) {
	want := "ISBN | Title | Author | Year | Status\n-----+-------+--------+------+-------\n"
	if got := New().Report(); got != want {
		t.Errorf("Report() пустой библиотеки =\n%q\nожидалось\n%q", got, want)
	}
}

func BenchmarkSearch(b *testing.B) {
	l := New()
	for i := range 1000 {
		_ = l.Add(Book{ISBN: strings.Repeat("1", 1+i%9) + strings.Repeat("0", i/9+1), Title: "Книга", Author: "Автор"})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l.Search("кни")
	}
}
