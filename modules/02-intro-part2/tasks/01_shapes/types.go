package shapes

// Shape — геометрическая фигура. Интерфейс маленький: только то, что
// действительно нужно потребителям.
type Shape interface {
	Area() float64
	Perimeter() float64
}

// Scaler — фигуры, которые можно масштабировать «на месте».
// Реализуется только указателями (*Circle, *Rect), потому что метод меняет
// получателя.
type Scaler interface {
	Scale(k float64)
}

// Circle — круг радиуса R.
type Circle struct {
	R float64
}

// Rect — прямоугольник W x H.
type Rect struct {
	W, H float64
}

// Triangle — треугольник со сторонами A, B, C.
type Triangle struct {
	A, B, C float64
}

// ByArea — срез фигур, упорядочиваемый по площади (реализует sort.Interface).
type ByArea []Shape
