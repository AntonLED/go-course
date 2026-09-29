//go:build solution

package authcors

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// BasicAuth проверяет логин/пароль из заголовка Authorization.
func BasicAuth(realm string, users map[string]string) func(http.Handler) http.Handler {
	// Хэшируем заранее: сравнение хэшей фиксированной длины через ConstantTimeCompare
	// не выдаёт длину пароля и не зависит от позиции первого несовпадения.
	hashed := make(map[string][32]byte, len(users))
	for u, p := range users {
		hashed[u] = sha256.Sum256([]byte(p))
	}
	challenge := `Basic realm="` + realm + `", charset="UTF-8"`
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, pass, ok := r.BasicAuth()
			if ok {
				want, known := hashed[user]
				got := sha256.Sum256([]byte(pass))
				// Сравниваем всегда, даже для неизвестного пользователя — меньше утечек по времени.
				match := subtle.ConstantTimeCompare(got[:], want[:]) == 1
				if known && match {
					ctx := context.WithValue(r.Context(), userKey{}, user)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
			w.Header().Set("WWW-Authenticate", challenge)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}
}

// UserFrom возвращает имя аутентифицированного пользователя.
func UserFrom(ctx context.Context) (string, bool) {
	u, ok := ctx.Value(userKey{}).(string)
	return u, ok
}

// CORS реализует простые и preflight-запросы.
func CORS(cfg CORSConfig) func(http.Handler) http.Handler {
	anyOrigin := slices.Contains(cfg.AllowedOrigins, "*")
	methods := make([]string, len(cfg.AllowedMethods))
	for i, m := range cfg.AllowedMethods {
		methods[i] = strings.ToUpper(m)
	}
	allowedHeaders := make(map[string]bool, len(cfg.AllowedHeaders))
	for _, h := range cfg.AllowedHeaders {
		allowedHeaders[http.CanonicalHeaderKey(strings.TrimSpace(h))] = true
	}
	originAllowed := func(o string) bool {
		return anyOrigin || slices.Contains(cfg.AllowedOrigins, o)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Ответ зависит от Origin — кэши (CDN, браузер) должны это учитывать.
			w.Header().Add("Vary", "Origin")
			origin := r.Header.Get("Origin")
			preflight := r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != ""

			if origin == "" {
				next.ServeHTTP(w, r) // не CORS-запрос
				return
			}
			if !originAllowed(origin) {
				if preflight {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				// Простой запрос выполняем, но без CORS-заголовков — браузер сам не отдаст ответ JS.
				next.ServeHTTP(w, r)
				return
			}

			h := w.Header()
			if anyOrigin && !cfg.AllowCredentials {
				h.Set("Access-Control-Allow-Origin", "*")
			} else {
				h.Set("Access-Control-Allow-Origin", origin)
			}
			if cfg.AllowCredentials {
				h.Set("Access-Control-Allow-Credentials", "true")
			}
			if !preflight {
				next.ServeHTTP(w, r)
				return
			}

			// Preflight.
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			reqMethod := strings.ToUpper(r.Header.Get("Access-Control-Request-Method"))
			if !slices.Contains(methods, reqMethod) {
				forbid(w)
				return
			}
			if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
				for _, name := range strings.Split(reqHeaders, ",") {
					name = strings.TrimSpace(name)
					if name != "" && !allowedHeaders[http.CanonicalHeaderKey(name)] {
						forbid(w)
						return
					}
				}
			}
			h.Set("Access-Control-Allow-Methods", strings.Join(methods, ", "))
			if len(cfg.AllowedHeaders) > 0 {
				h.Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))
			}
			if cfg.MaxAge > 0 {
				h.Set("Access-Control-Max-Age", strconv.Itoa(int(cfg.MaxAge.Seconds())))
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// forbid отвечает 403 без CORS-заголовков.
func forbid(w http.ResponseWriter) {
	h := w.Header()
	h.Del("Access-Control-Allow-Origin")
	h.Del("Access-Control-Allow-Credentials")
	w.WriteHeader(http.StatusForbidden)
}
