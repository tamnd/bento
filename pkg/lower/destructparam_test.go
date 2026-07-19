package lower

import (
	"errors"
	"strings"
	"testing"
)

// A destructured parameter has no Go equivalent, so the whole object or array
// arrives in one synthesized field and the names the pattern bound are read out of
// it at the top of the body. The reads are the same struct-field selectors and
// indexed reads a `const {a} = o` or `const [x] = xs` statement lowers to.

// TestDestructuredParamLowersToEntryBindings proves an object-pattern parameter
// and an array-pattern parameter each bind their names from the held field at body
// entry rather than leaving the names undefined.
func TestDestructuredParamLowersToEntryBindings(t *testing.T) {
	cases := []struct {
		name  string
		src   string
		wants []string
	}{
		{
			"object",
			"function area({w, h}: {w: number, h: number}): number { return w * h; }\nconsole.log(area({w: 3, h: 4}));\n",
			[]string{"w := __0.W", "h := __0.H"},
		},
		{
			"array",
			"function diff([x, y]: number[]): number { return x - y; }\nconsole.log(diff([9, 4]));\n",
			[]string{"x := __0.AtI(0)", "y := __0.AtI(1)"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			for _, want := range tc.wants {
				if !strings.Contains(source, want) {
					t.Errorf("destructured parameter did not print %q:\n%s", want, source)
				}
			}
		})
	}
}

// TestDestructuredParamRuns builds and runs object, array, and mixed patterns so
// each bound name is proven to carry the right field or element against the
// JavaScript result rather than just the emitted shape.
func TestDestructuredParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function area({w, h}: {w: number, h: number}): number {
  return w * h;
}
function diff([x, y]: number[]): number {
  return x - y;
}
function label({name, id}: {name: string, id: number}): string {
  return name + id;
}
function shift(base: number, {by}: {by: number}): number {
  return base + by;
}
console.log(area({w: 3, h: 4}));
console.log(diff([9, 4]));
console.log(label({name: "n", id: 7}));
console.log(shift(10, {by: 5}));
`
	if got, want := runProgramGo(t, src), "12\n5\nn7\n15\n"; got != want {
		t.Fatalf("destructured parameter program printed %q, want %q", got, want)
	}
}

// TestDestructuredParamDefaultRuns proves a default inside a destructured parameter
// fills the bound name when the source slot is undefined and keeps the read value
// otherwise, for both the object and the array pattern forms.
func TestDestructuredParamDefaultRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function box({w = 2, h}: {w?: number, h: number}): number {
  return w * h;
}
function head([a, b = 9]: number[]): number {
  return a + b;
}
let missing: {w?: number, h: number} = {h: 4};
let full: {w?: number, h: number} = {w: 3, h: 4};
console.log(box(missing));
console.log(box(full));
console.log(head([10, 5]));
console.log(head([10]));
`
	if got, want := runProgramGo(t, src), "8\n12\n15\n19\n"; got != want {
		t.Fatalf("destructured parameter default printed %q, want %q", got, want)
	}
}

