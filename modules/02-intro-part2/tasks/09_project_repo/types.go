package repo

import (
	"cmp"
	"errors"
	"fmt"
	"sync"
)

// Sentinel-ошибки репозитория. Все ошибки методов оборачивают одну из них
// (проверяйте через errors.Is).
var (
	ErrNotFound     = errors.New("not found")
	ErrDuplicate    = errors.New("already exists")
	ErrInvalid      = errors.New("invalid entity")
	ErrConflict     = errors.New("unique constraint violation")
	ErrUnknownIndex = errors.New("unknown index")
)

// Entity — то, что можно хранить в репозитории.
type Entity[ID cmp.Ordered] interface {
	GetID() ID
	Validate() error // nil, если сущность корректна
}

// ConflictError — нарушение уникального индекса. errors.Is(err, ErrConflict)
// для неё истинно благодаря методу Is.
type ConflictError struct {
	Index string // имя индекса
	Key   string // конфликтующий ключ
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("unique index %q: key %q already taken", e.Index, e.Key)
}

func (e *ConflictError) Is(target error) bool { return target == ErrConflict }

// index — вторичный индекс: ключ -> множество ID.
type index[ID cmp.Ordered, E any] struct {
	key    func(E) string
	unique bool
	m      map[string]map[ID]struct{}
}

// Repo — потокобезопасный generic in-memory репозиторий с вторичными
// индексами. Создаётся через New.
type Repo[ID cmp.Ordered, E Entity[ID]] struct {
	mu      sync.RWMutex
	items   map[ID]E
	indexes map[string]*index[ID, E]
}
