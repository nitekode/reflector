package reflector

import (
	"errors"
	"net"
	"net/url"
	"reflect"
	"testing"
	"time"
)

// clearCustomConverters resets the global registries so a test starts clean and
// does not leak registrations into other tests.
func clearCustomConverters() {
	customDecoders = map[reflect.Type]fieldDecoder{}
	customEncoders = map[reflect.Type]fieldEncoder{}
}

func TestCustomEncoderDecoderRoundTrip(t *testing.T) {
	structCache.Clear()
	clearCustomConverters()

	AddDecoder(func(raw string) (time.Duration, error) {
		return time.ParseDuration(raw)
	})
	AddEncoder(func(d time.Duration) (string, error) {
		return d.String(), nil
	})

	type config struct {
		Timeout time.Duration
	}

	var cfg config
	if err := FillFromMap(&cfg, map[string]string{"Timeout": "1m30s"}); err != nil {
		t.Fatalf("FillFromMap() error = %v", err)
	}
	if cfg.Timeout != 90*time.Second {
		t.Fatalf("Timeout = %v, want %v", cfg.Timeout, 90*time.Second)
	}

	got, err := ToMap(cfg)
	if err != nil {
		t.Fatalf("ToMap() error = %v", err)
	}
	if got["Timeout"] != "1m30s" {
		t.Fatalf("ToMap()[Timeout] = %q, want %q", got["Timeout"], "1m30s")
	}
}

func TestCustomDecoderError(t *testing.T) {
	structCache.Clear()
	clearCustomConverters()

	AddDecoder(func(raw string) (time.Duration, error) {
		return time.ParseDuration(raw)
	})

	type config struct {
		Timeout time.Duration
	}

	var cfg config
	err := FillFromMap(&cfg, map[string]string{"Timeout": "not-a-duration"})
	var fieldErr *FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("FillFromMap() error = %v, want *FieldError", err)
	}
	if fieldErr.Field != "Timeout" {
		t.Fatalf("FieldError.Field = %q, want %q", fieldErr.Field, "Timeout")
	}
}

func TestUnregisteredTypeStillErrors(t *testing.T) {
	structCache.Clear()
	clearCustomConverters()

	// A []string field has a kind the built-in switches do not handle, so with
	// nothing registered for it the encode and decode paths still report the
	// type errors they did before.
	type config struct {
		Labels []string
	}

	t.Run("decode", func(t *testing.T) {
		var cfg config
		err := FillFromMap(&cfg, map[string]string{"Labels": "a,b"})
		var target DecodeTypeError
		if !errors.As(err, &target) {
			t.Fatalf("FillFromMap() error = %v, want DecodeTypeError", err)
		}
		if target.Type != reflect.TypeFor[[]string]() {
			t.Fatalf("DecodeTypeError.Type = %v, want []string", target.Type)
		}
	})

	t.Run("encode", func(t *testing.T) {
		_, err := ToMap(config{Labels: []string{"a", "b"}})
		var target EncodeTypeError
		if !errors.As(err, &target) {
			t.Fatalf("ToMap() error = %v, want EncodeTypeError", err)
		}
		if target.Type != reflect.TypeFor[[]string]() {
			t.Fatalf("EncodeTypeError.Type = %v, want []string", target.Type)
		}
	})
}

func TestCustomDecoderAppliesToDefaults(t *testing.T) {
	structCache.Clear()
	clearCustomConverters()

	AddDecoder(func(raw string) (time.Duration, error) {
		return time.ParseDuration(raw)
	})

	type config struct {
		Timeout time.Duration `default:"5s"`
	}

	got, err := NewStruct[config](WithDefaultTag("default"))
	if err != nil {
		t.Fatalf("NewStruct() error = %v", err)
	}
	if got.Timeout != 5*time.Second {
		t.Fatalf("Timeout = %v, want %v", got.Timeout, 5*time.Second)
	}
}

