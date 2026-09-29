package semver

import "errors"

// Version — семантическая версия в стиле Go-модулей: vMAJOR.MINOR.PATCH[-PRE][+BUILD].
type Version struct {
	Major, Minor, Patch uint64
	Pre                 []string // идентификаторы pre-release: "rc.1" -> ["rc", "1"]; nil, если нет
	Build               string   // метаданные сборки без '+'; в сравнении не участвуют
}

var (
	// ErrInvalid — строка не является корректной версией.
	ErrInvalid = errors.New("invalid semantic version")
	// ErrMajorMismatch — путь модуля не соответствует major-версии (правило /vN).
	ErrMajorMismatch = errors.New("module path does not match major version")
)
