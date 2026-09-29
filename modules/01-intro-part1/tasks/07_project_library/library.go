//go:build !solution

// Package library — «Задание 1»: мини-библиотека.
// Объединяет структуры и методы, map, срезы (и их aliasing), строки с unicode,
// сортировку и форматированный вывод.
package library

// Library хранит книги и сведения о выдаче.
// Нулевое значение Library не обязано быть рабочим — используйте New.
type Library struct {
	// TODO: например, map ISBN → *запись (книга + кому выдана).
}

// New создаёт пустую библиотеку.
func New() *Library {
	// TODO: реализуйте
	panic("TODO")
}

// Add добавляет книгу.
//   - ISBN нормализуется: удаляются '-' и пробелы; результат должен быть непустым
//     и состоять из цифр (допускается 'X' или 'x' последним символом, хранится как 'X');
//   - Title и Author обрезаются (strings.TrimSpace) и не должны быть пустыми;
//   - Tags: TrimSpace + ToLower, пустые выбрасываются, повторы удаляются,
//     порядок первых вхождений сохраняется;
//   - некорректные данные → ErrInvalidBook, повтор ISBN (после нормализации) → ErrDuplicate.
//
// Библиотека НЕ должна разделять память с b.Tags вызывающего.
func (l *Library) Add(b Book) error {
	// TODO: реализуйте
	panic("TODO")
}

// Get возвращает копию книги по ISBN (ISBN нормализуется так же, как в Add).
// Изменение результата (в т.ч. его Tags) не должно влиять на библиотеку.
func (l *Library) Get(isbn string) (Book, bool) {
	// TODO: реализуйте
	panic("TODO")
}

// Checkout выдаёт книгу читателю.
// Пустой (после TrimSpace) reader → ErrInvalidReader; нет книги → ErrNotFound;
// уже выдана → ошибка с errors.Is(err, ErrCheckedOut).
func (l *Library) Checkout(isbn, reader string) error {
	// TODO: реализуйте
	panic("TODO")
}

// Return возвращает книгу. Нет книги → ErrNotFound; не выдана → ErrNotCheckedOut.
func (l *Library) Return(isbn string) error {
	// TODO: реализуйте
	panic("TODO")
}

// Search ищет книги, у которых Title или Author содержит query без учёта
// регистра (включая кириллицу). query обрезается TrimSpace; пустой — все книги.
// Результат (копии книг) отсортирован по strings.ToLower(Title), затем по ISBN.
// Ничего не найдено — пустой срез (len 0).
func (l *Library) Search(query string) []Book {
	// TODO: реализуйте
	panic("TODO")
}

// AuthorStats возвращает статистику по авторам, отсортированную по убыванию
// числа книг, при равенстве — по имени автора (возрастание).
func (l *Library) AuthorStats() []AuthorStat {
	// TODO: реализуйте
	panic("TODO")
}

// TopTags возвращает до n самых частых тегов: по убыванию Count, затем по Tag.
// n <= 0 → пустой результат.
func (l *Library) TopTags(n int) []TagCount {
	// TODO: реализуйте
	panic("TODO")
}

// Report формирует текстовую таблицу по всем книгам (порядок — как в Search("")).
//
// Колонки: ISBN, Title, Author, Year, Status. Status — "available" или "out: <reader>".
// Ширина колонки — максимальная длина В рунах среди заголовка и значений.
// Ячейки выравниваются влево и разделяются " | ". Вторая строка — разделитель:
// '-' по ширине колонки, соединённые "-+-". У каждой строки обрезаются
// пробелы справа, каждая строка заканчивается '\n'.
//
//	ISBN | Title    | Author | Year | Status
//	-----+----------+--------+------+----------
//	1    | Война... | Толстой | ...
func (l *Library) Report() string {
	// TODO: реализуйте
	panic("TODO")
}
