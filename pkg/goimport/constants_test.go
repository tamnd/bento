package goimport

import "testing"

// constsOf type-checks a source package and returns the classified constants, so a
// test asserts on the marshal keywords the lowerer reads without loading a real
// module. It mirrors sigOf for the constant surface, running the same classifier
// Constants runs over a loaded package.
func constsOf(t *testing.T, src string) map[string]string {
	t.Helper()
	return classifyConstants(checkSource(t, src))
}

// TestConstantsClassifiesScalars checks the common shapes: a typed string and bool
// carry their keyword, and an untyped numeric constant is classified by its default
// type so a reference to it marshals as that type.
func TestConstantsClassifiesScalars(t *testing.T) {
	got := constsOf(t, `package p
const Name = "bento"
const On bool = true
const Pi = 3.14159
const Max = 1 << 20
const Big int64 = 1 << 40
`)
	want := map[string]string{
		"Name": "string",
		"On":   "bool",
		"Pi":   "float64",
		"Max":  "int",
		"Big":  "int64",
	}
	for name, kw := range want {
		if got[name] != kw {
			t.Errorf("constant %s classified as %q, want %q", name, got[name], kw)
		}
	}
}

// TestConstantsSkipsDefinedType proves a constant of a defined type over a basic
// (the time.Duration shape) is not classified as its underlying number, because it
// projects to a branded alias, not number, so its crossing differs and it is left
// for a later slice.
func TestConstantsSkipsDefinedType(t *testing.T) {
	got := constsOf(t, `package p
type Level int
const High Level = 3
const Plain = 3
`)
	if _, ok := got["High"]; ok {
		t.Errorf("a defined-type constant was classified, want a skip")
	}
	if got["Plain"] != "int" {
		t.Errorf("a plain int constant classified as %q, want int", got["Plain"])
	}
}
