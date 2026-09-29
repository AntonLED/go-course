package di

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type Config struct{ DSN string }

type DB struct{ cfg Config }

type UserRepo interface{ Find(id int) string }

type pgRepo struct{ db *DB }

func (r *pgRepo) Find(id int) string { return "user-" + r.db.cfg.DSN }

type Service struct {
	repo UserRepo
	db   *DB
}

func TestGraphAndSingletons(t *testing.T) {
	var cfgCalls, dbCalls, repoCalls atomic.Int32
	c := New()
	mustProvide(t, c, func() Config { cfgCalls.Add(1); return Config{DSN: "pg"} })
	mustProvide(t, c, func(cfg Config) (*DB, error) { dbCalls.Add(1); return &DB{cfg: cfg}, nil })
	mustProvide(t, c, func(db *DB) *pgRepo { repoCalls.Add(1); return &pgRepo{db: db} }, As(new(UserRepo)))
	mustProvide(t, c, func(r UserRepo, db *DB) *Service { return &Service{repo: r, db: db} })

	var s1, s2 *Service
	if err := c.Invoke(func(s *Service) { s1 = s }); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if err := c.Invoke(func(s *Service, r *pgRepo, u UserRepo) error {
		s2 = s
		if UserRepo(r) != u {
			t.Error("*pgRepo и UserRepo должны быть одним и тем же экземпляром (As)")
		}
		return nil
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if s1 == nil || s1 != s2 {
		t.Fatalf("Service должен быть синглтоном: %p != %p", s1, s2)
	}
	if got := s1.repo.Find(1); got != "user-pg" {
		t.Errorf("Find = %q, ожидалось user-pg", got)
	}
	if s1.db != s1.repo.(*pgRepo).db {
		t.Error("*DB должен быть один на весь граф")
	}
	if cfgCalls.Load() != 1 || dbCalls.Load() != 1 || repoCalls.Load() != 1 {
		t.Errorf("каждый конструктор должен вызываться ровно 1 раз: cfg=%d db=%d repo=%d",
			cfgCalls.Load(), dbCalls.Load(), repoCalls.Load())
	}
}

func TestLazy(t *testing.T) {
	c := New()
	called := false
	mustProvide(t, c, func() *DB { called = true; return &DB{} })
	if called {
		t.Fatal("Provide не должен вызывать конструктор")
	}
	if err := c.Invoke(func() {}); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("конструктор ненужного типа не должен вызываться")
	}
}

func TestProvideErrors(t *testing.T) {
	c := New()
	mustProvide(t, c, func() *DB { return &DB{} })
	var nilFn func() int
	tests := []struct {
		name string
		ctor any
		opts []Option
		want error
	}{
		{"не функция", 42, nil, ErrNotFunc},
		{"nil-функция", nilFn, nil, ErrNotFunc},
		{"без результата", func() {}, nil, ErrBadSignature},
		{"три результата", func() (int, string, error) { return 0, "", nil }, nil, ErrBadSignature},
		{"второй не error", func() (int, string) { return 0, "" }, nil, ErrBadSignature},
		{"variadic", func(xs ...int) int { return 0 }, nil, ErrBadSignature},
		{"дубликат", func() *DB { return nil }, nil, ErrDuplicate},
		{"дубликат *Lifecycle", func() *Lifecycle { return nil }, nil, ErrDuplicate},
		{"As не указатель", func() *pgRepo { return nil }, []Option{As(UserRepo(nil))}, ErrBadOption},
		{"As не интерфейс", func() *pgRepo { return nil }, []Option{As(new(DB))}, ErrBadOption},
		{"As не реализует", func() Config { return Config{} }, []Option{As(new(UserRepo))}, ErrBadOption},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := c.Provide(tc.ctor, tc.opts...); !errors.Is(err, tc.want) {
				t.Errorf("Provide = %v, ожидалось %v", err, tc.want)
			}
		})
	}
}

func TestInvokeErrors(t *testing.T) {
	c := New()
	if err := c.Invoke("x"); !errors.Is(err, ErrNotFunc) {
		t.Errorf("Invoke(string) = %v, ожидалось ErrNotFunc", err)
	}
	if err := c.Invoke(func() int { return 1 }); !errors.Is(err, ErrBadSignature) {
		t.Errorf("Invoke(func() int) = %v, ожидалось ErrBadSignature", err)
	}
	err := c.Invoke(func(*DB) {})
	if !errors.Is(err, ErrMissing) || !strings.Contains(err.Error(), "*di.DB") {
		t.Errorf("Invoke без провайдера = %v, ожидалось ErrMissing с упоминанием *di.DB", err)
	}
	sentinel := errors.New("boom")
	if err := c.Invoke(func() error { return sentinel }); err != sentinel {
		t.Errorf("Invoke должен вернуть ошибку функции как есть, получено %v", err)
	}
}

func TestMissingTransitive(t *testing.T) {
	c := New()
	mustProvide(t, c, func(cfg Config) *DB { return &DB{cfg: cfg} })
	err := c.Invoke(func(*DB) {})
	if !errors.Is(err, ErrMissing) || !strings.Contains(err.Error(), "di.Config") {
		t.Errorf("ожидалось ErrMissing для di.Config, получено %v", err)
	}
}

