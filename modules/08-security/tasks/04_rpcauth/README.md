# 04. Аутентификация и авторизация RPC через интерсепторы

Задача к уроку [«Безопасность в gRPC»](../../lessons/03-grpc-security.md).

В реальном gRPC-сервисе проверки безопасности не размазаны по обработчикам, а собраны в цепочку интерсепторов: один ловит паники, другой смотрит на клиентский сертификат, третий проверяет токен, четвёртый — права на конкретный метод. Чтобы не тянуть grpc-go в курс, в `types.go` в миниатюре повторён его API: `metadata.MD`, `peer.Peer`, `status`/`codes`, `UnaryServerInterceptor`, `credentials.PerRPCCredentials`. Ваша задача — написать поверх него те же интерсепторы, что пишут в настоящих сервисах.

## Что сделать

Код пишется в `rpcauth.go`.

```go
func ChainUnary(interceptors ...UnaryServerInterceptor) UnaryServerInterceptor
func Recovery() UnaryServerInterceptor
func MTLSAuth(allowed []string) UnaryServerInterceptor
func BearerAuth(validate func(ctx context.Context, token string) (Principal, error), skipMethods ...string) UnaryServerInterceptor
func RequireScopes(methodScopes map[string][]string) UnaryServerInterceptor
func PrincipalFromContext(ctx context.Context) (Principal, bool)
func ServiceFromContext(ctx context.Context) (string, bool)
func StaticToken(token string) PerRPCCredentials
func AttachCredentials(ctx context.Context, creds PerRPCCredentials, secure bool, uri string) (context.Context, error)
```

| Интерсептор | Поведение |
|---|---|
| `ChainUnary(a, b, c)` | `a` — самый внешний: `a> b> c> handler <c <b <a`; любой может прервать цепочку |
| `Recovery` | паника → `Internal` без текста паники |
| `MTLSAuth` | идентичность из проверенного сертификата (`VerifiedChains[0][0]`): SPIFFE URI SAN, иначе CN; нет → `Unauthenticated`; не в списке → `PermissionDenied` |
| `BearerAuth` | ровно один `authorization: Bearer <token>` (схема без учёта регистра); ошибка → `Unauthenticated` с текстом `invalid token`; `skipMethods` — без проверки |
| `RequireScopes` | метода нет в карте → `PermissionDenied` (deny by default); нет Principal → `Unauthenticated`; нет scope → `PermissionDenied` |

Клиентская сторона проще. `StaticToken` возвращает метаданные `{"authorization": "Bearer <token>"}` и требует защищённого транспорта. `AttachCredentials` делает то же, что клиент grpc-go перед каждым вызовом: отказывается отправлять токен по незащищённому каналу (возвращает `ErrInsecureTransport`), дописывает метаданные к исходящему контексту и при этом не мутирует исходную `MD`.

Для сравнения — так то же самое выглядит в настоящем grpc-go:

```go
creds := credentials.NewTLS(&tls.Config{Certificates: ..., ClientCAs: pool, ClientAuth: tls.RequireAndVerifyClientCert})
s := grpc.NewServer(grpc.Creds(creds), grpc.ChainUnaryInterceptor(recovery, mtlsAuth, bearerAuth, requireScopes))

p, _ := peer.FromContext(ctx)
tlsInfo := p.AuthInfo.(credentials.TLSInfo)
leaf := tlsInfo.State.VerifiedChains[0][0]
```

## Подвохи

В gRPC нет кодов 401 и 403, их роль играют `Unauthenticated` («кто ты?») и `PermissionDenied` («знаю тебя, но нельзя»). Путать их не стоит: клиент по-разному реагирует на «перелогинься» и «тебе сюда нельзя».

`RequireAnyClientCert` заполняет `PeerCertificates` без всякой проверки, поэтому доверяйте только `VerifiedChains`.

Если у сертификата есть SPIFFE ID, CN не учитывается вовсе. Иначе сертификат с `CN=billing` и чужим SPIFFE ID пройдёт как `billing`.

Не рассказывайте клиенту, почему токен невалиден — истёк, не та подпись или неизвестный kid. Атакующему эта подсказка полезнее, чем честному клиенту.

И следите за замыканиями в `ChainUnary`: каждой итерации цикла нужны свои копии переменных `interceptor` и `next`.

## Запуск проверки

```bash
go test -race ./modules/08-security/tasks/04_rpcauth/
go test -race -tags solution ./modules/08-security/tasks/04_rpcauth/   # эталон
```
