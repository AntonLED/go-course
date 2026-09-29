//go:build !solution

package config

// String возвращает "debug", "info", "warn" или "error" (для неизвестного — "Level(N)").
func (l Level) String() string {
	// TODO
	return ""
}

// Set разбирает уровень без учёта регистра; "warning" — синоним "warn".
// Неизвестное значение — ошибка.
func (l *Level) Set(s string) error {
	// TODO
	return nil
}

// String — значения через запятую.
func (s *StringList) String() string {
	// TODO
	return ""
}

// Set добавляет значения; "a,b" добавляет два элемента, пустые элементы
// (после strings.TrimSpace) пропускаются.
func (s *StringList) Set(v string) error {
	// TODO
	return nil
}

// Parse разбирает args (без имени программы, т.е. os.Args[1:]) и окружение.
//
// getenv — функция вида os.Getenv (в тестах подменяется). Ошибки:
//   - -h/-help → ошибка, для которой errors.Is(err, flag.ErrHelp);
//   - неизвестный флаг, неверное значение флага или переменной окружения → ошибка;
//   - Timeout <= 0 → ошибка.
//
// Используйте собственный flag.NewFlagSet(..., flag.ContinueOnError) и
// fs.SetOutput(io.Discard) — Parse не должна ничего печатать и вызывать os.Exit.
func Parse(args []string, getenv func(string) string) (Config, error) {
	// TODO
	return Config{}, nil
}
