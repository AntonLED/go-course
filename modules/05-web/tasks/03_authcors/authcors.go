//go:build !solution

package authcors

import (
	"context"
	"net/http"
)

// BasicAuth возвращает middleware HTTP Basic-аутентификации.
//   - Логин/пароль — из r.BasicAuth(); сравнение пароля — за постоянное время
//     (crypto/subtle.ConstantTimeCompare поверх sha256-хэшей).
//   - Неудача → 401 и заголовок WWW-Authenticate: Basic realm="<realm>", charset="UTF-8".
//   - Успех → имя пользователя кладётся в контекст (см. UserFrom).
func BasicAuth(realm string, users map[string]string) func(http.Handler) http.Handler {
	// TODO
	return func(next http.Handler) http.Handler { return next }
}

// UserFrom возвращает имя пользователя, положенное BasicAuth.
func UserFrom(ctx context.Context) (string, bool) {
	// TODO
	return "", false
}

// CORS возвращает middleware, реализующее политику cfg (подробности в README).
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	// TODO
	return func(next http.Handler) http.Handler { return next }
}
