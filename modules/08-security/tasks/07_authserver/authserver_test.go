package authserver

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	spaRedirect = "https://shop.example.com/callback"
	verifier    = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	challenge   = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.now }
func (c *clock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type env struct {
	t   *testing.T
	s   *Server
	clk *clock
}

func newEnv(t *testing.T) *env {
	clk := &clock{now: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)}
	s := New([]Client{
		{ID: "spa", RedirectURIs: []string{spaRedirect, "https://shop.example.com/cb2?src=app"}, Scopes: []string{"openid", "orders:read"}},
		{ID: "billing", Secret: "b:s3cret/+", RedirectURIs: []string{"https://billing.example.com/cb"}, Scopes: []string{"orders:read", "orders:write"}},
		{ID: "gateway", Secret: "gw-secret", Scopes: []string{"introspect"}},
	}, Options{CodeTTL: time.Minute, TokenTTL: time.Hour, Now: clk.Now})
	return &env{t: t, s: s, clk: clk}
}

func authorizeURL(params map[string]string) string {
	q := url.Values{
		"response_type": {"code"}, "client_id": {"spa"}, "redirect_uri": {spaRedirect},
		"scope": {"orders:read"}, "state": {"xyz"}, "code_challenge": {challenge}, "code_challenge_method": {"S256"},
	}
	for k, v := range params {
		if v == "-" {
			q.Del(k)
		} else {
			q.Set(k, v)
		}
	}
	return "/authorize?" + q.Encode()
}

func (e *env) authorize(params map[string]string, user string) *httptest.ResponseRecorder {
	req := httptest.NewRequest("GET", authorizeURL(params), nil)
	if user != "" {
		req.Header.Set(UserHeader, user)
	}
	rec := httptest.NewRecorder()
	e.s.ServeHTTP(rec, req)
	return rec
}

func (e *env) code(params map[string]string) string {
	e.t.Helper()
	rec := e.authorize(params, "alice")
	if rec.Code != http.StatusFound {
		e.t.Fatalf("/authorize: код %d, ожидался 302; тело %s", rec.Code, rec.Body)
	}
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc.Query().Get("state") != "xyz" || loc.Query().Get("code") == "" {
		e.t.Fatalf("редирект %s: нет code или state", loc)
	}
	return loc.Query().Get("code")
}

type tokenResp struct {
	status int
	body   map[string]any
	header http.Header
}

func (e *env) post(path string, form url.Values, basicID, basicSecret string) tokenResp {
	req := httptest.NewRequest("POST", path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if basicID != "" {
		req.SetBasicAuth(url.QueryEscape(basicID), url.QueryEscape(basicSecret))
	}
	rec := httptest.NewRecorder()
	e.s.ServeHTTP(rec, req)
	var m map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &m)
	return tokenResp{rec.Code, m, rec.Header()}
}

func exchangeForm(code string) url.Values {
	return url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {spaRedirect},
		"client_id": {"spa"}, "code_verifier": {verifier}}
}

func (e *env) introspect(token string) map[string]any {
	return e.post("/introspect", url.Values{"token": {token}}, "gateway", "gw-secret").body
}

func TestAuthorizationCodeFlow(t *testing.T) {
	e := newEnv(t)
	code := e.code(nil)
	r := e.post("/token", exchangeForm(code), "", "")
	if r.status != 200 {
		t.Fatalf("/token: %d %v", r.status, r.body)
	}
	if r.body["token_type"] != "Bearer" || r.body["expires_in"] != float64(3600) || r.body["scope"] != "orders:read" {
		t.Errorf("ответ /token: %v", r.body)
	}
	if r.header.Get("Cache-Control") != "no-store" {
		t.Error("ответ с токеном должен иметь Cache-Control: no-store")
	}
	at, _ := r.body["access_token"].(string)
	info := e.introspect(at)
	if info["active"] != true || info["sub"] != "alice" || info["client_id"] != "spa" || info["scope"] != "orders:read" {
		t.Errorf("introspect: %v", info)
	}
	if info["exp"] != float64(e.clk.Now().Add(time.Hour).Unix()) {
		t.Errorf("exp = %v", info["exp"])
	}
	e.clk.Advance(time.Hour)
	if info := e.introspect(at); info["active"] != false {
		t.Errorf("просроченный токен активен: %v", info)
	}
	if info := e.introspect("garbage"); info["active"] != false || len(info) != 1 {
		t.Errorf("неизвестный токен: %v, ожидалось только {\"active\": false}", info)
	}
}

