//go:build !solution

// Package websec — защитные механизмы веб-приложения: security-заголовки,
// безопасные cookie, CSRF (double submit), CORS, защита от path traversal и XSS.
package websec

import (
	"io"
	"net/http"
	"time"
)

// SecurityHeaders выставляет (до вызова next):
//
//	Content-Security-Policy: default-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'
//	X-Content-Type-Options: nosniff
//	X-Frame-Options: DENY
//	Referrer-Policy: no-referrer
//	Strict-Transport-Security: max-age=63072000; includeSubDomains   — только если r.TLS != nil
func SecurityHeaders(next http.Handler) http.Handler {
	// TODO: реализуйте
	return next
}

// SetSessionCookie ставит cookie SessionCookieName: Path=/, MaxAge в секундах,
// HttpOnly, Secure, SameSite=Lax, без Domain.
func SetSessionCookie(w http.ResponseWriter, value string, maxAge time.Duration) {
	// TODO: реализуйте
}

// CSRF — защита double submit cookie:
//   - если в запросе нет валидной cookie CSRFCookieName (32 случайных байта в
//     base64.RawURLEncoding), сгенерировать токен и поставить cookie
//     (Path=/, Secure, SameSite=Strict, НЕ HttpOnly — её читает JS);
//   - безопасные методы (GET, HEAD, OPTIONS, TRACE) пропускать;
//   - для остальных: если есть заголовок Origin, его host должен совпадать с r.Host;
//     токен из заголовка CSRFHeaderName (или поля формы CSRFFormField) должен
//     совпасть с токеном из cookie запроса — сравнение subtle.ConstantTimeCompare;
//     иначе 403 и next не вызывается;
//   - токен кладётся в контекст (см. CSRFToken).
func CSRF(next http.Handler) http.Handler {
	// TODO: реализуйте
	return next
}

// CSRFToken возвращает CSRF-токен текущего запроса (положенный CSRF) или "".
func CSRFToken(r *http.Request) string {
	// TODO: реализуйте
	return ""
}

// SafeJoin соединяет root и путь из URL так, чтобы результат не мог выйти за root.
//   - ведущие "/" отбрасываются ("/img/a.png" → root/img/a.png);
//   - пустой путь, NUL-байт, выход за корень ("..", "a/../../x") → ErrUnsafePath;
//   - используйте filepath.IsLocal (Go 1.20+) и filepath.Join.
func SafeJoin(root, userPath string) (string, error) {
	// TODO: реализуйте
	return root + "/" + userPath, nil
}

// CORS разрешает кросс-доменные запросы с cookie только для точных совпадений
// Origin из allowedOrigins ("null" не разрешается никогда).
//   - всегда добавлять Vary: Origin;
//   - разрешённый Origin → Access-Control-Allow-Origin: <origin> (не "*") и
//     Access-Control-Allow-Credentials: true;
//   - preflight (OPTIONS + Access-Control-Request-Method): разрешённый → 204 с
//     Allow-Methods "GET, POST, PUT, DELETE", Allow-Headers "Content-Type, X-CSRF-Token",
//     Max-Age "600"; неразрешённый → 403; next не вызывается;
//   - обычный запрос с неразрешённым Origin → next без CORS-заголовков.
func CORS(allowedOrigins []string, next http.Handler) http.Handler {
	// TODO: реализуйте
	return next
}

// Greeting пишет в w HTML:
//
//	<p>Hello, NAME!</p><a href="HOMEPAGE">homepage</a>
//
// так, чтобы name и homepage не могли внедрить скрипт (используйте html/template).
func Greeting(w io.Writer, name, homepage string) error {
	// TODO: реализуйте
	_, err := io.WriteString(w, "<p>Hello, "+name+"!</p><a href=\""+homepage+"\">homepage</a>")
	return err
}
