//go:build solution

package findfiles

import (
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// Find обходит fsys начиная с root и возвращает подходящие обычные файлы.
func Find(fsys fs.FS, root string, opt Options) ([]File, error) {
	if !fs.ValidPath(root) {
		return nil, &fs.PathError{Op: "find", Path: root, Err: fs.ErrInvalid}
	}
	// path.Match проверяет весь шаблон и вернёт ErrBadPattern, даже если имя не совпало.
	if _, err := path.Match(opt.Pattern, ""); err != nil {
		return nil, fmt.Errorf("шаблон %q: %w", opt.Pattern, err)
	}
	// Глубина считается от root: "a" внутри "." имеет глубину 1.
	depth := func(p string) int {
		rel := p
		if root != "." {
			rel = strings.TrimPrefix(strings.TrimPrefix(p, root), "/")
		}
		if rel == "" || rel == "." {
			return 0
		}
		return strings.Count(rel, "/") + 1
	}

	var out []File
	err := fs.WalkDir(fsys, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err // включая ошибку для несуществующего root
		}
		if p != root && opt.SkipHidden && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return fs.SkipDir // не заходим внутрь
			}
			return nil
		}
		dep := depth(p)
		if d.IsDir() {
			if opt.MaxDepth > 0 && dep >= opt.MaxDepth && p != root {
				return fs.SkipDir // файлы внутри были бы глубже MaxDepth
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil // симлинки, сокеты и т.п.
		}
		if opt.MaxDepth > 0 && dep > opt.MaxDepth {
			return nil
		}
		if opt.Pattern != "" {
			if ok, _ := path.Match(opt.Pattern, d.Name()); !ok {
				return nil
			}
		}
		info, err := d.Info() // Info — лишний stat, поэтому только после дешёвых фильтров
		if err != nil {
			return err
		}
		if info.Size() < opt.MinSize {
			return nil
		}
		out = append(out, File{Path: p, Size: info.Size()})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Tree рисует дерево каталога root в стиле утилиты tree.
func Tree(fsys fs.FS, root string) (string, error) {
	var sb strings.Builder
	sb.WriteString(root + "\n")
	var walk func(dir, prefix string) error
	walk = func(dir, prefix string) error {
		entries, err := fs.ReadDir(fsys, dir) // уже отсортированы по имени
		if err != nil {
			return err
		}
		for i, e := range entries {
			last := i == len(entries)-1
			branch, indent := "├── ", "│   "
			if last {
				branch, indent = "└── ", "    "
			}
			sb.WriteString(prefix + branch + e.Name() + "\n")
			if e.IsDir() {
				if err := walk(path.Join(dir, e.Name()), prefix+indent); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(root, ""); err != nil {
		return "", err
	}
	return sb.String(), nil
}
