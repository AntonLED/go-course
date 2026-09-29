//go:build solution

// Package jsonrpc — сервер и клиент JSON-RPC 2.0 поверх HTTP.
package jsonrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
)

const version = "2.0"

// maxBody — ограничение на размер тела запроса.
const maxBody = 1 << 20

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  json.RawMessage `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	// ID: nil — поле отсутствует (нотификация), "null" — явный null.
	ID json.RawMessage `json:"id,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
	ID      json.RawMessage `json:"id"`
}

var nullID = json.RawMessage("null")

type handlerFunc func(ctx context.Context, params json.RawMessage) (any, error)

// errInvalidParams — сигнал из обёртки Register о неподходящих параметрах.
type errInvalidParams struct{ err error }

func (e errInvalidParams) Error() string { return e.err.Error() }

// Server — JSON-RPC 2.0 сервер, реализует http.Handler.
type Server struct {
	mu      sync.RWMutex
	methods map[string]handlerFunc
}

// NewServer создаёт пустой сервер.
func NewServer() *Server {
	return &Server{methods: make(map[string]handlerFunc)}
}

// Register регистрирует типизированный обработчик метода.
// Повторная регистрация того же имени — паника (ошибка программиста).
func Register[P, R any](s *Server, method string, fn func(ctx context.Context, params P) (R, error)) {
	h := func(ctx context.Context, raw json.RawMessage) (any, error) {
		var p P
		if len(raw) > 0 && !bytes.Equal(raw, nullID) {
			if err := json.Unmarshal(raw, &p); err != nil {
				return nil, errInvalidParams{err}
			}
		}
		return fn(ctx, p)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, dup := s.methods[method]; dup {
		panic("jsonrpc: method registered twice: " + method)
	}
	s.methods[method] = h
}

// ServeHTTP обрабатывает одиночные и пакетные вызовы.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		writeJSON(w, errorResponse(nullID, CodeParseError, "Parse error"))
		return
	}
	body = bytes.TrimSpace(body)

	if len(body) > 0 && body[0] == '[' {
		var elems []json.RawMessage
		if err := json.Unmarshal(body, &elems); err != nil {
			writeJSON(w, errorResponse(nullID, CodeParseError, "Parse error"))
			return
		}
		if len(elems) == 0 {
			writeJSON(w, errorResponse(nullID, CodeInvalidRequest, "Invalid Request"))
			return
		}
		// Элементы пакета независимы — выполняем параллельно, порядок сохраняем.
		resps := make([]*response, len(elems))
		var wg sync.WaitGroup
		for i, e := range elems {
			wg.Add(1)
			go func() {
				defer wg.Done()
				resps[i] = s.handleOne(r.Context(), e)
			}()
		}
		wg.Wait()
		out := make([]*response, 0, len(resps))
		for _, resp := range resps {
			if resp != nil {
				out = append(out, resp)
			}
		}
		if len(out) == 0 { // одни нотификации — отвечать нечем
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, out)
		return
	}

	if !json.Valid(body) {
		writeJSON(w, errorResponse(nullID, CodeParseError, "Parse error"))
		return
	}
	resp := s.handleOne(r.Context(), body)
	if resp == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, resp)
}

// handleOne обрабатывает один Request-объект. nil — ответ не нужен (нотификация).
func (s *Server) handleOne(ctx context.Context, raw json.RawMessage) *response {
	var req request
	if err := json.Unmarshal(raw, &req); err != nil {
		// Не объект (например, 1) или поле jsonrpc не строка. Если это всё же
		// объект с валидным id — отвечаем с этим id, иначе с null.
		id := nullID
		var probe struct {
			ID json.RawMessage `json:"id"`
		}
		if json.Unmarshal(raw, &probe) == nil && probe.ID != nil && validID(probe.ID) {
			id = probe.ID
		}
		return errorResponse(id, CodeInvalidRequest, "Invalid Request")
	}
	idOK := req.ID == nil || validID(req.ID)
	var method string
	if req.JSONRPC != version || !idOK || json.Unmarshal(req.Method, &method) != nil || method == "" ||
		!validParams(req.Params) {
		id := nullID
		if req.ID != nil && idOK {
			id = req.ID
		}
		return errorResponse(id, CodeInvalidRequest, "Invalid Request")
	}
	isNotification := req.ID == nil

	s.mu.RLock()
	h, ok := s.methods[method]
	s.mu.RUnlock()
	var resp *response
	if !ok {
		resp = errorResponse(req.ID, CodeMethodNotFound, "Method not found")
	} else {
		resp = call(ctx, h, req)
	}
	if isNotification {
		return nil // даже при ошибке нотификация не получает ответа
	}
	return resp
}

func call(ctx context.Context, h handlerFunc, req request) (resp *response) {
	defer func() {
		if p := recover(); p != nil {
			resp = errorResponse(req.ID, CodeInternalError, "Internal error")
		}
	}()
	res, err := h(ctx, req.Params)
	if err != nil {
		var rpcErr *Error
		var ip errInvalidParams
		switch {
		case errors.As(err, &ip):
			return &response{JSONRPC: version, ID: req.ID,
				Error: &Error{Code: CodeInvalidParams, Message: "Invalid params", Data: ip.Error()}}
		case errors.As(err, &rpcErr):
			return &response{JSONRPC: version, ID: req.ID, Error: rpcErr}
		default:
			// Не раскрываем внутренние детали клиенту.
			return errorResponse(req.ID, CodeInternalError, "Internal error")
		}
	}
	b, err := json.Marshal(res)
	if err != nil {
		return errorResponse(req.ID, CodeInternalError, "Internal error")
	}
	return &response{JSONRPC: version, Result: b, ID: req.ID}
}

