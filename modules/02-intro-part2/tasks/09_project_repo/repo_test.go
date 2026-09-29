package repo

import (
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type FieldError struct{ Field string }

func (e *FieldError) Error() string { return "bad field " + e.Field }

type User struct {
	ID    int
	Email string
	City  string
	Age   int
}

func (u User) GetID() int { return u.ID }

func (u User) Validate() error {
	var errs []error
	if !strings.Contains(u.Email, "@") {
		errs = append(errs, &FieldError{"email"})
	}
	if u.Age < 0 {
		errs = append(errs, &FieldError{"age"})
	}
	return errors.Join(errs...)
}

// Проверка на этапе компиляции: User удовлетворяет Entity[int].
var _ Entity[int] = User{}

func newRepo(t *testing.T) *Repo[int, User] {
	t.Helper()
	r := New[int, User]()
	if err := r.AddIndex("email", func(u User) string { return strings.ToLower(u.Email) }, true); err != nil {
		t.Fatalf("AddIndex(email): %v", err)
	}
	if err := r.AddIndex("city", func(u User) string { return u.City }, false); err != nil {
		t.Fatalf("AddIndex(city): %v", err)
	}
	return r
}

func seed(t *testing.T, r *Repo[int, User]) {
	t.Helper()
	for _, u := range []User{
		{3, "c@x.io", "Москва", 30},
		{1, "a@x.io", "Казань", 20},
		{5, "e@x.io", "Москва", 50},
		{2, "b@x.io", "Москва", 25},
		{4, "d@x.io", "Пермь", 40},
	} {
		if err := r.Insert(u); err != nil {
			t.Fatalf("Insert(%v): %v", u, err)
		}
	}
}

func ids[E Entity[int]](s iter.Seq[E]) []int {
	var out []int
	for e := range s {
		out = append(out, e.GetID())
	}
	return out
}

func TestInsertGet(t *testing.T) {
	r := newRepo(t)
	seed(t, r)
	if r.Len() != 5 {
		t.Errorf("Len = %d, ожидалось 5", r.Len())
	}
	u, err := r.Get(3)
	if err != nil || u.Email != "c@x.io" {
		t.Errorf("Get(3) = %v, %v", u, err)
	}
	_, err = r.Get(42)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(42) err = %v, ожидалась обёртка ErrNotFound", err)
	}
	if err == ErrNotFound {
		t.Errorf("ошибка должна содержать контекст, а не быть голым sentinel")
	}
}

func TestInsertErrors(t *testing.T) {
	r := newRepo(t)
	seed(t, r)

	// Валидация: и ErrInvalid, и исходная ошибка доступны.
	err := r.Insert(User{ID: 10, Email: "no-at", Age: -1})
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("невалидный: errors.Is(err, ErrInvalid) = false; err = %v", err)
	}
	var fe *FieldError
	if !errors.As(err, &fe) || fe.Field != "email" {
		t.Errorf("невалидный: errors.As(*FieldError) не нашёл поле email; err = %v", err)
	}
	if !strings.Contains(err.Error(), "bad field age") {
		t.Errorf("сообщение должно включать все ошибки валидации: %q", err)
	}

	// Дубликат ID.
	err = r.Insert(User{ID: 1, Email: "new@x.io"})
	if !errors.Is(err, ErrDuplicate) {
		t.Errorf("дубликат ID: err = %v, ожидалась обёртка ErrDuplicate", err)
	}

	// Конфликт уникального индекса (регистр игнорируется ключом индекса).
	err = r.Insert(User{ID: 10, Email: "A@X.IO", City: "Сочи"})
	var ce *ConflictError
	if !errors.As(err, &ce) || ce.Index != "email" || ce.Key != "a@x.io" {
		t.Errorf("конфликт: err = %v, ожидался *ConflictError{email, a@x.io}", err)
	}
	if !errors.Is(err, ErrConflict) {
		t.Errorf("errors.Is(err, ErrConflict) = false: у ConflictError есть метод Is")
	}
	// Атомарность: ничего не изменилось.
	if r.Len() != 5 {
		t.Errorf("после неудачных вставок Len = %d, ожидалось 5", r.Len())
	}
	if seq, _ := r.Find("city", "Сочи"); len(ids(seq)) != 0 {
		t.Errorf("неудачная вставка оставила след в индексе city")
	}
}

func TestInsertAtomicMultipleUnique(t *testing.T) {
	r := New[int, User]()
	r.AddIndex("a_email", func(u User) string { return u.Email }, true)
	r.AddIndex("b_city", func(u User) string { return u.City }, true)
	if err := r.Insert(User{ID: 1, Email: "x@y", City: "Омск"}); err != nil {
		t.Fatal(err)
	}
	err := r.Insert(User{ID: 2, Email: "z@y", City: "Омск"})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("ожидался конфликт по b_city, err = %v", err)
	}
	if _, err := r.Lookup("a_email", "z@y"); !errors.Is(err, ErrNotFound) {
		t.Errorf("неудачная вставка оставила запись в индексе a_email: %v", err)
	}
}

