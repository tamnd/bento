package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestDynamicIncDecEmits pins the shape of the lowering: a ++ or -- on a dynamic
// target in statement position reads the target, runs the numeric update through
// value.Inc or value.Dec, and assigns the result back, since a boxed value has no
// Go ++ to apply.
func TestDynamicIncDecEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"increment",
			"function f(x: any): any { x++; return x; }\nconsole.log(f(1));\n",
			"x = value.Inc(x)",
		},
		{
			"decrement",
			"function f(x: any): any { x--; return x; }\nconsole.log(f(1));\n",
			"x = value.Dec(x)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("dynamic update did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestBoxedLocalIncDecEmits pins that a ++ on a local declared without an
// initializer routes through value.Inc even after control flow narrows the local
// to number. var count; count = 0; count++ types count number at the ++, but the
// storage is still a value.Value box, so a Go count++ would try to increment a box
// and fail to build. The update reads the box through value.Inc instead.
func TestBoxedLocalIncDecEmits(t *testing.T) {
	const src = "var count;\ncount = 0;\ncount++;\nconsole.log(count);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "count = value.Inc(count)") {
		t.Errorf("boxed-local increment did not print value.Inc:\n%s", source)
	}
	if strings.Contains(source, "count++") {
		t.Errorf("boxed-local increment printed a Go count++ over a box:\n%s", source)
	}
}

// TestBoxedLocalIncDecValueHandsBack pins that reading the result of a ++ on a
// boxed local hands back rather than emitting the float64 closure the number path
// builds, which would not compile against the box. Statement position lowers the
// same update through value.Inc; this is the value form that still needs a slice.
func TestBoxedLocalIncDecValueHandsBack(t *testing.T) {
	const src = "var count;\ncount = 0;\nconst y = count++;\nconsole.log(y);\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "boxed local") {
		t.Errorf("hand-back reason = %q, want it to mention a boxed local", nyl.Reason)
	}
}

// TestDynamicIncDecHandsBack pins the boundary: a ++ or -- whose result is used as
// a value needs the old value in a temporary, so it hands back until that slice
// lands, even though the same update in statement position lowers.
func TestDynamicIncDecHandsBack(t *testing.T) {
	const src = "function f(x: any): any { const y = x++; return y; }\nconsole.log(f(5));\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "value-position ++") {
		t.Errorf("hand-back reason = %q, want it to mention a value-position update", nyl.Reason)
	}
}

// TestDynamicIncDecRuns builds and runs ++ and -- on a dynamic target: a number
// updates, a numeric string and a boolean coerce to a number first, the ToNumeric
// contract the update keeps rather than the concatenation the + operator would do.
func TestDynamicIncDecRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function bump(x: any): any {
  x++;
  return x;
}
function drop(x: any): any {
  x--;
  return x;
}
console.log(bump(5));
console.log(bump("5"));
console.log(bump(true));
console.log(drop(3));
`
	got := runProgramGo(t, src)
	want := "6\n" +
		"6\n" +
		"2\n" +
		"2\n"
	if got != want {
		t.Fatalf("dynamic update program printed %q, want %q", got, want)
	}
}

// TestDynamicNonPlusCompoundEmits pins the shape of a compound assignment other
// than + on a dynamic target: combineBinary coerces the boxed target through
// ToNumber and runs the native operator, and value.Number boxes the float64 result
// back into the value.Value slot.
func TestDynamicNonPlusCompoundEmits(t *testing.T) {
	const src = `let x: any = 10;
x -= 3;
console.log(x);
`
	got := renderProgram(t, src)
	if !strings.Contains(got, "value.Number(value.ToNumber(x) - 3)") {
		t.Fatalf("want a boxed ToNumber subtraction, got:\n%s", got)
	}
}

// TestDynamicNonPlusCompoundRuns runs every arithmetic and bitwise compound on a
// dynamic local end to end, including a value-position use that coerces down.
func TestDynamicNonPlusCompoundRuns(t *testing.T) {
	skipIfShort(t)
	const src = `let x: any = 12;
x -= 3;
x *= 2;
x /= 2;
x %= 4;
x **= 5;
console.log(x);
let y: any = 6;
y &= 3;
y |= 8;
y ^= 1;
y <<= 2;
y >>= 1;
const r: number = (y -= 2);
console.log(r);
console.log(y);
`
	if got, want := runProgramGo(t, src), "1\n20\n20\n"; got != want {
		t.Fatalf("dynamic non-plus compound printed %q, want %q", got, want)
	}
}
