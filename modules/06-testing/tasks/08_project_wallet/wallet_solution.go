//go:build solution

package wallet

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"
)

// ---------- Часть 1. Фейки ----------

// FakeClock — управляемые часы.
type FakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func NewFakeClock(t time.Time) *FakeClock { return &FakeClock{t: t} }

func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *FakeClock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = t
}

func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// MemStore — хранилище в памяти. Account — значение без ссылочных полей,
// поэтому map[string]Account сама по себе отдаёт копии.
type MemStore struct {
	PutErr error

	mu   sync.Mutex
	m    map[string]Account
	puts int
}

func NewMemStore(accounts ...Account) *MemStore {
	s := &MemStore{m: make(map[string]Account, len(accounts))}
	for _, a := range accounts {
		s.m[a.ID] = a
	}
	return s
}

func (s *MemStore) Get(_ context.Context, id string) (Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.m[id]
	if !ok {
		return Account{}, fmt.Errorf("memstore: %q: %w", id, ErrNotFound)
	}
	return a, nil
}

func (s *MemStore) Put(_ context.Context, a Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.PutErr != nil {
		return s.PutErr
	}
	if s.m == nil { // нулевое значение MemStore тоже работает
		s.m = make(map[string]Account)
	}
	s.m[a.ID] = a
	s.puts++
	return nil
}

func (s *MemStore) PutCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.puts
}

// SpyNotifier — «шпион»: записывает вызовы, возвращает заданную ошибку.
type SpyNotifier struct {
	Err error

	mu   sync.Mutex
	sent []Notification
}

func (n *SpyNotifier) Notify(_ context.Context, msg Notification) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.sent = append(n.sent, msg)
	return n.Err
}

func (n *SpyNotifier) Sent() []Notification {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.sent) // копия — вызывающий не испортит внутреннее состояние
}

// ---------- Часть 2. Тесты ----------

// env — свежий набор фейков на каждый сценарий: сценарии не влияют друг на друга.
type env struct {
	store *MemStore
	clock *FakeClock
	spy   *SpyNotifier
	svc   Service
}

// Время в НЕ-UTC поясе, близко к полуночи: 01:30 по Москве = 22:30 UTC предыдущего дня.
// Так ловится баг «день считается в локальном времени».
var (
	msk   = time.FixedZone("MSK", 3*60*60)
	start = time.Date(2024, 3, 10, 1, 30, 0, 0, msk)
	today = "2024-03-09" // UTC-день момента start
)

