//go:build solution

// Package jwt — выпуск и проверка JWT (HS256, RS256, ES256) на стандартной
// библиотеке, с защитой от alg=none и подмены алгоритма, ротацией ключей по kid и JWKS.
package jwt

import (
	"crypto"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strings"
	"time"
)

var b64 = base64.RawURLEncoding // base64url без паддинга (RFC 7515 §2)

// signingInput = base64url(header) + "." + base64url(payload).
func signingInput(alg, kid string, c Claims) (string, error) {
	h, err := json.Marshal(Header{Alg: alg, Typ: "JWT", Kid: kid})
	if err != nil {
		return "", err
	}
	p, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return b64.EncodeToString(h) + "." + b64.EncodeToString(p), nil
}

// SignHS256 подписывает claims общим секретом (HMAC-SHA256).
func SignHS256(c Claims, kid string, key []byte) (string, error) {
	si, err := signingInput("HS256", kid, c)
	if err != nil {
		return "", err
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(si))
	return si + "." + b64.EncodeToString(m.Sum(nil)), nil
}

// SignRS256 подписывает claims ключом RSA (RSASSA-PKCS1-v1_5 + SHA-256).
func SignRS256(c Claims, kid string, key *rsa.PrivateKey) (string, error) {
	si, err := signingInput("RS256", kid, c)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(si))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return si + "." + b64.EncodeToString(sig), nil
}

// SignES256 подписывает claims ключом ECDSA P-256. Подпись — R||S по 32 байта
// (RFC 7518 §3.4), а НЕ ASN.1 DER, который возвращает ecdsa.SignASN1.
func SignES256(c Claims, kid string, key *ecdsa.PrivateKey) (string, error) {
	si, err := signingInput("ES256", kid, c)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(si))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return si + "." + b64.EncodeToString(sig), nil
}

// Verifier проверяет токены. Ключи хранятся раздельно по семействам
// алгоритмов: ключ ищется по паре (alg, kid), поэтому RSA-ключ никогда не
// будет использован как HMAC-секрет (атака подмены алгоритма).
type Verifier struct {
	HMACKeys map[string][]byte
	RSAKeys  map[string]*rsa.PublicKey
	ECKeys   map[string]*ecdsa.PublicKey

	Issuer   string        // если не пусто — iss обязан совпасть
	Audience string        // если не пусто — должен входить в aud
	Leeway   time.Duration // допуск рассинхронизации часов
	Now      func() time.Time
}

