//go:build solution

// Package library — «Задание 1»: мини-библиотека.
// Объединяет структуры и методы, map, срезы (и их aliasing), строки с unicode,
// сортировку и форматированный вывод.
package library

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// entry — внутренняя запись: книга и текущий читатель ("" — книга на месте).
type entry struct {
	book   Book
	reader string
}

// Library хранит книги и сведения о выдаче.
type Library struct {
	// Храним указатели: элемент map неадресуем (m[k].field = v не компилируется),
	// а через *entry можно менять запись на месте.
	books map[string]*entry
}

// New создаёт пустую библиотеку.
func New() *Library {
	return &Library{books: make(map[string]*entry)}
}

// normalizeISBN убирает '-' и пробелы и проверяет формат.
func normalizeISBN(s string) (string, bool) {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r == '-' || r == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	isbn := b.String()
	if isbn == "" {
		return "", false
	}
	for i := 0; i < len(isbn); i++ { // побайтово можно: допустимы только ASCII-символы
		c := isbn[i]
		switch {
		case c >= '0' && c <= '9':
		case (c == 'X' || c == 'x') && i == len(isbn)-1:
		default:
			return "", false
		}
	}
	return strings.ToUpper(isbn), true
}

// normalizeTags чистит теги и всегда возвращает новый срез.
func normalizeTags(tags []string) []string {
	if len(tags) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(tags))
	res := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		res = append(res, t)
	}
	return res
}

// clone возвращает глубокую копию книги (срез Tags копируется).
func (b Book) clone() Book {
	b.Tags = slices.Clone(b.Tags) // b — уже копия структуры, но Tags общие
	return b
}

// Add добавляет книгу.
func (l *Library) Add(b Book) error {
	isbn, ok := normalizeISBN(b.ISBN)
	if !ok {
		return fmt.Errorf("%w: bad isbn %q", ErrInvalidBook, b.ISBN)
	}
	b.ISBN = isbn
	b.Title = strings.TrimSpace(b.Title)
	b.Author = strings.TrimSpace(b.Author)
	if b.Title == "" || b.Author == "" {
		return fmt.Errorf("%w: empty title or author", ErrInvalidBook)
	}
	if _, exists := l.books[isbn]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicate, isbn)
	}
	b.Tags = normalizeTags(b.Tags) // новый срез — не зависим от вызывающего
	l.books[isbn] = &entry{book: b}
	return nil
}

// lookup находит запись по «сырому» ISBN.
func (l *Library) lookup(isbn string) (*entry, bool) {
	norm, ok := normalizeISBN(isbn)
	if !ok {
		return nil, false
	}
	e, ok := l.books[norm]
	return e, ok
}

// Get возвращает копию книги.
func (l *Library) Get(isbn string) (Book, bool) {
	e, ok := l.lookup(isbn)
	if !ok {
		return Book{}, false
	}
	return e.book.clone(), true
}

// Checkout выдаёт книгу читателю.
func (l *Library) Checkout(isbn, reader string) error {
	reader = strings.TrimSpace(reader)
	if reader == "" {
		return ErrInvalidReader
	}
	e, ok := l.lookup(isbn)
	if !ok {
		return ErrNotFound
	}
	if e.reader != "" {
		return fmt.Errorf("%w: by %s", ErrCheckedOut, e.reader)
	}
	e.reader = reader // меняем запись через указатель
	return nil
}

// Return возвращает книгу.
func (l *Library) Return(isbn string) error {
	e, ok := l.lookup(isbn)
	if !ok {
		return ErrNotFound
	}
	if e.reader == "" {
		return ErrNotCheckedOut
	}
	e.reader = ""
	return nil
}

// sortedEntries возвращает записи в каноническом порядке (Title без регистра, ISBN).
// Обход map случаен, поэтому сортировка обязательна.
func (l *Library) sortedEntries(keep func(*entry) bool) []*entry {
	res := make([]*entry, 0, len(l.books))
	for _, e := range l.books {
		if keep == nil || keep(e) {
			res = append(res, e)
		}
	}
	slices.SortFunc(res, func(a, b *entry) int {
		return cmp.Or(
			strings.Compare(strings.ToLower(a.book.Title), strings.ToLower(b.book.Title)),
			strings.Compare(a.book.ISBN, b.book.ISBN),
		)
	})
	return res
}

// Search ищет книги по подстроке в названии или авторе.
func (l *Library) Search(query string) []Book {
	q := strings.ToLower(strings.TrimSpace(query))
	entries := l.sortedEntries(func(e *entry) bool {
		// strings.ToLower работает с Unicode: "ТОЛСТОЙ" → "толстой".
		return strings.Contains(strings.ToLower(e.book.Title), q) ||
			strings.Contains(strings.ToLower(e.book.Author), q)
	})
	res := make([]Book, 0, len(entries))
	for _, e := range entries {
		res = append(res, e.book.clone())
	}
	return res
}

// AuthorStats возвращает статистику по авторам.
func (l *Library) AuthorStats() []AuthorStat {
	// Значение в map — структура. m[k].Books++ не скомпилируется,
	// поэтому читаем копию, меняем и кладём обратно.
	stats := make(map[string]AuthorStat)
	for _, e := range l.books {
		s := stats[e.book.Author] // для нового автора — нулевое значение
		s.Author = e.book.Author
		s.Books++
		if e.reader != "" {
			s.CheckedOut++
		}
		stats[e.book.Author] = s
	}
	res := make([]AuthorStat, 0, len(stats))
	for _, s := range stats {
		res = append(res, s)
	}
	slices.SortFunc(res, func(a, b AuthorStat) int {
		return cmp.Or(cmp.Compare(b.Books, a.Books), strings.Compare(a.Author, b.Author))
	})
	return res
}

// TopTags возвращает n самых частых тегов.
func (l *Library) TopTags(n int) []TagCount {
	if n <= 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, e := range l.books {
		for _, t := range e.book.Tags {
			counts[t]++
		}
	}
	res := make([]TagCount, 0, len(counts))
	for t, c := range counts {
		res = append(res, TagCount{Tag: t, Count: c})
	}
	slices.SortFunc(res, func(a, b TagCount) int {
		return cmp.Or(cmp.Compare(b.Count, a.Count), strings.Compare(a.Tag, b.Tag))
	})
	return res[:min(n, len(res))]
}

// Report формирует текстовую таблицу.
func (l *Library) Report() string {
	rows := [][]string{{"ISBN", "Title", "Author", "Year", "Status"}}
	for _, e := range l.sortedEntries(nil) {
		status := "available"
		if e.reader != "" {
			status = "out: " + e.reader
		}
		rows = append(rows, []string{
			e.book.ISBN, e.book.Title, e.book.Author, strconv.Itoa(e.book.Year), status,
		})
	}

	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			// len(cell) дал бы байты — кириллица «растянула» бы колонку вдвое.
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}

	var b strings.Builder
	writeRow := func(cells []string, sep string) {
		var line strings.Builder
		for i, cell := range cells {
			if i > 0 {
				line.WriteString(sep)
			}
			// %-*s: fmt считает ширину в рунах.
			fmt.Fprintf(&line, "%-*s", widths[i], cell)
		}
		b.WriteString(strings.TrimRight(line.String(), " "))
		b.WriteByte('\n')
	}

	writeRow(rows[0], " | ")
	dashes := make([]string, len(widths))
	for i, w := range widths {
		dashes[i] = strings.Repeat("-", w)
	}
	writeRow(dashes, "-+-")
	for _, row := range rows[1:] {
		writeRow(row, " | ")
	}
	return b.String()
}
