package secureapi

import "time"

// Config — настройки защищённого API.
type Config struct {
	// JWTKeys — секреты HS256 по kid (ротация: старый и новый ключи действуют одновременно).
	JWTKeys  map[string][]byte
	Issuer   string // ожидаемый iss
	Audience string // ожидаемая aud (строка или элемент массива aud)
	Leeway   time.Duration

	// AllowedServices — кому (по mTLS) разрешено вызывать /v1/*: SPIFFE ID
	// (URI SAN со схемой spiffe) или, если его нет, CommonName клиентского сертификата.
	AllowedServices []string

	// Rate limiting по subject токена: token bucket ёмкостью Burst,
	// пополняется со скоростью RatePerSecond.
	RatePerSecond float64
	Burst         int

	Now func() time.Time // по умолчанию time.Now
}

// Item — ресурс API.
type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Права доступа.
const (
	ScopeRead  = "items:read"
	ScopeWrite = "items:write"
	RoleAdmin  = "admin"
)
