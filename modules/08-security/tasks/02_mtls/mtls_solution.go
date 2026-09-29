//go:build solution

// Package mtls — HTTPS-сервер с взаимной TLS-аутентификацией (mTLS)
// и авторизацией по идентичности из клиентского сертификата.
package mtls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"slices"
)

// secureCipherSuites — только ECDHE (forward secrecy) + AEAD.
// Для TLS 1.3 набор шифров не настраивается — он всегда безопасен.
var secureCipherSuites = []uint16{
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
}

// ServerTLSConfig — конфигурация сервера, требующего клиентский сертификат.
func ServerTLSConfig(cert tls.Certificate, clientCAs *x509.CertPool) *tls.Config {
	return &tls.Config{
		MinVersion:       tls.VersionTLS12,
		Certificates:     []tls.Certificate{cert},
		ClientAuth:       tls.RequireAndVerifyClientCert,
		ClientCAs:        clientCAs,
		CipherSuites:     secureCipherSuites,
		CurvePreferences: []tls.CurveID{tls.X25519, tls.CurveP256},
	}
}

// ClientTLSConfig — конфигурация клиента: доверяет только roots,
// проверяет имя сервера serverName и (опционально) предъявляет свой сертификат.
func ClientTLSConfig(clientCert *tls.Certificate, roots *x509.CertPool, serverName string) *tls.Config {
	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    roots,
		ServerName: serverName,
	}
	if clientCert != nil {
		cfg.Certificates = []tls.Certificate{*clientCert}
	}
	return cfg
}

// PeerIdentity извлекает идентичность из проверенного сертификата клиента.
func PeerIdentity(state *tls.ConnectionState) (Identity, bool) {
	// PeerCertificates заполняется даже без проверки (ClientAuth: RequestClientCert),
	// поэтому доверяем только наличию проверенной цепочки.
	if state == nil || len(state.VerifiedChains) == 0 || len(state.VerifiedChains[0]) == 0 {
		return Identity{}, false
	}
	leaf := state.VerifiedChains[0][0]
	id := Identity{CommonName: leaf.Subject.CommonName, DNSNames: slices.Clone(leaf.DNSNames)}
	for _, u := range leaf.URIs {
		id.URIs = append(id.URIs, u.String())
	}
	return id, true
}

type ctxKey struct{}

// IdentityFromContext возвращает идентичность, положенную RequireIdentity.
func IdentityFromContext(ctx context.Context) (Identity, bool) {
	id, ok := ctx.Value(ctxKey{}).(Identity)
	return id, ok
}

// RequireIdentity пропускает только клиентов, одно из имён которых входит в allowed.
func RequireIdentity(allowed []string, next http.Handler) http.Handler {
	allow := make(map[string]bool, len(allowed))
	for _, a := range allowed {
		allow[a] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := PeerIdentity(r.TLS)
		if !ok {
			http.Error(w, "client certificate required", http.StatusUnauthorized)
			return
		}
		if !slices.ContainsFunc(id.Names(), func(n string) bool { return allow[n] }) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}
