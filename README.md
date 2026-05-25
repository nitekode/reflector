# reflector

Small reflection helpers for Go.

`reflector` helps with three common jobs:

- inspect structs and functions
- convert structs to and from `map[string]string`
- call functions dynamically, with optional string-to-type decoding

## Installation

```bash
go get github.com/nitekode/reflector
```

## Quick Example

```go
type Common struct {
	Verbose bool
}

type Config struct {
	Common
	Name string `json:"name" default:"api"`
	Port int    `json:"port" default:"8080"`
}

si, _ := reflector.InspectStruct(Config{})
fmt.Println(si.Embeds(Common{})) // true

cfg, _ := reflector.NewStruct(Config{}, map[string]string{
	"Verbose": "true",
}, reflector.WithNameTag("json"), reflector.WithDefaultTag("default"))

values, _ := reflector.ToMap(cfg, reflector.WithNameTag("json"))
fmt.Println(values["port"]) // 8080

add := func(a, b int) int { return a + b }
out, _ := reflector.Call(add, []any{"20", "22"}, reflector.WithStringDecoding())
fmt.Println(out[0].Interface()) // 42
```

## Structs

Inspect a struct:

```go
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

si, _ := reflector.InspectStruct(User{})
fmt.Println(si.Name)                 // User
fmt.Println(si.Fields[0].Name)       // ID
fmt.Println(si.Fields[0].Tags["json"]) // id
```

Build a struct from strings:

```go
type Settings struct {
	Host   string
	Port   int
	Secure bool
}

settings, _ := reflector.NewStruct(Settings{}, map[string]string{
	"Host":   "localhost",
	"Port":   "443",
	"Secure": "true",
})
```

Convert a struct to strings:

```go
values, _ := reflector.ToMap(Settings{
	Host:   "localhost",
	Port:   443,
	Secure: true,
})
```

## Functions

Inspect a function:

```go
sum := func(base int, nums ...int) int { return base }

fi, _ := reflector.InspectFunc(sum)
fmt.Println(fi.IsVariadic) // true
fmt.Println(fi.MinArgs)    // 1
```

Call with typed inputs:

```go
add := func(a, b int) int { return a + b }
out, _ := reflector.Call(add, []any{20, 22})
```

Call with string decoding:

```go
add := func(a, b int) int { return a + b }
out, _ := reflector.Call(add, []any{"20", "22"}, reflector.WithStringDecoding())
```

## Supported Types

For `NewStruct`, `ToMap`, and `WithStringDecoding()`:

- `string`
- `bool`
- `int`, `int8`, `int16`, `int32`, `int64`
- `float32`, `float64`

## Notes

- `NewStruct` and `ToMap` use exported fields.
- `WithNameTag("json")` switches `NewStruct` and `ToMap` to tag-based names.
- `WithDefaultTag("default")` lets `NewStruct` fill missing fields from tags.
- `Embeds` checks direct embedded fields only.
- `Call` is strict by default.
