package certchain

import (
	"crypto/ecdsa"
	"crypto/x509"
	"errors"
)

// KeyPair — сертификат и его закрытый ключ.
type KeyPair struct {
	Cert    *x509.Certificate
	CertPEM []byte // PEM-блок "CERTIFICATE"
	Key     *ecdsa.PrivateKey
}

// Ошибки VerifyChain. Возвращаемая ошибка должна удовлетворять errors.Is с
// одной из них и при этом сохранять исходную ошибку crypto/x509 (errors.As).
var (
	ErrBadPEM           = errors.New("certchain: no valid certificate in PEM")
	ErrExpired          = errors.New("certchain: certificate expired or not yet valid")
	ErrUnknownAuthority = errors.New("certchain: certificate signed by unknown authority")
	ErrHostname         = errors.New("certchain: certificate is not valid for host")
	ErrUsage            = errors.New("certchain: certificate not valid for this usage")
	// ErrConstraint — нарушены ограничения CA (например, MaxPathLen).
	ErrConstraint = errors.New("certchain: chain violates CA constraints")
)
