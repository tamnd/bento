package lower

import (
	"strings"
	"testing"
)

// TestTypeofStaticFolds pins that typeof over a statically typed, side-effect-free
// operand folds to the tag as a string constant, so a typed program pays nothing at
// runtime for it and the operand does not appear in the output.
func TestTypeofStaticFolds(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"number", "export function k(x: number): string { return typeof x; }", `value.FromGoString("number")`},
		{"string", "export function k(x: string): string { return typeof x; }", `value.FromGoString("string")`},
		{"boolean", "export function k(x: boolean): string { return typeof x; }", `value.FromGoString("boolean")`},
		{"bigint", "export function k(x: bigint): string { return typeof x; }", `value.FromGoString("bigint")`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			source := renderProgram(t, c.src+"\n")
			if !strings.Contains(source, c.want) {
				t.Errorf("typeof %s did not fold to %q:\n%s", c.name, c.want, source)
			}
		})
	}
}

// TestTypeofFunctionTag pins that a callable operand folds to "function", the tag
// only a value with call signatures carries, told apart from a plain object.
func TestTypeofFunctionTag(t *testing.T) {
	const src = "export function k(f: () => number): string { return typeof f; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, `value.FromGoString("function")`) {
		t.Errorf("typeof over a function did not fold to \"function\":\n%s", source)
	}
}

// TestTypeofObjectTag pins that a plain object operand folds to "object".
func TestTypeofObjectTag(t *testing.T) {
	const src = "interface Pt { x: number }\n" +
		"export function k(p: Pt): string { return typeof p; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, `value.FromGoString("object")`) {
		t.Errorf("typeof over an object did not fold to \"object\":\n%s", source)
	}
}

// TestTypeofClassConstructorTag pins that typeof a class constructor folds to
// "function". A class type carries construct signatures rather than call
// signatures, and checking only call signatures folded it to "object", which
// broke the harness sta.js self-test (typeof Test262Error === "function").
func TestTypeofClassConstructorTag(t *testing.T) {
	const src = "class C { m(): number { return 1; } }\n" +
		"export function k(): string { return typeof C; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, `value.FromGoString("function")`) {
		t.Errorf("typeof over a class constructor did not fold to \"function\":\n%s", source)
	}
}

// TestTypeofDynamicCallsRuntime pins that a dynamic operand defers the tag to
// runtime, evaluated once and asked through value.Value.TypeOf, since its kind is
// not known until the boxed value exists.
func TestTypeofDynamicCallsRuntime(t *testing.T) {
	const src = "export function k(x: any): string { return typeof x; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "x.TypeOf()") {
		t.Errorf("typeof over a dynamic operand did not call TypeOf:\n%s", source)
	}
}
