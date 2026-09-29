package secureapi

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------- PKI

type pki struct {
	caCert *x509.Certificate
	caKey  *ecdsa.PrivateKey
	caPEM  []byte
	serial int64
}

func newPKI(t *testing.T) *pki {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Shop Internal CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return &pki{caCert: cert, caKey: key, caPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), serial: 1}
}

// issue возвращает PEM сертификата и ключа.
func (p *pki) issue(t *testing.T, cn, spiffe string, server bool) (certPEM, keyPEM []byte) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	p.serial++
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(p.serial), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	if server {
		tmpl.DNSNames = []string{"items.internal"}
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	} else {
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}
	if spiffe != "" {
		u, _ := url.Parse(spiffe)
		tmpl.URIs = []*url.URL{u}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, p.caCert, &key.PublicKey, p.caKey)
	if err != nil {
		t.Fatal(err)
	}
	kder, _ := x509.MarshalECPrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kder})
}

// ---------------------------------------------------------------- JWT

var (
	keyV1 = []byte("key-v1-key-v1-key-v1-key-v1-key!")
	keyV2 = []byte("key-v2-key-v2-key-v2-key-v2-key!")
)

func sign(t *testing.T, header, payload map[string]any, key []byte) string {
	t.Helper()
	h, _ := json.Marshal(header)
	p, _ := json.Marshal(payload)
	si := base64.RawURLEncoding.EncodeToString(h) + "." + base64.RawURLEncoding.EncodeToString(p)
	m := hmac.New(sha256.New, key)
	m.Write([]byte(si))
	return si + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// ---------------------------------------------------------------- стенд

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
	pki *pki
	srv *httptest.Server
	clk *clock
	cli map[string]*http.Client
}

func newEnv(t *testing.T, burst int) *env {
	t.Helper()
	p := newPKI(t)
	clk := &clock{now: time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)}
	h := NewAPI(Config{
		JWTKeys:         map[string][]byte{"v1": keyV1, "v2": keyV2},
		Issuer:          "https://auth.shop.local",
		Audience:        "items-api",
		Leeway:          30 * time.Second,
		AllowedServices: []string{"spiffe://shop.local/orders", "gateway"},
		RatePerSecond:   1, Burst: burst,
		Now: clk.Now,
	})
	certPEM, keyPEM := p.issue(t, "items", "", true)
	cfg, err := ServerTLSConfig(certPEM, keyPEM, p.caPEM)
	if err != nil {
		t.Fatalf("ServerTLSConfig: %v", err)
	}
	srv := httptest.NewUnstartedServer(h)
	srv.Config.ErrorLog = log.New(io.Discard, "", 0)
	srv.TLS = cfg
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return &env{t: t, pki: p, srv: srv, clk: clk, cli: map[string]*http.Client{}}
}

