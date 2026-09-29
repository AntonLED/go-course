# Модуль 05. Веб-разработка на Go

## Цели

Большая часть Go-кода в продакшене так или иначе говорит по HTTP: принимает запросы, ходит в соседние сервисы, отдаёт страницы и статику. В этом модуле мы разберём, как делать это надёжно, опираясь в первую очередь на стандартную библиотеку.

К концу модуля вы научитесь поднимать HTTP-сервер «по-взрослому», с таймаутами, health-чеками и graceful shutdown. Вы спроектируете роутинг на `ServeMux` из Go 1.22+ и напишете собственные middleware: recover, request-id, логирование, auth и CORS. Вы будете строго разбирать запросы с учётом лимитов, JSON и форм и единообразно отвечать ошибками по RFC 9457. Вы безопасно отрендерите HTML через `html/template` и научитесь отдавать статику из `embed` с поддержкой ETag и 304.

На клиентской стороне вы напишете надёжный HTTP-клиент с таймаутами, пулом соединений, ретраями с backoff и RoundTripper-middleware. Наконец, вы разберётесь, что дают chi, gin, echo и fiber и когда стандартной библиотеки вполне достаточно. Всё это вы будете тестировать через `httptest`.

## Уроки

Уроки лучше проходить по порядку: каждый следующий опирается на идеи предыдущего, а последний пункт — итоговое задание модуля.

1. [Основы HTTP и запуск сервера в Go](lessons/01-http-server.md)
2. [Роутинг и middleware](lessons/02-routing-middleware.md)
3. [Работа с запросами и ответами](lessons/03-requests-responses.md)
4. [Шаблоны и статические файлы](lessons/04-templates-static.md)
5. [HTTP-клиент в Go](lessons/05-http-client.md)
6. [Популярные фреймворки для HTTP](lessons/06-frameworks.md)
7. Задание 5 — [09_project_todo](tasks/09_project_todo)

## Задачи

К каждому уроку привязаны одна-две задачи. В таблице указано, после какого урока за них браться.

| # | Задача | Урок | О чём |
|---|---|---|---|
| 01 | [graceful](tasks/01_graceful) | 1 | `http.Server` с таймаутами, `/healthz` + `/readyz`, `Serve` с graceful shutdown |
| 02 | [middleware](tasks/02_middleware) | 2 | `Chain`, `StatusRecorder` с `Unwrap`, `Recover`, `RequestID`, `Logging` в slog |
| 03 | [authcors](tasks/03_authcors) | 2 | BasicAuth с constant-time сравнением, CORS с preflight |
| 04 | [jsonapi](tasks/04_jsonapi) | 3 | строгий `DecodeJSON` (400/413/415/422), Problem Details, хендлер создания |
| 05 | [templates](tasks/05_templates) | 4 | `html/template`: layout/block, FuncMap, XSS, рендер в буфер |
| 06 | [static](tasks/06_static) | 4 | файловый сервер поверх `fs.FS`/`embed`: ETag, 304, Cache-Control, dot-файлы |
| 07 | [retry](tasks/07_retry) | 5 | клиент с ретраями, backoff, `Retry-After`, контекстом; RoundTripper-middleware |
| 08 | [miniweb](tasks/08_miniweb) | 6 | мини-фреймворк: группы, цепочки middleware, `Context`, ошибки из хендлеров |
| 09 | [project_todo](tasks/09_project_todo) | **Задание 5** | REST API задач на stdlib: CRUD, валидация, пагинация, middleware, problem+json |

## Как проверять

Каждая задача — отдельный пакет. Заготовка (файлы `*.go` с тегом `!solution`) компилируется, но тесты на ней падают. Пишите решение прямо в заготовке и запускайте тесты:

```
go test -race ./modules/05-web/tasks/01_graceful/
go test -race ./modules/05-web/...           # весь модуль
```

Эталонные решения лежат в `*_solution.go` под тегом `solution`. Заглядывайте в них только после собственной попытки, иначе задача потеряет смысл:

```
go test -race -tags solution ./modules/05-web/...
```

Закрепить материал поможет тест по модулю: [quiz.json](quiz.json) (39 вопросов).
