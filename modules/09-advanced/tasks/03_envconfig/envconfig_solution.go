//go:build solution

package envconfig

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
)

var (
	durationType        = reflect.TypeFor[time.Duration]()
	textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// Load заполняет структуру *dst значениями переменных окружения по тегам.
func Load(dst any, lookup LookupFunc) error {
	rv := reflect.ValueOf(dst)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return ErrInvalidTarget
	}
	var errs []error
	loadStruct(rv.Elem(), "", "", lookup, &errs)
	return errors.Join(errs...)
}

func loadStruct(sv reflect.Value, pathPrefix, envPrefix string, lookup LookupFunc, errs *[]error) {
	st := sv.Type()
	for i := 0; i < st.NumField(); i++ {
		sf := st.Field(i)
		if !sf.IsExported() {
			continue
		}
		fv := sv.Field(i) // адресуемо, т.к. sv получен через Elem() указателя
		path := pathPrefix + sf.Name
		key, hasKey := sf.Tag.Lookup("env")

		// Вложенная структура без env-тега (и не TextUnmarshaler) — рекурсия.
		if !hasKey && sf.Type.Kind() == reflect.Struct && !isTextUnmarshaler(sf.Type) {
			loadStruct(fv, path+".", envPrefix+sf.Tag.Get("prefix"), lookup, errs)
			continue
		}
		if !hasKey || key == "" {
			continue
		}
		key = envPrefix + key

		raw, ok := lookup(key)
		if !ok {
			// default применяется только если переменной нет (а не если она пустая).
			raw, ok = sf.Tag.Lookup("default")
		}
		if sf.Tag.Get("required") == "true" && raw == "" {
			*errs = append(*errs, &FieldError{Field: path, Key: key, Err: ErrRequired})
			continue
		}
		if !ok {
			continue // не задано и нет default — оставляем как есть
		}
		sep := sf.Tag.Get("sep")
		if sep == "" {
			sep = ","
		}
		if err := setValue(fv, raw, sep); err != nil {
			*errs = append(*errs, &FieldError{Field: path, Key: key, Err: err})
		}
	}
}

func isTextUnmarshaler(t reflect.Type) bool {
	return reflect.PointerTo(t).Implements(textUnmarshalerType)
}

// setValue разбирает raw в значение типа поля v (v должно быть settable).
func setValue(v reflect.Value, raw, sep string) error {
	t := v.Type()

	// 1. Типы с собственным парсером: *T реализует encoding.TextUnmarshaler.
	if v.CanAddr() && isTextUnmarshaler(t) {
		return v.Addr().Interface().(encoding.TextUnmarshaler).UnmarshalText([]byte(raw))
	}
	// 2. time.Duration — это int64, поэтому проверяем ДО switch по Kind.
	if t == durationType {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		v.SetInt(int64(d))
		return nil
	}

	switch t.Kind() {
	case reflect.Pointer:
		// Выделяем память и заполняем значение под указателем.
		p := reflect.New(t.Elem())
		if err := setValue(p.Elem(), raw, sep); err != nil {
			return err
		}
		v.Set(p)
	case reflect.String:
		v.SetString(raw)
	case reflect.Bool:
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		// bitSize = t.Bits(): "300" в int8 — ошибка переполнения, а не молчаливое 44.
		n, err := strconv.ParseInt(raw, 10, t.Bits())
		if err != nil {
			return err
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := strconv.ParseUint(raw, 10, t.Bits())
		if err != nil {
			return err
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		f, err := strconv.ParseFloat(raw, t.Bits())
		if err != nil {
			return err
		}
		v.SetFloat(f)
	case reflect.Slice:
		if strings.TrimSpace(raw) == "" {
			v.Set(reflect.MakeSlice(t, 0, 0))
			return nil
		}
		parts := strings.Split(raw, sep)
		s := reflect.MakeSlice(t, len(parts), len(parts))
		for i, p := range parts {
			if err := setValue(s.Index(i), strings.TrimSpace(p), sep); err != nil {
				return fmt.Errorf("элемент %d: %w", i, err)
			}
		}
		v.Set(s)
	default:
		return fmt.Errorf("%w: %s", ErrUnsupported, t)
	}
	return nil
}
