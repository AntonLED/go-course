package modgraph

import (
	"errors"
	"strconv"
	"strings"
)

// Module — модуль конкретной версии. У главного модуля Version == "".
type Module struct {
	Path    string
	Version string
}

func (m Module) String() string {
	if m.Version == "" {
		return m.Path
	}
	return m.Path + "@" + m.Version
}

// Requirement — строка из блока require.
type Requirement struct {
	Module
	Indirect bool // помечена комментарием "// indirect"
}

// Replace — директива replace. Old.Version == "" означает «все версии».
// New.Version == "" означает локальный путь (./fork, ../lib).
type Replace struct {
	Old, New Module
}

// File — результат разбора go.mod.
type File struct {
	Module  string
	Go      string
	Require []Requirement
	Replace []Replace
}

// Graph — граф требований: какая версия модуля каких версий других модулей
// требует (как в выводе `go mod graph`). Модуль, которого нет среди ключей,
// считается не имеющим зависимостей.
type Graph map[Module][]Module

var (
	// ErrSyntax — синтаксическая ошибка в go.mod.
	ErrSyntax = errors.New("go.mod syntax error")
	// ErrNotRequired — модуль не входит в граф зависимостей.
	ErrNotRequired = errors.New("module is not required")
)

// CompareVersions сравнивает упрощённые версии "vMAJOR.MINOR.PATCH" (без
// pre-release). Уже готова — используй её в BuildList.
func CompareVersions(a, b string) int {
	pa := strings.Split(strings.TrimPrefix(a, "v"), ".")
	pb := strings.Split(strings.TrimPrefix(b, "v"), ".")
	for i := 0; i < 3; i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}
