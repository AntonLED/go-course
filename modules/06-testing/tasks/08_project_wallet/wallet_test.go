package wallet

import (
	"context"
	"errors"
	"fmt"
	"math"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ==================== Часть 1: фейки ====================

func TestFakeClock(t *testing.T) {
	t0 := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	c := NewFakeClock(t0)
	if !c.Now().Equal(t0) {
		t.Fatalf("Now() = %v, ожидалось %v", c.Now(), t0)
	}
	c.Advance(time.Hour)
	if !c.Now().Equal(t0.Add(time.Hour)) {
		t.Errorf("после Advance: %v", c.Now())
	}
	t1 := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	c.Set(t1)
	if !c.Now().Equal(t1) {
		t.Errorf("после Set: %v", c.Now())
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				c.Advance(time.Second)
				_ = c.Now()
			}
		}()
	}
	wg.Wait()
	if !c.Now().Equal(t1.Add(800 * time.Second)) {
		t.Errorf("конкурентные Advance потеряли обновления: %v", c.Now())
	}
	var _ Clock = c
}

func TestMemStore(t *testing.T) {
	ctx := context.Background()
	s := NewMemStore(Account{ID: "a", Balance: 1}, Account{ID: "b", Balance: 2})
	a, err := s.Get(ctx, "a")
	if err != nil || a.Balance != 1 {
		t.Fatalf("Get(a) = %+v, %v", a, err)
	}
	if _, err := s.Get(ctx, "zzz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get несуществующего: %v, ожидалась ErrNotFound", err)
	}
	a.Balance = 999 // изменение копии не должно попасть в хранилище
	if got, _ := s.Get(ctx, "a"); got.Balance != 1 {
		t.Error("Get должен возвращать копию")
	}
	if s.PutCount() != 0 {
		t.Errorf("PutCount() = %d до Put", s.PutCount())
	}
	if err := s.Put(ctx, Account{ID: "c", Balance: 3}); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(ctx, "c"); err != nil || got.Balance != 3 || s.PutCount() != 1 {
		t.Errorf("после Put: %+v %v, PutCount=%d", got, err, s.PutCount())
	}
	boom := errors.New("boom")
	s.PutErr = boom
	if err := s.Put(ctx, Account{ID: "d"}); !errors.Is(err, boom) {
		t.Errorf("PutErr: %v", err)
	}
	if _, err := s.Get(ctx, "d"); !errors.Is(err, ErrNotFound) || s.PutCount() != 1 {
		t.Error("при PutErr ничего не должно сохраняться и считаться")
	}
	s.PutErr = nil
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = s.Put(ctx, Account{ID: strconv.Itoa(i)})
				_, _ = s.Get(ctx, "a")
			}
		}(i)
	}
	wg.Wait()
	if s.PutCount() != 401 {
		t.Errorf("PutCount() = %d, ожидалось 401", s.PutCount())
	}
	var _ Store = s
}

func TestSpyNotifier(t *testing.T) {
	ctx := context.Background()
	n := &SpyNotifier{}
	if len(n.Sent()) != 0 {
		t.Error("новый шпион должен быть пуст")
	}
	_ = n.Notify(ctx, Notification{"a", "x"})
	n.Err = errors.New("down")
	if err := n.Notify(ctx, Notification{"b", "y"}); !errors.Is(err, n.Err) {
		t.Errorf("Notify должен возвращать Err: %v", err)
	}
	sent := n.Sent()
	if len(sent) != 2 || sent[0] != (Notification{"a", "x"}) || sent[1] != (Notification{"b", "y"}) {
		t.Errorf("Sent() = %v", sent)
	}
	sent[0].Text = "hacked"
	if n.Sent()[0].Text != "x" {
		t.Error("Sent() должен возвращать копию")
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_ = n.Notify(ctx, Notification{})
				_ = n.Sent()
			}
		}()
	}
	wg.Wait()
	if len(n.Sent()) != 402 {
		t.Errorf("потеряны вызовы: %d", len(n.Sent()))
	}
	var _ Notifier = n
}

// ==================== Часть 2: CheckService против мутантов ====================

type spyTB struct {
	testing.TB
	mu     sync.Mutex
	failed bool
	msgs   []string
}

