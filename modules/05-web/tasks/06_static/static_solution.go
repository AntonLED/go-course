//go:build solution

package static

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

type file struct {
	data    []byte
	etag    string
	modTime time.Time
}

// FileServer заранее читает все файлы из fsys, считает ETag и отдаёт их с кэш-заголовками.
func FileServer(fsys fs.FS) (http.Handler, error) {
	files := make(map[string]*file)
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		files[p] = &file{
			data: data,
			// Сильный ETag: кавычки обязательны по RFC 9110.
			etag:    `"` + hex.EncodeToString(sum[:8]) + `"`,
			modTime: info.ModTime(),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		name := resolve(r.URL.Path)
		if hidden(name) {
			http.NotFound(w, r)
			return
		}
		f, ok := files[name]
		if !ok {
			// Каталог? Отдаём его index.html (листинг каталогов не показываем).
			idx := path.Join(name, "index.html")
			if name == "." {
				idx = "index.html"
			}
			if f, ok = files[idx]; !ok {
				http.NotFound(w, r)
				return
			}
			name = idx
		}
		h := w.Header()
		h.Set("ETag", f.etag)
		if strings.HasSuffix(name, ".html") {
			h.Set("Cache-Control", "no-cache") // кэшировать можно, но каждый раз ревалидировать
		} else {
			h.Set("Cache-Control", "public, max-age=86400")
		}
		// ServeContent сам: Content-Type по расширению, If-None-Match/If-Modified-Since → 304,
		// Range-запросы, HEAD без тела.
		http.ServeContent(w, r, name, f.modTime, bytes.NewReader(f.data))
	}), nil
}

// resolve превращает URL-путь в имя для fs.FS: без ведущего "/", без ".." (path.Clean).
func resolve(urlPath string) string {
	p := path.Clean("/" + urlPath)
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "."
	}
	return p
}

// hidden — есть ли в пути сегмент, начинающийся с точки (.env, .git/...).
func hidden(name string) bool {
	if name == "." {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}
