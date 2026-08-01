package lower

import (
	"errors"
	"strings"
	"testing"
)

const boxedSigPrelude = "type Row = { id: number; tag: string };\n" +
	"const m = JSON.parse('{}') as Record<string, Row>;\n"

// TestAParameterACallSiteBoxesTakesABoxedSlot pins the parameter half. Without it the
// declaration asked typeExpr for the Go struct Row interns to and the call site handed
// it a value.Value, which is Go that does not compile rather than a hand-back.
func TestAParameterACallSiteBoxesTakesABoxedSlot(t *testing.T) {
	const src = boxedSigPrelude +
		"function f(r: Row): number { return r.id; }\n" +
		"console.log(f(Object.values(m)[0]));"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func F(r value.Value)") {
		t.Fatalf("a parameter a call site boxes did not take a boxed slot:\n%s", out)
	}
	if !strings.Contains(out, `r.Get(value.FromGoString("id"))`) {
		t.Fatalf("a read of the boxed parameter did not dispatch at run time:\n%s", out)
	}
}

// TestAStaticArgumentBoxesIntoABoxedParameter is the half that makes one signature
// serve every call. A Go function has one parameter list, so the literal call has to
// arrive in the same slot the boxed call does, which it does by boxing on the way in.
func TestAStaticArgumentBoxesIntoABoxedParameter(t *testing.T) {
	const src = boxedSigPrelude +
		"function f(r: Row): number { return r.id; }\n" +
		"console.log(f(Object.values(m)[0]), f({ id: 7, tag: 'q' }));"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func F(r value.Value)") {
		t.Fatalf("a parameter a call site boxes did not take a boxed slot:\n%s", out)
	}
	if !strings.Contains(out, "value.NewObject()") {
		t.Fatalf("a static literal argument did not box on its way into the boxed slot:\n%s", out)
	}
}

// TestAParameterOnlyEverPassedStaticValuesKeepsItsShape is the boundary in the other
// direction. Nothing hands this function a box, so its parameter keeps the Go struct
// and no boxing rides into a call that never needed one.
func TestAParameterOnlyEverPassedStaticValuesKeepsItsShape(t *testing.T) {
	const src = boxedSigPrelude +
		"function f(r: Row): number { return r.id; }\n" +
		"console.log(f({ id: 7, tag: 'q' }));"
	out := renderProgram(t, src)
	if strings.Contains(out, "func F(r value.Value)") {
		t.Fatalf("a parameter no call site boxes took a boxed slot anyway:\n%s", out)
	}
}

// TestAFunctionWhoseReturnsAreAllBoxesAnswersOne pins the return half, and the call
// after it: the result is a box whatever shape the function declares, so the read off
// the call dispatches instead of reading a Go field the box does not have.
func TestAFunctionWhoseReturnsAreAllBoxesAnswersOne(t *testing.T) {
	const src = boxedSigPrelude +
		"function g(): Row { return Object.values(m)[0]; }\n" +
		"console.log(g().tag);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func G() value.Value") {
		t.Fatalf("a function whose returns are all boxes did not answer a box:\n%s", out)
	}
	if !strings.Contains(out, `.Get(value.FromGoString("tag"))`) {
		t.Fatalf("a read off the call did not dispatch at run time:\n%s", out)
	}
}

// TestAnArrowWhoseBodyIsABoxAnswersOne pins the return half at the const-bound form, both
// ways it renders a result: a block body from the signature, a concise body from the body
// expression. The concise form used to spell the declared struct and return a value.Value
// into it, which is Go that does not build rather than a hand-back.
func TestAnArrowWhoseBodyIsABoxAnswersOne(t *testing.T) {
	for name, src := range map[string]string{
		"concise": boxedSigPrelude +
			"const g = (): Row => Object.values(m)[0];\n" +
			"console.log(g().tag);",
		"block": boxedSigPrelude +
			"const g = (): Row => { return Object.values(m)[0]; };\n" +
			"console.log(g().tag);",
		"function expression": boxedSigPrelude +
			"const g = function (): Row { return Object.values(m)[0]; };\n" +
			"console.log(g().tag);",
	} {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, src)
			if !strings.Contains(out, "func() value.Value") {
				t.Fatalf("an arrow whose body is a box did not answer a box:\n%s", out)
			}
			if !strings.Contains(out, `.Get(value.FromGoString("tag"))`) {
				t.Fatalf("a read off the call did not dispatch at run time:\n%s", out)
			}
		})
	}
}