func (s *spyTB) rec(fail bool, m string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failed = s.failed || fail
	s.msgs = append(s.msgs, m)
}
func (s *spyTB) Helper()                   {}
func (s *spyTB) Log(a ...any)              { s.rec(false, fmt.Sprint(a...)) }
func (s *spyTB) Logf(f string, a ...any)   { s.rec(false, fmt.Sprintf(f, a...)) }
func (s *spyTB) Error(a ...any)            { s.rec(true, fmt.Sprint(a...)) }
func (s *spyTB) Errorf(f string, a ...any) { s.rec(true, fmt.Sprintf(f, a...)) }
func (s *spyTB) Fail()                     { s.rec(true, "Fail") }
func (s *spyTB) FailNow()                  { s.rec(true, "FailNow"); runtime.Goexit() }
func (s *spyTB) Fatal(a ...any)            { s.rec(true, fmt.Sprint(a...)); runtime.Goexit() }
func (s *spyTB) Fatalf(f string, a ...any) { s.rec(true, fmt.Sprintf(f, a...)); runtime.Goexit() }
func (s *spyTB) SkipNow()                  { s.rec(false, "SkipNow"); runtime.Goexit() }
func (s *spyTB) Skip(a ...any)             { s.SkipNow() }
func (s *spyTB) Skipf(f string, a ...any)  { s.SkipNow() }
func (s *spyTB) Failed() bool              { s.mu.Lock(); defer s.mu.Unlock(); return s.failed }

func runCheck(t *testing.T, newService func(Deps) Service) *spyTB {
	spy := &spyTB{TB: t}
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			if v := recover(); v != nil {
				spy.rec(true, fmt.Sprintf("panic: %v", v))
			}
		}()
		CheckService(spy, newService)
	}()
	<-done
	return spy
}

// mutant — копия Wallet с «переключателем» бага.
type mutant struct {
	d   Deps
	bug string
}

func (m *mutant) is(b string) bool { return m.bug == b }

func (m *mutant) Balance(ctx context.Context, id string) (int64, error) {
	a, err := m.d.Store.Get(ctx, id)
	if err != nil {
		if m.is("balance-missing-zero") {
			return 0, nil
		}
		return 0, err
	}
	return a.Balance, nil
}

func (m *mutant) notify(ctx context.Context, id, kind string, amount int64) error {
	text := kind + " " + strconv.FormatInt(amount, 10)
	if m.is("wrong-notify-text") && kind == "withdraw" {
		text = "withdrawal " + strconv.FormatInt(amount, 10)
	}
	return m.d.Notifier.Notify(ctx, Notification{id, text})
}

func (m *mutant) Deposit(ctx context.Context, id string, amount int64) error {
	if amount <= 0 && !m.is("deposit-negative-allowed") {
		return ErrInvalidAmount
	}
	a, err := m.d.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if a.Frozen && !m.is("deposit-ignores-frozen") {
		return ErrFrozen
	}
	a.Balance += amount
	if !m.is("deposit-no-put") {
		if err := m.d.Store.Put(ctx, a); err != nil {
			return err
		}
	}
	_ = m.notify(ctx, id, "deposit", amount)
	return nil
}

func (m *mutant) Withdraw(ctx context.Context, id string, amount int64) (err error) {
	if m.is("notify-on-failure") {
		defer func() {
			if err != nil {
				_ = m.notify(ctx, id, "withdraw", amount)
			}
		}()
	}
	if amount < 0 || amount == 0 && !m.is("withdraw-zero-allowed") {
		return ErrInvalidAmount
	}
	a, err := m.d.Store.Get(ctx, id)
	if err != nil {
		return err
	}
	if a.Frozen && !m.is("withdraw-ignores-frozen") && !m.is("frozen-after-funds") {
		return ErrFrozen
	}
	now := m.d.Clock.Now()
	if !m.is("day-local-time") {
		now = now.UTC()
	}
	day := now.Format(time.DateOnly)
	if a.SpentDay != day && !m.is("limit-no-day-reset") {
		a.SpentDay, a.SpentToday = day, 0
	}
	fundsErr := func() error {
		if m.is("funds-strict") && a.Balance <= amount || a.Balance < amount {
			return ErrInsufficientFunds
		}
		return nil
	}
	if m.is("funds-before-limit") {
		if err := fundsErr(); err != nil {
			return err
		}
	}
	over := a.SpentToday+amount > a.DailyLimit
	if m.is("limit-inclusive") {
		over = a.SpentToday+amount >= a.DailyLimit
	}
	if m.is("limit-not-accumulated") {
		over = amount > a.DailyLimit
	}
	limited := a.DailyLimit > 0 || m.is("limit-zero-means-zero")
	if limited && over && !m.is("limit-ignored") {
		return ErrLimitExceeded
	}
	if err := fundsErr(); err != nil {
		return err
	}
	if a.Frozen && m.is("frozen-after-funds") {
		return ErrFrozen
	}
	a.Balance -= amount
	a.SpentToday += amount
	if m.is("notify-before-put") {
		_ = m.notify(ctx, id, "withdraw", amount)
	}
	if !m.is("no-put-on-withdraw") {
		if err := m.d.Store.Put(ctx, a); err != nil && !m.is("put-error-ignored") {
			return err
		}
	}
	if m.is("no-notify-withdraw") || m.is("notify-before-put") {
		return nil
	}
	if err := m.notify(ctx, id, "withdraw", amount); err != nil && m.is("notify-error-propagated") {
		return err
	}
	return nil
}

