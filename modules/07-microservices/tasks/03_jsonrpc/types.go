package jsonrpc

import "fmt"

// Стандартные коды ошибок JSON-RPC 2.0.
// Диапазон -32000..-32099 зарезервирован под ошибки сервера (приложения).
const (
	CodeParseError     = -32700 // невалидный JSON
	CodeInvalidRequest = -32600 // JSON валиден, но это не Request-объект
	CodeMethodNotFound = -32601 // метод не зарегистрирован
	CodeInvalidParams  = -32602 // параметры не подходят методу
	CodeInternalError  = -32603 // внутренняя ошибка (паника, неизвестная ошибка)
)

// Error — объект ошибки JSON-RPC. Реализует error.
// Обработчик может вернуть *Error, чтобы задать код и сообщение самостоятельно.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// BatchElem — один элемент пакетного вызова Client.Batch.
type BatchElem struct {
	Method string
	Params any  // nil — без params
	Result any  // указатель, куда распаковать result (может быть nil)
	Notify bool // true — нотификация (без id, ответа не ждём)
	Error  error
}
