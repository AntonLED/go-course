//go:build !solution

// Package bank — задача к уроку «Указатели, структуры, методы».
package bank

// String возвращает имя операции: "deposit", "withdraw", "transfer_in",
// "transfer_out", "interest"; для неизвестных значений — "TxKind(N)".
func (k TxKind) String() string {
	// TODO: реализуйте
	panic("TODO")
}

// Account — банковский счёт. Суммы — в копейках (int64), чтобы не иметь дела с float.
// Поля неэкспортируемые: менять баланс можно только методами.
type Account struct {
	// TODO: добавьте поля (id, баланс, история операций)
}

// NewAccount создаёт счёт. initial < 0 → ErrInvalidAmount.
// Если initial > 0, в историю пишется Tx{TxDeposit, initial}.
func NewAccount(id string, initial int64) (*Account, error) {
	// TODO: реализуйте
	panic("TODO")
}

// ID возвращает идентификатор счёта.
func (a *Account) ID() string { panic("TODO") }

// Balance возвращает текущий баланс в копейках.
func (a *Account) Balance() int64 { panic("TODO") }

// Deposit пополняет счёт. amount <= 0 → ErrInvalidAmount.
func (a *Account) Deposit(amount int64) error { panic("TODO") }

// Withdraw снимает деньги. amount <= 0 → ErrInvalidAmount,
// amount > баланса → ошибка, для которой errors.Is(err, ErrInsufficientFunds).
// При ошибке состояние счёта не меняется.
func (a *Account) Withdraw(amount int64) error { panic("TODO") }

// History возвращает копию истории операций (изменение результата
// не должно влиять на счёт).
func (a *Account) History() []Tx { panic("TODO") }

// String реализует fmt.Stringer: "<id>: <рубли>.<копейки две цифры>",
// например "acc-1: 12.34", "acc-2: 0.05", "acc-3: 0.00".
func (a *Account) String() string { panic("TODO") }

// Transfer переводит amount со счёта from на счёт to атомарно:
// при любой ошибке оба счёта остаются без изменений.
//   - from == to (тот же указатель) → ErrSameAccount;
//   - amount <= 0 → ErrInvalidAmount; не хватает денег → ErrInsufficientFunds.
//
// В истории: у from — Tx{TxTransferOut, amount}, у to — Tx{TxTransferIn, amount}.
func Transfer(from, to *Account, amount int64) error {
	// TODO: реализуйте
	panic("TODO")
}

// SavingsAccount — накопительный счёт. Встраивает Account, поэтому
// методы Deposit, Balance, History, String и т.д. продвигаются (promoted).
type SavingsAccount struct {
	Account
	MinBalance int64 // неснижаемый остаток
	RateBP     int64 // ставка в базисных пунктах (1% = 100 bp) за один период
}

// NewSavings создаёт накопительный счёт. initial < minBalance или
// отрицательные параметры → ErrInvalidAmount.
func NewSavings(id string, initial, minBalance, rateBP int64) (*SavingsAccount, error) {
	// TODO: реализуйте
	panic("TODO")
}

// Withdraw перекрывает (shadowing) Account.Withdraw: снятие разрешено, только если
// после него баланс >= MinBalance, иначе ошибка с errors.Is(err, ErrInsufficientFunds).
func (s *SavingsAccount) Withdraw(amount int64) error {
	// TODO: реализуйте
	panic("TODO")
}

// AddInterest начисляет проценты: balance*RateBP/10000 (округление вниз).
// Если начисление > 0 — пополняет счёт и пишет Tx{TxInterest, начисление}.
// Возвращает начисленную сумму.
func (s *SavingsAccount) AddInterest() int64 {
	// TODO: реализуйте
	panic("TODO")
}
