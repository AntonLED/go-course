package mtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// ---- тестовая PKI

type ca struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

var serial int64

func newCA(t *testing.T, cn string) *ca {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial++
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &ca{cert, key, pool}
}

func (c *ca) issue(t *testing.T, tmpl *x509.Certificate) tls.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	serial++
	tmpl.SerialNumber = big.NewInt(serial)
	tmpl.NotBefore, tmpl.NotAfter = time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	tmpl.KeyUsage = x509.KeyUsageDigitalSignature
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, &key.PublicKey, c.key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, _ := x509.ParseCertificate(der)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}
}

func (c *ca) server(t *testing.T) tls.Certificate {
	return c.issue(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "inventory"}, DNSNames: []string{"inventory.internal", "localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
}

func (c *ca) client(t *testing.T, cn string, uris ...string) tls.Certificate {
	tmpl := &x509.Certificate{
		Subject:     pkix.Name{CommonName: cn},
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	for _, u := range uris {
		pu, _ := url.Parse(u)
		tmpl.URIs = append(tmpl.URIs, pu)
	}
	return c.issue(t, tmpl)
}

// ---- стенд

type env struct {
	ca   *ca
	srv  *httptest.Server
	host string
}

func newEnv(t *testing.T, allowed []string, mutate func(*tls.Config)) *env {
	t.Helper()
	c := newCA(t, "Internal CA")
	h := RequireIdentity(allowed, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := IdentityFromContext(r.Context())
		if !ok {
			http.Error(w, "no identity in context", 500)
			return
		}
		fmt.Fprintf(w, "hello %s", id.CommonName)
	}))
	srv := httptest.NewUnstartedServer(h)
	srv.Config.ErrorLog = log.New(io.Discard, "", 0) // не шумим ошибками рукопожатия
	srv.TLS = ServerTLSConfig(c.server(t), c.pool)
	if mutate != nil {
		mutate(srv.TLS)
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return &env{ca: c, srv: srv}
}

func (e *env) get(t *testing.T, cfg *tls.Config) (int, string, error) {
	t.Helper()
	tr := &http.Transport{TLSClientConfig: cfg}
	defer tr.CloseIdleConnections()
	resp, err := (&http.Client{Transport: tr, Timeout: 5 * time.Second}).Get(e.srv.URL)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.TLS == nil || resp.TLS.Version < tls.VersionTLS12 {
		t.Errorf("соединение не TLS 1.2+: %+v", resp.TLS)
	}
	return resp.StatusCode, string(b), nil
}

func TestServerTLSConfigFields(t *testing.T) {
	c := newCA(t, "CA")
	cfg := ServerTLSConfig(c.server(t), c.pool)
	if cfg.MinVersion < tls.VersionTLS12 {
		t.Errorf("MinVersion = %x, ожидалось не ниже TLS 1.2", cfg.MinVersion)
	}
	if cfg.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("ClientAuth = %v, ожидалось RequireAndVerifyClientCert", cfg.ClientAuth)
	}
	if cfg.ClientCAs != c.pool || len(cfg.Certificates) != 1 {
		t.Error("ClientCAs/Certificates не заданы")
	}
	if len(cfg.CipherSuites) == 0 {
		t.Error("CipherSuites не задан — для TLS 1.2 ограничьте список ECDHE+AEAD")
	}
	insecure := map[uint16]bool{}
	for _, cs := range tls.InsecureCipherSuites() {
		insecure[cs.ID] = true
	}
	for _, id := range cfg.CipherSuites {
		name := tls.CipherSuiteName(id)
		if insecure[id] || !strings.HasPrefix(name, "TLS_ECDHE_") || !(strings.Contains(name, "GCM") || strings.Contains(name, "CHACHA20")) {
			t.Errorf("небезопасный или без forward secrecy шифр: %s", name)
		}
	}
	if len(cfg.CurvePreferences) == 0 || cfg.CurvePreferences[0] != tls.X25519 {
		t.Errorf("CurvePreferences = %v, ожидалось X25519 первым", cfg.CurvePreferences)
	}

	cl := ClientTLSConfig(nil, c.pool, "inventory.internal")
	if cl.RootCAs != c.pool || cl.ServerName != "inventory.internal" || cl.MinVersion < tls.VersionTLS12 || cl.InsecureSkipVerify {
		t.Errorf("ClientTLSConfig: %+v", cl)
	}
	if len(cl.Certificates) != 0 {
		t.Error("без clientCert клиент не должен предъявлять сертификат")
	}
}

func TestMTLSAuthorized(t *testing.T) {
	e := newEnv(t, []string{"orders"}, nil)
	cert := e.ca.client(t, "orders")
	code, body, err := e.get(t, ClientTLSConfig(&cert, e.ca.pool, "inventory.internal"))
	if err != nil {
		t.Fatalf("запрос с правильным сертификатом: %v", err)
	}
	if code != 200 || body != "hello orders" {
		t.Errorf("ответ %d %q, ожидалось 200 \"hello orders\"", code, body)
	}
}

