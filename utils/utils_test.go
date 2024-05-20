package utils

import (
	"testing"
	"time"
)

func TestCalcHalfLife(t *testing.T) {
	ts := time.Duration(90 * time.Hour * 24)
	hl := 90
	w := 1.0
	want := 0.5
	got := CalcHalfLife(ts, float64(hl), w)
	if got != want {
		t.Errorf("CalcHalfLife(%v, %v, %v) = %v; want %v", ts, hl, w, got, want)
	}
}
