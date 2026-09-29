# 06. OAuth 2.0 клиент: authorization code + PKCE

Задача к уроку [«Аутентификация и авторизация (OAuth 2.0)»](../../lessons/05-oauth2.md).

Кнопка «Войти через…» скрывает за собой целый протокол: редирект на сервер авторизации, возврат с кодом, обмен кода на токен, а потом ещё и обновление токена. В Go всё это обычно делает `golang.org/x/oauth2`. Здесь вы повторите его работу сами: построите ссылку авторизации с PKCE, обработаете callback с проверкой `state`, обменяете код на токен, а также реализуете client credentials и refresh.

## Что сделать

Код пишется в `oauthclient.go`. `Config`, `Token` и ошибки уже есть в `types.go`.

```go
func NewVerifier() (string, error)
func NewState() (string, error)
func ChallengeS256(verifier string) string
func (c *Config) AuthCodeURL(state, challenge string) (string, error)
func (c *Config) HandleCallback(r *http.Request, expectedState string) (code string, err error)
func (c *Config) Exchange(ctx context.Context, code, verifier string) (*Token, error)
func (c *Config) ClientCredentials(ctx context.Context) (*Token, error)
func (c *Config) Refresh(ctx context.Context, refreshToken string) (*Token, error)
```

Поток authorization code + PKCE выглядит так:

```
браузер ─▶ /authorize?response_type=code&client_id&redirect_uri&scope&state&code_challenge&code_challenge_method=S256
        ◀─ 302 redirect_uri?code=...&state=...
клиент  ─▶ POST /token  grant_type=authorization_code&code&redirect_uri&code_verifier
        ◀─ {"access_token":..,"token_type":"Bearer","expires_in":3600,"refresh_token":..}
```

### PKCE

Клиент придумывает случайный `code_verifier`, а на сервер авторизации сначала отправляет только его хеш:

$$
\text{code\_challenge} = \mathrm{BASE64URL\text{-}NOPAD}\bigl(\mathrm{SHA256}(\text{code\_verifier})\bigr)
$$

Проверить себя можно по тест-вектору из RFC 7636, приложение B: verifier `dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk` даёт challenge `E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM`.

### Остальные требования

- `AuthCodeURL` сохраняет query-параметры, которые уже были в `AuthURL`. Несколько `scope` объединяются через пробел.
- `HandleCallback` проверяет всё в строгом порядке. Сначала `state`: сравнение за постоянное время, а пустой ожидаемый `state` всегда даёт `ErrStateMismatch`. Затем параметр `error`, который превращается в `*AuthError`. И только потом `code`.
- Запросы к token endpoint отправляются как `application/x-www-form-urlencoded` с заголовком `Accept: application/json`. Конфиденциальный клиент аутентифицируется через Basic с `url.QueryEscape(id):url.QueryEscape(secret)`, публичный передаёт `client_id` в теле. Ответ не с кодом 200 превращается в `*TokenError`. Поле `token_type` должно равняться `Bearer` без учёта регистра, а `Expiry` вычисляется как `Now + expires_in`.
- Если при `Refresh` сервер не прислал новый `refresh_token`, оставьте старый.

## Подвохи

Сравнение `"" == ""` истинно. Если приложение забыло сохранить `state` в сессии, наивная проверка сравнит две пустые строки и пропустит атаку login CSRF. Именно поэтому пустой ожидаемый `state` — всегда ошибка.

Спецсимволы в `client_secret` (`:`, `+`, `/`) ломают Basic-аутентификацию, если перед этим не выполнить form-urlencode, как требует RFC 6749 §2.3.1.

`redirect_uri` при обмене кода должен совпадать с тем, что был передан в `/authorize`.

Параметры token endpoint передаются в теле запроса, а не в query: секреты не должны попадать в логи и заголовок Referer.

Зачем вообще нужен PKCE? Без него перехваченный код — через лог или через чужое приложение, зарегистрировавшее ту же custom scheme, — можно обменять на токен. С PKCE для обмена нужен ещё `code_verifier`, а он никогда не покидал клиента.

## Запуск проверки

```bash
go test -race ./modules/08-security/tasks/06_oauthclient/
go test -race -tags solution ./modules/08-security/tasks/06_oauthclient/   # эталон
```
