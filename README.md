# reflector

**Turn structs into `map[string]string` and back, fill them with defaults, and call functions with string arguments. No hand-written `reflect`.**

```go
// Build a struct with its defaults, then overlay values from anywhere.
cfg, _ := reflector.NewStruct[Config](reflector.WithDefaultTag("default"))
reflector.FillFromMap(&cfg, values, reflector.WithNameTag("env"))
```

## Features

- **One call to decode, one to encode.** `FillFromMap` turns a `map[string]string` into a struct; `ToMap` turns it back. Field names and defaults come from struct tags.
- **Embedded structs just work.** Fields promoted from embedded structs are flattened and written through to the right place, at any depth. Build one struct from the smaller pieces it embeds.
- **Call functions dynamically.** Invoke any function from a `[]any`; string arguments are decoded to the types it expects, and a trailing `error` return comes back as a normal `error`.
- **Errors name the field.** A failed decode returns a `*FieldError` with the field name and path. `errors.As` still reaches the underlying cause.
- **Cached and dependency-free.** Type inspection is memoized per type, and it uses nothing but the standard library.

## Install

```bash
go get github.com/nitekode/reflector@latest
```

## Quick Start

```go
package main

import (
	"fmt"

	"github.com/nitekode/reflector"
)

type Config struct {
	Host    string `json:"host" default:"localhost"`
	Port    int    `json:"port" default:"8080"`
	Verbose bool   `json:"verbose"`
}

func main() {
	// Start from the defaults, then overlay values (parsed from env, a file, or flags).
	cfg, _ := reflector.NewStruct[Config](reflector.WithDefaultTag("default"))
	reflector.FillFromMap(&cfg, map[string]string{
		"port":    "9000",
		"verbose": "true",
	}, reflector.WithNameTag("json"))

	fmt.Printf("%+v\n", cfg) // {Host:localhost Port:9000 Verbose:true}

	// Convert back to strings.
	out, _ := reflector.ToMap(cfg, reflector.WithNameTag("json"))
	fmt.Println(out["host"]) // localhost

	// Call a function with string arguments, decoded to its parameter types.
	sum := func(a, b int) int { return a + b }
	result, _ := reflector.Call(sum, []any{"20", "22"}, reflector.WithStringDecoding())
	fmt.Println(result[0]) // 42
}
```

## Why This Library?

`reflect` is powerful but easy to get subtly wrong: walking into embedded structs, checking whether a field can be set, allocating nil embedded pointers, parsing tags. Most helpers that smooth this over bind from `map[string]any` (mapstructure, `encoding/json`) and stop there. reflector focuses on the `map[string]string` case (config, env, form values), adds defaults and embedded flattening, and also does dynamic function calls, in one small dependency-free package.

## API Overview

### Decode strings into a struct

```go
// Construct a value with defaults read from the `default` tag.
cfg, _ := reflector.NewStruct[Config](reflector.WithDefaultTag("default"))

// Overlay string values. Only the keys you pass are written; the rest keep their value.
reflector.FillFromMap(&cfg, map[string]string{"port": "9000"}, reflector.WithNameTag("json"))
```

`WithNameTag(tag)` matches field names by that struct tag instead of the Go field
name. `WithDefaultTag(tag)` tells `NewStruct` which tag holds defaults.

### Turn a struct back into strings

```go
out, _ := reflector.ToMap(cfg, reflector.WithNameTag("json"))
// out["host"] == "localhost"
```

### Call a function dynamically

```go
add := func(a, b int) int { return a + b }

// Arguments already of the right type.
out, _ := reflector.Call(add, []any{20, 22}) // out[0] == 42

// String arguments, decoded to the parameter types.
out, _ = reflector.Call(add, []any{"20", "22"}, reflector.WithStringDecoding())
```

`Call` returns the results as plain values. If the function's last return value is
an error, `Call` returns it as the `error` and leaves it out of the results: a
`func() error` gives you an empty slice and the error, and a `func() (T, error)`
gives you `[]any{T}` and the error.

### Inspect a type

```go
si, _ := reflector.InspectStruct(App{})
si.Fields            // flattened exported fields, including ones promoted from embeds
si.Fields[0].Tags    // the field's struct tags
si.Fields[0].Index   // its index path, e.g. [0 1] for a promoted field
si.EmbeddedStructs   // every embedded struct, at any depth
si.Embeds(Logging{}) // true if Logging is embedded anywhere in App

fi, _ := reflector.InspectFunc(handler)
fi.Params     // parameter types
fi.Returns    // return types
fi.IsVariadic // whether the last parameter is variadic
fi.MinArgs    // how many arguments are required
```

Inspection is cached per type. `StructFieldInfo` and `EmbeddedStructInfo` are
exported, so you can store and pass them around.

### Handle errors

`ErrNotAStruct`, and the typed `DecodeTypeError` / `EncodeTypeError` (both carry the
offending `Type`). When `FillFromMap`, `NewStruct`, or `FillFromStruct` fail on a
specific field, they return a `*FieldError` with the field's `Name` and `Index`
path. It unwraps to the cause, so `errors.As` still reaches errors like
`DecodeTypeError`:

```go
err := reflector.FillFromMap(&cfg, values, reflector.WithNameTag("json"))
var fe *reflector.FieldError
if errors.As(err, &fe) {
	fmt.Printf("field %q is invalid: %v\n", fe.Field, fe.Err)
}
```

## Advanced Usage

### Building one struct from several pieces

Each function does one job, so filling a struct from more than one source stays predictable:

```go
type Logging struct {
	Level string `env:"LOG_LEVEL" default:"info"`
}
type Server struct {
	Logging
	Port int `env:"PORT" default:"8080"`
}
type App struct {
	Server
	Name string `env:"APP_NAME"`
}

// 1. Construct with defaults applied at every embedded level.
app, _ := reflector.NewStruct[App](reflector.WithDefaultTag("default"))

// 2. Overlay values from somewhere else, matched by the env tag.
reflector.FillFromMap(&app, map[string]string{"PORT": "9000"}, reflector.WithNameTag("env"))

// 3. Or copy values in from a separately-built piece.
reflector.FillFromStruct(&app, Logging{Level: "debug"})
```

The rules, one line each:

- **`NewStruct`** applies defaults at every depth: a `default` tag on a field inside an embedded struct is applied too, in the right place.
- **`FillFromMap`** writes only the keys you pass; fields you leave out keep their value. It never applies defaults (even with `WithDefaultTag`); that is `NewStruct`'s job.
- **`FillFromStruct`** copies only the fields the source declares itself, into wherever that struct sits inside the destination. Each source fills a different set of fields, so order never matters.

### Tag-based names

`WithNameTag("json")` switches name matching to the tag value. Options after a comma
are ignored (`json:"port,omitempty"` becomes `port`), and `json:"-"` excludes the
field. Fields without the tag fall back to their Go field name.

### Supported types

For `FillFromMap`, `NewStruct` defaults, `ToMap`, and `WithStringDecoding()`:
`string`, `bool`, `int`/`int8`/`int16`/`int32`/`int64`, `float32`/`float64`.

Unsupported types surface as `DecodeTypeError` / `EncodeTypeError`. Unexported fields
are never read or written.

## Contributing

Contributions welcome. Please open an issue first for anything beyond a small bug fix, and run `go test ./...` before submitting.

## License

MIT. See [LICENSE](LICENSE).
