//go:build solution

package validate

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Validate проверяет структуру (или указатель на структуру) по тегам `validate:"..."`.
func Validate(v any) error {
	rv := reflect.ValueOf(v)
	// Разыменовываем указатели: Validate(&cfg) и Validate(cfg) эквивалентны.
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return ErrNotStruct
		}
		rv = rv.Elem()
	}
	if rv.Kind() != reflect.Struct {
		return ErrNotStruct
	}
	var errs ValidationErrors
	if err := walkStruct(rv, "", &errs); err != nil {
		return err
	}
	if len(errs) > 0 {
		return errs
	}
	return nil
}

// walkStruct обходит поля структуры. prefix — путь до структуры ("" для корня).
func walkStruct(rv reflect.Value, prefix string, errs *ValidationErrors) error {
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		sf := rt.Field(i)
		// Неэкспортируемые поля нельзя прочитать через Interface(),
		// и валидировать их — не наше дело.
		if !sf.IsExported() {
			continue
		}
		tag := sf.Tag.Get("validate")
		if tag == "-" {
			continue
		}
		path := sf.Name
		if prefix != "" {
			path = prefix + "." + sf.Name
		}
		fv := rv.Field(i)
		if err := checkField(fv, tag, path, errs); err != nil {
			return err
		}
		if err := descend(fv, path, errs); err != nil {
			return err
		}
	}
	return nil
}

// descend рекурсивно заходит во вложенные структуры, указатели на них
// и срезы/массивы структур.
func descend(fv reflect.Value, path string, errs *ValidationErrors) error {
	switch fv.Kind() {
	case reflect.Pointer:
		if fv.IsNil() {
			return nil
		}
		return descend(fv.Elem(), path, errs)
	case reflect.Struct:
		return walkStruct(fv, path, errs)
	case reflect.Slice, reflect.Array:
		// Заходим только в элементы-структуры (или указатели на них):
		// правила тега относятся к самому срезу, а не к элементам.
		et := fv.Type().Elem()
		for et.Kind() == reflect.Pointer {
			et = et.Elem()
		}
		if et.Kind() != reflect.Struct {
			return nil
		}
		for j := 0; j < fv.Len(); j++ {
			if err := descend(fv.Index(j), fmt.Sprintf("%s[%d]", path, j), errs); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkField применяет правила тега к значению поля. Первое нарушенное
// правило записывается в errs, остальные правила поля пропускаются.
func checkField(fv reflect.Value, tag, path string, errs *ValidationErrors) error {
	if tag == "" {
		return nil
	}
	for _, rule := range strings.Split(tag, ",") {
		name, param, _ := strings.Cut(strings.TrimSpace(rule), "=")
		ok, err := apply(fv, name, param)
		if err != nil {
			return fmt.Errorf("%w: поле %s, правило %q: %v", ErrBadTag, path, rule, err)
		}
		if name == "omitempty" {
			if !ok { // значение пустое — остальные правила не применяем
				return nil
			}
			continue
		}
		if !ok {
			*errs = append(*errs, FieldError{Field: path, Rule: name, Param: param})
			return nil
		}
	}
	return nil
}

// isEmpty: нулевое значение либо пустая строка/срез/map.
func isEmpty(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Slice, reflect.Map, reflect.String:
		return v.Len() == 0
	}
	// Для остальных (в т.ч. массивов) — нулевое значение.
	return v.IsZero()
}

// apply возвращает ok=true, если правило выполнено. Для omitempty ok=false
// означает «значение пустое».
func apply(v reflect.Value, name, param string) (bool, error) {
	switch name {
	case "required":
		return !isEmpty(v), nil
	case "omitempty":
		return !isEmpty(v), nil
	}

	switch name {
	case "min", "max", "email", "oneof":
	default:
		// Неизвестное правило — ошибка тега, даже если значение nil.
		return false, fmt.Errorf("неизвестное правило %q", name)
	}

	// Остальные правила применяются к значению под указателем.
	// nil-указатель без required считаем «нечего проверять».
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return true, nil
		}
		v = v.Elem()
	}

	switch name {
	case "min", "max":
		return checkBound(v, name, param)
	case "email":
		if v.Kind() != reflect.String {
			return false, fmt.Errorf("email применим только к строкам, а не к %s", v.Kind())
		}
		return isEmail(v.String()), nil
	case "oneof":
		if param == "" {
			return false, fmt.Errorf("oneof без вариантов")
		}
		var s string
		switch v.Kind() {
		case reflect.String:
			s = v.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			s = strconv.FormatInt(v.Int(), 10)
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			s = strconv.FormatUint(v.Uint(), 10)
		default:
			return false, fmt.Errorf("oneof неприменим к %s", v.Kind())
		}
		for _, opt := range strings.Fields(param) {
			if opt == s {
				return true, nil
			}
		}
		return false, nil
	}
	return false, fmt.Errorf("неизвестное правило %q", name)
}

// checkBound реализует min/max: длина для строк (в рунах!), срезов и map,
// значение — для чисел.
func checkBound(v reflect.Value, name, param string) (bool, error) {
	bound, err := strconv.ParseFloat(param, 64)
	if err != nil {
		return false, fmt.Errorf("параметр %q не число", param)
	}
	var x float64
	switch v.Kind() {
	case reflect.String:
		x = float64(utf8.RuneCountInString(v.String()))
	case reflect.Slice, reflect.Array, reflect.Map:
		x = float64(v.Len())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		x = float64(v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		x = float64(v.Uint())
	case reflect.Float32, reflect.Float64:
		x = v.Float()
	default:
		return false, fmt.Errorf("%s неприменим к %s", name, v.Kind())
	}
	if name == "min" {
		return x >= bound, nil
	}
	return x <= bound, nil
}

// isEmail — упрощённая проверка: ровно одна '@', непустая локальная часть,
// в домене есть точка не по краям, нет пробельных символов.
func isEmail(s string) bool {
	if strings.IndexFunc(s, unicode.IsSpace) >= 0 || strings.Count(s, "@") != 1 {
		return false
	}
	local, domain, _ := strings.Cut(s, "@")
	if local == "" || !strings.Contains(domain, ".") {
		return false
	}
	return !strings.HasPrefix(domain, ".") && !strings.HasSuffix(domain, ".") && !strings.Contains(domain, "..")
}