// TestOneReturnedBoxSettlesTheWholeFunction pins the body whose returns disagree. A Go
// function has one result type and the box is the only one of the two the other returns
// can be brought to, so the literal boxes on its way out. The ternary form is the same
// disagreement written in one expression.
func TestOneReturnedBoxSettlesTheWholeFunction(t *testing.T) {
	for name, src := range map[string]string{
		"two returns": boxedSigPrelude +
			"function g(b: boolean): Row { if (b) return Object.values(m)[0]; return { id: 0, tag: 'd' }; }\n" +
			"console.log(g(true).tag);",
		"ternary": boxedSigPrelude +
			"function g(b: boolean): Row { return b ? Object.values(m)[0] : { id: 0, tag: 'd' }; }\n" +
			"console.log(g(true).tag);",
	} {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, src)
			if !strings.Contains(out, "func G(b bool) value.Value") {
				t.Fatalf("a function that returns a box on one path did not answer a box:\n%s", out)
			}
			if !strings.Contains(out, "value.NewObject()") {
				t.Fatalf("the static return did not box on its way out:\n%s", out)
			}
		})
	}
}

// TestAMethodTakesAndAnswersABox pins the class half at both ends: the parameter a call
// site boxes takes the value slot with the literal argument boxing into it, and the body
// that hands back a box gives the method a value.Value result. A static method takes the
// same rewrite through the package function it becomes.
func TestAMethodTakesAndAnswersABox(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"parameter": {boxedSigPrelude +
			"class C { take(r: Row): number { return r.id; } }\n" +
			"const c = new C();\nconsole.log(c.take(Object.values(m)[0]), c.take({ id: 7, tag: 'q' }));",
			"Take(r value.Value)"},
		"result": {boxedSigPrelude +
			"class C { head(): Row { return Object.values(m)[0]; } }\n" +
			"console.log(new C().head().tag);",
			"Head() value.Value"},
		"this receiver": {boxedSigPrelude +
			"class C { head(): Row { return Object.values(m)[0]; } tag(): string { return this.head().tag; } }\n" +
			"console.log(new C().tag());",
			`.Get(value.FromGoString("tag"))`},
		"static parameter": {boxedSigPrelude +
			"class C { static take(r: Row): number { return r.id; } }\n" +
			"console.log(C.take(Object.values(m)[0]), C.take({ id: 7, tag: 'q' }));",
			"(r value.Value)"},
		"static result": {boxedSigPrelude +
			"class C { static head(): Row { return Object.values(m)[0]; } }\n" +
			"console.log(C.head().tag);",
			"() value.Value"},
	} {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Fatalf("a method did not take the boxed signature, want %q in:\n%s", tc.want, out)
			}
		})
	}
}

// TestAGetterAnswersABox pins the accessor. A getter has no parameters, so only the
// return half applies, and the site that has to know it is holding a box is the property
// read rather than a call. A getter of a plain type keeps its Go result, which is what
// says the rewrite is driven by the body and not by the accessor being an accessor.
func TestAGetterAnswersABox(t *testing.T) {
	for name, tc := range map[string]struct{ src, want, notWant string }{
		"instance": {boxedSigPrelude +
			"class C { get head(): Row { return Object.values(m)[0]; } }\n" +
			"console.log(new C().head.tag);",
			"Head() value.Value", ""},
		"read dispatches": {boxedSigPrelude +
			"class C { get head(): Row { return Object.values(m)[0]; } }\n" +
			"console.log(new C().head.tag);",
			`.Get(value.FromGoString("tag"))`, ""},
		"static": {boxedSigPrelude +
			"class C { static get head(): Row { return Object.values(m)[0]; } }\n" +
			"console.log(C.head.tag);",
			"() value.Value", ""},
		"plain body keeps its shape": {boxedSigPrelude +
			"class C { get count(): number { return 5; } }\n" +
			"console.log(new C().count + 1);",
			"", "Count() value.Value"},
	} {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, tc.src)
			if tc.want != "" && !strings.Contains(out, tc.want) {
				t.Fatalf("a getter did not answer a box, want %q in:\n%s", tc.want, out)
			}
			if tc.notWant != "" && strings.Contains(out, tc.notWant) {
				t.Fatalf("a getter whose body is no box answered one anyway:\n%s", out)
			}
		})
	}
}