func TestAuthorizeNoRedirectOnBadClientOrURI(t *testing.T) {
	e := newEnv(t)
	cases := []map[string]string{
		{"client_id": "unknown"},
		{"redirect_uri": "https://evil.com/callback"},
		{"redirect_uri": spaRedirect + "/../evil"},
		{"redirect_uri": spaRedirect + "?x=1"},
		{"redirect_uri": "https://shop.example.com/callback/"},
		{"redirect_uri": "-"}, // у spa два зарегистрированных URI — выбрать нельзя
	}
	for _, p := range cases {
		rec := e.authorize(p, "alice")
		if rec.Code != http.StatusBadRequest || rec.Header().Get("Location") != "" {
			t.Errorf("%v: код %d Location %q; ожидалось 400 без редиректа (open redirect!)", p, rec.Code, rec.Header().Get("Location"))
		}
	}
}

func TestAuthorizeErrorsRedirect(t *testing.T) {
	e := newEnv(t)
	cases := []struct {
		params map[string]string
		user   string
		err    string
	}{
		{map[string]string{"response_type": "token"}, "alice", "unsupported_response_type"},
		{map[string]string{"code_challenge": "-"}, "alice", "invalid_request"},
		{map[string]string{"code_challenge_method": "plain"}, "alice", "invalid_request"},
		{map[string]string{"code_challenge_method": "-"}, "alice", "invalid_request"},
		{map[string]string{"code_challenge": "short"}, "alice", "invalid_request"},
		{map[string]string{"scope": "orders:read admin"}, "alice", "invalid_scope"},
		{nil, "", "access_denied"},
	}
	for _, c := range cases {
		rec := e.authorize(c.params, c.user)
		loc, _ := url.Parse(rec.Header().Get("Location"))
		if rec.Code != http.StatusFound || loc == nil || !strings.HasPrefix(loc.String(), spaRedirect) {
			t.Errorf("%v: код %d Location %q; ожидался редирект на redirect_uri", c.params, rec.Code, rec.Header().Get("Location"))
			continue
		}
		if loc.Query().Get("error") != c.err || loc.Query().Get("state") != "xyz" || loc.Query().Has("code") {
			t.Errorf("%v: %s; ожидалось error=%s и state=xyz", c.params, loc, c.err)
		}
	}
}

func TestAuthorizeKeepsRedirectQuery(t *testing.T) {
	e := newEnv(t)
	rec := e.authorize(map[string]string{"redirect_uri": "https://shop.example.com/cb2?src=app"}, "alice")
	loc, _ := url.Parse(rec.Header().Get("Location"))
	if loc == nil || loc.Query().Get("src") != "app" || loc.Query().Get("code") == "" {
		t.Errorf("Location %q: query redirect_uri должен сохраниться", rec.Header().Get("Location"))
	}
}

func TestTokenValidation(t *testing.T) {
	e := newEnv(t)
	mut := func(f func(url.Values)) url.Values {
		v := exchangeForm(e.code(nil))
		f(v)
		return v
	}
	cases := []struct {
		name   string
		form   url.Values
		status int
		err    string
	}{
		{"неверный verifier", mut(func(v url.Values) { v.Set("code_verifier", strings.Repeat("a", 43)) }), 400, "invalid_grant"},
		{"verifier короче 43", mut(func(v url.Values) { v.Set("code_verifier", "abc") }), 400, "invalid_grant"},
		{"нет verifier", mut(func(v url.Values) { v.Del("code_verifier") }), 400, "invalid_grant"},
		{"другой redirect_uri", mut(func(v url.Values) { v.Set("redirect_uri", "https://shop.example.com/cb2?src=app") }), 400, "invalid_grant"},
		{"неизвестный код", mut(func(v url.Values) { v.Set("code", "nope") }), 400, "invalid_grant"},
		{"неизвестный клиент", mut(func(v url.Values) { v.Set("client_id", "who") }), 401, "invalid_client"},
		{"grant password", mut(func(v url.Values) { v.Set("grant_type", "password") }), 400, "unsupported_grant_type"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := e.post("/token", c.form, "", "")
			if r.status != c.status || r.body["error"] != c.err {
				t.Errorf("%d %v; ожидалось %d %s", r.status, r.body, c.status, c.err)
			}
			if c.status == 401 && r.header.Get("WWW-Authenticate") == "" {
				t.Error("401 без WWW-Authenticate")
			}
		})
	}
}

