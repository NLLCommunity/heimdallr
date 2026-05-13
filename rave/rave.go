package rave

import (
	"errors"
	"reflect"
	"strings"
	"unicode"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/handler"
	"github.com/disgoorg/snowflake/v2"
)

type getter func(d discord.SlashCommandInteractionData, name string) (any, bool)

var parserTypeGetters = map[reflect.Type]getter{
	reflect.TypeFor[discord.Attachment](): func(d discord.SlashCommandInteractionData, name string) (any, bool) {
		return d.OptAttachment(name)
	},
	reflect.TypeFor[discord.Member](): func(d discord.SlashCommandInteractionData, name string) (any, bool) {
		v, ok := d.OptMember(name)
		return v.Member, ok
	},
	reflect.TypeFor[discord.MentionableValue](): func(d discord.SlashCommandInteractionData, name string) (any, bool) {
		return d.OptMentionable(name)
	},
	reflect.TypeFor[discord.ResolvedChannel](): func(d discord.SlashCommandInteractionData, name string) (any, bool) {
		return d.OptChannel(name)
	},
	reflect.TypeFor[discord.ResolvedMember](): func(d discord.SlashCommandInteractionData, name string) (any, bool) {
		return d.OptMember(name)
	},
	reflect.TypeFor[discord.Role](): func(d discord.SlashCommandInteractionData, name string) (any, bool) {
		return d.OptRole(name)
	},
	reflect.TypeFor[snowflake.ID](): func(d discord.SlashCommandInteractionData, name string) (any, bool) {
		return d.OptSnowflake(name)
	},
	reflect.TypeFor[discord.User](): func(d discord.SlashCommandInteractionData, name string) (any, bool) {
		return d.OptUser(name)
	},
}

var ErrUnsupportedParseType = errors.New("type parameter must be a struct")

func ParseSlashCommandArgs[T any](e *handler.CommandEvent) (data *T, err error) {
	data = new(T)
	scid := e.SlashCommandInteractionData()

	dataType := reflect.TypeFor[T]()
	dataValue := reflect.ValueOf(data)
	dataElem := dataValue.Elem()

	if dataElem.Kind() != reflect.Struct {
		return nil, ErrUnsupportedParseType
	}

	for i := 0; i < dataElem.NumField(); i++ {
		targetField := dataType.Field(i)
		targetFieldValue := dataElem.Field(i)

		if !targetFieldValue.IsValid() || !targetFieldValue.CanSet() {
			continue
		}

		name, ok := optionName(targetField)
		if !ok {
			continue
		}

		baseType, ptrDepth := derefType(targetField.Type)

		if g, ok := parserTypeGetters[baseType]; ok {
			if v, ok := g(scid, name); ok {
				setValue(targetFieldValue, v, ptrDepth)
			}
			continue
		}

		switch baseType.Kind() {
		case reflect.Bool:
			if v, ok := scid.OptBool(name); ok {
				setValue(targetFieldValue, v, ptrDepth)
			}
		case reflect.Float32, reflect.Float64:
			if v, ok := scid.OptFloat(name); ok {
				setFloat(targetFieldValue, v, ptrDepth)
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			if v, ok := scid.OptInt(name); ok {
				setInt(targetFieldValue, int64(v), ptrDepth)
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if v, ok := scid.OptInt(name); ok {
				setUint(targetFieldValue, uint64(v), ptrDepth)
			}
		case reflect.String:
			if v, ok := scid.OptString(name); ok {
				setValue(targetFieldValue, v, ptrDepth)
			}
		default:
			// none
		}
	}

	// scid.Attachment("a") // => discord.Attachment
	// scid.Bool("a")  // => bool
	// scid.Channel("a")  // => discord.ResolvedChannel
	// scid.Float("a")  // => float64
	// scid.Int("a")  // => int
	// scid.Member("a")  // => discord.ResolvedMember
	// scid.Mentionable("a")  // => discord.MentionableValue
	// scid.Role("a")  // => discord.Role
	// scid.Snowflake("a")  // => snowflake.ID
	// scid.String("a")  // => string

	return
}

// optionName returns the discord option name to look up for a struct field,
// and false if the field should be skipped. `rave:"-"` skips; `rave:"x"`
// renames to "x"; absent/empty derives a kebab-case name from the field name
// (e.g. TargetUser -> target-user, SnowflakeIDNow -> snowflake-id-now).
func optionName(f reflect.StructField) (string, bool) {
	tag, ok := f.Tag.Lookup("rave")
	if ok && tag == "-" {
		return "", false
	}
	if ok && tag != "" {
		return tag, true
	}
	return pascalToKebab(f.Name), true
}

// pascalToKebab converts a PascalCase/camelCase identifier to kebab-case,
// preserving acronym boundaries: a hyphen is inserted before an uppercase
// rune when the previous rune is lowercase, or when the previous rune is
// uppercase and the next rune is lowercase (the end of an acronym).
func pascalToKebab(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			atAcronymEnd := unicode.IsUpper(prev) && i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if unicode.IsLower(prev) || atAcronymEnd {
				b.WriteByte('-')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func derefType(t reflect.Type) (baseType reflect.Type, ptrDepth int) {
	baseType = t
	for baseType.Kind() == reflect.Pointer {
		baseType = baseType.Elem()
		ptrDepth++
	}
	return
}

func setValue(field reflect.Value, v any, ptrDepth int) {
	value := reflect.ValueOf(v)

	for range ptrDepth {
		newPtr := reflect.New(value.Type())
		newPtr.Elem().Set(value)
		value = newPtr
	}

	if value.Type().AssignableTo(field.Type()) {
		field.Set(value)
		return
	}

	if value.Type().ConvertibleTo(field.Type()) {
		field.Set(value.Convert(field.Type()))
		return
	}
}

func setInt(field reflect.Value, vi int64, ptrDepth int) {
	target := field
	if ptrDepth > 0 {
		// Build pointer chain to the base kind
		base := field.Type()
		for range ptrDepth {
			base = base.Elem()
		}
		p := reflect.New(base)
		target = p.Elem()
		defer field.Set(p)
	}
	if target.OverflowInt(vi) {
		return
	}
	target.SetInt(vi)
}

func setUint(field reflect.Value, vu uint64, ptrDepth int) {
	target := field
	if ptrDepth > 0 {
		base := field.Type()
		for range ptrDepth {
			base = base.Elem()
		}
		p := reflect.New(base)
		target = p.Elem()
		defer field.Set(p)
	}
	if target.OverflowUint(vu) {
		return
	}
	target.SetUint(vu)
}

func setFloat(field reflect.Value, vf float64, ptrDepth int) {
	target := field
	if ptrDepth > 0 {
		base := field.Type()
		for range ptrDepth {
			base = base.Elem()
		}
		p := reflect.New(base)
		target = p.Elem()
		defer field.Set(p)
	}

	switch target.Kind() {
	case reflect.Float32:
		target.SetFloat(float64(float32(vf)))
	case reflect.Float64:
		target.SetFloat(vf)
	}
}
