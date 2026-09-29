# 06. Банковский счёт

Задача к уроку [«Указатели, структуры, методы»](../../lessons/06-structs-methods.md). Модель банковского счёта — удобный полигон для разговора о получателях методов, встраивании и о том, почему нельзя отдавать наружу внутренний срез. Ошибки здесь обходятся дорого: метод, который молча меняет копию вместо оригинала, в настоящем банке означал бы потерянные деньги.

## Что сделать

В `types.go` уже есть ошибки-сентинелы, тип `TxKind` с константами на `iota` и структура `Tx`. В `bank.go` реализуйте:

```go
func (k TxKind) String() string            // "deposit", ..., "TxKind(42)"

type Account struct{ /* ваши поля */ }
func NewAccount(id string, initial int64) (*Account, error)
func (a *Account) ID() string
func (a *Account) Balance() int64
func (a *Account) Deposit(amount int64) error
func (a *Account) Withdraw(amount int64) error
func (a *Account) History() []Tx           // копия!
func (a *Account) String() string          // "acc-1: 12.34"
func Transfer(from, to *Account, amount int64) error

type SavingsAccount struct {
	Account                                // встраивание
	MinBalance int64
	RateBP     int64
}
func NewSavings(id string, initial, minBalance, rateBP int64) (*SavingsAccount, error)
func (s *SavingsAccount) Withdraw(amount int64) error  // перекрывает Account.Withdraw
func (s *SavingsAccount) AddInterest() int64
```

Все суммы хранятся в копейках (`int64`) — с деньгами во `float64` не работают. Подробные требования к каждому методу написаны в комментариях к заготовке.

## Подвохи

Методы, которые меняют состояние, должны иметь получатель-указатель. С value receiver `Deposit` изменил бы копию счёта, а оригинал остался бы прежним.

С `Stringer` похожая история: `fmt.Sprint(a)` вызовет `String()`, только если этот метод входит в method set значения. Для `*Account` он туда входит. А внутри `TxKind.String()` нельзя вызывать `fmt.Sprintf("%v", k)` — `fmt` снова позовёт `String()`, и вы получите бесконечную рекурсию.

`Transfer` должен быть атомарным: сначала выполните все проверки и только потом меняйте балансы, чтобы при ошибке ни один счёт не остался в промежуточном состоянии.

`History` обязан вернуть копию. Иначе вызывающий код сможет переписать историю операций через общий backing array.

У `SavingsAccount` через встраивание продвигаются `Deposit`, `Balance` и `String`, но `Withdraw` перекрыт собственным методом. `s.Account.Withdraw` — явный вызов базового метода. Это не виртуальный вызов: `Account` ничего не знает о `SavingsAccount`.

Ошибки оборачивайте через `fmt.Errorf("%w: ...", ErrInsufficientFunds, ...)`, потому что тест проверяет их через `errors.Is`.

## Запуск проверки

```bash
go test ./modules/01-intro-part1/tasks/06_bank/
go test -tags solution ./modules/01-intro-part1/tasks/06_bank/
```
