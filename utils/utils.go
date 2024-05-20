package utils

import (
	"cmp"
	"math"
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
