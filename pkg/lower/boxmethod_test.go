package lower

import (
	"strings"
	"testing"
)

// A comparison against an object that carries a coercion method runs the object
// through the abstract-to-primitive protocol before it compares. The runtime half
// (value.ToPrimitive reading valueOf, toString, and Symbol.toPrimitive) landed in
// #503; this slice adds the lowering half, so boxObjectLiteral boxes a plain
// parameterless method whose body is a single return into a value.NewFunc closure.
// A dynamic-typed object then routes its comparison through dynamicBinary, which
// already coerces a dynamic operand through value.Less and value.LooseEquals, so
// the method the object carries is the one the coercion calls.

func TestBoxedValueOfCoercesForRelational(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const o: any = { valueOf() { return 5 } }; console.log(o < 10);`); got != "true\n" {
		t.Fatalf("{ valueOf: () => 5 } < 10 = %q, want true", got)
	}
}

func TestBoxedValueOfCoercesForRelationalFalse(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const o: any = { valueOf() { return 5 } }; console.log(o > 8);`); got != "false\n" {
		t.Fatalf("{ valueOf: () => 5 } > 8 = %q, want false", got)
	}
}

func TestBoxedValueOfCoercesForEquality(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const o: any = { valueOf() { return 5 } }; console.log(o == 5);`); got != "true\n" {
		t.Fatalf("{ valueOf: () => 5 } == 5 = %q, want true", got)
	}
}

func TestBoxedSymbolToPrimitiveCoercesForEquality(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const o: any = { [Symbol.toPrimitive]() { return 5 } }; console.log(o == 5);`); got != "true\n" {
		t.Fatalf("{ [Symbol.toPrimitive]: () => 5 } == 5 = %q, want true", got)
	}
}

func TestBoxedSymbolToPrimitiveCoercesForRelational(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const o: any = { [Symbol.toPrimitive]() { return 7 } }; console.log(o > 3);`); got != "true\n" {
		t.Fatalf("{ [Symbol.toPrimitive]: () => 7 } > 3 = %q, want true", got)
	}
}

// The boxed method is emitted as a value.NewFunc closure that ignores its
// arguments, so the coercion protocol finds a callable in the slot the method
// named.
func TestBoxedMethodEmitsNewFuncClosure(t *testing.T) {
	src := `const o: any = { valueOf() { return 5 } }; console.log(o < 10);`
	got := renderProgramTolerant(t, src)
	if !strings.Contains(got, "value.NewFunc") {
		t.Fatalf("emit did not box the method through value.NewFunc:\n%s", got)
	}
	if !strings.Contains(got, "value.Less") {
		t.Fatalf("emit did not route the comparison through value.Less:\n%s", got)
	}
}

// A method that reads its receiver still hands back: the free closure carries no
// this binding, so a this in the body must decline rather than lower to a stray
// reference. The whole object literal then routes to the engine.
func TestBoxedThisMethodHandsBack(t *testing.T) {
	src := `const o: any = { x: 5, valueOf() { return this.x } }; console.log(o < 10);`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "this") && !strings.Contains(reason, "later slice") {
		t.Fatalf("this-reading method reason = %q, want a handback", reason)
	}
}
