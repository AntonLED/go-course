// Команда quiz — веб-морда для тестов курса.
//
//	go run ./cmd/quiz            # поднять сервер на http://localhost:8080
//	go run ./cmd/quiz -addr :9000
//	go run ./cmd/quiz -build     # собрать quiz/data.js, чтобы открывать quiz/index.html без сервера
//
// Вопросы берутся из modules/*/quiz.json при каждом запросе, так что правки
// видны сразу после перезагрузки страницы.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func main() {
	addr := flag.String("addr", "localhost:8080", "адрес для HTTP-сервера")
	build := flag.Bool("build", false, "только записать quiz/data.js и выйти")
	root := flag.String("root", ".", "корень курса (где лежат modules/ и quiz/)")
	flag.Parse()

	if *build {
		js, err := dataJS(*root)
		if err != nil {
			log.Fatal(err)
		}
		out := filepath.Join(*root, "quiz", "data.js")
		if err := os.WriteFile(out, js, 0o644); err != nil {
			log.Fatal(err)
		}
		fmt.Println("записано:", out)
		return
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /data.js", func(w http.ResponseWriter, r *http.Request) {
		js, err := dataJS(*root)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(js)
	})
	mux.Handle("GET /", http.FileServer(http.Dir(filepath.Join(*root, "quiz"))))

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	fmt.Printf("Тесты курса: http://%s/\n", *addr)
	log.Fatal(srv.ListenAndServe())
}

// dataJS собирает все modules/*/quiz.json в один JS-файл: window.QUIZ = [...].
func dataJS(root string) ([]byte, error) {
	files, err := filepath.Glob(filepath.Join(root, "modules", "*", "quiz.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	mods := make([]json.RawMessage, 0, len(files))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		if !json.Valid(b) {
			return nil, fmt.Errorf("%s: невалидный JSON", f)
		}
		var c bytes.Buffer
		if err := json.Compact(&c, b); err != nil {
			return nil, err
		}
		mods = append(mods, c.Bytes())
	}
	all, err := json.Marshal(mods)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString("// Сгенерировано: go run ./cmd/quiz -build. Не редактировать руками.\nwindow.QUIZ = ")
	buf.Write(all)
	buf.WriteString(";\n")
	return buf.Bytes(), nil
}
