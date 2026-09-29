# 05. JWT руками: HS256, RS256, ES256, kid и JWKS

Задача к уроку [«Аутентификация и авторизация (JWT)»](../../lessons/04-jwt.md).

JWT выглядит простым форматом — три куска base64, разделённых точками. Но у него богатая история уязвимостей, и почти все они возникают не из-за слабой криптографии, а из-за того, что верификатор слишком доверяет заголовку токена. Библиотеки вроде `github.com/golang-jwt/jwt/v5` в курсе недоступны, поэтому выпуск и проверку токенов вы напишете сами на `crypto/hmac`, `crypto/rsa` и `crypto/ecdsa` — и заодно увидите, откуда берутся классические атаки.

## Что сделать

Код пишется в `jwt.go`. `Header`, `Claims`, `Audience` и ошибки уже есть в `types.go`.

```go
func SignHS256(c Claims, kid string, key []byte) (string, error)
func SignRS256(c Claims, kid string, key *rsa.PrivateKey) (string, error)
func SignES256(c Claims, kid string, key *ecdsa.PrivateKey) (string, error)

type Verifier struct {
    HMACKeys map[string][]byte
    RSAKeys  map[string]*rsa.PublicKey
    ECKeys   map[string]*ecdsa.PublicKey
    Issuer, Audience string
    Leeway   time.Duration
    Now      func() time.Time
}
func (v *Verifier) Verify(token string) (*Claims, error)
func (v *Verifier) AddJWKS(data []byte) error
```

Токен имеет вид `base64url(header).base64url(payload).base64url(signature)`, причём base64url используется без паддинга (`base64.RawURLEncoding`). Заголовок выглядит как `{"alg":"HS256","typ":"JWT","kid":"..."}`; если `kid` пустой, поле не пишется.

| alg | Подпись |
|---|---|
| HS256 | `HMAC-SHA256(key, header.payload)` |
| RS256 | `rsa.SignPKCS1v15(…, crypto.SHA256, sha256(header.payload))` |
| ES256 | ECDSA P-256 над SHA-256; подпись — 64 байта `R‖S` (RFC 7518 §3.4) |

### Порядок проверки в Verify

Порядок здесь принципиален: всё, что лежит в payload, нельзя читать, пока не проверена подпись. `Verify` проверяет шаги строго по очереди и на каждом возвращает свою ошибку:

1. Структура токена, включая запрет параметра `crit` в заголовке, — `ErrMalformed`.
2. Алгоритм — `ErrUnsupportedAlg`.
3. Ключ, который ищется по паре (alg, kid), — `ErrUnknownKey`.
4. Подпись — `ErrSignature`.
5. Только после этого разбирается payload. Поле `exp` обязательно: без него `ErrMissingExp`, истёкший токен — `ErrExpired`.
6. `nbf` — `ErrNotYetValid`.
7. `iss` — `ErrIssuer`.
8. `aud` — `ErrAudience`.

`exp` и `nbf` проверяются с допуском `Leeway`.

### AddJWKS

`AddJWKS` разбирает набор ключей вида `{"keys":[...]}` по RFC 7517. Поддерживаются RSA-ключи (`n`, `e`, длиной не меньше 2048 бит) и EC P-256 (`x` и `y` по 32 байта, точка обязана лежать на кривой). Ключи с `use`, отличным от `sig`, и ключи неизвестного `kty` пропускаются.

## Атаки, от которых защищают тесты

- **`alg: none`** (а также `None` и `NONE`) — «неподписанный» токен. Защита — разрешать только явный список алгоритмов.
- **Подмена RS256 на HS256.** Злоумышленник берёт *публичный* RSA-ключ сервера и использует его как HMAC-секрет. Если верификатор выбирает ключ по `kid`, а алгоритм — по заголовку, подпись сойдётся. Защищает от этого раздельное хранение ключей для каждого семейства алгоритмов.
- **Подмена payload** с сохранением старой подписи.
- **DER вместо `R‖S`** в ES256: `ecdsa.SignASN1` возвращает подпись в ASN.1, а JWS требует сырые 64 байта.
- **Невалидная точка EC** в JWKS (invalid curve attack) и слабые RSA-ключи.
- **`exp == now`.** Такой токен уже истёк: текущее время должно быть строго раньше `exp`.
- **`crit` в заголовке.** По RFC 7515 §4.1.11 токен с непонятыми критическими расширениями недействителен. Мы не поддерживаем ни одного расширения, значит, любой `crit` отвергается.

## Подвохи

HMAC сравнивайте через `hmac.Equal`, а не через `bytes.Equal` или `==`: только первый работает за постоянное время.

Поле `aud` бывает и строкой, и массивом строк. Это уже учтено в типе `Audience`.

`R` и `S` в ES256 нужно дополнять ведущими нулями до 32 байт — удобнее всего через `big.Int.FillBytes`. Без этого время от времени будет попадаться подпись короче 64 байт, и такой баг легко пропустить в ручном тестировании.

## Запуск проверки

```bash
go test -race ./modules/08-security/tasks/05_jwt/
go test -race -tags solution ./modules/08-security/tasks/05_jwt/   # эталон
```
