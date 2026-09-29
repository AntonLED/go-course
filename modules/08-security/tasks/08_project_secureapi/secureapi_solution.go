//go:build solution

// Package secureapi — защищённый API: mTLS между сервисами, JWT-авторизация
// по scopes и ролям, rate limiting и корректные коды 401/403/429.
package secureapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ServerTLSConfig собирает конфигурацию сервера из PEM: собственный
// сертификат и ключ + CA, которым подписаны сертификаты клиентов-сервисов.
func ServerTLSConfig(certPEM, keyPEM, clientCAPEM []byte) (*tls.Config, error) {
	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("server key pair: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(clientCAPEM) {
		return nil, errors.New("no client CA certificates in PEM")
	}
	return &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}, nil
}

// ---------------------------------------------------------------- JWT

type claims struct {
	Iss   string          `json:"iss"`
	Sub   string          `json:"sub"`
	Aud   json.RawMessage `json:"aud"`
	Exp   int64           `json:"exp"`
	Nbf   int64           `json:"nbf"`
	Scope string          `json:"scope"`
	Roles []string        `json:"roles"`
}

func (c *claims) audiences() []string {
	var one string
	if json.Unmarshal(c.Aud, &one) == nil {
		return []string{one}
	}
	var many []string
	_ = json.Unmarshal(c.Aud, &many)
	return many
}

var b64 = base64.RawURLEncoding

func (a *api) verifyJWT(token string) (*claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("malformed")
	}
	hb, err := b64.DecodeString(parts[0])
	if err != nil {
		return nil, err
	}
	var h struct{ Alg, Kid string }
	if err := json.Unmarshal(hb, &h); err != nil {
		return nil, err
	}
	if h.Alg != "HS256" { // явный список: никаких none и подмен
		return nil, fmt.Errorf("alg %q not allowed", h.Alg)
	}
	key, ok := a.cfg.JWTKeys[h.Kid]
	if !ok {
		return nil, errors.New("unknown kid")
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return nil, err
	}
	m := hmac.New(sha256.New, key)
	m.Write([]byte(parts[0] + "." + parts[1]))
	if !hmac.Equal(sig, m.Sum(nil)) {
		return nil, errors.New("bad signature")
	}
	pb, err := b64.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var c claims
	if err := json.Unmarshal(pb, &c); err != nil {
		return nil, err
	}
	now := a.now()
	switch {
	case c.Exp == 0 || !now.Before(time.Unix(c.Exp, 0).Add(a.cfg.Leeway)):
		return nil, errors.New("expired")
	case c.Nbf != 0 && now.Before(time.Unix(c.Nbf, 0).Add(-a.cfg.Leeway)):
		return nil, errors.New("not yet valid")
	case c.Iss != a.cfg.Issuer:
		return nil, errors.New("bad issuer")
	case !slices.Contains(c.audiences(), a.cfg.Audience):
		return nil, errors.New("bad audience")
	case c.Sub == "":
		return nil, errors.New("no subject")
	}
	return &c, nil
}

// ---------------------------------------------------------------- rate limit

type bucket struct {
	tokens float64
	last   time.Time
}

type limiter struct {
	mu      sync.Mutex
	rate    float64
	burst   float64
	buckets map[string]*bucket
}

// allow забирает токен из корзины key; при отказе возвращает, через сколько
// появится следующий токен.
func (l *limiter) allow(key string, now time.Time) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	if el := now.Sub(b.last).Seconds(); el > 0 {
		b.tokens = math.Min(l.burst, b.tokens+el*l.rate)
		b.last = now
	}
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	if l.rate <= 0 {
		return false, time.Hour
	}
	return false, time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
}

// ---------------------------------------------------------------- API

type api struct {
	cfg Config
	lim *limiter

	mu     sync.Mutex
	items  []Item
	nextID int
}

func (a *api) now() time.Time {
	if a.cfg.Now != nil {
		return a.cfg.Now()
	}
	return time.Now()
}

