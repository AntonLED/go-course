# Сборка Docker-контейнера

Go словно создан для контейнеров. Результат сборки — один статический бинарник, которому не нужны ни рантайм, ни внешние библиотеки. Хороший образ Go-сервиса весит 5–20 МБ, благодаря кэшу собирается за секунды и не содержит ничего, кроме самого бинарника и, возможно, CA-сертификатов и tzdata. Посмотрим, как к этому прийти и о каких деталях не забыть по дороге.

## Multi-stage build

Для компиляции нужен тулчейн Go, а для запуска нет. Поэтому Dockerfile делят на стадии: в первой собираем, во вторую копируем только результат.

```dockerfile
# syntax=docker/dockerfile:1
# фиксированный тег, не latest
FROM golang:1.23.4-alpine AS build
WORKDIR /src
# сначала только зависимости → слой с go mod download кэшируется, пока go.mod/go.sum не меняются
# (комментарий в Dockerfile — только целая строка, начинающаяся с #; «хвостовых» комментариев нет)
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/app ./cmd/app

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app"]
```

Стадия сборки с тулчейном весит 300 с лишним мегабайт, и в итоговый образ она не попадает. Обратите внимание на порядок инструкций: сначала копируются только `go.mod` и `go.sum`, и слой с `go mod download` пересобирается лишь при изменении зависимостей, а не при каждой правке кода.

![Из стадии `build` в финальный distroless-образ копируется только бинарь; тулчейн, кэш модулей и исходники остаются в промежуточном образе](img/docker-multistage.svg)

## Флаги сборки

Каждый флаг в команде `go build` выше стоит там не случайно:

| Флаг | Зачем |
|---|---|
| `CGO_ENABLED=0` | чистый Go, статическая линковка — бинарнику не нужна libc (иначе в scratch/distroless-static он не запустится: `no such file or directory` от динамического загрузчика). Пакеты `net` и `os/user` переходят на Go-реализации |
| `-trimpath` | убрать абсолютные пути (`/home/alice/src/...`) из бинарника: воспроизводимость, меньше утечек |
| `-ldflags "-s -w"` | `-s` — без таблицы символов, `-w` — без DWARF. Минус 25–35% размера; стектрейсы паник остаются (pclntab не трогается), но отладчик не поможет |
| `-ldflags "-X main.version=1.2.3"` | задать значение строковой **переменной** (не константы) пакета при линковке: `-X 'path/to/pkg.Var=value'` |
| `GOOS/GOARCH` | кросс-компиляция; с BuildKit: `FROM --platform=$BUILDPLATFORM golang...` + `GOARCH=$TARGETARCH` — быстрые multi-arch образы без эмуляции |

Флаг `-X` — не единственный способ узнать версию. Go сам вшивает в бинарник информацию о сборке, и её можно прочитать из программы:

```go
bi, ok := debug.ReadBuildInfo()   // runtime/debug
bi.GoVersion, bi.Main.Path, bi.Main.Version // "(devel)" при go build из исходников (до Go 1.24; с 1.24 — версия из VCS-тега или псевдоверсия)
for _, s := range bi.Settings { /* vcs.revision, vcs.time, vcs.modified, -ldflags, CGO_ENABLED... */ }
```

Снаружи то же самое покажет `go version -m ./app`. VCS-информация попадает в бинарник, если сборка идёт внутри git-репозитория (режим `-buildvcs=auto`). В Docker каталог `.git` обычно исключён из контекста, поэтому хэш коммита передают через `--build-arg`.

## Базовые образы

Какой образ взять для финальной стадии? Вариантов немного:

| Образ | Размер | Что внутри | Когда |
|---|---|---|---|
| `scratch` | 0 | ничего | статический бинарник; CA-сертификаты, tzdata (или `import _ "time/tzdata"`) и `/etc/passwd` для non-root копировать самому |
| `gcr.io/distroless/static-debian12` | ~2 МБ | CA, tzdata, `/etc/passwd` с `nonroot` (65532), без shell | лучший дефолт для Go |
| `gcr.io/distroless/base` | ~20 МБ | + glibc | если нужен CGO |
| `alpine` | ~7 МБ | musl, busybox, apk | нужен shell для отладки; учтите, что musl — не glibc |

