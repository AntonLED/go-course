package bank

import (
	"errors"
	"fmt"
	"slices"
	"testing"
)

func mustAccount(t *testing.T, id string, initial int64) *Account {
	t.Helper()
	a, err := NewAccount(id, initial)
	if err != nil {
		t.Fatalf("NewAccount(%q, %d): неожиданная ошибка %v", id, initial, err)
	}
	return a
}

func TestTxKindString(t *testing.T) {
	tests := []struct {
		k    TxKind
		want string
	}{
		{TxDeposit, "deposit"},
		{TxWithdraw, "withdraw"},
		{TxTransferIn, "transfer_in"},
		{TxTransferOut, "transfer_out"},
		{TxInterest, "interest"},
		{TxKind(42), "TxKind(42)"},
		{TxKind(-1), "TxKind(-1)"},
	}
	for _, tt := range tests {
		if got := fmt.Sprint(tt.k); got != tt.want {
			t.Errorf("fmt.Sprint(TxKind(%d)) = %q, ожидалось %q", int(tt.k), got, tt.want)
		}
	}
}

func TestNewAccount(t *testing.T) {
	if _, err := NewAccount("x", -1); !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("NewAccount с отрицательным балансом: err = %v, ожидалось ErrInvalidAmount", err)
	}
	a := mustAccount(t, "acc-0", 0)
	if a.Balance() != 0 || len(a.History()) != 0 || a.ID() != "acc-0" {
		t.Errorf("пустой счёт: balance=%d history=%v id=%q", a.Balance(), a.History(), a.ID())
	}
	b := mustAccount(t, "acc-1", 500)
	if want := []Tx{{TxDeposit, 500}}; !slices.Equal(b.History(), want) {
		t.Errorf("история нового счёта = %v, ожидалось %v", b.History(), want)
	}
}

func TestDepositWithdraw(t *testing.T) {
	a := mustAccount(t, "a", 1000)
	if err := a.Deposit(250); err != nil {
		t.Fatalf("Deposit(250): %v", err)
	}
	for _, bad := range []int64{0, -5} {
		if err := a.Deposit(bad); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("Deposit(%d) = %v, ожидалось ErrInvalidAmount", bad, err)
		}
		if err := a.Withdraw(bad); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("Withdraw(%d) = %v, ожидалось ErrInvalidAmount", bad, err)
		}
	}
	if err := a.Withdraw(2000); !errors.Is(err, ErrInsufficientFunds) {
		t.Errorf("Withdraw(2000) при балансе 1250 = %v, ожидалось ErrInsufficientFunds", err)
	}
	if err := a.Withdraw(1250); err != nil {
		t.Errorf("Withdraw всего баланса: %v", err)
	}
	if a.Balance() != 0 {
		t.Errorf("Balance = %d, ожидалось 0", a.Balance())
	}
	want := []Tx{{TxDeposit, 1000}, {TxDeposit, 250}, {TxWithdraw, 1250}}
	if got := a.History(); !slices.Equal(got, want) {
		t.Errorf("History = %v, ожидалось %v (неудачные операции не пишутся)", got, want)
	}
}

func TestHistoryIsCopy(t *testing.T) {
	a := mustAccount(t, "a", 100)
	_ = a.Deposit(1)
	h := a.History()
	h[0].Amount = 999999
	_ = append(h[:1], Tx{TxWithdraw, 1})
	if got := a.History(); got[0].Amount != 100 || got[1] != (Tx{TxDeposit, 1}) {
		t.Errorf("History() вернул срез, разделяющий память со счётом: %v", got)
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		bal  int64
		want string
	}{
		{0, "acc: 0.00"},
		{5, "acc: 0.05"},
		{1234, "acc: 12.34"},
		{100000, "acc: 1000.00"},
	}
	for _, tt := range tests {
		a := mustAccount(t, "acc", tt.bal)
		if got := fmt.Sprint(a); got != tt.want {
			t.Errorf("fmt.Sprint(счёт с %d коп.) = %q, ожидалось %q", tt.bal, got, tt.want)
		}
	}
}

