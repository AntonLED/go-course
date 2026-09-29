//go:build solution

package appconfig

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// fileConfig — схема JSON-файла. Указатели позволяют отличить «ключ
// отсутствует» (nil — слой не перекрывает) от «задано нулевое значение».
type fileConfig struct {
	Env      *string   `json:"env"`
	HTTPAddr *string   `json:"http_addr"`
	LogLevel *string   `json:"log_level"`
	Timeout  *string   `json:"timeout"` // "5s": time.Duration в JSON — это число наносекунд
	DBURL    *string   `json:"db_url"`
	APIToken *string   `json:"api_token"`
	Features *[]string `json:"features"`
}

// Load собирает конфигурацию: Defaults < файл < env < флаги, затем валидирует.
func Load(src Source) (Config, error) {
	lookup := src.LookupEnv
	if lookup == nil {
		lookup = func(string) (string, bool) { return "", false }
	}

	// 1. Флаги разбираем первыми (путь к файлу может прийти из -config),
	//    но применяем последними.
	fs := flag.NewFlagSet("app", flag.ContinueOnError)
	fs.SetOutput(io.Discard) // библиотечный код не пишет в stderr
	configPath := fs.String("config", "", "путь к JSON-файлу конфигурации")
	fEnv := fs.String("env", "", "окружение: dev|stage|prod")
	fAddr := fs.String("http-addr", "", "адрес HTTP-сервера")
	fLevel := fs.String("log-level", "", "уровень логирования")
	fTimeout := fs.Duration("timeout", 0, "таймаут запросов")
	fDB := fs.String("db-url", "", "строка подключения к БД")
	fToken := fs.String("api-token", "", "API-токен")
	fFeatures := fs.String("features", "", "feature flags через запятую")
	if err := fs.Parse(src.Args); err != nil {
		return Config{}, fmt.Errorf("appconfig: флаги: %w", err)
	}
	// Какие флаги были заданы явно: flag.Visit обходит только их.
	// Иначе значение по умолчанию флага ("") затёрло бы нижние слои.
	setFlags := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })

	cfg := Defaults()

	// 2. Файл: путь из -config, иначе из APP_CONFIG.
	path, _ := lookup("APP_CONFIG")
	if setFlags["config"] {
		path = *configPath
	}
	if path != "" {
		if err := applyFile(&cfg, path, src.ReadFile); err != nil {
			return Config{}, err
		}
	}

	// 3. Переменные окружения.
	if err := applyEnv(&cfg, lookup); err != nil {
		return Config{}, err
	}

	// 4. Явно заданные флаги.
	for name := range setFlags {
		switch name {
		case "env":
			cfg.Env = *fEnv
		case "http-addr":
			cfg.HTTPAddr = *fAddr
		case "log-level":
			cfg.LogLevel = *fLevel
		case "timeout":
			cfg.Timeout = *fTimeout
		case "db-url":
			cfg.DBURL = *fDB
		case "api-token":
			cfg.APIToken = *fToken
		case "features":
			cfg.Features = splitList(*fFeatures)
		}
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func applyFile(cfg *Config, path string, readFile func(string) ([]byte, error)) error {
	if readFile == nil {
		return fmt.Errorf("appconfig: задан файл %q, но ReadFile == nil", path)
	}
	data, err := readFile(path)
	if err != nil {
		return fmt.Errorf("appconfig: чтение %s: %w", path, err)
	}
	var fc fileConfig
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields() // опечатка в ключе не должна молча игнорироваться
	if err := dec.Decode(&fc); err != nil {
		return fmt.Errorf("appconfig: разбор %s: %w", path, err)
	}
	setIf(&cfg.Env, fc.Env)
	setIf(&cfg.HTTPAddr, fc.HTTPAddr)
	setIf(&cfg.LogLevel, fc.LogLevel)
	setIf(&cfg.DBURL, fc.DBURL)
	setIf(&cfg.APIToken, fc.APIToken)
	if fc.Timeout != nil {
		d, err := time.ParseDuration(*fc.Timeout)
		if err != nil {
			return fmt.Errorf("appconfig: %s: timeout: %w", path, err)
		}
		cfg.Timeout = d
	}
	if fc.Features != nil {
		cfg.Features = slices.Clone(*fc.Features)
	}
	return nil
}

func setIf[T any](dst *T, src *T) {
	if src != nil {
		*dst = *src
	}
}

func applyEnv(cfg *Config, lookup func(string) (string, bool)) error {
	strs := []struct {
		key string
		dst *string
	}{
		{"APP_ENV", &cfg.Env},
		{"APP_HTTP_ADDR", &cfg.HTTPAddr},
		{"APP_LOG_LEVEL", &cfg.LogLevel},
		{"APP_DB_URL", &cfg.DBURL},
		{"APP_API_TOKEN", &cfg.APIToken},
	}
	for _, s := range strs {
		if v, ok := lookup(s.key); ok {
			*s.dst = v
		}
	}
	if v, ok := lookup("APP_TIMEOUT"); ok {
		d, err := time.ParseDuration(v)
		if err != nil {
			return fmt.Errorf("appconfig: APP_TIMEOUT: %w", err)
		}
		cfg.Timeout = d
	}
	if v, ok := lookup("APP_FEATURES"); ok {
		cfg.Features = splitList(v)
	}
	return nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Validate проверяет конфигурацию и возвращает все нарушения сразу.
func (c Config) Validate() error {
	var errs []error
	bad := func(format string, args ...any) {
		errs = append(errs, fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, args...)...))
	}
	if !slices.Contains([]string{"dev", "stage", "prod"}, c.Env) {
		bad("env=%q, ожидается dev|stage|prod", c.Env)
	}
	if !slices.Contains([]string{"debug", "info", "warn", "error"}, c.LogLevel) {
		bad("log_level=%q", c.LogLevel)
	}
	if _, port, err := net.SplitHostPort(c.HTTPAddr); err != nil {
		bad("http_addr=%q: %v", c.HTTPAddr, err)
	} else if p, err := strconv.Atoi(port); err != nil || p < 1 || p > 65535 {
		bad("http_addr=%q: некорректный порт", c.HTTPAddr)
	}
	if c.Timeout <= 0 {
		bad("timeout должен быть > 0, получено %s", c.Timeout)
	}
	if c.DBURL != "" {
		if u, err := url.Parse(c.DBURL); err != nil || u.Scheme == "" || u.Host == "" {
			// Саму строку в ошибку не кладём: в ней может быть пароль.
			bad("db_url: некорректный URL")
		}
	}
	// В проде требования строже.
	if c.Env == "prod" {
		if c.DBURL == "" {
			bad("db_url обязателен в prod")
		}
		if c.APIToken == "" {
			bad("api_token обязателен в prod")
		}
		if c.LogLevel == "debug" {
			bad("log_level=debug запрещён в prod")
		}
	}
	return errors.Join(errs...)
}

