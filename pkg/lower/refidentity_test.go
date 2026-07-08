package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestReferenceIdentityEmits pins the shape of the lowering: an equality between
// two operands of the same non-primitive reference type becomes a Go pointer
// comparison, === and == to a Go ==, !== and != to a Go !=, since JavaScript
// object equality is reference identity and both operands lower to a pointer.
func TestReferenceIdentityEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"objectStrictEq",
			"const a = { x: 1 };\nconst b = { x: 1 };\nconsole.log(a === b);\n",
			"a == b",
		},
		{
			"objectStrictNe",
			"const a = { x: 1 };\nconst b = { x: 1 };\nconsole.log(a !== b);\n",
			"a != b",
		},
		{
			"objectLooseEq",
			"const a = { x: 1 };\nconst b = { x: 1 };\nconsole.log(a == b);\n",
			"a == b",
		},
		{
			"arrayStrictEq",
			"const p = [1, 2];\nconst q = [1, 2];\nconsole.log(p === q);\n",
			"p == q",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("reference identity did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestReferenceIdentityHandsBack pins the boundary: two operands whose object
// shapes differ render to different Go pointer types, so a Go == on them would
// not type-check. Their values can never be the same reference, so the answer is
// statically known, but that fold is a later slice and the form hands back rather
// than emit a comparison of unlike pointers.
func TestReferenceIdentityHandsBack(t *testing.T) {
	const src = "const a: { x: number } = { x: 1 };\n" +
		"const b: { x: number; y: number } = { x: 1, y: 2 };\n" +
		"console.log(a === b);\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "mixed or non-primitive operands") {
		t.Errorf("hand-back reason = %q, want it to mention mixed or non-primitive operands", nyl.Reason)
	}
}

// TestReferenceIdentityRuns builds and runs object and array identity end to end:
// two distinct literals are never equal, an alias of one is equal to it, !== and
// the loose == and != agree with === over references since neither operand is a
// primitive to coerce.
func TestReferenceIdentityRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = { x: 1 };
const b = { x: 1 };
const c = a;
console.log(a === b);
console.log(a === c);
console.log(a !== b);
console.log(a == b);
console.log(a != b);
const p = [1, 2];
const q = [1, 2];
const r = p;
console.log(p === q);
console.log(p === r);
`
	got := runProgramGo(t, src)
	want := "false\n" +
		"true\n" +
		"true\n" +
		"false\n" +
		"true\n" +
		"false\n" +
		"true\n"
	if got != want {
		t.Fatalf("reference identity program printed %q, want %q", got, want)
	}
}
