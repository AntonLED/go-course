//go:build !solution

package pages

import (
	"errors"
	"io"
	"net/http"
)

// Каркас страницы можно взять за основу. Обратите внимание на {{block "content" .}} —
// шаблон "content" нужно определить (define) отдельно.
const layout = `{{define "layout"}}<!doctype html>
<html lang="ru">
<head><meta charset="utf-8"><title>{{.Title}}</title></head>
<body>
<main>{{block "content" .}}<p>нет содержимого</p>{{end}}</main>
</body>
</html>
{{end}}`

// TODO: шаблон "content", функции price и join, приветствие пользователя.

// Renderer хранит разобранные шаблоны.
type Renderer struct {
	// TODO
}

// NewRenderer разбирает шаблоны (html/template!) один раз.
func NewRenderer() (*Renderer, error) {
	// TODO
	return nil, errors.New("TODO")
}

// Render выполняет шаблон "layout" с данными p.
func (r *Renderer) Render(w io.Writer, p Page) error {
	// TODO
	return errors.New("TODO")
}

// Handler: load(req) → Render в буфер → 200 text/html; charset=utf-8.
// Любая ошибка (load или рендеринга) → 500 без кусков HTML в ответе.
func (r *Renderer) Handler(load func(*http.Request) (Page, error)) http.Handler {
	// TODO
	return http.NotFoundHandler()
}
