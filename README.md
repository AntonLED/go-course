# Курс подготовки по Go

Это курс для тех, кто хочет не просто писать на Go, а понимать, что происходит под капотом, и спокойно проходить технические собеседования. Он идёт по силлабусу из девяти модулей: от срезов и строк до планировщика, gRPC, TLS и сборки контейнеров. В нём 47 конспектов, 78 задач с автотестами и эталонными решениями и 337 вопросов для самопроверки.

Всё построено на стандартной библиотеке, поэтому скачивать ничего не придётся: достаточно Go 1.23 или новее (итераторы и range-over-func появились именно в 1.23).

## Как читать

Удобнее всего читать курс как книгу. Она собрана на VitePress: слева оглавление, есть поиск и тёмная тема, в конце каждого урока короткий тест, а на странице задачи можно посмотреть заготовку, тесты и спрятанное под спойлер эталонное решение.

```bash
cd go-course/book
npm install      # один раз, нужен Node 18+
npm run dev      # откроется http://localhost:5173, правки в modules/ подхватываются на лету
```

Если нужен статический сайт, `npm run build` положит его в `book/.vitepress/dist`, а `npm run preview` покажет. Прогресс по тестам и отмеченные задачи хранятся в вашем браузере.

## Как поделиться

Книгу можно выложить на GitHub Pages, чтобы она открывалась по ссылке вида `https://<ваш-логин>.github.io/go-course/`. Нужны git и GitHub CLI (`brew install gh`, затем `gh auth login`), после чего достаточно одной команды из корня курса:

```bash
./publish.sh            # создаст публичный репозиторий go-course, включит Pages и отправит код
```

Сайт собирается сам на каждый push в `main` (workflow лежит в `.github/workflows/pages.yml`), так что для обновления хватит повторного `./publish.sh` или обычного `git push`. Если хочется всё сделать руками: создайте репозиторий, отправьте в него код, а в настройках включите Settings → Pages → Source: GitHub Actions.

Учтите, что репозиторий публичный, и эталонные решения в нём тоже видны. На сайте они спрятаны под спойлер, но при желании их легко найти.

## Как решать задачи

Каждая задача лежит в своей папке. Вы пишете код в файле-заготовке: изначально там `panic("TODO")`, и все тесты красные. Рядом лежит эталон, но он отделён build-тегом и в обычную сборку не попадает.

```
modules/03-concurrency/tasks/08_errgroup/
  README.md               условие, сигнатуры, подсказки
  errgroup.go             ← заготовка, её и пишете (//go:build !solution)
  errgroup_solution.go    эталон (//go:build solution)
  errgroup_test.go        тесты
```

Проверить своё решение можно командой `go test ./modules/03-concurrency/tasks/08_errgroup/`, а эталон — той же командой с флагом `-tags solution`. Советую почти всегда добавлять `-race`: в модулях 03, 05, 07 и 08 многие тесты рассчитаны именно на детектор гонок. Последняя задача каждого модуля (`NN_project_*`) соответствует «Заданию N» из силлабуса и собирает материал модуля в небольшой проект.

Посмотреть общую картину помогает `cmd/check`:

```bash
go run ./cmd/check            # весь курс
go run ./cmd/check 3 5        # модули 03 и 05
go run ./cmd/check -v -race 3 # с детектором гонок и списком упавших тестов
go run ./cmd/check -solution  # убедиться, что эталоны зелёные (78/78)
```

Вопросы для самопроверки есть и вне книги: `go run ./cmd/quiz` поднимет их на http://localhost:8080, а после `go run ./cmd/quiz -build` файл `quiz/index.html` открывается просто двойным кликом. Там же есть экзамен из 40 случайных вопросов и работа над ошибками.

## Программа

