package rpcauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net"
	"net/url"
	"strings"
	"testing"
	"time"
)

func echo(ctx context.Context, req any) (any, error) { return req, nil }

func call(ic UnaryServerInterceptor, ctx context.Context, method string) (any, error) {
	return ic(ctx, "req", &UnaryServerInfo{FullMethod: method}, echo)
}

func TestChainUnaryOrder(t *testing.T) {
	var log []string
	mk := func(name string) UnaryServerInterceptor {
		return func(ctx context.Context, req any, info *UnaryServerInfo, h UnaryHandler) (any, error) {
			log = append(log, name+">")
			resp, err := h(ctx, req)
			log = append(log, "<"+name)
			return resp, err
		}
	}
	chain := ChainUnary(mk("a"), mk("b"), mk("c"))
	resp, err := chain(context.Background(), 1, &UnaryServerInfo{FullMethod: "/x"}, func(ctx context.Context, req any) (any, error) {
		log = append(log, "handler")
		return req.(int) + 1, nil
	})
	if got := strings.Join(log, " "); got != "a> b> c> handler <c <b <a" {
		t.Errorf("порядок вызовов %q, ожидалось %q", got, "a> b> c> handler <c <b <a")
	}
	if resp != 2 || err != nil {
		t.Errorf("результат %v, %v", resp, err)
	}

	// цепочка вызывается повторно — состояние не должно «прилипать»
	log = nil
	_, _ = chain(context.Background(), 1, &UnaryServerInfo{}, echo)
	if len(log) != 6 {
		t.Errorf("повторный вызов цепочки: %v", log)
	}

	// короткое замыкание
	deny := func(ctx context.Context, req any, info *UnaryServerInfo, h UnaryHandler) (any, error) {
		return nil, Errorf(PermissionDenied, "no")
	}
	called := false
	_, err = ChainUnary(deny, mk("z"))(context.Background(), nil, &UnaryServerInfo{}, func(context.Context, any) (any, error) {
		called = true
		return nil, nil
	})
	if CodeOf(err) != PermissionDenied || called {
		t.Errorf("интерсептор должен уметь прервать цепочку: err=%v called=%v", err, called)
	}
	if resp, err := ChainUnary()(context.Background(), "x", &UnaryServerInfo{}, echo); resp != "x" || err != nil {
		t.Errorf("пустая цепочка: %v %v", resp, err)
	}
}

func TestRecovery(t *testing.T) {
	_, err := Recovery()(context.Background(), nil, &UnaryServerInfo{}, func(context.Context, any) (any, error) {
		panic("db password is hunter2")
	})
	if CodeOf(err) != Internal || strings.Contains(err.Error(), "hunter2") {
		t.Errorf("паника: %v; ожидался Internal без деталей", err)
	}
}

func validator(ctx context.Context, token string) (Principal, error) {
	switch token {
	case "reader":
		return Principal{Subject: "alice", Scopes: []string{"items:read"}}, nil
	case "writer":
		return Principal{Subject: "bob", Scopes: []string{"items:read", "items:write"}}, nil
	}
	return Principal{}, errors.New("signature mismatch for key k1")
}

func withAuth(vals ...string) context.Context {
	md := MD{}
	if len(vals) > 0 {
		md["authorization"] = vals
	}
	return NewIncomingContext(context.Background(), md)
}

func TestBearerAuth(t *testing.T) {
	var got Principal
	ic := BearerAuth(validator, "/grpc.health.v1.Health/Check")
	handler := func(ctx context.Context, req any) (any, error) {
		got, _ = PrincipalFromContext(ctx)
		return "ok", nil
	}
	cases := []struct {
		name string
		ctx  context.Context
		code Code
		sub  string
	}{
		{"валидный", withAuth("Bearer reader"), OK, "alice"},
		{"схема в нижнем регистре", withAuth("bearer writer"), OK, "bob"},
		{"без метаданных", context.Background(), Unauthenticated, ""},
		{"нет заголовка", withAuth(), Unauthenticated, ""},
		{"Basic", withAuth("Basic YWxpY2U6cHc="), Unauthenticated, ""},
		{"пустой токен", withAuth("Bearer "), Unauthenticated, ""},
		{"без пробела", withAuth("Bearerreader"), Unauthenticated, ""},
		{"два заголовка", withAuth("Bearer reader", "Bearer writer"), Unauthenticated, ""},
		{"неверный токен", withAuth("Bearer forged"), Unauthenticated, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got = Principal{}
			_, err := ic(c.ctx, nil, &UnaryServerInfo{FullMethod: "/items.v1.Items/List"}, handler)
			if CodeOf(err) != c.code {
				t.Fatalf("код %d (%v), ожидался %d", CodeOf(err), err, c.code)
			}
			if got.Subject != c.sub {
				t.Errorf("Principal.Subject = %q, ожидалось %q", got.Subject, c.sub)
			}
			if err != nil && strings.Contains(err.Error(), "k1") {
				t.Errorf("детали ошибки валидации утекли клиенту: %v", err)
			}
		})
	}
	if _, err := call(ic, context.Background(), "/grpc.health.v1.Health/Check"); err != nil {
		t.Errorf("health-check должен пропускаться без токена: %v", err)
	}
}

