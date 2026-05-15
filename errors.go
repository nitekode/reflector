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

type ErrDecoderUnsupportedType struct {
	Type reflect.Type
}

func (e ErrDecoderUnsupportedType) Error() string {
	return fmt.Sprintf("decoder has no support for %s", e.Type)
}

type ErrEncoderUnsupportedType struct {
	Type reflect.Type
}

func (e ErrEncoderUnsupportedType) Error() string {
	return fmt.Sprintf("encoder has no support for %s", e.Type)
}
