package rave

import (
	"reflect"
	"testing"
)

func TestSetValueRejectsImpossibleAssignment(t *testing.T) {
	type declared struct{}

	var target *declared
	err := setValue(reflect.ValueOf(&target).Elem(), 1, 1)

	if err == nil {
		t.Fatal("setValue accepted an int for *declared")
	}
	if target != nil {
		t.Fatalf("setValue mutated target to %#v", target)
	}
}
