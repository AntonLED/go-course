//go:build solution

// Package certchain — выпуск цепочки root → intermediate → leaf на ECDSA P-256
// и проверка цепочки доверия через crypto/x509.
package certchain

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"time"
)

// randomSerial — случайный положительный 128-битный серийный номер
// (CA/B Forum требует ≥ 64 бит энтропии).
func randomSerial() (*big.Int, error) {
	return rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
}

// issue генерирует ключ, подписывает шаблон ключом parent (или самим собой,
// если parent == nil) и упаковывает результат.
func issue(tmpl *x509.Certificate, parent *KeyPair) (*KeyPair, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	tmpl.SerialNumber = serial

	issuerCert, signer := tmpl, key // самоподписанный
	if parent != nil {
		issuerCert, signer = parent.Cert, parent.Key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, issuerCert, &key.PublicKey, signer)
	if err != nil {
		return nil, fmt.Errorf("certchain: create certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &KeyPair{
		Cert:    cert,
		CertPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		Key:     key,
	}, nil
}

// NewRootCA выпускает самоподписанный корневой сертификат.
func NewRootCA(cn string, notBefore, notAfter time.Time) (*KeyPair, error) {
	return issue(&x509.Certificate{
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLen:            1, // под корнем допустим один промежуточный CA
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}, nil)
}

// NewIntermediate выпускает промежуточный CA, подписанный parent.
func NewIntermediate(parent *KeyPair, cn string, notBefore, notAfter time.Time) (*KeyPair, error) {
	return issue(&x509.Certificate{
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLen:            0,
		MaxPathLenZero:        true, // иначе MaxPathLen=0 означает «не ограничено»
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}, parent)
}

// NewLeaf выпускает конечный серверный сертификат.
func NewLeaf(parent *KeyPair, cn string, dnsNames []string, notBefore, notAfter time.Time) (*KeyPair, error) {
	return issue(&x509.Certificate{
		Subject:               pkix.Name{CommonName: cn},
		DNSNames:              dnsNames,
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		BasicConstraintsValid: true, // IsCA: false
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, parent)
}

// ParseCertsPEM разбирает все блоки CERTIFICATE (прочие блоки пропускаются).
func ParseCertsPEM(data []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		c, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrBadPEM, err)
		}
		certs = append(certs, c)
	}
	if len(certs) == 0 {
		return nil, ErrBadPEM
	}
	return certs, nil
}

// VerifyChain проверяет leaf по доверенным корням rootsPEM, используя
// intermediatesPEM (может быть пустым) для построения цепочки.
func VerifyChain(leafPEM, intermediatesPEM, rootsPEM []byte, dnsName string, now time.Time) ([][]*x509.Certificate, error) {
	leaves, err := ParseCertsPEM(leafPEM)
	if err != nil {
		return nil, fmt.Errorf("leaf: %w", err)
	}
	roots := x509.NewCertPool()
	rootCerts, err := ParseCertsPEM(rootsPEM)
	if err != nil {
		return nil, fmt.Errorf("roots: %w", err)
	}
	for _, c := range rootCerts {
		roots.AddCert(c)
	}
	inters := x509.NewCertPool()
	if len(intermediatesPEM) > 0 {
		ics, err := ParseCertsPEM(intermediatesPEM)
		if err != nil {
			return nil, fmt.Errorf("intermediates: %w", err)
		}
		for _, c := range ics {
			inters.AddCert(c)
		}
	}

	chains, err := leaves[0].Verify(x509.VerifyOptions{
		DNSName:       dnsName,
		Roots:         roots, // НЕ системные корни: доверяем только переданным
		Intermediates: inters,
		CurrentTime:   now,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	if err != nil {
		return nil, classify(err)
	}
	return chains, nil
}

// classify сопоставляет ошибку x509 с сентинелом, сохраняя исходную.
func classify(err error) error {
	var (
		inv  x509.CertificateInvalidError
		host x509.HostnameError
		ua   x509.UnknownAuthorityError
	)
	switch {
	case errors.As(err, &inv) && inv.Reason == x509.Expired:
		return fmt.Errorf("%w: %w", ErrExpired, err)
	case errors.As(err, &inv) && inv.Reason == x509.IncompatibleUsage:
		return fmt.Errorf("%w: %w", ErrUsage, err)
	case errors.As(err, &inv) && (inv.Reason == x509.TooManyIntermediates ||
		inv.Reason == x509.NotAuthorizedToSign || inv.Reason == x509.CANotAuthorizedForThisName):
		return fmt.Errorf("%w: %w", ErrConstraint, err)
	case errors.As(err, &host):
		return fmt.Errorf("%w: %w", ErrHostname, err)
	case errors.As(err, &ua):
		return fmt.Errorf("%w: %w", ErrUnknownAuthority, err)
	default:
		return err
	}
}

// SPKIFingerprint — отпечаток открытого ключа для pinning:
// base64(sha256(SubjectPublicKeyInfo)). Не меняется при перевыпуске
// сертификата с тем же ключом.
func SPKIFingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return base64.StdEncoding.EncodeToString(sum[:])
}
