//go:build solution

// Package websec — защитные механизмы веб-приложения: security-заголовки,
// безопасные cookie, CSRF (double submit), CORS, защита от path traversal и XSS.
package websec

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// SecurityHeaders выставляет защитные заголовки до вызова next.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		// HSTS имеет смысл только в ответе по HTTPS: по HTTP браузер его игнорирует,
		// а злоумышленник посередине всё равно может его вырезать.
		if r.TLS != nil {
			h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// SetSessionCookie ставит сессионную cookie с безопасными атрибутами.
func SetSessionCookie(w http.ResponseWriter, value string, maxAge time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		MaxAge:   int(maxAge / time.Second),
		HttpOnly: true, // недоступна из JS → не украсть через XSS
		Secure:   true, // только по HTTPS
		SameSite: http.SameSiteLaxMode,
	})
}

type csrfKey struct{}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func validToken(s string) bool {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return err == nil && len(b) == 32
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions || m == http.MethodTrace
}

// CSRF — защита double submit cookie.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var cookieToken string
		if c, err := r.Cookie(CSRFCookieName); err == nil && validToken(c.Value) {
			cookieToken = c.Value
		}

		if !safeMethod(r.Method) {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					http.Error(w, "cross-origin request rejected", http.StatusForbidden)
					return
				}
			}
			sent := r.Header.Get(CSRFHeaderName)
			if sent == "" {
				sent = r.PostFormValue(CSRFFormField)
			}
			// Сравнение за постоянное время: обычное == выдаёт по времени,
			// сколько первых символов совпало.
			if cookieToken == "" || subtle.ConstantTimeCompare([]byte(sent), []byte(cookieToken)) != 1 {
				http.Error(w, "invalid CSRF token", http.StatusForbidden)
				return
			}
		}

		token := cookieToken
		if token == "" {
			token = newToken()
			http.SetCookie(w, &http.Cookie{
				Name:     CSRFCookieName,
				Value:    token,
				Path:     "/",
				Secure:   true,
				SameSite: http.SameSiteStrictMode,
				// HttpOnly: false — JS должен прочитать токен и положить в заголовок.
			})
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), csrfKey{}, token)))
	})
}

// CSRFToken возвращает токен текущего запроса (для подстановки в формы).
func CSRFToken(r *http.Request) string {
	s, _ := r.Context().Value(csrfKey{}).(string)
	return s
}

// SafeJoin безопасно соединяет корень и путь от пользователя.
func SafeJoin(root, userPath string) (string, error) {
	if strings.ContainsRune(userPath, 0) {
		return "", ErrUnsafePath
	}
	p := strings.TrimLeft(userPath, "/")
	if p == "" {
		return "", ErrUnsafePath
	}
	p = filepath.FromSlash(p)
	// IsLocal: относительный, не пустой, лексически не выходит за корень
	// (на Windows ещё и не зарезервированное имя вроде NUL).
	if !filepath.IsLocal(p) {
		return "", ErrUnsafePath
	}
	return filepath.Join(root, p), nil
}

// CORS разрешает кросс-доменные запросы с учётными данными только из allowedOrigins.
func CORS(allowedOrigins []string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Ответ зависит от Origin — кэши должны это учитывать.
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		allowed := origin != "" && origin != "null" && slices.Contains(allowedOrigins, origin)
		preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""

		if preflight {
			if !allowed {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Access-Control-Allow-Credentials", "true")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
			h.Set("Access-Control-Allow-Headers", "Content-Type, "+CSRFHeaderName)
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if allowed {
			// Никогда не "*" вместе с credentials и никогда не эхо любого Origin.
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		next.ServeHTTP(w, r)
	})
}

var greetingTmpl = template.Must(template.New("g").Parse(
	`<p>Hello, {{.Name}}!</p><a href="{{.Homepage}}">homepage</a>`))

// Greeting рендерит приветствие; html/template экранирует данные по контексту
// (текст, атрибут, URL) и обезвреживает javascript:-ссылки.
func Greeting(w io.Writer, name, homepage string) error {
	return greetingTmpl.Execute(w, struct{ Name, Homepage string }{name, homepage})
}
