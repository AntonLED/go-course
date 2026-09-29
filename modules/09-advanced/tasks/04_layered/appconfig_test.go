package appconfig

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"
)

func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func filesOf(m map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if s, ok := m[p]; ok {
			return []byte(s), nil
		}
		return nil, fmt.Errorf("open %s: %w", p, fs.ErrNotExist)
	}
}

func TestDefaultsOnly(t *testing.T) {
	cfg, err := Load(Source{})
	if err != nil {
		t.Fatalf("Load(пусто) = %v", err)
	}
	if !reflect.DeepEqual(cfg, Defaults()) {
		t.Errorf("Load(пусто) = %+v, ожидалось Defaults() = %+v", cfg, Defaults())
	}
}

func TestPriority(t *testing.T) {
	files := filesOf(map[string]string{
		"/etc/app.json": `{"env":"stage","http_addr":":9000","log_level":"warn","timeout":"10s","features":["a","b"],"api_token":"file-token"}`,
		"/other.json":   `{"http_addr":":7000"}`,
	})
	tests := []struct {
		name string
		src  Source
		want func(*Config)
	}{
		{
			name: "только файл через APP_CONFIG",
			src:  Source{LookupEnv: envOf(map[string]string{"APP_CONFIG": "/etc/app.json"}), ReadFile: files},
			want: func(c *Config) {
				c.Env, c.HTTPAddr, c.LogLevel, c.Timeout = "stage", ":9000", "warn", 10*time.Second
				c.Features, c.APIToken = []string{"a", "b"}, "file-token"
			},
		},
		{
			name: "env перекрывает файл",
			src: Source{LookupEnv: envOf(map[string]string{
				"APP_CONFIG": "/etc/app.json", "APP_HTTP_ADDR": ":9100", "APP_TIMEOUT": "1m", "APP_FEATURES": " x , ,y ",
			}), ReadFile: files},
			want: func(c *Config) {
				c.Env, c.HTTPAddr, c.LogLevel, c.Timeout = "stage", ":9100", "warn", time.Minute
				c.Features, c.APIToken = []string{"x", "y"}, "file-token"
			},
		},
		{
			name: "флаги перекрывают env, незаданные флаги — нет",
			src: Source{
				Args:      []string{"-http-addr=:9200", "-timeout", "2s", "-log-level=error"},
				LookupEnv: envOf(map[string]string{"APP_CONFIG": "/etc/app.json", "APP_HTTP_ADDR": ":9100", "APP_ENV": "dev"}),
				ReadFile:  files,
			},
			want: func(c *Config) {
				c.Env, c.HTTPAddr, c.LogLevel, c.Timeout = "dev", ":9200", "error", 2*time.Second
				c.Features, c.APIToken = []string{"a", "b"}, "file-token"
			},
		},
		{
			name: "флаг -config важнее APP_CONFIG",
			src: Source{
				Args:      []string{"-config", "/other.json"},
				LookupEnv: envOf(map[string]string{"APP_CONFIG": "/etc/app.json"}),
				ReadFile:  files,
			},
			want: func(c *Config) { c.HTTPAddr = ":7000" },
		},
		{
			name: "явно пустой флаг тоже перекрывает",
			src: Source{
				Args:      []string{"-api-token="},
				LookupEnv: envOf(map[string]string{"APP_API_TOKEN": "env-token"}),
			},
			want: func(c *Config) {},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Load(tc.src)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			want := Defaults()
			tc.want(&want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Load:\n получено  %+v\n ожидалось %+v", got, want)
			}
		})
	}
}

func TestLoadErrors(t *testing.T) {
	files := filesOf(map[string]string{
		"/typo.json":   `{"htp_addr":":1"}`,
		"/bad.json":    `{"timeout":"soon"}`,
		"/broken.json": `{`,
	})
	tests := []struct {
		name string
		src  Source
	}{
		{"нет файла", Source{Args: []string{"-config=/nope.json"}, ReadFile: files}},
		{"неизвестный ключ", Source{Args: []string{"-config=/typo.json"}, ReadFile: files}},
		{"плохой timeout в файле", Source{Args: []string{"-config=/bad.json"}, ReadFile: files}},
		{"битый JSON", Source{Args: []string{"-config=/broken.json"}, ReadFile: files}},
		{"плохой APP_TIMEOUT", Source{LookupEnv: envOf(map[string]string{"APP_TIMEOUT": "10"})}},
		{"неизвестный флаг", Source{Args: []string{"-verbose"}}},
		{"плохой -timeout", Source{Args: []string{"-timeout=abc"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Load(tc.src); err == nil {
				t.Error("ожидалась ошибка")
			}
		})
	}
	// Отсутствие файла должно сохранять цепочку ошибок.
	_, err := Load(Source{Args: []string{"-config=/nope.json"}, ReadFile: files})
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("ошибка чтения файла должна оборачивать исходную (%%w): %v", err)
	}
}

