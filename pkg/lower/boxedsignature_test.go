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

// TestAFieldAStoreBoxesTakesTheValueSlot pins the field half. The struct field is the one
// place the Go type is written, so a store that hands it a box settles it for every other
// store, and the reads that follow go through the value model rather than a Go field the
// struct no longer has.
func TestAFieldAStoreBoxesTakesTheValueSlot(t *testing.T) {
	const src = boxedSigPrelude +
		"class S { first: Row = Object.values(m)[0]; tag(): string { return this.first.tag; } }\n" +
		"console.log(new S().tag());"
	out := renderProgram(t, src)
	if !strings.Contains(out, "First value.Value `json:\"first\"`") {
		t.Fatalf("a field a store boxes did not take the value slot:\n%s", out)
	}
	if !strings.Contains(out, `s.First.Get(value.FromGoString("tag"))`) {
		t.Fatalf("a read of the boxed field did not dispatch at run time:\n%s", out)
	}
}

// TestAStoreFromASiblingMethodBoxesTheField pins the receiver the pass has to supply
// itself. `this` has no class in hand while the pass is deciding, so without walking the
// program with the declaring class set, neither the receiver of the store nor the sibling
// call it stores was recognizable, and the field kept a Go type the store could not fill.
func TestAStoreFromASiblingMethodBoxesTheField(t *testing.T) {
	const src = boxedSigPrelude +
		"class S { last: Row = { id: 0, tag: 'z' }; head(): Row { return m['a']; } keep(): void { this.last = this.head(); } }\n" +
		"const s = new S();\ns.keep();\nconsole.log(s.last.tag);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "Last value.Value `json:\"last\"`") {
		t.Fatalf("a store from a sibling method did not box the field:\n%s", out)
	}
}

// TestAMethodHandingBackABoxedFieldAnswersABox is the same receiver read at the result.
// The return half decides from the body, so `return this.first` is only a box once the
// field it names is known to hold one.
func TestAMethodHandingBackABoxedFieldAnswersABox(t *testing.T) {
	const src = boxedSigPrelude +
		"class S { first: Row = m['a']; get(): Row { return this.first; } }\n" +
		"console.log(new S().get().tag);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func (s *S) Get() value.Value") {
		t.Fatalf("a method handing back a boxed field did not answer a box:\n%s", out)
	}
}

// TestAConstructorParameterACallSiteBoxesTakesABoxedSlot pins the half that faces the
// call site. A constructor's whole job is moving its arguments into fields, so the field
// rewrite is worth nothing until new S(box) has somewhere to put what it is handed, and
// the literal call site agrees with it by boxing on the way in.
func TestAConstructorParameterACallSiteBoxesTakesABoxedSlot(t *testing.T) {
	const src = boxedSigPrelude +
		"class S { r: Row; constructor(r: Row) { this.r = r; } }\n" +
		"console.log(new S(Object.values(m)[0]).r.tag, new S({ id: 7, tag: 'q' }).r.tag);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func NewS(r value.Value) *S") {
		t.Fatalf("a constructor parameter a call site boxes did not take a boxed slot:\n%s", out)
	}
	if !strings.Contains(out, "value.NewObject()") {
		t.Fatalf("a static literal argument did not box on its way into the constructor:\n%s", out)
	}
}

// TestAFieldInABaseTakesTheValueSlot lifts note 389's boundary. A method stops at a
// hierarchy because its Go signature is written again at the vtable and at the interface;
// a field is written into the struct of the class that declares it and nowhere else, so
// the one-place condition still holds and the derived struct reaches it through Go's own
// promotion.
func TestAFieldInABaseTakesTheValueSlot(t *testing.T) {
	const src = boxedSigPrelude +
		"class B { first: Row = Object.values(m)[0]; }\n" +
		"class C extends B { extra = 2; }\n" +
		"console.log(new C().first.tag, new C().extra);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "First value.Value `json:\"first\"`") {
		t.Fatalf("a field in a base did not take the value slot:\n%s", out)
	}
	if strings.Count(out, "First value.Value") != 1 {
		t.Fatalf("the boxed field was written into more than the declaring struct:\n%s", out)
	}
}

