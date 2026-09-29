package authcors

import "time"

// CORSConfig описывает политику CORS.
type CORSConfig struct {
	// AllowedOrigins — список разрешённых Origin (точное совпадение) или "*".
	AllowedOrigins []string
	// AllowedMethods — методы для preflight (например, GET, POST, DELETE).
	AllowedMethods []string
	// AllowedHeaders — разрешённые заголовки запроса (сравнение без учёта регистра).
	AllowedHeaders []string
	// AllowCredentials — разрешить куки/Authorization. С ним "*" в ответе запрещён спецификацией.
	AllowCredentials bool
	// MaxAge — сколько браузер может кэшировать preflight (0 — не отправлять заголовок).
	MaxAge time.Duration
}

type userKey struct{}
