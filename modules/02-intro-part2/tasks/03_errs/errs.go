//go:build !solution

package errs

// ValidateUser проверяет пользователя и возвращает все нарушения сразу,
// объединённые через errors.Join, либо nil.
//
// Правила (проверять в этом порядке):
//   - Name: после strings.TrimSpace не пустое       -> &ValidationError{"name", "обязательное поле"}
//   - Age: 0 <= Age <= 150                          -> &ValidationError{"age", "вне диапазона 0..150"}
//   - Email: ровно один '@', непустые части до/после -> &ValidationError{"email", "некорректный адрес"}
//
// внимание: в этой заготовке уже есть баг — найди его (тест «валидный
// пользователь» падает, хотя «ошибок нет»).
func ValidateUser(u User) error {
	var verr *ValidationError
	// TODO: проверки
	return verr
}

// FieldErrors обходит дерево ошибок (Unwrap() error и Unwrap() []error) в
// глубину, в прямом порядке (сначала сам узел, потом дети слева направо), и
// собирает все *ValidationError. Для nil — nil.
func FieldErrors(err error) []*ValidationError {
	return nil // TODO
}

// FindUser ищет пользователя по id. Если нет — ошибка, оборачивающая
// ErrNotFound с сообщением "find user 42: not found".
func FindUser(db map[int]User, id int) (User, error) {
	panic("TODO")
}

// ProfileName находит пользователя и возвращает его имя. Ошибку FindUser
// оборачивает: "profile: find user 42: not found". Не логирует её — только
// возвращает с контекстом.
func ProfileName(db map[int]User, id int) (string, error) {
	panic("TODO")
}

// Error для PanicError: "panic: <Value>" (через %v).
func (e *PanicError) Error() string { panic("TODO") }

// Unwrap возвращает Value, если это error, иначе nil. Благодаря этому
// errors.Is/As «видят» ошибку, с которой была вызвана panic.
func (e *PanicError) Unwrap() error { panic("TODO") }

// SafeCall вызывает f и превращает панику в *PanicError. Если паники не
// было — возвращает nil. Подсказка: defer + recover + именованный результат.
func SafeCall(f func()) (err error) {
	f()
	return nil // TODO
}
