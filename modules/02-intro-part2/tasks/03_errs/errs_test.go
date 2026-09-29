package errs

import (
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"runtime"
	"testing"
)

func TestValidateUserValid(t *testing.T) {
	for _, u := range []User{
		{Name: "Гофер", Age: 13, Email: "gopher@go.dev"},
		{Name: "A", Age: 0, Email: "a@b"},   // границы диапазона включаются
		{Name: "B", Age: 150, Email: "a@b"}, // границы диапазона включаются
	} {
		err := ValidateUser(u)
		if err != nil {
			t.Fatalf("ValidateUser(%+v) = %#v (%T), ожидалось nil. "+
				"Подсказка: интерфейс с nil-указателем внутри не равен nil", u, err, err)
		}
	}
}

func TestValidateUserInvalid(t *testing.T) {
	tests := []struct {
		name   string
		u      User
		fields []string
	}{
		{"пустое имя", User{Name: "   ", Age: 20, Email: "a@b"}, []string{"name"}},
		{"возраст", User{Name: "A", Age: 151, Email: "a@b"}, []string{"age"}},
		{"отрицательный возраст", User{Name: "A", Age: -1, Email: "a@b"}, []string{"age"}},
		{"email без @", User{Name: "A", Age: 1, Email: "ab"}, []string{"email"}},
		{"email два @", User{Name: "A", Age: 1, Email: "a@b@c"}, []string{"email"}},
		{"email пустой домен", User{Name: "A", Age: 1, Email: "a@"}, []string{"email"}},
		{"email пустая локальная часть", User{Name: "A", Age: 1, Email: "@b"}, []string{"email"}},
		{"всё плохо", User{Age: 200}, []string{"name", "age", "email"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateUser(tt.u)
			if err == nil {
				t.Fatalf("ValidateUser(%+v) = nil, ожидались ошибки по полям %v", tt.u, tt.fields)
			}
			// errors.As находит первую *ValidationError в дереве.
			var ve *ValidationError
			if !errors.As(err, &ve) || ve.Field != tt.fields[0] {
				t.Errorf("errors.As: %v, ожидалось поле %q", ve, tt.fields[0])
			}
			var got []string
			for _, fe := range FieldErrors(err) {
				got = append(got, fe.Field)
			}
			if !reflect.DeepEqual(got, tt.fields) {
				t.Errorf("FieldErrors = %v, ожидалось %v (ошибка: %v)", got, tt.fields, err)
			}
		})
	}
}

func TestFieldErrorsTree(t *testing.T) {
	if FieldErrors(nil) != nil {
		t.Errorf("FieldErrors(nil) должно быть nil")
	}
	a := &ValidationError{"a", "x"}
	b := &ValidationError{"b", "y"}
	c := &ValidationError{"c", "z"}
	// Смешанное дерево: fmt.Errorf с одним %w, с двумя %w, errors.Join.
	err := fmt.Errorf("top: %w",
		errors.Join(
			fmt.Errorf("left: %w", a),
			fmt.Errorf("pair: %w and %w", b, errors.New("plain")),
		),
	)
	err = errors.Join(err, nil, c)
	got := FieldErrors(err)
	want := []*ValidationError{a, b, c}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("FieldErrors = %v, ожидалось %v", got, want)
	}
	if got := FieldErrors(errors.New("просто ошибка")); len(got) != 0 {
		t.Errorf("FieldErrors(без валидации) = %v, ожидалось пусто", got)
	}
}

func TestFindUserAndProfile(t *testing.T) {
	db := map[int]User{1: {Name: "Роб", Age: 60, Email: "rob@go.dev"}}
	u, err := FindUser(db, 1)
	if err != nil || u.Name != "Роб" {
		t.Fatalf("FindUser(1) = %v, %v", u, err)
	}
	_, err = FindUser(db, 42)
	if !errors.Is(err, ErrNotFound) || err.Error() != "find user 42: not found" {
		t.Errorf("FindUser(42) err = %q, ожидалось обёртку ErrNotFound \"find user 42: not found\"", err)
	}
	if err == ErrNotFound {
		t.Errorf("FindUser должен ОБОРАЧИВАТЬ ErrNotFound с контекстом, а не возвращать как есть")
	}

	name, err := ProfileName(db, 1)
	if err != nil || name != "Роб" {
		t.Errorf("ProfileName(1) = %q, %v", name, err)
	}
	_, err = ProfileName(db, 7)
	if !errors.Is(err, ErrNotFound) || err.Error() != "profile: find user 7: not found" {
		t.Errorf("ProfileName(7) err = %q, ожидалось \"profile: find user 7: not found\" и errors.Is(ErrNotFound)", err)
	}
	if errors.Is(err, fs.ErrNotExist) {
		t.Errorf("errors.Is не должен совпадать с чужим sentinel")
	}
}

func TestSafeCall(t *testing.T) {
	if err := SafeCall(func() {}); err != nil {
		t.Errorf("SafeCall(без паники) = %v, ожидалось nil", err)
	}

	err := SafeCall(func() { panic("всё пропало") })
	var pe *PanicError
	if !errors.As(err, &pe) || pe.Value != "всё пропало" || err.Error() != "panic: всё пропало" {
		t.Fatalf("SafeCall(panic string) = %v, ожидалось *PanicError{\"всё пропало\"}", err)
	}
	if pe.Unwrap() != nil {
		t.Errorf("Unwrap для не-error значения должен вернуть nil")
	}

	// Паника с ошибкой: errors.Is должен «увидеть» её через Unwrap.
	err = SafeCall(func() { panic(fmt.Errorf("wrapped: %w", ErrNotFound)) })
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("SafeCall(panic(err)): errors.Is(err, ErrNotFound) = false; err = %v", err)
	}

	// Runtime-паника (выход за границы) реализует runtime.Error.
	err = SafeCall(func() {
		var s []int
		_ = s[5]
	})
	var re runtime.Error
	if !errors.As(err, &re) {
		t.Errorf("SafeCall(index out of range): ожидалось, что errors.As найдёт runtime.Error, err = %v", err)
	}

	// panic(nil) с Go 1.21 тоже ловится.
	err = SafeCall(func() { panic(nil) })
	var pn *runtime.PanicNilError
	if !errors.As(err, &pn) {
		t.Errorf("SafeCall(panic(nil)) = %v, ожидалось *PanicError с *runtime.PanicNilError", err)
	}
}
