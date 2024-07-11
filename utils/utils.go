package utils

import (
	"cmp"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func Ref[T any](v T) *T {
	return &v
}

func CalcHalfLife(timeSince time.Duration, halfLifeTimeDays, weight float64) float64 {
	if halfLifeTimeDays == 0.0 {
		return weight
	}
	return weight * (math.Pow(0.5, timeSince.Hours()/(halfLifeTimeDays*24)))
}

func Max[T cmp.Ordered](a, b T) T {
	if a > b {
		return a
	}
	return b
}

func Min[T cmp.Ordered](a, b T) T {
	if a < b {
		return a
	}
	return b
}

func WrapRef[T any](v *T) (T, bool) {
	if v == nil {
		return *new(T), false
	}
	return *v, true
}

func RefDefault[T any](v *T, def T) T {
	if v == nil {
		return def
	}
	return *v
}
func Iif[T any](cond bool, t, f T) T {
	if cond {
		return t
	}
	return f
}

func Any(vs ...bool) bool {
	for _, v := range vs {
		if v {
			return true
		}
	}
	return false
}

func All(vs ...bool) bool {
	for _, v := range vs {
		if !v {
			return false
		}
	}
	return true
}

var LongDurationRegex = regexp.MustCompile(`^(?:(?P<years>\d+)y)? *(?:(?P<months>\d+)mo)? *(?:(?P<weeks>\d+)w)? *(?:(?P<days>\d+)d)? *(?:(?P<hours>\d+)h)? *(?:(?P<minutes>\d+)m)? *(?:(?P<seconds>\d+)s)?$`)

// ParseLongDuration parses a string into a time.Duration.
// It supports the following format:
//   - 1y2mo3w4d5h6m3s (year, month, week, day, hour, minute, second)
func ParseLongDuration(s string) (time.Duration, error) {
	names := LongDurationRegex.SubexpNames()
	matches := LongDurationRegex.FindStringSubmatch(s)

	if len(matches) == 0 {
		return 0, errors.New("invalid duration")
	}

	duration := time.Duration(0)

	for i, match := range matches {
		num, err := strconv.Atoi(match)
		if err != nil {
			continue
		}
		switch names[i] {
		case "years":
			duration += time.Duration(num) * time.Hour * 24 * 365
		case "months":
			duration += time.Duration(num) * time.Hour * 24 * 30
		case "weeks":
			duration += time.Duration(num) * time.Hour * 24 * 7
		case "days":
			duration += time.Duration(num) * time.Hour * 24
		case "hours":
			duration += time.Duration(num) * time.Hour
		case "minutes":
			duration += time.Duration(num) * time.Minute
		case "seconds":
			duration += time.Duration(num) * time.Second
		}
	}

	return duration, nil
}
func FormatFloatUpToPrec(num float64, prec int) string {
	str := strconv.FormatFloat(num, 'f', prec, 64)
	str = strings.TrimRight(str, "0")
	str = strings.TrimSuffix(str, ".")

	return str
}
