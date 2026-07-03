package goimport

import "testing"

// constsOf type-checks a source package and returns the classified constants, so a
// test asserts on the marshal keywords the lowerer reads without loading a real
// module. It mirrors sigOf for the constant surface, running the same classifier
// Constants runs over a loaded package.
func constsOf(t *testing.T, src string) map[string]ConstInfo {
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
		if got[name].Keyword != kw {
			t.Errorf("constant %s classified as %q, want %q", name, got[name].Keyword, kw)
		}
	}
}

// TestConstantsClassifiesDefinedType proves a constant of a defined type over a
// basic (the time.Duration shape) is classified by its underlying number and
// marked Defined, so a reference to it reads the qualified Go const and strips the
// brand before marshaling as that number. A plain int stays undefined.
func TestConstantsClassifiesDefinedType(t *testing.T) {
	got := constsOf(t, `package p
type Level int
const High Level = 3
const Plain = 3
`)
	high, ok := got["High"]
	if !ok {
		t.Fatalf("a defined-type constant was not classified, want its underlying number")
	}
	if high.Keyword != "int" || !high.Defined {
		t.Errorf("defined-type constant classified as %+v, want {int true}", high)
	}
	if got["Plain"].Keyword != "int" || got["Plain"].Defined {
		t.Errorf("a plain int constant classified as %+v, want {int false}", got["Plain"])
	}
}
