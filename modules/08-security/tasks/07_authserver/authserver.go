//go:build !solution

// Package authserver — минимальный OAuth 2.0 authorization server:
// authorization code + PKCE (S256), client credentials, introspection.
package authserver

import "net/http"

// Server — authorization server; реализует http.Handler.
type Server struct {
	// TODO: поля (клиенты, опции, mutex, коды, токены)
}

// New создаёт сервер с зарегистрированными клиентами.
func New(clients []Client, opts Options) *Server {
	// TODO: реализуйте
	return &Server{}
}

// ServeHTTP обслуживает три эндпоинта.
//
// GET /authorize?response_type&client_id&redirect_uri&scope&state&code_challenge&code_challenge_method
//  1. неизвестный client_id → 400, без редиректа;
//  2. redirect_uri должен точно совпасть с зарегистрированным (если не передан и
//     зарегистрирован ровно один — берётся он); иначе → 400 без редиректа;
//  3. дальнейшие ошибки — редирект 302 на redirect_uri с error=... (+state):
//     response_type != code → unsupported_response_type;
//     нет PKCE, метод не S256 или challenge не 43 символа → invalid_request;
//     scope вне Client.Scopes → invalid_scope (пустой scope = все разрешённые);
//     нет заголовка UserHeader → access_denied;
//  4. успех: 302 на redirect_uri?code=<случайный>&state=<state>
//     (существующий query в redirect_uri сохраняется).
//
// POST /token (form)
//   - аутентификация клиента: HTTP Basic (id и secret — url.QueryUnescape) для
//     конфиденциальных; client_id в форме — только для публичных; иначе 401
//     {"error":"invalid_client"} + WWW-Authenticate;
//   - grant_type=authorization_code: code существует, не просрочен (CodeTTL),
//     выдан этому клиенту, redirect_uri совпадает с переданным в /authorize
//     (не передавался там — не передаётся и здесь),
//     SHA256(code_verifier) == challenge (verifier 43..128 символов) — иначе 400 invalid_grant;
//     код одноразовый: повторное предъявление → invalid_grant и отзыв всех
//     токенов, выданных по этому коду;
//   - grant_type=client_credentials: только конфиденциальные клиенты
//     (публичный → unauthorized_client), scope как в /authorize, sub = client_id;
//   - иной grant_type → unsupported_grant_type;
//   - успех: 200 {"access_token","token_type":"Bearer","expires_in","scope"},
//     заголовок Cache-Control: no-store.
//
// POST /introspect (form: token) — только для клиента с Basic-аутентификацией
// (иначе 401): {"active":true,"sub","client_id","scope","exp","token_type"} или
// {"active":false} для неизвестного, просроченного или отозванного токена.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO: реализуйте
	http.Error(w, "TODO", http.StatusNotImplemented)
}
