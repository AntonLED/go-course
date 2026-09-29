//go:build solution

// Package rpcauth — аутентификация и авторизация RPC-вызовов через
// интерсепторы: mTLS-идентичность сервиса, bearer-токены, scopes, per-RPC credentials.
package rpcauth

import (
	"context"
	"slices"
	"strings"
)

// ChainUnary объединяет интерсепторы; первый — самый внешний.
func ChainUnary(interceptors ...UnaryServerInterceptor) UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *UnaryServerInfo, handler UnaryHandler) (any, error) {
		// Строим цепочку с конца: h_n = handler, h_i = interceptor_i(h_{i+1}).
		h := handler
		for i := len(interceptors) - 1; i >= 0; i-- {
			ic, next := interceptors[i], h
			h = func(ctx context.Context, req any) (any, error) {
				return ic(ctx, req, info, next)
			}
		}
		return h(ctx, req)
	}
}

// Recovery превращает панику в Internal, не раскрывая подробностей клиенту.
func Recovery() UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *UnaryServerInfo, handler UnaryHandler) (resp any, err error) {
		defer func() {
			if p := recover(); p != nil {
				resp, err = nil, Errorf(Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}

type (
	principalKey struct{}
	serviceKey   struct{}
)

// PrincipalFromContext возвращает Principal, положенный BearerAuth.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

// ServiceFromContext возвращает идентичность сервиса, положенную MTLSAuth.
func ServiceFromContext(ctx context.Context) (string, bool) {
	s, ok := ctx.Value(serviceKey{}).(string)
	return s, ok
}

// MTLSAuth пропускает только сервисы из allowed, определяя идентичность по
// проверенному клиентскому сертификату: SPIFFE ID (URI SAN) или CN.
func MTLSAuth(allowed []string) UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *UnaryServerInfo, handler UnaryHandler) (any, error) {
		p, ok := PeerFromContext(ctx)
		if !ok || p.TLS == nil || len(p.TLS.VerifiedChains) == 0 || len(p.TLS.VerifiedChains[0]) == 0 {
			return nil, Errorf(Unauthenticated, "verified client certificate required")
		}
		leaf := p.TLS.VerifiedChains[0][0]
		id := leaf.Subject.CommonName
		for _, u := range leaf.URIs {
			if u.Scheme == "spiffe" {
				id = u.String()
				break
			}
		}
		if !slices.Contains(allowed, id) {
			return nil, Errorf(PermissionDenied, "service %q is not allowed", id)
		}
		return handler(context.WithValue(ctx, serviceKey{}, id), req)
	}
}

// BearerAuth проверяет заголовок authorization: Bearer <token>.
func BearerAuth(validate func(ctx context.Context, token string) (Principal, error), skipMethods ...string) UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *UnaryServerInfo, handler UnaryHandler) (any, error) {
		if slices.Contains(skipMethods, info.FullMethod) {
			return handler(ctx, req)
		}
		md, _ := FromIncomingContext(ctx)
		vals := md.Get("authorization")
		if len(vals) != 1 {
			return nil, Errorf(Unauthenticated, "exactly one authorization header required")
		}
		scheme, token, found := strings.Cut(vals[0], " ")
		if !found || !strings.EqualFold(scheme, "Bearer") || token == "" {
			return nil, Errorf(Unauthenticated, "bearer token required")
		}
		principal, err := validate(ctx, token)
		if err != nil {
			// Причину пишем в лог, а клиенту — общий ответ.
			return nil, Errorf(Unauthenticated, "invalid token")
		}
		return handler(context.WithValue(ctx, principalKey{}, principal), req)
	}
}

// RequireScopes проверяет, что у Principal есть все scopes, нужные методу.
// Метод, которого нет в карте, запрещён (deny by default).
func RequireScopes(methodScopes map[string][]string) UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *UnaryServerInfo, handler UnaryHandler) (any, error) {
		need, ok := methodScopes[info.FullMethod]
		if !ok {
			return nil, Errorf(PermissionDenied, "method %s is not allowed", info.FullMethod)
		}
		p, ok := PrincipalFromContext(ctx)
		if !ok {
			return nil, Errorf(Unauthenticated, "authentication required")
		}
		for _, s := range need {
			if !slices.Contains(p.Scopes, s) {
				return nil, Errorf(PermissionDenied, "missing scope %q", s)
			}
		}
		return handler(ctx, req)
	}
}

type staticToken string

func (t staticToken) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + string(t)}, nil
}

func (staticToken) RequireTransportSecurity() bool { return true }

// StaticToken — per-RPC credentials с фиксированным bearer-токеном
// (аналог oauth.TokenSource / credentials.PerRPCCredentials).
func StaticToken(token string) PerRPCCredentials { return staticToken(token) }

// AttachCredentials добавляет метаданные creds в исходящий контекст —
// так поступает клиент grpc-go перед каждым вызовом.
func AttachCredentials(ctx context.Context, creds PerRPCCredentials, secure bool, uri string) (context.Context, error) {
	if creds.RequireTransportSecurity() && !secure {
		return nil, ErrInsecureTransport
	}
	extra, err := creds.GetRequestMetadata(ctx, uri)
	if err != nil {
		return nil, Errorf(Unauthenticated, "get request metadata: %v", err)
	}
	old, _ := FromOutgoingContext(ctx)
	md := make(MD, len(old)+len(extra))
	for k, v := range old {
		md[k] = slices.Clone(v) // не мутируем чужую карту
	}
	for k, v := range extra {
		k = strings.ToLower(k)
		md[k] = append(md[k], v)
	}
	return NewOutgoingContext(ctx, md), nil
}
