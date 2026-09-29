package jsonapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type item struct {
	Name string `json:"name"`
	Qty  int    `json:"qty"`
}

func TestDecodeJSON(t *testing.T) {
	tests := []struct {
		name  string
		ct    string
		body  string
		max   int64
		code  int // 0 — ожидается успех
		field string
	}{
		{"ok", "application/json", `{"name":"a","qty":2}`, 1024, 0, ""},
		{"okCharset", "application/json; charset=utf-8", `{"name":"a"}`, 1024, 0, ""},
		{"okSpaces", "application/json", "  {\"name\":\"a\"}\n\n", 1024, 0, ""},
		{"noCT", "", `{"name":"a"}`, 1024, 415, ""},
		{"textPlain", "text/plain", `{"name":"a"}`, 1024, 415, ""},
		{"badCT", "application/", `{"name":"a"}`, 1024, 415, ""},
		{"tooBig", "application/json", `{"name":"` + strings.Repeat("x", 2000) + `"}`, 1024, 413, ""},
		{"tooBigTrailing", "application/json", `{"name":"a"}` + strings.Repeat(" ", 2000), 100, 413, ""},
		{"empty", "application/json", ``, 1024, 400, ""},
		{"syntax", "application/json", `{"name":}`, 1024, 400, ""},
		{"truncated", "application/json", `{"name":"a"`, 1024, 400, ""},
		{"wrongType", "application/json", `{"qty":"много"}`, 1024, 400, "qty"},
		{"unknownField", "application/json", `{"name":"a","admin":true}`, 1024, 400, ""},
		{"twoObjects", "application/json", `{"name":"a"}{"name":"b"}`, 1024, 400, ""},
		{"garbageAfter", "application/json", `{"name":"a"} x`, 1024, 400, ""},
		{"array", "application/json", `[1,2]`, 1024, 400, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(tt.body))
			if tt.ct != "" {
				req.Header.Set("Content-Type", tt.ct)
			}
			var dst item
			err := DecodeJSON(httptest.NewRecorder(), req, &dst, tt.max)
			if tt.code == 0 {
				if err != nil {
					t.Fatalf("неожиданная ошибка: %v", err)
				}
				if dst.Name != "a" {
					t.Errorf("dst = %+v", dst)
				}
				return
			}
			var p *Problem
			if !errors.As(err, &p) {
				t.Fatalf("ожидался *Problem с кодом %d, получено %v", tt.code, err)
			}
			if p.Status != tt.code {
				t.Errorf("Status = %d, ожидалось %d (detail: %s)", p.Status, tt.code, p.Detail)
			}
			if tt.field != "" && p.Errors[tt.field] == "" {
				t.Errorf("ожидалась ошибка по полю %q, Errors=%v", tt.field, p.Errors)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		in   CreateUser
		bad  []string
		name string
	}{
		{CreateUser{"Анна", "anna@example.com", 30}, nil, "ok"},
		{CreateUser{"   ", "a@b.c", 30}, []string{"name"}, "blankName"},
		{CreateUser{strings.Repeat("я", 50), "a@b.c", 18}, nil, "name50runes"},
		{CreateUser{strings.Repeat("я", 51), "a@b.c", 150}, []string{"name"}, "name51runes"},
		{CreateUser{"A", "not-an-email", 20}, []string{"email"}, "badEmail"},
		{CreateUser{"A", "Anna <anna@example.com>", 20}, []string{"email"}, "displayName"},
		{CreateUser{"A", "a@b.c", 17}, []string{"age"}, "young"},
		{CreateUser{"A", "a@b.c", 151}, []string{"age"}, "old"},
		{CreateUser{"", "", 0}, []string{"name", "email", "age"}, "allBad"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := tt.in.Validate()
			if len(errs) != len(tt.bad) {
				t.Fatalf("Validate(%+v) = %v, ожидались ошибки по %v", tt.in, errs, tt.bad)
			}
			for _, f := range tt.bad {
				if errs[f] == "" {
					t.Errorf("нет ошибки по полю %q: %v", f, errs)
				}
			}
		})
	}
}

func TestWriteJSONAndProblem(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := WriteJSON(rec, 202, map[string]int{"n": 1}); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 202 || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
		t.Errorf("WriteJSON: код %d, Content-Type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	if strings.TrimSpace(rec.Body.String()) != `{"n":1}` {
		t.Errorf("WriteJSON тело %q", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	if err := WriteJSON(rec, 200, math.Inf(1)); err == nil {
		t.Error("WriteJSON(+Inf) должен вернуть ошибку")
	}
	if rec.Body.Len() != 0 || rec.Header().Get("Content-Type") != "" {
		t.Error("при ошибке сериализации WriteJSON не должен ничего писать")
	}

	rec = httptest.NewRecorder()
	WriteProblem(rec, &Problem{Status: 404, Detail: "нет такого"})
	if rec.Code != 404 || rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Errorf("WriteProblem: код %d, Content-Type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var p Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p.Type != "about:blank" || p.Title != "Not Found" || p.Status != 404 || p.Detail != "нет такого" {
		t.Errorf("WriteProblem тело: %+v", p)
	}
}

func post(h http.Handler, ct, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("POST", "/users", strings.NewReader(body))
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestCreateUserHandler(t *testing.T) {
	var got CreateUser
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewCreateUserHandler(func(ctx context.Context, in CreateUser) (User, error) {
		got = in
		switch in.Email {
		case "taken@example.com":
			return User{}, ErrConflict
		case "db@example.com":
			return User{}, errors.New("pq: connection refused to 10.0.0.5")
		}
		return User{ID: 7, Name: in.Name, Email: in.Email, Age: in.Age}, nil
	}, logger)

	rec := post(h, "application/json", `{"name":"  Анна ","email":"anna@example.com","age":30}`)
	if rec.Code != 201 {
		t.Fatalf("код %d, ожидалось 201; тело %s", rec.Code, rec.Body)
	}
	if got.Name != "Анна" {
		t.Errorf("в create передано имя %q, ожидалось обрезанное \"Анна\"", got.Name)
	}
	if loc := rec.Header().Get("Location"); loc != "/users/7" {
		t.Errorf("Location = %q", loc)
	}
	var u User
	if err := json.Unmarshal(rec.Body.Bytes(), &u); err != nil || u.ID != 7 {
		t.Errorf("тело ответа %s (%v)", rec.Body, err)
	}

	cases := []struct {
		name, ct, body string
		code           int
	}{
		{"415", "text/plain", `{}`, 415},
		{"400", "application/json", `{"name":"x","role":"admin"}`, 400},
		{"422", "application/json", `{"name":"","email":"x","age":1}`, 422},
		{"409", "application/json", `{"name":"a","email":"taken@example.com","age":20}`, 409},
		{"500", "application/json", `{"name":"a","email":"db@example.com","age":20}`, 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := post(h, c.ct, c.body)
			if rec.Code != c.code {
				t.Fatalf("код %d, ожидалось %d", rec.Code, c.code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Errorf("Content-Type = %q", ct)
			}
			var p Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("тело не Problem: %s", rec.Body)
			}
			if p.Status != c.code || p.Instance != "/users" {
				t.Errorf("Problem = %+v", p)
			}
			if c.code == 422 && len(p.Errors) != 3 {
				t.Errorf("422: Errors = %v, ожидалось 3 поля", p.Errors)
			}
			if c.code == 500 && strings.Contains(rec.Body.String(), "10.0.0.5") {
				t.Error("500 не должен раскрывать внутренние детали ошибки")
			}
		})
	}
}
