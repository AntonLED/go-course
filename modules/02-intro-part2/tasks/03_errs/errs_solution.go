//go:build solution

package errs

import (
	"errors"
	"fmt"
	"strings"
)

func ValidateUser(u User) error {
	// Копим ошибки в срез интерфейсов error, а не *ValidationError: иначе
	// легко вернуть «типизированный nil» — интерфейс с (type=*ValidationError,
	// value=nil), который != nil.
	var errs []error
	if strings.TrimSpace(u.Name) == "" {
		errs = append(errs, &ValidationError{Field: "name", Msg: "обязательное поле"})
	}
	if u.Age < 0 || u.Age > 150 {
		errs = append(errs, &ValidationError{Field: "age", Msg: "вне диапазона 0..150"})
	}
	if !validEmail(u.Email) {
		errs = append(errs, &ValidationError{Field: "email", Msg: "некорректный адрес"})
	}
	return errors.Join(errs...) // для пустого среза — настоящий nil
}

func validEmail(s string) bool {
	local, domain, ok := strings.Cut(s, "@")
	return ok && local != "" && domain != "" && !strings.Contains(domain, "@")
}

func FieldErrors(err error) []*ValidationError {
	var out []*ValidationError
	var walk func(error)
	walk = func(e error) {
		if e == nil {
			return
		}
		if ve, ok := e.(*ValidationError); ok {
			out = append(out, ve)
		}
		switch x := e.(type) {
		case interface{ Unwrap() []error }: // errors.Join, fmt.Errorf с несколькими %w
			for _, c := range x.Unwrap() {
				walk(c)
			}
		case interface{ Unwrap() error }:
			walk(x.Unwrap())
		}
	}
	walk(err)
	return out
}

func FindUser(db map[int]User, id int) (User, error) {
	u, ok := db[id]
	if !ok {
		return User{}, fmt.Errorf("find user %d: %w", id, ErrNotFound)
	}
	return u, nil
}

func ProfileName(db map[int]User, id int) (string, error) {
	u, err := FindUser(db, id)
	if err != nil {
		// Добавляем контекст и возвращаем. Логировать здесь не нужно —
		// это решит тот, кто в итоге обработает ошибку.
		return "", fmt.Errorf("profile: %w", err)
	}
	return u.Name, nil
}

func (e *PanicError) Error() string { return fmt.Sprintf("panic: %v", e.Value) }

func (e *PanicError) Unwrap() error {
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}

func SafeCall(f func()) (err error) {
	defer func() {
		// recover работает только непосредственно в отложенной функции.
		// С Go 1.21 panic(nil) превращается в *runtime.PanicNilError,
		// так что r != nil для любой паники.
		if r := recover(); r != nil {
			err = &PanicError{Value: r}
		}
	}()
	f()
	return nil
}