// TestABoxCrossingIntoASignatureThisPassLeavesAloneHandsBack pins what the pass does not
// rewrite. A function referenced as a value has its Go type read by that reference, and a
// method in a hierarchy has its signature read again at the vtable and at every call
// through an interface; neither is one place a rewrite can land, so a box reaching one is
// an honest hand-back rather than the Go that does not compile.
//
// An async body is the third: what its Go func hands back is the promise rather than the
// value a return carries, so the result type is not this pass's to rewrite.
func TestABoxCrossingIntoASignatureThisPassLeavesAloneHandsBack(t *testing.T) {
	for name, tc := range map[string]struct{ src, reason string }{
		"value use": {boxedSigPrelude +
			"const pick = (r: Row): number => r.id;\n" +
			"console.log(Object.values(m).map(pick));",
			"coercing a dynamic value into this static type"},
		"override": {boxedSigPrelude +
			"class A { head(): Row { return m['a']; } }\n" +
			"class B extends A { head(): Row { return m['b']; } }\n" +
			"console.log(new B().head().tag);",
			"coercing a dynamic value into this static type"},
		"async method": {boxedSigPrelude +
			"class C { async head(): Promise<Row> { return m['a']; } }\n" +
			"new C().head().then((r) => console.log(r.tag));",
			"coercing a dynamic value into this static type"},
		"async": {boxedSigPrelude +
			"async function g(): Promise<Row> { return Object.values(m)[0]; }\n" +
			"g().then((r) => console.log(r.tag));",
			"coercing a dynamic value into this static type"},
		"answers a box as a value": {boxedSigPrelude +
			"const g = (): Row => Object.values(m)[0];\n" +
			"const fns = [g];\nconsole.log(fns[0]().tag);",
			"passes as a value"},
	} {
		t.Run(name, func(t *testing.T) {
			prog := compile(t, tc.src)
			r := NewRenderer(prog)
			_, err := r.RenderProgram(entryFile(t, prog))
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
			}
			if !strings.Contains(nyl.Reason, tc.reason) {
				t.Errorf("hand-back reason = %q, want it to name %q", nyl.Reason, tc.reason)
			}
		})
	}
}

// TestABoxReachingASignatureThroughAnotherCallIsSeen pins the fixpoint. outer's
// parameter is known to hold a box only from the top-level call, and inner's only from
// outer's body once that is settled, so a single pass over the call sites would leave
// inner reading a Go struct out of a value.Value.
func TestABoxReachingASignatureThroughAnotherCallIsSeen(t *testing.T) {
	const src = boxedSigPrelude +
		"function inner(r: Row): string { return r.tag; }\n" +
		"function outer(r: Row): string { return inner(r); }\n" +
		"console.log(outer(Object.values(m)[0]));"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func Inner(r value.Value)") || !strings.Contains(out, "func Outer(r value.Value)") {
		t.Fatalf("a box reaching a signature through another call was not seen:\n%s", out)
	}
}

// TestANameBoundToABoxIsSeenAtACallSite pins the reading of a binding's declaration. The
// pass runs before any body lowers, so there is no dynamic-locals set to ask; without
// answering from the declaration the everyday two-step (bind the box, then pass it) went
// unseen and the call handed back.
func TestANameBoundToABoxIsSeenAtACallSite(t *testing.T) {
	const src = boxedSigPrelude +
		"function f(r: Row): number { return r.id; }\n" +
		"const first = Object.values(m)[0];\n" +
		"console.log(f(first));"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func F(r value.Value)") {
		t.Fatalf("a name bound to a box was not seen at the call site:\n%s", out)
	}
}
