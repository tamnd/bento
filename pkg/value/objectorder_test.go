package value

import (
	"strings"
	"testing"
)

// Own string keys enumerate in the spec's order: integer indices ascending first,
// then the remaining string keys in insertion order, regardless of how the keys
// were added.
func TestOrderedStringKeys(t *testing.T) {
	o := NewObject()
	for _, k := range []string{"b", "2", "a", "1", "10"} {
		o.SetKey(FromGoString(k), Undefined)
	}
	keys := o.object().orderedStringKeys()
	var got []string
	for _, k := range keys {
		got = append(got, k.ToGoString())
	}
	want := []string{"1", "2", "10", "b", "a"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("orderedStringKeys = %v, want %v", got, want)
	}
}

// An array's dense indices enumerate ahead of any named string key it carries.
func TestOrderedStringKeysArray(t *testing.T) {
	a := NewArrayValue([]Value{Number(10), Number(20), Number(30)})
	a.SetKey(FromGoString("tag"), StringValue(FromGoString("x")))
	keys := a.object().orderedStringKeys()
	var got []string
	for _, k := range keys {
		got = append(got, k.ToGoString())
	}
	want := []string{"0", "1", "2", "tag"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("array orderedStringKeys = %v, want %v", got, want)
	}
}

// The rest object copies properties in the same deterministic order, so a
// downstream enumeration of the rest matches the source.
func TestObjectRestPreservesOrder(t *testing.T) {
	o := NewObject()
	for _, k := range []string{"b", "2", "1", "a"} {
		o.SetKey(FromGoString(k), Number(0))
	}
	rest := o.ObjectRest(FromGoString("b"))
	var got []string
	for _, k := range rest.object().orderedStringKeys() {
		got = append(got, k.ToGoString())
	}
	want := []string{"1", "2", "a"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("rest order = %v, want %v", got, want)
	}
}
