package jwt

import (
	"encoding/json"
	"errors"
)

// Header — заголовок JWS.
type Header struct {
	Alg string `json:"alg"`
	Typ string `json:"typ,omitempty"`
	Kid string `json:"kid,omitempty"`
}

// Audience — claim "aud": по RFC 7519 это строка или массив строк.
type Audience []string

// UnmarshalJSON принимает и "aud": "x", и "aud": ["x", "y"].
func (a *Audience) UnmarshalJSON(b []byte) error {
	var one string
	if err := json.Unmarshal(b, &one); err == nil {
		*a = Audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

// MarshalJSON пишет одну аудиторию строкой, несколько — массивом.
func (a Audience) MarshalJSON() ([]byte, error) {
	if len(a) == 1 {
		return json.Marshal(a[0])
	}
	return json.Marshal([]string(a))
}

// Claims — зарегистрированные claims (RFC 7519 §4.1) и пара приватных.
// Время — NumericDate: секунды Unix; 0 означает «не задано».
type Claims struct {
	Issuer    string   `json:"iss,omitempty"`
	Subject   string   `json:"sub,omitempty"`
	Audience  Audience `json:"aud,omitempty"`
	ExpiresAt int64    `json:"exp,omitempty"`
	NotBefore int64    `json:"nbf,omitempty"`
	IssuedAt  int64    `json:"iat,omitempty"`
	ID        string   `json:"jti,omitempty"`
	Scope     string   `json:"scope,omitempty"` // через пробел, как в OAuth 2.0
	Roles     []string `json:"roles,omitempty"`
}

var (
	ErrMalformed      = errors.New("jwt: malformed token")
	ErrUnsupportedAlg = errors.New("jwt: unsupported algorithm")
	ErrUnknownKey     = errors.New("jwt: unknown key id for algorithm")
	ErrSignature      = errors.New("jwt: invalid signature")
	ErrMissingExp     = errors.New("jwt: missing exp claim")
	ErrExpired        = errors.New("jwt: token expired")
	ErrNotYetValid    = errors.New("jwt: token not valid yet")
	ErrIssuer         = errors.New("jwt: invalid issuer")
	ErrAudience       = errors.New("jwt: invalid audience")
)
