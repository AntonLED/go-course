package jwt

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	now    = time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	secret = []byte("0123456789abcdef0123456789abcdef")
	enc    = base64.RawURLEncoding
)

var rsaKey = sync.OnceValue(func() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
})

func ecKey(t *testing.T) *ecdsa.PrivateKey {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func validClaims() Claims {
	return Claims{
		Issuer: "https://auth.example.com", Subject: "user-42", Audience: Audience{"orders-api"},
		ExpiresAt: now.Add(time.Hour).Unix(), IssuedAt: now.Unix(), Scope: "orders:read",
	}
}

func verifier() *Verifier {
	return &Verifier{
		HMACKeys: map[string][]byte{"h1": secret},
		Issuer:   "https://auth.example.com", Audience: "orders-api",
		Now: func() time.Time { return now },
	}
}

func decodePart(t *testing.T, s string) map[string]any {
	t.Helper()
	b, err := enc.DecodeString(s)
	if err != nil {
		t.Fatalf("часть %q — не base64url без паддинга: %v", s, err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("часть не JSON: %s", b)
	}
	return m
}

func TestSignHS256Format(t *testing.T) {
	tok, err := SignHS256(validClaims(), "h1", secret)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 || strings.ContainsAny(tok, "=+/") {
		t.Fatalf("токен %q: ожидалось 3 части в base64url без паддинга", tok)
	}
	h := decodePart(t, parts[0])
	if h["alg"] != "HS256" || h["typ"] != "JWT" || h["kid"] != "h1" {
		t.Errorf("заголовок %v", h)
	}
	p := decodePart(t, parts[1])
	if p["sub"] != "user-42" || p["aud"] != "orders-api" || p["exp"] != float64(now.Add(time.Hour).Unix()) {
		t.Errorf("payload %v (одна аудитория пишется строкой)", p)
	}
	if _, ok := p["nbf"]; ok {
		t.Error("незаданные claims не должны попадать в payload")
	}
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(parts[0] + "." + parts[1]))
	if parts[2] != enc.EncodeToString(m.Sum(nil)) {
		t.Error("подпись HS256 не совпадает с HMAC-SHA256(header.payload)")
	}
	// без kid поле отсутствует
	tok, _ = SignHS256(validClaims(), "", secret)
	if _, ok := decodePart(t, strings.Split(tok, ".")[0])["kid"]; ok {
		t.Error("пустой kid не должен попадать в заголовок")
	}
}

func TestJWTIOVector(t *testing.T) {
	// Классический пример с jwt.io: подпись корректна, но exp нет.
	const tok = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9." +
		"eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiaWF0IjoxNTE2MjM5MDIyfQ." +
		"SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c"
	v := &Verifier{HMACKeys: map[string][]byte{"": []byte("your-256-bit-secret")}, Now: func() time.Time { return now }}
	if _, err := v.Verify(tok); !errors.Is(err, ErrMissingExp) {
		t.Errorf("верный секрет: %v, ожидалась ErrMissingExp (подпись проверяется раньше claims)", err)
	}
	v.HMACKeys[""] = []byte("wrong")
	if _, err := v.Verify(tok); !errors.Is(err, ErrSignature) {
		t.Errorf("неверный секрет: %v, ожидалась ErrSignature", err)
	}
}

func TestVerifyHS256(t *testing.T) {
	tok, _ := SignHS256(validClaims(), "h1", secret)
	c, err := verifier().Verify(tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if c.Subject != "user-42" || c.Scope != "orders:read" || len(c.Audience) != 1 {
		t.Errorf("claims %+v", c)
	}
}

func mutate(f func(*Claims)) Claims {
	c := validClaims()
	f(&c)
	return c
}

func TestVerifyClaims(t *testing.T) {
	cases := []struct {
		name   string
		claims Claims
		leeway time.Duration
		want   error
	}{
		{"нет exp", mutate(func(c *Claims) { c.ExpiresAt = 0 }), 0, ErrMissingExp},
		{"истёк", mutate(func(c *Claims) { c.ExpiresAt = now.Add(-time.Second).Unix() }), 0, ErrExpired},
		{"exp == now — уже истёк", mutate(func(c *Claims) { c.ExpiresAt = now.Unix() }), 0, ErrExpired},
		{"истёк, но в пределах leeway", mutate(func(c *Claims) { c.ExpiresAt = now.Add(-20 * time.Second).Unix() }), 30 * time.Second, nil},
		{"истёк сильнее leeway", mutate(func(c *Claims) { c.ExpiresAt = now.Add(-time.Minute).Unix() }), 30 * time.Second, ErrExpired},
		{"nbf в будущем", mutate(func(c *Claims) { c.NotBefore = now.Add(time.Minute).Unix() }), 0, ErrNotYetValid},
		{"nbf в будущем в пределах leeway", mutate(func(c *Claims) { c.NotBefore = now.Add(10 * time.Second).Unix() }), 30 * time.Second, nil},
		{"nbf == now", mutate(func(c *Claims) { c.NotBefore = now.Unix() }), 0, nil},
		{"чужой iss", mutate(func(c *Claims) { c.Issuer = "https://evil.com" }), 0, ErrIssuer},
		{"нет iss", mutate(func(c *Claims) { c.Issuer = "" }), 0, ErrIssuer},
		{"чужая aud", mutate(func(c *Claims) { c.Audience = Audience{"billing-api"} }), 0, ErrAudience},
		{"нет aud", mutate(func(c *Claims) { c.Audience = nil }), 0, ErrAudience},
		{"aud-массив", mutate(func(c *Claims) { c.Audience = Audience{"billing-api", "orders-api"} }), 0, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tok, _ := SignHS256(c.claims, "h1", secret)
			v := verifier()
			v.Leeway = c.leeway
			_, err := v.Verify(tok)
			if c.want == nil && err != nil || c.want != nil && !errors.Is(err, c.want) {
				t.Errorf("Verify: %v, ожидалось %v", err, c.want)
			}
		})
	}
}

func forge(header, payload map[string]any, sig []byte) string {
	h, _ := json.Marshal(header)
	p, _ := json.Marshal(payload)
	return enc.EncodeToString(h) + "." + enc.EncodeToString(p) + "." + enc.EncodeToString(sig)
}

func hs(key []byte, header, payload map[string]any) string {
	unsigned := forge(header, payload, nil)
	si := unsigned[:len(unsigned)-1]
	m := hmac.New(sha256.New, key)
	m.Write([]byte(si))
	return si + "." + enc.EncodeToString(m.Sum(nil))
}

func payload() map[string]any {
	return map[string]any{"iss": "https://auth.example.com", "sub": "admin", "aud": "orders-api", "exp": now.Add(time.Hour).Unix()}
}

func TestAttacks(t *testing.T) {
	good, _ := SignHS256(validClaims(), "h1", secret)
	parts := strings.Split(good, ".")

	v := verifier()
	v.RSAKeys = map[string]*rsa.PublicKey{"r1": &rsaKey().PublicKey}
	pubDER, _ := x509.MarshalPKIXPublicKey(&rsaKey().PublicKey)
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubDER})

	// подменённый payload с исходной подписью
	evilPayload, _ := json.Marshal(payload())
	tampered := parts[0] + "." + enc.EncodeToString(evilPayload) + "." + parts[2]

	cases := []struct {
		name string
		tok  string
		want error
	}{
		{"alg none", forge(map[string]any{"alg": "none", "typ": "JWT"}, payload(), nil), ErrUnsupportedAlg},
		{"alg None", forge(map[string]any{"alg": "None"}, payload(), nil), ErrUnsupportedAlg},
		{"alg NONE с kid", forge(map[string]any{"alg": "NONE", "kid": "h1"}, payload(), nil), ErrUnsupportedAlg},
		{"alg HS512", hs(secret, map[string]any{"alg": "HS512", "kid": "h1"}, payload()), ErrUnsupportedAlg},
		{"подмена RS256→HS256 (публичный ключ как секрет)", hs(pubPEM, map[string]any{"alg": "HS256", "kid": "r1"}, payload()), ErrUnknownKey},
		{"подмена с DER-ключом", hs(pubDER, map[string]any{"alg": "HS256", "kid": "r1"}, payload()), ErrUnknownKey},
		{"crit с неподдерживаемым расширением", hs(secret, map[string]any{"alg": "HS256", "kid": "h1", "crit": []string{"x-ext"}, "x-ext": 1}, payload()), ErrMalformed},
		{"неизвестный kid", hs(secret, map[string]any{"alg": "HS256", "kid": "h9"}, payload()), ErrUnknownKey},
		{"подменённый payload", tampered, ErrSignature},
		{"пустая подпись", parts[0] + "." + parts[1] + ".", ErrSignature},
		{"2 части", parts[0] + "." + parts[1], ErrMalformed},
		{"4 части", good + ".x", ErrMalformed},
		{"паддинг в base64", parts[0] + "=." + parts[1] + "." + parts[2], ErrMalformed},
		{"заголовок не JSON", enc.EncodeToString([]byte("hello")) + "." + parts[1] + "." + parts[2], ErrMalformed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := v.Verify(c.tok); !errors.Is(err, c.want) {
				t.Errorf("Verify: %v, ожидалась %v", err, c.want)
			}
		})
	}
}

