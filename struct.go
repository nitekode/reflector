package reflector

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

var structCache sync.Map

type structFieldInfo struct {
	Index       int
	Name        string
	Type        reflect.Type
	Kind        reflect.Kind
	IsAnonymous bool
	IsExported  bool
	Tags        map[string]string
	Encode      fieldEncoder
	Decode      fieldDecoder
}

type structInfo struct {
	Name     string
	Type     reflect.Type
	Fields   []*structFieldInfo
	Exported []*structFieldInfo
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

func (si structInfo) Embeds(target any) bool {
	targetType, ok := normalizeEmbeddedStructType(target)
	if !ok {
		return false
	}

	for _, field := range si.Fields {
		if !field.IsAnonymous {
			continue
		}

		fieldType, ok := normalizeEmbeddedStructType(field.Type)
		if !ok {
			continue
		}

		if fieldType == targetType {
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
	if si, found := structCache.Load(typ); found {
		return si.(structInfo), nil
	}

	if typ.Kind() != reflect.Struct {
		return si, ErrNotAStruct
	}

	si.Name = typ.Name()
	si.Type = typ

	si.Fields = make([]*structFieldInfo, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)

		fi := structFieldInfo{
			Index:       i,
			Name:        field.Name,
			Type:        field.Type,
			Kind:        field.Type.Kind(),
			IsAnonymous: field.Anonymous,
			IsExported:  field.IsExported(),
			Tags:        parseStructTag(string(field.Tag)),
			Encode:      pickEncoder(field.Type),
			Decode:      pickDecoder(field.Type),
		}
		si.Fields[i] = &fi

		if fi.IsExported {
			si.Exported = append(si.Exported, &fi)
		}
	}

	// Put the inspected struct in the cache and prime it
	structCache.LoadOrStore(typ, si)

	return
}

func NewStruct[T any](strct T, input map[string]string, opts ...StructOption) (T, error) {
	si, err := InspectStruct(strct)
	if err != nil {
		return strct, err
	}

	structOpts := parseStructOptions(opts)
	fields, err := resolveStructFields(si.Exported, structOpts)
	if err != nil {
		return strct, err
	}

	structInst := reflect.New(si.Type).Elem()
	for _, field := range fields {
		if value, found := input[field.name]; found {
			if err := field.Decode(structInst.Field(field.Index), value); err != nil {
				return strct, fmt.Errorf("reflector: failed to decode field %q with value %q: %w", field.name, value, err)
			}
			continue
		}

		if field.hasDefault {
			if err := field.Decode(structInst.Field(field.Index), field.defaultValue); err != nil {
				return strct, fmt.Errorf("reflector: failed to decode default for field %q with value %q: %w", field.name, field.defaultValue, err)
			}
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
	fields, err := resolveStructFields(si.Exported, structOpts)
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
		val, err := field.Encode(v.Field(field.Index))
		if err != nil {
			return nil, fmt.Errorf("reflector: failed to encode field %q: %w", field.name, err)
		}
		out[field.name] = val
	}

	return out, nil
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

func normalizeEmbeddedStructType(target any) (reflect.Type, bool) {
	if target == nil {
		return nil, false
	}

	var typ reflect.Type
	if t, ok := target.(reflect.Type); ok {
		typ = t
	} else {
		typ = reflect.TypeOf(target)
	}

	for typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}

	if typ == nil || typ.Kind() != reflect.Struct {
		return nil, false
	}

	return typ, true
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
		// Anonymous embeds are exposed for inspection via Fields/Embeds,
		// but map conversion only operates on direct named fields.
		if field.IsAnonymous {
			continue
		}

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
