package rave

import (
	"encoding"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCustomIDFromVars(t *testing.T) {
	pattern, err := compileCustomIDPattern("/role/assign/{roleID}")
	require.NoError(t, err)

	id, err := customIDFromVars(pattern, Vars{"roleID": uint64(42)})
	require.NoError(t, err)
	require.Equal(t, "/role/assign/42", id)
}

func TestCustomIDFromVarsRejectsMissingAndUnknownValues(t *testing.T) {
	pattern, err := compileCustomIDPattern("/report/{channel}/{message}")
	require.NoError(t, err)

	_, err = customIDFromVars(pattern, Vars{"channel": 1})
	require.ErrorIs(t, err, ErrInvalidCustomIDValues)
	require.Contains(t, err.Error(), "message")

	_, err = customIDFromVars(pattern, Vars{"channel": 1, "message": 2, "extra": 3})
	require.ErrorIs(t, err, ErrInvalidCustomIDValues)
	require.Contains(t, err.Error(), "extra")
}

func TestCompileCustomIDPatternRejectsMalformedPatterns(t *testing.T) {
	patterns := []string{
		"",
		"missing-leading-slash",
		"/empty/{}",
		"/partial/{value",
		"/partial/value}",
		"/embedded/prefix-{value}",
		"/duplicate/{value}/{value}",
	}

	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			_, err := compileCustomIDPattern(pattern)
			require.ErrorIs(t, err, ErrInvalidCustomIDPattern)
		})
	}
}

func TestCompileCustomIDPatternRejectsInvalidUTF8(t *testing.T) {
	_, err := compileCustomIDPattern("/literal/\xff")
	require.ErrorIs(t, err, ErrInvalidCustomIDPattern)
}

func TestCustomIDLengthUsesUnicodeCodePoints(t *testing.T) {
	pattern, err := compileCustomIDPattern("/id/{value}")
	require.NoError(t, err)

	_, err = customIDFromVars(pattern, Vars{"value": strings.Repeat("å", 96)})
	require.NoError(t, err)

	_, err = customIDFromVars(pattern, Vars{"value": strings.Repeat("å", 97)})
	require.True(t, errors.Is(err, ErrCustomIDTooLong))
}

type customIDStructValues struct {
	RoleID    uint64
	MaxActive int    `rave:"max-active"`
	Ignored   string `rave:"-"`
}

type duplicateCustomIDStructValues struct {
	ID    string
	Alias string `rave:"id"`
}

type schemaTextMarshalerValue struct{}

func (schemaTextMarshalerValue) MarshalText() ([]byte, error) { return []byte("text"), nil }

type schemaStringerValue struct{}

func (schemaStringerValue) String() string { return "string" }

type pointerOnlySchemaStringerValue struct{}

func (*pointerOnlySchemaStringerValue) String() string { return "pointer-string" }

type supportedCustomIDSchemaValues struct {
	TextMarshaler          schemaTextMarshalerValue
	Stringer               schemaStringerValue
	TextMarshalerInterface encoding.TextMarshaler
	StringerInterface      fmt.Stringer
	String                 string
	Bool                   bool
	Int                    int
	Int8                   int8
	Int16                  int16
	Int32                  int32
	Int64                  int64
	Uint                   uint
	Uint8                  uint8
	Uint16                 uint16
	Uint32                 uint32
	Uint64                 uint64
	Uintptr                uintptr
	Float32                float32
	Float64                float64
	Pointer                **string
	PointerStringer        *pointerOnlySchemaStringerValue
}

func TestVarsFromStructUsesRaveNamingRules(t *testing.T) {
	values, err := varsFromStruct(&customIDStructValues{RoleID: 7, MaxActive: 3, Ignored: "x"})
	require.NoError(t, err)
	require.Equal(t, Vars{"role-id": uint64(7), "max-active": 3}, values)
}

func TestValidateCustomIDSchema(t *testing.T) {
	pattern, err := compileCustomIDPattern("/button/{role-id}/{max-active}")
	require.NoError(t, err)
	require.NoError(t, validateCustomIDSchema[customIDStructValues](pattern))

	type missing struct {
		RoleID uint64
	}
	require.ErrorIs(t, validateCustomIDSchema[missing](pattern), ErrInvalidCustomIDValues)

	type extra struct {
		RoleID    uint64
		MaxActive int `rave:"max-active"`
		Other     string
	}
	require.ErrorIs(t, validateCustomIDSchema[extra](pattern), ErrInvalidCustomIDValues)
}

func TestValidateCustomIDSchemaAcceptsOnlyExactVarsType(t *testing.T) {
	pattern, err := compileCustomIDPattern("/button/{dynamic}")
	require.NoError(t, err)

	require.NoError(t, validateCustomIDSchema[Vars](pattern))
	require.ErrorIs(t, validateCustomIDSchema[*Vars](pattern), ErrInvalidCustomIDValues)
}

func TestValidateCustomIDSchemaAcceptsEncoderDispatchTypes(t *testing.T) {
	pattern, err := compileCustomIDPattern("/id/{text-marshaler}/{stringer}/{text-marshaler-interface}/{stringer-interface}/{string}/{bool}/{int}/{int8}/{int16}/{int32}/{int64}/{uint}/{uint8}/{uint16}/{uint32}/{uint64}/{uintptr}/{float32}/{float64}/{pointer}/{pointer-stringer}")
	require.NoError(t, err)

	require.NoError(t, validateCustomIDSchema[supportedCustomIDSchemaValues](pattern))
}

