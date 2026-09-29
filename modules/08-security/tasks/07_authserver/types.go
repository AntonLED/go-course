package authserver

import "time"

// Client — зарегистрированный OAuth-клиент.
type Client struct {
	ID           string
	Secret       string   // пусто — публичный клиент
	RedirectURIs []string // точные значения, без шаблонов
	Scopes       []string // scopes, которые клиенту разрешено запрашивать
}

// Options — настройки сервера. Нулевые значения — значения по умолчанию.
type Options struct {
	CodeTTL  time.Duration    // срок жизни authorization code (по умолчанию 60s)
	TokenTTL time.Duration    // срок жизни access token (по умолчанию 1h)
	Now      func() time.Time // часы (по умолчанию time.Now)
}

// UserHeader — заголовок, которым тесты «логинят» пользователя в /authorize
// (в настоящем сервере здесь была бы сессия и страница согласия).
const UserHeader = "X-User"
