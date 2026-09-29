//go:build solution

package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
)

var levelNames = map[Level]string{
	LevelDebug: "debug",
	LevelInfo:  "info",
	LevelWarn:  "warn",
	LevelError: "error",
}

// String возвращает "debug", "info", "warn" или "error" (для неизвестного — "Level(N)").
func (l Level) String() string {
	if s, ok := levelNames[l]; ok {
		return s
	}
	return fmt.Sprintf("Level(%d)", int(l))
}

// Set разбирает уровень без учёта регистра; "warning" — синоним "warn".
func (l *Level) Set(s string) error {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "warning" {
		s = "warn"
	}
	for lv, name := range levelNames {
		if name == s {
			*l = lv
			return nil
		}
	}
	return fmt.Errorf("неизвестный уровень %q (debug|info|warn|error)", s)
}

// String — значения через запятую. По контракту flag.Value метод может быть
// вызван на нулевом получателе (в т.ч. nil-указателе), поэтому nil-safe.
func (s *StringList) String() string {
	if s == nil {
		return ""
	}
	return strings.Join(*s, ",")
}

// Set добавляет значения; пустые элементы пропускаются.
func (s *StringList) Set(v string) error {
	for _, part := range strings.Split(v, ",") {
		if part = strings.TrimSpace(part); part != "" {
			*s = append(*s, part)
		}
	}
	return nil
}

// Parse разбирает args (без имени программы) и окружение.
func Parse(args []string, getenv func(string) string) (Config, error) {
	cfg := Config{
		Addr:    ":8080",
		Timeout: 5 * time.Second,
		Level:   LevelInfo,
	}

	// 1. Окружение перекрывает значения по умолчанию. Ошибку в env лучше
	//    сообщить с именем переменной — пользователь не видит её в командной строке.
	if v := getenv("APP_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := getenv("APP_TIMEOUT"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return Config{}, fmt.Errorf("APP_TIMEOUT: %w", err)
		}
		cfg.Timeout = d
	}
	if v := getenv("APP_LEVEL"); v != "" {
		if err := cfg.Level.Set(v); err != nil {
			return Config{}, fmt.Errorf("APP_LEVEL: %w", err)
		}
	}

	// 2. Флаги перекрывают окружение: текущие значения cfg становятся
	//    значениями по умолчанию для флагов.
	fs := flag.NewFlagSet("app", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.Addr, "addr", cfg.Addr, "адрес для прослушивания")
	fs.DurationVar(&cfg.Timeout, "timeout", cfg.Timeout, "таймаут запроса")
	fs.Var(&cfg.Level, "level", "уровень логирования: debug|info|warn|error")
	var tags StringList
	fs.Var(&tags, "tag", "тег (можно повторять, можно через запятую)")
	fs.BoolVar(&cfg.Verbose, "v", false, "подробный вывод")

	if err := fs.Parse(args); err != nil {
		// Ошибку возвращаем как есть: для -h/-help это flag.ErrHelp, и errors.Is сработает.
		return Config{}, err
	}
	cfg.Tags = tags
	cfg.Files = fs.Args()

	if cfg.Timeout <= 0 {
		return Config{}, errors.New("timeout должен быть > 0")
	}
	return cfg, nil
}
