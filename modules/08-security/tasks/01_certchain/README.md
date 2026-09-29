# 01. Цепочка сертификатов: выпуск и проверка

Задача к уроку [«TLS, сертификаты, цепочки сертификатов»](../../lessons/01-tls-certificates.md).

Внутри компании сервисы обычно доверяют не публичным центрам сертификации, а собственной PKI: корневой CA хранится где-то в сейфе, промежуточный подписывает сертификаты сервисов. Когда такая цепочка ломается, `x509` выдаёт ошибку, по которой не всегда понятно, что именно не так. В этой задаче вы соберёте маленькую PKI из трёх звеньев — root CA, intermediate CA и leaf — и напишете проверку цепочки доверия, которая объясняет причину отказа.

## Что сделать

Код пишется в `certchain.go`. Тип `KeyPair` и ошибки уже есть в `types.go`.

```go
func NewRootCA(cn string, notBefore, notAfter time.Time) (*KeyPair, error)
func NewIntermediate(parent *KeyPair, cn string, notBefore, notAfter time.Time) (*KeyPair, error)
func NewLeaf(parent *KeyPair, cn string, dnsNames []string, notBefore, notAfter time.Time) (*KeyPair, error)
func ParseCertsPEM(data []byte) ([]*x509.Certificate, error)
func VerifyChain(leafPEM, intermediatesPEM, rootsPEM []byte, dnsName string, now time.Time) ([][]*x509.Certificate, error)
func SPKIFingerprint(cert *x509.Certificate) string
```

У каждого звена свои обязательные поля:

| Сертификат | Ключевые поля |
|---|---|
| root | самоподписанный; `IsCA`, `BasicConstraintsValid`, `MaxPathLen: 1`, `KeyUsage: CertSign\|CRLSign` |
| intermediate | подписан root; `IsCA`, `MaxPathLen: 0` + `MaxPathLenZero: true`, `KeyUsage: CertSign\|CRLSign` |
| leaf | подписан intermediate; `IsCA: false` (`BasicConstraintsValid: true`), `DNSNames`, `KeyUsage: DigitalSignature`, `ExtKeyUsage: [ServerAuth]` |

Все ключи — ECDSA P-256, серийные номера — случайные 128-битные числа (`rand.Int`).

`VerifyChain` доверяет только корням из `rootsPEM` и вызывает проверку с `CurrentTime: now` и `KeyUsages: [ServerAuth]`. Возвращаемая ошибка должна отвечать сразу на два вопроса: через `errors.Is` она сравнивается с нашим сентинелом, а через `errors.As` из неё достаётся исходная ошибка `crypto/x509`.

| Ситуация | Сентинел | Ошибка x509 |
|---|---|---|
| нет сертификатов / битый DER | `ErrBadPEM` | — |
| просрочен / ещё не действует | `ErrExpired` | `CertificateInvalidError{Reason: Expired}` |
| не тот CA, нет intermediate, leaf в роли CA | `ErrUnknownAuthority` | `UnknownAuthorityError` |
| имя не совпадает с SAN | `ErrHostname` | `HostnameError` |
| нет `ServerAuth` в ExtKeyUsage | `ErrUsage` | `CertificateInvalidError{Reason: IncompatibleUsage}` |
| нарушен `MaxPathLen` и т.п. | `ErrConstraint` | `CertificateInvalidError{Reason: TooManyIntermediates}` |

`SPKIFingerprint` возвращает `base64(sha256(cert.RawSubjectPublicKeyInfo))` — тот же отпечаток, что используется в HPKP и `pin-sha256`.

## Подвохи

`MaxPathLen: 0` без `MaxPathLenZero: true` означает не «ноль промежуточных», а «ограничения нет». Это частая ошибка при выпуске intermediate.

Начиная с Go 1.15 CommonName больше не используется для проверки имени хоста — смотрят только на SAN (`DNSNames`). Сертификат с правильным CN, но без SAN, проверку не пройдёт.

Wildcard `*.svc.example.com` покрывает ровно один уровень: ни `a.b.svc.example.com`, ни сам `svc.example.com` под него не попадают.

Якорем доверия может быть и intermediate: корню не обязательно быть самоподписанным, достаточно оказаться в пуле доверенных.

Если оставить `VerifyOptions.Roots == nil`, Go возьмёт системные корни. Для внутренней PKI это совсем не то, что нужно.

И наконец, в PEM-файле могут встретиться посторонние блоки, например ключи. Их нужно пропускать, а не падать.

## Полезные команды

```bash
openssl x509 -in leaf.pem -noout -text        # посмотреть поля
openssl verify -CAfile root.pem -untrusted inter.pem leaf.pem
```

## Запуск проверки

```bash
go test -race ./modules/08-security/tasks/01_certchain/
go test -race -tags solution ./modules/08-security/tasks/01_certchain/   # эталон
```
