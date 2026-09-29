package envconfig

import (
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) LookupFunc {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

type DBConfig struct {
	Host string `env:"HOST" default:"localhost"`
	Port uint16 `env:"PORT" default:"5432"`
	Pass string `env:"PASSWORD" required:"true"`
}

type Config struct {
	Port     int           `env:"PORT" default:"8080"`
	Debug    bool          `env:"DEBUG"`
	Timeout  time.Duration `env:"TIMEOUT" default:"5s"`
	Hosts    []string      `env:"HOSTS" default:"a, b"`
	Ports    []int         `env:"PORTS" sep:";"`
	Ratio    float64       `env:"RATIO" default:"0.5"`
	Level    slog.Level    `env:"LOG_LEVEL" default:"INFO"`
	MaxConns *int          `env:"MAX_CONNS"`
	Name     string        `env:"NAME"`
	DB       DBConfig      `prefix:"DB_"`
	NoTag    string
	private  string `env:"PRIVATE"`
}

func TestLoadDefaultsAndValues(t *testing.T) {
	var c Config
	err := Load(&c, env(map[string]string{
		"DEBUG":       "true",
		"PORTS":       "1; 2;3",
		"LOG_LEVEL":   "warn",
		"MAX_CONNS":   "7",
		"NAME":        "",
		"DB_PASSWORD": "s3cret",
		"DB_PORT":     "6432",
		"PRIVATE":     "x",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	checks := []struct {
		name      string
		got, want any
	}{
		{"Port (default)", c.Port, 8080},
		{"Debug", c.Debug, true},
		{"Timeout (default)", c.Timeout, 5 * time.Second},
		{"Hosts (default, trim)", c.Hosts, []string{"a", "b"}},
		{"Ports (sep=;)", c.Ports, []int{1, 2, 3}},
		{"Ratio", c.Ratio, 0.5},
		{"Level (TextUnmarshaler)", c.Level, slog.LevelWarn},
		{"Name (пустая, но заданная)", c.Name, ""},
		{"DB.Host (default)", c.DB.Host, "localhost"},
		{"DB.Port (prefix)", c.DB.Port, uint16(6432)},
		{"DB.Pass", c.DB.Pass, "s3cret"},
		{"private не трогаем", c.private, ""},
	}
	for _, ch := range checks {
		if !reflect.DeepEqual(ch.got, ch.want) {
			t.Errorf("%s = %#v, ожидалось %#v", ch.name, ch.got, ch.want)
		}
	}
	if c.MaxConns == nil || *c.MaxConns != 7 {
		t.Errorf("MaxConns = %v, ожидался указатель на 7", c.MaxConns)
	}
}

func TestEmptyIsNotUnset(t *testing.T) {
	type C struct {
		Host  string   `env:"HOST" default:"localhost"`
		Hosts []string `env:"HOSTS" default:"a,b"`
	}
	var c C
	if err := Load(&c, env(map[string]string{"HOST": "", "HOSTS": ""})); err != nil {
		t.Fatal(err)
	}
	if c.Host != "" {
		t.Errorf("HOST задана пустой — default не должен применяться, получено %q", c.Host)
	}
	if c.Hosts == nil || len(c.Hosts) != 0 {
		t.Errorf("HOSTS=\"\" должно дать пустой (не nil) срез, получено %#v", c.Hosts)
	}
}

func TestKeepExistingWhenUnset(t *testing.T) {
	type C struct {
		Name string `env:"NAME"`
	}
	c := C{Name: "preset"}
	if err := Load(&c, env(nil)); err != nil {
		t.Fatal(err)
	}
	if c.Name != "preset" {
		t.Errorf("без переменной и default значение должно сохраниться, получено %q", c.Name)
	}
}

func TestErrorsCollected(t *testing.T) {
	type C struct {
		Small int8          `env:"SMALL"`
		Flag  bool          `env:"FLAG"`
		D     time.Duration `env:"D"`
		U     uint          `env:"U"`
		Ch    chan int      `env:"CH"`
		Pass  string        `env:"PASS" required:"true"`
		Token string        `env:"TOKEN" required:"true"`
		OK    int           `env:"OK"`
	}
	var c C
	err := Load(&c, env(map[string]string{
		"SMALL": "300", // переполнение int8
		"FLAG":  "yes",
		"D":     "10", // нет единиц измерения
		"U":     "-1",
		"CH":    "1",
		"TOKEN": "", // задана, но пустая — required не выполнен
		"OK":    "42",
	}))
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	for _, f := range []string{"Small", "Flag", "D", "U", "Ch", "Pass", "Token"} {
		if !strings.Contains(err.Error(), "поле "+f+" ") {
			t.Errorf("ошибка должна упоминать поле %s: %v", f, err)
		}
	}
	if !errors.Is(err, ErrRequired) {
		t.Error("errors.Is(err, ErrRequired) должно быть true")
	}
	if !errors.Is(err, ErrUnsupported) {
		t.Error("errors.Is(err, ErrUnsupported) должно быть true (chan int)")
	}
	var fe *FieldError
	if !errors.As(err, &fe) {
		t.Error("errors.As(err, *FieldError) должно быть true")
	}
	if c.OK != 42 {
		t.Errorf("корректные поля заполняются несмотря на ошибки в других: OK = %d", c.OK)
	}
}

func TestRequiredWithDefault(t *testing.T) {
	type C struct {
		A string `env:"A" required:"true" default:"x"`
	}
	var c C
	if err := Load(&c, env(nil)); err != nil || c.A != "x" {
		t.Errorf("required+default: err=%v A=%q", err, c.A)
	}
}

func TestFieldErrorPath(t *testing.T) {
	var c Config
	err := Load(&c, env(map[string]string{"DB_PORT": "70000", "DB_PASSWORD": "x"}))
	var fe *FieldError
	if !errors.As(err, &fe) {
		t.Fatalf("ожидалась *FieldError, получено %v", err)
	}
	if fe.Field != "DB.Port" || fe.Key != "DB_PORT" {
		t.Errorf("FieldError{Field:%q Key:%q}, ожидалось DB.Port / DB_PORT", fe.Field, fe.Key)
	}
}

func TestInvalidTarget(t *testing.T) {
	var c Config
	var nilPtr *Config
	x := 5
	for _, dst := range []any{nil, c, nilPtr, &x} {
		if err := Load(dst, env(nil)); !errors.Is(err, ErrInvalidTarget) {
			t.Errorf("Load(%T) = %v, ожидалось ErrInvalidTarget", dst, err)
		}
	}
}
