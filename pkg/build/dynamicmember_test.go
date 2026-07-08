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
