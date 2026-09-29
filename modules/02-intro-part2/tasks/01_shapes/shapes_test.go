package shapes

import (
	"fmt"
	"math"
	"sort"
	"testing"
)

// Проверки реализации интерфейсов на этапе компиляции.
var (
	_ Shape          = Circle{}
	_ Shape          = Rect{}
	_ Shape          = Triangle{}
	_ Shape          = (*Circle)(nil)
	_ Scaler         = (*Circle)(nil)
	_ Scaler         = (*Rect)(nil)
	_ fmt.Stringer   = Circle{}
	_ fmt.Stringer   = Rect{}
	_ fmt.Stringer   = Triangle{}
	_ sort.Interface = ByArea(nil)
)

const eps = 1e-9

func near(a, b float64) bool { return math.Abs(a-b) < eps }

func TestAreaPerimeter(t *testing.T) {
	tests := []struct {
		s          Shape
		area, peri float64
	}{
		{Circle{R: 1}, math.Pi, 2 * math.Pi},
		{Circle{R: 0}, 0, 0},
		{Rect{W: 2, H: 3}, 6, 10},
		{Triangle{3, 4, 5}, 6, 12},
		{Triangle{1, 1, 5}, 0, 7}, // невозможный треугольник
		{Triangle{1, 2, 3}, 0, 6}, // вырожденный
		{Triangle{-3, 4, 5}, 0, 6},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%v", tt.s), func(t *testing.T) {
			if got := tt.s.Area(); !near(got, tt.area) {
				t.Errorf("%v.Area() = %v, ожидалось %v", tt.s, got, tt.area)
			}
			if got := tt.s.Perimeter(); !near(got, tt.peri) {
				t.Errorf("%v.Perimeter() = %v, ожидалось %v", tt.s, got, tt.peri)
			}
		})
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		s    fmt.Stringer
		want string
	}{
		{Circle{R: 1.5}, "Circle(r=1.5)"},
		{Rect{W: 2, H: 3}, "Rect(2x3)"},
		{Triangle{3, 4, 5}, "Triangle(3, 4, 5)"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("String() = %q, ожидалось %q", got, tt.want)
		}
		// fmt сам вызывает String() у Stringer.
		if got := fmt.Sprint(tt.s); got != tt.want {
			t.Errorf("fmt.Sprint = %q, ожидалось %q", got, tt.want)
		}
	}
	// У *Circle тоже есть String (method set указателя включает методы значения).
	if got := fmt.Sprint(&Circle{R: 2}); got != "Circle(r=2)" {
		t.Errorf("fmt.Sprint(&Circle{2}) = %q, ожидалось %q", got, "Circle(r=2)")
	}
}

func TestSortByArea(t *testing.T) {
	s := []Shape{Rect{W: 10, H: 10}, Circle{R: 1}, Triangle{3, 4, 5}, Rect{W: 1, H: 1}}
	sort.Sort(ByArea(s))
	want := []string{"Rect(1x1)", "Circle(r=1)", "Triangle(3, 4, 5)", "Rect(10x10)"}
	for i, sh := range s {
		if got := fmt.Sprint(sh); got != want[i] {
			t.Fatalf("после сортировки s[%d] = %s, ожидалось %s (весь срез: %v)", i, got, want[i], s)
		}
	}
}

func TestTotalArea(t *testing.T) {
	if got := TotalArea(); got != 0 {
		t.Errorf("TotalArea() = %v, ожидалось 0", got)
	}
	got := TotalArea(Rect{W: 2, H: 3}, nil, Triangle{3, 4, 5})
	if !near(got, 12) {
		t.Errorf("TotalArea(Rect 2x3, nil, Triangle 3-4-5) = %v, ожидалось 12", got)
	}
}

func TestLargest(t *testing.T) {
	if s, ok := Largest(nil); ok || s != nil {
		t.Errorf("Largest(nil) = (%v, %v), ожидалось (nil, false)", s, ok)
	}
	a, b := Rect{W: 2, H: 2}, Rect{W: 1, H: 4}
	s, ok := Largest([]Shape{Circle{R: 0.1}, a, b})
	if !ok || s != Shape(a) {
		t.Errorf("Largest = (%v, %v), ожидалось (%v, true) — при равенстве первая", s, ok, a)
	}
}

func TestScaleAll(t *testing.T) {
	c := &Circle{R: 1}
	r := &Rect{W: 1, H: 2}
	val := Circle{R: 5}
	shapes := []Shape{c, val, r, Triangle{3, 4, 5}}
	if n := ScaleAll(shapes, 2); n != 2 {
		t.Errorf("ScaleAll вернула %d, ожидалось 2 (Scaler реализуют только *Circle и *Rect)", n)
	}
	if c.R != 2 || r.W != 2 || r.H != 4 {
		t.Errorf("после ScaleAll: c=%v r=%v, ожидалось Circle(r=2), Rect(2x4)", c, r)
	}
	if shapes[1].(Circle).R != 5 {
		t.Errorf("значение Circle внутри интерфейса не должно меняться")
	}
}

func TestDescribe(t *testing.T) {
	var nilRect *Rect
	tests := []struct {
		name string
		s    Shape
		want string
	}{
		{"nil", nil, "пусто"},
		{"Circle", Circle{R: 1}, "круг"},
		{"*Circle", &Circle{R: 1}, "круг"},
		{"квадрат", Rect{W: 2, H: 2}, "квадрат"},
		{"*квадрат", &Rect{W: 3, H: 3}, "квадрат"},
		{"прямоугольник", Rect{W: 2, H: 3}, "прямоугольник"},
		{"typed nil", nilRect, "пустой указатель"},
		{"Triangle", Triangle{3, 4, 5}, "неизвестная фигура"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Describe(tt.s); got != tt.want {
				t.Errorf("Describe(%#v) = %q, ожидалось %q", tt.s, got, tt.want)
			}
		})
	}
}

func BenchmarkSortByArea(b *testing.B) {
	base := make([]Shape, 1000)
	for i := range base {
		base[i] = Rect{W: float64(i % 37), H: float64(i % 11)}
	}
	s := make([]Shape, len(base))
	for i := 0; i < b.N; i++ {
		copy(s, base)
		sort.Sort(ByArea(s))
	}
}
