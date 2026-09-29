package certchain

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"math/big"
	"testing"
	"time"
)

var t0 = time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

type pki struct {
	root, inter, leaf *KeyPair
}

func mustPKI(t *testing.T) pki {
	t.Helper()
	root, err := NewRootCA("Test Root CA", t0.Add(-time.Hour), t0.AddDate(10, 0, 0))
	if err != nil || root == nil {
		t.Fatalf("NewRootCA: %v, %v", root, err)
	}
	inter, err := NewIntermediate(root, "Test Intermediate CA", t0.Add(-time.Hour), t0.AddDate(5, 0, 0))
	if err != nil || inter == nil {
		t.Fatalf("NewIntermediate: %v, %v", inter, err)
	}
	leaf, err := NewLeaf(inter, "api.example.com", []string{"api.example.com", "*.svc.example.com"}, t0.Add(-time.Hour), t0.AddDate(0, 3, 0))
	if err != nil || leaf == nil {
		t.Fatalf("NewLeaf: %v, %v", leaf, err)
	}
	return pki{root, inter, leaf}
}

func TestCertificateFields(t *testing.T) {
	p := mustPKI(t)

	r := p.root.Cert
	if !r.IsCA || !r.BasicConstraintsValid || r.MaxPathLen != 1 {
		t.Errorf("root: IsCA=%v BasicConstraintsValid=%v MaxPathLen=%d; ожидалось true, true, 1", r.IsCA, r.BasicConstraintsValid, r.MaxPathLen)
	}
	if r.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("root: нет KeyUsageCertSign")
	}
	if r.Subject.CommonName != "Test Root CA" || !bytes.Equal(r.RawSubject, r.RawIssuer) {
		t.Errorf("root: должен быть самоподписанным с CN %q", "Test Root CA")
	}
	if err := r.CheckSignatureFrom(r); err != nil {
		t.Errorf("root: подпись не проверяется собственным ключом: %v", err)
	}
	if _, ok := r.PublicKey.(*ecdsa.PublicKey); !ok || r.PublicKey.(*ecdsa.PublicKey).Curve != elliptic.P256() {
		t.Error("root: ожидался ключ ECDSA P-256")
	}

	i := p.inter.Cert
	if !i.IsCA || i.MaxPathLen != 0 || !i.MaxPathLenZero {
		t.Errorf("intermediate: IsCA=%v MaxPathLen=%d MaxPathLenZero=%v; ожидалось true, 0, true", i.IsCA, i.MaxPathLen, i.MaxPathLenZero)
	}
	if err := i.CheckSignatureFrom(r); err != nil {
		t.Errorf("intermediate не подписан root: %v", err)
	}
	if i.Issuer.CommonName != "Test Root CA" {
		t.Errorf("intermediate: Issuer %q", i.Issuer.CommonName)
	}

	l := p.leaf.Cert
	if l.IsCA || !l.BasicConstraintsValid {
		t.Errorf("leaf: IsCA=%v BasicConstraintsValid=%v; ожидалось false, true", l.IsCA, l.BasicConstraintsValid)
	}
	if l.KeyUsage != x509.KeyUsageDigitalSignature || len(l.ExtKeyUsage) != 1 || l.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
		t.Errorf("leaf: KeyUsage=%v ExtKeyUsage=%v", l.KeyUsage, l.ExtKeyUsage)
	}
	if len(l.DNSNames) != 2 || l.DNSNames[0] != "api.example.com" {
		t.Errorf("leaf: DNSNames=%v", l.DNSNames)
	}
	if err := l.CheckSignatureFrom(i); err != nil {
		t.Errorf("leaf не подписан intermediate: %v", err)
	}
	if !l.NotAfter.Equal(t0.AddDate(0, 3, 0)) {
		t.Errorf("leaf: NotAfter=%v", l.NotAfter)
	}

	// серийные номера случайные и положительные
	if r.SerialNumber.Sign() <= 0 || r.SerialNumber.Cmp(i.SerialNumber) == 0 || r.SerialNumber.BitLen() < 64 {
		t.Errorf("серийные номера должны быть случайными 128-битными: %v %v", r.SerialNumber, i.SerialNumber)
	}
	// PEM и ключ соответствуют сертификату
	block, _ := pem.Decode(p.leaf.CertPEM)
	if block == nil || block.Type != "CERTIFICATE" || !bytes.Equal(block.Bytes, l.Raw) {
		t.Error("CertPEM не соответствует Cert")
	}
	if !p.leaf.Key.PublicKey.Equal(l.PublicKey) {
		t.Error("Key не соответствует публичному ключу сертификата")
	}
}

func concat(bs ...[]byte) []byte { return bytes.Join(bs, nil) }

