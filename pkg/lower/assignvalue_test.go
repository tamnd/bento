package lower

import (
	"strings"
	"testing"
)

// An assignment read for its value, not run as a statement on its own line, has no
// direct Go form: Go's assignment is a statement and yields nothing. The value form
// rides an immediately-called closure that assigns the target and returns it, since
// the value of x = e in JavaScript is the value assigned.

// TestAssignmentValueLowersToAssignThenReturn proves const r = (x = 5) lowers to a
// closure that assigns the target and returns it.
func TestAssignmentValueLowersToAssignThenReturn(t *testing.T) {
	const src = "let x = 0; const r = (x = 5); console.log(r, x);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "x = 5\n\t\treturn x") {
		t.Errorf("assignment in value position did not lower to an assign-then-return closure:\n%s", source)
	}
}

// TestAssignmentValueFormsRun builds and runs the value form in a binding, a while
// condition, and an element-assignment right side, so the assigned value is proven
// against the JavaScript result rather than just the emitted shape.
func TestAssignmentValueFormsRun(t *testing.T) {
	skipIfShort(t)
	const src = `
let x = 0;
const r = (x = 5);
console.log(r, x);

let i = 0;
const n = 3;
while ((i = i + 1) < n) {
  console.log(i);
}
console.log("done", i);

const a = [0, 0];
let y = 0;
a[0] = (y = 7);
console.log(a[0], y);
`
	if got, want := runProgramGo(t, src), "5 5\n1\n2\ndone 3\n7 7\n"; got != want {
		t.Fatalf("assignment value form printed %q, want %q", got, want)
	}
}

// TestAssignmentValueWideningYieldsNarrowType proves an assignment whose slot is
// wider than the assigned value yields the narrow assigned type, not the slot's. The
// value of d = x ?? "x", where d is string | undefined, is a string, so the closure
// returns a string while the string | undefined slot receives the widened value. The
// nullish coalesce d ?? (d = x ?? "x") reads that string branch, so the two arms of
// the coalesce share one Go type and the whole expression compiles.
func TestAssignmentValueWideningYieldsNarrowType(t *testing.T) {
	const src = "let x: string | undefined; let d: string | undefined; const r = d ?? (d = x ?? \"x\"); console.log(r);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "d = value.Some[value.BStr](") {
		t.Errorf("widening assignment did not store the value widened to the slot:\n%s", source)
	}
}

// TestAssignmentValueWideningRuns builds and runs the widening form so the assigned
// value the expression yields is proven against the JavaScript result.
func TestAssignmentValueWideningRuns(t *testing.T) {
	skipIfShort(t)
	const src = `let x: string | undefined;
let d: string | undefined;
console.log(d ?? (d = x ?? "x"));
console.log(d);
`
	if got, want := runProgramGo(t, src), "x\nx\n"; got != want {
		t.Fatalf("widening assignment value form printed %q, want %q", got, want)
	}
}

// TestAssignmentValuePropertyLowersToAssignThenReturn proves an assignment into a
// class or object field, read for its value, lowers to a closure that binds the
// right-hand side to a typed temp, writes it through the field selector, and
// returns the temp.
func TestAssignmentValuePropertyLowersToAssignThenReturn(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"classField",
			"class P { x = 0; set(v: number): number { return this.x = v; } }\nconst p = new P(); console.log(p.set(7));\n",
			"p.X = _bt0",
		},
		{
			"objectField",
			"const o = { n: 0 }; console.log(o.n = 5);\n",
			"o.N = _bt0",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("property assignment value did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestAssignmentValuePropertyAndTypedLocalsRun builds and runs the value form over
// a class field, an object field, a string local, and a chained assignment, so the
// assigned value each expression evaluates to is proven against the JavaScript
// result rather than just the emitted shape.
func TestAssignmentValuePropertyAndTypedLocalsRun(t *testing.T) {
	skipIfShort(t)
	const src = `class P { x = 0; set(v: number): number { return this.x = v; } }
const p = new P();
console.log(p.set(7), p.x);

const o = { n: 0 };
console.log((o.n = 5), o.n);

let s = "";
console.log(s = "hi");

let a = 0;
let b = 0;
console.log(a = b = 9, a, b);
`
	if got, want := runProgramGo(t, src), "7 7\n5 5\nhi\n9 9 9\n"; got != want {
		t.Fatalf("assignment value form printed %q, want %q", got, want)
	}
}

// TestAssignmentValueOnUnsupportedTargetHandsBack proves the value form still hands
// back where the write has no plain lvalue: an array element uses method access and
// a dynamic local has a boxed narrowed slot, so each names its own later slice.
func TestAssignmentValueOnUnsupportedTargetHandsBack(t *testing.T) {
	for _, src := range []string{
		"const a = [1, 2]; console.log(a[0] = 9);\n",
		"let d: any = 0; console.log(d = 5);\n",
	} {
		renderProgramHandBack(t, src)
	}
}
