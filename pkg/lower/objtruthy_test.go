package lower

import (
	"strings"
	"testing"
)

// TestObjectTruthyEmits pins the object-in-boolean-position lowering: an object
// operand is always truthy, so a repeatable object condition collapses to the Go
// constant true rather than testing a value with no falsy member. A null-only
// operand is always falsy and collapses to false.
func TestObjectTruthyEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"object condition is always true",
			"function f(o: { x: number }): number { if (o) { return 1; } return 0; }\nconsole.log(f({ x: 2 }));\n",
			"if true {",
		},
		{
			"object in a while is always true",
			"function f(o: { x: number }): number { while (o) { return 1; } return 0; }\nconsole.log(f({ x: 2 }));\n",
			"for true {",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("object condition did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestObjectTruthyRuns builds and runs an object condition: the object branch is
// taken because an object is always truthy, so the guarded body runs.
func TestObjectTruthyRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function pick(o: { x: number }): number {
  if (o) {
    return o.x;
  }
  return -1;
}
console.log(pick({ x: 7 }));
`
	got := runProgramGo(t, src)
	if got != "7\n" {
		t.Fatalf("object condition program printed %q, want %q", got, "7\n")
	}
}