У минимальных образов есть очевидное неудобство: без shell не получится сделать `docker exec -it ... sh`. Отлаживать такие контейнеры можно через `kubectl debug` с эфемерным контейнером или через вариант образа `:debug`, который есть у distroless.

И фиксируйте версии: `golang:1.23.4-alpine3.20`, а ещё лучше digest `@sha256:...`. Тег `latest` делает сборку невоспроизводимой: завтра под тем же именем окажется другой образ.

## Безопасность

Запускайте процесс **не от root**: `USER nonroot:nonroot` или `USER 65532:65532`. Может возникнуть вопрос, как тогда слушать порт 80. Классически порты ниже 1024 требуют root или capability `CAP_NET_BIND_SERVICE`. Docker начиная с версии 20.10 снимает это ограничение через sysctl `net.ipv4.ip_unprivileged_port_start=0`, но полагаться на это не стоит; проще слушать 8080.

Секретов не должно быть ни в `ENV`, ни в `ARG`, ни в слоях образа: `docker history` покажет их любому. Если секрет нужен во время сборки, например для доступа к приватным модулям, используйте `RUN --mount=type=secret,id=netrc`.

Минимальный образ хорош не только размером: чем меньше в нём лишнего, тем меньше поверхность атаки и тем меньше CVE найдёт сканер. А в Kubernetes можно включить `readOnlyRootFilesystem: true`, ведь Go-сервису обычно не нужно писать на диск.

## Кэш и `.dockerignore`

Docker пересобирает слой и все слои после него, как только меняются входные данные. Поэтому то, что меняется редко (`go.mod`, `go.sum`, скачивание модулей), ставьте выше, а то, что меняется часто (исходники), ниже.

Но даже инвалидированный слой не обязан собираться с нуля. Инструкции BuildKit `RUN --mount=type=cache,target=/go/pkg/mod` и `target=/root/.cache/go-build` сохраняют кэш модулей и компиляции между сборками.

Файл `.dockerignore` с `.git`, `bin/`, `*.md` и локальными `.env` уменьшает контекст сборки, ускоряет её и защищает от утечек. BuildKit также ищет файл `<Dockerfile>.dockerignore` рядом с конкретным Dockerfile. И никогда не пишите `COPY . .` в финальной стадии: в образ уедут исходники, а заодно и секреты из рабочей директории.

![Правка кода пересобирает только слои начиная с `COPY . .`; слой с `go mod download` пересобирается лишь при изменении `go.mod`/`go.sum`](img/docker-layers-cache.svg)

## HEALTHCHECK

```dockerfile
HEALTHCHECK --interval=10s --timeout=3s --retries=3 CMD ["/app", "-healthcheck"]
```

В distroless нет `curl` и `wget`, поэтому проверку делает сам бинарник: с флагом `-healthcheck` он выполняет `GET /healthz` и возвращает код выхода. Kubernetes инструкцию `HEALTHCHECK` игнорирует, там проверки описываются в манифесте как `livenessProbe` и `readinessProbe`, и это две разные вещи. **Liveness** отвечает на вопрос, жив ли процесс. Не проверяйте в ней базу данных: если база упадёт, Kubernetes перезапустит все поды разом, и это ничем не поможет. **Readiness** отвечает на вопрос, готов ли под принимать трафик; при остановке она начинает возвращать 503.

## Сигналы и PID 1

Как контейнер останавливается? `docker stop` и Kubernetes посылают процессу **SIGTERM**, ждут grace period (в Docker 10 секунд, в Kubernetes 30) и затем посылают **SIGKILL**. Если за это время сервис успеет доработать активные запросы и закрыться, остановка пройдёт незаметно для пользователей. Но для этого сигнал должен дойти до приложения.

Здесь и кроется ловушка с формой инструкции. **Shell-форма** `CMD /app` запускает `/bin/sh -c /app`, и PID 1 в контейнере становится shell. Заменит ли он себя приложением через exec, зависит от конкретного shell и команды. Если не заменит, сигналы до приложения не дойдут, и оно будет убито по SIGKILL без graceful shutdown. В distroless и scratch shell нет вовсе, так что shell-форма там просто не запустится. Используйте **exec-форму** `ENTRYPOINT ["/app"]`.

