# Урок 5. Аутентификация и авторизация (OAuth 2.0)

Вы наверняка видели кнопку «Разрешить сервису X читать ваш календарь Google». Раньше такую интеграцию делали бы просто: сервис X спрашивал ваш пароль от Google и ходил с ним куда хотел. OAuth 2.0 (RFC 6749) появился, чтобы так делать было не нужно. Это протокол **делегированной авторизации**: пользователь разрешает приложению доступ к своим ресурсам на другом сервисе, не сообщая приложению свой пароль, причём доступ ограничен и отзываем.

При этом у OAuth есть граница, которую часто упускают. Сам по себе он **не протокол аутентификации**: токен доступа говорит «предъявителю разрешено читать календарь», но не говорит, кто этот предъявитель. Аутентификацию поверх OAuth даёт OpenID Connect, о котором речь пойдёт в конце урока.

## Роли

В каждом OAuth-потоке участвуют четыре стороны:

| Роль | Пример |
|---|---|
| Resource Owner | пользователь |
| Client | приложение, которое хочет доступ (SPA, мобильное приложение, backend) |
| Authorization Server (AS) | выдаёт токены: Keycloak, Auth0, Google, Hydra |
| Resource Server (RS) | API, принимающее access token |

Клиенты делятся на два вида, и от этого зависит многое в выборе потока. **Конфиденциальные** клиенты, например backend, могут хранить секрет. **Публичные** — SPA, мобильные и CLI-приложения — не могут: любой секрет, зашитый в код, который работает на устройстве пользователя, рано или поздно извлекут.

## Authorization Code + PKCE

Это основной поток для всех клиентов, за которыми стоит пользователь. Вот он по шагам:

```
1. Клиент: verifier = random(32 байта) → base64url (43 символа)
           challenge = BASE64URL(SHA256(verifier))
           state = random()                      ← сохраняем в сессии
2. Браузер → AS: GET /authorize?response_type=code&client_id=shop&redirect_uri=https://shop/cb
                 &scope=openid%20orders:read&state=…&code_challenge=…&code_challenge_method=S256
3. Пользователь логинится на AS и соглашается
4. AS → браузер: 302 https://shop/cb?code=SplxlOBe…&state=…
5. Клиент проверяет state и меняет код:
   POST /token  grant_type=authorization_code&code=…&redirect_uri=https://shop/cb&code_verifier=…
6. AS проверяет SHA256(verifier) == challenge и одноразовость кода → {access_token, refresh_token, id_token}
```

Идея потока в том, что токены никогда не проходят через браузер. Браузер переносит только короткоживущий одноразовый код, а обменивает его на токены сам клиент прямым запросом к AS.

![Тот же поток как диаграмма последовательности. Оранжевое идёт через браузер и может утечь; секретный `code_verifier` уходит только в прямом запросе клиента к AS, а access token — на resource server.](img/oauth-pkce-flow.svg)

Но код всё-таки проходит через браузер, а значит, может утечь: через логи, заголовок Referer, открытый редирект или вредоносное мобильное приложение, зарегистрировавшее тот же custom URL scheme. Для публичного клиента, у которого нет секрета, перехваченный код равносилен токену. Эту дыру закрывает **PKCE** (RFC 7636).

Перед началом потока клиент генерирует случайный `code_verifier` и держит его у себя. В запрос `/authorize` уходит только его хеш:

$$
\text{code\_challenge} = \mathrm{BASE64URL}\bigl(\mathrm{SHA256}(\mathrm{ASCII}(\text{code\_verifier}))\bigr)
$$

При обмене кода клиент предъявляет сам `code_verifier`, и AS проверяет, что $\mathrm{BASE64URL}(\mathrm{SHA256}(\text{code\_verifier})) = \text{code\_challenge}$. Хеш необратим, поэтому тот, кто видел редирект и знает `code_challenge`, восстановить verifier не может, а без verifier, который никогда не покидал клиента, перехваченный код на токен не обменять.

Про размер verifier. Из 32 случайных байт получается $256$ бит энтропии, а в base64url каждые 6 бит дают один символ, так что строка занимает $\lceil 256 / 6 \rceil = 43$ символа. Это ровно нижняя граница RFC 7636: verifier должен состоять из 43–128 символов набора unreserved (`A-Z a-z 0-9 - . _ ~`). Метод `plain`, при котором $\text{code\_challenge} = \text{code\_verifier}$, почти бесполезен: challenge передаётся через браузер, и тот, кто видел запрос `/authorize`, знает и verifier. Используйте `S256`.

Насколько PKCE обязателен? RFC 9700 требует его для публичных клиентов и рекомендует для конфиденциальных, а OAuth 2.1 (пока черновик) делает его обязательным практически для всех клиентов, включая конфиденциальные.

![Код и `code_challenge` видны всем, кто видел редирект, а `code_verifier` — только клиенту. Без него перехваченный код на токен не обменять.](img/pkce-intercept.svg)

Для проверки своей реализации используйте тест-вектор из RFC 7636, приложение B: из verifier `dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk` должен получиться challenge `E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM`.

## state против CSRF

