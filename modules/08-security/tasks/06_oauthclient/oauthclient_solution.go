//go:build solution

// Package oauthclient — клиент OAuth 2.0: authorization code + PKCE (S256),
// client credentials, refresh token.
package oauthclient

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func randomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewVerifier создаёт PKCE code_verifier: 32 случайных байта → 43 символа
// из алфавита [A-Za-z0-9-_] (RFC 7636 §4.1 допускает 43..128).
func NewVerifier() (string, error) { return randomString(32) }

// NewState создаёт непредсказуемый state для защиты от CSRF.
func NewState() (string, error) { return randomString(16) }

// ChallengeS256 — code_challenge = BASE64URL(SHA256(ASCII(code_verifier))).
func ChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthCodeURL строит ссылку на authorization endpoint.
func (c *Config) AuthCodeURL(state, challenge string) (string, error) {
	u, err := url.Parse(c.AuthURL)
	if err != nil {
		return "", err
	}
	q := u.Query() // сохраняем параметры, уже бывшие в AuthURL
	q.Set("response_type", "code")
	q.Set("client_id", c.ClientID)
	if c.RedirectURL != "" {
		q.Set("redirect_uri", c.RedirectURL)
	}
	if len(c.Scopes) > 0 {
		q.Set("scope", strings.Join(c.Scopes, " "))
	}
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// HandleCallback разбирает запрос на redirect_uri и возвращает code.
func (c *Config) HandleCallback(r *http.Request, expectedState string) (string, error) {
	q := r.URL.Query()
	got := q.Get("state")
	// Пустой ожидаемый state — ошибка программиста, а не повод принять "" == "".
	if expectedState == "" || subtle.ConstantTimeCompare([]byte(got), []byte(expectedState)) != 1 {
		return "", ErrStateMismatch
	}
	if e := q.Get("error"); e != "" {
		return "", &AuthError{Code: e, Description: q.Get("error_description")}
	}
	code := q.Get("code")
	if code == "" {
		return "", ErrMissingCode
	}
	return code, nil
}

// Exchange меняет authorization code на токен, предъявляя code_verifier.
func (c *Config) Exchange(ctx context.Context, code, verifier string) (*Token, error) {
	v := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
	}
	if c.RedirectURL != "" {
		v.Set("redirect_uri", c.RedirectURL) // обязан совпасть с тем, что был в /authorize
	}
	return c.tokenRequest(ctx, v)
}

// ClientCredentials получает токен от имени самого клиента (machine-to-machine).
func (c *Config) ClientCredentials(ctx context.Context) (*Token, error) {
	v := url.Values{"grant_type": {"client_credentials"}}
	if len(c.Scopes) > 0 {
		v.Set("scope", strings.Join(c.Scopes, " "))
	}
	return c.tokenRequest(ctx, v)
}

// Refresh обновляет access token. Если сервер не выдал новый refresh token,
// продолжаем использовать старый.
func (c *Config) Refresh(ctx context.Context, refreshToken string) (*Token, error) {
	tok, err := c.tokenRequest(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
	})
	if err != nil {
		return nil, err
	}
	if tok.RefreshToken == "" {
		tok.RefreshToken = refreshToken
	}
	return tok, nil
}

func (c *Config) tokenRequest(ctx context.Context, v url.Values) (*Token, error) {
	if c.ClientSecret == "" {
		v.Set("client_id", c.ClientID) // публичный клиент идентифицирует себя в теле
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.TokenURL, strings.NewReader(v.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if c.ClientSecret != "" {
		// RFC 6749 §2.3.1: id и secret сначала form-urlencode, затем Basic.
		req.SetBasicAuth(url.QueryEscape(c.ClientID), url.QueryEscape(c.ClientSecret))
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("oauth2: token request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("oauth2: read token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		te := &TokenError{}
		_ = json.Unmarshal(body, te) // тело может быть и не JSON
		te.StatusCode = resp.StatusCode
		return nil, te
	}
	var tok Token
	if err := json.Unmarshal(body, &tok); err != nil {
		return nil, fmt.Errorf("oauth2: decode token response: %w", err)
	}
	if tok.AccessToken == "" {
		return nil, fmt.Errorf("oauth2: server response missing access_token")
	}
	if !strings.EqualFold(tok.TokenType, "Bearer") {
		return nil, fmt.Errorf("%w: %q", ErrBadTokenType, tok.TokenType)
	}
	if tok.ExpiresIn > 0 {
		now := time.Now
		if c.Now != nil {
			now = c.Now
		}
		tok.Expiry = now().Add(time.Duration(tok.ExpiresIn) * time.Second)
	}
	return &tok, nil
}
