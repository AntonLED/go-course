//go:build solution

// Package authserver — минимальный OAuth 2.0 authorization server:
// authorization code + PKCE (S256), client credentials, introspection.
package authserver

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

type authCode struct {
	clientID    string
	redirectURI string // redirect_uri из /authorize как был передан
	challenge   string
	scope       string
	user        string
	expires     time.Time
	used        bool
	tokens      []string // токены, выданные по этому коду (для отзыва при повторе)
}

type accessToken struct {
	sub, clientID, scope string
	expires              time.Time
	revoked              bool
}

// Server — authorization server; реализует http.Handler.
type Server struct {
	clients map[string]Client
	opts    Options
	mux     *http.ServeMux

	mu     sync.Mutex
	codes  map[string]*authCode
	tokens map[string]*accessToken
}

// New создаёт сервер с зарегистрированными клиентами.
func New(clients []Client, opts Options) *Server {
	if opts.CodeTTL <= 0 {
		opts.CodeTTL = time.Minute
	}
	if opts.TokenTTL <= 0 {
		opts.TokenTTL = time.Hour
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	s := &Server{
		clients: map[string]Client{},
		opts:    opts,
		mux:     http.NewServeMux(),
		codes:   map[string]*authCode{},
		tokens:  map[string]*accessToken{},
	}
	for _, c := range clients {
		s.clients[c.ID] = c
	}
	s.mux.HandleFunc("GET /authorize", s.authorize)
	s.mux.HandleFunc("POST /token", s.token)
	s.mux.HandleFunc("POST /introspect", s.introspect)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func challengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func eq(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// grantedScope проверяет запрошенные scopes; пустой запрос — все разрешённые.
func grantedScope(c Client, requested string) (string, bool) {
	if requested == "" {
		return strings.Join(c.Scopes, " "), true
	}
	for _, sc := range strings.Fields(requested) {
		if !slices.Contains(c.Scopes, sc) {
			return "", false
		}
	}
	return strings.Join(strings.Fields(requested), " "), true
}

func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	client, ok := s.clients[q.Get("client_id")]
	if !ok {
		// Неизвестному клиенту нельзя делать редирект: redirect_uri не проверен.
		http.Error(w, "unknown client_id", http.StatusBadRequest)
		return
	}
	redirectParam := q.Get("redirect_uri") // как передан (может быть пустым)
	redirectURI := redirectParam
	switch {
	case redirectURI == "" && len(client.RedirectURIs) == 1:
		redirectURI = client.RedirectURIs[0]
	case !slices.Contains(client.RedirectURIs, redirectURI): // только точное совпадение
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}
	target, err := url.Parse(redirectURI)
	if err != nil {
		http.Error(w, "invalid redirect_uri", http.StatusBadRequest)
		return
	}
	state := q.Get("state")
	redirect := func(params url.Values) {
		tq := target.Query()
		for k, v := range params {
			tq[k] = v
		}
		if state != "" {
			tq.Set("state", state)
		}
		target.RawQuery = tq.Encode()
		http.Redirect(w, r, target.String(), http.StatusFound)
	}
	fail := func(code, desc string) {
		redirect(url.Values{"error": {code}, "error_description": {desc}})
	}

	if q.Get("response_type") != "code" {
		fail("unsupported_response_type", "only response_type=code is supported")
		return
	}
	challenge := q.Get("code_challenge")
	if q.Get("code_challenge_method") != "S256" || len(challenge) != 43 {
		fail("invalid_request", "PKCE with code_challenge_method=S256 is required")
		return
	}
	scope, ok := grantedScope(client, q.Get("scope"))
	if !ok {
		fail("invalid_scope", "requested scope is not allowed")
		return
	}
	user := r.Header.Get(UserHeader)
	if user == "" {
		fail("access_denied", "user did not authorize the request")
		return
	}

	code := randomToken()
	s.mu.Lock()
	s.codes[code] = &authCode{
		clientID: client.ID, redirectURI: redirectParam, challenge: challenge,
		scope: scope, user: user, expires: s.opts.Now().Add(s.opts.CodeTTL),
	}
	s.mu.Unlock()
	redirect(url.Values{"code": {code}})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store") // токены не кэшируются
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func oauthError(w http.ResponseWriter, code string) {
	status := http.StatusBadRequest
	if code == "invalid_client" {
		w.Header().Set("WWW-Authenticate", `Basic realm="token"`)
		status = http.StatusUnauthorized
	}
	writeJSON(w, status, map[string]string{"error": code})
}

// authenticateClient: Basic для конфиденциальных, client_id в форме — для публичных.
func (s *Server) authenticateClient(r *http.Request) (Client, bool) {
	if id, secret, ok := r.BasicAuth(); ok {
		id, err1 := url.QueryUnescape(id)
		secret, err2 := url.QueryUnescape(secret)
		c, known := s.clients[id]
		if err1 != nil || err2 != nil || !known || c.Secret == "" || !eq(secret, c.Secret) {
			return Client{}, false
		}
		return c, true
	}
	c, known := s.clients[r.PostForm.Get("client_id")]
	if !known || c.Secret != "" { // конфиденциальный клиент обязан предъявить секрет
		return Client{}, false
	}
	return c, true
}

func (s *Server) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, "invalid_request")
		return
	}
	client, ok := s.authenticateClient(r)
	if !ok {
		oauthError(w, "invalid_client")
		return
	}
	now := s.opts.Now()

	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		s.mu.Lock()
		defer s.mu.Unlock()
		code, ok := s.codes[r.PostForm.Get("code")]
		if !ok {
			oauthError(w, "invalid_grant")
			return
		}
		if code.used {
			// Повторное предъявление кода — признак кражи: отзываем выданное (RFC 6749 §4.1.2).
			for _, t := range code.tokens {
				s.tokens[t].revoked = true
			}
			oauthError(w, "invalid_grant")
			return
		}
		verifier := r.PostForm.Get("code_verifier")
		if !now.Before(code.expires) ||
			code.clientID != client.ID ||
			r.PostForm.Get("redirect_uri") != code.redirectURI ||
			len(verifier) < 43 || len(verifier) > 128 ||
			!eq(challengeS256(verifier), code.challenge) {
			oauthError(w, "invalid_grant")
			return
		}
		code.used = true
		at := randomToken()
		s.tokens[at] = &accessToken{sub: code.user, clientID: client.ID, scope: code.scope, expires: now.Add(s.opts.TokenTTL)}
		code.tokens = append(code.tokens, at)
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": at, "token_type": "Bearer",
			"expires_in": int64(s.opts.TokenTTL / time.Second), "scope": code.scope,
		})

	case "client_credentials":
		if client.Secret == "" {
			oauthError(w, "unauthorized_client")
			return
		}
		scope, ok := grantedScope(client, r.PostForm.Get("scope"))
		if !ok {
			oauthError(w, "invalid_scope")
			return
		}
		at := randomToken()
		s.mu.Lock()
		s.tokens[at] = &accessToken{sub: client.ID, clientID: client.ID, scope: scope, expires: now.Add(s.opts.TokenTTL)}
		s.mu.Unlock()
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token": at, "token_type": "Bearer",
			"expires_in": int64(s.opts.TokenTTL / time.Second), "scope": scope,
		})

	default:
		oauthError(w, "unsupported_grant_type")
	}
}

func (s *Server) introspect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		oauthError(w, "invalid_request")
		return
	}
	// Интроспекция доступна только аутентифицированным конфиденциальным клиентам
	// (resource server'ам), иначе это оракул для перебора токенов.
	if _, _, hasBasic := r.BasicAuth(); !hasBasic {
		oauthError(w, "invalid_client")
		return
	}
	if _, ok := s.authenticateClient(r); !ok {
		oauthError(w, "invalid_client")
		return
	}
	s.mu.Lock()
	t, ok := s.tokens[r.PostForm.Get("token")]
	var resp map[string]any
	if ok && !t.revoked && s.opts.Now().Before(t.expires) {
		resp = map[string]any{
			"active": true, "sub": t.sub, "client_id": t.clientID,
			"scope": t.scope, "exp": t.expires.Unix(), "token_type": "Bearer",
		}
	} else {
		resp = map[string]any{"active": false}
	}
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, resp)
}
