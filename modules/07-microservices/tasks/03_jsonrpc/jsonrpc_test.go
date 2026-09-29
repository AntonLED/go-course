package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

type divParams struct {
	A float64 `json:"a"`
	B float64 `json:"b"`
}

type testEnv struct {
	srv      *httptest.Server
	notified atomic.Int64
}

func newEnv(t *testing.T) *testEnv {
	t.Helper()
	env := &testEnv{}
	s := NewServer()
	Register(s, "subtract", func(_ context.Context, p []int) (int, error) {
		if len(p) != 2 {
			return 0, &Error{Code: CodeInvalidParams, Message: "need 2 params"}
		}
		return p[0] - p[1], nil
	})
	Register(s, "sum", func(_ context.Context, p []int) (int, error) {
		total := 0
		for _, x := range p {
			total += x
		}
		return total, nil
	})
	Register(s, "div", func(_ context.Context, p divParams) (float64, error) {
		if p.B == 0 {
			return 0, &Error{Code: -32000, Message: "division by zero"}
		}
		return p.A / p.B, nil
	})
	Register(s, "notify_hello", func(_ context.Context, p []int) (struct{}, error) {
		env.notified.Add(1)
		return struct{}{}, nil
	})
	Register(s, "get_data", func(context.Context, struct{}) ([]any, error) {
		return []any{"hello", 5}, nil
	})
	Register(s, "leak", func(context.Context, struct{}) (int, error) {
		return 0, errors.New("pq: password authentication failed for user admin")
	})
	Register(s, "boom", func(context.Context, struct{}) (int, error) {
		panic("nil map")
	})
	Register(s, "nothing", func(context.Context, struct{}) (*int, error) {
		return nil, nil
	})
	Register(s, "whoami", func(ctx context.Context, _ struct{}) (string, error) {
		v, _ := ctx.Value(ctxKey{}).(string)
		return v, nil
	})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, "alice")))
	})
	env.srv = httptest.NewServer(h)
	t.Cleanup(env.srv.Close)
	return env
}

type ctxKey struct{}

func (e *testEnv) post(t *testing.T, body string) (int, string) {
	t.Helper()
	resp, err := http.Post(e.srv.URL, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK && !strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		t.Errorf("Content-Type ответа %q, ожидался application/json", resp.Header.Get("Content-Type"))
	}
	return resp.StatusCode, string(b)
}

// normalize приводит JSON к сравнимому виду; для массивов порядок не важен.
func normalize(t *testing.T, s string) any {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		t.Fatalf("невалидный JSON в ответе %q: %v", s, err)
	}
	if arr, ok := v.([]any); ok {
		keys := make([]string, len(arr))
		for i, x := range arr {
			b, _ := json.Marshal(x)
			keys[i] = string(b)
		}
		sort.Strings(keys)
		return keys
	}
	return v
}