У PID 1 в Linux есть ещё одна особенность: он не получает сигналы с действием по умолчанию. Если приложение не установило обработчик SIGTERM, сигнал будет проигнорирован. Go-рантайм обработчики ставит, а `signal.NotifyContext` делает остановку явной. Кроме того, PID 1 должен «пожинать» зомби-процессы, поэтому если сервис запускает дочерние процессы, используйте `docker run --init` (tini).

Вот как выглядит корректная остановка:

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
go srv.Serve(ln)
<-ctx.Done()
ready.Store(false)             // readiness → 503
time.Sleep(drainDelay)         // балансировщик убирает под из ротации
shCtx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
defer cancel()
srv.Shutdown(shCtx)            // закрыть listener, дождаться активных запросов
```

Пауза `drainDelay` нужна, чтобы балансировщик успел увидеть, что под больше не ready, и перестал слать ему запросы. Всё вместе должно уложиться в grace period:

$$
t_\text{drain} + t_\text{shutdown} < T_\text{grace}
$$

![После SIGTERM под сразу перестаёт быть ready, ждёт `drainDelay` (здесь 5 с для примера), затем `Shutdown` дожидается активных запросов. Сигнал дойдёт до Go, только если приложение — PID 1 (exec-форма)](img/docker-shutdown.svg)

## GOMAXPROCS и память в контейнере

До Go 1.25 `GOMAXPROCS` по умолчанию равнялся числу CPU **хоста**, а не CPU-лимиту cgroup. Под с `limits.cpu: 2` на 64-ядерной ноде запускал 64 P, и они конкурировали за квоту в два ядра, что приводило к троттлингу CFS и росту латентности. Стандартное решение — [`go.uber.org/automaxprocs`](https://github.com/uber-go/automaxprocs), подключаемый одной строкой `import _ "go.uber.org/automaxprocs"`. Библиотека читает `cpu.max` (cgroup v2) или `cpu.cfs_quota_us` (v1) и выставляет

$$
\mathrm{GOMAXPROCS} = \max\left(1, \left\lfloor \frac{\text{quota}}{\text{period}} \right\rfloor\right)
$$

Начиная с Go 1.25 рантайм учитывает лимит cgroup сам.

С памятью похожая история, но без автоматического решения в рантайме: он не знает про лимит контейнера. Выставьте `GOMEMLIMIT` примерно в 90% лимита, иначе GC не успеет среагировать и придёт OOMKiller. Библиотека `KimMachineGun/automemlimit` делает это автоматически.

## Цель: образ меньше 20 МБ

Типичный итог — distroless/static (около 2 МБ) плюс бинарник, собранный с `-s -w` (5–15 МБ). Проверить размер можно через `docker image ls`, а разобрать образ по слоям — утилитой `dive`. Дальнейшее сжатие вроде UPX обычно того не стоит: сжатый бинарник медленнее стартует и вызывает ложные срабатывания антивирусов.

## Вопросы с собеседований

1. Зачем нужен multi-stage build? Почему бы не собрать в `golang:latest` и не запустить там же?
2. Что даёт `CGO_ENABLED=0` и почему без него бинарник не запускается в scratch?
3. Чем shell-форма `CMD` отличается от exec-формы и как это связано с graceful shutdown?
4. Как вшить версию в бинарник? Что вернёт `debug.ReadBuildInfo`?
5. Почему GOMAXPROCS в контейнере может оказаться неправильным и как это исправить?
6. Чем liveness-проба отличается от readiness?

## Ссылки

- https://docs.docker.com/build/building/multi-stage/ , https://docs.docker.com/build/cache/optimize/
- https://github.com/GoogleContainerTools/distroless
- https://pkg.go.dev/runtime/debug#ReadBuildInfo , https://pkg.go.dev/cmd/link
- https://go.dev/doc/go1.25#runtime (container-aware GOMAXPROCS)

## Практика: задачи [10_docker](../tasks/10_docker), [11_project_service](../tasks/11_project_service)
