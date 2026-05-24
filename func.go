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
}

type funcInfo struct {
	Value      reflect.Value
	Params     []funcParamInfo
	Returns    []reflect.Type
	IsVariadic bool
	MinArgs    int
}

func InspectFunc(fn any) (fi funcInfo, err error) {
	if fn == nil {
		return fi, ErrNotAFunc
	}

	fi.Value = reflect.ValueOf(fn)
	typ := fi.Value.Type()

	// Check if this func has already been inspected and is in the cache
	if fi, found := funcCache.Load(typ); found {
		return fi.(funcInfo), nil
	}

	if typ.Kind() != reflect.Func {
		return fi, ErrNotAFunc
	}

	fi.Params = make([]funcParamInfo, typ.NumIn())
	for i := 0; i < typ.NumIn(); i++ {
		fi.Params[i] = funcParamInfo{
			Index: i,
			Type:  typ.In(i),
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
	}

	// Put the inspected func in the cache and prime it
	funcCache.LoadOrStore(typ, fi)

	return
}

func Call(fn any, inputs []any) ([]reflect.Value, error) {
	fi, err := InspectFunc(fn)
	if err != nil {
		return nil, err
	}

	if fi.IsVariadic && len(inputs) < fi.MinArgs {
		return nil, fmt.Errorf("reflector: expected at least %d args, got %d", fi.MinArgs, len(inputs))
	}
	if !fi.IsVariadic && len(inputs) != len(fi.Params) {
		return nil, fmt.Errorf("reflector: expected %d args, got %d", len(fi.Params), len(inputs))
	}

	args := make([]reflect.Value, 0, len(inputs))

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
			expectedType = pi.Type.Elem() // []string -> string
		}

		if !value.Type().AssignableTo(expectedType) {
			return nil, fmt.Errorf("reflector: param %d - expected %s, got %s", i, expectedType, value.Type())
		}

		args = append(args, value)
	}

	return fi.Value.Call(args), nil
}