func TestCodeExpires(t *testing.T) {
	e := newEnv(t)
	code := e.code(nil)
	e.clk.Advance(time.Minute)
	if r := e.post("/token", exchangeForm(code), "", ""); r.body["error"] != "invalid_grant" {
		t.Errorf("просроченный код: %d %v", r.status, r.body)
	}
}

func TestCodeBoundToClient(t *testing.T) {
	e := newEnv(t)
	code := e.code(nil) // выдан spa
	form := exchangeForm(code)
	form.Del("client_id")
	if r := e.post("/token", form, "billing", "b:s3cret/+"); r.body["error"] != "invalid_grant" {
		t.Errorf("чужой клиент предъявил код spa: %d %v", r.status, r.body)
	}
}

func TestCodeReuseRevokesTokens(t *testing.T) {
	e := newEnv(t)
	code := e.code(nil)
	r := e.post("/token", exchangeForm(code), "", "")
	at, _ := r.body["access_token"].(string)
	if r.status != 200 || at == "" {
		t.Fatalf("первый обмен: %d %v", r.status, r.body)
	}
	if r2 := e.post("/token", exchangeForm(code), "", ""); r2.body["error"] != "invalid_grant" {
		t.Errorf("повторный обмен: %d %v, ожидался invalid_grant", r2.status, r2.body)
	}
	if info := e.introspect(at); info["active"] != false {
		t.Errorf("после повторного предъявления кода токен должен быть отозван: %v", info)
	}
}

func TestCodeConcurrentExchange(t *testing.T) {
	e := newEnv(t)
	code := e.code(nil)
	var ok atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := e.post("/token", exchangeForm(code), "", ""); r.status == 200 {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 {
		t.Errorf("код обменян %d раз, ожидалось ровно 1", ok.Load())
	}
}

func TestConfidentialClient(t *testing.T) {
	e := newEnv(t)
	params := map[string]string{"client_id": "billing", "redirect_uri": "-", "scope": "orders:write"}
	code := e.code(params) // единственный redirect_uri подставляется сам
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}}

	// без секрета, с client_id в теле — нельзя
	withID := url.Values{"client_id": {"billing"}}
	for k, v := range form {
		withID[k] = v
	}
	if r := e.post("/token", withID, "", ""); r.status != 401 || r.body["error"] != "invalid_client" {
		t.Errorf("конфиденциальный клиент без секрета: %d %v", r.status, r.body)
	}
	if r := e.post("/token", form, "billing", "wrong"); r.status != 401 {
		t.Errorf("неверный секрет: %d %v", r.status, r.body)
	}
	r := e.post("/token", form, "billing", "b:s3cret/+")
	if r.status != 200 || r.body["scope"] != "orders:write" {
		t.Errorf("Basic с urlencoded секретом: %d %v", r.status, r.body)
	}
}

func TestClientCredentials(t *testing.T) {
	e := newEnv(t)
	r := e.post("/token", url.Values{"grant_type": {"client_credentials"}, "scope": {"orders:read"}}, "billing", "b:s3cret/+")
	if r.status != 200 {
		t.Fatalf("client_credentials: %d %v", r.status, r.body)
	}
	at, _ := r.body["access_token"].(string)
	if info := e.introspect(at); info["sub"] != "billing" || info["active"] != true {
		t.Errorf("introspect: %v", info)
	}
	if r := e.post("/token", url.Values{"grant_type": {"client_credentials"}, "scope": {"admin"}}, "billing", "b:s3cret/+"); r.body["error"] != "invalid_scope" {
		t.Errorf("недопустимый scope: %v", r.body)
	}
	if r := e.post("/token", url.Values{"grant_type": {"client_credentials"}, "client_id": {"spa"}}, "", ""); r.body["error"] != "unauthorized_client" {
		t.Errorf("публичный клиент и client_credentials: %d %v", r.status, r.body)
	}
}

func TestIntrospectRequiresAuth(t *testing.T) {
	e := newEnv(t)
	if r := e.post("/introspect", url.Values{"token": {"x"}, "client_id": {"spa"}}, "", ""); r.status != 401 {
		t.Errorf("introspect без Basic: %d", r.status)
	}
	if r := e.post("/introspect", url.Values{"token": {"x"}}, "gateway", "bad"); r.status != 401 {
		t.Errorf("introspect с неверным секретом: %d", r.status)
	}
}

func TestChallengeConstant(t *testing.T) {
	sum := sha256.Sum256([]byte(verifier))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != challenge {
		t.Fatal("тест повреждён")
	}
}