var bugs = []string{
	"balance-missing-zero", "deposit-negative-allowed", "deposit-ignores-frozen", "deposit-no-put",
	"withdraw-zero-allowed", "withdraw-ignores-frozen", "frozen-after-funds", "day-local-time",
	"limit-no-day-reset", "funds-strict", "funds-before-limit", "limit-inclusive", "limit-not-accumulated",
	"limit-zero-means-zero", "limit-ignored", "notify-before-put", "no-put-on-withdraw", "put-error-ignored",
	"no-notify-withdraw", "notify-error-propagated", "notify-on-failure", "wrong-notify-text",
}

func TestCheckServicePassesOnWallet(t *testing.T) {
	spy := runCheck(t, func(d Deps) Service { return New(d) })
	if spy.failed {
		t.Fatalf("CheckService нашла ошибки в ПРАВИЛЬНОМ Wallet:\n%s", strings.Join(spy.msgs, "\n"))
	}
	// «Нулевой» мутант совпадает с Wallet — проверка тестового стенда.
	spy = runCheck(t, func(d Deps) Service { return &mutant{d: d} })
	if spy.failed {
		t.Fatalf("CheckService падает на мутанте без багов:\n%s", strings.Join(spy.msgs, "\n"))
	}
	var calls int
	runCheck(t, func(d Deps) Service { calls++; return New(d) })
	if calls < 3 {
		t.Errorf("CheckService создала сервис %d раз — используйте свежие фейки для независимых сценариев", calls)
	}
}

func TestCheckServiceCatchesMutants(t *testing.T) {
	sort.Strings(bugs)
	for _, bug := range bugs {
		t.Run(bug, func(t *testing.T) {
			spy := runCheck(t, func(d Deps) Service { return &mutant{d: d, bug: bug} })
			if !spy.failed {
				t.Errorf("мутант %q выжил — добавьте сценарий, который его ловит", bug)
			}
		})
	}
}

// ==================== Часть 3: горячий путь ====================

func refStatement(e Entry) string {
	u := uint64(e.Amount)
	sign := ""
	if e.Amount < 0 {
		sign, u = "-", -u
	}
	return fmt.Sprintf("%s %s %s %s%d.%02d\n", e.Time.UTC().Format(time.RFC3339), e.Kind, e.AccountID, sign, u/100, u%100)
}

func raceEnabled() bool {
	if bi, ok := debug.ReadBuildInfo(); ok {
		for _, s := range bi.Settings {
			if s.Key == "-race" {
				return s.Value == "true"
			}
		}
	}
	return false
}

func TestAppendStatement(t *testing.T) {
	msk := time.FixedZone("MSK", 3*3600)
	entries := []Entry{
		{time.Date(2024, 3, 5, 10, 0, 0, 0, time.UTC), "withdraw", "alice", -1234},
		{time.Date(2024, 3, 5, 1, 0, 0, 0, msk), "deposit", "bob", 5},
		{time.Date(1999, 12, 31, 23, 59, 59, 999, time.UTC), "fee", "", 0},
		{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "x", "кошелёк", 100},
		{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "min", "m", math.MinInt64},
		{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "max", "m", math.MaxInt64},
		{time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), "neg", "m", -7},
	}
	for _, e := range entries {
		got := string(AppendStatement([]byte(">"), e))
		if want := ">" + refStatement(e); got != want {
			t.Errorf("AppendStatement(%+v)\n получено %q\nожидалось %q", e, got, want)
		}
	}
	if raceEnabled() {
		t.Log("проверка аллокаций пропущена под -race")
		return
	}
	e := entries[0]
	buf := make([]byte, 0, 128)
	if n := testing.AllocsPerRun(200, func() { buf = AppendStatement(buf[:0], e) }); n > 0 {
		t.Errorf("AppendStatement: %.0f аллокаций на вызов, лимит 0", n)
	}
}

func BenchmarkAppendStatement(b *testing.B) {
	e := Entry{time.Date(2024, 3, 5, 10, 0, 0, 0, time.UTC), "withdraw", "alice", -1234}
	buf := make([]byte, 0, 128)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf = AppendStatement(buf[:0], e)
	}
}