func TestDurationTimeURLIPRoundTrip(t *testing.T) {
	structCache.Clear()
	clearCustomConverters()

	type config struct {
		Timeout  time.Duration
		Start    time.Time
		Endpoint url.URL
		Bind     net.IP
	}

	in := map[string]string{
		"Timeout":  "1m30s",
		"Start":    "2026-05-30T12:00:00Z",
		"Endpoint": "https://example.com/path?q=1",
		"Bind":     "192.168.0.1",
	}

	var cfg config
	if err := FillFromMap(&cfg, in); err != nil {
		t.Fatalf("FillFromMap() error = %v", err)
	}

	if cfg.Timeout != 90*time.Second {
		t.Errorf("Timeout = %v, want %v", cfg.Timeout, 90*time.Second)
	}
	wantStart, _ := time.Parse(time.RFC3339, "2026-05-30T12:00:00Z")
	if !cfg.Start.Equal(wantStart) {
		t.Errorf("Start = %v, want %v", cfg.Start, wantStart)
	}
	if cfg.Endpoint.String() != "https://example.com/path?q=1" {
		t.Errorf("Endpoint = %q, want %q", cfg.Endpoint.String(), "https://example.com/path?q=1")
	}
	if !cfg.Bind.Equal(net.ParseIP("192.168.0.1")) {
		t.Errorf("Bind = %v, want %v", cfg.Bind, net.ParseIP("192.168.0.1"))
	}

	got, err := ToMap(cfg)
	if err != nil {
		t.Fatalf("ToMap() error = %v", err)
	}
	want := map[string]string{
		"Timeout":  "1m30s",
		"Start":    "2026-05-30T12:00:00Z",
		"Endpoint": "https://example.com/path?q=1",
		"Bind":     "192.168.0.1",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("ToMap()[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestDurationTimeIPDecodeErrors(t *testing.T) {
	structCache.Clear()
	clearCustomConverters()

	tests := []struct {
		name  string
		field string
		input map[string]string
	}{
		{"duration", "Timeout", map[string]string{"Timeout": "not-a-duration"}},
		{"time", "Start", map[string]string{"Start": "not-a-timestamp"}},
		{"ip", "Bind", map[string]string{"Bind": "not-an-ip"}},
	}

	type config struct {
		Timeout time.Duration
		Start   time.Time
		Bind    net.IP
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var cfg config
			err := FillFromMap(&cfg, tt.input)
			var fieldErr *FieldError
			if !errors.As(err, &fieldErr) {
				t.Fatalf("FillFromMap() error = %v, want *FieldError", err)
			}
			if fieldErr.Field != tt.field {
				t.Fatalf("FieldError.Field = %q, want %q", fieldErr.Field, tt.field)
			}
		})
	}
}

func TestCustomConverterOverridesStandardType(t *testing.T) {
	structCache.Clear()
	clearCustomConverters()

	// A non-RFC3339 layout, only reachable if the user registration wins over the
	// default time.Time handling.
	const layout = "2006-01-02"
	AddDecoder(func(raw string) (time.Time, error) {
		return time.Parse(layout, raw)
	})
	AddEncoder(func(tm time.Time) (string, error) {
		return tm.Format(layout), nil
	})

	type config struct {
		Start time.Time
	}

	var cfg config
	if err := FillFromMap(&cfg, map[string]string{"Start": "2026-05-30"}); err != nil {
		t.Fatalf("FillFromMap() error = %v", err)
	}
	want, _ := time.Parse(layout, "2026-05-30")
	if !cfg.Start.Equal(want) {
		t.Fatalf("Start = %v, want %v", cfg.Start, want)
	}

	got, err := ToMap(cfg)
	if err != nil {
		t.Fatalf("ToMap() error = %v", err)
	}
	if got["Start"] != "2026-05-30" {
		t.Fatalf("ToMap()[Start] = %q, want %q", got["Start"], "2026-05-30")
	}
}

