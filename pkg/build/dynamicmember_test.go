package build

import (
	"strings"
	"testing"
)

// TestAbsentPropertyReadLowers pins that a read of a property a fixed shape does
// not declare no longer gates the build. The checker reports "Property 'foo'
// does not exist on type '{ a: number; }'", but on a receiver that interned to a
// Go struct the property is a provable miss, so the front door tolerates the
// report and the read folds to the undefined singleton.
func TestAbsentPropertyReadLowers(t *testing.T) {
	src := "const o = { a: 1 };\nconsole.log(o.foo);\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("absent-property read should lower, got: %v", err)
	}
	if !strings.Contains(out, "value.MissingProperty") {
		t.Fatalf("expected the missing read to fold to value.MissingProperty, got:\n%s", out)
	}
}

// TestDidYouMeanPropertyReadLowers pins the same tolerance for the spelling-
// suggestion variant, "Property 'colr' does not exist ... Did you mean 'color'?",
// which is the same absent-property read the runtime answers with undefined.
func TestDidYouMeanPropertyReadLowers(t *testing.T) {
	src := "const o = { color: 1 };\nconsole.log(o.colr);\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("did-you-mean read should lower, got: %v", err)
	}
	if !strings.Contains(out, "value.MissingProperty") {
		t.Fatalf("expected the missing read to fold to value.MissingProperty, got:\n%s", out)
	}
}

// TestPresentPropertyReadUnchanged pins that guarding the missing-property path
// did not disturb a declared property: it still lowers to the Go struct field,
// not a boxed lookup.
func TestPresentPropertyReadUnchanged(t *testing.T) {
	src := "const o = { a: 1 };\nconsole.log(o.a);\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("present-property read should lower, got: %v", err)
	}
	if !strings.Contains(out, "o.A") {
		t.Fatalf("expected the declared property to read the struct field, got:\n%s", out)
	}
}

// TestSideEffectingMissingReadPreservesReceiver pins that a missing read on a
// receiver that carries an effect (a call) lowers rather than hands back: the
// receiver is lowered as the argument to value.MissingProperty, so getObj().foo
// still runs getObj exactly once and the effect is not lost.
func TestSideEffectingMissingReadPreservesReceiver(t *testing.T) {
	src := "function f() { return { a: 1 }; }\nconsole.log(f().foo);\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("missing read on a call receiver should lower, got: %v", err)
	}
	if !strings.Contains(out, "value.MissingProperty(F())") {
		t.Fatalf("expected the receiver call to be preserved as the helper's argument, got:\n%s", out)
	}
}

// TestMissingReadFlowsThroughArithmetic pins that the missing read routes as a
// dynamic operand rather than a static miss: undefined * 2 is NaN in JavaScript,
// and the read lowers through the boxed value path (value.MissingProperty) so the
// multiply runs the dynamic coercion, not a struct-field read. The receiver stays
// referenced through the helper, so the Go compiler does not flag it as unused
// even when the missing read is the binding's only use.
func TestMissingReadFlowsThroughArithmetic(t *testing.T) {
	src := "const o = { a: 1 };\nconst n: number = o.foo * 2;\nconsole.log(n);\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("missing read in arithmetic should lower, got: %v", err)
	}
	if !strings.Contains(out, "value.MissingProperty") {
		t.Fatalf("expected the missing read to route through the boxed value path, got:\n%s", out)
	}
}

// TestAbsentElementReadLowers pins that o["k"] with a string-literal key the
// fixed shape does not declare folds the same way the dotted read does. The
// checker reports it as an index error (7053) rather than a missing property, so
// the front door tolerates that code too and the read lowers to undefined instead
// of emitting a struct-field selector the shape has no field for.
func TestAbsentElementReadLowers(t *testing.T) {
	src := "const o = { a: 1 };\nconsole.log(o[\"b\"]);\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("absent element read should lower, got: %v", err)
	}
	if !strings.Contains(out, "value.MissingProperty") {
		t.Fatalf("expected the absent element read to fold to value.MissingProperty, got:\n%s", out)
	}
}

// TestPresentElementReadUnchanged pins that a declared property read through the
// bracket spelling still lowers to the Go struct field, so the presence guard did
// not disturb o["a"].
func TestPresentElementReadUnchanged(t *testing.T) {
	src := "const o = { a: 1 };\nconsole.log(o[\"a\"]);\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("present element read should lower, got: %v", err)
	}
	if !strings.Contains(out, "o.A") {
		t.Fatalf("expected the declared property to read the struct field, got:\n%s", out)
	}
}

