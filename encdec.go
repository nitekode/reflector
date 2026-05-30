package reflector

import (
	"fmt"
	"net"
	"net/url"
	"reflect"
	"strconv"
	"time"
)

type fieldEncoder func(field reflect.Value) (string, error)
type fieldDecoder func(field reflect.Value, raw string) error

// customEncoders and customDecoders hold converters registered for specific
// types via AddEncoder and AddDecoder. pickEncoder and pickDecoder check them
// before falling back to the built-in kind switches.
//
// Writes happen through AddEncoder/AddDecoder, reads happen when a struct is
// first inspected. Register your custom types before the library inspects any
// struct that uses them (in practice, at program start) so there is no
// concurrent write and read.
var (
	customDecoders = map[reflect.Type]fieldDecoder{}
	customEncoders = map[reflect.Type]fieldEncoder{}
)

// AddDecoder registers a decoder for type T. After this, any struct field of
// type T is filled by calling fn with the raw string instead of using the
// built-in decoding. This is how you teach reflector a type its built-in
// decoding does not cover, such as time.Duration or your own named types.
//
// A decoder registered for T takes priority over the built-in handling for T's
// kind, so it also works for named types whose underlying kind is a built-in
// one (time.Duration is an int64, for example). Registering T again replaces
// the earlier decoder.
func AddDecoder[T any](fn func(raw string) (T, error)) {
	t := reflect.TypeOf((*T)(nil)).Elem()
	customDecoders[t] = func(field reflect.Value, raw string) error {
		v, err := fn(raw)
		if err != nil {
			return err
		}
		field.Set(reflect.ValueOf(v))
		return nil
	}
}

// AddEncoder registers an encoder for type T. After this, any struct field of
// type T is turned into a string by calling fn instead of using the built-in
// encoding. It is the encode-side counterpart to AddDecoder and follows the
// same priority and replacement rules.
func AddEncoder[T any](fn func(v T) (string, error)) {
	t := reflect.TypeOf((*T)(nil)).Elem()
	customEncoders[t] = func(field reflect.Value) (string, error) {
		return fn(field.Interface().(T))
	}
}

func pickEncoder(t reflect.Type, tags map[string]string) fieldEncoder {
	if h, ok := customEncoders[t]; ok {
		return h
	}

	switch t {
	case reflect.TypeFor[time.Duration]():
		return func(f reflect.Value) (string, error) {
			return time.Duration(f.Int()).String(), nil
		}
	case reflect.TypeFor[time.Time]():
		layout := tags["time_format"]
		if layout == "" {
			layout = tags["layout"]
		}
		return func(f reflect.Value) (string, error) {
			tm := f.Interface().(time.Time)
			if layout != "" {
				return tm.Format(layout), nil
			}
			b, err := tm.MarshalText()
			return string(b), err
		}
	case reflect.TypeFor[url.URL]():
		return func(f reflect.Value) (string, error) {
			u := f.Interface().(url.URL)
			return u.String(), nil
		}
	case reflect.TypeFor[net.IP]():
		return func(f reflect.Value) (string, error) {
			return f.Interface().(net.IP).String(), nil
		}
	}

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
			return "", EncodeTypeError{Type: t}
		}
	}
}

func pickDecoder(t reflect.Type, tags map[string]string) fieldDecoder {
	if h, ok := customDecoders[t]; ok {
		return h
	}

	switch t {
	case reflect.TypeFor[time.Duration]():
		return func(f reflect.Value, raw string) error {
			d, err := time.ParseDuration(raw)
			if err != nil {
				return err
			}
			f.SetInt(int64(d))
			return nil
		}
	case reflect.TypeFor[time.Time]():
		layout := tags["time_format"]
		if layout == "" {
			layout = tags["layout"]
		}
		return func(f reflect.Value, raw string) error {
			var tm time.Time
			if layout != "" {
				parsed, err := time.Parse(layout, raw)
				if err != nil {
					return err
				}
				tm = parsed
			} else if err := tm.UnmarshalText([]byte(raw)); err != nil {
				return err
			}
			f.Set(reflect.ValueOf(tm))
			return nil
		}
	case reflect.TypeFor[url.URL]():
		return func(f reflect.Value, raw string) error {
			u, err := url.Parse(raw)
			if err != nil {
				return err
			}
			f.Set(reflect.ValueOf(*u))
			return nil
		}
	case reflect.TypeFor[net.IP]():
		return func(f reflect.Value, raw string) error {
			ip := net.ParseIP(raw)
			if ip == nil {
				return fmt.Errorf("invalid IP address %q", raw)
			}
			f.Set(reflect.ValueOf(ip))
			return nil
		}
	}

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
	case reflect.Float32, reflect.Float64:
		return func(f reflect.Value, raw string) error {
			n, err := strconv.ParseFloat(raw, t.Bits())
			if err != nil {
				return err
			}
			f.SetFloat(n)
			return nil
		}
	default:
		return func(f reflect.Value, raw string) error {
			return DecodeTypeError{t}
		}
	}
}
