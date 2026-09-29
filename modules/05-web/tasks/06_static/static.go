//go:build !solution

package static

import (
	"errors"
	"io/fs"
	"net/http"
)

// FileServer возвращает обработчик статики из fsys (требования — в README):
// ETag по содержимому, 304 на If-None-Match, Cache-Control, index.html для каталогов,
// 404 для скрытых файлов и каталогов без index.html, 405 для не GET/HEAD.
func FileServer(fsys fs.FS) (http.Handler, error) {
	// TODO
	return nil, errors.New("TODO")
}
