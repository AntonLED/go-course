//go:build !solution

package findfiles

import "io/fs"

// Find обходит fsys начиная с root и возвращает обычные файлы (не каталоги),
// подходящие под opt, в порядке обхода fs.WalkDir (лексикографическом внутри каталога).
//
// Ошибки: некорректный root (fs.ValidPath) или несуществующий root,
// некорректный Pattern (path.ErrBadPattern) — даже если файлов нет.
func Find(fsys fs.FS, root string, opt Options) ([]File, error) {
	// TODO: fs.WalkDir + fs.SkipDir
	panic("TODO")
}

// Tree рисует дерево каталога root в стиле утилиты tree:
//
//	root
//	├── a.txt
//	├── dir
//	│   └── b.txt
//	└── z.go
//
// Первая строка — root как есть, элементы каталога — в порядке fs.ReadDir,
// каждая строка заканчивается '\n'. Пустой каталог — только строка root.
func Tree(fsys fs.FS, root string) (string, error) {
	// TODO: рекурсия по fs.ReadDir с накоплением префикса
	panic("TODO")
}
