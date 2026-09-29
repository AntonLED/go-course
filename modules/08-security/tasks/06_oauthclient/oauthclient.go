//go:build !solution

// Package oauthclient — клиент OAuth 2.0: authorization code + PKCE (S256),
// client credentials, refresh token.
package oauthclient

import (
	"context"
	"net/http"
)

// NewVerifier создаёт PKCE code_verifier: 32 байта из crypto/rand в
// base64.RawURLEncoding (43 символа).
func NewVerifier() (string, error) {
	// TODO: реализуйте
	return "", nil
}

// NewState создаёт непредсказуемый state (16 байт из crypto/rand, base64url).
func NewState() (string, error) {
	// TODO: реализуйте
	return "", nil
}

// ChallengeS256 возвращает BASE64URL-NOPAD(SHA256(verifier)).
// Тест-вектор RFC 7636, приложение B:
// "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk" → "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM".
func ChallengeS256(verifier string) string {
	// TODO: реализуйте
	return verifier
}

// AuthCodeURL строит URL authorization endpoint: к query из AuthURL (сохранить!)
// добавить response_type=code, client_id, redirect_uri (если задан),
// scope (Scopes через пробел, если есть), state, code_challenge, code_challenge_method=S256.
func (c *Config) AuthCodeURL(state, challenge string) (string, error) {
	// TODO: реализуйте (url.Parse, u.Query(), q.Set, q.Encode())
	return c.AuthURL, nil
}

// HandleCallback разбирает запрос на redirect_uri:
//   - state не совпал с expectedState (сравнение за постоянное время; пустой
//     expectedState — всегда ошибка) → ErrStateMismatch;
//   - есть параметр error → *AuthError{Code, Description};
//   - нет code → ErrMissingCode.
func (c *Config) HandleCallback(r *http.Request, expectedState string) (string, error) {
	// TODO: реализуйте
	return r.URL.Query().Get("code"), nil
}

// Exchange: POST TokenURL, form: grant_type=authorization_code, code,
// redirect_uri (если задан), code_verifier.
//
// Общие правила для всех запросов к token endpoint:
//   - Content-Type: application/x-www-form-urlencoded, Accept: application/json;
//   - конфиденциальный клиент (ClientSecret != "") — HTTP Basic, где id и secret
//     предварительно пропущены через url.QueryEscape (RFC 6749 §2.3.1);
//     публичный — client_id в теле формы;
//   - не 200 → *TokenError (StatusCode + поля error/error_description, если JSON);
//   - пустой access_token → ошибка; token_type не "Bearer" (без учёта регистра) →
//     ошибка ErrBadTokenType; Expiry = Now() + ExpiresIn секунд (если ExpiresIn > 0).
func (c *Config) Exchange(ctx context.Context, code, verifier string) (*Token, error) {
	// TODO: реализуйте
	return nil, nil
}

// ClientCredentials: grant_type=client_credentials, scope (если есть).
func (c *Config) ClientCredentials(ctx context.Context) (*Token, error) {
	// TODO: реализуйте
	return nil, nil
}

// Refresh: grant_type=refresh_token, refresh_token. Если в ответе нет нового
// refresh_token — вернуть в Token старый.
func (c *Config) Refresh(ctx context.Context, refreshToken string) (*Token, error) {
	// TODO: реализуйте
	return nil, nil
}
