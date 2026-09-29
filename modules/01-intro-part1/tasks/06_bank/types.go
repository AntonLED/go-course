package bank

import "errors"

// Ошибки-сентинелы: проверяйте их через errors.Is.
var (
	ErrInvalidAmount     = errors.New("bank: invalid amount")
	ErrInsufficientFunds = errors.New("bank: insufficient funds")
	ErrSameAccount       = errors.New("bank: transfer to the same account")
)

// TxKind — тип операции. Значения задаются через iota.
type TxKind int

const (
	TxDeposit     TxKind = iota // пополнение (в т.ч. начальный баланс)
	TxWithdraw                  // снятие
	TxTransferIn                // входящий перевод
	TxTransferOut               // исходящий перевод
	TxInterest                  // начисление процентов
)

// Tx — запись в истории операций. Структура сравнима через ==.
type Tx struct {
	Kind   TxKind
	Amount int64 // в копейках, всегда > 0
}
