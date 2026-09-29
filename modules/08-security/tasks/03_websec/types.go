package websec

import "errors"

// Имена cookie и полей. Префикс __Host- заставляет браузер требовать
// Secure, Path=/ и отсутствие Domain — cookie нельзя подменить с поддомена.
const (
	SessionCookieName = "__Host-session"
	CSRFCookieName    = "__Host-csrf"
	CSRFHeaderName    = "X-CSRF-Token"
	CSRFFormField     = "csrf_token"
)

// ErrUnsafePath — путь пытается выйти за пределы корня или некорректен.
var ErrUnsafePath = errors.New("websec: unsafe path")