func TestVerifyChainOK(t *testing.T) {
	p := mustPKI(t)
	for _, host := range []string{"api.example.com", "db.svc.example.com", ""} {
		chains, err := VerifyChain(p.leaf.CertPEM, p.inter.CertPEM, p.root.CertPEM, host, t0)
		if err != nil {
			t.Fatalf("VerifyChain(%q): %v", host, err)
		}
		if len(chains) != 1 || len(chains[0]) != 3 {
			t.Fatalf("ожидалась одна цепочка из 3 сертификатов, получено %v", chains)
		}
		if chains[0][2].Subject.CommonName != "Test Root CA" {
			t.Errorf("цепочка должна заканчиваться корнем, а заканчивается %q", chains[0][2].Subject.CommonName)
		}
	}
	// в PEM могут быть посторонние блоки и несколько сертификатов
	keyBlock := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte{1, 2, 3}})
	other, _ := NewRootCA("Other", t0.Add(-time.Hour), t0.AddDate(1, 0, 0))
	if _, err := VerifyChain(concat(keyBlock, p.leaf.CertPEM), p.inter.CertPEM, concat(other.CertPEM, p.root.CertPEM), "api.example.com", t0); err != nil {
		t.Errorf("посторонние блоки PEM и несколько корней: %v", err)
	}
}

func TestVerifyChainErrors(t *testing.T) {
	p := mustPKI(t)
	foreign, _ := NewRootCA("Evil Root", t0.Add(-time.Hour), t0.AddDate(1, 0, 0))
	cnOnly, _ := NewLeaf(p.inter, "legacy.example.com", nil, t0.Add(-time.Hour), t0.AddDate(0, 1, 0))
	notYet, _ := NewLeaf(p.inter, "api.example.com", []string{"api.example.com"}, t0.Add(time.Hour), t0.AddDate(0, 1, 0))

	cases := []struct {
		name                string
		leaf, inters, roots []byte
		host                string
		now                 time.Time
		want                error
	}{
		{"просрочен", p.leaf.CertPEM, p.inter.CertPEM, p.root.CertPEM, "api.example.com", t0.AddDate(0, 4, 0), ErrExpired},
		{"ещё не действует", notYet.CertPEM, p.inter.CertPEM, p.root.CertPEM, "api.example.com", t0, ErrExpired},
		{"чужой CA", p.leaf.CertPEM, p.inter.CertPEM, foreign.CertPEM, "api.example.com", t0, ErrUnknownAuthority},
		{"нет intermediate", p.leaf.CertPEM, nil, p.root.CertPEM, "api.example.com", t0, ErrUnknownAuthority},
		{"intermediate вместо корня", p.leaf.CertPEM, nil, p.inter.CertPEM, "api.example.com", t0, nil},
		{"неверный SAN", p.leaf.CertPEM, p.inter.CertPEM, p.root.CertPEM, "evil.com", t0, ErrHostname},
		{"wildcard не покрывает 2 уровня", p.leaf.CertPEM, p.inter.CertPEM, p.root.CertPEM, "a.b.svc.example.com", t0, ErrHostname},
		{"wildcard не покрывает сам домен", p.leaf.CertPEM, p.inter.CertPEM, p.root.CertPEM, "svc.example.com", t0, ErrHostname},
		{"CN без SAN не считается", cnOnly.CertPEM, p.inter.CertPEM, p.root.CertPEM, "legacy.example.com", t0, ErrHostname},
		{"мусор вместо leaf", []byte("hello"), p.inter.CertPEM, p.root.CertPEM, "", t0, ErrBadPEM},
		{"пустые roots", p.leaf.CertPEM, p.inter.CertPEM, nil, "", t0, ErrBadPEM},
		{"битый DER", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: []byte{0x30, 0x03, 1, 2, 3}}), nil, p.root.CertPEM, "", t0, ErrBadPEM},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := VerifyChain(c.leaf, c.inters, c.roots, c.host, c.now)
			if c.want == nil {
				// доверять intermediate как якорю — легально: якорь не обязан быть самоподписанным
				if err != nil {
					t.Errorf("ошибка %v, ожидался успех", err)
				}
				return
			}
			if !errors.Is(err, c.want) {
				t.Errorf("ошибка %v, ожидалась %v", err, c.want)
			}
		})
	}
}

