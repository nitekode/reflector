package reflector

import (
	"errors"
	"reflect"
	"testing"
)

func TestInspectFunc(t *testing.T) {
	funcCache.Clear()

	add := func(a, b int) int { return a + b }
	sum := func(base int, nums ...int) int {
		total := base
		for _, num := range nums {
			total += num
		}
		return total
	}

	t.Run("non-variadic", func(t *testing.T) {
		fi, err := InspectFunc(add)
		if err != nil {
			t.Fatalf("InspectFunc() error = %v", err)
		}

		if fi.IsVariadic {
			t.Fatal("expected non-variadic function")
		}
		if fi.MinArgs != 2 {
			t.Fatalf("MinArgs = %d, want 2", fi.MinArgs)
		}
		if len(fi.Params) != 2 {
			t.Fatalf("len(Params) = %d, want 2", len(fi.Params))
		}
		if fi.Params[0].Type != reflect.TypeFor[int]() || fi.Params[1].Type != reflect.TypeFor[int]() {
			t.Fatalf("param types = [%v %v], want [int int]", fi.Params[0].Type, fi.Params[1].Type)
		}
		if len(fi.Returns) != 1 || fi.Returns[0] != reflect.TypeFor[int]() {
			t.Fatalf("returns = %v, want [int]", fi.Returns)
		}
	})

	t.Run("variadic", func(t *testing.T) {
		fi, err := InspectFunc(sum)
		if err != nil {
			t.Fatalf("InspectFunc() error = %v", err)
		}

		if !fi.IsVariadic {
			t.Fatal("expected variadic function")
		}
		if fi.MinArgs != 1 {
			t.Fatalf("MinArgs = %d, want 1", fi.MinArgs)
		}
		if !fi.Params[1].IsVariadic {
			t.Fatal("expected last param to be marked variadic")
		}
		if fi.Params[1].Type != reflect.TypeFor[[]int]() {
			t.Fatalf("variadic param type = %v, want []int", fi.Params[1].Type)
		}
	})

	t.Run("not a function", func(t *testing.T) {
		_, err := InspectFunc(123)
		if !errors.Is(err, ErrNotAFunc) {
			t.Fatalf("InspectFunc() error = %v, want %v", err, ErrNotAFunc)
		}
	})
}

func TestCall(t *testing.T) {
	funcCache.Clear()

	add := func(a, b int) int { return a + b }
	sum := func(base int, nums ...int) int {
		total := base
		for _, num := range nums {
			total += num
		}
		return total
	}

	tests := []struct {
		name     string
		fn       any
		inputs   []any
		want     []any
		wantErr  string
		checkErr error
	}{
		{
			name:   "fixed-arity",
			fn:     add,
			inputs: []any{2, 3},
			want:   []any{5},
		},
		{
			name:   "variadic",
			fn:     sum,
			inputs: []any{10, 1, 2, 3},
			want:   []any{16},
		},
		{
			name:    "wrong-arity",
			fn:      add,
			inputs:  []any{1},
			wantErr: "reflector: expected 2 args, got 1",
		},
		{
			name:    "wrong-type",
			fn:      add,
			inputs:  []any{1, "two"},
			wantErr: "reflector: param 1 - expected int, got string",
		},
		{
			name:     "not-a-function",
			fn:       "not-a-func",
			inputs:   nil,
			checkErr: ErrNotAFunc,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Call(tt.fn, tt.inputs)
			if tt.checkErr != nil {
				if !errors.Is(err, tt.checkErr) {
					t.Fatalf("Call() error = %v, want %v", err, tt.checkErr)
				}
				return
			}
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("Call() error = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Call() error = %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("len(results) = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if value := got[i].Interface(); value != tt.want[i] {
					t.Fatalf("result[%d] = %v, want %v", i, value, tt.want[i])
				}
			}
		})
	}
}
