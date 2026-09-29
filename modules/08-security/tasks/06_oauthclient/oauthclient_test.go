package oauthclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestChallengeS256RFC7636(t *testing.T) {
	// RFC 7636, Appendix B
	const verifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const want = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := ChallengeS256(verifier); got != want {
		t.Errorf("ChallengeS256(%q) = %q, ожидалось %q", verifier, got, want)
	}
}

func TestNewVerifierAndState(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		v, err := NewVerifier()
		if err != nil || len(v) != 43 || strings.Trim(v, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_") != "" {
			t.Fatalf("NewVerifier() = %q, %v; ожидалось 43 символа base64url", v, err)
		}
		s, err := NewState()
		if err != nil || len(s) < 22 {
			t.Fatalf("NewState() = %q, %v; ожидалось ≥ 128 бит энтропии", s, err)
		}
		if seen[v] || seen[s] {
			t.Fatal("значения повторяются")
		}
		seen[v], seen[s] = true, true
	}
}

func TestAuthCodeURL(t *testing.T) {
	c := &Config{
		ClientID: "shop-spa", AuthURL: "https://auth.example.com/authorize?prompt=consent",
		RedirectURL: "https://shop.example.com/callback", Scopes: []string{"openid", "orders:read"},
	}
	raw, err := c.AuthCodeURL("st4te", "ch4llenge")
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host != "auth.example.com" || u.Path != "/authorize" {
		t.Fatalf("URL %q", raw)
	}
	q := u.Query()
	want := map[string]string{
		"response_type": "code", "client_id": "shop-spa", "redirect_uri": "https://shop.example.com/callback",
		"scope": "openid orders:read", "state": "st4te", "code_challenge": "ch4llenge",
		"code_challenge_method": "S256", "prompt": "consent",
	}
	for k, v := range want {
		if q.Get(k) != v {
			t.Errorf("параметр %s = %q, ожидалось %q", k, q.Get(k), v)
		}
	}
	if strings.Contains(raw, "orders:read openid") || strings.Contains(raw, " ") {
		t.Errorf("URL не экранирован: %s", raw)
	}
	c.Scopes = nil
	raw, _ = c.AuthCodeURL("s", "c")
	if u, _ := url.Parse(raw); u.Query().Has("scope") {
		t.Error("без Scopes параметр scope не нужен")
	}
}

func TestHandleCallback(t *testing.T) {
	c := &Config{}
	cases := []struct {
		name, query, expected string
		code                  string
		err                   error
	}{
		{"ok", "code=abc&state=s1", "s1", "abc", nil},
		{"чужой state", "code=abc&state=evil", "s1", "", ErrStateMismatch},
		{"нет state", "code=abc", "s1", "", ErrStateMismatch},
		{"пустой ожидаемый state", "code=abc&state=", "", "", ErrStateMismatch},
		{"нет code", "state=s1", "s1", "", ErrMissingCode},
		{"ошибка при чужом state", "error=access_denied&state=evil", "s1", "", ErrStateMismatch},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "https://shop.example.com/callback?"+cs.query, nil)
			code, err := c.HandleCallback(r, cs.expected)
			if code != cs.code || !errors.Is(err, cs.err) {
				t.Errorf("HandleCallback = %q, %v; ожидалось %q, %v", code, err, cs.code, cs.err)
			}
		})
	}
	r := httptest.NewRequest("GET", "/callback?error=access_denied&error_description=User+said+no&state=s1", nil)
	_, err := c.HandleCallback(r, "s1")
	var ae *AuthError
	if !errors.As(err, &ae) || ae.Code != "access_denied" || ae.Description != "User said no" {
		t.Errorf("error в callback: %v, ожидался *AuthError{access_denied, User said no}", err)
	}
}

// fakeAS — authorization server, проверяющий запросы клиента.
type fakeAS struct {
	t      *testing.T
	mu     sync.Mutex
	codes  map[string]string // code → code_challenge
	last   url.Values
	lastID string
	lastPW string
	reply  func(w http.ResponseWriter, form url.Values) bool // true — ответ уже записан
}