// Enabled сообщает, включён ли feature flag (без учёта регистра).
func (c Config) Enabled(feature string) bool {
	return slices.ContainsFunc(c.Features, func(f string) bool { return strings.EqualFold(f, feature) })
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	return "***"
}

// maskURL заменяет пароль на xxxxx (url.URL.Redacted). Если URL не
// разбирается — маскируем целиком: лучше потерять информацию, чем секрет.
func maskURL(s string) string {
	if s == "" {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil {
		return "***"
	}
	return u.Redacted()
}

// String возвращает представление с замаскированными секретами.
func (c Config) String() string {
	return fmt.Sprintf("env=%s http_addr=%s log_level=%s timeout=%s db_url=%s api_token=%s features=[%s]",
		c.Env, c.HTTPAddr, c.LogLevel, c.Timeout, maskURL(c.DBURL), maskSecret(c.APIToken),
		strings.Join(c.Features, ","))
}

// LogValue реализует slog.LogValuer.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("env", c.Env),
		slog.String("http_addr", c.HTTPAddr),
		slog.String("log_level", c.LogLevel),
		slog.Duration("timeout", c.Timeout),
		slog.String("db_url", maskURL(c.DBURL)),
		slog.String("api_token", maskSecret(c.APIToken)),
		slog.String("features", strings.Join(c.Features, ",")),
	)
}
