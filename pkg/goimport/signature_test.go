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

// TestClassifySliceOfScalars proves a slice of a plain basic classifies as a slice
// crossing carrying its element keyword, both as a parameter and a result, so the
// lowerer marshals a []string or []float64 element by element (section 6.4).
func TestClassifySliceOfScalars(t *testing.T) {
	sig := sigOf(t, `package p
func F(names []string, sizes []float64) []int { return nil }
`, "F")
	if !sig.OK {
		t.Fatal("slice-of-scalars signature classified as not lowerable")
	}
	if want := []string{"slice", "slice"}; !slices.Equal(sig.Params, want) {
		t.Errorf("params = %v, want %v", sig.Params, want)
	}
	if want := []string{"string", "float64"}; !slices.Equal(sig.ParamElem, want) {
		t.Errorf("param elements = %v, want %v", sig.ParamElem, want)
	}
	if want := []string{"slice"}; !slices.Equal(sig.Results, want) {
		t.Errorf("results = %v, want %v", sig.Results, want)
	}
	if want := []string{"int"}; !slices.Equal(sig.ResultElem, want) {
		t.Errorf("result elements = %v, want %v", sig.ResultElem, want)
	}
}

// TestClassifyRejectsByteSlice proves a []byte is not a slice crossing: it projects
// to a Uint8Array (section 7.3), a later slice, so it clears OK and the call hands
// back rather than marshal it as a number array.
func TestClassifyRejectsByteSlice(t *testing.T) {
	if sig := sigOf(t, `package p
func F(b []byte) int { return 0 }`, "F"); sig.OK {
		t.Error("a []byte parameter classified as lowerable, want a hand-back")
	}
	if sig := sigOf(t, `package p
func F() []byte { return nil }`, "F"); sig.OK {
		t.Error("a []byte result classified as lowerable, want a hand-back")
	}
}

// TestClassifyRejectsSliceOfComposite proves a slice whose element is not a plain
// basic (a slice of a struct, a slice of a slice) is not covered by this slice, so it
// clears OK.
func TestClassifyRejectsSliceOfComposite(t *testing.T) {
	if sig := sigOf(t, `package p
type T struct{ X int }
func F(t []T) int { return 0 }`, "F"); sig.OK {
		t.Error("a slice of a struct classified as lowerable, want a hand-back")
	}
	if sig := sigOf(t, `package p
func F(m [][]int) int { return 0 }`, "F"); sig.OK {
		t.Error("a slice of a slice classified as lowerable, want a hand-back")
	}
}

// TestClassifyOpaqueHandle proves a foreign named type the bridge does not project,
// a field-free struct or a named func type, classifies as an opaque crossing that
// carries its import path and Go name, both as a parameter and a result, so the
// lowerer holds it as a token and hands it back (section 6.13).
func TestClassifyOpaqueHandle(t *testing.T) {
	sig := sigOf(t, `package p
type Level struct{ n int }
func F(opt Level) Level { return opt }
`, "F")
	if !sig.OK {
		t.Fatal("opaque-handle signature classified as not lowerable")
	}
	if want := []string{"opaque"}; !slices.Equal(sig.Params, want) {
		t.Errorf("params = %v, want %v", sig.Params, want)
	}
	if want := []string{"opaque"}; !slices.Equal(sig.Results, want) {
		t.Errorf("results = %v, want %v", sig.Results, want)
	}
	path, name := SplitOpaqueElem(sig.ResultElem[0])
	if name != "Level" || path == "" {
		t.Errorf("result opaque element = %q.%q, want a package path and Level", path, name)
	}
	if p2, n2 := SplitOpaqueElem(sig.ParamElem[0]); p2 != path || n2 != name {
		t.Errorf("param opaque element = %q.%q, want the same %q.%q", p2, n2, path, name)
	}

	fn := sigOf(t, `package p
type Option func(n int)
func F() Option { return nil }
`, "F")
	if !fn.OK || !slices.Equal(fn.Results, []string{"opaque"}) {
		t.Errorf("named func type classified OK=%v results=%v, want an opaque result", fn.OK, fn.Results)
	}
}

// TestClassifyRejectsClassAndInterface holds the boundary between an opaque token
// and a richer projection: a struct with an exported field or an exported method is
// a class (section 6.7), a named interface is the interface projection (section 6.8),
// and an empty interface is any (section 6.12), so none of them is an opaque handle
// and each clears OK in this slice.
func TestClassifyRejectsClassAndInterface(t *testing.T) {
	cases := map[string]string{
		"exported field": `package p
type T struct{ X int }
func F() T { return T{} }`,
		"exported method": `package p
type T struct{ n int }
func (t T) M() int { return t.n }
func F() T { return T{} }`,
		"named interface": `package p
type I interface{ M() int }
func F() I { return nil }`,
		"empty interface": `package p
type Any interface{}
func F() Any { return nil }`,
	}
	for name, src := range cases {
		if sig := sigOf(t, src, "F"); sig.OK {
			t.Errorf("%s: classified as lowerable, want a hand-back", name)
		}
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
