//go:build !solution

package appconfig

import "log/slog"

// Load собирает конфигурацию по слоям (каждый следующий перекрывает предыдущий):
//
//	Defaults() < JSON-файл < переменные окружения APP_* < флаги командной строки
//
// и валидирует результат (Validate). Подробности — в README.
func Load(src Source) (Config, error) {
	panic("TODO")
}

// Validate проверяет конфигурацию; все нарушения объединяются, каждое
// оборачивает ErrInvalid.
func (c Config) Validate() error {
	panic("TODO")
}

// Enabled сообщает, включён ли feature flag (без учёта регистра).
func (c Config) Enabled(feature string) bool {
	panic("TODO")
}

// String возвращает представление конфигурации с замаскированными секретами.
func (c Config) String() string {
	panic("TODO")
}

// LogValue реализует slog.LogValuer: секреты не должны попадать в логи.
func (c Config) LogValue() slog.Value {
	panic("TODO")
}
