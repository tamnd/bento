package lower

import (
	"strings"
	"testing"
)

// TestContextualUnionCallSigArrowReturnsUnion pins that a concise arrow assigned to a
// union of interfaces whose call signatures agree on parameters but differ on return
// types returns the union of those returns, so a sibling assignment returning the
// other arm fits the same variable. Without it the arrow returned its body's own type
// (value.BStr) and the later number-returning assignment did not build against it.
func TestContextualUnionCallSigArrowReturnsUnion(t *testing.T) {
	src := `interface B { (a: number): string; }
interface C { (a: number): number; }
{
    var f: B | C = a => a.toString();
    f = a => a;
}
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "func(a float64) NumOrStr") {
		t.Fatalf("union-call-signature arrow did not adopt the union return type:\n%s", out)
	}
	if !strings.Contains(out, "return NumOrStrOfStr(") || !strings.Contains(out, "return NumOrStrOfNum(a)") {
		t.Fatalf("arrow bodies were not coerced into the union arms:\n%s", out)
	}
}

// TestContextualUnionCallSigMismatchedParamsUntouched guards the narrowing: a union
// whose members' call signatures differ on parameters yields a signature with a never
// parameter (the collapsed intersection), which the arrow was not typed at, so the
// arrow stays the any-parameter, any-return form TypeScript gives it rather than
// wrongly adopting the slot's return.
func TestContextualUnionCallSigMismatchedParamsUntouched(t *testing.T) {
	src := `interface B { (a: number): string; }
interface D { (b: string): number; }
{
    var g: B | D = (a: number) => a.toString();
}
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "func(a float64) value.BStr") {
		t.Fatalf("mismatched-parameter union arrow was not left as its body-typed form:\n%s", out)
	}
	if !strings.Contains(out, "return value.NumberToString(a)") {
		t.Fatalf("mismatched-parameter union arrow body was wrongly coerced into a union arm:\n%s", out)
	}
}