// client возвращает HTTP-клиент с клиентским сертификатом (cn == "" — без сертификата).
func (e *env) client(cn, spiffe string) *http.Client {
	key := cn + "|" + spiffe
	if c, ok := e.cli[key]; ok {
		return c
	}
	roots := x509.NewCertPool()
	roots.AddCert(e.pki.caCert)
	cfg := &tls.Config{RootCAs: roots, ServerName: "items.internal", MinVersion: tls.VersionTLS12}
	if cn != "" {
		certPEM, keyPEM := e.pki.issue(e.t, cn, spiffe, false)
		cert, err := tls.X509KeyPair(certPEM, keyPEM)
		if err != nil {
			e.t.Fatal(err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	tr := &http.Transport{TLSClientConfig: cfg}
	e.t.Cleanup(tr.CloseIdleConnections)
	c := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	e.cli[key] = c
	return c
}

func (e *env) orders() *http.Client { return e.client("orders-pod-1", "spiffe://shop.local/orders") }

type resp struct {
	code   int
	body   string
	header http.Header
}

func (e *env) do(c *http.Client, method, path, token, body string) resp {
	e.t.Helper()
	req, _ := http.NewRequest(method, e.srv.URL+path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", token)
	}
	r, err := c.Do(req)
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return resp{r.StatusCode, string(b), r.Header}
}

func (e *env) token(sub, scope string, roles []string, mod func(h, p map[string]any)) string {
	h := map[string]any{"alg": "HS256", "typ": "JWT", "kid": "v1"}
	p := map[string]any{
		"iss": "https://auth.shop.local", "sub": sub, "aud": []string{"items-api", "other"},
		"exp": e.clk.Now().Add(time.Hour).Unix(), "iat": e.clk.Now().Unix(), "scope": scope,
	}
	if roles != nil {
		p["roles"] = roles
	}
	if mod != nil {
		mod(h, p)
	}
	key := keyV1
	if h["kid"] == "v2" {
		key = keyV2
	}
	return "Bearer " + sign(e.t, h, p, key)
}

// ---------------------------------------------------------------- тесты

func TestServerTLSConfig(t *testing.T) {
	p := newPKI(t)
	certPEM, keyPEM := p.issue(t, "items", "", true)
	cfg, err := ServerTLSConfig(certPEM, keyPEM, p.caPEM)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MinVersion < tls.VersionTLS12 || cfg.ClientAuth != tls.RequireAndVerifyClientCert || cfg.ClientCAs == nil || len(cfg.Certificates) != 1 {
		t.Errorf("конфигурация: MinVersion=%x ClientAuth=%v", cfg.MinVersion, cfg.ClientAuth)
	}
	if _, err := ServerTLSConfig([]byte("junk"), keyPEM, p.caPEM); err == nil {
		t.Error("битый сертификат сервера должен давать ошибку")
	}
	if _, err := ServerTLSConfig(certPEM, keyPEM, []byte("no certs here")); err == nil {
		t.Error("пустой CA должен давать ошибку")
	}
	otherCert, _ := p.issue(t, "other", "", true)
	if _, err := ServerTLSConfig(otherCert, keyPEM, p.caPEM); err == nil {
		t.Error("ключ не от того сертификата должен давать ошибку")
	}
}

func TestHappyPath(t *testing.T) {
	e := newEnv(t, 100)
	c := e.orders()
	if r := e.do(c, "GET", "/healthz", "", ""); r.code != 200 || r.body != "ok" {
		t.Errorf("/healthz: %d %q", r.code, r.body)
	}
	rw := e.token("alice", "items:read items:write", nil, nil)
	if r := e.do(c, "GET", "/v1/items", rw, ""); r.code != 200 || strings.TrimSpace(r.body) != "[]" {
		t.Errorf("пустой список: %d %q", r.code, r.body)
	}
	r := e.do(c, "POST", "/v1/items", rw, `{"name":"book"}`)
	var it Item
	if r.code != 201 || json.Unmarshal([]byte(r.body), &it) != nil || it.ID != "1" || it.Name != "book" {
		t.Fatalf("создание: %d %s", r.code, r.body)
	}
	if r := e.do(c, "POST", "/v1/items", rw, `{"name":""}`); r.code != 400 {
		t.Errorf("пустое имя: %d", r.code)
	}
	if r := e.do(c, "GET", "/v1/items", rw, ""); !strings.Contains(r.body, `"book"`) {
		t.Errorf("список после создания: %s", r.body)
	}
	// ротация ключей: токен на новом ключе v2
	v2 := e.token("alice", "items:read", nil, func(h, p map[string]any) { h["kid"] = "v2" })
	if r := e.do(c, "GET", "/v1/items", v2, ""); r.code != 200 {
		t.Errorf("токен с kid=v2: %d %s", r.code, r.body)
	}
	// gateway авторизован по CN (без SPIFFE ID)
	if r := e.do(e.client("gateway", ""), "GET", "/v1/items", rw, ""); r.code != 200 {
		t.Errorf("gateway по CN: %d %s", r.code, r.body)
	}
}

func TestAuthorization(t *testing.T) {
	e := newEnv(t, 100)
	c := e.orders()
	reader := e.token("bob", "items:read", nil, nil)
	r := e.do(c, "POST", "/v1/items", reader, `{"name":"x"}`)
	if r.code != 403 {
		t.Fatalf("POST только с items:read: %d, ожидалось 403", r.code)
	}
	if w := r.header.Get("WWW-Authenticate"); !strings.Contains(w, `error="insufficient_scope"`) || !strings.Contains(w, `scope="items:write"`) {
		t.Errorf("WWW-Authenticate = %q", w)
	}
	// scope — это список через пробел, а не подстрока
	tricky := e.token("bob", "items:readonly", nil, nil)
	if r := e.do(c, "GET", "/v1/items", tricky, ""); r.code != 403 {
		t.Errorf("scope items:readonly не даёт items:read: %d", r.code)
	}

	writer := e.token("carol", "items:write", nil, nil)
	e.do(c, "POST", "/v1/items", writer, `{"name":"x"}`)
	if r := e.do(c, "DELETE", "/v1/items/1", writer, ""); r.code != 403 {
		t.Errorf("DELETE без роли admin: %d", r.code)
	}
	admin := e.token("root", "", []string{"admin"}, nil)
	if r := e.do(c, "DELETE", "/v1/items/1", admin, ""); r.code != 204 {
		t.Errorf("DELETE админом: %d %s", r.code, r.body)
	}
	if r := e.do(c, "DELETE", "/v1/items/1", admin, ""); r.code != 404 {
		t.Errorf("повторный DELETE: %d", r.code)
	}
}

func TestAuthentication(t *testing.T) {
	e := newEnv(t, 100)
	c := e.orders()
	r := e.do(c, "GET", "/v1/items", "", "")
	if r.code != 401 || !strings.HasPrefix(r.header.Get("WWW-Authenticate"), "Bearer") || strings.Contains(r.header.Get("WWW-Authenticate"), "error=") {
		t.Errorf("без токена: %d, WWW-Authenticate=%q; ожидалось 401 и Bearer без error", r.code, r.header.Get("WWW-Authenticate"))
	}
	var body map[string]string
	if json.Unmarshal([]byte(r.body), &body) != nil || body["error"] == "" {
		t.Errorf("тело ошибки: %q", r.body)
	}

	good := e.token("alice", "items:read", nil, nil)
	parts := strings.Split(strings.TrimPrefix(good, "Bearer "), ".")
	cases := map[string]string{
		"истёк":             e.token("alice", "items:read", nil, func(h, p map[string]any) { p["exp"] = e.clk.Now().Add(-time.Minute).Unix() }),
		"нет exp":           e.token("alice", "items:read", nil, func(h, p map[string]any) { delete(p, "exp") }),
		"nbf в будущем":     e.token("alice", "items:read", nil, func(h, p map[string]any) { p["nbf"] = e.clk.Now().Add(time.Hour).Unix() }),
		"чужой iss":         e.token("alice", "items:read", nil, func(h, p map[string]any) { p["iss"] = "https://evil" }),
		"чужая aud":         e.token("alice", "items:read", nil, func(h, p map[string]any) { p["aud"] = "billing-api" }),
		"нет sub":           e.token("", "items:read", nil, nil),
		"неизвестный kid":   e.token("alice", "items:read", nil, func(h, p map[string]any) { h["kid"] = "v0" }),
		"alg none":          "Bearer " + base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","kid":"v1"}`)) + "." + parts[1] + ".",
		"подменённая часть": "Bearer " + parts[0] + "." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"root","scope":"items:read","iss":"https://auth.shop.local","aud":"items-api","exp":9999999999}`)) + "." + parts[2],
		"Basic":             "Basic YWxpY2U6cHc=",
		"мусор":             "Bearer abc",
	}
	for name, tok := range cases {
		r := e.do(c, "GET", "/v1/items", tok, "")
		if r.code != 401 || !strings.Contains(r.header.Get("WWW-Authenticate"), `error="invalid_token"`) {
			t.Errorf("%s: %d, WWW-Authenticate=%q; ожидалось 401 invalid_token", name, r.code, r.header.Get("WWW-Authenticate"))
		}
	}
	// истёк, но в пределах leeway 30s
	lee := e.token("alice", "items:read", nil, func(h, p map[string]any) { p["exp"] = e.clk.Now().Add(-10 * time.Second).Unix() })
	if r := e.do(c, "GET", "/v1/items", lee, ""); r.code != 200 {
		t.Errorf("exp в пределах leeway: %d", r.code)
	}
	// aud строкой
	single := e.token("alice", "items:read", nil, func(h, p map[string]any) { p["aud"] = "items-api" })
	if r := e.do(c, "GET", "/v1/items", single, ""); r.code != 200 {
		t.Errorf("aud строкой: %d", r.code)
	}
}

func TestMTLS(t *testing.T) {
	e := newEnv(t, 100)
	tok := e.token("alice", "items:read", nil, nil)
	// валидный сертификат нашего CA, но сервис не в списке
	if r := e.do(e.client("analytics", "spiffe://shop.local/analytics"), "GET", "/v1/items", tok, ""); r.code != 403 {
		t.Errorf("чужой сервис: %d, ожидалось 403", r.code)
	}
	// CN совпадает с разрешённым, но SPIFFE ID другой — решает SPIFFE ID
	if r := e.do(e.client("gateway", "spiffe://shop.local/analytics"), "GET", "/v1/items", tok, ""); r.code != 403 {
		t.Errorf("CN gateway + чужой SPIFFE: %d, ожидалось 403", r.code)
	}
	// без клиентского сертификата TLS не установится
	req, _ := http.NewRequest("GET", e.srv.URL+"/healthz", nil)
	if r, err := e.client("", "").Do(req); err == nil {
		r.Body.Close()
		t.Errorf("запрос без клиентского сертификата прошёл: %d", r.StatusCode)
	}
	// без TLS вообще (обработчик напрямую) — 401
	h := NewAPI(Config{JWTKeys: map[string][]byte{"v1": keyV1}, AllowedServices: []string{"gateway"}, RatePerSecond: 1, Burst: 1})
	rec := httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/v1/items", nil)
	req.Header.Set("Authorization", tok)
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("запрос без TLS: %d, ожидалось 401", rec.Code)
	}
	// сертификат есть в PeerCertificates, но НЕ проверен (как при RequestClientCert /
	// RequireAnyClientCert): доверять ему нельзя — 401
	gwPEM, _ := e.pki.issue(t, "gateway", "", false)
	blk, _ := pem.Decode(gwPEM)
	gwCert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	h2 := NewAPI(Config{
		JWTKeys: map[string][]byte{"v1": keyV1}, Issuer: "https://auth.shop.local", Audience: "items-api",
		AllowedServices: []string{"gateway"}, RatePerSecond: 1, Burst: 10, Now: e.clk.Now,
	})
	withState := func(st *tls.ConnectionState) int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/v1/items", nil)
		req.Header.Set("Authorization", tok)
		req.TLS = st
		h2.ServeHTTP(rec, req)
		return rec.Code
	}
	if code := withState(&tls.ConnectionState{PeerCertificates: []*x509.Certificate{gwCert}}); code != 401 {
		t.Errorf("непроверенный клиентский сертификат (нет VerifiedChains): %d, ожидалось 401", code)
	}
	// контроль: тот же сертификат в проверенной цепочке — 200
	if code := withState(&tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{gwCert},
		VerifiedChains:   [][]*x509.Certificate{{gwCert, e.pki.caCert}},
	}); code != 200 {
		t.Errorf("проверенный сертификат gateway: %d, ожидалось 200", code)
	}
}

func TestRateLimit(t *testing.T) {
	e := newEnv(t, 3)
	c := e.orders()
	alice := e.token("alice", "items:read", nil, nil)
	bob := e.token("bob", "items:read", nil, nil)
	for i := 0; i < 3; i++ {
		if r := e.do(c, "GET", "/v1/items", alice, ""); r.code != 200 {
			t.Fatalf("запрос %d в пределах burst: %d", i+1, r.code)
		}
	}
	r := e.do(c, "GET", "/v1/items", alice, "")
	if r.code != 429 || r.header.Get("Retry-After") != "1" {
		t.Fatalf("4-й запрос: %d Retry-After=%q; ожидалось 429 и 1", r.code, r.header.Get("Retry-After"))
	}
	if r := e.do(c, "GET", "/v1/items", bob, ""); r.code != 200 {
		t.Errorf("у другого субъекта своя корзина: %d", r.code)
	}
	e.clk.Advance(500 * time.Millisecond)
	if r := e.do(c, "GET", "/v1/items", alice, ""); r.code != 429 {
		t.Errorf("через 0.5s токен ещё не накопился: %d", r.code)
	}
	e.clk.Advance(500 * time.Millisecond)
	if r := e.do(c, "GET", "/v1/items", alice, ""); r.code != 200 {
		t.Errorf("через 1s токен накопился: %d", r.code)
	}
	e.clk.Advance(time.Hour)
	alice = e.token("alice", "items:read", nil, nil) // старый токен истёк бы (держится только за счёт leeway)
	for i := 0; i < 3; i++ {
		if r := e.do(c, "GET", "/v1/items", alice, ""); r.code != 200 {
			t.Errorf("после простоя корзина полна, но не больше Burst: запрос %d → %d", i+1, r.code)
		}
	}
	if r := e.do(c, "GET", "/v1/items", alice, ""); r.code != 429 {
		t.Errorf("ёмкость корзины ограничена Burst: %d", r.code)
	}
	// невалидные токены не тратят корзину легитимного пользователя, а запросы без прав — тратят
	e.clk.Advance(time.Hour)
	noScope := e.token("carol", "", nil, nil)
	for i := 0; i < 3; i++ {
		e.do(c, "GET", "/v1/items", noScope, "")
	}
	if r := e.do(c, "GET", "/v1/items", noScope, ""); r.code != 429 {
		t.Errorf("rate limit проверяется до авторизации: %d, ожидалось 429", r.code)
	}
}
