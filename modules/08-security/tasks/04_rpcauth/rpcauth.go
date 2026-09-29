//go:build !solution

// Package rpcauth — аутентификация и авторизация RPC-вызовов через
// интерсепторы: mTLS-идентичность сервиса, bearer-токены, scopes, per-RPC credentials.
package rpcauth

import "context"

// ChainUnary объединяет интерсепторы в один; interceptors[0] — самый внешний
// (как grpc.ChainUnaryInterceptor). Без интерсепторов — просто вызвать handler.
func ChainUnary(interceptors ...UnaryServerInterceptor) UnaryServerInterceptor {
	// TODO: реализуйте
	return func(ctx context.Context, req any, info *UnaryServerInfo, handler UnaryHandler) (any, error) {
		return handler(ctx, req)
	}
}

// Recovery превращает панику обработчика в ошибку с кодом Internal
// (текст паники клиенту не отдавать).
func Recovery() UnaryServerInterceptor {
	// TODO: реализуйте
	return func(ctx context.Context, req any, info *UnaryServerInfo, handler UnaryHandler) (any, error) {
		return handler(ctx, req)
	}
}

// PrincipalFromContext возвращает Principal, положенный BearerAuth.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	// TODO: реализуйте
	return Principal{}, false
}

// ServiceFromContext возвращает идентичность сервиса, положенную MTLSAuth.
func ServiceFromContext(ctx context.Context) (string, bool) {
	// TODO: реализуйте
	return "", false
}

// MTLSAuth — авторизация сервиса по клиентскому сертификату:
//   - нет Peer, нет TLS или нет проверенной цепочки (VerifiedChains) → Unauthenticated;
//   - идентичность: первый URI SAN со схемой "spiffe", иначе Subject.CommonName;
//   - не входит в allowed → PermissionDenied;
//   - иначе кладёт идентичность в контекст (ServiceFromContext).
func MTLSAuth(allowed []string) UnaryServerInterceptor {
	// TODO: реализуйте
	return ChainUnary()
}

// BearerAuth — аутентификация по метаданным "authorization: Bearer <token>":
//   - методы из skipMethods (например, health-check) пропускаются без проверки;
//   - должен быть ровно один заголовок authorization; схема Bearer без учёта
//     регистра; пустой токен или другая схема → Unauthenticated;
//   - ошибка validate → Unauthenticated с сообщением "invalid token" (детали не раскрывать);
//   - иначе кладёт Principal в контекст.
func BearerAuth(validate func(ctx context.Context, token string) (Principal, error), skipMethods ...string) UnaryServerInterceptor {
	// TODO: реализуйте
	return ChainUnary()
}

// RequireScopes — авторизация по scopes: methodScopes[info.FullMethod] — все
// необходимые scopes. Метода нет в карте → PermissionDenied (deny by default);
// нет Principal → Unauthenticated; не хватает scope → PermissionDenied.
func RequireScopes(methodScopes map[string][]string) UnaryServerInterceptor {
	// TODO: реализуйте
	return ChainUnary()
}

// StaticToken — PerRPCCredentials с метаданными {"authorization": "Bearer " + token},
// требующие защищённого транспорта.
func StaticToken(token string) PerRPCCredentials {
	// TODO: реализуйте
	return nil
}

// AttachCredentials добавляет метаданные creds к исходящему контексту
// (существующие исходящие метаданные сохраняются, исходная MD не мутируется).
// Если creds требуют защищённого транспорта, а secure == false → ErrInsecureTransport.
func AttachCredentials(ctx context.Context, creds PerRPCCredentials, secure bool, uri string) (context.Context, error) {
	// TODO: реализуйте
	return ctx, nil
}
