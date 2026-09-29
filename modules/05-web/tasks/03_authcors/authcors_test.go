package authcors

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

func TestBasicAuth(t *testing.T) {
	var gotUser string
	var called bool
	h := BasicAuth("admin area", map[string]string{"alice": "s3cret", "bob": "пароль"})(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			gotUser, _ = UserFrom(r.Context())
		}))
	tests := []struct {
		name       string
		user, pass string
		noAuth     bool
		code       int
	}{
		{"ok", "alice", "s3cret", false, 200},
		{"okUnicode", "bob", "пароль", false, 200},
		{"wrongPass", "alice", "s3cre", false, 401},
		{"prefixPass", "alice", "s3cret!", false, 401},
		{"unknownUser", "eve", "s3cret", false, 401},
		{"emptyPass", "alice", "", false, 401},
		{"noHeader", "", "", true, 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called, gotUser = false, ""
			req := httptest.NewRequest("GET", "/", nil)
			if !tt.noAuth {
				req.SetBasicAuth(tt.user, tt.pass)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tt.code {
				t.Fatalf("код %d, ожидалось %d", rec.Code, tt.code)
			}
			if tt.code == 200 {
				if !called || gotUser != tt.user {
					t.Errorf("обработчик вызван=%v, пользователь в контексте %q, ожидалось %q", called, gotUser, tt.user)
				}
				return
			}
			if called {
				t.Error("при 401 обработчик не должен вызываться")
			}
			want := `Basic realm="admin area", charset="UTF-8"`
			if got := rec.Header().Get("WWW-Authenticate"); got != want {
				t.Errorf("WWW-Authenticate = %q, ожидалось %q", got, want)
			}
		})
	}
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer abc")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 401 {
		t.Errorf("Bearer вместо Basic: код %d, ожидалось 401", rec.Code)
	}
	if _, ok := UserFrom(req.Context()); ok {
		t.Error("UserFrom на пустом контексте должен вернуть ok=false")
	}
}

type corsCase struct {
	name       string
	method     string
	origin     string
	reqMethod  string // Access-Control-Request-Method
	reqHeaders string // Access-Control-Request-Headers
	wantCode   int
	wantNext   bool
	wantACAO   string
	wantCreds  string
	wantMethod string
	wantHdrs   string
	wantMaxAge string
}

func runCORS(t *testing.T, cfg CORSConfig, cases []corsCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var nextCalled bool
			h := CORS(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextCalled = true
				w.WriteHeader(200)
			}))
			req := httptest.NewRequest(tt.method, "/api", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if tt.reqMethod != "" {
				req.Header.Set("Access-Control-Request-Method", tt.reqMethod)
			}
			if tt.reqHeaders != "" {
				req.Header.Set("Access-Control-Request-Headers", tt.reqHeaders)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			hd := rec.Header()
			if rec.Code != tt.wantCode {
				t.Errorf("код %d, ожидалось %d", rec.Code, tt.wantCode)
			}
			if nextCalled != tt.wantNext {
				t.Errorf("next вызван=%v, ожидалось %v", nextCalled, tt.wantNext)
			}
			check := func(name, want string) {
				if got := hd.Get(name); got != want {
					t.Errorf("%s = %q, ожидалось %q", name, got, want)
				}
			}
			check("Access-Control-Allow-Origin", tt.wantACAO)
			check("Access-Control-Allow-Credentials", tt.wantCreds)
			check("Access-Control-Allow-Methods", tt.wantMethod)
			check("Access-Control-Allow-Headers", tt.wantHdrs)
			check("Access-Control-Max-Age", tt.wantMaxAge)
			if !slices.Contains(hd.Values("Vary"), "Origin") {
				t.Errorf("нет Vary: Origin (Vary=%v)", hd.Values("Vary"))
			}
		})
	}
}

func TestCORSList(t *testing.T) {
	cfg := CORSConfig{
		AllowedOrigins:   []string{"https://app.example.com", "https://admin.example.com"},
		AllowedMethods:   []string{"GET", "POST", "DELETE"},
		AllowedHeaders:   []string{"Content-Type", "X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           10 * time.Minute,
	}
	const app = "https://app.example.com"
	runCORS(t, cfg, []corsCase{
		{name: "noOrigin", method: "GET", wantCode: 200, wantNext: true},
		{name: "simpleAllowed", method: "GET", origin: app, wantCode: 200, wantNext: true, wantACAO: app, wantCreds: "true"},
		{name: "simpleForeign", method: "GET", origin: "https://evil.com", wantCode: 200, wantNext: true},
		{name: "originCaseMatters", method: "GET", origin: "https://APP.example.com", wantCode: 200, wantNext: true},
		{name: "preflightOK", method: "OPTIONS", origin: app, reqMethod: "DELETE", reqHeaders: "content-type, x-request-id",
			wantCode: 204, wantACAO: app, wantCreds: "true", wantMethod: "GET, POST, DELETE",
			wantHdrs: "Content-Type, X-Request-ID", wantMaxAge: "600"},
		{name: "preflightNoHeaders", method: "OPTIONS", origin: app, reqMethod: "POST",
			wantCode: 204, wantACAO: app, wantCreds: "true", wantMethod: "GET, POST, DELETE",
			wantHdrs: "Content-Type, X-Request-ID", wantMaxAge: "600"},
		{name: "preflightBadMethod", method: "OPTIONS", origin: app, reqMethod: "PATCH", wantCode: 403},
		{name: "preflightBadHeader", method: "OPTIONS", origin: app, reqMethod: "GET", reqHeaders: "X-Secret", wantCode: 403},
		{name: "preflightForeign", method: "OPTIONS", origin: "https://evil.com", reqMethod: "GET", wantCode: 403},
		{name: "plainOptions", method: "OPTIONS", origin: app, wantCode: 200, wantNext: true, wantACAO: app, wantCreds: "true"},
	})
}

func TestCORSWildcard(t *testing.T) {
	cfg := CORSConfig{AllowedOrigins: []string{"*"}, AllowedMethods: []string{"get", "put"}}
	runCORS(t, cfg, []corsCase{
		{name: "simple", method: "GET", origin: "https://any.io", wantCode: 200, wantNext: true, wantACAO: "*"},
		{name: "preflight", method: "OPTIONS", origin: "https://any.io", reqMethod: "PUT",
			wantCode: 204, wantACAO: "*", wantMethod: "GET, PUT"},
	})
	// С credentials "*" запрещён — эхо Origin.
	cfg.AllowCredentials = true
	runCORS(t, cfg, []corsCase{
		{name: "credsEcho", method: "GET", origin: "https://any.io", wantCode: 200, wantNext: true, wantACAO: "https://any.io", wantCreds: "true"},
	})
}