// TestDestructuredParamRestRuns proves an array-pattern parameter with a trailing rest
// binds the fixed slots by index and gathers the tail into the rest target at body
// entry.
func TestDestructuredParamRestRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function tail([first, ...rest]: number[]): number {
  return first + rest.length;
}
console.log(tail([10, 20, 30, 40]));
console.log(tail([7]));
`
	if got, want := runProgramGo(t, src), "13\n7\n"; got != want {
		t.Fatalf("array rest parameter printed %q, want %q", got, want)
	}
}

// TestDestructuredParamObjectRestHandsBack proves an object rest in a parameter
// pattern hands back, the same phase-7 gate the declaration and assignment forms take:
// gathering the remaining own properties into an object needs the object model the AOT
// cannot enumerate yet, so it hands back rather than emit a partial gather.
func TestDestructuredParamObjectRestHandsBack(t *testing.T) {
	const src = "function f({a, ...rest}: {a: number, b: number}): number { return a; }\nf({a: 1, b: 2});\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
}

// TestDestructuredParamRenameRuns proves a renamed target in an object-pattern
// parameter binds the renamed name at body entry, reading the source property off the
// held object into the renamed local.
func TestDestructuredParamRenameRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function f({a: x}: {a: number}): number { return x; }\nconsole.log(f({a: 41}));\n"
	if got, want := runProgramGo(t, src), "41\n"; got != want {
		t.Fatalf("object rename parameter printed %q, want %q", got, want)
	}
}

// TestDestructuredParamNestedArrayRuns proves a nested array pattern in a parameter
// binds the whole tree at body entry, reading each inner element off the slot the
// outer pattern selected on the held argument.
func TestDestructuredParamNestedArrayRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function f([[a, b], [c, d]]: number[][]): number { return a + b + c + d; }\nconsole.log(f([[1, 2], [3, 4]]));\n"
	if got, want := runProgramGo(t, src), "10\n"; got != want {
		t.Fatalf("nested array parameter printed %q, want %q", got, want)
	}
}

// TestDestructuredParamNestedObjectRuns proves a nested object pattern in a parameter
// binds the inner properties at body entry off the value the outer property selected.
func TestDestructuredParamNestedObjectRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function f({ p: { x, y } }: { p: { x: number; y: number } }): number { return x + y; }\nconsole.log(f({ p: { x: 1, y: 2 } }));\n"
	if got, want := runProgramGo(t, src), "3\n"; got != want {
		t.Fatalf("nested object parameter printed %q, want %q", got, want)
	}
}

// TestArrowDestructuredParamRuns proves an arrow function reads its destructured
// parameter's bound names out of the synthesized field at body entry, for both the
// block body and the concise body forms and for object and array patterns.
func TestArrowDestructuredParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const area = ({w, h}: {w: number; h: number}): number => w * h;
const diff = ([x, y]: number[]): number => { return x - y; };
const shout = ({a}: {a: number}) => console.log(a);
console.log(area({w: 3, h: 4}));
console.log(diff([9, 4]));
shout({a: 7});
`
	if got, want := runProgramGo(t, src), "12\n5\n7\n"; got != want {
		t.Fatalf("arrow destructured parameter printed %q, want %q", got, want)
	}
}

// TestFunctionExprDestructuredParamRuns proves an anonymous function expression binds
// its destructured parameter the same way an arrow and a declaration do.
func TestFunctionExprDestructuredParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const area = function({w, h}: {w: number; h: number}): number { return w * h; };\nconsole.log(area({w: 5, h: 6}));\n"
	if got, want := runProgramGo(t, src), "30\n"; got != want {
		t.Fatalf("function expression destructured parameter printed %q, want %q", got, want)
	}
}

// TestMethodDestructuredParamRuns proves an instance method reads its destructured
// parameter's bound names at body entry, for object and array patterns.
func TestMethodDestructuredParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `class C {
  area({w, h}: {w: number; h: number}): number { return w * h; }
  diff([x, y]: number[]): number { return x - y; }
}
const c = new C();
console.log(c.area({w: 3, h: 4}));
console.log(c.diff([9, 4]));
`
	if got, want := runProgramGo(t, src), "12\n5\n"; got != want {
		t.Fatalf("method destructured parameter printed %q, want %q", got, want)
	}
}

// TestStaticMethodDestructuredParamRuns proves a static method binds its destructured
// parameter through the same entry bindings a plain function takes.
func TestStaticMethodDestructuredParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = "class C {\n  static twice({a}: {a: number}): number { return a * 2; }\n}\nconsole.log(C.twice({a: 5}));\n"
	if got, want := runProgramGo(t, src), "10\n"; got != want {
		t.Fatalf("static method destructured parameter printed %q, want %q", got, want)
	}
}

// TestAsyncMethodDestructuredParamHandsBack proves an async method with a destructured
// parameter hands back, since the promise coroutine has no entry-binding hook yet.
func TestAsyncMethodDestructuredParamHandsBack(t *testing.T) {
	const src = "class C {\n  async m({a}: {a: number}): Promise<number> { return a; }\n}\nnew C().m({a: 1});\n"
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "destructured parameter") {
		t.Fatalf("async method handback reason = %q, want a destructured-parameter reason", reason)
	}
}