type A struct{}
type B struct{}
type C struct{}

func TestCycle(t *testing.T) {
	c := New()
	mustProvide(t, c, func(*B) *A { return &A{} })
	mustProvide(t, c, func(*C) *B { return &B{} })
	mustProvide(t, c, func(*A) *C { return &C{} })
	err := c.Invoke(func(*A) {})
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("ожидалось ErrCycle, получено %v", err)
	}
	if !strings.Contains(err.Error(), "*di.A -> *di.B -> *di.C -> *di.A") {
		t.Errorf("сообщение должно содержать путь цикла, получено %q", err)
	}
}

func TestSelfCycleViaInterface(t *testing.T) {
	c := New()
	mustProvide(t, c, func(UserRepo) *pgRepo { return &pgRepo{} }, As(new(UserRepo)))
	if err := c.Invoke(func(UserRepo) {}); !errors.Is(err, ErrCycle) {
		t.Fatalf("ожидалось ErrCycle, получено %v", err)
	}
}

func TestConstructorError(t *testing.T) {
	c := New()
	sentinel := errors.New("нет соединения")
	fail := true
	calls := 0
	mustProvide(t, c, func() (*DB, error) {
		calls++
		if fail {
			return nil, sentinel
		}
		return &DB{}, nil
	})
	invoked := false
	err := c.Invoke(func(*DB) { invoked = true })
	if !errors.Is(err, sentinel) {
		t.Fatalf("ожидалась ошибка конструктора (errors.Is), получено %v", err)
	}
	if invoked {
		t.Fatal("функция Invoke не должна вызываться при ошибке конструктора")
	}
	fail = false
	if err := c.Invoke(func(*DB) {}); err != nil {
		t.Fatalf("после ошибки конструктор должен вызываться повторно: %v", err)
	}
	if calls != 2 {
		t.Errorf("calls = %d, ожидалось 2 (ошибка не кэшируется)", calls)
	}
}

func TestLifecycle(t *testing.T) {
	var log []string
	var mu sync.Mutex
	rec := func(s string) {
		mu.Lock()
		defer mu.Unlock()
		log = append(log, s)
	}
	c := New()
	mustProvide(t, c, func(lc *Lifecycle) *DB {
		lc.Append(Hook{
			OnStart: func(context.Context) error { rec("db start"); return nil },
			OnStop:  func(context.Context) error { rec("db stop"); return nil },
		})
		return &DB{}
	})
	mustProvide(t, c, func(lc *Lifecycle, db *DB) *Service {
		lc.Append(Hook{
			OnStart: func(context.Context) error { rec("svc start"); return nil },
			OnStop:  func(context.Context) error { rec("svc stop"); return errors.New("stop fail") },
		})
		return &Service{db: db}
	})
	if err := c.Invoke(func(*Service) {}); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	err := c.Stop(ctx)
	if err == nil || !strings.Contains(err.Error(), "stop fail") {
		t.Errorf("Stop должен вернуть ошибку OnStop, получено %v", err)
	}
	want := "db start,svc start,svc stop,db stop"
	if got := strings.Join(log, ","); got != want {
		t.Errorf("порядок хуков = %s, ожидалось %s", got, want)
	}
}

func TestLifecycleRollback(t *testing.T) {
	var log []string
	c := New()
	sentinel := errors.New("порт занят")
	if err := c.Invoke(func(lc *Lifecycle) {
		lc.Append(Hook{
			OnStart: func(context.Context) error { log = append(log, "1 start"); return nil },
			OnStop:  func(context.Context) error { log = append(log, "1 stop"); return nil },
		})
		lc.Append(Hook{OnStop: func(context.Context) error { log = append(log, "2 stop"); return nil }})
		lc.Append(Hook{
			OnStart: func(context.Context) error { return sentinel },
			OnStop:  func(context.Context) error { log = append(log, "3 stop"); return nil },
		})
	}); err != nil {
		t.Fatal(err)
	}
	err := c.Start(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("Start = %v, ожидалась ошибка OnStart", err)
	}
	want := "1 start,2 stop,1 stop"
	if got := strings.Join(log, ","); got != want {
		t.Errorf("откат = %s, ожидалось %s (хук, чей OnStart упал, не останавливаем)", got, want)
	}
	log = nil
	if err := c.Stop(context.Background()); err != nil || len(log) != 0 {
		t.Errorf("повторный Stop после отката ничего не должен делать: err=%v log=%v", err, log)
	}
}

func TestConcurrentInvoke(t *testing.T) {
	c := New()
	var calls atomic.Int32
	mustProvide(t, c, func() *DB { calls.Add(1); return &DB{} })
	var wg sync.WaitGroup
	ptrs := make([]*DB, 50)
	for i := range ptrs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := c.Invoke(func(db *DB) { ptrs[i] = db }); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Errorf("конструктор вызван %d раз, ожидалось 1", calls.Load())
	}
	for _, p := range ptrs {
		if p != ptrs[0] {
			t.Fatal("все горутины должны получить один экземпляр")
		}
	}
}

func mustProvide(t *testing.T, c *Container, ctor any, opts ...Option) {
	t.Helper()
	if err := c.Provide(ctor, opts...); err != nil {
		t.Fatalf("Provide: %v", err)
	}
}