| # | Модуль | Уроки | Задачи |
|---|---|---|---|
| 01 | [Введение в Go. Часть 1](modules/01-intro-part1/README.md) | [знакомство](modules/01-intro-part1/lessons/01-meet-go.md) · [синтаксис](modules/01-intro-part1/lessons/02-syntax.md) · [массивы и срезы](modules/01-intro-part1/lessons/03-slices.md) · [строки](modules/01-intro-part1/lessons/04-strings.md) · [map](modules/01-intro-part1/lessons/05-maps.md) · [указатели, структуры, методы](modules/01-intro-part1/lessons/06-structs-methods.md) | 7 · Задание 1: `07_project_library` |
| 02 | [Введение в Go. Часть 2](modules/02-intro-part2/README.md) | [интерфейсы](modules/02-intro-part2/lessons/01-interfaces.md) · [ошибки](modules/02-intro-part2/lessons/02-errors.md) · [пакеты и модули](modules/02-intro-part2/lessons/03-packages-modules.md) · [дженерики](modules/02-intro-part2/lessons/04-generics.md) · [итераторы](modules/02-intro-part2/lessons/05-iterators.md) | 9 · Задание 2: `09_project_repo` |
| 03 | [Параллельное программирование](modules/03-concurrency/README.md) | [модель PMG](modules/03-concurrency/lessons/01-intro-pmg.md) · [горутины](modules/03-concurrency/lessons/02-goroutines.md) · [синхронизация](modules/03-concurrency/lessons/03-sync.md) · [каналы и паттерны](modules/03-concurrency/lessons/04-channels-patterns.md) · [context](modules/03-concurrency/lessons/05-context.md) · [errgroup, singleflight](modules/03-concurrency/lessons/06-errgroup-singleflight.md) | 10 · Задание 3: `10_project_crawler` |
| 04 | [Работа с данными](modules/04-data/README.md) | [ввод/вывод](modules/04-data/lessons/01-io.md) · [командная строка](modules/04-data/lessons/02-cli.md) · [файлы](modules/04-data/lessons/03-files.md) · [JSON, YAML](modules/04-data/lessons/04-json-yaml.md) · [SQL](modules/04-data/lessons/05-sql.md) | 10 · Задание 4: `10_project_importer` |
| 05 | [Веб-разработка на Go](modules/05-web/README.md) | [HTTP-сервер](modules/05-web/lessons/01-http-server.md) · [роутинг и middleware](modules/05-web/lessons/02-routing-middleware.md) · [запросы и ответы](modules/05-web/lessons/03-requests-responses.md) · [шаблоны и статика](modules/05-web/lessons/04-templates-static.md) · [HTTP-клиент](modules/05-web/lessons/05-http-client.md) · [фреймворки](modules/05-web/lessons/06-frameworks.md) | 9 · Задание 5: `09_project_todo` |
| 06 | [Тестирование и отладка](modules/06-testing/README.md) | [тестирование](modules/06-testing/lessons/01-testing.md) · [моки и API](modules/06-testing/lessons/02-mocking.md) · [бенчмарки](modules/06-testing/lessons/03-benchmarks.md) · [профилирование](modules/06-testing/lessons/04-profiling.md) | 8 · Задание 6: `08_project_wallet` |
| 07 | [Основы микросервисов](modules/07-microservices/README.md) | [введение](modules/07-microservices/lessons/01-intro.md) · [JSON-RPC](modules/07-microservices/lessons/02-jsonrpc.md) · [gRPC + protobuf](modules/07-microservices/lessons/03-grpc-protobuf.md) | 6 · Задание 7: `06_project_orders` |
| 08 | [Вопросы безопасности](modules/08-security/README.md) | [TLS и сертификаты](modules/08-security/lessons/01-tls-certificates.md) · [HTTPS](modules/08-security/lessons/02-http-security.md) · [gRPC](modules/08-security/lessons/03-grpc-security.md) · [JWT](modules/08-security/lessons/04-jwt.md) · [OAuth 2.0](modules/08-security/lessons/05-oauth2.md) | 8 · Задание 8: `08_project_secureapi` |
| 09 | [Продвинутая разработка](modules/09-advanced/README.md) | [рефлексия](modules/09-advanced/lessons/01-reflection.md) · [DI](modules/09-advanced/lessons/02-di.md) · [конфигурации](modules/09-advanced/lessons/03-config.md) · [память](modules/09-advanced/lessons/04-memory.md) · [unsafe](modules/09-advanced/lessons/05-unsafe.md) · [логи, трейсы, метрики](modules/09-advanced/lessons/06-observability.md) · [Docker](modules/09-advanced/lessons/07-docker.md) | 11 · Задание 9: `11_project_service` |

## Про сторонние библиотеки

Курс сознательно обходится стандартной библиотекой. Там, где в реальном проекте вы бы взяли готовый пакет, урок показывает его настоящий API, а задача предлагает написать сам механизм. Так гораздо лучше видно, как всё работает внутри, и как раз об этом любят спрашивать на собеседованиях.

| Тема | В проде | В задаче курса |
|---|---|---|
| errgroup, singleflight | `golang.org/x/sync` | пишете свои `errgroup` и `singleflight` (03/08, 03/09) |
| SQL | pgx, lib/pq, sqlite | настоящий `database/sql` поверх учебного драйвера `modules/04-data/memsql` |
| YAML | `gopkg.in/yaml.v3` | парсер подмножества YAML (04/08) |
| gin, chi, echo | фреймворк | мини-фреймворк на `ServeMux` 1.22 (05/08) |
| gRPC, protobuf | `google.golang.org/grpc`, protoc | wire-формат protobuf и gRPC-фрейминг с мультиплексированием (07/04, 07/05) |
| JWT, OAuth2 | golang-jwt, x/oauth2 | HS256/RS256/ES256, PKCE и authorization server руками (08/05–07) |
| DI, конфиг, метрики | fx/wire, viper, prometheus | reflect-контейнер, envconfig, реестр метрик в Prometheus-формате (09) |

## В каком темпе проходить

Мне кажется разумным темп около недели на модуль: днём читать конспект, вечером решать задачи, а в конце пройти тест модуля и сделать финальное задание. Если база уже есть, первые два модуля можно пролистать быстрее, а вот над параллельным программированием и безопасностью лучше не торопиться.

Если захотите дописать курс: структура описана в [CONVENTIONS.md](CONVENTIONS.md), правила текста в [STYLE.md](STYLE.md), а схем в [DIAGRAMS.md](DIAGRAMS.md).
