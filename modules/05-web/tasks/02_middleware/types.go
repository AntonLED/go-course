package middleware

import "net/http"

// Middleware — стандартная форма middleware в экосистеме net/http.
type Middleware func(http.Handler) http.Handler

// HeaderRequestID — заголовок с идентификатором запроса.
const HeaderRequestID = "X-Request-ID"

// ctxKey — неэкспортируемый тип ключа контекста: исключает коллизии с другими пакетами.
type ctxKey struct{}
