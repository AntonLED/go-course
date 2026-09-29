# 02. HTTPS и mTLS между сервисами

Задача к уроку [«Безопасность в HTTP (HTTPS)»](../../lessons/02-http-security.md).

В обычном HTTPS сертификат предъявляет только сервер. Между сервисами этого мало: складу `inventory` важно знать, кто именно к нему пришёл. Поэтому здесь используется взаимный TLS (mTLS), где сертификат предъявляют обе стороны. Сервис `inventory` должен принимать соединения только от сервисов с сертификатом внутреннего CA, а к API пускать только разрешённых — в нашем случае `orders`. Даже если у другого сервиса сертификат тоже валиден, это ещё не повод его пускать.

## Что сделать

Код пишется в `mtls.go`. Тип `Identity` уже есть в `types.go`.

```go
func ServerTLSConfig(cert tls.Certificate, clientCAs *x509.CertPool) *tls.Config
func ClientTLSConfig(clientCert *tls.Certificate, roots *x509.CertPool, serverName string) *tls.Config
func PeerIdentity(state *tls.ConnectionState) (Identity, bool)
func IdentityFromContext(ctx context.Context) (Identity, bool)
func RequireIdentity(allowed []string, next http.Handler) http.Handler
```

- **Сервер** использует `MinVersion: TLS 1.2`, `ClientAuth: RequireAndVerifyClientCert` и `ClientCAs`. Для TLS 1.2 разрешены только наборы `TLS_ECDHE_*` с GCM или ChaCha20-Poly1305, кривые — X25519 и P-256.
- **Клиент** доверяет только `roots`, а не системным корням, проверяет `serverName` и предъявляет `clientCert`, если он задан. Никакого `InsecureSkipVerify`.
- **PeerIdentity** достаёт CN, DNS SAN и URI SAN (SPIFFE ID) из проверенного сертификата, то есть из `state.VerifiedChains[0][0]`.
- **RequireIdentity** отвечает `401`, если проверенного сертификата нет, и `403`, если сертификат есть, но ни одно из его имён не входит в `allowed`. В остальных случаях `Identity` кладётся в контекст запроса и управление переходит к `next`.

В тестах всё это собирается так:

```go
srv := httptest.NewUnstartedServer(RequireIdentity([]string{"orders"}, h))
srv.TLS = ServerTLSConfig(serverCert, caPool)
srv.StartTLS()
client := &http.Client{Transport: &http.Transport{
    TLSClientConfig: ClientTLSConfig(&ordersCert, caPool, "inventory.internal"),
}}
```

## Подвохи

Аутентификация — это ещё не авторизация. Сервис `billing` с валидным сертификатом успешно проходит TLS-рукопожатие, но к API должен получить `403`.

Режимы `tls.RequestClientCert` и `RequireAnyClientCert` сертификат не проверяют: `PeerCertificates` заполняется тем, что прислал клиент, каким бы оно ни было. Доверять можно только `VerifiedChains`.

В TLS 1.3 сервер узнаёт, что клиентского сертификата нет, уже после того, как клиент посчитал рукопожатие завершённым. Поэтому у клиента ошибка всплывает только при первом чтении — впрочем, `http.Client` всё равно вернёт её из `Do`.

`CipherSuites` влияет только на TLS 1.2: наборы шифров TLS 1.3 в Go не настраиваются.

`ServerName` клиента сверяется с SAN сертификата сервера, а не с адресом в URL. Поэтому в тесте клиент ходит на `127.0.0.1`, а проверяет имя `inventory.internal`.

## Запуск проверки

```bash
go test -race ./modules/08-security/tasks/02_mtls/
go test -race -tags solution ./modules/08-security/tasks/02_mtls/   # эталон
```
