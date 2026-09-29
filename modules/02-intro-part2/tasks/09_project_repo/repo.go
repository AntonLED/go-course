//go:build !solution

package repo

import (
	"cmp"
	"iter"
)

// New создаёт пустой репозиторий.
func New[ID cmp.Ordered, E Entity[ID]]() *Repo[ID, E] {
	panic("TODO")
}

// AddIndex регистрирует вторичный индекс name по ключу key(e) и строит его
// по уже имеющимся данным.
//   - пустое имя или уже существующий индекс -> ошибка, оборачивающая ErrDuplicate;
//   - unique == true и в данных уже есть два элемента с одинаковым ключом ->
//     ошибка, оборачивающая *ConflictError; индекс при этом НЕ добавляется.
func (r *Repo[ID, E]) AddIndex(name string, key func(E) string, unique bool) error {
	panic("TODO")
}

// Insert добавляет новую сущность. Порядок проверок:
//  1. e.Validate() != nil -> ошибка, оборачивающая И ErrInvalid, И исходную
//     ошибку валидации ("repo: insert <id>: invalid entity: <err>");
//  2. такой ID уже есть -> обёртка ErrDuplicate;
//  3. конфликт по любому уникальному индексу -> обёртка *ConflictError.
//
// Операция атомарна: при ошибке ни данные, ни индексы не меняются.
func (r *Repo[ID, E]) Insert(e E) error {
	panic("TODO")
}

// Update заменяет существующую сущность (по e.GetID()). Нет такой -> обёртка
// ErrNotFound; невалидна -> ErrInvalid (+исходная ошибка); конфликт с другой
// сущностью по уникальному индексу -> *ConflictError. Индексы обновляются
// (старые ключи удаляются). Атомарно.
func (r *Repo[ID, E]) Update(e E) error {
	panic("TODO")
}

// Delete удаляет сущность и её записи в индексах. Нет такой -> ErrNotFound.
func (r *Repo[ID, E]) Delete(id ID) error {
	panic("TODO")
}

// Get возвращает сущность по ID или обёртку ErrNotFound.
func (r *Repo[ID, E]) Get(id ID) (E, error) {
	panic("TODO")
}

// Len — количество сущностей.
func (r *Repo[ID, E]) Len() int {
	panic("TODO")
}

// All — все сущности в порядке возрастания ID. Итератор работает по
// снимку, сделанному в момент начала обхода: внутри цикла можно вызывать
// Insert/Update/Delete (без дедлока), изменения в текущий обход не попадают.
func (r *Repo[ID, E]) All() iter.Seq2[ID, E] {
	panic("TODO")
}

// Where — сущности, удовлетворяющие pred, в порядке возрастания ID (снимок).
func (r *Repo[ID, E]) Where(pred func(E) bool) iter.Seq[E] {
	panic("TODO")
}

// Find — сущности с данным ключом индекса в порядке возрастания ID (снимок).
// Неизвестный индекс -> (nil, обёртка ErrUnknownIndex).
func (r *Repo[ID, E]) Find(indexName, key string) (iter.Seq[E], error) {
	panic("TODO")
}

// Lookup — первая (по ID) сущность с данным ключом индекса. Удобно для
// уникальных индексов. Неизвестный индекс -> ErrUnknownIndex, нет — ErrNotFound.
func (r *Repo[ID, E]) Lookup(indexName, key string) (E, error) {
	panic("TODO")
}

// Paginate пропускает offset элементов и отдаёт не более limit следующих.
// limit <= 0 — пустая последовательность; offset < 0 трактуется как 0.
// Не запрашивает у источника лишних элементов.
func Paginate[T any](s iter.Seq[T], offset, limit int) iter.Seq[T] {
	panic("TODO")
}
