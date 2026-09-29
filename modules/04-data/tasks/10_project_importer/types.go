// Package importer — CLI-утилита импорта товаров из CSV / JSON Lines в SQL-базу
// с валидацией, транзакцией и JSON-отчётом («Задание 4»).
package importer

// Коды завершения.
const (
	ExitOK      = 0 // всё импортировано (или -h)
	ExitFailure = 1 // фатальная ошибка (ввод/вывод, формат, БД) или ошибки в режиме -strict
	ExitUsage   = 2 // неверные аргументы
	ExitPartial = 3 // импорт выполнен, но часть записей пропущена как невалидные
)

// Schema — таблица, куда пишет импорт.
const Schema = `CREATE TABLE IF NOT EXISTS products (
	sku         TEXT PRIMARY KEY,
	name        TEXT NOT NULL,
	price_cents INTEGER NOT NULL,
	qty         INTEGER NOT NULL,
	category    TEXT
)`

// Product — валидированная запись.
type Product struct {
	SKU        string
	Name       string
	PriceCents int64
	Qty        int64
	Category   *string // nil → NULL
}

// Report — JSON-отчёт, который Run печатает в stdout.
type Report struct {
	Total    int           `json:"total"`    // сколько записей данных прочитано (без заголовка и пустых строк)
	Inserted int           `json:"inserted"` // новых товаров
	Updated  int           `json:"updated"`  // обновлённых (sku уже был в базе)
	Skipped  int           `json:"skipped"`  // невалидных записей (== len(Errors))
	DryRun   bool          `json:"dry_run"`
	Errors   []RecordError `json:"errors"` // всегда массив, не null
}

// RecordError — ошибка в конкретной записи.
type RecordError struct {
	Line  int    `json:"line"`          // номер строки во входных данных (с 1)
	SKU   string `json:"sku,omitempty"` // если удалось прочитать
	Error string `json:"error"`
}
