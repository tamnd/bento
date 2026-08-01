package lower

import (
	"strings"
	"testing"
)

// TestAssertionOverABoxKeepsTheDynamicPath pins the routing. An assertion over a value
// that is already a boxed value.Value is erased at run time, so the read off it has to
// dispatch through the runtime Get rather than select a Go struct field named for the
// asserted type's property. The emit selecting .Message off a value.Value is what this
// used to be, and Go does not compile that.
func TestAssertionOverABoxKeepsTheDynamicPath(t *testing.T) {
	for name, src := range map[string]string{
		"caught error": "function f(): void { throw new Error('bad'); }\n" +
			"try { f(); } catch (e) { console.log((e as Error).message); }",
		"parsed shape": "console.log((JSON.parse('{\"a\":1}') as { a: number }).a);",
		"any shape":    "const o: any = { n: 2 };\nconsole.log((o as { n: number }).n);",
		"angle form":   "const o: any = { n: 2 };\nconsole.log((<{ n: number }>o).n);",
	} {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, src)
			if !strings.Contains(out, ".Get(value.FromGoString(") {
				t.Fatalf("a read off an asserted box did not dispatch dynamically:\n%s", out)
			}
		})
	}
}

// TestOptionalDynamicPropertyNeedsNoOption pins the field an optional any or unknown gets.
// Both lower to a value.Value, which already carries undefined as one of the kinds it can
// hold, and the zero Value is exactly that undefined, so an omitted member reads as
// undefined with nothing written for it and an Opt would only give the shape a second
// spelling of absent. This is the whole of why the Error type could not intern at all: the
// es2022 lib declares cause?: unknown on it.
func TestOptionalDynamicPropertyNeedsNoOption(t *testing.T) {
	const src = "type Bag = { a: number; b?: unknown; c?: any };\n" +
		"const g: Bag = { a: 1 };\nconsole.log(g.a);"
	out := renderProgram(t, src)
	if strings.Contains(out, "value.Opt[value.Value]") {
		t.Fatalf("an optional dynamic property took an option wrapper:\n%s", out)
	}
	if !strings.Contains(out, "B value.Value") || !strings.Contains(out, "C value.Value") {
		t.Fatalf("an optional dynamic property did not take a bare value:\n%s", out)
	}
}

// TestABuiltInErrorNeverInternsAsAStruct guards the consequence of making the optional
// unknown property renderable. The Error type was only held out of the struct path by
// accident, on cause?: unknown, and once that shape rendered it started interning like
// any other plain object. It is not one: an error is the *value.Error the throw path
// raises and the boxing path hands to a dynamic sink through its own ToValue. Boxing one
// through ObjectFromStruct instead read the error as an empty object, so String(err) gave
// "Error" where node gives "Error: boom". That is a wrong answer rather than a hand-back.
func TestABuiltInErrorNeverInternsAsAStruct(t *testing.T) {
	const src = "function f(v: any): void { console.log(v); }\nf(new Error('boom'));"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.NewError(value.FromGoString(\"boom\")).ToValue()") {
		t.Fatalf("a thrown error boxed into a dynamic slot did not take its own ToValue:\n%s", out)
	}
	if strings.Contains(out, "ObjectFromStruct") {
		t.Fatalf("a built-in error interned as a struct shape:\n%s", out)
	}
}

// TestAssertionToAPrimitiveStillCoerces pins the boundary the erasure stops at. An
// assertion to a number, string, or boolean keeps the coercion it has, since the box has
// to land in a Go float64, value.BStr, or bool for every static sink one can flow into,
// and those sinks do not all know how to take a box yet. Erasing it would be the more
// faithful answer, since Node reads ('7' as number) + 1 as the concat "71", and that is
// its own slice.
func TestAssertionToAPrimitiveStillCoerces(t *testing.T) {
	const src = "const x: any = 7;\nconst n: number = x as number;\nconsole.log(n + 1);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.ToNumber(") {
		t.Fatalf("an assertion to a primitive dropped its coercion:\n%s", out)
	}
}
