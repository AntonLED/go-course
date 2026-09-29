//go:build !solution

// Package mtls — HTTPS-сервер с взаимной TLS-аутентификацией (mTLS)
// и авторизацией по идентичности из клиентского сертификата.
package mtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
)

// ServerTLSConfig возвращает конфигурацию сервера:
//   - MinVersion не ниже TLS 1.2, серверный сертификат cert;
//   - клиентский сертификат обязателен и проверяется по clientCAs;
//   - CipherSuites (для TLS 1.2) — только ECDHE + AEAD (GCM / ChaCha20-Poly1305);
//   - CurvePreferences — X25519 и P-256.
func ServerTLSConfig(cert tls.Certificate, clientCAs *x509.CertPool) *tls.Config {
	// TODO: реализуйте
	return &tls.Config{}
}

// ClientTLSConfig возвращает конфигурацию клиента: доверять только roots,
// проверять имя сервера serverName, MinVersion TLS 1.2; если clientCert != nil —
// предъявлять его серверу.
func ClientTLSConfig(clientCert *tls.Certificate, roots *x509.CertPool, serverName string) *tls.Config {
	// TODO: реализуйте
	return &tls.Config{}
}

// PeerIdentity извлекает Identity из клиентского сертификата — но только если
// он проверен (state.VerifiedChains не пуст). state == nil → false.
func PeerIdentity(state *tls.ConnectionState) (Identity, bool) {
	// TODO: реализуйте (URI SAN — u.String())
	return Identity{}, false
}

// IdentityFromContext возвращает идентичность, положенную RequireIdentity.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	// TODO: реализуйте
	return Identity{}, false
}

// RequireIdentity — middleware авторизации по сертификату:
//   - нет TLS или нет проверенного клиентского сертификата → 401;
//   - ни CN, ни один DNS/URI SAN не входит в allowed → 403;
//   - иначе кладёт Identity в контекст и вызывает next.
func RequireIdentity(allowed []string, next http.Handler) http.Handler {
	// TODO: реализуйте
	return next
}
