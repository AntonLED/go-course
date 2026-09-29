//go:build !solution

package shapes

// Area возвращает площадь круга (math.Pi * R * R).
func (c Circle) Area() float64 { panic("TODO") }

// Perimeter возвращает длину окружности.
func (c Circle) Perimeter() float64 { panic("TODO") }

// String реализует fmt.Stringer: "Circle(r=1.5)" (числа — через %g).
func (c Circle) String() string { panic("TODO") }

// Scale умножает радиус на k. Получатель — указатель!
func (c *Circle) Scale(k float64) { panic("TODO") }

func (r Rect) Area() float64      { panic("TODO") }
func (r Rect) Perimeter() float64 { panic("TODO") }

// String: "Rect(2x3)".
func (r Rect) String() string { panic("TODO") }

// Scale умножает обе стороны на k.
func (r *Rect) Scale(k float64) { panic("TODO") }

// Area — по формуле Герона. Для вырожденного/невозможного треугольника
// (нарушено неравенство треугольника или сторона <= 0) возвращает 0.
func (t Triangle) Area() float64 { panic("TODO") }

// Perimeter — сумма сторон A+B+C (всегда, даже для невалидного треугольника).
func (t Triangle) Perimeter() float64 { panic("TODO") }

// String: "Triangle(3, 4, 5)".
func (t Triangle) String() string { panic("TODO") }

// Методы sort.Interface для ByArea: сортировка по возрастанию площади.
func (s ByArea) Len() int           { panic("TODO") }
func (s ByArea) Less(i, j int) bool { panic("TODO") }
func (s ByArea) Swap(i, j int)      { panic("TODO") }

// TotalArea — сумма площадей. Элементы-nil (nil-интерфейс) пропускаются.
func TotalArea(shapes ...Shape) float64 { panic("TODO") }

// Largest возвращает фигуру с максимальной площадью; ok=false для пустого среза.
// При равенстве площадей — первая встреченная.
func Largest(shapes []Shape) (s Shape, ok bool) { panic("TODO") }

// ScaleAll масштабирует все фигуры, которые реализуют Scaler, и
// возвращает их количество. Подсказка: type assertion `s.(Scaler)`.
func ScaleAll(shapes []Shape, k float64) int { panic("TODO") }

// Describe возвращает описание по динамическому типу (type switch):
//
//	nil                 -> "пусто"
//	Circle, *Circle     -> "круг"
//	Rect, *Rect         -> "квадрат", если W == H, иначе "прямоугольник"
//	(*Rect)(nil)        -> "пустой указатель"  (интерфейс НЕ nil, а указатель внутри — nil)
//	всё остальное       -> "неизвестная фигура"
func Describe(s Shape) string { panic("TODO") }
