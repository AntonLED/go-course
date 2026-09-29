# 10. Production-ready Docker-образ + версия, health, graceful shutdown

Урок: [Сборка Docker-контейнера](../../lessons/07-docker.md)

Собрать Go-сервис в Docker-образ можно за пять строк, и образ даже будет работать. Проблемы вылезут потом: образ весит гигабайт, запускается от root, не реагирует на SIGTERM и при каждом деплое обрывает запросы, а на вопрос «какая версия сейчас в проде?» ответить нечем. Задача состоит из двух частей. В первой вы исправите **Dockerfile** (его проверяет тест-линтер, написанный на Go), во второй напишете **Go-код**, который делает бинарник удобным для жизни в контейнере. Точка входа уже готова: [`cmd/server/main.go`](cmd/server/main.go).

## Часть 1. Dockerfile

Сейчас в файле `Dockerfile` в этой директории лежит анти-пример — исправьте его. Рядом создайте `Dockerfile.dockerignore`: BuildKit ищет файл `<имя Dockerfile>.dockerignore` рядом с самим Dockerfile. Контекст сборки — корень репозитория, где лежит `go.mod`:

```
docker build -f modules/09-advanced/tasks/10_docker/Dockerfile \
  --build-arg VERSION=1.2.3 --build-arg COMMIT=$(git rev-parse --short HEAD) -t course-server .
docker run --rm -p 8080:8080 course-server
curl localhost:8080/version
docker image ls course-server      # цель: < 20 МБ
```

Линтер (`dockerfile_test.go`) проверяет следующее:

1. Сборка многостадийная (multi-stage): в файле не меньше двух `FROM`.
2. У каждого образа фиксированный тег (не `latest` и не пустой) или digest. `ARG`, объявленные до первого `FROM`, подставляются.
3. В стадии сборки `go build` запускается с `CGO_ENABLED=0`, `-trimpath` и `-ldflags`, где есть `-s -w` и `-X main.version=...`.
4. Используется `RUN --mount=type=cache,...` — кэш модулей и сборки BuildKit.
5. Финальный образ — `scratch` или `gcr.io/distroless/*`.
6. `USER` задан и это не root (не `root` и не `0`).
7. Есть `EXPOSE`.
8. В финальной стадии используется только `COPY --from=...`, никакого `COPY . .`.
9. `ENTRYPOINT` или `CMD` записаны в exec-форме (`["/server"]`). В shell-форме PID 1 будет `/bin/sh`, и SIGTERM не дойдёт до приложения.
10. `HEALTHCHECK` тоже в exec-форме. В distroless нет ни shell, ни curl, поэтому бинарник проверяет себя сам: `CMD ["/server", "-healthcheck"]`.
11. `Dockerfile.dockerignore` исключает `.git`.

## Часть 2. Go-код (`dockerapp.go`)
```go
func ReadInfo(version, commit string) Info
func NewHandler(info Info, ready func() bool) http.Handler
func Run(ctx context.Context, ln net.Listener, h http.Handler, shutdownTimeout time.Duration) error
func Probe(ctx context.Context, url string) error
func MaxProcsFromCgroup(cpuMax string) (int, error)
```

### ReadInfo

`version` и `commit` приходят из `-ldflags -X` и могут оказаться пустыми. Из `debug.ReadBuildInfo()` берутся `GoVersion` и `Module` (это `Main.Path`). Если `version` пуст, используется `Main.Version` (кроме значения `"(devel)"`), а если и его нет — `"dev"`. Если пуст `commit`, используется настройка `vcs.revision`, а без неё — `"unknown"`. Поле `Dirty` истинно, когда `vcs.modified == "true"`.

### NewHandler

- `GET /healthz` всегда отвечает 200. Это liveness-проба, и внешние зависимости она не проверяет: если база упала, перезапуск нашего контейнера её не починит.
- `GET /readyz` отвечает 200 или 503, если `ready()` вернул false. При `ready == nil` сервис всегда готов.
- `GET /version` отдаёт `Info` в JSON с `Content-Type: application/json`.
- Другие методы получают 405, неизвестные пути — 404. Паттерны `ServeMux` из Go 1.22 делают это сами.

### Run

`Run` создаёт `http.Server` с `ReadHeaderTimeout` и запускает `Serve(ln)` в горутине. Если `Serve` упал сам, `Run` возвращает его ошибку.

При отмене `ctx` вызывается `Shutdown` с таймаутом `shutdownTimeout`. Контекст для него нужен новый — исходный уже отменён, и `Shutdown` с ним завершился бы мгновенно. Активные запросы дорабатывают, новые подключения не принимаются. При успехе `Run` возвращает `nil`, а по таймауту — ошибку, оборачивающую `context.DeadlineExceeded`, закрыв оставшиеся соединения через `Close()`.

### Probe

`Probe` делает GET-запрос и возвращает ошибку, если запрос не удался или статус отличается от 200. Контекст `ctx` при этом уважается.

### MaxProcsFromCgroup

Функция разбирает файл `cpu.max` из cgroup v2 и возвращает, сколько процессоров выделено контейнеру. Если квота задана числом, результат считается так:

$$
\text{procs} = \max\left(1, \left\lfloor \frac{\text{quota}}{\text{period}} \right\rfloor\right)
$$

Примеры:

- `"200000 100000"` → 2;
- `"150000 100000"` → 1 (округление вниз);
- `"max 100000"` → 0, что означает «лимита нет»;
- период по умолчанию — 100000;
- минимум — 1;
- мусор на входе → ошибка.

Так работает `uber-go/automaxprocs`, а начиная с Go 1.25 рантайм делает это сам.

## Проверка
```
go test ./modules/09-advanced/tasks/10_docker/
```
