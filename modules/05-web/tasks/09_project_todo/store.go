package todoapi

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

// Todo — задача.
type Todo struct {
	ID        int64     `json:"id"`
	Title     string    `json:"title"`
	Done      bool      `json:"done"`
	CreatedAt time.Time `json:"created_at"`
}

// ErrNotFound — задачи с таким ID нет.
var ErrNotFound = errors.New("todo not found")

// Store — хранилище. Интерфейс объявлен у потребителя (HTTP-слоя), реализаций может быть много.
type Store interface {
	Create(ctx context.Context, t Todo) (Todo, error)                 // назначает ID и CreatedAt
	Get(ctx context.Context, id int64) (Todo, error)                  // ErrNotFound
	List(ctx context.Context, offset, limit int) ([]Todo, int, error) // страница по возрастанию ID + общее число
	Update(ctx context.Context, t Todo) (Todo, error)                 // ErrNotFound; CreatedAt не меняется
	Delete(ctx context.Context, id int64) error                       // ErrNotFound
}

// MemStore — потокобезопасное хранилище в памяти (готовое, писать не нужно).
type MemStore struct {
	mu    sync.Mutex
	seq   int64
	items map[int64]Todo
	now   func() time.Time
}

// NewMemStore создаёт хранилище; now == nil → time.Now.
func NewMemStore(now func() time.Time) *MemStore {
	if now == nil {
		now = time.Now
	}
	return &MemStore{items: make(map[int64]Todo), now: now}
}

func (s *MemStore) Create(_ context.Context, t Todo) (Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	t.ID = s.seq
	t.CreatedAt = s.now().UTC()
	s.items[t.ID] = t
	return t, nil
}

func (s *MemStore) Get(_ context.Context, id int64) (Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.items[id]
	if !ok {
		return Todo{}, ErrNotFound
	}
	return t, nil
}

func (s *MemStore) List(_ context.Context, offset, limit int) ([]Todo, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	all := make([]Todo, 0, len(s.items))
	for _, t := range s.items {
		all = append(all, t)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID < all[j].ID })
	total := len(all)
	if offset >= total {
		return []Todo{}, total, nil
	}
	end := min(offset+limit, total)
	return all[offset:end], total, nil
}

func (s *MemStore) Update(_ context.Context, t Todo) (Todo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, ok := s.items[t.ID]
	if !ok {
		return Todo{}, ErrNotFound
	}
	t.CreatedAt = old.CreatedAt
	s.items[t.ID] = t
	return t, nil
}

func (s *MemStore) Delete(_ context.Context, id int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return ErrNotFound
	}
	delete(s.items, id)
	return nil
}
