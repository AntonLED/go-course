//go:build !solution

// Package jsonrpc — сервер и клиент JSON-RPC 2.0 поверх HTTP.
package jsonrpc

import (
	"context"
	"net/http"
)

// Server — JSON-RPC 2.0 сервер, реализует http.Handler.
//
// Подсказка: храните map[string]func(ctx, json.RawMessage) (any, error) —
// generic-функция Register оборачивает типизированный обработчик в такую
// «сырую» форму (распаковка params в P, возврат R как any).
type Server struct {
	// TODO: поля
}

// NewServer создаёт пустой сервер.
func NewServer() *Server {
	// TODO: реализуйте
	return &Server{}
}

// Register регистрирует типизированный обработчик метода.
//   - params отсутствуют → в fn приходит нулевое P (явный "params": null —
//     не массив и не объект, на него ServeHTTP отвечает -32600 ещё до вызова);
//   - params не распаковываются в P → ошибка -32602 Invalid params;
//   - повторная регистрация того же имени — паника.
//
// Пример: Register(s, "sum", func(ctx context.Context, xs []int) (int, error) {...})
func Register[P, R any](s *Server, method string, fn func(ctx context.Context, params P) (R, error)) {
	// TODO: реализуйте
}

// ServeHTTP обрабатывает POST-запросы с одиночным Request-объектом или пакетом.
//
//   - не POST → 405;
//   - невалидный JSON → {"jsonrpc":"2.0","error":{"code":-32700,...},"id":null};
//   - не Request-объект (jsonrpc != "2.0", method не строка/пустой, id не
//     строка/число/null, params не массив/объект) → -32600 (id — null, если id невалиден);
//   - неизвестный метод → -32601;
//   - обработчик вернул *Error → он как есть; другая ошибка или паника →
//     -32603 "Internal error" (текст исходной ошибки клиенту НЕ отдавать!);
//   - нотификация (нет поля id; "id": null — это НЕ нотификация) → ответа нет;
//   - пакет: массив ответов (без нотификаций); пустой массив → одиночная -32600;
//     если отвечать нечего → 204 без тела. Одиночная нотификация → тоже 204.
//
// Ответы — HTTP 200 и Content-Type: application/json.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// TODO: реализуйте
	http.Error(w, "TODO", http.StatusNotImplemented)
}

// Client — клиент JSON-RPC 2.0 поверх HTTP.
type Client struct {
	// TODO: поля (url, *http.Client, счётчик id)
}

// NewClient создаёт клиента; hc == nil — http.DefaultClient.
func NewClient(url string, hc *http.Client) *Client {
	// TODO: реализуйте
	return &Client{}
}

// Call вызывает метод с уникальным числовым id и распаковывает result в
// result (если не nil). Ошибка из ответа сервера возвращается как *Error;
// не-200 HTTP-статус и несовпадение id — тоже ошибки.
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	// TODO: реализуйте
	panic("TODO")
}

// Notify отправляет нотификацию (без поля id). Ожидается 204 (или 200).
func (c *Client) Notify(ctx context.Context, method string, params any) error {
	// TODO: реализуйте
	panic("TODO")
}

// Batch отправляет все elems одним HTTP-запросом (JSON-массив).
// Ответы сопоставляются по id (порядок в ответе произвольный!).
// Ошибка отдельного вызова пишется в elem.Error (*Error), результат — в elem.Result.
// Возвращаемая ошибка — только транспортная / ошибка всего пакета.
func (c *Client) Batch(ctx context.Context, elems []*BatchElem) error {
	// TODO: реализуйте
	panic("TODO")
}
