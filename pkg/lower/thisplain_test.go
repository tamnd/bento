package lower

import (
	"strings"
	"testing"
)

// A plain function's `this` is the call's to decide, and a Go closure has no receiver
// slot for a call to decide it through, so a strict body reads the undefined a
// receiver-free call leaves. That is what Node's suite asks for: 769 of its tests
// stopped at one of the three refusals these tests replace, most of them because
// test/common/index.js wraps a callback in `function (...) { fn.apply(this, args) }`.
//
// The tests below are the two halves of that. What lowers: a function expression, a
// nested declaration, a top-level declaration, and an arrow that inherits from any of
// them. What still hands back: sloppy mode, where `this` is the global object bento
// has no value for, and the three syntactic places a value goes straight into a
// property, where JavaScript would supply the owning object as the receiver.

// TestAStrictFunctionExpressionReadsThisAsUndefined is the shape common/index.js
// reaches through mustCall: a wrapper that only passes `this` along to apply.
func TestAStrictFunctionExpressionReadsThisAsUndefined(t *testing.T) {
	src := `"use strict";
function wrap(fn: (x: number) => number): (x: number) => number {
  const g = function (x: number): number { return typeof this === "undefined" ? fn(x) : 0; };
  return g;
}
console.log(wrap((x: number) => x + 1)(1));`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "value.Undefined") {
		t.Errorf("the function expression's this did not lower to undefined:\n%s", out)
	}
}

// TestAStrictNestedFunctionReadsThisAsUndefined covers the declaration spelling, the
// family that held 593 of the group's tests on its own.
func TestAStrictNestedFunctionReadsThisAsUndefined(t *testing.T) {
	src := `"use strict";
function outer(): string {
  function inner(): string { return typeof this; }
  return inner();
}
console.log(outer());`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "value.Undefined") {
		t.Errorf("the nested declaration's this did not lower to undefined:\n%s", out)
	}
}

// TestAStrictTopLevelFunctionReadsThisAsUndefined covers the declaration that is not
// nested in anything, which reached the bare `this outside a lowered class body`
// refusal rather than either of the two upfront ones.
func TestAStrictTopLevelFunctionReadsThisAsUndefined(t *testing.T) {
	src := `"use strict";
function describe(): string { return typeof this; }
console.log(describe());`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "value.Undefined") {
		t.Errorf("the top-level declaration's this did not lower to undefined:\n%s", out)
	}
}

// TestAStrictPlainFunctionsThisRunsAsUndefined is the end-to-end check. A `this` that
// lowered to something Go accepts but JavaScript does not name is a wrong answer, not
// a build failure, so only running it says which.
func TestAStrictPlainFunctionsThisRunsAsUndefined(t *testing.T) {
	skipIfShort(t)
	src := `"use strict";
function describe(): string { return typeof this; }
function outer(): string {
  function inner(): string { return typeof this; }
  const expr = function (): string { return typeof this; };
  const arrow = () => typeof this;
  return inner() + " " + expr() + " " + arrow();
}
console.log(describe());
console.log(outer());
`
	got := runProgramGoTolerant(t, src)
	want := "undefined\nundefined undefined undefined\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestAnArrowInsideAPlainFunctionSeesTheFunctionsThis is the scoping assertion. An
// arrow has no this of its own and reads the enclosing function's, so an arrow inside
// a plain function inside a method must see the plain function's undefined and not
// the method's receiver.
func TestAnArrowInsideAPlainFunctionSeesTheFunctionsThis(t *testing.T) {
	src := `"use strict";
class Counter {
  n = 1;
  describe(): string {
    const expr = function (): string {
      const arrow = () => typeof this;
      return arrow();
    };
    return expr() + String(this.n);
  }
}
console.log(new Counter().describe());`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "value.Undefined") {
		t.Fatalf("the arrow did not read the plain function's this:\n%s", out)
	}
	// The method's own this is still the receiver, so the two bindings coexist rather
	// than one replacing the other.
	if !strings.Contains(out, ".N") {
		t.Errorf("the method's own this stopped reaching the receiver field:\n%s", out)
	}
}

