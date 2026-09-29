// Package config — разбор флагов командной строки и переменных окружения в Config.
package config

import "time"

// Level — уровень логирования. Реализует flag.Value (методы String и Set — часть задачи).
type Level int

const (
	LevelDebug Level = iota - 1
	LevelInfo        // значение по умолчанию (нулевое)
	LevelWarn
	LevelError
)

// StringList — повторяемый флаг: `-tag a -tag b,c` → [a b c]. Реализует flag.Value.
type StringList []string

// Config — итоговая конфигурация.
//
// Приоритет источников: значения по умолчанию < переменные окружения < флаги.
type Config struct {
	Addr    string        // -addr,    env APP_ADDR,    по умолчанию ":8080"
	Timeout time.Duration // -timeout, env APP_TIMEOUT, по умолчанию 5s; должно быть > 0
	Level   Level         // -level,   env APP_LEVEL,   по умолчанию info
	Tags    []string      // -tag (повторяемый, значения через запятую), по умолчанию пусто
	Verbose bool          // -v
	Files   []string      // позиционные аргументы
}
