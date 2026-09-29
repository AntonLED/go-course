# Модуль 08. Вопросы безопасности

Рано или поздно каждый бэкенд-разработчик слышит на ревью вопрос «а это безопасно?», и отвечать на него приходится не общими словами, а конкретно: почему клиент доверяет этому сертификату, что будет, если токен украдут, чем плох этот `redirect_uri`. Этот модуль о том, как устроены механизмы, на которых держится безопасность сервисов: TLS и PKI, защита веб-приложений, безопасность gRPC, JWT, OAuth 2.0 и OpenID Connect.

В уроках мы показываем реальный API библиотек, с которыми вы встретитесь в работе: grpc-go, golang-jwt, x/oauth2 и x/crypto. А в задачах ключевые механизмы вы реализуете сами, на стандартной библиотеке: `crypto/x509`, `crypto/tls`, `crypto/hmac`, `crypto/ecdsa`, `crypto/rsa` и `net/http/httptest`. Написав проверку подписи JWT или PKCE руками, гораздо лучше понимаешь, от чего защищает библиотека и где её можно неправильно использовать.

## Цели

После модуля вы будете:

- понимать рукопожатие TLS 1.2 и 1.3, устройство X.509 и цепочек доверия, уметь выпустить и проверить сертификаты в Go;
- настраивать HTTPS и mTLS и защищаться от CSRF, XSS, path traversal, timing-атак и неправильного CORS;
- строить аутентификацию и авторизацию в gRPC через credentials и интерсепторы;
- знать, как устроен JWT и какие классические атаки на него существуют;
- понимать потоки OAuth 2.0 (authorization code с PKCE, client credentials) и OIDC с обеих сторон: со стороны клиента и со стороны сервера авторизации.

## Уроки

1. [TLS, сертификаты, цепочки сертификатов](lessons/01-tls-certificates.md)
2. [Безопасность в HTTP (HTTPS)](lessons/02-http-security.md)
3. [Безопасность в gRPC](lessons/03-grpc-security.md)
4. [Аутентификация и авторизация (JWT)](lessons/04-jwt.md)
5. [Аутентификация и авторизация (OAuth 2.0)](lessons/05-oauth2.md)

## Задачи

Проходить модуль удобнее всего по порядку: каждая задача опирается на урок, указанный в таблице, а последняя, «Задание 8», собирает всё вместе в один защищённый API.

| Задача | Урок | Суть |
|---|---|---|
| [01_certchain](tasks/01_certchain/) | 1 | Root → intermediate → leaf (ECDSA P-256), `VerifyChain` с классификацией ошибок, SPKI-пин |
| [02_mtls](tasks/02_mtls/) | 2 | HTTPS + mTLS на `httptest`, авторизация по CN/SAN/SPIFFE, 401 и 403 |
| [03_websec](tasks/03_websec/) | 2 | Security headers, secure cookie, CSRF double submit, CORS, `SafeJoin`, XSS |
| [04_rpcauth](tasks/04_rpcauth/) | 3 | Интерсепторы в стиле grpc-go: chain, recovery, mTLS identity, bearer, scopes, per-RPC credentials |
| [05_jwt](tasks/05_jwt/) | 4 | HS256/RS256/ES256 руками, `alg=none`, подмена алгоритма, kid-ротация, JWKS |
| [06_oauthclient](tasks/06_oauthclient/) | 5 | PKCE (вектор RFC 7636), authorize URL, callback со `state`, обмен кода, client credentials, refresh |
| [07_authserver](tasks/07_authserver/) | 5 | Мини authorization server: одноразовые коды, PKCE, точный `redirect_uri`, introspection |
| [08_project_secureapi](tasks/08_project_secureapi/) | **Задание 8** | Защищённый API: mTLS + JWT (scopes/роли) + rate limiting + корректные 401/403/429 |

Когда закончите с задачами, проверьте себя тестом по модулю: [quiz.json](quiz.json).

## Как проверять

Тесты запускаются как для вашего решения, так и для эталонного (с тегом `solution`). Флаг `-race` не лишний: в сервере авторизации и в итоговом проекте к хранилищу кодов и счётчикам rate limiting обращаются из многих горутин одновременно.

```bash
# ваше решение
go test -race ./modules/08-security/...
# эталоны
go test -race -tags solution ./modules/08-security/...
go vet ./modules/08-security/... && go vet -tags solution ./modules/08-security/...
```
