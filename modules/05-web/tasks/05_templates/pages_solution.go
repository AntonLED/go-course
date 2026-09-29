//go:build solution

package pages

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"
)

// layout задаёт каркас; {{block}} = define + template: значение по умолчанию,
// которое можно переопределить в другом шаблоне.
const layout = `{{define "layout"}}<!doctype html>
<html lang="ru">
<head><meta charset="utf-8"><title>{{.Title}}</title></head>
<body>
{{- if .User}}
<p class="greeting">Привет, {{.User}}!</p>
{{- end}}
<main>{{block "content" .}}<p>нет содержимого</p>{{end}}</main>
</body>
</html>
{{end}}`

const content = `{{define "content"}}
<h1>{{.Title}}</h1>
{{- with .Items}}
<ul>
{{- range .}}
<li class="item"><a href="{{.URL}}">{{.Title}}</a> <span class="price">{{price .PriceCents}}</span>
{{- with .Tags}} <span class="tags">{{join . ", "}}</span>{{end}}</li>
{{- end}}
</ul>
{{- else}}
<p class="empty">Пусто</p>
{{- end}}
{{end}}`

// price форматирует копейки. Второй результат error — если он не nil,
// Execute прерывается с этой ошибкой.
func price(cents int64) (string, error) {
	if cents < 0 {
		return "", errors.New("отрицательная цена")
	}
	return fmt.Sprintf("%d.%02d ₽", cents/100, cents%100), nil
}

// Renderer хранит разобранные шаблоны. Разбирать шаблоны на каждый запрос —
// лишняя работа; *template.Template безопасен для конкурентного Execute.
type Renderer struct {
	t *template.Template
}

// NewRenderer разбирает шаблоны один раз.
func NewRenderer() (*Renderer, error) {
	t, err := template.New("page").
		Funcs(template.FuncMap{"price": price, "join": strings.Join}). // Funcs — ДО Parse
		Parse(layout)
	if err != nil {
		return nil, err
	}
	// Переопределять шаблон (здесь — "content" из {{block}}) можно только
	// отдельным вызовом Parse: в одном Parse повторный define — ошибка.
	if t, err = t.Parse(content); err != nil {
		return nil, err
	}
	return &Renderer{t: t}, nil
}

// Render выполняет шаблон "layout".
func (r *Renderer) Render(w io.Writer, p Page) error {
	return r.t.ExecuteTemplate(w, "layout", p)
}

// Handler рендерит страницу в буфер и только потом отправляет клиенту.
func (r *Renderer) Handler(load func(*http.Request) (Page, error)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		p, err := load(req)
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		// Ошибка может случиться посреди Execute — если писать прямо в w,
		// клиент получит 200 и обрезанный HTML.
		var buf bytes.Buffer
		if err := r.Render(&buf, p); err != nil {
			log.Printf("render: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = buf.WriteTo(w)
	})
}
