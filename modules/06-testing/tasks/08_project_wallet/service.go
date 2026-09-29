package wallet

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
)

// Этот файл — «боевой» код сервиса. Его НЕ нужно менять: его нужно покрыть тестами.

// Account — состояние счёта. Деньги — в копейках.
type Account struct {
	ID         string
	Balance    int64
	DailyLimit int64 // лимит списаний за сутки (UTC); 0 — без лимита
	Frozen     bool
	SpentDay   string // "2006-01-02" — UTC-день, к которому относится SpentToday
	SpentToday int64
}

// Notification — уведомление владельцу счёта.
type Notification struct {
	AccountID string
	Text      string
}

// Зависимости сервиса — интерфейсы, объявленные здесь, у потребителя.
type (
	Clock interface {
		Now() time.Time
	}
	Store interface {
		Get(ctx context.Context, id string) (Account, error) // ErrNotFound, если счёта нет
		Put(ctx context.Context, a Account) error
	}
	Notifier interface {
		Notify(ctx context.Context, n Notification) error
	}
)

// Deps — всё, что нужно сервису.
type Deps struct {
	Store    Store
	Clock    Clock
	Notifier Notifier
}

// Service — публичный контракт кошелька.
type Service interface {
	Balance(ctx context.Context, id string) (int64, error)
	Deposit(ctx context.Context, id string, amount int64) error
	Withdraw(ctx context.Context, id string, amount int64) error
}

var (
	ErrNotFound          = errors.New("wallet: account not found")
	ErrInvalidAmount     = errors.New("wallet: amount must be positive")
	ErrFrozen            = errors.New("wallet: account is frozen")
	ErrInsufficientFunds = errors.New("wallet: insufficient funds")
	ErrLimitExceeded     = errors.New("wallet: daily limit exceeded")
)

// Wallet — реализация Service.
type Wallet struct {
	d Deps
}

var _ Service = (*Wallet)(nil)

// New создаёт сервис.
func New(d Deps) *Wallet { return &Wallet{d: d} }

// Balance возвращает баланс. Нет счёта → ошибка, для которой errors.Is(err, ErrNotFound).
func (w *Wallet) Balance(ctx context.Context, id string) (int64, error) {
	a, err := w.d.Store.Get(ctx, id)
	if err != nil {
		return 0, fmt.Errorf("balance %q: %w", id, err)
	}
	return a.Balance, nil
}

// Deposit зачисляет amount.
//
//  1. amount <= 0 → ErrInvalidAmount (хранилище не трогаем);
//  2. счёт не найден → ErrNotFound; заморожен → ErrFrozen;
//  3. Balance += amount, Put (ошибка Put → вернуть её, без уведомления);
//  4. уведомление {ID, "deposit <amount>"}; ошибка уведомления операцию НЕ проваливает.
func (w *Wallet) Deposit(ctx context.Context, id string, amount int64) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	a, err := w.d.Store.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("deposit %q: %w", id, err)
	}
	if a.Frozen {
		return ErrFrozen
	}
	a.Balance += amount
	if err := w.d.Store.Put(ctx, a); err != nil {
		return fmt.Errorf("deposit %q: save: %w", id, err)
	}
	w.notify(ctx, id, "deposit", amount)
	return nil
}

// Withdraw списывает amount. Проверки строго в этом порядке:
//
//  1. amount <= 0 → ErrInvalidAmount (хранилище не трогаем);
//  2. счёт не найден → ErrNotFound; заморожен → ErrFrozen;
//  3. если UTC-день Clock.Now() отличается от SpentDay — SpentToday обнуляется;
//  4. DailyLimit > 0 и SpentToday+amount > DailyLimit → ErrLimitExceeded (ровно до лимита — можно);
//  5. Balance < amount → ErrInsufficientFunds (списать весь баланс — можно);
//  6. Balance -= amount, SpentToday += amount, Put (ошибка → вернуть, без уведомления);
//  7. уведомление {ID, "withdraw <amount>"}; ошибка уведомления операцию НЕ проваливает.
//
// При любой ошибке состояние счёта не меняется и уведомление не отправляется.
func (w *Wallet) Withdraw(ctx context.Context, id string, amount int64) error {
	if amount <= 0 {
		return ErrInvalidAmount
	}
	a, err := w.d.Store.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("withdraw %q: %w", id, err)
	}
	if a.Frozen {
		return ErrFrozen
	}
	day := w.d.Clock.Now().UTC().Format(time.DateOnly)
	if a.SpentDay != day {
		a.SpentDay, a.SpentToday = day, 0
	}
	if a.DailyLimit > 0 && a.SpentToday+amount > a.DailyLimit {
		return ErrLimitExceeded
	}
	if a.Balance < amount {
		return ErrInsufficientFunds
	}
	a.Balance -= amount
	a.SpentToday += amount
	if err := w.d.Store.Put(ctx, a); err != nil {
		return fmt.Errorf("withdraw %q: save: %w", id, err)
	}
	w.notify(ctx, id, "withdraw", amount)
	return nil
}

func (w *Wallet) notify(ctx context.Context, id, kind string, amount int64) {
	// Уведомление — «best effort»: деньги уже списаны, откатывать из-за SMS нельзя.
	_ = w.d.Notifier.Notify(ctx, Notification{AccountID: id, Text: kind + " " + strconv.FormatInt(amount, 10)})
}

// Entry — строка выписки.
type Entry struct {
	Time      time.Time
	Kind      string
	AccountID string
	Amount    int64 // копейки, может быть отрицательной
}
