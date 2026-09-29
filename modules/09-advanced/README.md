# Модуль 9. Продвинутая разработка в Go

Код, который проходит тесты, и сервис, который спокойно живёт в продакшене, — это разные вещи. Этот модуль о том, что лежит между ними. Мы начнём с рефлексии и построим на ней инфраструктуру, которую обычно берут из библиотек: валидатор, загрузчик конфигурации, DI-контейнер. Затем разберёмся, как Go управляет памятью и когда оправдан `unsafe`. Закончим наблюдаемостью (логи, метрики, трейсы) и упаковкой сервиса в контейнер.

## Что вы будете уметь

Пройдя модуль, вы сможете осознанно использовать `reflect`: обходить структуры и теги, вызывать функции, понимать, во что это обходится, и выбирать альтернативы вроде дженериков и кодогенерации. Вы научитесь проектировать приложение с явными зависимостями (constructor injection, composition root, functional options) и поймёте, как устроены wire и fx изнутри.

Вы соберёте слоистую конфигурацию, где значения по умолчанию перекрываются файлом, файл — переменными окружения, а те — флагами, с валидацией и маскированием секретов. Научитесь читать вывод escape analysis, настраивать GC через `GOGC` и `GOMEMLIMIT`, писать код без лишних аллокаций и находить утечки. Узнаете, как корректно применять `unsafe`: шесть допустимых шаблонов, zero-copy конверсии и проверку checkptr.

В части про наблюдаемость вы напишете свой `slog.Handler`, метрики в формате Prometheus и пропагацию W3C Trace Context. А в конце соберёте минимальный безопасный образ и научите сервис корректно завершаться по SIGTERM.

## Уроки

1. [Рефлексия](lessons/01-reflection.md)
2. [Внедрение зависимостей (DI)](lessons/02-di.md)
3. [Управление конфигурациями и средами](lessons/03-config.md)
4. [Управление памятью и аллокациями](lessons/04-memory.md)
5. [unsafe](lessons/05-unsafe.md)
6. [Логгирование, трейсинг, метрики](lessons/06-observability.md)
7. [Сборка Docker-контейнера](lessons/07-docker.md)

## Задачи

| # | Задача | Урок | Суть |
|---|---|---|---|
| 01 | [validate](tasks/01_validate) | Рефлексия | валидатор структур по тегам `validate:"required,min=3,email"`, вложенные структуры и срезы |
| 02 | [di](tasks/02_di) | DI | мини-dig: Provide/Invoke на reflect, синглтоны, `As`, циклы, Lifecycle OnStart/OnStop |
| 03 | [envconfig](tasks/03_envconfig) | Конфигурации | заполнение структуры из env по тегам `env/default/required/sep/prefix` |
| 04 | [layered](tasks/04_layered) | Конфигурации | defaults < JSON < env < flags, валидация, маскирование секретов, feature flags |
| 05 | [memory](tasks/05_memory) | Память | порядок полей, zero-alloc, `sync.Pool`, утечки через срезы |
| 06 | [unsafe](tasks/06_unsafe) | unsafe | zero-copy `[]byte`↔`string`, заголовок среза, `Offsetof`, `Reinterpret` |
| 07 | [slog](tasks/07_slog) | Наблюдаемость | JSON `slog.Handler` с маскированием секретов, проверка `slogtest` |
| 08 | [metrics](tasks/08_metrics) | Наблюдаемость | Counter/Gauge/Histogram с лейблами, Prometheus text format |
| 09 | [tracing](tasks/09_tracing) | Наблюдаемость | W3C traceparent, спаны, HTTP-пропагация, дерево спанов |
| 10 | [docker](tasks/10_docker) | Docker | Dockerfile под линтер + build info, health, graceful shutdown, GOMAXPROCS |
| 11 | [project_service](tasks/11_project_service) | **Задание 9** | KV-сервис: config + DI + slog с request-id + /metrics + трейсинг + graceful shutdown + Dockerfile |

Задачи идут в том же порядке, что и уроки, и последняя, `project_service`, собирает всё вместе: это небольшой KV-сервис, в котором встречаются конфигурация, DI, логи с request-id, метрики, трейсинг, graceful shutdown и Dockerfile. Её стоит делать в самом конце.

Внешние библиотеки (uber/fx, wire, viper, zap, OpenTelemetry, Prometheus client) в уроках показаны через их настоящий API, чтобы вы узнали их в чужом коде. В задачах же соответствующие механизмы реализуются на стандартной библиотеке: так лучше видно, как они устроены.

## Как проверять

Тесты запускаются из корня репозитория:

```
go test ./modules/09-advanced/tasks/01_validate/          # одна задача
go test -race ./modules/09-advanced/...                   # всё (unsafe-задачи — обязательно с -race: checkptr)
go vet ./modules/09-advanced/...
```

Эталонные решения лежат рядом, в файлах `*_solution.go` (для Docker — `Dockerfile.solution`), и подключаются тегом сборки: `go test -tags solution ./modules/09-advanced/...`. Заглядывайте в них только после собственной попытки, иначе задача потеряет смысл.

Проверить понимание теории поможет тест `quiz.json` из 37 вопросов.