// TestDynamicElementWriteLowers pins that a bracket write o[k] = v on a dynamic
// receiver lowers rather than handing back: the receiver has no static field to
// store into, so the write dispatches at runtime through the boxed value's SetKey,
// the mirror of the dynamic Get a read takes. A string-literal key stores a named
// property the same way the dotted write o.k = v does.
func TestDynamicElementWriteLowers(t *testing.T) {
	src := "const o: any = {};\no[\"k\"] = 1;\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("dynamic element write should lower, got: %v", err)
	}
	if !strings.Contains(out, "SetKey") {
		t.Fatalf("expected the string-key write to route through SetKey, got:\n%s", out)
	}
}

// TestDynamicNumberElementWriteLowers pins that a numeric index write on a dynamic
// receiver routes through SetIndex, the write mirror of the GetIndex a numeric read
// takes, so a[i] = v lands in an array element by the same rule the read resolves.
func TestDynamicNumberElementWriteLowers(t *testing.T) {
	src := "const a: any = [];\na[0] = 5;\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("dynamic number element write should lower, got: %v", err)
	}
	if !strings.Contains(out, "SetIndex") {
		t.Fatalf("expected the number-index write to route through SetIndex, got:\n%s", out)
	}
}

// TestDynamicComputedElementWriteLowers pins that a computed dynamic key write
// routes through SetElem, the write mirror of the GetElem a dynamic read takes, so
// the key is coerced to a property key at runtime the same way the read coerces it.
func TestDynamicComputedElementWriteLowers(t *testing.T) {
	src := "const o: any = {};\nlet k: any = \"x\";\no[k] = 7;\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("dynamic computed element write should lower, got: %v", err)
	}
	if !strings.Contains(out, "SetElem") {
		t.Fatalf("expected the computed-key write to route through SetElem, got:\n%s", out)
	}
}

// TestComputedElementReadHandsBack pins that o[k] with a computed key on a fixed
// shape hands back rather than lowering: the shape cannot prove that key absent,
// so folding to undefined would be wrong and emitting a selector would not
// compile. It waits on the struct-to-value boxer slice.
func TestComputedElementReadHandsBack(t *testing.T) {
	src := "const o = { a: 1 };\nconst k = \"a\";\nconsole.log(o[k]);\n"
	if _, err := compileSource(t, src); err == nil {
		t.Fatalf("computed element read on a fixed shape should hand back, but it lowered")
	}
}

// TestPrivateNameOutsideClassGates pins that a private-name access outside any
// class body gates the build rather than being tolerated as an absent-property
// read. The checker reports "Property '#x' does not exist on type 'void'" as a
// 2339, the same code a normal missing property draws, but a private name is a
// hard error anywhere but the class that declares it, so the front door surfaces
// it. This is the early error test262's negative parse tests demand: an AOT build
// error over the invalid source is the rejection those tests check for.
func TestPrivateNameOutsideClassGates(t *testing.T) {
	src := "var fn = function() { (() => {})().#x };\n"
	if _, err := compileSource(t, src); err == nil {
		t.Fatalf("a private name outside a class should gate the build, but it lowered")
	}
}

// TestUndeclaredPrivateNameInClassGates pins that reading a private name the class
// never declared also gates: this.#y where the class has no #y is a hard error in
// the language, spelled as the same 2339 over a #-prefixed property, so it is not
// the tolerable absent-property read that folds to undefined.
func TestUndeclaredPrivateNameInClassGates(t *testing.T) {
	src := "class C { get() { return this.#y; } }\nnew C().get();\n"
	if _, err := compileSource(t, src); err == nil {
		t.Fatalf("an undeclared private name in a class should gate the build, but it lowered")
	}
}

// TestNormalMissingPropertyStillTolerated pins the boundary: a normal missing
// property is still tolerated and folds to undefined, so gating the private-name
// miss did not disturb the ordinary absent-property read.
func TestNormalMissingPropertyStillTolerated(t *testing.T) {
	src := "const o = { a: 1 };\nconsole.log(o.b);\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("a normal missing property should still lower, got: %v", err)
	}
	if !strings.Contains(out, "value.MissingProperty") {
		t.Fatalf("expected the normal missing read to fold to value.MissingProperty, got:\n%s", out)
	}
}