func TestRequireScopes(t *testing.T) {
	ic := ChainUnary(BearerAuth(validator), RequireScopes(map[string][]string{
		"/items.v1.Items/List":   {"items:read"},
		"/items.v1.Items/Create": {"items:read", "items:write"},
		"/items.v1.Items/Ping":   {},
	}))
	cases := []struct {
		token, method string
		code          Code
	}{
		{"reader", "/items.v1.Items/List", OK},
		{"reader", "/items.v1.Items/Create", PermissionDenied},
		{"writer", "/items.v1.Items/Create", OK},
		{"reader", "/items.v1.Items/Ping", OK},
		{"writer", "/items.v1.Items/Delete", PermissionDenied}, // нет в карте — запрещено
	}
	for _, c := range cases {
		if _, err := call(ic, withAuth("Bearer "+c.token), c.method); CodeOf(err) != c.code {
			t.Errorf("%s → %s: %v, ожидался код %d", c.token, c.method, err, c.code)
		}
	}
	// без BearerAuth в цепочке — нет Principal
	only := RequireScopes(map[string][]string{"/m": {"x"}})
	if _, err := call(only, context.Background(), "/m"); CodeOf(err) != Unauthenticated {
		t.Errorf("нет Principal: %v, ожидался Unauthenticated", err)
	}
}

// ---- mTLS: настоящее TLS-рукопожатие через net.Pipe

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pool *x509.CertPool
}

func newTestCA(t *testing.T) *testCA {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "mesh CA"},
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
	return &testCA{cert, key, pool}
}

