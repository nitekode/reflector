package reflector

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
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

func TestCallDistinguishesSameSignatureFuncs(t *testing.T) {
	funcCache.Clear()

	// Two different functions with the identical signature func(int) int. The
	// first call caches the type metadata; the second must still invoke itself,
	// not the cached function.
	a := func(x int) int { return x + 1 }
	b := func(x int) int { return x + 100 }

	out1, err := Call(a, []any{10})
	if err != nil {
		t.Fatalf("Call(a) error = %v", err)
	}
	out2, err := Call(b, []any{10})
	if err != nil {
		t.Fatalf("Call(b) error = %v", err)
	}

	if out1[0] != 11 {
		t.Fatalf("a(10) = %v, want 11", out1[0])
	}
	if out2[0] != 110 {
		t.Fatalf("b(10) = %v, want 110 (called the wrong function)", out2[0])
	}
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
			name:   "variadic with no extra args",
			fn:     sum,
			inputs: []any{10},
			want:   []any{10},
		},
		{
			name:   "variadic slice input",
			fn:     sum,
			inputs: []any{10, []int{1, 2, 3}},
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
				if got[i] != tt.want[i] {
					t.Fatalf("result[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestCallSplitsTrailingError(t *testing.T) {
	funcCache.Clear()

	t.Run("error only, nil", func(t *testing.T) {
		fn := func() error { return nil }
		got, err := Call(fn, nil)
		if err != nil {
			t.Fatalf("Call() error = %v, want nil", err)
		}
		if len(got) != 0 {
			t.Fatalf("results = %v, want empty", got)
		}
	})

	t.Run("error only, non-nil", func(t *testing.T) {
		want := errors.New("boom")
		fn := func() error { return want }
		got, err := Call(fn, nil)
		if !errors.Is(err, want) {
			t.Fatalf("Call() error = %v, want %v", err, want)
		}
		if len(got) != 0 {
			t.Fatalf("results = %v, want empty", got)
		}
	})

	t.Run("value and error", func(t *testing.T) {
		fn := func(n int) (int, error) { return n * 2, nil }
		got, err := Call(fn, []any{21})
		if err != nil {
			t.Fatalf("Call() error = %v", err)
		}
		if len(got) != 1 || got[0] != 42 {
			t.Fatalf("results = %v, want [42]", got)
		}
	})

	t.Run("no error in signature", func(t *testing.T) {
		fn := func() int { return 7 }
		got, err := Call(fn, nil)
		if err != nil {
			t.Fatalf("Call() error = %v", err)
		}
		if len(got) != 1 || got[0] != 7 {
			t.Fatalf("results = %v, want [7]", got)
		}
	})
}

func TestCallWithStringDecoding(t *testing.T) {
	funcCache.Clear()

	add := func(a, b int) int { return a + b }
	mix := func(enabled bool, ratio float64, name string) string {
		return name + ":" + strconv.FormatBool(enabled) + ":" + strconv.FormatFloat(ratio, 'f', -1, 64)
	}
	sum := func(base int, nums ...int) int {
		total := base
		for _, num := range nums {
			total += num
		}
		return total
	}
	takesSlice := func(values []string) int { return len(values) }

	t.Run("fixed-arity decode", func(t *testing.T) {
		got, err := Call(add, []any{"2", "3"}, WithStringDecoding())
		if err != nil {
			t.Fatalf("Call() error = %v", err)
		}

		if len(got) != 1 || got[0] != 5 {
			t.Fatalf("Call() = %v, want [5]", got)
		}
	})

	t.Run("mixed typed and decoded inputs", func(t *testing.T) {
		got, err := Call(mix, []any{true, "1.5", "alice"}, WithStringDecoding())
		if err != nil {
			t.Fatalf("Call() error = %v", err)
		}

		if len(got) != 1 || got[0] != "alice:true:1.5" {
			t.Fatalf("Call() = %v, want [alice:true:1.5]", got)
		}
	})

	t.Run("variadic decode", func(t *testing.T) {
		got, err := Call(sum, []any{"10", "1", "2", "3"}, WithStringDecoding())
		if err != nil {
			t.Fatalf("Call() error = %v", err)
		}

		if len(got) != 1 || got[0] != 16 {
			t.Fatalf("Call() = %v, want [16]", got)
		}
	})

	t.Run("variadic decode with no extra args", func(t *testing.T) {
		got, err := Call(sum, []any{"10"}, WithStringDecoding())
		if err != nil {
			t.Fatalf("Call() error = %v", err)
		}

		if len(got) != 1 || got[0] != 10 {
			t.Fatalf("Call() = %v, want [10]", got)
		}
	})

	t.Run("variadic mixed typed and decoded args", func(t *testing.T) {
		got, err := Call(sum, []any{10, "1", 2, "3"}, WithStringDecoding())
		if err != nil {
			t.Fatalf("Call() error = %v", err)
		}

		if len(got) != 1 || got[0] != 16 {
			t.Fatalf("Call() = %v, want [16]", got)
		}
	})

	t.Run("strict by default", func(t *testing.T) {
		_, err := Call(add, []any{"2", "3"})
		if err == nil || err.Error() != "reflector: param 0 - expected int, got string" {
			t.Fatalf("Call() error = %v, want strict type mismatch", err)
		}
	})

	t.Run("unsupported decode type", func(t *testing.T) {
		_, err := Call(takesSlice, []any{"a,b"}, WithStringDecoding())
		var target ErrDecoderUnsupportedType
		if !errors.As(err, &target) {
			t.Fatalf("Call() error = %v, want ErrDecoderUnsupportedType", err)
		}
		if target.Type != reflect.TypeFor[[]string]() {
			t.Fatalf("unsupported decoder type = %v, want []string", target.Type)
		}
	})

	t.Run("invalid decode value", func(t *testing.T) {
		_, err := Call(add, []any{"2", "three"}, WithStringDecoding())
		if err == nil {
			t.Fatal("Call() error = nil, want decode failure")
		}
		if !strings.Contains(err.Error(), "param 1") {
			t.Fatalf("Call() error = %q, want param context", err.Error())
		}
		if !strings.Contains(err.Error(), "invalid syntax") {
			t.Fatalf("Call() error = %q, want parse failure", err.Error())
		}
	})
}
