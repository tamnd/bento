package lower

import (
	"strings"
	"testing"
)

// TestClassAsValueLowersToStaticSingleton pins that a bare class reference used as
// a value, the class object itself rather than a construction or a static-member
// read, lowers to a static-side singleton distinct from the *C instance pointer.
// The arrow that returns the class returns the static struct type, not the
// instance pointer the plain type path would emit, and the class emits the
// static-side type and its var only because it is used this way.
func TestClassAsValueLowersToStaticSingleton(t *testing.T) {
	src := `class _this {
}
var f = () => _this;
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "type _thisClass struct") {
		t.Fatalf("static-side type was not emitted:\n%s", out)
	}
	if !strings.Contains(out, "var _thisClassValue = _thisClass{}") {
		t.Fatalf("static-side singleton was not emitted:\n%s", out)
	}
	if !strings.Contains(out, "func() _thisClass {") {
		t.Fatalf("arrow return type was not the static side:\n%s", out)
	}
	if !strings.Contains(out, "return _thisClassValue") {
		t.Fatalf("class value did not lower to the singleton:\n%s", out)
	}
}

// TestClassNotUsedAsValueEmitsNoStaticSide pins that a class only constructed or
// read for a static member emits neither the static-side type nor its var, so the
// representation is added only where a class is genuinely used as a value.
func TestClassNotUsedAsValueEmitsNoStaticSide(t *testing.T) {
	src := `class A {
    static n = 1;
}
let a = new A();
console.log(A.n);
`
	out := renderProgram(t, src)
	if strings.Contains(out, "AClass") {
		t.Fatalf("static-side representation leaked into a class never used as a value:\n%s", out)
	}
}

// TestClassAsValueRuns builds and runs the shape to prove the class value flows
// into a slot and the program completes with no output, the observable behavior
// of a class reference that is assigned and never constructed through.
func TestClassAsValueRuns(t *testing.T) {
	skipIfShort(t)
	src := `class _this {
}
var f = () => _this;
`
	out := renderProgram(t, src)
	if got := goRunSource(t, out); got != "" {
		t.Fatalf("class-as-value run mismatch:\n got %q\nwant %q", got, "")
	}
}