// TestANonStrictFunctionThatReadsThisHandsBack is the limit. Sloppy mode binds the
// global object, which bento has no value for, so the unit runs on the engine.
func TestANonStrictFunctionThatReadsThisHandsBack(t *testing.T) {
	src := `function describe(): string { return typeof this; }
console.log(describe());`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "non-strict") {
		t.Errorf("reason = %q, want the sloppy-mode hand-back", reason)
	}
}

// TestAFunctionInAnObjectLiteralPropertyHandsBack is the receiver position. The
// closure fills a struct field and `o.m()` calls that field with nothing bound, where
// JavaScript would bind the object, so lowering it would answer undefined for a this
// the source named.
func TestAFunctionInAnObjectLiteralPropertyHandsBack(t *testing.T) {
	src := `"use strict";
const o = { m: function (): string { return typeof this; } };
console.log(o.m());`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "straight into a property") {
		t.Errorf("reason = %q, want the receiver-position hand-back", reason)
	}
}

// TestAFunctionInAClassFieldHandsBack is the same position spelled as a field
// initializer, where `c.m()` binds the instance.
func TestAFunctionInAClassFieldHandsBack(t *testing.T) {
	src := `"use strict";
class Holder {
  m = function (): string { return typeof this; };
}
const h = new Holder();
console.log(typeof h.m);`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "straight into a property") {
		t.Errorf("reason = %q, want the receiver-position hand-back", reason)
	}
}

// TestAFunctionStoredIntoAPropertyHandsBack is the third spelling, the store rather
// than the initializer.
func TestAFunctionStoredIntoAPropertyHandsBack(t *testing.T) {
	src := `"use strict";
class Holder {
  m: () => string = (): string => "";
}
const h = new Holder();
h.m = function (): string { return typeof this; };
console.log(h.m());`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "straight into a property") {
		t.Errorf("reason = %q, want the receiver-position hand-back", reason)
	}
}

// TestCallSupplyingAReceiverToAThisReadingFunctionHandsBack guards the other door
// into the same mistake. bento drops the this argument of .call and .apply because
// the callee never reads it, and a callee that now does read it would get undefined
// where the source named a receiver. The receiver here is a literal on purpose: a
// name or an object literal is not droppable at all and stops one gate earlier, so
// only a droppable non-nullish value reaches this one.
func TestCallSupplyingAReceiverToAThisReadingFunctionHandsBack(t *testing.T) {
	src := `"use strict";
function describe(): string { return typeof this; }
console.log(describe.call(5));`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "supplying a receiver") {
		t.Errorf("reason = %q, want the receiver hand-back", reason)
	}
}

// TestCallWithANullReceiverStillLowers is what that guard leaves alone. Strict mode
// passes null and undefined through untouched rather than substituting the global
// object, so the callee reads the same undefined the dropped argument leaves.
func TestCallWithANullReceiverStillLowers(t *testing.T) {
	src := `"use strict";
function describe(): string { return typeof this; }
console.log(describe.call(null));`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "value.Undefined") {
		t.Errorf("a null receiver did not lower through the plain call:\n%s", out)
	}
}

// TestAMethodStillReadsThisAsItsReceiver is the no-regression assertion. Nothing here
// touches a class body's this, and the plain-function scope must not leak into one.
func TestAMethodStillReadsThisAsItsReceiver(t *testing.T) {
	src := `class Counter {
  n = 1;
  bump(): number { this.n = this.n + 1; return this.n; }
}
console.log(new Counter().bump());`
	out := renderProgram(t, src)
	if strings.Contains(out, "value.Undefined") {
		t.Errorf("a method's this stopped being its receiver:\n%s", out)
	}
}
