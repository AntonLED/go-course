//go:build !solution

// Package certchain — выпуск цепочки root → intermediate → leaf на ECDSA P-256
// и проверка цепочки доверия через crypto/x509.
package certchain

import (
	"crypto/x509"
	"time"
)

// NewRootCA выпускает самоподписанный корневой сертификат (ключ ECDSA P-256):
// Subject.CommonName = cn, IsCA, BasicConstraintsValid, MaxPathLen = 1,
// KeyUsage = CertSign | CRLSign, случайный 128-битный серийный номер.
func NewRootCA(cn string, notBefore, notAfter time.Time) (*KeyPair, error) {
	// TODO: ecdsa.GenerateKey, x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	return nil, nil
}

// NewIntermediate выпускает промежуточный CA, подписанный parent:
// IsCA, MaxPathLen = 0 (не забудьте MaxPathLenZero!), KeyUsage = CertSign | CRLSign.
func NewIntermediate(parent *KeyPair, cn string, notBefore, notAfter time.Time) (*KeyPair, error) {
	// TODO: реализуйте
	return nil, nil
}

// NewLeaf выпускает конечный серверный сертификат, подписанный parent:
// IsCA = false (BasicConstraintsValid = true), DNSNames = dnsNames,
// KeyUsage = DigitalSignature, ExtKeyUsage = [ServerAuth].
func NewLeaf(parent *KeyPair, cn string, dnsNames []string, notBefore, notAfter time.Time) (*KeyPair, error) {
	// TODO: реализуйте
	return nil, nil
}

// ParseCertsPEM разбирает все блоки "CERTIFICATE" из data (прочие блоки,
// например ключи, пропускает). Ни одного сертификата или битый DER → ErrBadPEM.
func ParseCertsPEM(data []byte) ([]*x509.Certificate, error) {
	// TODO: реализуйте (pem.Decode в цикле)
	return nil, ErrBadPEM
}

// VerifyChain проверяет первый сертификат из leafPEM:
//   - доверенные корни — только из rootsPEM (не системные);
//   - intermediatesPEM (может быть пустым) — для построения цепочки;
//   - dnsName (если не пуст) проверяется по SAN; время — now;
//   - требуемое назначение — ExtKeyUsageServerAuth.
//
// Ошибки: ErrBadPEM, ErrExpired, ErrUnknownAuthority, ErrHostname, ErrUsage,
// ErrConstraint (TooManyIntermediates, NotAuthorizedToSign, CANotAuthorizedForThisName) —
// обёрнутые вместе с исходной ошибкой x509 (fmt.Errorf("%w: %w", ...)).
func VerifyChain(leafPEM, intermediatesPEM, rootsPEM []byte, dnsName string, now time.Time) ([][]*x509.Certificate, error) {
	// TODO: реализуйте
	return nil, ErrUnknownAuthority
}

// SPKIFingerprint возвращает base64 (StdEncoding) от SHA-256 поля
// RawSubjectPublicKeyInfo — отпечаток для certificate/public key pinning.
func SPKIFingerprint(cert *x509.Certificate) string {
	// TODO: реализуйте
	return ""
}