func TestAddIndex(t *testing.T) {
	r := newRepo(t)
	seed(t, r)
	if err := r.AddIndex("email", func(User) string { return "" }, false); !errors.Is(err, ErrDuplicate) {
		t.Errorf("повторный индекс: err = %v", err)
	}
	if err := r.AddIndex("", func(User) string { return "" }, false); !errors.Is(err, ErrDuplicate) {
		t.Errorf("пустое имя индекса: err = %v", err)
	}
	// Уникальный индекс по неуникальным данным — конфликт, индекс не создан.
	err := r.AddIndex("city_u", func(u User) string { return u.City }, true)
	if !errors.Is(err, ErrConflict) {
		t.Errorf("уникальный индекс по городу: err = %v, ожидался конфликт", err)
	}
	if _, err := r.Find("city_u", "Москва"); !errors.Is(err, ErrUnknownIndex) {
		t.Errorf("индекс city_u не должен был создаться: err = %v", err)
	}
	// Индекс, добавленный после данных, строится по ним.
	if err := r.AddIndex("decade", func(u User) string { return fmt.Sprint(u.Age / 10) }, false); err != nil {
		t.Fatal(err)
	}
	seq, _ := r.Find("decade", "2")
	if got := ids(seq); !slices.Equal(got, []int{1, 2}) {
		t.Errorf("Find(decade=2) = %v, ожидалось [1 2]", got)
	}
}

func TestFindLookup(t *testing.T) {
	r := newRepo(t)
	seed(t, r)
	seq, err := r.Find("city", "Москва")
	if err != nil {
		t.Fatal(err)
	}
	if got := ids(seq); !slices.Equal(got, []int{2, 3, 5}) {
		t.Errorf("Find(city=Москва) = %v, ожидалось [2 3 5] (по возрастанию ID)", got)
	}
	if _, err := r.Find("nope", "x"); !errors.Is(err, ErrUnknownIndex) {
		t.Errorf("Find(неизвестный индекс) err = %v", err)
	}
	u, err := r.Lookup("email", "d@x.io")
	if err != nil || u.ID != 4 {
		t.Errorf("Lookup(email=d@x.io) = %v, %v", u, err)
	}
	if _, err := r.Lookup("email", "zzz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Lookup(нет) err = %v", err)
	}
	if _, err := r.Lookup("nope", "x"); !errors.Is(err, ErrUnknownIndex) {
		t.Errorf("Lookup(неизвестный индекс) err = %v", err)
	}
}

