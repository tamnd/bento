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

// TestTypeofPrimitiveUnionRunsPerArm proves typeof over a primitive tagged-sum union
// reports each arm's tag at run time: the union carries no self-describing box, so the
// operand is asked for its tag through the union's generated TypeOf method, and the
// answer follows the arm the value currently holds.
func TestTypeofPrimitiveUnionRunsPerArm(t *testing.T) {
	skipIfShort(t)
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"number then string",
			"let x: number | string = 1;\nconsole.log(typeof x);\nx = \"z\";\nconsole.log(typeof x);\n",
			"number\nstring\n",
		},
		{
			"number string boolean",
			"let x: number | string | boolean = false;\nconsole.log(typeof x);\nx = 3;\nconsole.log(typeof x);\nx = \"z\";\nconsole.log(typeof x);\n",
			"boolean\nnumber\nstring\n",
		},
		{
			"number then bigint",
			"let x: number | bigint = 1n;\nconsole.log(typeof x);\nx = 2;\nconsole.log(typeof x);\n",
			"bigint\nnumber\n",
		},
		{
			"in concatenation",
			"let x: number | string = 1;\nconsole.log(\"t=\" + typeof x);\n",
			"t=number\n",
		},
		{
			"in a template",
			"let x: number | string | boolean = true;\nconsole.log(`${typeof x}`);\n",
			"boolean\n",
		},
		{
			"as a function return",
			"function f(x: number | string): string { return typeof x; }\nconsole.log(f(1), f(\"a\"));\n",
			"number string\n",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			if got := runProgramGo(t, c.src); got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

// TestTypeofUnionEmitsMethod pins that typeof over a primitive union lowers to a call
// on the union's TypeOf method, the runtime tag switch, rather than folding.
func TestTypeofUnionEmitsMethod(t *testing.T) {
	const src = "export function k(x: number | string): string { return typeof x; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".TypeOf()") {
		t.Errorf("typeof over a union did not call the TypeOf method:\n%s", source)
	}
	if !strings.Contains(source, "func (u NumOrStr) TypeOf()") {
		t.Errorf("the union TypeOf method was not emitted:\n%s", source)
	}
}

// TestTypeofUnionCompareStillFolds pins that a typeof compare, typeof x === "number",
// keeps folding to a discriminant-tag test and does not route through the TypeOf
// method, so the compare path the narrowing uses is untouched by the bare-typeof slice.
func TestTypeofUnionCompareStillFolds(t *testing.T) {
	skipIfShort(t)
	const src = "let x: number | string = 1;\nconsole.log(typeof x === \"number\", typeof x === \"string\");\n"
	if got := runProgramGo(t, src); got != "true false\n" {
		t.Fatalf("got %q, want %q", got, "true false\n")
	}
}
