//go:build solution

package shapes

import (
	"fmt"
	"math"
)

func (c Circle) Area() float64      { return math.Pi * c.R * c.R }
func (c Circle) Perimeter() float64 { return 2 * math.Pi * c.R }
func (c Circle) String() string     { return fmt.Sprintf("Circle(r=%g)", c.R) }

// Scale меняет получателя, поэтому получатель — указатель. Следствие:
// Circle (значение) не реализует Scaler, а *Circle — реализует.
func (c *Circle) Scale(k float64) { c.R *= k }

func (r Rect) Area() float64      { return r.W * r.H }
func (r Rect) Perimeter() float64 { return 2 * (r.W + r.H) }
func (r Rect) String() string     { return fmt.Sprintf("Rect(%gx%g)", r.W, r.H) }
func (r *Rect) Scale(k float64)   { r.W *= k; r.H *= k }

func (t Triangle) valid() bool {
	return t.A > 0 && t.B > 0 && t.C > 0 &&
		t.A+t.B > t.C && t.A+t.C > t.B && t.B+t.C > t.A
}

func (t Triangle) Area() float64 {
	if !t.valid() {
		return 0
	}
	p := (t.A + t.B + t.C) / 2
	return math.Sqrt(p * (p - t.A) * (p - t.B) * (p - t.C))
}

func (t Triangle) Perimeter() float64 { return t.A + t.B + t.C }
func (t Triangle) String() string     { return fmt.Sprintf("Triangle(%g, %g, %g)", t.A, t.B, t.C) }

func (s ByArea) Len() int           { return len(s) }
func (s ByArea) Less(i, j int) bool { return s[i].Area() < s[j].Area() }
func (s ByArea) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }

func TotalArea(shapes ...Shape) float64 {
	var sum float64
	for _, s := range shapes {
		if s == nil { // nil-интерфейс: вызов метода на нём — panic
			continue
		}
		sum += s.Area()
	}
	return sum
}

func Largest(shapes []Shape) (Shape, bool) {
	if len(shapes) == 0 {
		return nil, false
	}
	best := shapes[0]
	for _, s := range shapes[1:] {
		if s.Area() > best.Area() {
			best = s
		}
	}
	return best, true
}

func ScaleAll(shapes []Shape, k float64) int {
	n := 0
	for _, s := range shapes {
		// Circle{} внутри интерфейса — копия значения, её нельзя изменить,
		// и в её method set нет Scale. Поэтому assertion срабатывает только
		// для *Circle/*Rect.
		if sc, ok := s.(Scaler); ok {
			sc.Scale(k)
			n++
		}
	}
	return n
}

func Describe(s Shape) string {
	switch v := s.(type) {
	case nil:
		return "пусто"
	case Circle, *Circle:
		// В case с несколькими типами v имеет тип интерфейса (Shape).
		return "круг"
	case *Rect:
		if v == nil {
			return "пустой указатель"
		}
		return describeRect(*v)
	case Rect:
		return describeRect(v)
	default:
		return "неизвестная фигура"
	}
}

func describeRect(r Rect) string {
	if r.W == r.H {
		return "квадрат"
	}
	return "прямоугольник"
}
