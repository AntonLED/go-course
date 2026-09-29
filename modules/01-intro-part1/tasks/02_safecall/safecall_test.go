package safecall

import (
	"errors"
	"io"
	"runtime"
	"testing"
)

func TestSafeCallNoPanic(t *testing.T) {
	called := false
	if err := SafeCall(func() { called = true }); err != nil {
		t.Fatalf("SafeCall без паники вернул %v, ожидался nil", err)
	}
	if !called {
		t.Fatal("SafeCall не вызвал функцию")
	}
}

func TestSafeCallNil(t *testing.T) {
	if err := SafeCall(nil); !errors.Is(err, ErrNilFunc) {
		t.Fatalf("SafeCall(nil) = %v, ожидался ErrNilFunc", err)
	}
}

func TestSafeCallPanics(t *testing.T) {
	t.Run("строка", func(t *testing.T) {
		err := SafeCall(func() { panic("boom") })
		var pe *PanicError
		if !errors.As(err, &pe) {
			t.Fatalf("ожидался *PanicError, получено %T (%v)", err, err)
		}
		if pe.Value != "boom" {
			t.Errorf("PanicError.Value = %v, ожидалось \"boom\"", pe.Value)
		}
		if err.Error() != "panic: boom" {
			t.Errorf("Error() = %q, ожидалось %q", err.Error(), "panic: boom")
		}
	})
	t.Run("ошибка оборачивается", func(t *testing.T) {
		err := SafeCall(func() { panic(io.EOF) })
		if !errors.Is(err, io.EOF) {
			t.Fatalf("errors.Is(err, io.EOF) = false для %v", err)
		}
	})
	t.Run("runtime error", func(t *testing.T) {
		err := SafeCall(func() {
			var s []int
			i := 3
			_ = s[i]
		})
		var re runtime.Error
		if !errors.As(err, &re) {
			t.Fatalf("ожидалась runtime.Error внутри, получено %v", err)
		}
	})
	t.Run("panic(nil)", func(t *testing.T) {
		err := SafeCall(func() { panic(nil) })
		if err == nil {
			t.Fatal("panic(nil) должна давать ошибку (Go 1.21+: *runtime.PanicNilError)")
		}
		var pn *runtime.PanicNilError
		if !errors.As(err, &pn) {
			t.Errorf("ожидалась *runtime.PanicNilError внутри, получено %v", err)
		}
	})
	t.Run("nil map", func(t *testing.T) {
		err := SafeCall(func() {
			var m map[string]int
			m["x"] = 1
		})
		if err == nil {
			t.Fatal("запись в nil map должна паниковать")
		}
	})
}

func TestCounter(t *testing.T) {
	next, reset := Counter(10, 5)
	for i, want := range []int{10, 15, 20} {
		if got := next(); got != want {
			t.Fatalf("вызов %d: next() = %d, ожидалось %d", i, got, want)
		}
	}
	reset()
	if got := next(); got != 10 {
		t.Fatalf("после reset next() = %d, ожидалось 10", got)
	}
	next2, _ := Counter(0, -1)
	next2()
	if got := next2(); got != -1 {
		t.Fatalf("второй счётчик: next() = %d, ожидалось -1", got)
	}
	if got := next(); got != 15 {
		t.Fatalf("счётчики должны быть независимы: next() = %d, ожидалось 15", got)
	}
}

func TestCompose(t *testing.T) {
	inc := func(x int) int { return x + 1 }
	dbl := func(x int) int { return x * 2 }

	if got := Compose()(7); got != 7 {
		t.Errorf("Compose()(7) = %d, ожидалось 7", got)
	}
	if got := Compose(inc, dbl)(3); got != 8 {
		t.Errorf("Compose(inc, dbl)(3) = %d, ожидалось 8 (слева направо)", got)
	}
	if got := Compose(dbl, inc)(3); got != 7 {
		t.Errorf("Compose(dbl, inc)(3) = %d, ожидалось 7", got)
	}

	fs := []func(int) int{inc, inc}
	f := Compose(fs...)
	fs[0] = func(x int) int { return x * 10 } // не должно влиять на уже построенную композицию
	if got := f(1); got != 3 {
		t.Errorf("после изменения исходного среза f(1) = %d, ожидалось 3 (нужна копия fs)", got)
	}
}
