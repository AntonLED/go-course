// Package findfiles — поиск файлов и вывод дерева каталогов поверх абстракции fs.FS.
package findfiles

// Options — критерии поиска.
type Options struct {
	Pattern    string // glob по имени файла (path.Match), "" — любые
	MinSize    int64  // минимальный размер в байтах (включительно)
	MaxDepth   int    // 0 — без ограничений; 1 — только файлы прямо в root; 2 — плюс один уровень вложенности …
	SkipHidden bool   // пропускать файлы и каталоги, имя которых начинается с "." (в скрытые каталоги не заходить)
}

// File — найденный файл.
type File struct {
	Path string // путь внутри fsys, со слэшами (как отдаёт fs.WalkDir)
	Size int64
}
