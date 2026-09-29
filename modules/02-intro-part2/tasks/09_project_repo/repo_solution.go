//go:build solution

package repo

import (
	"cmp"
	"fmt"
	"iter"
	"maps"
	"slices"
)

func New[ID cmp.Ordered, E Entity[ID]]() *Repo[ID, E] {
	return &Repo[ID, E]{
		items:   make(map[ID]E),
		indexes: make(map[string]*index[ID, E]),
	}
}

func (ix *index[ID, E]) add(k string, id ID) {
	ids := ix.m[k]
	if ids == nil {
		ids = make(map[ID]struct{})
		ix.m[k] = ids
	}
	ids[id] = struct{}{}
}

func (ix *index[ID, E]) remove(k string, id ID) {
	ids := ix.m[k]
	delete(ids, id)
	if len(ids) == 0 {
		delete(ix.m, k) // не копим пустые множества
	}
}

func (r *Repo[ID, E]) AddIndex(name string, key func(E) string, unique bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name == "" {
		return fmt.Errorf("repo: add index: empty name: %w", ErrDuplicate)
	}
	if _, ok := r.indexes[name]; ok {
		return fmt.Errorf("repo: add index %q: %w", name, ErrDuplicate)
	}
	ix := &index[ID, E]{key: key, unique: unique, m: make(map[string]map[ID]struct{})}
	for id, e := range r.items {
		k := key(e)
		if unique && len(ix.m[k]) > 0 {
			return fmt.Errorf("repo: add index %q: %w", name, &ConflictError{Index: name, Key: k})
		}
		ix.add(k, id)
	}
	r.indexes[name] = ix
	return nil
}

// checkUnique ищет конфликт по уникальным индексам, игнорируя сущность self
// (для Update). Вызывать под блокировкой. Индексы обходим в порядке имён,
// чтобы ошибка была детерминированной.
func (r *Repo[ID, E]) checkUnique(e E, self ID) error {
	for _, name := range slices.Sorted(maps.Keys(r.indexes)) {
		ix := r.indexes[name]
		if !ix.unique {
			continue
		}
		k := ix.key(e)
		for other := range ix.m[k] {
			if other != self {
				return &ConflictError{Index: name, Key: k}
			}
		}
	}
	return nil
}

func (r *Repo[ID, E]) Insert(e E) error {
	id := e.GetID()
	// Валидация не требует блокировки — делаем её до Lock.
	if err := e.Validate(); err != nil {
		// Два %w (Go 1.20+): и errors.Is(err, ErrInvalid), и errors.As к типу
		// ошибки валидации работают одновременно.
		return fmt.Errorf("repo: insert %v: %w: %w", id, ErrInvalid, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; ok {
		return fmt.Errorf("repo: insert %v: %w", id, ErrDuplicate)
	}
	// Сначала все проверки, потом все изменения — так операция атомарна.
	if err := r.checkUnique(e, id); err != nil {
		return fmt.Errorf("repo: insert %v: %w", id, err)
	}
	r.items[id] = e
	for _, ix := range r.indexes {
		ix.add(ix.key(e), id)
	}
	return nil
}

func (r *Repo[ID, E]) Update(e E) error {
	id := e.GetID()
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.items[id]
	if !ok {
		return fmt.Errorf("repo: update %v: %w", id, ErrNotFound)
	}
	if err := e.Validate(); err != nil {
		return fmt.Errorf("repo: update %v: %w: %w", id, ErrInvalid, err)
	}
	if err := r.checkUnique(e, id); err != nil {
		return fmt.Errorf("repo: update %v: %w", id, err)
	}
	for _, ix := range r.indexes {
		ix.remove(ix.key(old), id)
		ix.add(ix.key(e), id)
	}
	r.items[id] = e
	return nil
}

func (r *Repo[ID, E]) Delete(id ID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	old, ok := r.items[id]
	if !ok {
		return fmt.Errorf("repo: delete %v: %w", id, ErrNotFound)
	}
	for _, ix := range r.indexes {
		ix.remove(ix.key(old), id)
	}
	delete(r.items, id)
	return nil
}

func (r *Repo[ID, E]) Get(id ID) (E, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.items[id]
	if !ok {
		var zero E
		return zero, fmt.Errorf("repo: get %v: %w", id, ErrNotFound)
	}
	return e, nil
}

func (r *Repo[ID, E]) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}

// snapshot под RLock берёт множество ID (ids == nil — все) и копирует
// соответствующие сущности в порядке возрастания ID. Держать блокировку во
// время yield нельзя: тело цикла потребителя может вызвать Insert и получить
// дедлок (sync.RWMutex не реентерабелен).
func (r *Repo[ID, E]) snapshot(ids func() map[ID]struct{}) []E {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var keys []ID
	if ids == nil {
		keys = slices.Sorted(maps.Keys(r.items))
	} else {
		keys = slices.Sorted(maps.Keys(ids()))
	}
	out := make([]E, len(keys))
	for i, id := range keys {
		out[i] = r.items[id]
	}
	return out
}

func (r *Repo[ID, E]) All() iter.Seq2[ID, E] {
	return func(yield func(ID, E) bool) {
		for _, e := range r.snapshot(nil) {
			if !yield(e.GetID(), e) {
				return
			}
		}
	}
}

func (r *Repo[ID, E]) Where(pred func(E) bool) iter.Seq[E] {
	return func(yield func(E) bool) {
		for _, e := range r.snapshot(nil) {
			if pred(e) && !yield(e) {
				return
			}
		}
	}
}

func (r *Repo[ID, E]) Find(indexName, key string) (iter.Seq[E], error) {
	r.mu.RLock()
	ix, ok := r.indexes[indexName]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("repo: find %q: %w", indexName, ErrUnknownIndex)
	}
	return func(yield func(E) bool) {
		// Индекс читаем в момент начала обхода и под той же блокировкой,
		// что и данные, — иначе между чтениями их могли изменить.
		ids := func() map[ID]struct{} { return ix.m[key] }
		for _, e := range r.snapshot(ids) {
			if !yield(e) {
				return
			}
		}
	}, nil
}

func (r *Repo[ID, E]) Lookup(indexName, key string) (E, error) {
	var zero E
	seq, err := r.Find(indexName, key)
	if err != nil {
		return zero, err
	}
	for e := range seq {
		return e, nil
	}
	return zero, fmt.Errorf("repo: lookup %s=%q: %w", indexName, key, ErrNotFound)
}

func Paginate[T any](s iter.Seq[T], offset, limit int) iter.Seq[T] {
	return func(yield func(T) bool) {
		if limit <= 0 {
			return
		}
		i, sent := 0, 0
		for v := range s {
			if i++; i <= offset {
				continue
			}
			if !yield(v) {
				return
			}
			if sent++; sent == limit {
				return
			}
		}
	}
}