func TestRS256(t *testing.T) {
	tok, err := SignRS256(validClaims(), "r1", rsaKey())
	if err != nil {
		t.Fatal(err)
	}
	if h := decodePart(t, strings.Split(tok, ".")[0]); h["alg"] != "RS256" || h["kid"] != "r1" {
		t.Errorf("заголовок %v", h)
	}
	v := verifier()
	v.RSAKeys = map[string]*rsa.PublicKey{"r1": &rsaKey().PublicKey}
	if _, err := v.Verify(tok); err != nil {
		t.Errorf("Verify RS256: %v", err)
	}
	other, _ := rsa.GenerateKey(rand.Reader, 1024) // только для проверки «не тот ключ»
	v.RSAKeys["r1"] = &other.PublicKey
	if _, err := v.Verify(tok); !errors.Is(err, ErrSignature) {
		t.Errorf("не тот RSA-ключ: %v, ожидалась ErrSignature", err)
	}
}

func TestES256(t *testing.T) {
	key := ecKey(t)
	tok, err := SignES256(validClaims(), "e1", key)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(tok, ".")
	sig, _ := enc.DecodeString(parts[2])
	if len(sig) != 64 {
		t.Fatalf("подпись ES256 длиной %d, ожидалось 64 (R||S, не DER)", len(sig))
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&key.PublicKey, sum[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Error("подпись ES256 не проверяется как R||S")
	}

	v := verifier()
	v.ECKeys = map[string]*ecdsa.PublicKey{"e1": &key.PublicKey}
	if _, err := v.Verify(tok); err != nil {
		t.Errorf("Verify ES256: %v", err)
	}
	// DER-подпись (как у ecdsa.SignASN1) должна отвергаться
	der, _ := ecdsa.SignASN1(rand.Reader, key, sum[:])
	if _, err := v.Verify(parts[0] + "." + parts[1] + "." + enc.EncodeToString(der)); !errors.Is(err, ErrSignature) {
		t.Errorf("DER-подпись: %v, ожидалась ErrSignature", err)
	}
	// ES256-токен с kid RSA-ключа
	v.RSAKeys = map[string]*rsa.PublicKey{"r1": &rsaKey().PublicKey}
	tok2, _ := SignES256(validClaims(), "r1", key)
	if _, err := v.Verify(tok2); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("ES256 с kid RSA-ключа: %v, ожидалась ErrUnknownKey", err)
	}
}

