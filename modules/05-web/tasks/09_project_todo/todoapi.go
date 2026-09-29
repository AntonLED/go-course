//go:build !solution

package todoapi

import (
	"log/slog"
	"net/http"
)

// NewServer собирает REST API задач поверх store (полная спецификация — в README):
//
//	GET    /healthz
//	POST   /todos
//	GET    /todos?limit=&offset=
//	GET    /todos/{id}
//	PATCH  /todos/{id}
//	DELETE /todos/{id}
//
// плюс middleware Recover, RequestID и Logging (в logger), ошибки — application/problem+json.
// Возвращаемый http.Handler передаётся в http.Server — graceful shutdown остаётся за вызывающим.
func NewServer(store Store, logger *slog.Logger) http.Handler {
	// TODO
	return http.NotFoundHandler()
}
