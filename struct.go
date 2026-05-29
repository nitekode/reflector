package reflector

import (
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
)

var structCache sync.Map

type structFieldInfo struct {
	// Index locates the field from the top struct. It has one element per
	// step you take to reach it. A field declared directly on the struct is
	// just [2]; a field reached by going into embedded struct 0 and then its
	// field 1 is [0, 1]. Pass it to reflect.Value.FieldByIndex to get there.
	Index []int
	Name  string
	Type  reflect.Type
	Kind  reflect.Kind
	Tags  map[string]string
	// FromEmbedded is set when this field actually lives on an embedded
	// struct and was promoted up to the top struct. It holds that embedded
	// struct's type. It is nil for fields declared on the top struct itself.
	FromEmbedded reflect.Type
	Encode       fieldEncoder
	Decode       fieldDecoder
}

// embeddedStructInfo describes one struct that is embedded inside another.
type embeddedStructInfo struct {
	Name string
	Type reflect.Type
	// Path locates the embedded struct from the top struct, the same way
	// structFieldInfo.Index locates a field.
	Path []int
}

type structInfo struct {
	Name string
	Type reflect.Type
	// Fields lists every exported value-holding field, including ones that
	// come from embedded structs (those are pulled up to this list rather
	// than nested). The embedded structs themselves are not in this list;
	// they are in EmbeddedStructs.
	Fields []*structFieldInfo
	// EmbeddedStructs lists every embedded struct, at any depth, not just the
	// ones embedded directly. Unexported embeds are included too.
	EmbeddedStructs []embeddedStructInfo
}

type structOptions struct {
	nameTag    string
	defaultTag string
}

type StructOption func(*structOptions)

func WithNameTag(tag string) StructOption {
	return func(opts *structOptions) {
		opts.nameTag = tag
	}
}

func WithDefaultTag(tag string) StructOption {
	return func(opts *structOptions) {
		opts.defaultTag = tag
	}
}

// Embeds reports whether target's type is embedded in this struct, at any
// depth. target may be a struct value, a pointer to one, or a reflect.Type.
func (si structInfo) Embeds(target any) bool {
	targetType, ok := normalizeEmbeddedStructType(target)
	if !ok {
		return false
	}

	for _, emb := range si.EmbeddedStructs {
		if emb.Type == targetType {
			return true
		}
	}

	return false
}

func InspectStruct(s any) (si structInfo, err error) {
	if s == nil {
		return si, ErrNotAStruct
	}

	typ := reflect.TypeOf(s)

	if typ.Kind() == reflect.Pointer {
		// Resolve the pointer to the underlying type
		typ = typ.Elem()
	}

	// Check if this struct has already been inspected and is in the cache
	if cached, found := structCache.Load(typ); found {
		return cached.(structInfo), nil
	}

	if typ.Kind() != reflect.Struct {
		return si, ErrNotAStruct
	}

	si.Name = typ.Name()
	si.Type = typ

	for _, field := range reflect.VisibleFields(typ) {
		// An embedded struct (or pointer to one) is recorded so we know where
		// it lives, but it is not itself a value-holding field, so skip adding
		// it to Fields.
		if field.Anonymous {
			if embType, ok := derefStructType(field.Type); ok {
				si.EmbeddedStructs = append(si.EmbeddedStructs, embeddedStructInfo{
					Name: field.Name,
					Type: embType,
					Path: slices.Clone(field.Index),
				})
				continue
			}
		}

		if !field.IsExported() {
			continue
		}

		fi := structFieldInfo{
			Index:        slices.Clone(field.Index),
			Name:         field.Name,
			Type:         field.Type,
			Kind:         field.Type.Kind(),
			Tags:         parseStructTag(string(field.Tag)),
			FromEmbedded: declaringStructType(typ, field.Index),
			Encode:       pickEncoder(field.Type),
			Decode:       pickDecoder(field.Type),
		}
		si.Fields = append(si.Fields, &fi)
	}

	// Put the inspected struct in the cache and prime it
	structCache.LoadOrStore(typ, si)

	return si, nil
}

// NewStruct returns a new value of type T with its default-tagged fields set.
// Defaults are applied only when WithDefaultTag names the tag to read; with no
// such option the result is simply the zero value. NewStruct does not read any
// input values — use FillFromMap or FillFromStruct to populate the result.
func NewStruct[T any](opts ...StructOption) (T, error) {
	var zero T

	si, err := InspectStruct(zero)
	if err != nil {
		return zero, err
	}

	fields, err := resolveStructFields(si.Fields, parseStructOptions(opts))
	if err != nil {
		return zero, err
	}

	// Set each field that carries a default tag; fields without one stay zero.
	structInst := reflect.New(si.Type).Elem()
	for _, field := range fields {
		if !field.hasDefault {
			continue
		}

		target, err := fieldByIndexAlloc(structInst, field.Index)
		if err != nil {
			return zero, fmt.Errorf("reflector: failed to address field %q: %w", field.name, err)
		}
		if err := field.Decode(target, field.defaultValue); err != nil {
			return zero, fmt.Errorf("reflector: failed to decode default for field %q with value %q: %w", field.name, field.defaultValue, err)
		}
	}

	return structInst.Interface().(T), nil
}

