package reflector

import (
	"reflect"
	"strconv"
)

type fieldEncoder func(field reflect.Value) (string, error)
type fieldDecoder func(field reflect.Value, raw string) error

func pickEncoder(t reflect.Type) fieldEncoder {
	switch t.Kind() {
	case reflect.String:
		return func(f reflect.Value) (string, error) {
			return f.String(), nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(f reflect.Value) (string, error) {
			return strconv.FormatInt(f.Int(), 10), nil
		}
	case reflect.Bool:
		return func(f reflect.Value) (string, error) {
			return strconv.FormatBool(f.Bool()), nil
		}
	case reflect.Float32, reflect.Float64:
		return func(f reflect.Value) (string, error) {
			return strconv.FormatFloat(f.Float(), 'f', -1, 64), nil
		}
	default:
		return func(f reflect.Value) (string, error) {
			return "", ErrEncoderUnsupportedType{Type: t}
		}
	}
}

func pickDecoder(t reflect.Type) fieldDecoder {
	switch t.Kind() {
	case reflect.String:
		return func(f reflect.Value, raw string) error {
			f.SetString(raw)
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(f reflect.Value, raw string) error {
			i, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return err
			}
			f.SetInt(i)
			return nil
		}
	case reflect.Bool:
		return func(f reflect.Value, raw string) error {
			b, err := strconv.ParseBool(raw)
			if err != nil {
				return err
			}
			f.SetBool(b)
			return nil
		}
	default:
		return func(f reflect.Value, raw string) error {
			return ErrDecoderUnsupportedType{t}
		}
	}
}
