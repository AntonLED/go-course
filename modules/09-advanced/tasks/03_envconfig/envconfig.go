//go:build !solution

package envconfig

// Load заполняет структуру *dst значениями переменных окружения по тегам:
//
//	env:"PORT"        — имя переменной
//	default:"8080"    — значение, если переменная НЕ задана
//	required:"true"   — ошибка ErrRequired, если переменная не задана или пуста
//	sep:";"           — разделитель для срезов (по умолчанию ",")
//	prefix:"DB_"      — для вложенных структур: префикс имён их переменных
//
// Все ошибки собираются через errors.Join (не останавливаться на первой).
func Load(dst any, lookup LookupFunc) error {
	// TODO: reflect.ValueOf(dst).Elem(), обход полей, CanSet, SetInt/SetString/...
	panic("TODO")
}