func TestTimeFormatTag(t *testing.T) {
	structCache.Clear()
	clearCustomConverters()

	t.Run("time_format", func(t *testing.T) {
		structCache.Clear()
		type event struct {
			At time.Time `time_format:"2006-01-02"`
		}

		var e event
		if err := FillFromMap(&e, map[string]string{"At": "2026-05-30"}); err != nil {
			t.Fatalf("FillFromMap() error = %v", err)
		}
		want, _ := time.Parse("2006-01-02", "2026-05-30")
		if !e.At.Equal(want) {
			t.Fatalf("At = %v, want %v", e.At, want)
		}

		got, err := ToMap(e)
		if err != nil {
			t.Fatalf("ToMap() error = %v", err)
		}
		if got["At"] != "2026-05-30" {
			t.Fatalf("ToMap()[At] = %q, want %q", got["At"], "2026-05-30")
		}
	})

	t.Run("layout fallback", func(t *testing.T) {
		structCache.Clear()
		type event struct {
			At time.Time `layout:"2006-01-02 15:04"`
		}

		var e event
		if err := FillFromMap(&e, map[string]string{"At": "2026-05-30 14:30"}); err != nil {
			t.Fatalf("FillFromMap() error = %v", err)
		}
		got, err := ToMap(e)
		if err != nil {
			t.Fatalf("ToMap() error = %v", err)
		}
		if got["At"] != "2026-05-30 14:30" {
			t.Fatalf("ToMap()[At] = %q, want %q", got["At"], "2026-05-30 14:30")
		}
	})

	t.Run("time_format wins over layout", func(t *testing.T) {
		structCache.Clear()
		type event struct {
			At time.Time `time_format:"2006-01-02" layout:"2006-01-02 15:04"`
		}

		var e event
		if err := FillFromMap(&e, map[string]string{"At": "2026-05-30"}); err != nil {
			t.Fatalf("FillFromMap() error = %v", err)
		}
		got, err := ToMap(e)
		if err != nil {
			t.Fatalf("ToMap() error = %v", err)
		}
		if got["At"] != "2026-05-30" {
			t.Fatalf("ToMap()[At] = %q, want %q", got["At"], "2026-05-30")
		}
	})

	t.Run("no tag uses RFC 3339", func(t *testing.T) {
		structCache.Clear()
		type event struct {
			At time.Time
		}

		var e event
		if err := FillFromMap(&e, map[string]string{"At": "2026-05-30T12:00:00Z"}); err != nil {
			t.Fatalf("FillFromMap() error = %v", err)
		}
		got, err := ToMap(e)
		if err != nil {
			t.Fatalf("ToMap() error = %v", err)
		}
		if got["At"] != "2026-05-30T12:00:00Z" {
			t.Fatalf("ToMap()[At] = %q, want %q", got["At"], "2026-05-30T12:00:00Z")
		}
	})

	t.Run("two fields, different formats", func(t *testing.T) {
		structCache.Clear()
		type event struct {
			Day   time.Time `time_format:"2006-01-02"`
			Clock time.Time `time_format:"15:04"`
		}

		in := map[string]string{
			"Day":   "2026-05-30",
			"Clock": "14:30",
		}

		var e event
		if err := FillFromMap(&e, in); err != nil {
			t.Fatalf("FillFromMap() error = %v", err)
		}
		wantDay, _ := time.Parse("2006-01-02", "2026-05-30")
		if !e.Day.Equal(wantDay) {
			t.Errorf("Day = %v, want %v", e.Day, wantDay)
		}
		wantClock, _ := time.Parse("15:04", "14:30")
		if !e.Clock.Equal(wantClock) {
			t.Errorf("Clock = %v, want %v", e.Clock, wantClock)
		}

		got, err := ToMap(e)
		if err != nil {
			t.Fatalf("ToMap() error = %v", err)
		}
		for k, v := range in {
			if got[k] != v {
				t.Errorf("ToMap()[%q] = %q, want %q", k, got[k], v)
			}
		}
	})

	t.Run("bad input", func(t *testing.T) {
		structCache.Clear()
		type event struct {
			At time.Time `time_format:"2006-01-02"`
		}

		var e event
		err := FillFromMap(&e, map[string]string{"At": "30 May 2026"})
		var fieldErr *FieldError
		if !errors.As(err, &fieldErr) {
			t.Fatalf("FillFromMap() error = %v, want *FieldError", err)
		}
		if fieldErr.Field != "At" {
			t.Fatalf("FieldError.Field = %q, want %q", fieldErr.Field, "At")
		}
	})
}

func TestDurationDefault(t *testing.T) {
	structCache.Clear()
	clearCustomConverters()

	type config struct {
		Timeout time.Duration `default:"5s"`
	}

	got, err := NewStruct[config](WithDefaultTag("default"))
	if err != nil {
		t.Fatalf("NewStruct() error = %v", err)
	}
	if got.Timeout != 5*time.Second {
		t.Fatalf("Timeout = %v, want %v", got.Timeout, 5*time.Second)
	}
}
