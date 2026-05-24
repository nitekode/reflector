package reflector

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestInspectStruct(t *testing.T) {
	structCache.Clear()

	type embedded struct {
		Flag bool `flag:"embedded"`
	}

	type sample struct {
		embedded
		Name  string  `json:"name" note:"has spaces"`
		Age   int     `json:"age"`
		Score float64 `json:"score"`
		alive bool
	}

	si, err := InspectStruct(&sample{})
	if err != nil {
		t.Fatalf("InspectStruct() error = %v", err)
	}

	if si.Name != "sample" {
		t.Fatalf("Name = %q, want %q", si.Name, "sample")
	}
	if si.Type != reflect.TypeFor[sample]() {
		t.Fatalf("Type = %v, want %v", si.Type, reflect.TypeFor[sample]())
	}
	if len(si.Fields) != 5 {
		t.Fatalf("len(Fields) = %d, want 5", len(si.Fields))
	}
	if len(si.Exported) != 3 {
		t.Fatalf("len(Exported) = %d, want 3", len(si.Exported))
	}
	if si.Fields[0] == nil || !si.Fields[0].IsAnonymous {
		t.Fatal("expected first field to be the anonymous embedded field")
	}
	if si.Fields[1].Tags["json"] != "name" {
		t.Fatalf("Name json tag = %q, want %q", si.Fields[1].Tags["json"], "name")
	}
	if si.Fields[1].Tags["note"] != "has spaces" {
		t.Fatalf("Name note tag = %q, want %q", si.Fields[1].Tags["note"], "has spaces")
	}
}

func TestInspectStructErrors(t *testing.T) {
	structCache.Clear()

	_, err := InspectStruct(42)
	if !errors.Is(err, ErrNotAStruct) {
		t.Fatalf("InspectStruct() error = %v, want %v", err, ErrNotAStruct)
	}
}

func TestNewStruct(t *testing.T) {
	structCache.Clear()

	type sample struct {
		Name   string
		Age    int
		Active bool
		hidden string
	}

	got, err := NewStruct(sample{}, map[string]string{
		"Name":   "alice",
		"Age":    "42",
		"Active": "true",
		"hidden": "ignored",
	})
	if err != nil {
		t.Fatalf("NewStruct() error = %v", err)
	}

	want := sample{
		Name:   "alice",
		Age:    42,
		Active: true,
	}
	if got != want {
		t.Fatalf("NewStruct() = %#v, want %#v", got, want)
	}
}

func TestNewStructErrors(t *testing.T) {
	structCache.Clear()

	t.Run("decode failure", func(t *testing.T) {
		type sample struct {
			Age int
		}

		_, err := NewStruct(sample{}, map[string]string{"Age": "not-a-number"})
		if err == nil || !strings.Contains(err.Error(), `failed to decode field "Age"`) {
			t.Fatalf("NewStruct() error = %v, want decode error for Age", err)
		}
	})

	t.Run("unsupported decoder", func(t *testing.T) {
		type sample struct {
			Labels []string
		}

		_, err := NewStruct(sample{}, map[string]string{"Labels": "a,b"})
		var target ErrDecoderUnsupportedType
		if !errors.As(err, &target) {
			t.Fatalf("NewStruct() error = %v, want ErrDecoderUnsupportedType", err)
		}
		if target.Type != reflect.TypeFor[[]string]() {
			t.Fatalf("unsupported decoder type = %v, want []string", target.Type)
		}
	})
}

func TestToMap(t *testing.T) {
	structCache.Clear()

	type sample struct {
		Name   string
		Age    int
		Active bool
		Score  float64
		hidden string
	}

	input := &sample{
		Name:   "alice",
		Age:    42,
		Active: true,
		Score:  12.5,
		hidden: "ignored",
	}

	got, err := ToMap(input)
	if err != nil {
		t.Fatalf("ToMap() error = %v", err)
	}

	want := map[string]string{
		"Name":   "alice",
		"Age":    "42",
		"Active": "true",
		"Score":  strconv.FormatFloat(12.5, 'f', -1, 64),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToMap() = %#v, want %#v", got, want)
	}
}

func TestToMapErrors(t *testing.T) {
	structCache.Clear()

	t.Run("unsupported encoder", func(t *testing.T) {
		type sample struct {
			Labels []string
		}

		_, err := ToMap(sample{Labels: []string{"a", "b"}})
		var target ErrEncoderUnsupportedType
		if !errors.As(err, &target) {
			t.Fatalf("ToMap() error = %v, want ErrEncoderUnsupportedType", err)
		}
		if target.Type != reflect.TypeFor[[]string]() {
			t.Fatalf("unsupported encoder type = %v, want []string", target.Type)
		}
	})

	t.Run("nil pointer", func(t *testing.T) {
		type sample struct {
			Name string
		}

		var input *sample
		_, err := ToMap(input)
		if !errors.Is(err, ErrNotAStruct) {
			t.Fatalf("ToMap() error = %v, want %v", err, ErrNotAStruct)
		}
	})
}

func TestParseStructTag(t *testing.T) {
	tags := parseStructTag(`json:"name,omitempty" note:"a \"quoted\" value" empty:"" malformed`)

	want := map[string]string{
		"json":  "name,omitempty",
		"note":  `a "quoted" value`,
		"empty": "",
	}
	if !reflect.DeepEqual(tags, want) {
		t.Fatalf("parseStructTag() = %#v, want %#v", tags, want)
	}
}