type claimsKey struct{}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// serviceIdentity — SPIFFE ID или CN из проверенного клиентского сертификата.
func serviceIdentity(r *http.Request) (string, bool) {
	if r.TLS == nil || len(r.TLS.VerifiedChains) == 0 || len(r.TLS.VerifiedChains[0]) == 0 {
		return "", false
	}
	leaf := r.TLS.VerifiedChains[0][0]
	for _, u := range leaf.URIs {
		if u.Scheme == "spiffe" {
			return u.String(), true
		}
	}
	return leaf.Subject.CommonName, true
}

// protect — конвейер для /v1/*: mTLS → JWT → rate limit → права.
func (a *api) protect(scope, role string, next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 1. Кто звонит (сервис).
		svc, ok := serviceIdentity(r)
		if !ok {
			writeErr(w, http.StatusUnauthorized, "client certificate required")
			return
		}
		if !slices.Contains(a.cfg.AllowedServices, svc) {
			writeErr(w, http.StatusForbidden, "service not allowed")
			return
		}

		// 2. От чьего имени (пользователь/клиент из токена).
		authz := r.Header.Get("Authorization")
		if authz == "" {
			// RFC 6750 §3.1: без учётных данных — без кода ошибки.
			w.Header().Set("WWW-Authenticate", `Bearer realm="api"`)
			writeErr(w, http.StatusUnauthorized, "missing token")
			return
		}
		scheme, token, _ := strings.Cut(authz, " ")
		var c *claims
		var err error
		if !strings.EqualFold(scheme, "Bearer") || token == "" {
			err = errors.New("not a bearer token")
		} else {
			c, err = a.verifyJWT(token)
		}
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="api", error="invalid_token"`)
			writeErr(w, http.StatusUnauthorized, "invalid token")
			return
		}

		// 3. Rate limit по субъекту — после аутентификации, до авторизации.
		if ok, wait := a.lim.allow(c.Sub, a.now()); !ok {
			secs := int(math.Ceil(wait.Seconds()))
			w.Header().Set("Retry-After", strconv.Itoa(max(secs, 1)))
			writeErr(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		// 4. Что ему можно.
		if scope != "" && !slices.Contains(strings.Fields(c.Scope), scope) {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer realm="api", error="insufficient_scope", scope=%q`, scope))
			writeErr(w, http.StatusForbidden, "insufficient scope")
			return
		}
		if role != "" && !slices.Contains(c.Roles, role) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="api", error="insufficient_scope"`)
			writeErr(w, http.StatusForbidden, "insufficient role")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), claimsKey{}, c)))
	})
}

// NewAPI собирает HTTP API.
func NewAPI(cfg Config) http.Handler {
	a := &api{cfg: cfg, lim: &limiter{rate: cfg.RatePerSecond, burst: float64(cfg.Burst), buckets: map[string]*bucket{}}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	mux.Handle("GET /v1/items", a.protect(ScopeRead, "", a.list))
	mux.Handle("POST /v1/items", a.protect(ScopeWrite, "", a.create))
	mux.Handle("DELETE /v1/items/{id}", a.protect("", RoleAdmin, a.delete))
	return mux
}

func (a *api) list(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	items := slices.Clone(a.items)
	a.mu.Unlock()
	if items == nil {
		items = []Item{}
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *api) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&in); err != nil || strings.TrimSpace(in.Name) == "" {
		writeErr(w, http.StatusBadRequest, `body must be {"name": non-empty string}`)
		return
	}
	a.mu.Lock()
	a.nextID++
	it := Item{ID: strconv.Itoa(a.nextID), Name: in.Name}
	a.items = append(a.items, it)
	a.mu.Unlock()
	writeJSON(w, http.StatusCreated, it)
}

func (a *api) delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	a.mu.Lock()
	i := slices.IndexFunc(a.items, func(it Item) bool { return it.ID == id })
	if i >= 0 {
		a.items = slices.Delete(a.items, i, i+1)
	}
	a.mu.Unlock()
	if i < 0 {
		writeErr(w, http.StatusNotFound, "item not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