func TestValidateCustomIDSchemaRejectsUnsupportedFieldTypes(t *testing.T) {
	pattern, err := compileCustomIDPattern("/button/{id}")
	require.NoError(t, err)

	tests := []struct {
		name     string
		validate func() error
	}{
		{
			name: "slice",
			validate: func() error {
				type values struct{ ID []byte }
				return validateCustomIDSchema[values](pattern)
			},
		},
		{
			name: "interface",
			validate: func() error {
				type values struct{ ID any }
				return validateCustomIDSchema[values](pattern)
			},
		},
		{
			name: "struct",
			validate: func() error {
				type values struct{ ID struct{} }
				return validateCustomIDSchema[values](pattern)
			},
		},
		{
			name: "scalar with pointer-only stringer",
			validate: func() error {
				type values struct {
					ID pointerOnlySchemaStringerValue
				}
				return validateCustomIDSchema[values](pattern)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, test.validate(), ErrInvalidCustomIDValues)
		})
	}
}

func TestValidateCustomIDSchemaRejectsDuplicateResolvedNames(t *testing.T) {
	pattern, err := compileCustomIDPattern("/button/{id}")
	require.NoError(t, err)

	require.ErrorIs(t, validateCustomIDSchema[duplicateCustomIDStructValues](pattern), ErrInvalidCustomIDValues)
}

func TestVarsFromStructRejectsDuplicateResolvedNames(t *testing.T) {
	_, err := varsFromStruct(duplicateCustomIDStructValues{ID: "first", Alias: "second"})
	require.ErrorIs(t, err, ErrInvalidCustomIDValues)
}

func TestEncodeCustomIDValueRejectsNilUnsupportedAndSlash(t *testing.T) {
	pattern, err := compileCustomIDPattern("/id/{value}")
	require.NoError(t, err)

	var pointer *string
	for _, value := range []any{nil, pointer, []string{"unsupported"}, "contains/slash"} {
		_, err := customIDFromVars(pattern, Vars{"value": value})
		require.ErrorIs(t, err, ErrInvalidCustomIDValues)
	}
}

func TestCustomIDFromVarsRejectsNestedNilPointer(t *testing.T) {
	pattern, err := compileCustomIDPattern("/id/{value}")
	require.NoError(t, err)

	var inner *string
	outer := &inner
	_, err = customIDFromVars(pattern, Vars{"value": outer})
	require.ErrorIs(t, err, ErrInvalidCustomIDValues)
}

type customTextValue string

func (v customTextValue) MarshalText() ([]byte, error) { return []byte("text:" + string(v)), nil }

type customStringValue string

func (v customStringValue) String() string { return "string:" + string(v) }

type emptyCustomTextValue struct{}

func (emptyCustomTextValue) MarshalText() ([]byte, error) { return nil, nil }

type emptyCustomStringValue struct{}

func (emptyCustomStringValue) String() string { return "" }

type invalidUTF8CustomTextValue struct{}

func (invalidUTF8CustomTextValue) MarshalText() ([]byte, error) { return []byte{0xff}, nil }

type invalidUTF8CustomStringValue struct{}

func (invalidUTF8CustomStringValue) String() string { return "\xff" }

func TestCustomIDFromVarsRejectsEmptyEncodedValues(t *testing.T) {
	pattern, err := compileCustomIDPattern("/id/{value}")
	require.NoError(t, err)

	tests := []struct {
		name  string
		value any
	}{
		{name: "string", value: ""},
		{name: "text marshaler", value: emptyCustomTextValue{}},
		{name: "stringer", value: emptyCustomStringValue{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := customIDFromVars(pattern, Vars{"value": test.value})
			require.ErrorIs(t, err, ErrInvalidCustomIDValues)
		})
	}
}

func TestCustomIDFromVarsRejectsInvalidUTF8EncodedValues(t *testing.T) {
	pattern, err := compileCustomIDPattern("/id/{value}")
	require.NoError(t, err)

	tests := []struct {
		name  string
		value any
	}{
		{name: "string", value: "\xff"},
		{name: "text marshaler", value: invalidUTF8CustomTextValue{}},
		{name: "stringer", value: invalidUTF8CustomStringValue{}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := customIDFromVars(pattern, Vars{"value": test.value})
			require.ErrorIs(t, err, ErrInvalidCustomIDValues)
		})
	}
}

func TestEncodeCustomIDValueSupportsDocumentedTypes(t *testing.T) {
	pointer := "pointer"
	tests := []struct {
		value any
		want  string
	}{
		{value: "text", want: "text"},
		{value: true, want: "true"},
		{value: int64(-2), want: "-2"},
		{value: uint64(3), want: "3"},
		{value: uintptr(3), want: "3"},
		{value: float32(1.25), want: "1.25"},
		{value: float64(2.5), want: "2.5"},
		{value: customTextValue("value"), want: "text:value"},
		{value: customStringValue("value"), want: "string:value"},
		{value: &pointer, want: "pointer"},
	}

	for _, test := range tests {
		got, err := encodeCustomIDValue(test.value)
		require.NoError(t, err)
		require.Equal(t, test.want, got)
	}
}
