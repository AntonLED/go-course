# Модуль 02. Введение в Go. Часть 2

В первой части вы научились писать на Go простые программы. Эта часть о том, как из них вырастают настоящие проекты. Мы разберём, как в Go строятся абстракции (интерфейсы и дженерики), как язык обходится без исключений, как устроены пакеты и модули и как работают итераторы, появившиеся в Go 1.23. Почти каждая тема здесь — постоянный гость на собеседованиях, поэтому помимо «как писать» мы много говорим о том, «как это устроено внутри».

## Цели

К концу модуля вы будете понимать, из чего состоит интерфейсное значение (itab, пара type и data), и перестанете попадаться в ловушку «nil-интерфейс или nil-указатель». Вы научитесь проектировать маленькие интерфейсы на стороне потребителя и реализовывать `io.Reader` и `io.Writer` строго по контракту.

В обработке ошибок вы освоите обёртки с контекстом, `errors.Is`, `errors.As` и `errors.Join`, собственные типы ошибок, а заодно поймёте, когда уместны `panic` и `recover`.

Из урока про модули вы вынесете понимание `go.mod` и `go.sum`, SemVer и суффикса `/vN`, алгоритма MVS, директории `internal/`, workspaces и переменных GOPROXY и GOPRIVATE.

Наконец, вы будете уверенно писать дженерики с ограничениями (`~T`, `comparable`, `cmp.Ordered`), понимать, когда они не нужны, и создавать push- и pull-итераторы (`iter.Seq`, `iter.Pull`), корректно обрабатывающие ранний выход.

## Уроки

Уроки лучше проходить по порядку: итераторы опираются на дженерики, а итоговое задание собирает всё вместе.

1. [Интерфейсы](lessons/01-interfaces.md)
2. [Обработка ошибок в Go](lessons/02-errors.md)
3. [Управление пакетами и модулями](lessons/03-packages-modules.md)
4. [Дженерики](lessons/04-generics.md)
5. [Итераторы](lessons/05-iterators.md)
6. Задание 2 — [09_project_repo](tasks/09_project_repo)

## Задачи

К каждому уроку есть практические задачи. Последняя, `09_project_repo`, — это Задание 2 модуля: в ней понадобится почти всё, что вы изучили.

| Директория | Урок | Суть |
|---|---|---|
| [01_shapes](tasks/01_shapes) | Интерфейсы | Shape/Stringer/sort.Interface, method set, type switch, typed nil |
| [02_streams](tasks/02_streams) | Интерфейсы | Свои io.Reader/io.Writer: ROT13, LimitReader, CountingWriter, MultiWriter |
| [03_errs](tasks/03_errs) | Ошибки | Typed nil, errors.Join/Is/As, обход дерева ошибок, recover → error |
| [04_semver](tasks/04_semver) | Пакеты и модули | Разбор и сравнение SemVer, правило major version suffix |
| [05_modgraph](tasks/05_modgraph) | Пакеты и модули | Парсер go.mod, Minimal Version Selection, `go mod why` |
| [06_generics](tasks/06_generics) | Дженерики | Map/Filter/Reduce, Sum с `~`, MinMax с NaN, Set, Stack |
| [07_seq](tasks/07_seq) | Итераторы | Fibonacci, Filter/Map/Take/Enumerate/Zip/Chunk над iter.Seq |
| [08_tree](tasks/08_tree) | Итераторы, дженерики | Generic BST: in-order/Range с ранним выходом, MergeSorted через iter.Pull |
| [09_project_repo](tasks/09_project_repo) | **Задание 2** | Generic потокобезопасный репозиторий с индексами, ошибками-обёртками и итераторами |

## Как проверять

Каждая задача состоит из заготовки (`<name>.go` с тегом сборки `!solution`) и тестов к ней. Реализуйте заготовку и запустите тесты:

```
cd go-course  # корень курса
go test ./modules/02-intro-part2/tasks/01_shapes/
go test -race ./modules/02-intro-part2/...          # все задачи модуля
```

Эталонные решения лежат в файлах `*_solution.go` и собираются с тегом `solution`. Заглядывайте в них только после того, как попробовали решить задачу сами:

```
go test -tags solution ./modules/02-intro-part2/...
```

Проверить теорию поможет тест `quiz.json` из 38 вопросов.