func ToMap(strct any, opts ...StructOption) (map[string]string, error) {
	si, err := InspectStruct(strct)
	if err != nil {
		return nil, err
	}

	structOpts := parseStructOptions(opts)
	fields, err := resolveStructFields(si.Fields, structOpts)
	if err != nil {
		return nil, err
	}

	v := reflect.ValueOf(strct)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil, ErrNotAStruct
		}
		v = v.Elem()
	}

	out := make(map[string]string, len(fields))
	for _, field := range fields {
		fv, err := v.FieldByIndexErr(field.Index)
		if err != nil {
			// The field is inside an embedded pointer that is nil, so there
			// is no value to read. Skip it.
			continue
		}
		val, err := field.Encode(fv)
		if err != nil {
			return nil, fmt.Errorf("reflector: failed to encode field %q: %w", field.name, err)
		}
		out[field.name] = val
	}

	return out, nil
}

// FillFromStruct copies values from src into dst.
//
// dst must contain src's type as an embedded struct (at any depth), or be that
// type itself. FillFromStruct finds where src belongs inside dst and copies
// src's values into that spot.
//
// Only the fields that src declares on its own are copied. Anything src itself
// got from its own embedded structs is ignored. This matters when you fill dst
// from several sources: each source only writes its own fields, so they never
// overwrite each other and the order you call FillFromStruct in does not matter.
//
// src may be a struct value or a pointer to one.
func FillFromStruct[T any](dst *T, src any) error {
	if dst == nil || src == nil {
		return ErrNotAStruct
	}

	dstInfo, err := InspectStruct(*dst)
	if err != nil {
		return err
	}

	srcType, ok := normalizeEmbeddedStructType(src)
	if !ok {
		return ErrNotAStruct
	}

	var basePath []int
	if srcType != dstInfo.Type {
		found := false
		for _, emb := range dstInfo.EmbeddedStructs {
			if emb.Type == srcType {
				basePath = emb.Path
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("reflector: %s does not embed %s", dstInfo.Name, srcType)
		}
	}

	srcInfo, err := InspectStruct(src)
	if err != nil {
		return err
	}

	srcVal := reflect.ValueOf(src)
	for srcVal.Kind() == reflect.Pointer {
		if srcVal.IsNil() {
			return ErrNotAStruct
		}
		srcVal = srcVal.Elem()
	}

	dstVal := reflect.ValueOf(dst).Elem()
	for _, field := range srcInfo.Fields {
		if field.FromEmbedded != nil {
			continue // skip fields src got from its own embeds; copy only its own
		}

		dstPath := append(slices.Clone(basePath), field.Index...)
		target, err := fieldByIndexAlloc(dstVal, dstPath)
		if err != nil {
			return fmt.Errorf("reflector: failed to address field %q: %w", field.Name, err)
		}
		target.Set(srcVal.FieldByIndex(field.Index))
	}

	return nil
}

// FillFromMap writes string values from input into the matching fields of dst.
//
// A field is matched by its name, or by its WithNameTag tag value when that
// option is given. A field is written only when input has a matching key;
// fields that input does not mention keep whatever value they already hold.
// Fields promoted from embedded structs are matched and written too.
//
// Defaults are never applied here, even if WithDefaultTag is given: FillFromMap
// writes exactly the keys present in input and nothing else. Use NewStruct when
// you want defaults filled in for the keys an input omits.
func FillFromMap[T any](dst *T, input map[string]string, opts ...StructOption) error {
	if dst == nil {
		return ErrNotAStruct
	}

	si, err := InspectStruct(*dst)
	if err != nil {
		return err
	}

	fields, err := resolveStructFields(si.Fields, parseStructOptions(opts))
	if err != nil {
		return err
	}

	// Write only the fields the input names; fields it omits keep their value.
	dstVal := reflect.ValueOf(dst).Elem()
	for _, field := range fields {
		value, found := input[field.name]
		if !found {
			continue
		}

		target, err := fieldByIndexAlloc(dstVal, field.Index)
		if err != nil {
			return fmt.Errorf("reflector: failed to address field %q: %w", field.name, err)
		}
		if err := field.Decode(target, value); err != nil {
			return fmt.Errorf("reflector: failed to decode field %q with value %q: %w", field.name, value, err)
		}
	}

	return nil
}

type resolvedStructField struct {
	*structFieldInfo
	name         string
	defaultValue string
	hasDefault   bool
}

func parseStructTag(raw string) map[string]string {
	result := make(map[string]string)

	for raw = strings.TrimSpace(raw); raw != ""; raw = strings.TrimSpace(raw) {
		colon := strings.IndexByte(raw, ':')
		if colon < 1 {
			break
		}

		key := raw[:colon]
		raw = raw[colon+1:]

		if len(raw) == 0 || raw[0] != '"' {
			break
		}

		// Find the position of the closing quote
		closingQuotePos := 1
		quoteFound := false
		for ; closingQuotePos < len(raw); closingQuotePos++ {
			if raw[closingQuotePos] == '\\' {
				closingQuotePos++ // skip escaped char
				continue
			}
			if raw[closingQuotePos] == '"' {
				quoteFound = true
				break
			}
		}

		switch quoteFound {
		case true:
			closingQuotePos += 1
		case false:
			closingQuotePos = len(raw) - 1
		}

		val, err := strconv.Unquote(raw[:closingQuotePos])
		if err != nil {
			break
		}

		result[key] = val

		raw = raw[closingQuotePos:]
	}

	return result
}

// derefStructType follows t through any pointers (e.g. **T -> T) and returns
// the struct type at the end. The bool is false if t is nil or does not end at
// a struct.
func derefStructType(t reflect.Type) (reflect.Type, bool) {
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil || t.Kind() != reflect.Struct {
		return nil, false
	}
	return t, true
}

// declaringStructType answers "which struct does this field actually belong
// to?" for a field at the given index path inside top. If the field came from
// an embedded struct, it returns that embedded struct's type. If the field is
// declared on top directly (a one-step path), it returns nil.
func declaringStructType(top reflect.Type, index []int) reflect.Type {
	if len(index) <= 1 {
		return nil
	}

	t := top
	for _, idx := range index[:len(index)-1] {
		for t.Kind() == reflect.Pointer {
			t = t.Elem()
		}
		t = t.Field(idx).Type
	}
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

// fieldByIndexAlloc walks v along the index path to reach a field, just like
// reflect.Value.FieldByIndex. The difference: if the path passes through an
// embedded pointer that is nil, the plain version panics, so here we allocate
// a new value for it first. That way the field we return can be written to.
func fieldByIndexAlloc(v reflect.Value, index []int) (reflect.Value, error) {
	for i, idx := range index {
		if i > 0 {
			for v.Kind() == reflect.Pointer {
				if v.IsNil() {
					if !v.CanSet() {
						return reflect.Value{}, fmt.Errorf("cannot allocate nil embedded pointer of type %s", v.Type())
					}
					v.Set(reflect.New(v.Type().Elem()))
				}
				v = v.Elem()
			}
		}
		v = v.Field(idx)
	}
	return v, nil
}

// normalizeEmbeddedStructType takes target in whatever form the caller passed
// it — a struct value, a pointer to one, or a reflect.Type — and returns the
// plain struct type. The bool is false if target is not (or does not point to)
// a struct.
func normalizeEmbeddedStructType(target any) (reflect.Type, bool) {
	if target == nil {
		return nil, false
	}

	typ, ok := target.(reflect.Type)
	if !ok {
		typ = reflect.TypeOf(target)
	}

	return derefStructType(typ)
}

func parseStructOptions(opts []StructOption) structOptions {
	var structOpts structOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&structOpts)
		}
	}

	return structOpts
}