func TestValidate(t *testing.T) {
	prod := Defaults()
	prod.Env, prod.DBURL, prod.APIToken = "prod", "postgres://app:pw@db:5432/app", "t"
	tests := []struct {
		name    string
		mut     func(*Config)
		wantErr []string // подстроки, каждая должна встретиться
	}{
		{"валидный prod", func(c *Config) {}, nil},
		{"плохой env", func(c *Config) { c.Env = "production" }, []string{"env"}},
		{"плохой уровень", func(c *Config) { c.LogLevel = "trace" }, []string{"log_level"}},
		{"адрес без порта", func(c *Config) { c.HTTPAddr = "localhost" }, []string{"http_addr"}},
		{"порт вне диапазона", func(c *Config) { c.HTTPAddr = ":70000" }, []string{"http_addr"}},
		{"нулевой timeout", func(c *Config) { c.Timeout = 0 }, []string{"timeout"}},
		{"debug в prod", func(c *Config) { c.LogLevel = "debug" }, []string{"debug"}},
		{"prod без секретов — обе ошибки", func(c *Config) { c.DBURL, c.APIToken = "", "" }, []string{"db_url", "api_token"}},
		{"db_url без схемы", func(c *Config) { c.DBURL = "db:5432" }, []string{"db_url"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := prod
			tc.mut(&c)
			err := c.Validate()
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("Validate = %v, ожидалось nil", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate = %v, ожидалась ошибка, оборачивающая ErrInvalid", err)
			}
			for _, s := range tc.wantErr {
				if !strings.Contains(err.Error(), s) {
					t.Errorf("ошибка %q должна упоминать %q", err, s)
				}
			}
		})
	}
	// Ошибки валидации должны пробрасываться из Load.
	if _, err := Load(Source{Args: []string{"-env=prod"}}); !errors.Is(err, ErrInvalid) {
		t.Errorf("Load(-env=prod без секретов) = %v, ожидалось ErrInvalid", err)
	}
}

func TestSecretsMasked(t *testing.T) {
	c := Defaults()
	c.Env = "prod"
	c.DBURL = "postgres://app:SuperSecret@db:5432/app"
	c.APIToken = "tok-123456"
	c.Features = []string{"new-ui", "beta"}

	s := c.String()
	want := "env=prod http_addr=:8080 log_level=info timeout=5s db_url=postgres://app:xxxxx@db:5432/app api_token=*** features=[new-ui,beta]"
	if s != want {
		t.Errorf("String():\n получено  %q\n ожидалось %q", s, want)
	}
	if fmt.Sprintf("%v", c) != want || fmt.Sprint(&c) != want {
		t.Error("fmt-вывод должен использовать String()")
	}

	var buf bytes.Buffer
	slog.New(slog.NewJSONHandler(&buf, nil)).Info("config", "cfg", c)
	out := buf.String()
	for _, secret := range []string{"SuperSecret", "tok-123456"} {
		if strings.Contains(out, secret) {
			t.Errorf("секрет %q утёк в лог: %s", secret, out)
		}
	}
	if !strings.Contains(out, `"db_url":"postgres://app:xxxxx@db:5432/app"`) || !strings.Contains(out, `"env":"prod"`) {
		t.Errorf("лог должен содержать группу cfg с замаскированными полями: %s", out)
	}

	// Неразбираемый URL маскируется целиком.
	c.DBURL = "postgres://app:SuperSecret@db:port/app"
	if strings.Contains(c.String(), "SuperSecret") {
		t.Errorf("пароль из неразбираемого URL утёк: %s", c.String())
	}
	// Пустые секреты — пустая строка, а не ***.
	if s := Defaults().String(); !strings.Contains(s, "db_url= api_token= features=[]") {
		t.Errorf("пустые секреты не маскируются: %q", s)
	}
}

func TestEnabled(t *testing.T) {
	c := Config{Features: []string{"New-UI", "beta"}}
	if !c.Enabled("new-ui") || !c.Enabled("BETA") || c.Enabled("alpha") || (Config{}).Enabled("") {
		t.Error("Enabled работает неверно")
	}
}
