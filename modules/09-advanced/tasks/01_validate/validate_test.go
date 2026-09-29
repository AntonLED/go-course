package validate

import (
	"errors"
	"reflect"
	"testing"
)

type Address struct {
	City string `validate:"required"`
	Zip  string `validate:"min=6,max=6"`
}

type Item struct {
	SKU string `validate:"required"`
	Qty int    `validate:"min=1,max=100"`
}

type User struct {
	Name     string   `validate:"required,min=3,max=10"`
	Email    string   `validate:"required,email"`
	Nick     string   `validate:"omitempty,min=3"`
	Role     string   `validate:"oneof=admin user guest"`
	Age      int      `validate:"min=18"`
	Tags     []string `validate:"max=3"`
	Address  Address
	Billing  *Address
	Items    []Item `validate:"min=1"`
	internal string `validate:"required"` // неэкспортируемое — игнорируется
	Skip     string `validate:"-"`
}

func validUser() User {
	return User{
		Name:    "Алиса",
		Email:   "alice@example.com",
		Role:    "admin",
		Age:     30,
		Tags:    []string{"a"},
		Address: Address{City: "Москва", Zip: "101000"},
		Items:   []Item{{SKU: "x", Qty: 1}},
	}
}

func fieldErrs(t *testing.T, err error) ValidationErrors {
	t.Helper()
	var ve ValidationErrors
	if !errors.As(err, &ve) {
		t.Fatalf("ожидалась ValidationErrors, получено %T: %v", err, err)
	}
	return ve
}

func TestValidOK(t *testing.T) {
	u := validUser()
	if err := Validate(u); err != nil {
		t.Fatalf("Validate(валидный) = %v, ожидалось nil", err)
	}
	if err := Validate(&u); err != nil {
		t.Fatalf("Validate(&валидный) = %v, ожидалось nil", err)
	}
}

func TestRules(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*User)
		want   []FieldError
	}{
		{"required пустая строка", func(u *User) { u.Name = "" }, []FieldError{{"Name", "required", ""}}},
		{"min в рунах, не в байтах", func(u *User) { u.Name = "Юл" }, []FieldError{{"Name", "min", "3"}}},
		{"кириллица 10 рун проходит max=10", func(u *User) { u.Name = "Александрa" }, nil},
		{"max", func(u *User) { u.Name = "Константинопольский" }, []FieldError{{"Name", "max", "10"}}},
		{"email без точки в домене", func(u *User) { u.Email = "a@localhost" }, []FieldError{{"Email", "email", ""}}},
		{"email две собаки", func(u *User) { u.Email = "a@@b.ru" }, []FieldError{{"Email", "email", ""}}},
		{"email с пробелом", func(u *User) { u.Email = "a b@c.ru" }, []FieldError{{"Email", "email", ""}}},
		{"email пустой: только required", func(u *User) { u.Email = "" }, []FieldError{{"Email", "required", ""}}},
		{"omitempty пропускает пустое", func(u *User) { u.Nick = "" }, nil},
		{"omitempty не пропускает непустое", func(u *User) { u.Nick = "ab" }, []FieldError{{"Nick", "min", "3"}}},
		{"oneof", func(u *User) { u.Role = "root" }, []FieldError{{"Role", "oneof", "admin user guest"}}},
		{"min для числа", func(u *User) { u.Age = 17 }, []FieldError{{"Age", "min", "18"}}},
		{"max для среза", func(u *User) { u.Tags = []string{"a", "b", "c", "d"} }, []FieldError{{"Tags", "max", "3"}}},
		{"вложенная структура", func(u *User) { u.Address.City = "" }, []FieldError{{"Address.City", "required", ""}}},
		{"nil-указатель без required не проверяется", func(u *User) { u.Billing = nil }, nil},
		{"указатель на структуру проверяется", func(u *User) { u.Billing = &Address{City: "X", Zip: "1"} }, []FieldError{{"Billing.Zip", "min", "6"}}},
		{"элементы среза структур", func(u *User) {
			u.Items = []Item{{SKU: "a", Qty: 1}, {SKU: "", Qty: 500}}
		}, []FieldError{{"Items[1].SKU", "required", ""}, {"Items[1].Qty", "max", "100"}}},
		{"min=1 для пустого среза", func(u *User) { u.Items = nil }, []FieldError{{"Items", "min", "1"}}},
		{"skip и неэкспортируемые", func(u *User) { u.Skip = ""; u.internal = "" }, nil},
		{"несколько ошибок в порядке полей", func(u *User) { u.Name = ""; u.Age = 1; u.Address.Zip = "" }, []FieldError{
			{"Name", "required", ""}, {"Age", "min", "18"}, {"Address.Zip", "min", "6"},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u := validUser()
			tc.mutate(&u)
			err := Validate(&u)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("Validate = %v, ожидалось nil", err)
				}
				return
			}
			got := fieldErrs(t, err)
			if !reflect.DeepEqual([]FieldError(got), tc.want) {
				t.Fatalf("Validate:\n получено  %#v\n ожидалось %#v", got, tc.want)
			}
		})
	}
}

func TestPointerFields(t *testing.T) {
	type P struct {
		N *int    `validate:"required,min=5"`
		S *string `validate:"max=2"`
	}
	five, three, long := 5, 3, "long"
	if err := Validate(P{N: &five}); err != nil {
		t.Errorf("Validate(N=5) = %v, ожидалось nil", err)
	}
	got := fieldErrs(t, Validate(P{N: nil, S: &long}))
	want := []FieldError{{"N", "required", ""}, {"S", "max", "2"}}
	if !reflect.DeepEqual([]FieldError(got), want) {
		t.Errorf("получено %#v, ожидалось %#v", got, want)
	}
	got = fieldErrs(t, Validate(&P{N: &three}))
	if len(got) != 1 || got[0].Rule != "min" {
		t.Errorf("получено %#v, ожидалось нарушение min у N", got)
	}
}

func TestNotStruct(t *testing.T) {
	var nilUser *User
	for _, v := range []any{nil, 42, "str", nilUser, []User{}} {
		if err := Validate(v); !errors.Is(err, ErrNotStruct) {
			t.Errorf("Validate(%#v) = %v, ожидалось ErrNotStruct", v, err)
		}
	}
}

func TestBadTag(t *testing.T) {
	cases := []any{
		struct {
			A string `validate:"unknown"`
		}{"x"},
		struct {
			A string `validate:"min=abc"`
		}{"x"},
		struct {
			A bool `validate:"min=1"`
		}{true},
		struct {
			A int `validate:"email"`
		}{1},
		struct {
			Inner struct {
				B string `validate:"wat=1"`
			}
		}{},
	}
	for i, c := range cases {
		err := Validate(c)
		if !errors.Is(err, ErrBadTag) {
			t.Errorf("случай %d: Validate = %v, ожидалась ошибка ErrBadTag", i, err)
		}
		var ve ValidationErrors
		if errors.As(err, &ve) {
			t.Errorf("случай %d: ошибка тега не должна быть ValidationErrors", i)
		}
	}
}

func TestErrorString(t *testing.T) {
	err := ValidationErrors{{"A", "required", ""}, {"B", "min", "3"}}
	want := "A: нарушено правило required; B: нарушено правило min=3"
	if err.Error() != want {
		t.Errorf("Error() = %q, ожидалось %q", err.Error(), want)
	}
}

func BenchmarkValidate(b *testing.B) {
	u := validUser()
	for i := 0; i < b.N; i++ {
		_ = Validate(&u)
	}
}
