package reflector

import (
	"fmt"
	"reflect"
	"sync"
)

var funcCache sync.Map

type FuncParamInfo struct {
	Index      int
	Type       reflect.Type
	IsVariadic bool
	decode     fieldDecoder
}

type FuncInfo struct {
	Params     []FuncParamInfo
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

func InspectFunc(fn any) (fi FuncInfo, err error) {
	if fn == nil {
		return fi, ErrNotAFunc
	}

	typ := reflect.TypeOf(fn)

	// FuncInfo holds only signature-derived metadata, so it is safe to share
	// across every function of this type.
	if cached, found := funcCache.Load(typ); found {
		return cached.(FuncInfo), nil
	}

	if typ.Kind() != reflect.Func {
		return fi, ErrNotAFunc
	}

	fi.Params = make([]FuncParamInfo, typ.NumIn())
	for i := 0; i < typ.NumIn(); i++ {
		fi.Params[i] = FuncParamInfo{
			Index:  i,
			Type:   typ.In(i),
			decode: pickDecoder(typ.In(i), nil),
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
		fi.Params[len(fi.Params)-1].decode = pickDecoder(fi.Params[len(fi.Params)-1].Type.Elem(), nil)
	}

	// Put the inspected func in the cache and prime it
	funcCache.LoadOrStore(typ, fi)

	return
}

var errorType = reflect.TypeFor[error]()

// Call invokes fn with the given inputs and returns its results as plain
// values. With WithStringDecoding, string inputs are decoded into the
// parameter types fn expects.
//
// Following Go convention, if fn's last return value is an error it is taken
// out of the results and returned as the error (nil when fn returned a nil
// error). So func() error gives an empty result slice plus fn's error,
// func() (T, error) gives []any{T} plus the error, and func() T gives []any{T}
// and a nil error. The same error return also carries reflector's own failures
// (wrong argument count, a decode failure); those are prefixed "reflector:".
func Call(fn any, inputs []any, opts ...CallOption) ([]any, error) {
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
				err = pi.decode(value, raw)
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
	var out []reflect.Value
	if useCallSlice {
		out = fnVal.CallSlice(args)
	} else {
		out = fnVal.Call(args)
	}

	results := make([]any, len(out))
	for i, v := range out {
		results[i] = v.Interface()
	}

	// Split off a trailing error return, if fn has one.
	if n := len(fi.Returns); n > 0 && fi.Returns[n-1].Implements(errorType) {
		last := results[n-1]
		results = results[:n-1]
		if last != nil {
			return results, last.(error)
		}
	}

	return results, nil
}
