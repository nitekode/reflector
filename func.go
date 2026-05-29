package reflector

import (
	"fmt"
	"reflect"
	"sync"
)

var funcCache sync.Map

type funcParamInfo struct {
	Index      int
	Type       reflect.Type
	IsVariadic bool
	Decode     fieldDecoder
}

type funcInfo struct {
	Params     []funcParamInfo
	Returns    []reflect.Type
	IsVariadic bool
	MinArgs    int
}

type callOptions struct {
	stringDecoding bool
}

type CallOption func(*callOptions)

func WithStringDecoding() CallOption {
	return func(opts *callOptions) {
		opts.stringDecoding = true
	}
}

func InspectFunc(fn any) (fi funcInfo, err error) {
	if fn == nil {
		return fi, ErrNotAFunc
	}

	typ := reflect.TypeOf(fn)

	// funcInfo holds only signature-derived metadata, so it is safe to share
	// across every function of this type.
	if cached, found := funcCache.Load(typ); found {
		return cached.(funcInfo), nil
	}

	if typ.Kind() != reflect.Func {
		return fi, ErrNotAFunc
	}

	fi.Params = make([]funcParamInfo, typ.NumIn())
	for i := 0; i < typ.NumIn(); i++ {
		fi.Params[i] = funcParamInfo{
			Index:  i,
			Type:   typ.In(i),
			Decode: pickDecoder(typ.In(i)),
		}
	}

	fi.Returns = make([]reflect.Type, typ.NumOut())
	for i := 0; i < typ.NumOut(); i++ {
		fi.Returns[i] = typ.Out(i)
	}

	fi.IsVariadic = typ.IsVariadic()
	fi.MinArgs = len(fi.Params)

	if fi.IsVariadic {
		// Variadic param can receive zero args
		fi.MinArgs--

		// Set the last param as variadic
		fi.Params[len(fi.Params)-1].IsVariadic = true
		fi.Params[len(fi.Params)-1].Decode = pickDecoder(fi.Params[len(fi.Params)-1].Type.Elem())
	}

	// Put the inspected func in the cache and prime it
	funcCache.LoadOrStore(typ, fi)

	return
}

func Call(fn any, inputs []any, opts ...CallOption) ([]reflect.Value, error) {
	fi, err := InspectFunc(fn)
	if err != nil {
		return nil, err
	}

	var callOpts callOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&callOpts)
		}
	}

	if fi.IsVariadic && len(inputs) < fi.MinArgs {
		return nil, fmt.Errorf("reflector: expected at least %d args, got %d", fi.MinArgs, len(inputs))
	}
	if !fi.IsVariadic && len(inputs) != len(fi.Params) {
		return nil, fmt.Errorf("reflector: expected %d args, got %d", len(fi.Params), len(inputs))
	}

	args := make([]reflect.Value, 0, len(inputs))
	useCallSlice := false

	for i, input := range inputs {
		value := reflect.ValueOf(input)
		paramIndex := i
		if i >= len(fi.Params) {
			// All extra args map to the variadic param
			paramIndex = len(fi.Params) - 1
		}

		pi := fi.Params[paramIndex]
		expectedType := pi.Type
		if pi.IsVariadic {
			if len(inputs) == len(fi.Params) && value.IsValid() && value.Type().AssignableTo(pi.Type) {
				useCallSlice = true
			} else {
				expectedType = pi.Type.Elem() // []string -> string
			}
		}

		if callOpts.stringDecoding {
			if raw, ok := input.(string); ok && !value.Type().AssignableTo(expectedType) {
				value = reflect.New(expectedType).Elem()
				err = pi.Decode(value, raw)
				if err != nil {
					return nil, fmt.Errorf("reflector: param %d - failed to decode string as %s: %w", i, expectedType, err)
				}
			}
		}

		if !value.Type().AssignableTo(expectedType) {
			return nil, fmt.Errorf("reflector: param %d - expected %s, got %s", i, expectedType, value.Type())
		}

		args = append(args, value)
	}

	fnVal := reflect.ValueOf(fn)
	if useCallSlice {
		return fnVal.CallSlice(args), nil
	}

	return fnVal.Call(args), nil
}
