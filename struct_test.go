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

	type Embedded struct {
		Flag bool `flag:"embedded"`
	}

	type sample struct {
		Embedded
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

	// Fields are flattened: the promoted Flag plus the three exported direct
	// fields. The anonymous embedded field and the unexported alive are not
	// leaf fields.
	if len(si.Fields) != 4 {
		t.Fatalf("len(Fields) = %d, want 4", len(si.Fields))
	}

	flag := fieldByName(si, "Flag")
	if flag == nil {
		t.Fatal("expected promoted Flag field")
	}
	if flag.FromEmbedded != reflect.TypeFor[Embedded]() {
		t.Fatalf("Flag.FromEmbedded = %v, want %v", flag.FromEmbedded, reflect.TypeFor[Embedded]())
	}
	if !reflect.DeepEqual(flag.Index, []int{0, 0}) {
		t.Fatalf("Flag.Index = %v, want [0 0]", flag.Index)
	}

	name := fieldByName(si, "Name")
	if name == nil {
		t.Fatal("expected Name field")
	}
	if name.FromEmbedded != nil {
		t.Fatalf("Name.FromEmbedded = %v, want nil", name.FromEmbedded)
	}
	if !reflect.DeepEqual(name.Index, []int{1}) {
		t.Fatalf("Name.Index = %v, want [1]", name.Index)
	}
	if name.Tags["json"] != "name" {
		t.Fatalf("Name json tag = %q, want %q", name.Tags["json"], "name")
	}
	if name.Tags["note"] != "has spaces" {
		t.Fatalf("Name note tag = %q, want %q", name.Tags["note"], "has spaces")
	}

	if !si.Embeds(Embedded{}) {
		t.Fatal("expected sample to embed Embedded")
	}
}

