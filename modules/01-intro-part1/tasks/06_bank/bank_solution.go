//go:build solution

// Package bank — задача к уроку «Указатели, структуры, методы».
package bank

import (
	"fmt"
	"strconv"
)

// String реализует fmt.Stringer для TxKind.
// Получатель по значению: TxKind — маленький тип, копировать дёшево.
func (k TxKind) String() string {
	switch k {
	case TxDeposit:
		return "deposit"
	case TxWithdraw:
		return "withdraw"
	case TxTransferIn:
		return "transfer_in"
	case TxTransferOut:
		return "transfer_out"
	case TxInterest:
		return "interest"
	}
	// Нельзя писать fmt.Sprintf("%v", k) — это вызовет String() рекурсивно.
	return "TxKind(" + strconv.Itoa(int(k)) + ")"
}

// Account — банковский счёт.
type Account struct {
	id      string
	balance int64
	history []Tx
}

// NewAccount создаёт счёт.
func NewAccount(id string, initial int64) (*Account, error) {
	if initial < 0 {
		return nil, ErrInvalidAmount
	}
	a := &Account{id: id} // &T{} — то же, что new(T) + инициализация полей
	if initial > 0 {
		a.apply(TxDeposit, initial)
	}
	return a, nil
}

// apply меняет баланс и пишет историю; вызывается после всех проверок.
func (a *Account) apply(kind TxKind, amount int64) {
	switch kind {
	case TxWithdraw, TxTransferOut:
		a.balance -= amount
	default:
		a.balance += amount
	}
	a.history = append(a.history, Tx{Kind: kind, Amount: amount})
}

// Все методы — с получателем-указателем: они меняют состояние,
// а смешивать value и pointer receivers у одного типа — плохой тон.

// ID возвращает идентификатор счёта.
func (a *Account) ID() string { return a.id }

// Balance возвращает текущий баланс.
func (a *Account) Balance() int64 { return a.balance }

// Deposit пополняет счёт.
func (a *Account) Deposit(amount int64) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	a.apply(TxDeposit, amount)
	return nil
}

// Withdraw снимает деньги.
func (a *Account) Withdraw(amount int64) error {
	if err := a.canWithdraw(amount, 0); err != nil {
		return err
	}
	a.apply(TxWithdraw, amount)
	return nil
}

// canWithdraw проверяет, можно ли снять amount, оставив не меньше floor.
func (a *Account) canWithdraw(amount, floor int64) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	if a.balance-amount < floor {
		return fmt.Errorf("%w: balance %d, requested %d, floor %d",
			ErrInsufficientFunds, a.balance, amount, floor)
	}
	return nil
}

// History возвращает копию истории, чтобы вызывающий не мог
// изменить внутренний срез через общий backing array.
func (a *Account) History() []Tx {
	return append([]Tx(nil), a.history...)
}

// String реализует fmt.Stringer.
func (a *Account) String() string {
	return fmt.Sprintf("%s: %d.%02d", a.id, a.balance/100, a.balance%100)
}

// Transfer переводит деньги атомарно: сначала все проверки, потом изменения.
func Transfer(from, to *Account, amount int64) error {
	if from == to { // сравнение указателей — «тот же объект»
		return ErrSameAccount
	}
	if err := from.canWithdraw(amount, 0); err != nil {
		return err
	}
	from.apply(TxTransferOut, amount)
	to.apply(TxTransferIn, amount)
	return nil
}

// SavingsAccount — накопительный счёт.
type SavingsAccount struct {
	Account
	MinBalance int64
	RateBP     int64
}

// NewSavings создаёт накопительный счёт.
func NewSavings(id string, initial, minBalance, rateBP int64) (*SavingsAccount, error) {
	if minBalance < 0 || rateBP < 0 || initial < minBalance {
		return nil, ErrInvalidAmount
	}
	acc, err := NewAccount(id, initial)
	if err != nil {
		return nil, err
	}
	// *acc копирует структуру Account внутрь SavingsAccount; acc больше не используется.
	return &SavingsAccount{Account: *acc, MinBalance: minBalance, RateBP: rateBP}, nil
}

// Withdraw перекрывает продвинутый Account.Withdraw.
// Базовый метод по-прежнему доступен как s.Account.Withdraw.
func (s *SavingsAccount) Withdraw(amount int64) error {
	if err := s.canWithdraw(amount, s.MinBalance); err != nil {
		return err
	}
	s.apply(TxWithdraw, amount)
	return nil
}

// AddInterest начисляет проценты.
func (s *SavingsAccount) AddInterest() int64 {
	interest := s.balance * s.RateBP / 10000
	if interest > 0 {
		s.apply(TxInterest, interest)
	}
	return interest
}