Зачем нужен ещё и `state`, если есть PKCE? Представьте такую атаку. Злоумышленник запускает поток со своим аккаунтом, доходит до редиректа, получает код и не использует его, а подсовывает жертве ссылку `https://shop/cb?code=<код атакующего>`. Жертва переходит по ней и оказывается залогинена в аккаунт атакующего. Это называется login CSRF, и последствия бывают вполне материальными: жертва, например, привяжет к чужому аккаунту свою карту.

`state` — случайное значение, которое клиент привязывает к сессии браузера перед началом потока. При возврате на callback клиент сравнивает пришедший `state` с сохранённым, причём за постоянное время, и пустое значение никогда не должно проходить проверку. Ссылка атакующего содержит `state` из его сессии, а не из сессии жертвы, и отвергается. PKCE частично закрывает ту же дыру, а в OIDC для этого есть ещё и `nonce`.

## redirect_uri: только точное совпадение

Код отправляется туда, куда указывает `redirect_uri`, поэтому эта проверка на стороне AS критична. AS сравнивает `redirect_uri` с зарегистрированными значениями посимвольно, без префиксов и wildcard. Соблазнительная проверка `strings.HasPrefix(uri, "https://shop.com")` пропустит и `https://shop.com.evil.com`, и `https://shop.com/../evil`.

Не менее важно, что делать при несовпадении. Если клиент неизвестен или URI не совпал, AS показывает ошибку у себя и **не делает редирект**. Иначе сам сервер авторизации превращается в open redirect, через который уводят коды и токены. И последнее: в token-запросе `redirect_uri` должен совпадать с тем, что был передан в `/authorize`.

## Другие гранты

Authorization code подходит, когда есть пользователь и браузер. Для остальных ситуаций в OAuth есть другие гранты.

Когда пользователя нет вовсе и один сервис обращается к другому (machine-to-machine), используют **Client Credentials**. Клиент аутентифицируется своими учётными данными и получает токен от своего имени:
```
POST /token  Authorization: Basic base64(urlencode(id):urlencode(secret))
grant_type=client_credentials&scope=inventory:read
```

Для устройств без браузера или клавиатуры, вроде телевизора или CLI в духе `gh auth login`, есть **Device Authorization Grant** (RFC 8628). Устройство получает `device_code` и `user_code`, пользователь вводит `user_code` на другом устройстве, где есть браузер, а первое устройство тем временем опрашивает `/token` с `grant_type=urn:ietf:params:oauth:grant-type:device_code`. В ответ оно получает `authorization_pending` (пользователь ещё не подтвердил), `slow_down` (опрашивайте реже) или, наконец, токен.

![Device flow: устройство не может принять редирект, поэтому пользователь подтверждает вход на другом устройстве, а первое опрашивает `/token`, пока не получит токены. Пример `user_code` и `interval` взят из RFC 8628.](img/device-flow.svg)

Грант Refresh Token (`grant_type=refresh_token&refresh_token=…`) обменивает refresh token на новый access token. Для публичных клиентов его применяют с ротацией и обнаружением повторного использования. Если сервер не вернул в ответе новый refresh token, клиент продолжает пользоваться старым.

Два гранта из исходной спецификации считаются устаревшими и удалены в OAuth 2.1. В **Implicit** (`response_type=token`) токен приходил прямо во фрагменте URL (`#access_token=…`), оседал в истории браузера и в Referer, мог быть перехвачен, не был привязан к клиенту, а refresh token не выдавался вовсе. Существовал он потому, что SPA когда-то не могли сделать кросс-доменный POST на `/token`; с появлением CORS и PKCE этот поток стал ненужным. **Resource Owner Password Credentials** ещё хуже: приложение получает пароль пользователя, то есть делает ровно то, от чего OAuth должен был избавить.

## Scopes

`scope` — это список прав через пробел (`openid profile orders:read`), о которых просит клиент. Пользователь соглашается на них на странице AS, но AS вправе выдать меньше запрошенного и сообщает итог в поле `scope` ответа. Resource server, получив токен, проверяет, хватает ли scope для операции, и при нехватке отвечает `403` с заголовком `WWW-Authenticate: Bearer error="insufficient_scope"`.

Здесь легко ошибиться в модели. Scope ограничивает клиента («это приложение может читать заказы»), но не заменяет проверку прав пользователя на конкретный ресурс («этот заказ принадлежит этому пользователю»). Вторая проверка остаётся на вашей бизнес-логике.

## OpenID Connect

OIDC — это слой аутентификации поверх OAuth 2.0. Если клиент запросил scope `openid`, в ответе `/token` появляется **`id_token`**. Это JWT о пользователе, адресованный клиенту, с полями `iss`, `sub`, `aud` (равен `client_id`), `exp`, `iat`, `nonce`, `auth_time`, `email` и другими. Именно он сообщает клиенту, кто и как аутентифицировался.

Клиент обязан проверить `id_token`: подпись по JWKS провайдера, `iss`, совпадение `aud` с собственным `client_id`, `exp` и `nonce`. Частая ошибка — отправлять `id_token` в API. Так делать нельзя: для API предназначен `access_token` со своей аудиторией, а `id_token` адресован клиенту.

