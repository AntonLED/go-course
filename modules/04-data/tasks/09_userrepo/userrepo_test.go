package userrepo

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"gocourse/modules/04-data/memsql"
)

// setup создаёт отдельную базу на тест. MaxOpenConns(1): если код забудет
// rows.Close() или внутри транзакции пойдёт в r.db вместо tx, следующий
// запрос не получит соединение — и упадёт по таймауту контекста, а не повиснет.
func setup(t *testing.T) (*Repo, *sql.DB, context.Context) {
	t.Helper()
	dsn := memsql.NewDSN(t.Name())
	db, err := sql.Open("memsql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	t.Cleanup(func() { cancel(); db.Close(); memsql.Drop(dsn) })
	r := New(db)
	if err := r.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return r, db, ctx
}

func ptr[T any](v T) *T { return &v }

func TestCreateGet(t *testing.T) {
	r, db, ctx := setup(t)
	u := &User{Name: "Анна", Email: "anna@x.io", Phone: sql.NullString{String: "+7 900", Valid: true}, Age: ptr(30)}
	if err := r.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == 0 {
		t.Fatal("Create должен записать ID в u.ID")
	}
	u2 := &User{Name: "Борис", Email: "boris@x.io"} // Phone и Age — NULL
	if err := r.Create(ctx, u2); err != nil {
		t.Fatal(err)
	}
	if u2.ID == u.ID {
		t.Fatal("ID должны различаться")
	}

	got, err := r.Get(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Анна" || got.Email != "anna@x.io" || got.Phone != u.Phone || got.Age == nil || *got.Age != 30 {
		t.Errorf("Get = %+v", got)
	}
	got, err = r.Get(ctx, u2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Phone.Valid || got.Age != nil {
		t.Errorf("NULL-поля: Phone=%+v Age=%v, ожидалось Valid=false и nil", got.Phone, got.Age)
	}
	// NULL действительно лежит в базе как NULL, а не как "" или 0.
	var n int
	db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE phone IS NULL AND age IS NULL`).Scan(&n)
	if n != 1 {
		t.Errorf("строк с NULL в phone и age: %d, ожидалось 1", n)
	}

	if _, err := r.Get(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get(999): %v, ожидалось ErrNotFound", err)
	}
}

func TestCreateErrors(t *testing.T) {
	r, _, ctx := setup(t)
	if err := r.Create(ctx, &User{Name: "a", Email: "a@x"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Create(ctx, &User{Name: "b", Email: "a@x"}); !errors.Is(err, ErrDuplicateEmail) {
		t.Errorf("дубль email: %v, ожидалось ErrDuplicateEmail", err)
	}
	for _, u := range []*User{
		{Name: "", Email: "e@x"},
		{Name: "  ", Email: "e@x"},
		{Name: "x", Email: "no-at"},
		{Name: "x", Email: "e@x", Age: ptr(-1)},
	} {
		if err := r.Create(ctx, u); !errors.Is(err, ErrInvalid) {
			t.Errorf("Create(%+v): %v, ожидалось ErrInvalid", u, err)
		}
	}
	// SQL-инъекция через данные безвредна при плейсхолдерах.
	evil := &User{Name: "x'); DELETE FROM users; --", Email: "evil@x"}
	if err := r.Create(ctx, evil); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(ctx, evil.ID)
	if err != nil || got.Name != evil.Name {
		t.Errorf("имя с кавычками: %q, %v", got.Name, err)
	}
}

func TestListUpdateDelete(t *testing.T) {
	r, _, ctx := setup(t)
	empty, err := r.List(ctx, 10, 0)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Errorf("List пустой базы = %#v, %v; ожидался пустой не-nil срез", empty, err)
	}
	var ids []int64
	for _, name := range []string{"a", "b", "c", "d", "e"} {
		u := &User{Name: name, Email: name + "@x"}
		if err := r.Create(ctx, u); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, u.ID)
	}
	page, err := r.List(ctx, 2, 1)
	if err != nil || len(page) != 2 || page[0].Name != "b" || page[1].Name != "c" {
		t.Errorf("List(2, 1) = %+v, %v", page, err)
	}
	all, err := r.List(ctx, 0, 0)
	if err != nil || len(all) != 5 {
		t.Errorf("List(0, 0) = %d, %v; ожидалось 5 (limit по умолчанию)", len(all), err)
	}

	u := all[0]
	u.Name, u.Phone, u.Age = "A", sql.NullString{String: "123", Valid: true}, ptr(5)
	if err := r.Update(ctx, u); err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(ctx, u.ID)
	if got.Name != "A" || got.Phone.String != "123" || *got.Age != 5 {
		t.Errorf("после Update: %+v", got)
	}
	u.Phone, u.Age = sql.NullString{}, nil // обратно в NULL
	if err := r.Update(ctx, u); err != nil {
		t.Fatal(err)
	}
	got, _ = r.Get(ctx, u.ID)
	if got.Phone.Valid || got.Age != nil {
		t.Errorf("Update в NULL: %+v", got)
	}
	if err := r.Update(ctx, User{ID: 999, Name: "z", Email: "z@x"}); !errors.Is(err, ErrNotFound) {
		t.Errorf("Update(999): %v, ожидалось ErrNotFound", err)
	}
	u.Email = "b@x"
	if err := r.Update(ctx, u); !errors.Is(err, ErrDuplicateEmail) {
		t.Errorf("Update в занятый email: %v", err)
	}
	u.Email = ""
	if err := r.Update(ctx, u); !errors.Is(err, ErrInvalid) {
		t.Errorf("Update с пустым email: %v", err)
	}

	if err := r.Delete(ctx, ids[1]); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, ids[1]); !errors.Is(err, ErrNotFound) {
		t.Errorf("повторный Delete: %v, ожидалось ErrNotFound", err)
	}
	all, _ = r.List(ctx, 0, 0)
	if len(all) != 4 {
		t.Errorf("после Delete %d пользователей", len(all))
	}
}

func TestTransfer(t *testing.T) {
	r, _, ctx := setup(t)
	u := &User{Name: "a", Email: "a@x"}
	if err := r.Create(ctx, u); err != nil {
		t.Fatal(err)
	}
	if _, err := r.OpenAccount(ctx, 999, 10); !errors.Is(err, ErrNotFound) {
		t.Errorf("OpenAccount несуществующему: %v", err)
	}
	if _, err := r.OpenAccount(ctx, u.ID, -1); !errors.Is(err, ErrInvalid) {
		t.Errorf("OpenAccount с отрицательным балансом: %v", err)
	}
	a, err := r.OpenAccount(ctx, u.ID, 100)
	if err != nil {
		t.Fatal(err)
	}
	b, err := r.OpenAccount(ctx, u.ID, 0)
	if err != nil || a == b {
		t.Fatalf("OpenAccount: %d %d %v", a, b, err)
	}
	check := func(wantA, wantB int64) {
		t.Helper()
		ba, err1 := r.Balance(ctx, a)
		bb, err2 := r.Balance(ctx, b)
		if err1 != nil || err2 != nil || ba != wantA || bb != wantB {
			t.Errorf("балансы = %d, %d (%v, %v); ожидалось %d, %d", ba, bb, err1, err2, wantA, wantB)
		}
	}

	if err := r.Transfer(ctx, a, b, 30); err != nil {
		t.Fatal(err)
	}
	check(70, 30)
	if err := r.Transfer(ctx, a, b, 71); !errors.Is(err, ErrInsufficientFunds) {
		t.Errorf("перевод больше баланса: %v", err)
	}
	check(70, 30)
	if err := r.Transfer(ctx, a, 999, 10); !errors.Is(err, ErrNotFound) {
		t.Errorf("перевод на несуществующий счёт: %v", err)
	}
	check(70, 30) // списание не должно было произойти
	if err := r.Transfer(ctx, 999, a, 10); !errors.Is(err, ErrNotFound) {
		t.Errorf("перевод с несуществующего счёта: %v", err)
	}
	for _, amt := range []int64{0, -5} {
		if err := r.Transfer(ctx, a, b, amt); !errors.Is(err, ErrInvalid) {
			t.Errorf("Transfer(amount=%d): %v", amt, err)
		}
	}
	if err := r.Transfer(ctx, a, a, 1); !errors.Is(err, ErrInvalid) {
		t.Errorf("перевод самому себе: %v", err)
	}
	if err := r.Transfer(ctx, b, a, 30); err != nil {
		t.Fatal(err)
	}
	check(100, 0)
	if _, err := r.Balance(ctx, 999); !errors.Is(err, ErrNotFound) {
		t.Errorf("Balance(999): %v", err)
	}
}

func TestTransferConcurrent(t *testing.T) {
	r, db, ctx := setup(t)
	db.SetMaxOpenConns(4)
	u := &User{Name: "a", Email: "a@x"}
	r.Create(ctx, u)
	a, _ := r.OpenAccount(ctx, u.ID, 50)
	b, _ := r.OpenAccount(ctx, u.ID, 50)
	var wg sync.WaitGroup
	var mu sync.Mutex
	fails := 0
	for i := range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			from, to := a, b
			if i%2 == 1 {
				from, to = b, a
			}
			err := r.Transfer(ctx, from, to, 7)
			if err != nil && !errors.Is(err, ErrInsufficientFunds) {
				t.Errorf("Transfer: %v", err)
			}
			if err != nil {
				mu.Lock()
				fails++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	ba, _ := r.Balance(ctx, a)
	bb, _ := r.Balance(ctx, b)
	if ba+bb != 100 || ba < 0 || bb < 0 {
		t.Errorf("деньги не сохранились: %d + %d (неудачных переводов: %d)", ba, bb, fails)
	}
}

func TestContextCanceled(t *testing.T) {
	r, _, _ := setup(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := r.Create(ctx, &User{Name: "a", Email: "a@x"}); !errors.Is(err, context.Canceled) {
		t.Errorf("Create с отменённым контекстом: %v", err)
	}
	if _, err := r.List(ctx, 1, 0); !errors.Is(err, context.Canceled) {
		t.Errorf("List с отменённым контекстом: %v", err)
	}
}
