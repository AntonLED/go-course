package oauthclient

import (
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Config — настройки OAuth 2.0 клиента (по мотивам golang.org/x/oauth2.Config).
type Config struct {
	ClientID     string
	ClientSecret string // пусто — публичный клиент (SPA, мобильное приложение, CLI)
	AuthURL      string // authorization endpoint
	TokenURL     string // token endpoint
	RedirectURL  string
	Scopes       []string
	HTTPClient   *http.Client     // nil — http.DefaultClient
	Now          func() time.Time // nil — time.Now
}

// Token — ответ token endpoint (RFC 6749 §5.1).
type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresIn    int64     `json:"expires_in,omitempty"`
	Scope        string    `json:"scope,omitempty"`
	IDToken      string    `json:"id_token,omitempty"` // OpenID Connect
	Expiry       time.Time `json:"-"`                  // Now + ExpiresIn; нулевое, если ExpiresIn == 0
}

// TokenError — ошибка token endpoint (RFC 6749 §5.2), например invalid_grant.
type TokenError struct {
	StatusCode  int
	Code        string `json:"error"`
	Description string `json:"error_description"`
}

func (e *TokenError) Error() string {
	return fmt.Sprintf("oauth2: token endpoint: HTTP %d: %s %s", e.StatusCode, e.Code, e.Description)
}

// AuthError — ошибка, пришедшая на redirect_uri (?error=access_denied&...).
type AuthError struct {
	Code        string
	Description string
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("oauth2: authorization error: %s %s", e.Code, e.Description)
}

var (
	ErrStateMismatch = errors.New("oauth2: state mismatch")
	ErrMissingCode   = errors.New("oauth2: missing code in callback")
	ErrBadTokenType  = errors.New("oauth2: unsupported token type")
)
