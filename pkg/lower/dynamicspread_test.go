package lower

import (
	"strings"
	"testing"
)

// A dynamic call reads its arguments out of a []value.Value, so an argument list
// carrying a spread collects into that slice and goes out through Go's own variadic
// splat. The tests below pin the emitted shape rather than the runtime answer, which
// the conformance fixture covers end to end.

// TestDynamicSpreadSplicesTheSlice pins the whole point of the slice: a lone spread
// becomes the drained slice, splatted into Call, not a fixed argument list.
func TestDynamicSpreadSplicesTheSlice(t *testing.T) {
	src := "function j(a: any, b: any): void { console.log(a, b); }\nconst f: any = j;\nconst t: [string, number] = [\"x\", 2];\nf(...t);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "SpreadCallArgs") {
		t.Errorf("the spread operand was not drained by the iterator protocol:\n%s", source)
	}
	if !strings.Contains(source, "Call(value.SpreadCallArgs(") {
		t.Errorf("the drained slice was not handed straight to the call:\n%s", source)
	}
	if !strings.Contains(source, ")...)") {
		t.Errorf("the call did not use a variadic splat:\n%s", source)
	}
}

// TestDynamicCallWithoutSpreadKeepsItsFixedShape pins that the common call is
// untouched: with no spread there is no slice and no splat, just the boxed arguments
// the call always emitted.
func TestDynamicCallWithoutSpreadKeepsItsFixedShape(t *testing.T) {
	src := "function j(a: any, b: any): void { console.log(a, b); }\nconst f: any = j;\nf(\"x\", 2);\n"
	source := renderProgram(t, src)
	if strings.Contains(source, "SpreadCallArgs") {
		t.Errorf("a call with no spread drained something:\n%s", source)
	}
	if !strings.Contains(source, "Call(value.StringValue(value.FromGoString(\"x\"))") {
		t.Errorf("the fixed argument list did not survive:\n%s", source)
	}
}

// TestDynamicSpreadKeepsArgumentOrder pins that a fixed argument on either side of a
// spread lands where the source put it, which is what the append chain is for.
func TestDynamicSpreadKeepsArgumentOrder(t *testing.T) {
	src := "function j(a: any, b: any, c: any): void { console.log(a, b, c); }\nconst f: any = j;\nconst t: [string] = [\"m\"];\nf(\"lead\", ...t, \"trail\");\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "lead") || !strings.Contains(source, "trail") {
		t.Fatalf("the fixed arguments did not survive:\n%s", source)
	}
	lead := strings.Index(source, "\"lead\"")
	drain := strings.Index(source, "SpreadCallArgs")
	trail := strings.Index(source, "\"trail\"")
	if lead < 0 || drain < 0 || trail < 0 || !(lead < drain && drain < trail) {
		t.Errorf("the spliced arguments are out of source order (lead=%d drain=%d trail=%d):\n%s", lead, drain, trail, source)
	}
}

// TestDynamicSpreadEvaluatesItsOperandOnce pins the reason the operand is boxed and
// drained rather than spliced position by position. A tuple's arity is static, so its
// fields could have been read out one at a time, but that would evaluate the operand
// once per position and a spread of a call's result has to run that call exactly once.
func TestDynamicSpreadEvaluatesItsOperandOnce(t *testing.T) {
	src := "function j(a: any, b: any): void { console.log(a, b); }\nconst f: any = j;\n" +
		"function mk(): [string, number] { return [\"u\", 1]; }\nf(...mk());\n"
	source := renderProgram(t, src)
	body := source[strings.Index(source, "func main()"):]
	if n := strings.Count(body, "Mk()"); n != 1 {
		t.Errorf("the spread operand is evaluated %d times in main, want 1:\n%s", n, source)
	}
}

// TestDynamicSpreadOnAMethodCallThreadsTheReceiver pins that a spliced argument list
// still reaches CallMethod, so o.m(...xs) binds o as `this` the way o.m(x) does. The
// receiver and the key are fixed parameters ahead of the variadic, so one slice fits.
func TestDynamicSpreadOnAMethodCallThreadsTheReceiver(t *testing.T) {
	src := "const o: any = { m(a: any, b: any) { console.log(a, b); } };\nconst t: [string, number] = [\"x\", 2];\no.m(...t);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "CallMethod") {
		t.Errorf("a spliced method call did not thread the receiver:\n%s", source)
	}
	if !strings.Contains(source, "SpreadCallArgs") {
		t.Errorf("the spread operand was not drained:\n%s", source)
	}
}

// TestDynamicSpreadOfAnUnboxableOperandNamesTheType pins that the refusal names the
// element type standing in the way rather than the spread, since boxing the operand is
// the step that could not be taken and the spread itself is fine.
func TestDynamicSpreadOfAnUnboxableOperandNamesTheType(t *testing.T) {
	src := "function j(a: any, b: any): void { console.log(a, b); }\nconst f: any = j;\n" +
		"function* g(): Generator<number> { yield 1; yield 2; }\nf(...g());\n"
	reason := renderProgramHandBack(t, src)
	if strings.Contains(reason, "a spread argument in a dynamic call") {
		t.Errorf("the refusal blamed the spread rather than the box: %s", reason)
	}
	if !strings.Contains(reason, "boxing") {
		t.Errorf("the refusal did not name the box: %s", reason)
	}
}