// TestGeneratorFuncExprDestructuredParamRuns proves a generator function expression
// reads its destructured parameter's bound names at the top of the coroutine body.
func TestGeneratorFuncExprDestructuredParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const g = function* ({a, b}: {a: number; b: number}) { yield a; yield b; };
for (const v of g({a: 1, b: 2})) { console.log(v); }
`
	if got, want := runProgramGo(t, src), "1\n2\n"; got != want {
		t.Fatalf("generator expression destructured parameter printed %q, want %q", got, want)
	}
}

// TestAsyncFuncExprDestructuredParamRuns proves an await-free async function
// expression binds its destructured parameter at the top of the value.Async body.
func TestAsyncFuncExprDestructuredParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const f = async function ({x}: {x: number}): Promise<number> { return x * 2; };
f({x: 5}).then((v) => console.log(v));
`
	if got, want := runProgramGo(t, src), "10\n"; got != want {
		t.Fatalf("async expression destructured parameter printed %q, want %q", got, want)
	}
}

// TestAsyncArrowDestructuredParamRuns proves a block-body await-free async arrow binds
// its destructured parameter through the same asyncBody entry bindings.
func TestAsyncArrowDestructuredParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const f = async ({y}: {y: number}): Promise<number> => { return y + 1; };
f({y: 10}).then((v) => console.log(v));
`
	if got, want := runProgramGo(t, src), "11\n"; got != want {
		t.Fatalf("async arrow destructured parameter printed %q, want %q", got, want)
	}
}

// TestAsyncGeneratorExprDestructuredParamRuns proves an async generator function
// expression with a destructured parameter binds its names through the same coroutine
// entry-binding hook the declaration form uses.
func TestAsyncGeneratorExprDestructuredParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const g = async function* ({a}: {a: number}) { yield a; };
(async () => { const it = g({a: 1}); console.log((await it.next()).value); })();
`
	if got, want := runProgramGo(t, src), "1\n"; got != want {
		t.Fatalf("async generator expression destructured parameter printed %q, want %q", got, want)
	}
}

// TestNamedFuncExprDestructuredParamRuns proves a named function expression with a
// destructured parameter binds its names through blockBodyArrow's entry bindings, which
// the self-reference two-step wraps.
func TestNamedFuncExprDestructuredParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const g = function h({x, y}: {x: number; y: number}): number { return x + y; };
console.log(g({x: 3, y: 4}));
`
	if got, want := runProgramGo(t, src), "7\n"; got != want {
		t.Fatalf("named function expression destructured parameter printed %q, want %q", got, want)
	}
}

// TestNamedFuncExprRecursiveDestructuredParamRuns proves the self-reference two-step
// still resolves a recursive call inside a named function expression whose parameter is
// an array pattern, the name bound to the Go local wrapping the entry-binding closure.
func TestNamedFuncExprRecursiveDestructuredParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const fac = function f([n, acc]: number[]): number { return n <= 1 ? acc : f([n - 1, acc * n]); };
console.log(fac([5, 1]));
`
	if got, want := runProgramGo(t, src), "120\n"; got != want {
		t.Fatalf("recursive named function expression destructured parameter printed %q, want %q", got, want)
	}
}

// TestAwaitingAsyncExprDestructuredParamHandsBack proves an awaiting async function
// expression with a destructured parameter hands back: an awaiting body lowers through
// asyncCoroutineBody, which has no entry-binding hook yet.
func TestAwaitingAsyncExprDestructuredParamHandsBack(t *testing.T) {
	const src = `const f = async function ({x}: {x: number}): Promise<number> { return await Promise.resolve(x); };
f({x: 1});
`
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "destructured parameter") {
		t.Fatalf("awaiting async expression handback reason = %q, want a destructured-parameter reason", reason)
	}
}
