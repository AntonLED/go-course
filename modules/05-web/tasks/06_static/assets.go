package static

import (
	"embed"
	"io/fs"
)

// Файлы из assets/ вшиваются в бинарник на этапе компиляции.
//
//go:embed assets
var assets embed.FS

// Assets возвращает содержимое каталога assets/ как корень ФС
// (без префикса "assets/" в путях).
func Assets() fs.FS {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		panic(err) // невозможно: каталог гарантирован директивой embed
	}
	return sub
}
