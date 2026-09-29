package appconfig

import (
	"errors"
	"time"
)

// Config — итоговая конфигурация сервиса.
type Config struct {
	Env      string        // dev | stage | prod
	HTTPAddr string        // host:port, например ":8080"
	LogLevel string        // debug | info | warn | error
	Timeout  time.Duration // > 0
	DBURL    string        // секрет: содержит пароль (postgres://user:pass@host/db)
	APIToken string        // секрет
	Features []string      // включённые feature flags
}

// Defaults — самый нижний слой конфигурации.
func Defaults() Config {
	return Config{
		Env:      "dev",
		HTTPAddr: ":8080",
		LogLevel: "info",
		Timeout:  5 * time.Second,
	}
}

// Source — источники конфигурации. Инъекция вместо os.* делает Load тестируемым.
type Source struct {
	Args      []string                     // аргументы командной строки без имени программы
	LookupEnv func(string) (string, bool)  // как os.LookupEnv; nil = окружение пустое
	ReadFile  func(string) ([]byte, error) // как os.ReadFile
}

var (
	// ErrInvalid оборачивает все ошибки валидации.
	ErrInvalid = errors.New("appconfig: некорректная конфигурация")
)
