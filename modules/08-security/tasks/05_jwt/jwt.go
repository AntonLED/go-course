//go:build !solution

// Package jwt — выпуск и проверка JWT (HS256, RS256, ES256) на стандартной
// библиотеке, с защитой от alg=none и подмены алгоритма, ротацией ключей по kid и JWKS.
package jwt

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"time"
)

// Формат: base64url(header) "." base64url(payload) "." base64url(signature),
// base64url — без паддинга (base64.RawURLEncoding). Header: {"alg":..,"typ":"JWT","kid":..}
// (kid опускается, если пуст).

// SignHS256 подписывает claims общим секретом: HMAC-SHA256 над "header.payload".
func SignHS256(c Claims, kid string, key []byte) (string, error) {
	// TODO: реализуйте
	return "", nil
}

// SignRS256 подписывает claims ключом RSA: rsa.SignPKCS1v15 + SHA-256.
func SignRS256(c Claims, kid string, key *rsa.PrivateKey) (string, error) {
	// TODO: реализуйте
	return "", nil
}

// SignES256 подписывает claims ключом ECDSA P-256. Подпись — 64 байта R||S
// (каждое по 32 байта, big-endian, с ведущими нулями), НЕ ASN.1 DER.
func SignES256(c Claims, kid string, key *ecdsa.PrivateKey) (string, error) {
	// TODO: реализуйте
	return "", nil
}

// Verifier проверяет токены. Ключи раздельно по семействам алгоритмов,
// ключ ищется по паре (alg из заголовка, kid из заголовка).
type Verifier struct {
	HMACKeys map[string][]byte
	RSAKeys  map[string]*rsa.PublicKey
	ECKeys   map[string]*ecdsa.PublicKey

	Issuer   string        // если не пусто — iss обязан совпасть
	Audience string        // если не пусто — должен входить в aud
	Leeway   time.Duration // допуск рассинхронизации часов
	Now      func() time.Time
}

// Verify проверяет токен. Порядок и ошибки:
//  1. не 3 части / битый base64url / битый JSON заголовка → ErrMalformed;
//     заголовок содержит "crit" (критические расширения, которые мы не
//     поддерживаем, RFC 7515 §4.1.11) → тоже ErrMalformed;
//  2. alg не из {HS256, RS256, ES256} (в том числе "none") → ErrUnsupportedAlg;
//  3. нет ключа для (alg, kid) → ErrUnknownKey;
//  4. подпись не сходится (для ES256 — ещё и длина != 64) → ErrSignature;
//     HMAC сравнивать через hmac.Equal;
//  5. только теперь разбирать payload (битый → ErrMalformed);
//  6. exp == 0 → ErrMissingExp; now >= exp+Leeway → ErrExpired;
//     nbf != 0 и now < nbf-Leeway → ErrNotYetValid;
//  7. Issuer/Audience (если заданы) → ErrIssuer / ErrAudience.
//
// Now == nil → time.Now. Ошибки можно оборачивать (проверяются через errors.Is).
func (v *Verifier) Verify(token string) (*Claims, error) {
	// TODO: реализуйте
	return nil, ErrMalformed
}

// AddJWKS добавляет открытые ключи из JWK Set: {"keys":[...]}.
//   - kty "RSA": n, e — base64url big-endian; модуль короче 2048 бит → ошибка;
//   - kty "EC": crv "P-256", x и y — по 32 байта; точка должна лежать на кривой
//     (проверьте через ecdh.P256().NewPublicKey(0x04||x||y)) → иначе ошибка;
//   - ключи с use, отличным от "sig", и неизвестные kty пропускаются;
//   - ключ без kid → ошибка.
func (v *Verifier) AddJWKS(data []byte) error {
	// TODO: реализуйте
	return nil
}