func fieldByName(si structInfo, name string) *structFieldInfo {
	for _, f := range si.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func TestInspectStructErrors(t *testing.T) {
	structCache.Clear()

	_, err := InspectStruct(42)
	if !errors.Is(err, ErrNotAStruct) {
		t.Fatalf("InspectStruct() error = %v, want %v", err, ErrNotAStruct)
	}
}

func TestStructInfoEmbeds(t *testing.T) {
	structCache.Clear()

	type Embedded struct {
		Flag bool
	}

	type Other struct {
		Name string
	}

	type Nested struct {
		Embedded
	}

	type sample struct {
		Embedded
		*Other
		Named Embedded
		Nested
	}

	type directOnly struct {
		Nested
	}

	type namedOnly struct {
		Pointer *Other
	}

	si, err := InspectStruct(sample{})
	if err != nil {
		t.Fatalf("InspectStruct() error = %v", err)
	}

	directOnlyInfo, err := InspectStruct(directOnly{})
	if err != nil {
		t.Fatalf("InspectStruct() error = %v", err)
	}

	namedOnlyInfo, err := InspectStruct(namedOnly{})
	if err != nil {
		t.Fatalf("InspectStruct() error = %v", err)
	}

	tests := []struct {
		name   string
		si     structInfo
		target any
		want   bool
	}{
		{
			name:   "direct anonymous value embed",
			si:     si,
			target: Embedded{},
			want:   true,
		},
		{
			name:   "direct anonymous pointer embed matches underlying struct",
			si:     si,
			target: &Other{},
			want:   true,
		},
		{
			name:   "reflect.Type target",
			si:     si,
			target: reflect.TypeFor[Embedded](),
			want:   true,
		},
		{
			name:   "named field does not match",
			si:     namedOnlyInfo,
			target: Other{},
			want:   false,
		},
		{
			name:   "direct nested type matches",
			si:     si,
			target: reflect.TypeFor[Nested](),
			want:   true,
		},
		{
			name:   "transitively nested target matches",
			si:     directOnlyInfo,
			target: reflect.TypeFor[Embedded](),
			want:   true,
		},
		{
			name:   "unrelated type does not match",
			si:     si,
			target: struct{ Count int }{},
			want:   false,
		},
		{
			name:   "nil target",
			si:     si,
			target: nil,
			want:   false,
		},
		{
			name:   "non-struct target",
			si:     si,
			target: 42,
			want:   false,
		},
		{
			name:   "pointer to non-struct target",
			si:     si,
			target: new(int),
			want:   false,
		},
	}

	for _, tt := range tests {
		if got := tt.si.Embeds(tt.target); got != tt.want {
			t.Fatalf("%s: Embeds(%v) = %v, want %v", tt.name, tt.target, got, tt.want)
		}
	}
}

func TestStructInfoEmbedsUnexportedField(t *testing.T) {
	structCache.Clear()

	type group struct {
		Name string
	}

	type command struct {
		group
		Title string
	}

	si, err := InspectStruct(command{})
	if err != nil {
		t.Fatalf("InspectStruct() error = %v", err)
	}

	// Unexported embeds are still navigable, so Embeds reports them.
	if got := si.Embeds(group{}); !got {
		t.Fatalf("Embeds(group{}) = %v, want true", got)
	}
}

func TestNewStruct(t *testing.T) {
	structCache.Clear()

	type sample struct {
		Name   string
		Age    int
		Active bool
	}

	// With no default tag configured, NewStruct just returns the zero value.
	got, err := NewStruct[sample]()
	if err != nil {
		t.Fatalf("NewStruct() error = %v", err)
	}
	if got != (sample{}) {
		t.Fatalf("NewStruct() = %#v, want zero value", got)
	}
}

func TestNewStructAppliesDefaults(t *testing.T) {
	structCache.Clear()

	type sample struct {
		Name   string  `default:"alice"`
		Age    int     `default:"42"`
		Active bool    `default:"true"`
		Score  float64 `default:"12.5"`
		Empty  string  `default:""`
	}

	got, err := NewStruct[sample](WithDefaultTag("default"))
	if err != nil {
		t.Fatalf("NewStruct() error = %v", err)
	}

	want := sample{
		Name:   "alice",
		Age:    42,
		Active: true,
		Score:  12.5,
	}
	if got != want {
		t.Fatalf("NewStruct() = %#v, want %#v", got, want)
	}
}

func TestNewStructAppliesEmbeddedDefaults(t *testing.T) {
	structCache.Clear()

	type global struct {
		Verbose bool `default:"true"`
	}
	type group struct {
		global
		Greeting string `default:"hi"`
	}
	type command struct {
		group
		Name string `default:"deploy"`
	}

	// Defaults are applied at every depth, not just on the top struct: the
	// promoted Verbose field carries its default tag and is written through the
	// embedded structs.
	got, err := NewStruct[command](WithDefaultTag("default"))
	if err != nil {
		t.Fatalf("NewStruct() error = %v", err)
	}

	want := command{
		group: group{
			global:   global{Verbose: true},
			Greeting: "hi",
		},
		Name: "deploy",
	}
	if got != want {
		t.Fatalf("NewStruct() = %#v, want %#v", got, want)
	}
}

func TestNewStructThenFillFromMap(t *testing.T) {
	structCache.Clear()

	type sample struct {
		Name  string `json:"name" default:"alice"`
		Alias string `json:"alias" default:"guest"`
		Age   int    `json:"age" default:"42"`
	}

	// The intended layering: defaults form the base, the parsed map overlays
	// them, and fields the map omits keep their default.
	got, err := NewStruct[sample](WithDefaultTag("default"))
	if err != nil {
		t.Fatalf("NewStruct() error = %v", err)
	}
	if err := FillFromMap(&got, map[string]string{
		"name":  "bob",
		"alias": "",
	}, WithNameTag("json")); err != nil {
		t.Fatalf("FillFromMap() error = %v", err)
	}

	want := sample{
		Name:  "bob", // overlaid by the map
		Alias: "",    // overlaid with an empty value
		Age:   42,    // omitted by the map, default kept
	}
	if got != want {
		t.Fatalf("result = %#v, want %#v", got, want)
	}
}

func TestNewStructErrors(t *testing.T) {
	structCache.Clear()

	t.Run("invalid default value", func(t *testing.T) {
		type sample struct {
			Age int `default:"not-a-number"`
		}

		_, err := NewStruct[sample](WithDefaultTag("default"))
		if err == nil || !strings.Contains(err.Error(), `failed to decode default for field "Age"`) {
			t.Fatalf("NewStruct() error = %v, want default decode error for Age", err)
		}
	})

	t.Run("unsupported decoder for default", func(t *testing.T) {
		type sample struct {
			Labels []string `default:"a,b"`
		}

		_, err := NewStruct[sample](WithDefaultTag("default"))
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
	}

	input := &sample{
		Name:   "alice",
		Age:    42,
		Active: true,
		Score:  12.5,
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

func TestToMapWithNameTag(t *testing.T) {
	structCache.Clear()

	type sample struct {
		Name   string  `json:"name"`
		Age    int     `json:"age,omitempty"`
		Score  float64 `json:"score"`
		Hidden string  `json:"-"`
		Active bool
	}

	got, err := ToMap(sample{
		Name:   "alice",
		Age:    42,
		Score:  12.5,
		Hidden: "ignored",
		Active: true,
	}, WithNameTag("json"))
	if err != nil {
		t.Fatalf("ToMap() error = %v", err)
	}

	want := map[string]string{
		"name":   "alice",
		"age":    "42",
		"score":  strconv.FormatFloat(12.5, 'f', -1, 64),
		"Active": "true",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToMap() = %#v, want %#v", got, want)
	}
}

func TestToMapIncludesPromotedFields(t *testing.T) {
	structCache.Clear()

	type Common struct {
		Verbose bool
	}

	type sample struct {
		Common
		Name string
	}

	got, err := ToMap(sample{
		Common: Common{Verbose: true},
		Name:   "alice",
	})
	if err != nil {
		t.Fatalf("ToMap() error = %v", err)
	}

	want := map[string]string{
		"Verbose": "true",
		"Name":    "alice",
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

	t.Run("duplicate tagged names", func(t *testing.T) {
		type sample struct {
			First  string `opt:"name"`
			Second string `opt:"name,omitempty"`
		}

		_, err := ToMap(sample{First: "a", Second: "b"}, WithNameTag("opt"))
		if err == nil || !strings.Contains(err.Error(), `duplicate field name "name"`) {
			t.Fatalf("ToMap() error = %v, want duplicate field name error", err)
		}
	})
}

func TestFillFromStruct(t *testing.T) {
	structCache.Clear()

	type global struct {
		Verbose  bool
		internal int
	}
	type group struct {
		global
		Greeting string
	}
	type command struct {
		group
		Name string
	}

	cmd := command{Name: "deploy"}

	// Each source carries only its own fields, mirroring independent parsing.
	if err := FillFromStruct(&cmd, group{Greeting: "hello"}); err != nil {
		t.Fatalf("FillFromStruct(group) error = %v", err)
	}
	// Filling global after group must not clobber the Greeting set above:
	// own-fields merge makes the order irrelevant.
	if err := FillFromStruct(&cmd, global{Verbose: true}); err != nil {
		t.Fatalf("FillFromStruct(global) error = %v", err)
	}

	want := command{
		group: group{
			global:   global{Verbose: true},
			Greeting: "hello",
		},
		Name: "deploy",
	}
	if cmd != want {
		t.Fatalf("FillFromStruct() = %#v, want %#v", cmd, want)
	}
}

func TestFillFromStructRejectsNonEmbedded(t *testing.T) {
	structCache.Clear()

	type other struct{ X int }
	type command struct{ Name string }

	cmd := command{}
	err := FillFromStruct(&cmd, other{X: 1})
	if err == nil || !strings.Contains(err.Error(), "does not embed") {
		t.Fatalf("FillFromStruct() error = %v, want does-not-embed error", err)
	}
}

func TestFillFromMap(t *testing.T) {
	structCache.Clear()

	type config struct {
		Verbose bool
		Level   string
		Count   int
	}

	// Destination already holds values; the map names only a subset.
	cfg := config{Verbose: true, Level: "info", Count: 7}
	if err := FillFromMap(&cfg, map[string]string{
		"Level": "debug",
		"Count": "9",
	}); err != nil {
		t.Fatalf("FillFromMap() error = %v", err)
	}

	want := config{
		Verbose: true, // omitted from the map: left untouched
		Level:   "debug",
		Count:   9,
	}
	if cfg != want {
		t.Fatalf("FillFromMap() = %#v, want %#v", cfg, want)
	}
}

func TestFillFromMapDoesNotApplyDefaults(t *testing.T) {
	structCache.Clear()

	type config struct {
		Level string `default:"info"`
		Port  int    `default:"8080"`
	}

	// Port is omitted from the map but has a default tag, and WithDefaultTag is
	// enabled. The contract: FillFromMap never applies defaults, so Port keeps
	// its prior value rather than being reset to 8080.
	cfg := config{Level: "warn", Port: 3000}
	if err := FillFromMap(&cfg, map[string]string{
		"Level": "debug",
	}, WithDefaultTag("default")); err != nil {
		t.Fatalf("FillFromMap() error = %v", err)
	}

	want := config{
		Level: "debug",
		Port:  3000, // default NOT applied
	}
	if cfg != want {
		t.Fatalf("FillFromMap() = %#v, want %#v", cfg, want)
	}
}

func TestFillFromMapThroughEmbedded(t *testing.T) {
	structCache.Clear()

	type global struct {
		Verbose bool
	}
	type group struct {
		global
		Greeting string
	}
	type command struct {
		group
		Name string
	}

	cmd := command{Name: "deploy"}
	// Verbose is promoted from command.group.global; the key must write through
	// the embedded structs.
	if err := FillFromMap(&cmd, map[string]string{
		"Verbose":  "true",
		"Greeting": "hi",
	}); err != nil {
		t.Fatalf("FillFromMap() error = %v", err)
	}

	want := command{
		group: group{
			global:   global{Verbose: true},
			Greeting: "hi",
		},
		Name: "deploy",
	}
	if cmd != want {
		t.Fatalf("FillFromMap() = %#v, want %#v", cmd, want)
	}
}

func TestFillFromMapWithNameTag(t *testing.T) {
	structCache.Clear()

	type config struct {
		Name   string  `json:"name"`
		Age    int     `json:"age,omitempty"` // option after the comma is ignored
		Score  float64 `json:"score"`
		Hidden string  `json:"-"`             // excluded from matching
		Active bool                            // no tag: matched by field name
	}

	cfg := config{}
	if err := FillFromMap(&cfg, map[string]string{
		"name":   "alice",
		"age":    "42",
		"score":  "12.5",
		"Hidden": "ignored",
		"Active": "true",
	}, WithNameTag("json")); err != nil {
		t.Fatalf("FillFromMap() error = %v", err)
	}

	want := config{
		Name:   "alice",
		Age:    42,
		Score:  12.5,
		Active: true, // Hidden stays empty: json:"-" excludes it
	}
	if cfg != want {
		t.Fatalf("FillFromMap() = %#v, want %#v", cfg, want)
	}
}

func TestFillFromMapErrors(t *testing.T) {
	structCache.Clear()

	t.Run("decode failure", func(t *testing.T) {
		type sample struct {
			Age int
		}

		s := sample{}
		err := FillFromMap(&s, map[string]string{"Age": "not-a-number"})
		if err == nil || !strings.Contains(err.Error(), `failed to decode field "Age"`) {
			t.Fatalf("FillFromMap() error = %v, want decode error for Age", err)
		}
	})

	t.Run("unsupported decoder", func(t *testing.T) {
		type sample struct {
			Labels []string
		}

		s := sample{}
		err := FillFromMap(&s, map[string]string{"Labels": "a,b"})
		var target ErrDecoderUnsupportedType
		if !errors.As(err, &target) {
			t.Fatalf("FillFromMap() error = %v, want ErrDecoderUnsupportedType", err)
		}
		if target.Type != reflect.TypeFor[[]string]() {
			t.Fatalf("unsupported decoder type = %v, want []string", target.Type)
		}
	})

	t.Run("tagged fields do not fall back to field name", func(t *testing.T) {
		type sample struct {
			Name string `json:"name"`
		}

		s := sample{}
		if err := FillFromMap(&s, map[string]string{"Name": "alice"}, WithNameTag("json")); err != nil {
			t.Fatalf("FillFromMap() error = %v", err)
		}
		if s.Name != "" {
			t.Fatalf("Name = %q, want empty string", s.Name)
		}
	})

	t.Run("duplicate tagged names", func(t *testing.T) {
		type sample struct {
			First  string `opt:"name"`
			Second string `opt:"name,omitempty"`
		}

		s := sample{}
		err := FillFromMap(&s, map[string]string{"name": "alice"}, WithNameTag("opt"))
		if err == nil || !strings.Contains(err.Error(), `duplicate field name "name"`) {
			t.Fatalf("FillFromMap() error = %v, want duplicate field name error", err)
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