func TestKeyRotation(t *testing.T) {
	oldTok, _ := SignHS256(validClaims(), "2024-12", []byte("old-secret-old-secret-old-secret"))
	newTok, _ := SignHS256(validClaims(), "2025-06", []byte("new-secret-new-secret-new-secret"))
	v := verifier()
	v.HMACKeys = map[string][]byte{
		"2024-12": []byte("old-secret-old-secret-old-secret"),
		"2025-06": []byte("new-secret-new-secret-new-secret"),
	}
	for _, tok := range []string{oldTok, newTok} {
		if _, err := v.Verify(tok); err != nil {
			t.Errorf("в период ротации оба ключа действуют: %v", err)
		}
	}
	delete(v.HMACKeys, "2024-12")
	if _, err := v.Verify(oldTok); !errors.Is(err, ErrUnknownKey) {
		t.Errorf("отозванный ключ: %v, ожидалась ErrUnknownKey", err)
	}
}

func jwksJSON(t *testing.T, keys ...map[string]string) []byte {
	b, err := json.Marshal(map[string]any{"keys": keys})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func rsaJWK(kid string, pub *rsa.PublicKey) map[string]string {
	return map[string]string{"kty": "RSA", "kid": kid, "use": "sig", "alg": "RS256",
		"n": enc.EncodeToString(pub.N.Bytes()), "e": enc.EncodeToString(big.NewInt(int64(pub.E)).Bytes())}
}

func ecJWK(kid string, pub *ecdsa.PublicKey) map[string]string {
	x, y := make([]byte, 32), make([]byte, 32)
	pub.X.FillBytes(x)
	pub.Y.FillBytes(y)
	return map[string]string{"kty": "EC", "kid": kid, "crv": "P-256", "x": enc.EncodeToString(x), "y": enc.EncodeToString(y)}
}

func TestJWKS(t *testing.T) {
	ek := ecKey(t)
	v := verifier()
	encKey := rsaJWK("enc1", &rsaKey().PublicKey)
	encKey["use"] = "enc"
	data := jwksJSON(t,
		rsaJWK("r1", &rsaKey().PublicKey),
		ecJWK("e1", &ek.PublicKey),
		encKey,
		map[string]string{"kty": "OKP", "kid": "ed", "crv": "Ed25519", "x": "AAAA"},
	)
	if err := v.AddJWKS(data); err != nil {
		t.Fatalf("AddJWKS: %v", err)
	}
	if _, ok := v.RSAKeys["enc1"]; ok {
		t.Error("ключ с use=enc не должен использоваться для подписи")
	}
	if len(v.HMACKeys) != 1 {
		t.Error("AddJWKS не должен трогать HMAC-ключи")
	}
	rt, _ := SignRS256(validClaims(), "r1", rsaKey())
	et, _ := SignES256(validClaims(), "e1", ek)
	for _, tok := range []string{rt, et} {
		if _, err := v.Verify(tok); err != nil {
			t.Errorf("токен с ключом из JWKS: %v", err)
		}
	}

	// в пустой Verifier (nil-карты)
	empty := &Verifier{Now: func() time.Time { return now }}
	if err := empty.AddJWKS(jwksJSON(t, ecJWK("e1", &ek.PublicKey))); err != nil || empty.ECKeys["e1"] == nil {
		t.Errorf("AddJWKS в Verifier с nil-картами: %v", err)
	}

	// точка не на кривой
	bad := ecJWK("bad", &ek.PublicKey)
	y, _ := enc.DecodeString(bad["y"])
	y[31] ^= 1
	bad["y"] = enc.EncodeToString(y)
	if err := (&Verifier{}).AddJWKS(jwksJSON(t, bad)); err == nil {
		t.Error("точка не на кривой P-256 должна отвергаться")
	}
	small, _ := rsa.GenerateKey(rand.Reader, 1024)
	if err := (&Verifier{}).AddJWKS(jwksJSON(t, rsaJWK("weak", &small.PublicKey))); err == nil {
		t.Error("RSA-ключ < 2048 бит должен отвергаться")
	}
	if err := (&Verifier{}).AddJWKS(jwksJSON(t, ecJWK("", &ek.PublicKey))); err == nil {
		t.Error("ключ без kid должен отвергаться")
	}
	if err := (&Verifier{}).AddJWKS([]byte(`{"keys": 5}`)); err == nil {
		t.Error("битый JSON должен давать ошибку")
	}
}

func TestAudienceJSON(t *testing.T) {
	var c Claims
	for in, want := range map[string]string{`{"aud":"a"}`: "[a]", `{"aud":["a","b"]}`: "[a b]"} {
		c = Claims{}
		if err := json.Unmarshal([]byte(in), &c); err != nil || fmt.Sprint(c.Audience) != want {
			t.Errorf("Unmarshal(%s) = %v, %v", in, c.Audience, err)
		}
	}
}

func BenchmarkVerifyHS256(b *testing.B) {
	tok, _ := SignHS256(validClaims(), "h1", secret)
	v := verifier()
	for i := 0; i < b.N; i++ {
		_, _ = v.Verify(tok)
	}
}
