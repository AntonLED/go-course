package redact

import "log/slog"

// Redacted — чем заменяется значение секретного ключа.
const Redacted = "[REDACTED]"

// DefaultRedactKeys используются, если Options.RedactKeys == nil.
var DefaultRedactKeys = []string{"password", "token", "secret", "authorization"}

// Options настраивает Handler.
type Options struct {
	// Level — минимальный уровень; nil означает slog.LevelInfo.
	Level slog.Leveler
	// RedactKeys — ключи (без учёта регистра), значения которых маскируются
	// на любом уровне вложенности. nil → DefaultRedactKeys.
	RedactKeys []string
}
