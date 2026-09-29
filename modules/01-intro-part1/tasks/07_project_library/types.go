package library

import "errors"

// Ошибки-сентинелы. Методы могут оборачивать их (fmt.Errorf("%w ...")),
// тесты проверяют errors.Is.
var (
	ErrInvalidBook   = errors.New("library: invalid book")
	ErrInvalidReader = errors.New("library: invalid reader")
	ErrDuplicate     = errors.New("library: duplicate isbn")
	ErrNotFound      = errors.New("library: book not found")
	ErrCheckedOut    = errors.New("library: book already checked out")
	ErrNotCheckedOut = errors.New("library: book is not checked out")
)

// Book — описание книги.
type Book struct {
	ISBN   string
	Title  string
	Author string
	Year   int
	Tags   []string
}

// AuthorStat — статистика по автору.
type AuthorStat struct {
	Author     string
	Books      int // сколько книг автора в библиотеке
	CheckedOut int // сколько из них сейчас выдано
}

// TagCount — тег и число книг с ним.
type TagCount struct {
	Tag   string
	Count int
}
