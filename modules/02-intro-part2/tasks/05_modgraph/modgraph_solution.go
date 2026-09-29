//go:build solution

package modgraph

import (
	"fmt"
	"slices"
	"strings"
)

func syntaxErr(line int, format string, args ...any) error {
	return fmt.Errorf("go.mod:%d: %s: %w", line, fmt.Sprintf(format, args...), ErrSyntax)
}

func ParseGoMod(src string) (*File, error) {
	f := &File{}
	block := ""     // имя директивы открытого блока ("" — вне блока)
	blockStart := 0 // строка, где блок открыт
	for i, raw := range strings.Split(src, "\n") {
		lineNo := i + 1
		code, comment, _ := strings.Cut(raw, "//")
		fields := strings.Fields(code)
		indirect := strings.TrimSpace(comment) == "indirect"
		if len(fields) == 0 {
			continue
		}
		if block != "" {
			if len(fields) == 1 && fields[0] == ")" {
				block = ""
				continue
			}
			if err := f.directive(lineNo, block, fields, indirect); err != nil {
				return nil, err
			}
			continue
		}
		verb, args := fields[0], fields[1:]
		if len(args) == 1 && args[0] == "(" {
			switch verb {
			case "require", "replace", "exclude", "retract", "godebug":
				block, blockStart = verb, lineNo
				continue
			default:
				return nil, syntaxErr(lineNo, "блок %q не поддерживается", verb)
			}
		}
		if err := f.directive(lineNo, verb, args, indirect); err != nil {
			return nil, err
		}
	}
	if block != "" {
		return nil, syntaxErr(blockStart, "незакрытый блок %s", block)
	}
	if f.Module == "" {
		return nil, fmt.Errorf("go.mod: нет директивы module: %w", ErrSyntax)
	}
	return f, nil
}

// directive обрабатывает одну директиву (в блоке или однострочную).
func (f *File) directive(line int, verb string, args []string, indirect bool) error {
	switch verb {
	case "module":
		if len(args) != 1 {
			return syntaxErr(line, "module: нужен 1 аргумент")
		}
		f.Module = args[0]
	case "go":
		if len(args) != 1 {
			return syntaxErr(line, "go: нужен 1 аргумент")
		}
		f.Go = args[0]
	case "require":
		if len(args) != 2 {
			return syntaxErr(line, "require: нужно <path> <version>")
		}
		f.Require = append(f.Require, Requirement{Module{args[0], args[1]}, indirect})
	case "replace":
		arrow := slices.Index(args, "=>")
		if arrow < 0 {
			return syntaxErr(line, "replace: нет =>")
		}
		old, nw := args[:arrow], args[arrow+1:]
		if len(old) < 1 || len(old) > 2 || len(nw) < 1 || len(nw) > 2 {
			return syntaxErr(line, "replace: неверное число аргументов")
		}
		r := Replace{Old: Module{Path: old[0]}, New: Module{Path: nw[0]}}
		if len(old) == 2 {
			r.Old.Version = old[1]
		}
		if len(nw) == 2 {
			r.New.Version = nw[1]
		}
		f.Replace = append(f.Replace, r)
	case "toolchain", "exclude", "retract", "godebug":
		// допустимы, но нам не нужны
	default:
		return syntaxErr(line, "неизвестная директива %q", verb)
	}
	return nil
}

func BuildList(main Module, g Graph) []Module {
	selected := map[string]string{} // path -> максимальная версия
	visited := map[Module]bool{main: true}
	queue := []Module{main}
	for len(queue) > 0 {
		m := queue[0]
		queue = queue[1:]
		for _, dep := range g[m] {
			if dep.Path == main.Path || visited[dep] {
				continue
			}
			visited[dep] = true
			queue = append(queue, dep)
			// MVS: из всех упомянутых версий берём максимальную — это
			// минимальная версия, удовлетворяющая всем требованиям.
			if cur, ok := selected[dep.Path]; !ok || CompareVersions(dep.Version, cur) > 0 {
				selected[dep.Path] = dep.Version
			}
		}
	}
	out := make([]Module, 0, len(selected)+1)
	for p, v := range selected {
		out = append(out, Module{p, v})
	}
	slices.SortFunc(out, func(a, b Module) int { return strings.Compare(a.Path, b.Path) })
	return append([]Module{main}, out...)
}

func Why(main Module, g Graph, path string) ([]Module, error) {
	if main.Path == path {
		return []Module{main}, nil
	}
	parent := map[Module]Module{}
	visited := map[Module]bool{main: true}
	queue := []Module{main}
	for len(queue) > 0 {
		m := queue[0]
		queue = queue[1:]
		for _, dep := range g[m] {
			if visited[dep] {
				continue
			}
			visited[dep] = true
			parent[dep] = m
			if dep.Path == path {
				// Восстанавливаем путь по родителям.
				chain := []Module{dep}
				for cur := dep; cur != main; {
					cur = parent[cur]
					chain = append(chain, cur)
				}
				slices.Reverse(chain)
				return chain, nil
			}
			queue = append(queue, dep)
		}
	}
	return nil, fmt.Errorf("why %s: %w", path, ErrNotRequired)
}
