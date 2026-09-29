//go:build !solution

package jsonapi

import (
	"errors"
	"log/slog"
	"net/http"
)

// WriteJSON пишет v как JSON с Content-Type "application/json; charset=utf-8" и кодом status.
// Если v не сериализуется — вернуть ошибку, ничего не записав в w.
func WriteJSON(w http.ResponseWriter, status int, v any) error {
	// TODO
	return errors.New("TODO")
}

// WriteProblem пишет p с Content-Type "application/problem+json" и кодом p.Status.
// Пустой Type → "about:blank", пустой Title → http.StatusText(p.Status).
func WriteProblem(w http.ResponseWriter, p *Problem) {
	// TODO
}

// DecodeJSON читает из r.Body ровно один JSON-объект в dst.
// Возвращает nil или *Problem с кодом:
//
//	415 — Content-Type не application/json (параметры вроде charset допустимы);
//	413 — тело больше maxBytes (http.MaxBytesReader);
//	400 — пустое тело, синтаксис, неверный тип, неизвестное поле, лишние данные после объекта;
//	422 — dst реализует Validator и вернул непустую карту (она кладётся в Problem.Errors).
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	// TODO
	return errors.New("TODO")
}

// Validate проверяет CreateUser: name (после TrimSpace) 1..50 рун, email — корректный
// одиночный адрес (net/mail, без «Имя <...>»), age в [18, 150].
// Ключи ошибок: "name", "email", "age".
func (c *CreateUser) Validate() map[string]string {
	// TODO
	return nil
}

// NewCreateUserHandler возвращает обработчик POST /users:
//   - DecodeJSON с лимитом 1 MiB; ошибка → WriteProblem (Instance = r.URL.Path);
//   - create(r.Context(), in) с обрезанным пробелами Name;
//   - ErrConflict → 409; другая ошибка → 500 без деталей (детали — в logger);
//   - успех → 201, Location: /users/{id}, JSON пользователя.
func NewCreateUserHandler(create CreateFunc, logger *slog.Logger) http.Handler {
	// TODO
	return http.NotFoundHandler()
}
