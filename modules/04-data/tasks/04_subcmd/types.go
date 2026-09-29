// Package subcmd — CLI-утилита textkit с подкомандами, тестируемая без запуска процесса.
package subcmd

// Коды завершения.
const (
	ExitOK      = 0 // успех (в том числе -h)
	ExitFailure = 1 // ошибка выполнения (например, чтение stdin)
	ExitUsage   = 2 // неверное использование: нет/неизвестная команда, плохие флаги/аргументы
)

// Version — версия, которую печатает `textkit version`.
const Version = "1.2.0"

// Usage — текст общей справки (печатается в stderr).
const Usage = `usage: textkit [-json] <command> [flags] [args]

commands:
  count   [-lines] [-words] [-bytes]  статистика по stdin
  repeat  [-n N] [-sep S] TEXT...     повторить текст N раз
  upper                               stdin → stdout в верхнем регистре
  version                             версия
`