func (a *fakeAS) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/token" {
		http.Error(w, "not found", 404)
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
		a.t.Errorf("Content-Type = %q", ct)
	}
	if acc := r.Header.Get("Accept"); acc != "application/json" {
		a.t.Errorf("Accept = %q", acc)
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", 400)
		return
	}
	id, pw, _ := r.BasicAuth()
	a.mu.Lock()
	a.last, a.lastID, a.lastPW = r.PostForm, id, pw
	a.mu.Unlock()
	if r.URL.RawQuery != "" {
		a.t.Errorf("параметры нельзя передавать в query token endpoint: %s", r.URL.RawQuery)
	}
	if a.reply != nil && a.reply(w, r.PostForm) {
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	switch r.PostForm.Get("grant_type") {
	case "authorization_code":
		a.mu.Lock()
		ch, ok := a.codes[r.PostForm.Get("code")]
		delete(a.codes, r.PostForm.Get("code"))
		a.mu.Unlock()
		if !ok || ChallengeS256(r.PostForm.Get("code_verifier")) != ch {
			w.WriteHeader(400)
			io.WriteString(w, `{"error":"invalid_grant","error_description":"bad code or verifier"}`)
			return
		}
		io.WriteString(w, `{"access_token":"at-1","token_type":"Bearer","expires_in":3600,"refresh_token":"rt-1","scope":"orders:read","id_token":"x.y.z"}`)
	case "client_credentials":
		io.WriteString(w, `{"access_token":"svc-token","token_type":"bearer","expires_in":60}`)
	case "refresh_token":
		io.WriteString(w, `{"access_token":"at-2","token_type":"Bearer","expires_in":3600}`)
	default:
		w.WriteHeader(400)
		io.WriteString(w, `{"error":"unsupported_grant_type"}`)
	}
}

func setup(t *testing.T, secret string) (*fakeAS, *Config) {
	as := &fakeAS{t: t, codes: map[string]string{}}
	srv := httptest.NewServer(as)
	t.Cleanup(srv.Close)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	return as, &Config{
		ClientID: "shop client", ClientSecret: secret,
		AuthURL: srv.URL + "/authorize", TokenURL: srv.URL + "/token",
		RedirectURL: "https://shop.example.com/callback", Scopes: []string{"orders:read"},
		HTTPClient: srv.Client(), Now: func() time.Time { return now },
	}
}

func TestAuthorizationCodeFlowWithPKCE(t *testing.T) {
	as, c := setup(t, "")
	verifier, _ := NewVerifier()
	state, _ := NewState()
	raw, err := c.AuthCodeURL(state, ChallengeS256(verifier))
	if err != nil {
		t.Fatal(err)
	}
	// «пользователь» залогинился и согласился: AS запоминает challenge и выдаёт code
	u, _ := url.Parse(raw)
	as.codes["code-123"] = u.Query().Get("code_challenge")
	cb := httptest.NewRequest("GET", c.RedirectURL+"?code=code-123&state="+url.QueryEscape(u.Query().Get("state")), nil)
	code, err := c.HandleCallback(cb, state)
	if err != nil {
		t.Fatalf("HandleCallback: %v", err)
	}

	tok, err := c.Exchange(context.Background(), code, verifier)
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if tok.AccessToken != "at-1" || tok.RefreshToken != "rt-1" || tok.IDToken != "x.y.z" || tok.Scope != "orders:read" {
		t.Errorf("токен %+v", tok)
	}
	if want := c.Now().Add(time.Hour); !tok.Expiry.Equal(want) {
		t.Errorf("Expiry = %v, ожидалось %v", tok.Expiry, want)
	}
	f := as.last
	if f.Get("grant_type") != "authorization_code" || f.Get("code") != "code-123" ||
		f.Get("redirect_uri") != c.RedirectURL || f.Get("code_verifier") != verifier || f.Get("client_id") != "shop client" {
		t.Errorf("форма запроса токена: %v", f)
	}
	if as.lastID != "" {
		t.Error("публичный клиент не должен слать Basic-аутентификацию")
	}

	// повтор того же кода — invalid_grant
	_, err = c.Exchange(context.Background(), code, verifier)
	var te *TokenError
	if !errors.As(err, &te) || te.Code != "invalid_grant" || te.StatusCode != 400 || te.Description != "bad code or verifier" {
		t.Errorf("повторный обмен: %v, ожидался *TokenError invalid_grant", err)
	}
}

