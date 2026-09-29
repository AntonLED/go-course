//go:build solution

package jsonapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/mail"
	"strconv"
	"strings"
	"unicode/utf8"
)

// WriteJSON сериализует v и пишет ответ. Заголовки — строго ДО WriteHeader.
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	// Кодируем в память ДО WriteHeader: если Marshal упадёт, мы ещё можем ответить 500,
	// а не отдать клиенту уже отправленный код 200 с пустым телом.
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, err = w.Write(append(b, '\n'))
	return err
}

// WriteProblem пишет Problem Details с Content-Type application/problem+json.
func WriteProblem(w http.ResponseWriter, p *Problem) {
	if p.Type == "" {
		p.Type = "about:blank"
	}
	if p.Title == "" {
		p.Title = http.StatusText(p.Status)
	}
	b, _ := json.Marshal(p)
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(p.Status)
	_, _ = w.Write(b)
}

func problem(status int, detail string) *Problem {
	return &Problem{Status: status, Title: http.StatusText(status), Detail: detail}
}

// DecodeJSON читает тело запроса в dst. Возвращает nil или *Problem.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	ct := r.Header.Get("Content-Type")
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil || mt != "application/json" {
		return problem(http.StatusUnsupportedMediaType, "ожидается Content-Type: application/json")
	}

	// MaxBytesReader, в отличие от io.LimitReader, возвращает ошибку при превышении
	// и просит сервер закрыть соединение (клиент может продолжать слать гигабайты).
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var (
			syntaxErr *json.SyntaxError
			typeErr   *json.UnmarshalTypeError
			maxErr    *http.MaxBytesError
		)
		switch {
		case errors.As(err, &maxErr):
			return problem(http.StatusRequestEntityTooLarge, fmt.Sprintf("тело больше %d байт", maxErr.Limit))
		case errors.Is(err, io.EOF):
			return problem(http.StatusBadRequest, "пустое тело запроса")
		case errors.As(err, &syntaxErr):
			return problem(http.StatusBadRequest, fmt.Sprintf("некорректный JSON (позиция %d)", syntaxErr.Offset))
		case errors.Is(err, io.ErrUnexpectedEOF):
			return problem(http.StatusBadRequest, "некорректный JSON (обрыв)")
		case errors.As(err, &typeErr):
			p := problem(http.StatusBadRequest, "неверный тип поля")
			if typeErr.Field != "" {
				p.Errors = map[string]string{typeErr.Field: "ожидается " + typeErr.Type.String()}
			}
			return p
		case strings.HasPrefix(err.Error(), "json: unknown field "):
			// У encoding/json нет типизированной ошибки для неизвестного поля.
			return problem(http.StatusBadRequest, strings.TrimPrefix(err.Error(), "json: "))
		default:
			return problem(http.StatusBadRequest, err.Error())
		}
	}

	// Ровно один JSON-документ: `{"a":1}{"b":2}` или `{} мусор` — ошибка.
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return problem(http.StatusRequestEntityTooLarge, fmt.Sprintf("тело больше %d байт", maxErr.Limit))
		}
		return problem(http.StatusBadRequest, "тело должно содержать ровно один JSON-объект")
	}

	if v, ok := dst.(Validator); ok {
		if errs := v.Validate(); len(errs) > 0 {
			p := problem(http.StatusUnprocessableEntity, "ошибка валидации")
			p.Errors = errs
			return p
		}
	}
	return nil
}

// Validate проверяет бизнес-правила CreateUser.
func (c *CreateUser) Validate() map[string]string {
	errs := map[string]string{}
	name := strings.TrimSpace(c.Name)
	switch n := utf8.RuneCountInString(name); {
	case n == 0:
		errs["name"] = "обязательное поле"
	case n > 50:
		errs["name"] = "не длиннее 50 символов"
	}
	if a, err := mail.ParseAddress(c.Email); err != nil || a.Address != c.Email {
		errs["email"] = "некорректный email"
	}
	if c.Age < 18 || c.Age > 150 {
		errs["age"] = "от 18 до 150"
	}
	return errs
}

// NewCreateUserHandler — POST-обработчик создания пользователя.
func NewCreateUserHandler(create CreateFunc, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in CreateUser
		if err := DecodeJSON(w, r, &in, 1<<20); err != nil {
			var p *Problem
			if !errors.As(err, &p) {
				p = problem(http.StatusBadRequest, err.Error())
			}
			p.Instance = r.URL.Path
			WriteProblem(w, p)
			return
		}
		in.Name = strings.TrimSpace(in.Name)
		u, err := create(r.Context(), in)
		switch {
		case errors.Is(err, ErrConflict):
			WriteProblem(w, &Problem{Status: http.StatusConflict, Detail: "email уже занят", Instance: r.URL.Path})
			return
		case err != nil:
			// Детали — в лог, клиенту — ничего внутреннего.
			logger.Error("create user", "err", err)
			WriteProblem(w, &Problem{Status: http.StatusInternalServerError, Instance: r.URL.Path})
			return
		}
		w.Header().Set("Location", "/users/"+strconv.FormatInt(u.ID, 10))
		_ = WriteJSON(w, http.StatusCreated, u)
	})
}
