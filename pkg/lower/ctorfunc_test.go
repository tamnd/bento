package lower

import (
	"strings"
	"testing"
)

// The ES5 constructor function is how JavaScript wrote classes before it had them, and
// it is how Node's whole test/common helper tree is still written. A function bento
// lowers to a bare Go func has no .prototype to read, no bag to write one onto, no
// receiver for its body's this, and nothing new can be applied to, so every line of the
// idiom handed back. These tests pin the four lines that make it up, the method half
// that hangs off the prototype, and the two shapes that must keep their old lowering:
// a function nothing constructs, which stays a Go func, and a callback whose this is
// still the undefined a receiver-free call leaves.

// TestAConstructedFunctionLowersToARuntimeConstructor pins the declaration half: the
// name binds a value.Value built by value.NewCtor rather than a Go func, so the one
// slot serves the reference, the call, the new, and the prototype write.
func TestAConstructedFunctionLowersToARuntimeConstructor(t *testing.T) {
	src := `"use strict";
function Shape(name) { this.name = name; }
const s = new Shape("box");
console.log(s.name);`
	out := renderUncheckedJS(t, src)
	if !strings.Contains(out, "value.NewCtor(\"Shape\"") {
		t.Errorf("the declaration did not lower to a runtime constructor:\n%s", out)
	}
	if !strings.Contains(out, "value.Construct(Shape") {
		t.Errorf("new did not lower to the runtime construct:\n%s", out)
	}
	if strings.Contains(out, "func Shape(") {
		t.Errorf("the constructor also emitted a Go func:\n%s", out)
	}
}

// TestAConstructorBodyReadsTheObjectItIsBuilding pins that this inside the body is the
// receiver [[Construct]] bound, not the undefined a receiver-free call leaves. The
// closure takes it as a parameter, which is the whole difference from a plain function.
func TestAConstructorBodyReadsTheObjectItIsBuilding(t *testing.T) {
	src := `"use strict";
function Shape(name) { this.name = name; }
console.log(new Shape("box").name);`
	out := renderUncheckedJS(t, src)
	if !strings.Contains(out, "bentoThis") {
		t.Errorf("the constructor body did not bind a receiver:\n%s", out)
	}
	if strings.Contains(out, "value.Undefined.Set(") {
		t.Errorf("the body wrote onto undefined instead of the new object:\n%s", out)
	}
}

// TestAPrototypeMethodReadsItsReceiver is the half the idiom spends most of its lines
// on. A function written onto a prototype is called back off an instance, so its body's
// this is that instance; boxed as an ordinary callable it would read undefined and
// answer wrongly, which is worse than handing back.
func TestAPrototypeMethodReadsItsReceiver(t *testing.T) {
	src := `"use strict";
function Shape(name) { this.name = name; }
Shape.prototype.who = function () { return "S:" + this.name; };
console.log(new Shape("box").who());`
	out := renderUncheckedJS(t, src)
	if !strings.Contains(out, "value.NewMethod(") {
		t.Errorf("the prototype method did not lower to a method value:\n%s", out)
	}
	if !strings.Contains(out, "value.CallMethod(") {
		t.Errorf("the call off the instance did not thread a receiver:\n%s", out)
	}
}

// TestAConstructorChainsThroughCall pins F.call(this, ...), how ES5 code runs a base
// constructor's body over the object a derived constructor is building. bento's plain
// functions have no receiver slot and refuse a real this argument; a constructor value
// has one, so the argument is honored rather than dropped.
func TestAConstructorChainsThroughCall(t *testing.T) {
	src := `"use strict";
function Base(kind) { this.kind = kind; }
function Derived() { Base.call(this, "base"); }
Object.setPrototypeOf(Derived.prototype, Base.prototype);
console.log(new Derived().kind);`
	out := renderUncheckedJS(t, src)
	if !strings.Contains(out, "value.CallWithThis(Base") {
		t.Errorf("the base call did not honor its receiver:\n%s", out)
	}
}

// TestInstanceofOverAConstructorWalksTheChain pins that instanceof answers through the
// runtime chain walk, the `if (!(this instanceof F))` guard the idiom opens with, rather
// than reaching the refusal that only let a caught built-in error through.
func TestInstanceofOverAConstructorWalksTheChain(t *testing.T) {
	src := `"use strict";
function Shape() {}
Shape.prototype.tag = 1;
const s = new Shape();
console.log(s instanceof Shape);`
	out := renderUncheckedJS(t, src)
	if !strings.Contains(out, "value.InstanceOf(") {
		t.Errorf("instanceof did not lower to the runtime chain walk:\n%s", out)
	}
}

// TestAFunctionNothingConstructsStaysAGoFunc is the no-regression assertion that
// matters most. A Go func is enormously better than a boxed value, and the
// overwhelming majority of functions never see new or .prototype, so only the ones the
// program actually treats as constructors may leave that path.
func TestAFunctionNothingConstructsStaysAGoFunc(t *testing.T) {
	src := `"use strict";
function add(a: number, b: number): number { return a + b; }
console.log(add(1, 2));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "func Add(") {
		t.Errorf("an ordinary function stopped lowering to a Go func:\n%s", out)
	}
	if strings.Contains(out, "value.NewCtor(") {
		t.Errorf("an ordinary function was claimed as a constructor:\n%s", out)
	}
}

// TestACallbackThisIsStillUndefined is the other no-regression assertion. A function
// expression that reads this only to hand it back out, the shape common/index.js wraps
// every callback in, must keep reading the undefined Node's own invocation puts there;
// the method box is for a value written onto an object, not for every boxed callable.
func TestACallbackThisIsStillUndefined(t *testing.T) {
	src := `"use strict";
function run(fn: (x: number) => number): number { return fn(1); }
console.log(run(function (x: number): number { return typeof this === "undefined" ? x : -1; }));`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "value.Undefined") {
		t.Errorf("a callback's this stopped reading as undefined:\n%s", out)
	}
	if strings.Contains(out, "value.NewMethod(") {
		t.Errorf("a plain callback was boxed as a method:\n%s", out)
	}
}