func CheckService(t testing.TB, newService func(Deps) Service) {
	t.Helper()
	ctx := context.Background()
	newEnv := func(accs ...Account) *env {
		e := &env{store: NewMemStore(accs...), clock: NewFakeClock(start), spy: &SpyNotifier{}}
		e.svc = newService(Deps{Store: e.store, Clock: e.clock, Notifier: e.spy})
		return e
	}
	acc := func(e *env, id string) Account {
		t.Helper()
		a, err := e.store.Get(ctx, id)
		if err != nil {
			t.Fatalf("счёт %q пропал из хранилища: %v", id, err)
		}
		return a
	}
	expectErr := func(name string, got, want error) {
		t.Helper()
		if !errors.Is(got, want) {
			t.Errorf("%s: ошибка %v, ожидалась %v", name, got, want)
		}
	}
	expectSent := func(name string, e *env, want ...Notification) {
		t.Helper()
		if got := e.spy.Sent(); !slices.Equal(got, want) {
			t.Errorf("%s: уведомления %v, ожидались %v", name, got, want)
		}
	}
	// unchanged — при ошибке состояние счёта не меняется и уведомлений нет.
	unchanged := func(name string, e *env, before Account) {
		t.Helper()
		if after := acc(e, before.ID); after != before {
			t.Errorf("%s: при ошибке счёт изменился: %+v → %+v", name, before, after)
		}
		if e.store.PutCount() != 0 {
			t.Errorf("%s: при ошибке был Put", name)
		}
		expectSent(name, e)
	}

	// --- Balance ---
	{
		e := newEnv(Account{ID: "a", Balance: 500})
		if b, err := e.svc.Balance(ctx, "a"); err != nil || b != 500 {
			t.Errorf("Balance(a) = %d, %v; ожидалось 500", b, err)
		}
		_, err := e.svc.Balance(ctx, "ghost")
		expectErr("Balance(ghost)", err, ErrNotFound)
	}

	// --- Deposit ---
	{
		e := newEnv(Account{ID: "a", Balance: 100})
		if err := e.svc.Deposit(ctx, "a", 50); err != nil {
			t.Errorf("Deposit: %v", err)
		}
		if a := acc(e, "a"); a.Balance != 150 {
			t.Errorf("после Deposit(50) баланс %d, ожидалось 150", a.Balance)
		}
		expectSent("Deposit", e, Notification{"a", "deposit 50"})
	}
	for _, amount := range []int64{0, -10} {
		e := newEnv(Account{ID: "a", Balance: 100})
		before := acc(e, "a")
		expectErr(fmt.Sprintf("Deposit(%d)", amount), e.svc.Deposit(ctx, "a", amount), ErrInvalidAmount)
		unchanged(fmt.Sprintf("Deposit(%d)", amount), e, before)
	}
	{
		e := newEnv(Account{ID: "a", Balance: 100, Frozen: true})
		before := acc(e, "a")
		expectErr("Deposit(frozen)", e.svc.Deposit(ctx, "a", 10), ErrFrozen)
		unchanged("Deposit(frozen)", e, before)
		expectErr("Deposit(ghost)", e.svc.Deposit(ctx, "ghost", 10), ErrNotFound)
	}
	{
		e := newEnv(Account{ID: "a", Balance: 100})
		e.spy.Err = errors.New("sms gateway down")
		if err := e.svc.Deposit(ctx, "a", 1); err != nil {
			t.Errorf("ошибка уведомления не должна проваливать Deposit: %v", err)
		}
		if acc(e, "a").Balance != 101 {
			t.Error("Deposit при ошибке уведомления не сохранил баланс")
		}
	}
	{
		putErr := errors.New("disk full")
		e := newEnv(Account{ID: "a", Balance: 100})
		e.store.PutErr = putErr
		expectErr("Deposit(PutErr)", e.svc.Deposit(ctx, "a", 1), putErr)
		expectSent("Deposit(PutErr)", e)
	}

	// --- Withdraw: успех ---
	{
		e := newEnv(Account{ID: "a", Balance: 1000, DailyLimit: 500})
		if err := e.svc.Withdraw(ctx, "a", 200); err != nil {
			t.Fatalf("Withdraw(200): %v", err)
		}
		a := acc(e, "a")
		if a.Balance != 800 || a.SpentToday != 200 || a.SpentDay != today {
			t.Errorf("после Withdraw(200): %+v; ожидалось Balance=800 SpentToday=200 SpentDay=%s (UTC!)", a, today)
		}
		expectSent("Withdraw", e, Notification{"a", "withdraw 200"})
		if e.store.PutCount() != 1 {
			t.Errorf("Withdraw: Put вызван %d раз", e.store.PutCount())
		}
		// Ровно до лимита — можно.
		if err := e.svc.Withdraw(ctx, "a", 300); err != nil {
			t.Errorf("Withdraw ровно до лимита: %v", err)
		}
		// Сверх лимита — нельзя (лимит накопительный).
		before := acc(e, "a")
		expectErr("Withdraw сверх лимита", e.svc.Withdraw(ctx, "a", 1), ErrLimitExceeded)
		if acc(e, "a") != before {
			t.Error("при ErrLimitExceeded счёт изменился")
		}
		// Наступил новый UTC-день — лимит сбрасывается.
		e.clock.Advance(24 * time.Hour)
		if err := e.svc.Withdraw(ctx, "a", 400); err != nil {
			t.Errorf("Withdraw в новый день: %v", err)
		}
		if a := acc(e, "a"); a.SpentToday != 400 || a.SpentDay != "2024-03-10" {
			t.Errorf("новый день: %+v", a)
		}
	}
	{
		// Весь баланс списать можно; лимит 0 — без лимита.
		e := newEnv(Account{ID: "a", Balance: 700})
		if err := e.svc.Withdraw(ctx, "a", 700); err != nil {
			t.Errorf("Withdraw всего баланса без лимита: %v", err)
		}
		if acc(e, "a").Balance != 0 {
			t.Error("баланс после списания всего должен быть 0")
		}
	}
	{
		// Старый SpentDay — сброс; в тот же UTC-день (хотя по Москве уже другой) — без сброса.
		e := newEnv(
			Account{ID: "old", Balance: 1000, DailyLimit: 100, SpentDay: "2024-03-08", SpentToday: 100},
			Account{ID: "same", Balance: 1000, DailyLimit: 100, SpentDay: today, SpentToday: 100},
		)
		if err := e.svc.Withdraw(ctx, "old", 100); err != nil {
			t.Errorf("лимит прошлого дня должен сброситься: %v", err)
		}
		expectErr("лимит того же UTC-дня", e.svc.Withdraw(ctx, "same", 1), ErrLimitExceeded)
	}

	// --- Withdraw: ошибки ---
	type failCase struct {
		name   string
		acc    Account
		amount int64
		want   error
	}
	for _, c := range []failCase{
		{"zero", Account{ID: "a", Balance: 100}, 0, ErrInvalidAmount},
		{"negative", Account{ID: "a", Balance: 100}, -5, ErrInvalidAmount},
		{"frozen", Account{ID: "a", Balance: 100, Frozen: true}, 10, ErrFrozen},
		{"funds", Account{ID: "a", Balance: 100}, 101, ErrInsufficientFunds},
		{"limit", Account{ID: "a", Balance: 1000, DailyLimit: 50}, 51, ErrLimitExceeded},
		{"limitBeforeFunds", Account{ID: "a", Balance: 10, DailyLimit: 50}, 60, ErrLimitExceeded},
		{"frozenBeforeAll", Account{ID: "a", Balance: 10, DailyLimit: 5, Frozen: true}, 60, ErrFrozen},
	} {
		e := newEnv(c.acc)
		before := acc(e, "a")
		expectErr("Withdraw/"+c.name, e.svc.Withdraw(ctx, "a", c.amount), c.want)
		unchanged("Withdraw/"+c.name, e, before)
	}
	{
		e := newEnv()
		expectErr("Withdraw(ghost)", e.svc.Withdraw(ctx, "ghost", 1), ErrNotFound)
		expectSent("Withdraw(ghost)", e)
	}
	{
		e := newEnv(Account{ID: "a", Balance: 100})
		e.spy.Err = errors.New("push failed")
		if err := e.svc.Withdraw(ctx, "a", 10); err != nil {
			t.Errorf("ошибка уведомления не должна проваливать Withdraw: %v", err)
		}
		if acc(e, "a").Balance != 90 {
			t.Error("Withdraw при ошибке уведомления не сохранил баланс")
		}
		expectSent("Withdraw при ошибке уведомления", e, Notification{"a", "withdraw 10"})
	}
	{
		putErr := errors.New("disk full")
		e := newEnv(Account{ID: "a", Balance: 100})
		e.store.PutErr = putErr
		expectErr("Withdraw(PutErr)", e.svc.Withdraw(ctx, "a", 1), putErr)
		expectSent("Withdraw(PutErr)", e)
	}
}

// ---------- Часть 3. Горячий путь ----------

// AppendStatement — только Append-функции, никаких промежуточных строк.
func AppendStatement(dst []byte, e Entry) []byte {
	dst = e.Time.UTC().AppendFormat(dst, time.RFC3339) // для RFC3339 есть быстрый путь без аллокаций
	dst = append(dst, ' ')
	dst = append(dst, e.Kind...)
	dst = append(dst, ' ')
	dst = append(dst, e.AccountID...)
	dst = append(dst, ' ')
	u := uint64(e.Amount)
	if e.Amount < 0 {
		dst = append(dst, '-')
		u = -u
	}
	dst = strconv.AppendUint(dst, u/100, 10)
	dst = append(dst, '.')
	cents := u % 100
	dst = append(dst, byte('0'+cents/10), byte('0'+cents%10))
	return append(dst, '\n')
}
