package rave

import (
	"encoding"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxCustomIDRunes = 100

var (
	ErrInvalidCustomIDPattern = errors.New("invalid custom ID pattern")
	ErrInvalidCustomIDValues  = errors.New("invalid custom ID values")
	ErrCustomIDTooLong        = errors.New("custom ID exceeds 100 characters")
)

type Vars map[string]any

type customIDPattern struct {
	raw          string
	segments     []string
	placeholders map[int]string
	names        map[string]struct{}
}

func compileCustomIDPattern(pattern string) (customIDPattern, error) {
	if pattern == "" || !strings.HasPrefix(pattern, "/") {
		return customIDPattern{}, fmt.Errorf("%w: %q", ErrInvalidCustomIDPattern, pattern)
	}

	compiled := customIDPattern{
		raw:          pattern,
		segments:     strings.Split(strings.TrimPrefix(pattern, "/"), "/"),
		placeholders: make(map[int]string),
		names:        make(map[string]struct{}),
	}
	for index, segment := range compiled.segments {
		if !strings.ContainsAny(segment, "{}") {
			continue
		}
		if len(segment) < 3 || segment[0] != '{' || segment[len(segment)-1] != '}' ||
			strings.Count(segment, "{") != 1 || strings.Count(segment, "}") != 1 {
			return customIDPattern{}, fmt.Errorf("%w: %q", ErrInvalidCustomIDPattern, pattern)
		}

		name := segment[1 : len(segment)-1]
		if _, exists := compiled.names[name]; exists {
			return customIDPattern{}, fmt.Errorf("%w: %q", ErrInvalidCustomIDPattern, pattern)
		}
		compiled.placeholders[index] = name
		compiled.names[name] = struct{}{}
	}
	return compiled, nil
}

func customIDFromVars(pattern customIDPattern, values Vars) (string, error) {
	for name := range pattern.names {
		if _, ok := values[name]; !ok {
			return "", fmt.Errorf("%w: missing %q", ErrInvalidCustomIDValues, name)
		}
	}
	for name := range values {
		if _, ok := pattern.names[name]; !ok {
			return "", fmt.Errorf("%w: unknown %q", ErrInvalidCustomIDValues, name)
		}
	}

	segments := append([]string(nil), pattern.segments...)
	for index, name := range pattern.placeholders {
		encoded, err := encodeCustomIDValue(values[name])
		if err != nil {
			return "", fmt.Errorf("%w: %s: %v", ErrInvalidCustomIDValues, name, err)
		}
		if strings.Contains(encoded, "/") {
			return "", fmt.Errorf("%w: %s contains route separator", ErrInvalidCustomIDValues, name)
		}
		segments[index] = encoded
	}

	id := "/" + strings.Join(segments, "/")
	if utf8.RuneCountInString(id) > maxCustomIDRunes {
		return "", ErrCustomIDTooLong
	}
	return id, nil
}

func encodeCustomIDValue(value any) (string, error) {
	if value == nil {
		return "", errors.New("nil value")
	}

	reflected := reflect.ValueOf(value)
	if reflected.Kind() == reflect.Pointer && reflected.IsNil() {
		return "", errors.New("nil pointer")
	}
	if marshaler, ok := value.(encoding.TextMarshaler); ok {
		text, err := marshaler.MarshalText()
		if err != nil {
			return "", err
		}
		return string(text), nil
	}
	if stringer, ok := value.(fmt.Stringer); ok {
		return stringer.String(), nil
	}
	for reflected.Kind() == reflect.Pointer {
		if reflected.IsNil() {
			return "", errors.New("nil pointer")
		}
		reflected = reflected.Elem()
	}

	switch reflected.Kind() {
	case reflect.String:
		return reflected.String(), nil
	case reflect.Bool:
		return strconv.FormatBool(reflected.Bool()), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(reflected.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(reflected.Uint(), 10), nil
	case reflect.Float32, reflect.Float64:
		return strconv.FormatFloat(reflected.Float(), 'g', -1, reflected.Type().Bits()), nil
	default:
		return "", fmt.Errorf("unsupported value type %s", reflected.Type())
	}
}

func varsFromStruct(value any) (Vars, error) {
	if value == nil {
		return nil, fmt.Errorf("%w: expected struct, got nil", ErrInvalidCustomIDValues)
	}

	reflected := reflect.ValueOf(value)
	for reflected.Kind() == reflect.Pointer {
		if reflected.IsNil() {
			return nil, fmt.Errorf("%w: nil struct pointer", ErrInvalidCustomIDValues)
		}
		reflected = reflected.Elem()
	}
	if reflected.Kind() != reflect.Struct {
		return nil, fmt.Errorf("%w: expected struct, got %s", ErrInvalidCustomIDValues, reflected.Kind())
	}

	values := make(Vars)
	typeOfValue := reflected.Type()
	for index := range reflected.NumField() {
		field := typeOfValue.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name, ok := optionName(field)
		if !ok {
			continue
		}
		values[name] = reflected.Field(index).Interface()
	}
	return values, nil
}

func validateCustomIDSchema[T any](pattern customIDPattern) error {
	typeOfValue := reflect.TypeFor[T]()
	if typeOfValue == reflect.TypeFor[Vars]() {
		return nil
	}
	for typeOfValue.Kind() == reflect.Pointer {
		typeOfValue = typeOfValue.Elem()
	}
	if typeOfValue.Kind() != reflect.Struct {
		return fmt.Errorf("%w: expected struct, got %s", ErrInvalidCustomIDValues, typeOfValue)
	}

	names := make(map[string]struct{})
	for index := range typeOfValue.NumField() {
		field := typeOfValue.Field(index)
		if field.PkgPath != "" {
			continue
		}
		name, ok := optionName(field)
		if ok {
			names[name] = struct{}{}
		}
	}

	for name := range pattern.names {
		if _, ok := names[name]; !ok {
			return fmt.Errorf("%w: missing %q", ErrInvalidCustomIDValues, name)
		}
	}
	for name := range names {
		if _, ok := pattern.names[name]; !ok {
			return fmt.Errorf("%w: unknown %q", ErrInvalidCustomIDValues, name)
		}
	}
	return nil
}
