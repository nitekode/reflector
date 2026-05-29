package reflector

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	ErrNotAStruct = errors.New("reflector: not a struct")
	ErrNotAFunc   = errors.New("reflector: not a func")
)

type DecodeTypeError struct {
	Type reflect.Type
}

func (e DecodeTypeError) Error() string {
	return fmt.Sprintf("decoder has no support for %s", e.Type)
}

type EncodeTypeError struct {
	Type reflect.Type
}

func (e EncodeTypeError) Error() string {
	return fmt.Sprintf("encoder has no support for %s", e.Type)
}

// FieldError reports a failure while decoding, encoding, or reaching a specific
// struct field. Field is the field's resolved name and Index is its path within
// the struct (see StructFieldInfo.Index). Unwrap returns the underlying cause,
// so errors.As still reaches errors like DecodeTypeError.
type FieldError struct {
	Field string
	Index []int
	Err   error
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("reflector: field %q: %s", e.Field, e.Err)
}

func (e *FieldError) Unwrap() error {
	return e.Err
}