func resolveStructFields(fields []*structFieldInfo, opts structOptions) ([]resolvedStructField, error) {
	resolved := make([]resolvedStructField, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))

	for _, field := range fields {
		name, ok := resolveStructFieldName(field, opts)
		if !ok {
			continue
		}
		if _, found := seen[name]; found {
			return nil, fmt.Errorf("reflector: duplicate field name %q", name)
		}
		seen[name] = struct{}{}
		resolved = append(resolved, resolvedStructField{
			structFieldInfo: field,
			name:            name,
			defaultValue:    resolveStructFieldDefault(field, opts),
			hasDefault:      hasStructFieldDefault(field, opts),
		})
	}

	return resolved, nil
}

func resolveStructFieldName(field *structFieldInfo, opts structOptions) (string, bool) {
	name := field.Name
	if opts.nameTag == "" {
		return name, true
	}

	tagValue, found := field.Tags[opts.nameTag]
	if !found {
		return name, true
	}

	if comma := strings.IndexByte(tagValue, ','); comma >= 0 {
		tagValue = tagValue[:comma]
	}

	switch tagValue {
	case "":
		return "", false
	case "-":
		return "", false
	default:
		return tagValue, true
	}
}

func resolveStructFieldDefault(field *structFieldInfo, opts structOptions) string {
	if opts.defaultTag == "" {
		return ""
	}

	tagValue, found := field.Tags[opts.defaultTag]
	if !found || tagValue == "" {
		return ""
	}

	return tagValue
}

func hasStructFieldDefault(field *structFieldInfo, opts structOptions) bool {
	if opts.defaultTag == "" {
		return false
	}

	tagValue, found := field.Tags[opts.defaultTag]
	return found && tagValue != ""
}