func TestExchangeWrongVerifier(t *testing.T) {
	as, c := setup(t, "")
	as.codes["c"] = ChallengeS256("the-right-verifier-the-right-verifier-12345")
	_, err := c.Exchange(context.Background(), "c", "a-stolen-code-without-the-verifier-xxxxxxxx")
	var te *TokenError
	if !errors.As(err, &te) || te.Code != "invalid_grant" {
		t.Errorf("неверный verifier: %v", err)
	}
}

func TestConfidentialClientBasicAuth(t *testing.T) {
	as, c := setup(t, "s3cr:t/+&=")
	tok, err := c.ClientCredentials(context.Background())
	if err != nil {
		t.Fatalf("ClientCredentials: %v", err)
	}
	if tok.AccessToken != "svc-token" || tok.TokenType != "bearer" {
		t.Errorf("токен %+v (token_type сравнивается без учёта регистра)", tok)
	}
	if as.last.Get("grant_type") != "client_credentials" || as.last.Get("scope") != "orders:read" {
		t.Errorf("форма: %v", as.last)
	}
	// RFC 6749 §2.3.1: значения form-urlencoded до Basic
	if as.lastID != "shop+client" || as.lastPW != "s3cr%3At%2F%2B%26%3D" {
		t.Errorf("Basic: id=%q secret=%q; ожидалось url.QueryEscape от исходных", as.lastID, as.lastPW)
	}
	if as.last.Has("client_secret") || as.last.Has("client_id") {
		t.Error("конфиденциальный клиент с Basic не должен дублировать учётные данные в теле")
	}
}

func TestRefreshKeepsOldRefreshToken(t *testing.T) {
	as, c := setup(t, "secret")
	tok, err := c.Refresh(context.Background(), "rt-1")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "at-2" || tok.RefreshToken != "rt-1" {
		t.Errorf("Refresh: %+v; сервер не выдал новый refresh token — должен остаться старый", tok)
	}
	if as.last.Get("grant_type") != "refresh_token" || as.last.Get("refresh_token") != "rt-1" {
		t.Errorf("форма: %v", as.last)
	}
}

func TestTokenResponseValidation(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		check  func(error) bool
	}{
		{"не bearer", 200, `{"access_token":"x","token_type":"mac"}`, func(e error) bool { return errors.Is(e, ErrBadTokenType) }},
		{"нет access_token", 200, `{"token_type":"Bearer"}`, func(e error) bool { return e != nil }},
		{"не JSON", 200, `access_token=x`, func(e error) bool { return e != nil }},
		{"500 не JSON", 500, `oops`, func(e error) bool {
			var te *TokenError
			return errors.As(e, &te) && te.StatusCode == 500
		}},
		{"401 invalid_client", 401, `{"error":"invalid_client"}`, func(e error) bool {
			var te *TokenError
			return errors.As(e, &te) && te.Code == "invalid_client" && te.StatusCode == 401
		}},
	}
	for _, cs := range cases {
		t.Run(cs.name, func(t *testing.T) {
			as, c := setup(t, "")
			as.reply = func(w http.ResponseWriter, _ url.Values) bool {
				w.WriteHeader(cs.status)
				io.WriteString(w, cs.body)
				return true
			}
			tok, err := c.ClientCredentials(context.Background())
			if !cs.check(err) {
				t.Errorf("ответ %d %s: token=%+v err=%v", cs.status, cs.body, tok, err)
			}
		})
	}
}

func TestExchangeRespectsContext(t *testing.T) {
	as, c := setup(t, "")
	release := make(chan struct{})
	defer close(release)
	as.reply = func(w http.ResponseWriter, _ url.Values) bool { <-release; return true }
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Exchange(ctx, "c", "v"); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("ctx с таймаутом: %v", err)
	}
}