// TestAStoreInADerivedClassBoxesTheBaseField pins the receiver walking the chain. The
// store names the derived class, the field belongs to the base, and Go promotion is what
// joins them, so the pass has to match a receiver by what its property resolves to rather
// than by the class itself.
func TestAStoreInADerivedClassBoxesTheBaseField(t *testing.T) {
	const src = boxedSigPrelude +
		"class B { cur: Row = { id: 0, tag: 'z' }; }\n" +
		"class C extends B { load(k: string): void { this.cur = m[k]; } }\n" +
		"const c = new C();\nc.load('b');\nconsole.log(c.cur.tag);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "Cur value.Value `json:\"cur\"`") {
		t.Fatalf("a store in a derived class did not box the base field:\n%s", out)
	}
}

// TestASuperCallCarriesABoxToTheBaseConstructor pins the one call site a derived class
// makes that no other shape covers. The derived constructor hands its own parameter
// straight on, and only the base's declaration says what the field it fills holds, so
// without reading super(...) as a call into the base the chain stopped one class short.
func TestASuperCallCarriesABoxToTheBaseConstructor(t *testing.T) {
	const src = boxedSigPrelude +
		"class A { r: Row; constructor(r: Row) { this.r = r; } }\n" +
		"class B extends A { n: number; constructor(r: Row, n: number) { super(r); this.n = n; } }\n" +
		"console.log(new B(Object.values(m)[0], 4).r.tag);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func NewA(r value.Value) *A") {
		t.Fatalf("a super call did not carry the box to the base constructor:\n%s", out)
	}
	if !strings.Contains(out, "func NewB(r value.Value, n float64) *B") {
		t.Fatalf("the derived constructor did not take the boxed slot:\n%s", out)
	}
}

// TestACallbackParameterPassedOnBoxesTheCallee is the pre-pass half of the callback
// slice. Note 383 already gave an inline callback's non-primitive parameter a value.Value
// slot, but only at lowering time, so the whole-program pass never saw the box and a
// function the callback passed that parameter to still asked for the struct. Marking the
// parameter's symbol in the pre-pass is what carries the box across the call.
func TestACallbackParameterPassedOnBoxesTheCallee(t *testing.T) {
	const src = boxedSigPrelude +
		"function label(r: Row): string { return r.tag; }\n" +
		"console.log(Object.values(m).map((r: Row) => label(r)).join(','));"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func Label(r value.Value)") {
		t.Fatalf("a function a callback parameter is passed to did not take a boxed slot:\n%s", out)
	}
}

// TestACallbackAnsweringABoxTakesAValueResult is the result half. Once the parameter
// holds a box every expression the body builds from it is a box too, so the callback's
// own Go result has to be the box rather than the shape the checker read off the body.
func TestACallbackAnsweringABoxTakesAValueResult(t *testing.T) {
	const src = boxedSigPrelude +
		"function label(r: Row): string { return r.tag; }\n" +
		"console.log(Object.values(m).map((r: Row) => [r].map((q: Row) => label(q)).join('')).join(','));"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func(r value.Value) value.Value {") {
		t.Fatalf("a callback answering a box kept the checker's string result:\n%s", out)
	}
}

// TestAnInlineCallbackIsNotClaimedAsTheBindingsFunction pins a mis-claim the pass used to
// make. It looked for the function a declaration binds by finding the first function-like
// node anywhere under it, so `const out = xs.map((r) => ...)` handed it the inline
// callback and decided that callback by how the binding `out` is used, which here is a
// string. The function a name binds is the initializer itself and nothing nested in it.
func TestAnInlineCallbackIsNotClaimedAsTheBindingsFunction(t *testing.T) {
	const src = boxedSigPrelude +
		"const out = Object.values(m).map((r: Row) => r.tag);\n" +
		"console.log(out.join(','));"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.NewFunc") {
		t.Fatalf("the inline callback did not lower as a boxed callback:\n%s", out)
	}
}

// TestATernaryArmThatIsABoxSettlesTheWholeTernary reads the rule the parameter, the
// result and the field already take at the one expression that spells two values into a
// single slot. The IIFE a ternary lowers to has one Go result type, and only the box is
// one the other arm can be brought to.
func TestATernaryArmThatIsABoxSettlesTheWholeTernary(t *testing.T) {
	const src = boxedSigPrelude +
		"const out = Object.values(m).map((r: Row) => r.id > 1 ? r : { id: 0, tag: 'z' });\n" +
		"console.log(out.map((r: Row) => r.tag).join(','));"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func() value.Value {") {
		t.Fatalf("a ternary with a boxed arm did not answer a box:\n%s", out)
	}
	if !strings.Contains(out, "value.NewObject()") {
		t.Fatalf("the static arm did not box on its way into the shared slot:\n%s", out)
	}
}