Остальное в OIDC — удобная инфраструктура. Документ discovery по адресу `/.well-known/openid-configuration` описывает endpoints, `jwks_uri` и поддерживаемые алгоритмы, так что клиенту достаточно знать адрес провайдера. Endpoint `/userinfo` возвращает профиль пользователя по access token.

![Из одного ответа `/token` приходят два токена для разных получателей: `id_token` адресован клиенту (`aud` = `client_id`), `access_token` — API. Отправлять `id_token` в API нельзя.](img/oidc-tokens.svg)

В Go клиентскую сторону OIDC собирают из `github.com/coreos/go-oidc/v3` и `golang.org/x/oauth2`:

```go
provider, _ := oidc.NewProvider(ctx, "https://accounts.google.com")
cfg := oauth2.Config{
    ClientID: id, ClientSecret: secret, RedirectURL: "https://shop/cb",
    Endpoint: provider.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "email"},
}
verifier := oauth2.GenerateVerifier()
url := cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))
// в обработчике /cb:
tok, err := cfg.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
rawID, _ := tok.Extra("id_token").(string)
idt, err := provider.Verifier(&oidc.Config{ClientID: id}).Verify(ctx, rawID)
```

После обмена `cfg.Client(ctx, tok)` возвращает `*http.Client`, который сам подставляет заголовок `Authorization` и обновляет истёкший токен. Для M2M-потока в `golang.org/x/oauth2/clientcredentials` есть `clientcredentials.Config`.

## Introspection и валидация на resource server

Resource server может проверить access token двумя способами, и выбор между ними — компромисс между скоростью и возможностью отзыва.

Первый способ — проверять локально. Токен в этом случае — JWT (его профиль для access token описан в RFC 9068), и resource server сам проверяет подпись по JWKS, `iss`, `aud`, `exp` и scope. Это быстро и не требует сетевых запросов, но отозвать такой токен можно только одним способом — дождаться, пока истечёт его короткий TTL.

Второй способ — introspection (RFC 7662). Resource server, аутентифицировавшись, отправляет `POST /introspect` с `token=…` и получает ответ вроде `{"active":true,"sub":…,"scope":…,"exp":…}`. Токен при этом может быть непрозрачной строкой, а отзыв срабатывает мгновенно. Платой становится запрос к AS на каждый вызов, поэтому результат обычно кэшируют на несколько секунд.

Для явного отзыва токена клиентом есть отдельный endpoint `POST /revoke` (RFC 7009).

## Типичные ошибки

- `state` отсутствует или не проверяется, в том числе классическое `"" == ""`, при котором пустой `state` проходит проверку.
- `redirect_uri` проверяется по префиксу, или AS делает редирект на непроверенный URI.
- Код можно обменять повторно. RFC 6749 требует одноразовости (повторный запрос MUST быть отклонён) и рекомендует (SHOULD) заодно отозвать токены, уже выданные по этому коду.
- `id_token` используется для доступа к API, или клиент не проверяет его подпись и `aud`.
- Секрет клиента зашит в SPA или мобильное приложение.
- Токены передаются в query string и поэтому утекают в логи и Referer. Параметры `/token` передаются только в теле POST.
- Ответы `/token` отдаются без `Cache-Control: no-store`.

## Вопросы с собеседований

1. Чем OAuth 2.0 отличается от OpenID Connect? Что такое `id_token` и чем он отличается от access token?
2. Как работает PKCE и какую атаку он предотвращает?
3. Зачем нужен параметр `state`, если есть PKCE?
4. Почему implicit flow признан устаревшим?
5. Какой грант выбрать для M2M-взаимодействия, для SPA и для Smart TV?
6. Как resource server проверяет токен и когда предпочесть introspection локальной проверке JWT?
7. Почему `redirect_uri` нужно сравнивать точно, а не по префиксу?

## Ссылки

- [RFC 6749 (OAuth 2.0)](https://www.rfc-editor.org/rfc/rfc6749), [RFC 7636 (PKCE)](https://www.rfc-editor.org/rfc/rfc7636), [RFC 8628 (Device)](https://www.rfc-editor.org/rfc/rfc8628), [RFC 7662 (Introspection)](https://www.rfc-editor.org/rfc/rfc7662)
- [OAuth 2.0 Security Best Current Practice (RFC 9700)](https://www.rfc-editor.org/rfc/rfc9700), [OAuth 2.1 draft](https://oauth.net/2.1/)
- [OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0.html)
- [pkg.go.dev/golang.org/x/oauth2](https://pkg.go.dev/golang.org/x/oauth2), [github.com/coreos/go-oidc](https://github.com/coreos/go-oidc)

## Практика

- [06_oauthclient](../tasks/06_oauthclient/) — PKCE, authorize URL, callback со `state`, обмен кода, client credentials, refresh.
- [07_authserver](../tasks/07_authserver/) — мини authorization server: одноразовые коды, PKCE, точный `redirect_uri`, introspection.
- [08_project_secureapi](../tasks/08_project_secureapi/) — Задание 8.
