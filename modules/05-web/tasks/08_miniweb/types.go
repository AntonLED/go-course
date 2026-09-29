package miniweb

import "fmt"

// HandlerFunc — обработчик фреймворка: в отличие от http.HandlerFunc, возвращает ошибку.
// Так делают echo и (по сути) chi-обёртки: ошибка обрабатывается в одном месте.
type HandlerFunc func(c *Context) error

// Middleware оборачивает HandlerFunc.
type Middleware func(next HandlerFunc) HandlerFunc

// HTTPError — ошибка с HTTP-кодом, которую можно показать клиенту.
type HTTPError struct {
	Code    int
	Message string
}

func (e *HTTPError) Error() string { return fmt.Sprintf("http %d: %s", e.Code, e.Message) }

// NewHTTPError создаёт *HTTPError.
func NewHTTPError(code int, msg string) *HTTPError { return &HTTPError{Code: code, Message: msg} }
