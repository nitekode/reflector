package reflector

import (
	"errors"
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
