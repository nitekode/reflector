# reflector

Reflection helpers for Go — convert structs to and from `map[string]string`, copy values between structs that embed one another, and call functions with string arguments, so you never have to write `reflect` code by hand.

```go
type Server struct {
	Host string `env:"HOST" default:"localhost"`
	Port int    `env:"PORT" default:"8080"`
}

// Start from the defaults, then apply parsed values on top. The order never matters.
srv, _ := reflector.NewStruct[Server](reflector.WithDefaultTag("default"))
reflector.FillFromMap(&srv, map[string]string{"PORT": "9000"}, reflector.WithNameTag("env"))

// srv == Server{Host: "localhost", Port: 9000}
```

## Key Features

- **Struct ↔ `map[string]string`** with field names and defaults taken from struct tags.
- **Fills nested structs** — copy values into a struct from the smaller structs it embeds, however deeply nested.
- **Build then fill** — start from a struct's defaults, then layer values on top. Each step writes only what you hand it, so order doesn't matter.
- **Dynamic function calls** — call any function by value, optionally turning string arguments into the types it expects.
- **Inspection** — look at structs and functions at runtime: their fields, tags, embedded structs, parameters, and variadics.

## Installation

```bash
go get github.com/nitekode/reflector
```

## Quick Start

```go
package main

import (
	"fmt"

	"github.com/nitekode/reflector"
)

type Settings struct {
	Host   string `json:"host"`
	Port   int    `json:"port"`
	Secure bool   `json:"secure"`
}

func main() {
	// Decode strings into a struct, matching by the json tag.
	cfg := Settings{}
	reflector.FillFromMap(&cfg, map[string]string{
		"host":   "localhost",
		"port":   "443",
		"secure": "true",
	}, reflector.WithNameTag("json"))

	// And back to strings.
	values, _ := reflector.ToMap(cfg, reflector.WithNameTag("json"))
	fmt.Println(values["port"]) // 443

	// Call a function with string arguments, decoded to its parameter types.
	add := func(a, b int) int { return a + b }
	out, _ := reflector.Call(add, []any{"20", "22"}, reflector.WithStringDecoding())
	fmt.Println(out[0]) // 42
}
```

## Why This Library?

Go's `reflect` package is powerful but easy to get subtly wrong — walking into embedded structs, checking whether a field can be set, allocating nil embedded pointers, parsing tags. `reflector` handles all of that for you behind a few small functions, and sticks to one rule: *a fill writes exactly the values you give it, and defaults are only set when you first build the struct.* That keeps layering defaults, parsed values, and other structs easy to follow.

## API Overview

Grouped by use case.

### Inspection

```go
func InspectStruct(s any) (structInfo, error)
func InspectFunc(fn any) (funcInfo, error)
```

`structInfo` exposes `Fields` (a flattened list of exported fields, including ones
promoted from embedded structs, each with its `Index` path, `Tags`, and
`FromEmbedded` origin) and `EmbeddedStructs` (every embedded struct, at any depth).
`si.Embeds(T{})` reports whether `T` is embedded anywhere within.

`funcInfo` exposes `Params`, `Returns`, `IsVariadic`, and `MinArgs`.

### Build and populate structs

```go
func NewStruct[T any](opts ...StructOption) (T, error)                          // construct + defaults
func FillFromMap[T any](dst *T, input map[string]string, opts ...StructOption) error  // overlay string values
func FillFromStruct[T any](dst *T, src any) error                               // compose from another struct
func ToMap(strct any, opts ...StructOption) (map[string]string, error)          // struct -> strings
```

### Options

```go
func WithNameTag(tag string) StructOption     // match/emit field names by this struct tag
func WithDefaultTag(tag string) StructOption  // read default values from this struct tag (NewStruct only)
```

### Function calls

```go
func Call(fn any, inputs []any, opts ...CallOption) ([]any, error)
func WithStringDecoding() CallOption  // decode string inputs into parameter types
```

`Call` returns the function's results as plain values. If the function's last
return value is an error, `Call` hands it back as the `error` and leaves it out
of the results — so a `func() error` gives you an empty slice and the error, and
a `func() (T, error)` gives you `[]any{T}` and the error.

### Errors

`ErrNotAStruct`, and the typed `ErrDecoderUnsupportedType` / `ErrEncoderUnsupportedType`
(both carry the offending `Type`).

## Advanced Usage

### Building one struct from several pieces (the CLI pattern)

Each function does one job, so filling a struct from more than one source stays predictable:

```go
type Global struct {
	Verbose bool `flag:"verbose" default:"false"`
}
type Group struct {
	Global
	Greeting string `flag:"greeting" default:"hello"`
}
type Command struct {
	Group
	Name string `flag:"name"`
}

// 1. Construct with defaults applied at every embedded level.
cmd, _ := reflector.NewStruct[Command](reflector.WithDefaultTag("default"))

// 2. Overlay parsed flags, matched by the flag tag.
reflector.FillFromMap(&cmd, map[string]string{"greeting": "hi"}, reflector.WithNameTag("flag"))

// 3. Or compose from an independently-built scope struct.
reflector.FillFromStruct(&cmd, Global{Verbose: true})
```

The contract:

- **`NewStruct`** sets defaults at every level — a `default` tag on a field inside an
  embedded struct is applied too, in the right place.
- **`FillFromMap`** writes only the keys you pass; fields you leave out keep their value.
  It never sets defaults (even with `WithDefaultTag`) — that's `NewStruct`'s job.
- **`FillFromStruct`** copies only the fields the source struct declares itself, into
  wherever that struct sits inside the destination (however deeply nested). Since each
  source fills a different set of fields, you can apply several of them in any order.

### Tag-based names

`WithNameTag("json")` switches name matching to the tag value. Options after a comma
are ignored (`json:"port,omitempty"` → `port`), and `json:"-"` excludes the field.
Fields without the tag fall back to their Go field name.

### Supported types

For `FillFromMap`, `NewStruct` defaults, `ToMap`, and `WithStringDecoding()`:

- `string`
- `bool`
- `int`, `int8`, `int16`, `int32`, `int64`
- `float32`, `float64`

Unsupported types surface as `ErrDecoderUnsupportedType` / `ErrEncoderUnsupportedType`.
Unexported fields are never read or written.

## Contributing

Issues and feedback are welcome.

## License

MIT. See [LICENSE](LICENSE)