func TestVerifyChainKeepsX509Error(t *testing.T) {
	p := mustPKI(t)
	_, err := VerifyChain(p.leaf.CertPEM, p.inter.CertPEM, p.root.CertPEM, "evil.com", t0)
	var he x509.HostnameError
	if !errors.As(err, &he) || he.Host != "evil.com" {
		t.Errorf("ошибка %v должна содержать x509.HostnameError (оборачивайте через %%w)", err)
	}
	_, err = VerifyChain(p.leaf.CertPEM, p.inter.CertPEM, p.root.CertPEM, "", t0.AddDate(1, 0, 0))
	var ie x509.CertificateInvalidError
	if !errors.As(err, &ie) || ie.Reason != x509.Expired {
		t.Errorf("ошибка %v должна содержать x509.CertificateInvalidError{Reason: Expired}", err)
	}
}

// signRaw выпускает сертификат по произвольному шаблону (для «неправильных» цепочек).
func signRaw(t *testing.T, tmpl *x509.Certificate, parent *KeyPair) []byte {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl.SerialNumber = big.NewInt(time.Now().UnixNano())
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent.Cert, &key.PublicKey, parent.Key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestVerifyChainUsageAndConstraints(t *testing.T) {
	p := mustPKI(t)
	clientOnly := signRaw(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "client"}, DNSNames: []string{"api.example.com"},
		NotBefore: t0.Add(-time.Hour), NotAfter: t0.AddDate(0, 1, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}, p.inter)
	if _, err := VerifyChain(clientOnly, p.inter.CertPEM, p.root.CertPEM, "api.example.com", t0); !errors.Is(err, ErrUsage) {
		t.Errorf("сертификат только для ClientAuth: %v, ожидалась ErrUsage", err)
	}

	// Leaf пытается выступить в роли CA: цепочку построить нельзя.
	fake := signRaw(t, &x509.Certificate{
		Subject: pkix.Name{CommonName: "fake"}, DNSNames: []string{"bank.example.com"},
		NotBefore: t0.Add(-time.Hour), NotAfter: t0.AddDate(0, 1, 0),
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, p.leaf)
	if _, err := VerifyChain(fake, concat(p.leaf.CertPEM, p.inter.CertPEM), p.root.CertPEM, "bank.example.com", t0); !errors.Is(err, ErrUnknownAuthority) {
		t.Errorf("сертификат, подписанный leaf: %v, ожидалась ErrUnknownAuthority", err)
	}

	// Intermediate с MaxPathLen=0 не может подписывать другие CA.
	subCA := &x509.Certificate{
		Subject: pkix.Name{CommonName: "Sub CA"}, NotBefore: t0.Add(-time.Hour), NotAfter: t0.AddDate(1, 0, 0),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	subKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	subCA.SerialNumber = big.NewInt(42)
	der, err := x509.CreateCertificate(rand.Reader, subCA, p.inter.Cert, &subKey.PublicKey, p.inter.Key)
	if err != nil {
		t.Fatal(err)
	}
	subCert, _ := x509.ParseCertificate(der)
	sub := &KeyPair{Cert: subCert, Key: subKey, CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
	deep, err := NewLeaf(sub, "deep.example.com", []string{"deep.example.com"}, t0.Add(-time.Hour), t0.AddDate(0, 1, 0))
	if err != nil || deep == nil {
		t.Fatalf("NewLeaf: %v", err)
	}
	if _, err := VerifyChain(deep.CertPEM, concat(sub.CertPEM, p.inter.CertPEM), p.root.CertPEM, "deep.example.com", t0); !errors.Is(err, ErrConstraint) {
		t.Errorf("нарушение MaxPathLen: %v, ожидалась ErrConstraint", err)
	}
}

func TestSPKIFingerprint(t *testing.T) {
	p := mustPKI(t)
	sum := sha256.Sum256(p.leaf.Cert.RawSubjectPublicKeyInfo)
	want := base64.StdEncoding.EncodeToString(sum[:])
	if got := SPKIFingerprint(p.leaf.Cert); got != want {
		t.Errorf("SPKIFingerprint = %q, ожидалось %q", got, want)
	}
	if len(want) != 44 {
		t.Fatal("тест повреждён")
	}
	// перевыпуск с тем же ключом даёт тот же пин
	tmpl := *p.leaf.Cert
	tmpl.SerialNumber = big.NewInt(7)
	tmpl.NotAfter = t0.AddDate(1, 0, 0)
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, p.inter.Cert, &p.leaf.Key.PublicKey, p.inter.Key)
	if err != nil {
		t.Fatal(err)
	}
	renewed, _ := x509.ParseCertificate(der)
	if SPKIFingerprint(renewed) != SPKIFingerprint(p.leaf.Cert) {
		t.Error("пин по SPKI не должен меняться при перевыпуске с тем же ключом")
	}
	if SPKIFingerprint(p.root.Cert) == SPKIFingerprint(p.leaf.Cert) {
		t.Error("разные ключи — разные пины")
	}
}