// validID: id может быть строкой, числом или null.
func validID(id json.RawMessage) bool {
	switch c := id[0]; {
	case c == '"', c == '-', c >= '0' && c <= '9':
		return true
	default:
		return bytes.Equal(id, nullID)
	}
}

// validParams: params, если есть, — массив или объект.
func validParams(p json.RawMessage) bool {
	return len(p) == 0 || p[0] == '[' || p[0] == '{'
}

func errorResponse(id json.RawMessage, code int, msg string) *response {
	return &response{JSONRPC: version, Error: &Error{Code: code, Message: msg}, ID: id}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// ---------------------------------------------------------------- клиент

// Client — клиент JSON-RPC 2.0 поверх HTTP.
type Client struct {
	url    string
	hc     *http.Client
	nextID atomic.Uint64
}

// NewClient создаёт клиента; hc == nil — http.DefaultClient.
func NewClient(url string, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}
	return &Client{url: url, hc: hc}
}

func (c *Client) newRequest(method string, params any, notify bool) (request, error) {
	req := request{JSONRPC: version}
	req.Method, _ = json.Marshal(method)
	if params != nil {
		b, err := json.Marshal(params)
		if err != nil {
			return req, fmt.Errorf("jsonrpc: marshal params: %w", err)
		}
		req.Params = b
	}
	if !notify {
		req.ID = json.RawMessage(fmt.Sprint(c.nextID.Add(1)))
	}
	return req, nil
}

func (c *Client) post(ctx context.Context, payload any) (int, []byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(b))
	if err != nil {
		return 0, nil, err
	}
	hreq.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(hreq)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	return resp.StatusCode, body, err
}

// Call вызывает метод и распаковывает result в result (если не nil).
// Ошибка сервера возвращается как *Error.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	req, err := c.newRequest(method, params, false)
	if err != nil {
		return err
	}
	status, body, err := c.post(ctx, req)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("jsonrpc: unexpected HTTP status %d", status)
	}
	var resp response
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("jsonrpc: decode response: %w", err)
	}
	if resp.Error != nil {
		return resp.Error
	}
	if !bytes.Equal(resp.ID, req.ID) {
		return fmt.Errorf("jsonrpc: response id %s does not match request id %s", resp.ID, req.ID)
	}
	if result != nil {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("jsonrpc: decode result: %w", err)
		}
	}
	return nil
}

// Notify отправляет нотификацию (без id); сервер не присылает ответа.
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	req, err := c.newRequest(method, params, true)
	if err != nil {
		return err
	}
	status, _, err := c.post(ctx, req)
	if err != nil {
		return err
	}
	if status != http.StatusNoContent && status != http.StatusOK {
		return fmt.Errorf("jsonrpc: unexpected HTTP status %d", status)
	}
	return nil
}

// Batch отправляет несколько вызовов одним HTTP-запросом. Ошибки отдельных
// вызовов пишутся в BatchElem.Error; возвращаемая ошибка — транспортная.
func (c *Client) Batch(ctx context.Context, elems []*BatchElem) error {
	if len(elems) == 0 {
		return nil
	}
	reqs := make([]request, len(elems))
	byID := make(map[string]*BatchElem)
	for i, e := range elems {
		req, err := c.newRequest(e.Method, e.Params, e.Notify)
		if err != nil {
			return err
		}
		reqs[i] = req
		if !e.Notify {
			byID[string(req.ID)] = e
		}
	}
	status, body, err := c.post(ctx, reqs)
	if err != nil {
		return err
	}
	if len(byID) == 0 {
		if status != http.StatusNoContent && status != http.StatusOK {
			return fmt.Errorf("jsonrpc: unexpected HTTP status %d", status)
		}
		return nil
	}
	if status != http.StatusOK {
		return fmt.Errorf("jsonrpc: unexpected HTTP status %d", status)
	}
	var resps []response
	if err := json.Unmarshal(body, &resps); err != nil {
		// Сервер мог ответить одиночной ошибкой на весь пакет.
		var single response
		if json.Unmarshal(body, &single) == nil && single.Error != nil {
			return single.Error
		}
		return fmt.Errorf("jsonrpc: decode batch response: %w", err)
	}
	for _, resp := range resps {
		e, ok := byID[string(resp.ID)]
		if !ok {
			continue
		}
		delete(byID, string(resp.ID))
		switch {
		case resp.Error != nil:
			e.Error = resp.Error
		case e.Result != nil:
			if err := json.Unmarshal(resp.Result, e.Result); err != nil {
				e.Error = fmt.Errorf("jsonrpc: decode result: %w", err)
			}
		}
	}
	for _, e := range byID {
		e.Error = errors.New("jsonrpc: no response for request")
	}
	return nil
}
