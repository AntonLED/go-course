//go:build !solution

// Package secureapi — защищённый API: mTLS между сервисами, JWT-авторизация
// по scopes и ролям, rate limiting и корректные коды 401/403/429.
package secureapi

import (
	"crypto/tls"
	"net/http"
)

// ServerTLSConfig собирает конфигурацию сервера из PEM: собственный сертификат
// и ключ (tls.X509KeyPair) + CA для проверки клиентских сертификатов.
// MinVersion ≥ TLS 1.2, клиентский сертификат обязателен и проверяется.
// Битые PEM или отсутствие сертификатов CA → ошибка.
func ServerTLSConfig(certPEM, keyPEM, clientCAPEM []byte) (*tls.Config, error) {
	// TODO: реализуйте
	return &tls.Config{}, nil
}

// NewAPI собирает HTTP API.
//
//	GET    /healthz          → 200 "ok" (без проверок)
//	GET    /v1/items         → scope items:read   → 200 [Item...] (пустой — [])
//	POST   /v1/items {name}  → scope items:write  → 201 Item (ID "1", "2", ...); плохое тело → 400
//	DELETE /v1/items/{id}    → роль admin         → 204 | 404
//
// Конвейер проверок для /v1/* (строго в этом порядке):
//  1. mTLS: идентичность сервиса из проверенного сертификата (r.TLS.VerifiedChains):
//     SPIFFE URI SAN, иначе CN. Нет → 401; не в AllowedServices → 403.
//  2. JWT из "Authorization: Bearer <token>": только HS256, ключ по kid из JWTKeys,
//     exp обязателен, nbf, iss == Issuer, Audience ∈ aud (строка или массив),
//     sub не пуст; учитывать Leeway.
//     Нет заголовка → 401 + WWW-Authenticate: Bearer realm="api" (без error);
//     любой невалидный токен → 401 + WWW-Authenticate: Bearer realm="api", error="invalid_token".
//  3. Rate limit по sub: token bucket (Burst, RatePerSecond, часы cfg.Now) →
//     429 + Retry-After (целые секунды до следующего токена, минимум 1).
//  4. Права: нет scope → 403 + WWW-Authenticate: Bearer realm="api", error="insufficient_scope", scope="<нужный>";
//     нет роли → 403.
//
// Все ошибки — JSON {"error": "..."}.
func NewAPI(cfg Config) http.Handler {
	// TODO: реализуйте
	return http.NotFoundHandler()
}
