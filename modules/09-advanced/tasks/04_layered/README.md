# 04. Слоистая конфигурация: defaults < file < env < flags

Урок: [Управление конфигурациями и средами](../../lessons/03-config.md)

У реального сервиса конфигурация приходит сразу из нескольких мест: разумные значения по умолчанию зашиты в код, общие настройки лежат в файле, окружение переопределяет их для конкретного стенда, а флаг командной строки — для одного запуска. Нужно аккуратно наложить эти слои друг на друга, проверить результат и при этом не засветить пароль от базы в логах. Именно это вы и сделаете в пакете `appconfig`. Тип `Config`, `Defaults()`, `Source` и `ErrInvalid` уже есть в `types.go`.

```go
func Load(src Source) (Config, error)
func (c Config) Validate() error
func (c Config) Enabled(feature string) bool
func (c Config) String() string          // секреты замаскированы
func (c Config) LogValue() slog.Value    // секреты замаскированы и в логах
```

В `main` это выглядит так:
```go
cfg, err := appconfig.Load(appconfig.Source{Args: os.Args[1:], LookupEnv: os.LookupEnv, ReadFile: os.ReadFile})
```

## Слои

Каждый следующий слой перекрывает предыдущий.

1. **Defaults()** — значения по умолчанию.
2. **JSON-файл.** Путь к нему берётся из флага `-config`, если тот задан явно, иначе из `APP_CONFIG`; если пути нет, слой пропускается. Допустимые ключи: `env, http_addr, log_level, timeout ("5s"), db_url, api_token, features ([]string)`. Перекрываются только те ключи, что присутствуют в файле. Неизвестный ключ — ошибка (`DisallowUnknownFields`): опечатка в конфиге не должна молча игнорироваться. Ошибка чтения файла оборачивается через `%w`.
3. **Окружение:** `APP_ENV, APP_HTTP_ADDR, APP_LOG_LEVEL, APP_TIMEOUT, APP_DB_URL, APP_API_TOKEN, APP_FEATURES`. `APP_FEATURES` разбивается по запятой, элементы обрезаются `TrimSpace`, пустые отбрасываются. Переменная, заданная пустой, тоже перекрывает значение.
4. **Флаги:** `-env -http-addr -log-level -timeout -db-url -api-token -features -config`. Перекрывают только флаги, заданные явно (их перечисляет `flag.FlagSet.Visit`), в том числе явно пустые вроде `-api-token=`. Заведите свой `FlagSet` с `ContinueOnError` и `SetOutput(io.Discard)`: неизвестный флаг должен давать ошибку, а не `os.Exit`.

## Валидация (`Validate`)

`Validate` собирает все нарушения через `errors.Join`. Каждое из них оборачивает `ErrInvalid` и упоминает имя ключа: `env`, `log_level`, `http_addr`, `timeout`, `db_url`, `api_token`, а для запрета debug в prod — слово `debug`. Проверяется следующее:

- `env` — одно из `dev`, `stage`, `prod`; `log_level` — одно из `debug`, `info`, `warn`, `error`;
- `http_addr` имеет вид `host:port` с портом от 1 до 65535; `timeout > 0`;
- непустой `db_url` — это URL со схемой и хостом. Сам URL в текст ошибки не включайте: в нём пароль;
- в `prod` обязательны `db_url` и `api_token`, а `log_level=debug` запрещён.

`Load` возвращает ошибку валидации как есть.

## Маскирование секретов

`String()` печатает конфигурацию в одну строку, заменяя секреты:
```
env=prod http_addr=:8080 log_level=info timeout=5s db_url=postgres://app:xxxxx@db:5432/app api_token=*** features=[new-ui,beta]
```

- `db_url` маскируется через `url.URL.Redacted()`, а если URL не разбирается — заменяется на `***` целиком;
- `api_token` превращается в `***`; пустые секреты выводятся пустой строкой;
- `LogValue()` возвращает группу с теми же ключами, где `timeout` — это `slog.Duration`, а `features` — строка через запятую.

`Enabled` сравнивает имена фич без учёта регистра.

## Проверка
```
go test ./modules/09-advanced/tasks/04_layered/
```