func TestMTLSForbidden(t *testing.T) {
	e := newEnv(t, []string{"orders"}, nil)
	cert := e.ca.client(t, "billing")
	code, _, err := e.get(t, ClientTLSConfig(&cert, e.ca.pool, "inventory.internal"))
	if err != nil {
		t.Fatalf("валидный сертификат не того сервиса должен пройти TLS: %v", err)
	}
	if code != http.StatusForbidden {
		t.Errorf("код %d, ожидалось 403: аутентифицирован, но не авторизован", code)
	}
}

func TestMTLSSpiffeURI(t *testing.T) {
	const id = "spiffe://example.org/ns/prod/sa/orders"
	e := newEnv(t, []string{id}, nil)
	cert := e.ca.client(t, "pod-7f9c", id)
	code, _, err := e.get(t, ClientTLSConfig(&cert, e.ca.pool, "inventory.internal"))
	if err != nil || code != 200 {
		t.Errorf("авторизация по URI SAN: код %d, ошибка %v", code, err)
	}
}

func TestMTLSHandshakeFailures(t *testing.T) {
	e := newEnv(t, []string{"orders"}, nil)
	good := e.ca.client(t, "orders")
	evil := newCA(t, "Evil CA")
	evilCert := evil.client(t, "orders") // тот же CN, но чужой CA

	tooOld := ClientTLSConfig(&good, e.ca.pool, "inventory.internal")
	tooOld.MinVersion, tooOld.MaxVersion = tls.VersionTLS10, tls.VersionTLS11

	cases := []struct {
		name string
		cfg  *tls.Config
	}{
		{"без клиентского сертификата", ClientTLSConfig(nil, e.ca.pool, "inventory.internal")},
		{"сертификат чужого CA", ClientTLSConfig(&evilCert, e.ca.pool, "inventory.internal")},
		{"клиент не доверяет CA сервера", ClientTLSConfig(&good, evil.pool, "inventory.internal")},
		{"неверное имя сервера", ClientTLSConfig(&good, e.ca.pool, "billing.internal")},
		{"TLS 1.1", tooOld},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if code, _, err := e.get(t, c.cfg); err == nil {
				t.Errorf("ожидалась ошибка TLS, а получен HTTP %d", code)
			}
		})
	}
}

func TestUnverifiedCertIsNotTrusted(t *testing.T) {
	// Сервер ошибочно настроен запрашивать сертификат без проверки.
	// PeerCertificates заполнены, но доверять им нельзя.
	e := newEnv(t, []string{"orders"}, func(c *tls.Config) { c.ClientAuth = tls.RequestClientCert })
	evil := newCA(t, "Evil CA")
	forged := evil.client(t, "orders")
	code, _, err := e.get(t, ClientTLSConfig(&forged, e.ca.pool, "inventory.internal"))
	if err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if code != http.StatusUnauthorized {
		t.Errorf("непроверенный сертификат: код %d, ожидалось 401 (смотрите VerifiedChains, а не PeerCertificates)", code)
	}
	// без сертификата вообще
	code, _, err = e.get(t, ClientTLSConfig(nil, e.ca.pool, "inventory.internal"))
	if err != nil || code != http.StatusUnauthorized {
		t.Errorf("без сертификата: код %d, ошибка %v; ожидалось 401", code, err)
	}
}

func TestRequireIdentityPlainHTTP(t *testing.T) {
	h := RequireIdentity([]string{"orders"}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("запрос без TLS: код %d, ожидалось 401", rec.Code)
	}
	if _, ok := PeerIdentity(nil); ok {
		t.Error("PeerIdentity(nil) должен вернуть false")
	}
}

func TestPeerIdentity(t *testing.T) {
	c := newCA(t, "CA")
	cert := c.client(t, "orders", "spiffe://example.org/sa/orders")
	cert.Leaf.DNSNames = []string{"orders.internal"}
	state := &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{cert.Leaf},
		VerifiedChains:   [][]*x509.Certificate{{cert.Leaf, c.cert}},
	}
	id, ok := PeerIdentity(state)
	if !ok || id.CommonName != "orders" || len(id.URIs) != 1 || id.URIs[0] != "spiffe://example.org/sa/orders" ||
		len(id.DNSNames) != 1 || id.DNSNames[0] != "orders.internal" {
		t.Errorf("PeerIdentity = %+v, %v", id, ok)
	}
	state.VerifiedChains = nil
	if _, ok := PeerIdentity(state); ok {
		t.Error("без VerifiedChains идентичность не должна извлекаться")
	}
}
