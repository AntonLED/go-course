# Задача 04. SemVer: разбор, сравнение, правило /vN

Пакет `semver`. Каждый раз, когда вы пишете `go get`, инструмент сравнивает версии модулей и решает, какую взять. Go-модули версионируются по [SemVer 2.0.0](https://semver.org/lang/ru/) с префиксом `v`, а пути модулей с мажорной версией 2 и выше обязаны оканчиваться на `/vN`. В этой задаче вы реализуете ту же логику сами. Реализуйте в `semver.go`:

```go
func Parse(s string) (Version, error)             // "v1.2.3-rc.1+build" -> Version
func (v Version) String() string                  // обратно в каноническую строку
func Compare(a, b Version) int                    // -1 / 0 / +1 по правилам приоритета
func Max(vs ...string) (string, error)            // максимальная версия из списка
func CheckModulePath(path string, v Version) error // правило major version suffix
```

Тип `Version` и ошибки `ErrInvalid`, `ErrMajorMismatch` уже объявлены в `types.go`.

## Примеры

```
v1.0.0-alpha < v1.0.0-alpha.1 < v1.0.0-alpha.beta < v1.0.0-beta
  < v1.0.0-beta.2 < v1.0.0-beta.11 < v1.0.0-rc.1 < v1.0.0 < v1.10.0

Max("v1.9.0", "v1.10.0", "v1.2.0") == "v1.10.0"   // а не "v1.9.0", как при сравнении строк!

CheckModulePath("github.com/a/b",    v2.0.0) -> ErrMajorMismatch
CheckModulePath("github.com/a/b/v2", v2.0.0) -> nil
CheckModulePath("github.com/a/b/v1", v1.0.0) -> ErrMajorMismatch (суффиксы /v0, /v1 запрещены)
CheckModulePath("github.com/a/b/v02", v2.0.0) -> ErrMajorMismatch (суффикс — ровно "v<MAJOR>", без ведущих нулей)
```

## Подвохи

Ведущие нули запрещены в MAJOR, MINOR и PATCH, а также в числовых идентификаторах pre-release (например, `-01`). В build-метаданных они разрешены.

Разбирайте строку в правильном порядке: сначала отрежьте `+build` (внутри него тоже могут встречаться `-`), и только потом отделяйте `-pre` по первому `-`.

При сравнении pre-release числовой идентификатор всегда младше буквенного, поэтому `alpha.1 < alpha.beta`. Числовые идентификаторы сравниваются как числа, а не как строки: `beta.2 < beta.11`. Длинные числовые идентификаторы могут не поместиться в `uint64`, так что сравнивайте их сначала по длине, а потом лексикографически.

Build-метаданные на порядок не влияют: `v1.0.0+a == v1.0.0+b`.

Все ошибки разбора оборачивают `ErrInvalid` через `%w`.

## Проверка

```
go test ./modules/02-intro-part2/tasks/04_semver/
```
