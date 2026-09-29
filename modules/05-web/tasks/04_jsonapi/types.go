package jsonapi

import (
	"context"
	"errors"
	"fmt"
)

// Problem — тело ошибки по RFC 9457 (Problem Details for HTTP APIs).
// Реализует error, чтобы DecodeJSON мог возвращать его как обычную ошибку.
type Problem struct {
	Type     string            `json:"type,omitempty"`
	Title    string            `json:"title"`
	Status   int               `json:"status"`
	Detail   string            `json:"detail,omitempty"`
	Instance string            `json:"instance,omitempty"`
	Errors   map[string]string `json:"errors,omitempty"` // расширение: ошибки по полям
}

func (p *Problem) Error() string { return fmt.Sprintf("%d %s: %s", p.Status, p.Title, p.Detail) }

// Validator реализуют типы, умеющие проверять себя после декодирования.
// Пустая (или nil) карта означает «всё хорошо».
type Validator interface {
	Validate() map[string]string
}

// CreateUser — тело запроса POST /users.
type CreateUser struct {
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

// User — ответ.
type User struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Age   int    `json:"age"`
}

// ErrConflict возвращает хранилище, если email уже занят.
var ErrConflict = errors.New("conflict")

// CreateFunc — бизнес-логика создания пользователя.
type CreateFunc func(ctx context.Context, in CreateUser) (User, error)