// Примеры из раздела 7 спецификации JSON-RPC 2.0 (+ свои).
func TestServerSpecExamples(t *testing.T) {
	env := newEnv(t)
	cases := []struct {
		name, req, want string
	}{
		{"positional", `{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":1}`,
			`{"jsonrpc":"2.0","result":19,"id":1}`},
		{"named", `{"jsonrpc":"2.0","method":"div","params":{"a":1,"b":4},"id":"abc"}`,
			`{"jsonrpc":"2.0","result":0.25,"id":"abc"}`},
		{"app error", `{"jsonrpc":"2.0","method":"div","params":{"a":1,"b":0},"id":2}`,
			`{"jsonrpc":"2.0","error":{"code":-32000,"message":"division by zero"},"id":2}`},
		{"null id is a request", `{"jsonrpc":"2.0","method":"sum","params":[1,2],"id":null}`,
			`{"jsonrpc":"2.0","result":3,"id":null}`},
		{"nil result", `{"jsonrpc":"2.0","method":"nothing","id":7}`,
			`{"jsonrpc":"2.0","result":null,"id":7}`},
		{"method not found", `{"jsonrpc":"2.0","method":"foobar","id":"1"}`,
			`{"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":"1"}`},
		{"parse error", `{"jsonrpc":"2.0","method":"foobar,"params":"bar","baz]`,
			`{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"},"id":null}`},
		{"invalid request", `{"jsonrpc":"2.0","method":1,"params":"bar"}`,
			`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":null}`},
		{"wrong version", `{"jsonrpc":"1.0","method":"sum","params":[1],"id":5}`,
			`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":5}`},
		{"non-string version", `{"jsonrpc":2.0,"method":"sum","params":[1],"id":8}`,
			`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":8}`},
		{"object id", `{"jsonrpc":"2.0","method":"sum","params":[1],"id":{"x":1}}`,
			`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":null}`},
		{"primitive params", `{"jsonrpc":"2.0","method":"sum","params":5,"id":6}`,
			`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":6}`},
		{"empty batch", `[]`,
			`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":null}`},
		{"invalid batch json", `[{"jsonrpc":"2.0","method":"sum","params":[1,2,4],"id":"1"},{"jsonrpc":"2.0","method"]`,
			`{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error"},"id":null}`},
		{"invalid batch elems", `[1,2]`,
			`[{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":null},
			  {"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":null}]`},
		{"batch", `[
			{"jsonrpc":"2.0","method":"sum","params":[1,2,4],"id":"1"},
			{"jsonrpc":"2.0","method":"notify_hello","params":[7]},
			{"jsonrpc":"2.0","method":"subtract","params":[42,23],"id":"2"},
			{"foo":"boo"},
			{"jsonrpc":"2.0","method":"foo.get","params":{"name":"myself"},"id":"5"},
			{"jsonrpc":"2.0","method":"get_data","id":"9"}]`,
			`[{"jsonrpc":"2.0","result":7,"id":"1"},
			  {"jsonrpc":"2.0","result":19,"id":"2"},
			  {"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid Request"},"id":null},
			  {"jsonrpc":"2.0","error":{"code":-32601,"message":"Method not found"},"id":"5"},
			  {"jsonrpc":"2.0","result":["hello",5],"id":"9"}]`},
		{"context passed", `{"jsonrpc":"2.0","method":"whoami","id":1}`,
			`{"jsonrpc":"2.0","result":"alice","id":1}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			status, body := env.post(t, c.req)
			if status != http.StatusOK {
				t.Fatalf("HTTP %d, ожидалось 200; тело %q", status, body)
			}
			if got, want := normalize(t, body), normalize(t, c.want); !reflect.DeepEqual(got, want) {
				t.Errorf("запрос %s\nответ    %s\nожидалось %s", c.req, body, c.want)
			}
		})
	}
}

func TestServerInvalidParams(t *testing.T) {
	env := newEnv(t)
	_, body := env.post(t, `{"jsonrpc":"2.0","method":"div","params":[1,2],"id":1}`)
	var resp struct {
		Error *Error          `json:"error"`
		ID    json.RawMessage `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil || resp.Error == nil {
		t.Fatalf("ожидалась ошибка, ответ %q", body)
	}
	if resp.Error.Code != CodeInvalidParams || string(resp.ID) != "1" {
		t.Errorf("массив вместо объекта: код %d id %s, ожидалось -32602 и id 1", resp.Error.Code, resp.ID)
	}
}

func TestServerHidesInternalErrors(t *testing.T) {
	env := newEnv(t)
	for _, m := range []string{"leak", "boom"} {
		_, body := env.post(t, `{"jsonrpc":"2.0","method":"`+m+`","id":1}`)
		if strings.Contains(body, "password") || strings.Contains(body, "nil map") {
			t.Errorf("%s: внутренние детали утекли клиенту: %s", m, body)
		}
		want := `{"jsonrpc":"2.0","error":{"code":-32603,"message":"Internal error"},"id":1}`
		if !reflect.DeepEqual(normalize(t, body), normalize(t, want)) {
			t.Errorf("%s: ответ %s, ожидалось %s", m, body, want)
		}
	}
	// после паники сервер жив
	if _, body := env.post(t, `{"jsonrpc":"2.0","method":"sum","params":[2,2],"id":1}`); !strings.Contains(body, `"result":4`) {
		t.Errorf("после паники сервер ответил %s", body)
	}
}

func TestServerNotifications(t *testing.T) {
	env := newEnv(t)
	status, body := env.post(t, `{"jsonrpc":"2.0","method":"notify_hello","params":[1]}`)
	if status != http.StatusNoContent || body != "" {
		t.Errorf("одиночная нотификация: HTTP %d тело %q, ожидалось 204 без тела", status, body)
	}
	status, body = env.post(t, `[{"jsonrpc":"2.0","method":"notify_hello","params":[1]},{"jsonrpc":"2.0","method":"nope"}]`)
	if status != http.StatusNoContent || body != "" {
		t.Errorf("пакет из нотификаций: HTTP %d тело %q, ожидалось 204 без тела", status, body)
	}
	if n := env.notified.Load(); n != 2 {
		t.Errorf("нотификаций выполнено %d, ожидалось 2", n)
	}
}

func TestServerMethodNotAllowed(t *testing.T) {
	env := newEnv(t)
	resp, err := http.Get(env.srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET: HTTP %d, ожидалось 405", resp.StatusCode)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	s := NewServer()
	f := func(context.Context, int) (int, error) { return 0, nil }
	Register(s, "x", f)
	defer func() {
		if recover() == nil {
			t.Error("повторная регистрация метода должна паниковать")
		}
	}()
	Register(s, "x", f)
}

func TestClientCall(t *testing.T) {
	env := newEnv(t)
	c := NewClient(env.srv.URL, env.srv.Client())
	ctx := context.Background()

	var n int
	if err := c.Call(ctx, "subtract", []int{42, 23}, &n); err != nil || n != 19 {
		t.Errorf("Call(subtract) = %d, %v; ожидалось 19, nil", n, err)
	}
	var f float64
	if err := c.Call(ctx, "div", divParams{A: 3, B: 2}, &f); err != nil || f != 1.5 {
		t.Errorf("Call(div) = %v, %v; ожидалось 1.5, nil", f, err)
	}
	err := c.Call(ctx, "div", divParams{A: 1}, &f)
	var rpcErr *Error
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32000 || rpcErr.Message != "division by zero" {
		t.Errorf("Call(div by zero) err = %v, ожидался *Error{-32000}", err)
	}
	err = c.Call(ctx, "missing", nil, nil)
	if !errors.As(err, &rpcErr) || rpcErr.Code != CodeMethodNotFound {
		t.Errorf("Call(missing) err = %v, ожидался *Error{-32601}", err)
	}
	if err := c.Call(ctx, "sum", []int{1}, nil); err != nil {
		t.Errorf("Call с result=nil: %v", err)
	}
	if err := c.Notify(ctx, "notify_hello", []int{1}); err != nil {
		t.Errorf("Notify: %v", err)
	}
	if env.notified.Load() != 1 {
		t.Errorf("Notify не дошла до сервера")
	}
}

func TestClientConcurrentCalls(t *testing.T) {
	env := newEnv(t)
	c := NewClient(env.srv.URL, env.srv.Client())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var n int
			if err := c.Call(context.Background(), "sum", []int{i, i}, &n); err != nil || n != 2*i {
				t.Errorf("sum(%d,%d) = %d, %v", i, i, n, err)
			}
		}(i)
	}
	wg.Wait()
}

func TestClientBatch(t *testing.T) {
	env := newEnv(t)
	c := NewClient(env.srv.URL, env.srv.Client())
	var a, b int
	var d float64
	elems := []*BatchElem{
		{Method: "sum", Params: []int{1, 2, 3}, Result: &a},
		{Method: "notify_hello", Params: []int{0}, Notify: true},
		{Method: "subtract", Params: []int{10, 4}, Result: &b},
		{Method: "div", Params: divParams{A: 1, B: 0}, Result: &d},
		{Method: "nope"},
	}
	if err := c.Batch(context.Background(), elems); err != nil {
		t.Fatalf("Batch: %v", err)
	}
	if a != 6 || elems[0].Error != nil {
		t.Errorf("batch sum = %d, %v", a, elems[0].Error)
	}
	if b != 6 || elems[2].Error != nil {
		t.Errorf("batch subtract = %d, %v", b, elems[2].Error)
	}
	var rpcErr *Error
	if !errors.As(elems[3].Error, &rpcErr) || rpcErr.Code != -32000 {
		t.Errorf("batch div: Error = %v, ожидался *Error{-32000}", elems[3].Error)
	}
	if !errors.As(elems[4].Error, &rpcErr) || rpcErr.Code != CodeMethodNotFound {
		t.Errorf("batch nope: Error = %v, ожидался *Error{-32601}", elems[4].Error)
	}
	if elems[1].Error != nil {
		t.Errorf("нотификация в пакете получила ошибку %v", elems[1].Error)
	}
	// пакет только из нотификаций
	if err := c.Batch(context.Background(), []*BatchElem{{Method: "notify_hello", Notify: true}}); err != nil {
		t.Errorf("Batch из нотификаций: %v", err)
	}
	if env.notified.Load() != 2 {
		t.Errorf("нотификаций дошло %d, ожидалось 2", env.notified.Load())
	}
}

func TestClientHTTPErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}))
	defer srv.Close()
	c := NewClient(srv.URL, nil)
	if err := c.Call(context.Background(), "x", nil, nil); err == nil {
		t.Error("HTTP 502 должен давать ошибку")
	}
	// сервер, отвечающий чужим id
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"jsonrpc":"2.0","result":1,"id":999999}`)
	}))
	defer srv2.Close()
	if err := NewClient(srv2.URL, nil).Call(context.Background(), "x", nil, nil); err == nil {
		t.Error("ответ с чужим id должен давать ошибку")
	}
}