func TestTransfer(t *testing.T) {
	a := mustAccount(t, "a", 1000)
	b := mustAccount(t, "b", 0)

	if err := Transfer(a, b, 300); err != nil {
		t.Fatalf("Transfer: %v", err)
	}
	if a.Balance() != 700 || b.Balance() != 300 {
		t.Errorf("после перевода: a=%d b=%d, ожидалось 700 и 300", a.Balance(), b.Balance())
	}
	if got := a.History()[1]; got != (Tx{TxTransferOut, 300}) {
		t.Errorf("история a: %v", a.History())
	}
	if want := []Tx{{TxTransferIn, 300}}; !slices.Equal(b.History(), want) {
		t.Errorf("история b = %v, ожидалось %v", b.History(), want)
	}

	// Ошибки — без изменений на обоих счетах.
	cases := []struct {
		name     string
		from, to *Account
		amount   int64
		want     error
	}{
		{"мало денег", a, b, 701, ErrInsufficientFunds},
		{"ноль", a, b, 0, ErrInvalidAmount},
		{"отрицательная", b, a, -10, ErrInvalidAmount},
		{"тот же счёт", a, a, 10, ErrSameAccount},
	}
	for _, c := range cases {
		if err := Transfer(c.from, c.to, c.amount); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, ожидалось %v", c.name, err, c.want)
		}
	}
	if a.Balance() != 700 || b.Balance() != 300 || len(a.History()) != 2 || len(b.History()) != 1 {
		t.Errorf("неудачные переводы изменили счета: a=%v b=%v", a.History(), b.History())
	}
}

func TestSavings(t *testing.T) {
	for _, args := range [][3]int64{{50, 100, 0}, {100, -1, 0}, {100, 0, -5}} {
		if _, err := NewSavings("s", args[0], args[1], args[2]); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("NewSavings(%v) = %v, ожидалось ErrInvalidAmount", args, err)
		}
	}

	s, err := NewSavings("sav", 10000, 5000, 150) // 1.5%
	if err != nil {
		t.Fatalf("NewSavings: %v", err)
	}
	// Продвинутые методы Account.
	if got := fmt.Sprint(s); got != "sav: 100.00" {
		t.Errorf("fmt.Sprint(savings) = %q, ожидалось %q (продвижение String)", got, "sav: 100.00")
	}
	if err := s.Deposit(1000); err != nil {
		t.Fatal(err)
	}
	// Перекрытый Withdraw учитывает MinBalance.
	if err := s.Withdraw(6001); !errors.Is(err, ErrInsufficientFunds) {
		t.Errorf("Withdraw ниже неснижаемого остатка: err = %v", err)
	}
	if err := s.Withdraw(6000); err != nil {
		t.Errorf("Withdraw ровно до MinBalance: %v", err)
	}
	if s.Balance() != 5000 {
		t.Fatalf("Balance = %d, ожидалось 5000", s.Balance())
	}
	// Базовый метод доступен явно и игнорирует MinBalance.
	if err := s.Account.Withdraw(100); err != nil {
		t.Errorf("s.Account.Withdraw(100): %v", err)
	}

	if got := s.AddInterest(); got != 73 { // 4900 * 150 / 10000 = 73.5 → 73
		t.Errorf("AddInterest = %d, ожидалось 73", got)
	}
	if s.Balance() != 4973 {
		t.Errorf("Balance после процентов = %d, ожидалось 4973", s.Balance())
	}
	h := s.History()
	if h[len(h)-1] != (Tx{TxInterest, 73}) {
		t.Errorf("последняя операция = %v, ожидалось {interest 73}", h[len(h)-1])
	}

	z, _ := NewSavings("z", 10, 0, 100) // 10*100/10000 = 0
	if got := z.AddInterest(); got != 0 || len(z.History()) != 1 {
		t.Errorf("нулевые проценты не должны писаться в историю: got=%d history=%v", got, z.History())
	}
}