func (c *testCA) issue(t *testing.T, cn string, uri string, eku x509.ExtKeyUsage) tls.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: cn},
		DNSNames:  []string{cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		ExtKeyUsage: []x509.ExtKeyUsage{eku},
	}
	if uri != "" {
		u, _ := url.Parse(uri)
		tmpl.URIs = []*url.URL{u}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, &key.PublicKey, c.key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// handshake возвращает состояние соединения со стороны СЕРВЕРА.
func handshake(t *testing.T, ca *testCA, clientCN, clientURI string, auth tls.ClientAuthType) tls.ConnectionState {
	t.Helper()
	cConn, sConn := net.Pipe()
	defer cConn.Close()
	defer sConn.Close()
	srv := tls.Server(sConn, &tls.Config{
		Certificates: []tls.Certificate{ca.issue(t, "inventory", "", x509.ExtKeyUsageServerAuth)},
		ClientAuth:   auth, ClientCAs: ca.pool, MinVersion: tls.VersionTLS13,
	})
	cli := tls.Client(cConn, &tls.Config{
		Certificates: []tls.Certificate{ca.issue(t, clientCN, clientURI, x509.ExtKeyUsageClientAuth)},
		RootCAs:      ca.pool, ServerName: "inventory", MinVersion: tls.VersionTLS13,
	})
	errc := make(chan error, 1)
	go func() { errc <- cli.Handshake() }()
	if err := srv.Handshake(); err != nil {
		t.Fatalf("серверное рукопожатие: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("клиентское рукопожатие: %v", err)
	}
	return srv.ConnectionState()
}

func TestMTLSAuth(t *testing.T) {
	ca := newTestCA(t)
	ic := MTLSAuth([]string{"spiffe://shop.local/orders", "billing"})
	var gotService string
	h := func(ctx context.Context, req any) (any, error) {
		gotService, _ = ServiceFromContext(ctx)
		return "ok", nil
	}
	run := func(p *Peer) error {
		gotService = ""
		ctx := context.Background()
		if p != nil {
			ctx = NewPeerContext(ctx, p)
		}
		_, err := ic(ctx, nil, &UnaryServerInfo{FullMethod: "/x"}, h)
		return err
	}

	st := handshake(t, ca, "orders-7f9c", "spiffe://shop.local/orders", tls.RequireAndVerifyClientCert)
	if err := run(&Peer{TLS: &st}); err != nil || gotService != "spiffe://shop.local/orders" {
		t.Errorf("SPIFFE ID: err=%v service=%q", err, gotService)
	}
	st = handshake(t, ca, "billing", "", tls.RequireAndVerifyClientCert)
	if err := run(&Peer{TLS: &st}); err != nil || gotService != "billing" {
		t.Errorf("CN без URI: err=%v service=%q", err, gotService)
	}
	st = handshake(t, ca, "analytics", "spiffe://shop.local/analytics", tls.RequireAndVerifyClientCert)
	if err := run(&Peer{TLS: &st}); CodeOf(err) != PermissionDenied {
		t.Errorf("чужой сервис: %v, ожидался PermissionDenied", err)
	}
	// CN совпадает с разрешённым, но у сертификата есть SPIFFE ID — главным считается он
	st = handshake(t, ca, "billing", "spiffe://shop.local/analytics", tls.RequireAndVerifyClientCert)
	if err := run(&Peer{TLS: &st}); CodeOf(err) != PermissionDenied {
		t.Errorf("CN billing + SPIFFE analytics: %v, ожидался PermissionDenied", err)
	}
	// сертификат запрошен, но не проверен
	st = handshake(t, ca, "billing", "", tls.RequireAnyClientCert)
	if len(st.PeerCertificates) == 0 || len(st.VerifiedChains) != 0 {
		t.Fatal("тест повреждён")
	}
	if err := run(&Peer{TLS: &st}); CodeOf(err) != Unauthenticated {
		t.Errorf("непроверенный сертификат: %v, ожидался Unauthenticated", err)
	}
	if err := run(&Peer{}); CodeOf(err) != Unauthenticated {
		t.Errorf("без TLS: %v, ожидался Unauthenticated", err)
	}
	if err := run(nil); CodeOf(err) != Unauthenticated {
		t.Errorf("без Peer: %v, ожидался Unauthenticated", err)
	}
}

func TestPerRPCCredentials(t *testing.T) {
	creds := StaticToken("t0k3n")
	if !creds.RequireTransportSecurity() {
		t.Error("токен должен требовать защищённого транспорта")
	}
	if _, err := AttachCredentials(context.Background(), creds, false, "https://inventory/"); !errors.Is(err, ErrInsecureTransport) {
		t.Errorf("по незащищённому каналу: %v, ожидалась ErrInsecureTransport", err)
	}
	orig := MD{"x-request-id": {"42"}}
	ctx := NewOutgoingContext(context.Background(), orig)
	ctx, err := AttachCredentials(ctx, creds, true, "https://inventory/")
	if err != nil {
		t.Fatal(err)
	}
	md, _ := FromOutgoingContext(ctx)
	if got := md.Get("authorization"); len(got) != 1 || got[0] != "Bearer t0k3n" {
		t.Errorf("authorization = %v", got)
	}
	if got := md.Get("x-request-id"); len(got) != 1 || got[0] != "42" {
		t.Errorf("существующие метаданные потеряны: %v", md)
	}
	if len(orig) != 1 {
		t.Errorf("исходная MD изменена: %v", orig)
	}
}

func TestEndToEndChain(t *testing.T) {
	ca := newTestCA(t)
	st := handshake(t, ca, "orders", "", tls.RequireAndVerifyClientCert)
	server := ChainUnary(
		Recovery(),
		MTLSAuth([]string{"orders"}),
		BearerAuth(validator),
		RequireScopes(map[string][]string{"/items.v1.Items/List": {"items:read"}, "/items.v1.Items/Boom": {}}),
	)

	// клиент: прикрепляет токен, «транспорт» переносит outgoing → incoming
	ctx, err := AttachCredentials(context.Background(), StaticToken("reader"), true, "")
	if err != nil {
		t.Fatal(err)
	}
	out, _ := FromOutgoingContext(ctx)
	srvCtx := NewPeerContext(NewIncomingContext(context.Background(), out), &Peer{TLS: &st})

	if resp, err := call(server, srvCtx, "/items.v1.Items/List"); err != nil || resp != "req" {
		t.Errorf("List: %v, %v", resp, err)
	}
	_, err = server(srvCtx, nil, &UnaryServerInfo{FullMethod: "/items.v1.Items/Boom"}, func(context.Context, any) (any, error) {
		var m map[string]int
		m["x"] = 1 // паника
		return nil, nil
	})
	if CodeOf(err) != Internal {
		t.Errorf("паника в обработчике: %v, ожидался Internal", err)
	}
}