// Verify проверяет подпись и claims и возвращает claims.
func (v *Verifier) Verify(token string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("%w: expected 3 parts, got %d", ErrMalformed, len(parts))
	}
	hb, err := b64.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: header: %v", ErrMalformed, err)
	}
	var h Header
	if err := json.Unmarshal(hb, &h); err != nil {
		return nil, fmt.Errorf("%w: header: %v", ErrMalformed, err)
	}
	// RFC 7515 §4.1.11: если в "crit" перечислены расширения, которые мы не
	// понимаем, токен недействителен. Мы не поддерживаем ни одного расширения,
	// поэтому любой "crit" отвергаем.
	var rawHeader map[string]json.RawMessage
	if err := json.Unmarshal(hb, &rawHeader); err != nil {
		return nil, fmt.Errorf("%w: header: %v", ErrMalformed, err)
	}
	if _, ok := rawHeader["crit"]; ok {
		return nil, fmt.Errorf("%w: unsupported critical header parameters", ErrMalformed)
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return nil, fmt.Errorf("%w: signature: %v", ErrMalformed, err)
	}
	signed := []byte(parts[0] + "." + parts[1])

	// Алгоритм берём из заголовка, но ключ — только из «своего» хранилища.
	switch h.Alg {
	case "HS256":
		key, ok := v.HMACKeys[h.Kid]
		if !ok {
			return nil, fmt.Errorf("%w: HS256 kid %q", ErrUnknownKey, h.Kid)
		}
		m := hmac.New(sha256.New, key)
		m.Write(signed)
		if !hmac.Equal(sig, m.Sum(nil)) { // сравнение за постоянное время
			return nil, ErrSignature
		}
	case "RS256":
		key, ok := v.RSAKeys[h.Kid]
		if !ok {
			return nil, fmt.Errorf("%w: RS256 kid %q", ErrUnknownKey, h.Kid)
		}
		sum := sha256.Sum256(signed)
		if rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig) != nil {
			return nil, ErrSignature
		}
	case "ES256":
		key, ok := v.ECKeys[h.Kid]
		if !ok {
			return nil, fmt.Errorf("%w: ES256 kid %q", ErrUnknownKey, h.Kid)
		}
		if len(sig) != 64 {
			return nil, ErrSignature
		}
		sum := sha256.Sum256(signed)
		r := new(big.Int).SetBytes(sig[:32])
		s := new(big.Int).SetBytes(sig[32:])
		if !ecdsa.Verify(key, sum[:], r, s) {
			return nil, ErrSignature
		}
	default: // "none", "None", "HS512", ... — всё, что мы явно не поддерживаем
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedAlg, h.Alg)
	}

	// Payload разбираем только после проверки подписи.
	pb, err := b64.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("%w: payload: %v", ErrMalformed, err)
	}
	var c Claims
	if err := json.Unmarshal(pb, &c); err != nil {
		return nil, fmt.Errorf("%w: payload: %v", ErrMalformed, err)
	}
	if err := v.validate(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func (v *Verifier) validate(c *Claims) error {
	now := time.Now()
	if v.Now != nil {
		now = v.Now()
	}
	if c.ExpiresAt == 0 {
		return ErrMissingExp
	}
	// exp: текущее время должно быть строго раньше exp (+ leeway).
	if !now.Before(time.Unix(c.ExpiresAt, 0).Add(v.Leeway)) {
		return ErrExpired
	}
	if c.NotBefore != 0 && now.Before(time.Unix(c.NotBefore, 0).Add(-v.Leeway)) {
		return ErrNotYetValid
	}
	if v.Issuer != "" && c.Issuer != v.Issuer {
		return ErrIssuer
	}
	if v.Audience != "" && !slices.Contains(c.Audience, v.Audience) {
		return ErrAudience
	}
	return nil
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

// AddJWKS добавляет открытые ключи из JWK Set (RFC 7517). Ключи с use != "sig"
// (если use задан) и неизвестных типов пропускаются.
func (v *Verifier) AddJWKS(data []byte) error {
	var set struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.Unmarshal(data, &set); err != nil {
		return fmt.Errorf("jwks: %w", err)
	}
	for _, k := range set.Keys {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		if k.Kid == "" {
			return errors.New("jwks: key without kid")
		}
		switch k.Kty {
		case "RSA":
			pub, err := parseRSA(k)
			if err != nil {
				return fmt.Errorf("jwks: kid %q: %w", k.Kid, err)
			}
			if v.RSAKeys == nil {
				v.RSAKeys = map[string]*rsa.PublicKey{}
			}
			v.RSAKeys[k.Kid] = pub
		case "EC":
			pub, err := parseEC(k)
			if err != nil {
				return fmt.Errorf("jwks: kid %q: %w", k.Kid, err)
			}
			if v.ECKeys == nil {
				v.ECKeys = map[string]*ecdsa.PublicKey{}
			}
			v.ECKeys[k.Kid] = pub
		}
	}
	return nil
}

func parseRSA(k jwk) (*rsa.PublicKey, error) {
	nb, err := b64.DecodeString(k.N)
	if err != nil {
		return nil, err
	}
	eb, err := b64.DecodeString(k.E)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(nb)
	e := new(big.Int).SetBytes(eb)
	if n.BitLen() < 2048 {
		return nil, errors.New("RSA key shorter than 2048 bits")
	}
	if !e.IsInt64() || e.Int64() < 3 || e.Int64() > 1<<31-1 || e.Bit(0) == 0 {
		return nil, errors.New("invalid RSA exponent")
	}
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

func parseEC(k jwk) (*ecdsa.PublicKey, error) {
	if k.Crv != "P-256" {
		return nil, fmt.Errorf("unsupported curve %q", k.Crv)
	}
	x, err := b64.DecodeString(k.X)
	if err != nil {
		return nil, err
	}
	y, err := b64.DecodeString(k.Y)
	if err != nil {
		return nil, err
	}
	if len(x) != 32 || len(y) != 32 {
		return nil, errors.New("invalid P-256 coordinates length")
	}
	// Проверяем, что точка лежит на кривой (иначе — invalid curve attack).
	raw := append(append([]byte{4}, x...), y...)
	if _, err := ecdh.P256().NewPublicKey(raw); err != nil {
		return nil, fmt.Errorf("point is not on P-256: %w", err)
	}
	return &ecdsa.PublicKey{Curve: elliptic.P256(), X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}, nil
}
