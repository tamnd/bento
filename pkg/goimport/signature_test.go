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

// TestClassifyByteSlice proves a []byte is the whole-buffer crossing, not an
// element-by-element slice: it carries the keyword "bytes" with an empty element,
// both as a parameter and a result, so the lowerer marshals it as one Uint8Array
// through the byte bridge (section 7.3). A []uint8, the same type under its alias,
// classifies the same way.
func TestClassifyByteSlice(t *testing.T) {
	sig := sigOf(t, `package p
func F(b []byte) []byte { return b }`, "F")
	if !sig.OK {
		t.Fatal("a []byte signature classified as not lowerable, want the bytes crossing")
	}
	if want := []string{"bytes"}; !slices.Equal(sig.Params, want) {
		t.Errorf("params = %v, want %v", sig.Params, want)
	}
	if want := []string{"bytes"}; !slices.Equal(sig.Results, want) {
		t.Errorf("results = %v, want %v", sig.Results, want)
	}
	if sig.ParamElem[0] != "" || sig.ResultElem[0] != "" {
		t.Errorf("bytes crossing carried an element %q/%q, want none since the buffer crosses whole", sig.ParamElem[0], sig.ResultElem[0])
	}

	alias := sigOf(t, `package p
func F(b []uint8) int { return 0 }`, "F")
	if !alias.OK || !slices.Equal(alias.Params, []string{"bytes"}) {
		t.Errorf("[]uint8 classified OK=%v params=%v, want a bytes crossing", alias.OK, alias.Params)
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
// a class (section 6.7) and a named interface with methods is the interface
// projection (section 6.8), so neither is an opaque handle and each clears OK in this
// slice. An empty interface is not here because it is any, a covered crossing
// (section 6.12), which TestClassifyAnyInterface pins.
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
	}
	for name, src := range cases {
		if sig := sigOf(t, src, "F"); sig.OK {
			t.Errorf("%s: classified as lowerable, want a hand-back", name)
		}
	}
}

// TestClassifyAnyInterface proves a Go any (interface{}), its named empty-interface
// alias, and the literal interface{} all classify as an any crossing, both as a
// parameter and a result, so the lowerer boxes and unboxes them as dynamic bento
// values (section 6.12).
func TestClassifyAnyInterface(t *testing.T) {
	sig := sigOf(t, `package p
func F(v any) any { return v }
`, "F")
	if !sig.OK {
		t.Fatal("any signature classified as not lowerable")
	}
	if want := []string{"any"}; !slices.Equal(sig.Params, want) {
		t.Errorf("params = %v, want %v", sig.Params, want)
	}
	if want := []string{"any"}; !slices.Equal(sig.Results, want) {
		t.Errorf("results = %v, want %v", sig.Results, want)
	}

	lit := sigOf(t, `package p
func F(v interface{}) interface{} { return v }
`, "F")
	if !lit.OK || !slices.Equal(lit.Params, []string{"any"}) || !slices.Equal(lit.Results, []string{"any"}) {
		t.Errorf("interface{} classified OK=%v params=%v results=%v, want an any crossing", lit.OK, lit.Params, lit.Results)
	}

	named := sigOf(t, `package p
type Any interface{}
func F(v Any) Any { return v }
`, "F")
	if !named.OK || !slices.Equal(named.Results, []string{"any"}) {
		t.Errorf("named empty interface classified OK=%v results=%v, want an any result", named.OK, named.Results)
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

// TestClassifyRejectsUnsupported holds the boundary: a channel parameter, a
// two-value result, and an error in a non-trailing position each clear OK so the
// lowerer hands the call back rather than emit an unsound crossing.
func TestClassifyRejectsUnsupported(t *testing.T) {
	cases := map[string]string{
		"channel param": `package p
func F(c chan int) int { return 0 }`,
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

// TestClassifyVariadicScalar proves a ...T rest parameter is classified by its
// element type and flagged variadic, so the lowerer marshals each spread argument as
// one T and passes them positionally into the Go call (section 6.9). The element of a
// ...string is a plain string, and the fixed parameters ahead of it keep their own
// keywords.
func TestClassifyVariadicScalar(t *testing.T) {
	sig := sigOf(t, `package p
func F(sep string, parts ...string) string { return sep }
`, "F")
	if !sig.OK {
		t.Fatal("variadic-of-string signature classified as not lowerable")
	}
	if !sig.Variadic {
		t.Error("signature not flagged variadic, want the trailing rest parameter recognized")
	}
	if want := []string{"string", "string"}; !slices.Equal(sig.Params, want) {
		t.Errorf("params = %v, want the fixed string and the element string %v", sig.Params, want)
	}
	if want := []string{"string"}; !slices.Equal(sig.Results, want) {
		t.Errorf("results = %v, want %v", sig.Results, want)
	}
}

// TestClassifyVariadicNumericThrows proves a variadic numeric that also returns a
// trailing error keeps both crossings: the element is the number keyword, the error
// hoists to a throw, and the single non-error result rides through. This is the
// fmt.Println shape (a ...any returning (int, error)) reduced to a concrete element.
func TestClassifyVariadicNumericThrows(t *testing.T) {
	sig := sigOf(t, `package p
func F(vals ...int) (int, error) { return 0, nil }
`, "F")
	if !sig.OK || !sig.Variadic || !sig.Throws {
		t.Fatalf("variadic numeric with error classified OK=%v Variadic=%v Throws=%v, want all true", sig.OK, sig.Variadic, sig.Throws)
	}
	if want := []string{"int"}; !slices.Equal(sig.Params, want) {
		t.Errorf("params = %v, want the element int %v", sig.Params, want)
	}
	if want := []string{"int"}; !slices.Equal(sig.Results, want) {
		t.Errorf("results = %v, want the error dropped leaving [int] %v", sig.Results, want)
	}
}

// TestClassifyVariadicAny proves a ...any rest parameter classifies its element as
// the dynamic crossing, so each spread argument boxes to a bento value on the way in
// (section 6.12). This is the shape a variadic logger takes.
func TestClassifyVariadicAny(t *testing.T) {
	sig := sigOf(t, `package p
func F(args ...any) { }
`, "F")
	if !sig.OK || !sig.Variadic {
		t.Fatalf("variadic any classified OK=%v Variadic=%v, want both true", sig.OK, sig.Variadic)
	}
	if want := []string{"any"}; !slices.Equal(sig.Params, want) {
		t.Errorf("params = %v, want the element any %v", sig.Params, want)
	}
}

// TestClassifyVariadicOfComposite proves a variadic whose element is not a covered
// crossing (a slice of a struct spread as the tail) clears OK, so the call hands back
// rather than marshal an element it cannot cross.
func TestClassifyVariadicOfComposite(t *testing.T) {
	sig := sigOf(t, `package p
type T struct{ X int }
func F(items ...T) int { return 0 }
`, "F")
	if sig.OK {
		t.Error("variadic of a class struct classified as lowerable, want a hand-back")
	}
	if !sig.Variadic {
		t.Error("signature not flagged variadic even though it hands back, want the flag set for a diagnostic")
	}
}