func TestUpdateDelete(t *testing.T) {
	r := newRepo(t)
	seed(t, r)
	if err := r.Update(User{ID: 99, Email: "q@q"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update(нет) err = %v", err)
	}
	if err := r.Update(User{ID: 1, Email: "bad"}); !errors.Is(err, ErrInvalid) {
		t.Errorf("Update(невалидный) err = %v", err)
	}
	if err := r.Update(User{ID: 1, Email: "b@x.io"}); !errors.Is(err, ErrConflict) {
		t.Errorf("Update(конфликт с id=2) err = %v", err)
	}
	// Обновление себя тем же email — не конфликт.
	if err := r.Update(User{ID: 1, Email: "a@x.io", City: "Москва", Age: 21}); err != nil {
		t.Errorf("Update(тот же email) = %v", err)
	}
	seq, _ := r.Find("city", "Москва")
	if got := ids(seq); !slices.Equal(got, []int{1, 2, 3, 5}) {
		t.Errorf("после смены города Find(Москва) = %v", got)
	}
	seq, _ = r.Find("city", "Казань")
	if got := ids(seq); len(got) != 0 {
		t.Errorf("старый ключ индекса не удалён: Find(Казань) = %v", got)
	}
	if err := r.Update(User{ID: 1, Email: "new@x.io", City: "Москва"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Lookup("email", "a@x.io"); !errors.Is(err, ErrNotFound) {
		t.Errorf("старый email остался в индексе")
	}
	// Освободившийся email можно занять.
	if err := r.Insert(User{ID: 6, Email: "a@x.io"}); err != nil {
		t.Errorf("Insert со свободным email: %v", err)
	}

	if err := r.Delete(2); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(2); !errors.Is(err, ErrNotFound) {
		t.Errorf("повторный Delete err = %v", err)
	}
	if _, err := r.Lookup("email", "b@x.io"); !errors.Is(err, ErrNotFound) {
		t.Errorf("после Delete запись осталась в индексе")
	}
	if err := r.Insert(User{ID: 2, Email: "b@x.io"}); err != nil {
		t.Errorf("повторная вставка после Delete: %v", err)
	}
}

func TestIterators(t *testing.T) {
	r := newRepo(t)
	seed(t, r)
	var got []int
	for id, u := range r.All() {
		if id != u.ID {
			t.Fatalf("All: ключ %d не совпадает с ID %d", id, u.ID)
		}
		got = append(got, id)
	}
	if !slices.Equal(got, []int{1, 2, 3, 4, 5}) {
		t.Errorf("All = %v, ожидалось по возрастанию ID", got)
	}
	older := r.Where(func(u User) bool { return u.Age >= 30 })
	if got := ids(older); !slices.Equal(got, []int{3, 4, 5}) {
		t.Errorf("Where(Age>=30) = %v", got)
	}
	// Ранний выход.
	n := 0
	for range r.All() {
		n++
		if n == 2 {
			break
		}
	}
	for range r.Where(func(User) bool { return true }) {
		break
	}
	seq, _ := r.Find("city", "Москва")
	for range seq {
		break
	}
}

func TestPaginate(t *testing.T) {
	r := newRepo(t)
	seed(t, r)
	all := r.Where(func(User) bool { return true })
	tests := []struct {
		off, lim int
		want     []int
	}{
		{0, 2, []int{1, 2}},
		{2, 2, []int{3, 4}},
		{4, 2, []int{5}},
		{10, 2, nil},
		{-5, 1, []int{1}},
		{0, 0, nil},
		{1, -1, nil},
	}
	for _, tt := range tests {
		if got := ids(Paginate(all, tt.off, tt.lim)); !slices.Equal(got, tt.want) {
			t.Errorf("Paginate(%d, %d) = %v, ожидалось %v", tt.off, tt.lim, got, tt.want)
		}
	}
	// Не тянет лишнего из источника.
	pulled := 0
	src := func(yield func(int) bool) {
		for i := 0; ; i++ {
			pulled++
			if !yield(i) {
				return
			}
		}
	}
	got := slices.Collect(Paginate(src, 3, 2))
	if !slices.Equal(got, []int{3, 4}) || pulled != 5 {
		t.Errorf("Paginate(бесконечный, 3, 2) = %v, из источника взято %d (ожидалось 5)", got, pulled)
	}
}

// Модификация репозитория внутри цикла не должна приводить к дедлоку.
func TestNoDeadlockInsideLoop(t *testing.T) {
	r := newRepo(t)
	seed(t, r)
	done := make(chan []int)
	var moved []int
	go func() {
		// Find: изменение индекса внутри обхода.
		seq, _ := r.Find("city", "Москва")
		for u := range seq {
			moved = append(moved, u.ID)
			r.Update(User{ID: u.ID, Email: u.Email, City: "Тверь", Age: u.Age})
		}
		// Where: запись внутри обхода.
		for u := range r.Where(func(User) bool { return true }) {
			r.Update(u)
		}
		// All: вставка и удаление внутри обхода.
		var seen []int
		for id := range r.All() {
			seen = append(seen, id)
			r.Insert(User{ID: 100 + id, Email: fmt.Sprintf("n%d@x", id)})
			r.Delete(id)
		}
		done <- seen
	}()
	select {
	case seen := <-done:
		if !slices.Equal(moved, []int{2, 3, 5}) {
			t.Errorf("обход Find по снимку: %v, ожидалось [2 3 5]", moved)
		}
		if !slices.Equal(seen, []int{1, 2, 3, 4, 5}) {
			t.Errorf("обход по снимку: %v, ожидалось [1 2 3 4 5] (новые элементы не попадают)", seen)
		}
		if r.Len() != 5 {
			t.Errorf("Len = %d, ожидалось 5", r.Len())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("дедлок: итератор держит блокировку во время yield")
	}
}

func TestConcurrent(t *testing.T) {
	r := newRepo(t)
	var wg sync.WaitGroup
	const workers, per = 8, 100
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range per {
				id := w*per + i
				if err := r.Insert(User{ID: id, Email: fmt.Sprintf("u%d@x", id), City: fmt.Sprint(id % 3)}); err != nil {
					t.Errorf("Insert(%d): %v", id, err)
					return
				}
				if i%10 == 0 {
					for range r.All() {
					}
					if seq, err := r.Find("city", "1"); err == nil {
						for range seq {
						}
					}
				}
			}
		}()
	}
	// Все пытаются вставить один и тот же email — ровно один успех.
	var mu sync.Mutex
	wins := 0
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r.Insert(User{ID: 10000 + w, Email: "same@x"}) == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if wins != 1 {
		t.Errorf("уникальный email вставлен %d раз, ожидалось 1", wins)
	}
	if r.Len() != workers*per+1 {
		t.Errorf("Len = %d, ожидалось %d", r.Len(), workers*per+1)
	}
	seq, _ := r.Find("city", "1")
	want := 0
	for id := range workers * per {
		if id%3 == 1 {
			want++
		}
	}
	if n := len(ids(seq)); n != want {
		t.Errorf("Find(city=1) = %d элементов, ожидалось %d", n, want)
	}
}

func BenchmarkFind(b *testing.B) {
	r := New[int, User]()
	r.AddIndex("city", func(u User) string { return u.City }, false)
	for i := range 10000 {
		r.Insert(User{ID: i, Email: fmt.Sprintf("u%d@x", i), City: fmt.Sprint(i % 100)})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		seq, _ := r.Find("city", "42")
		for range seq {
		}
	}
}
