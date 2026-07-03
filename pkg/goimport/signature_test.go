package goimport

import (
	"go/types"
	"slices"
	"testing"
)

// sigOf type-checks a source package and classifies the named function, so a test
// asserts on the marshal keywords the lowerer will read without loading a real
// module.
func sigOf(t *testing.T, src, name string) FuncSig {
	t.Helper()
	pkg := checkSource(t, src)
	fn, ok := pkg.Scope().Lookup(name).(*types.Func)
	if !ok {
		t.Fatalf("no function %q in package", name)
	}
	return classifySignature(fn.Type().(*types.Signature))
}

// TestClassifyScalarSignature checks the common shape: string, boolean, and the
// numeric basics each carry their Go type keyword, and the whole signature is
// lowerable.
func TestClassifyScalarSignature(t *testing.T) {
	sig := sigOf(t, `package p
func F(s string, n int, big int64, f float64, ok bool) string { return s }
`, "F")
	if !sig.OK {
		t.Fatal("scalar signature classified as not lowerable")
	}
	if want := []string{"string", "int", "int64", "float64", "bool"}; !slices.Equal(sig.Params, want) {
		t.Errorf("params = %v, want %v", sig.Params, want)
	}
	if want := []string{"string"}; !slices.Equal(sig.Results, want) {
		t.Errorf("results = %v, want %v", sig.Results, want)
	}
}

// TestClassifyUnsignedAndByte checks the unsigned and alias numerics, since their
// result crossing (uint64 range check, byte widening) turns on the exact keyword.
func TestClassifyUnsignedAndByte(t *testing.T) {
	sig := sigOf(t, `package p
func F(b byte, r rune) uint64 { return 0 }
`, "F")
	if !sig.OK {
		t.Fatal("unsigned signature classified as not lowerable")
	}
	if want := []string{"uint8", "int32"}; !slices.Equal(sig.Params, want) {
		t.Errorf("byte and rune params = %v, want %v", sig.Params, want)
	}
	if want := []string{"uint64"}; !slices.Equal(sig.Results, want) {
		t.Errorf("results = %v, want %v", sig.Results, want)
	}
}

// TestClassifyVoidResult checks a function with no result is lowerable with an
// empty result list, the void-call shape.
func TestClassifyVoidResult(t *testing.T) {
	sig := sigOf(t, `package p
func F(n int) {}
`, "F")
	if !sig.OK {
		t.Fatal("void signature classified as not lowerable")
	}
	if len(sig.Results) != 0 {
		t.Errorf("void results = %v, want none", sig.Results)
	}
}

// TestClassifyRejectsUnsupported holds the boundary: a variadic, a slice
// parameter, a two-value result, and an error in a non-trailing position each
// clear OK so the lowerer hands the call back rather than emit an unsound crossing.
func TestClassifyRejectsUnsupported(t *testing.T) {
	cases := map[string]string{
		"variadic": `package p
func F(parts ...string) string { return "" }`,
		"slice param": `package p
func F(b []byte) int { return 0 }`,
		"two results": `package p
func F() (int, int) { return 0, 0 }`,
		"non-trailing error": `package p
func F() (error, string) { return nil, "" }`,
	}
	for name, src := range cases {
		if sig := sigOf(t, src, "F"); sig.OK {
			t.Errorf("%s: classified as lowerable, want a hand-back", name)
		}
	}
}

// TestClassifyTrailingErrorThrows proves the (T, error) idiom classifies as
// lowerable with Throws set and the error dropped from Results, and that an
// error-only result is the same shape with no value result, the two forms the
// throw bridge covers (section 6.6).
func TestClassifyTrailingErrorThrows(t *testing.T) {
	sig := sigOf(t, `package p
func F(s string) (string, error) { return s, nil }`, "F")
	if !sig.OK || !sig.Throws {
		t.Fatalf("(T, error) classified OK=%v Throws=%v, want both true", sig.OK, sig.Throws)
	}
	if len(sig.Results) != 1 || sig.Results[0] != "string" {
		t.Errorf("(string, error) results = %v, want the error dropped leaving [string]", sig.Results)
	}

	only := sigOf(t, `package p
func F() error { return nil }`, "F")
	if !only.OK || !only.Throws {
		t.Fatalf("error-only classified OK=%v Throws=%v, want both true", only.OK, only.Throws)
	}
	if len(only.Results) != 0 {
		t.Errorf("error-only results = %v, want no value result", only.Results)
	}
}

// TestClassifyNamedNumeric proves a defined type over a basic (the common
// time.Duration shape) classifies by its underlying number and records the named
// conversion, so a parameter converts to the named type on the way in and a result
// strips the brand on the way out (section 6.11).
func TestClassifyNamedNumeric(t *testing.T) {
	sig := sigOf(t, `package p
type Duration int64
func F(d Duration) Duration { return d }
`, "F")
	if !sig.OK {
		t.Fatal("named numeric classified as not lowerable, want its underlying number")
	}
	if want := []string{"int64"}; !slices.Equal(sig.Params, want) {
		t.Errorf("params = %v, want %v", sig.Params, want)
	}
	if len(sig.ParamConv) != 1 || sig.ParamConv[0].Name != "Duration" {
		t.Errorf("param conversion = %+v, want a Duration conversion", sig.ParamConv)
	}
	if want := []string{"int64"}; !slices.Equal(sig.Results, want) {
		t.Errorf("results = %v, want %v", sig.Results, want)
	}
	if !sig.ResultDefined {
		t.Error("result not marked defined, want the brand stripped on the way out")
	}
}
