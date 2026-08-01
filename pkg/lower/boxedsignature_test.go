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

// TestABoxCrossingIntoASignatureThisPassLeavesAloneHandsBack pins what the pass does not
// rewrite. A function referenced as a value has its Go type read by that reference, and
// a method's signature is read again at the vtable and at every call through an
// interface; neither is one place a rewrite can land, so a box reaching one is an honest
// hand-back rather than the Go that does not compile.
//
// A body that returns a box on one path and a struct on another is the third: a Go
// function has one result type and the two returns do not agree on it.
func TestABoxCrossingIntoASignatureThisPassLeavesAloneHandsBack(t *testing.T) {
	for name, src := range map[string]string{
		"value use": boxedSigPrelude +
			"const pick = (r: Row): number => r.id;\n" +
			"console.log(Object.values(m).map(pick));",
		"method": boxedSigPrelude +
			"class C { take(r: Row): number { return r.id; } }\n" +
			"console.log(new C().take(Object.values(m)[0]));",
		"returns disagree": boxedSigPrelude +
			"function g(b: boolean): Row { if (b) return Object.values(m)[0]; return { id: 0, tag: 'd' }; }\n" +
			"console.log(g(true).tag);",
	} {
		t.Run(name, func(t *testing.T) {
			prog := compile(t, src)
			r := NewRenderer(prog)
			_, err := r.RenderProgram(entryFile(t, prog))
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
			}
			if !strings.Contains(nyl.Reason, "coercing a dynamic value into this static type") {
				t.Errorf("hand-back reason = %q, want it to name the dynamic coercion", nyl.Reason)
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
