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

func NewStruct[T any](strct T, input map[string]string) (T, error) {
	si, err := InspectStruct(strct)
	if err != nil {
		return strct, err
	}

	structInst := reflect.New(si.Type).Elem()
	for _, field := range si.Exported {
		if value, found := input[field.Name]; found {
			if err := field.Decode(structInst.Field(field.Index), value); err != nil {
				return strct, fmt.Errorf("reflector: failed to decode field %q with value %q: %w", field.Name, value, err)
			}
		}
	}

	return structInst.Interface().(T), nil
}

func ToMap(strct any) (map[string]string, error) {
	si, err := InspectStruct(strct)
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

	out := make(map[string]string, len(si.Exported))
	for _, field := range si.Exported {
		val, err := field.Encode(v.Field(field.Index))
		if err != nil {
			return nil, fmt.Errorf("reflector: failed to encode field %q: %w", field.Name, err)
		}
		out[field.Name] = val
	}

	return out, nil
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
