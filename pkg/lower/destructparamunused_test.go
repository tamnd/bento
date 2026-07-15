package lower

import (
	"strings"
	"testing"
)

// A destructured parameter reads each bound name out of the held argument at body entry.
// When the body never reads one of those names the read is still emitted, so the name
// would be declared and not used in Go and the program would not compile. blankUnusedParamBinding
// appends `_ = name` for the orphaned member, the same blank a fold-orphaned variable
// declaration takes, so a parameter pattern with an unused member compiles. This covers the
// typed struct binder and the untyped dynamic binder, both of which the reads flow through.

// TestDestructuredParamUnusedMemberRuns proves a parameter member the body never reads no
// longer leaves the emitted Go with a declared-and-not-used error: each program compiles
// and prints the member it does read, across the typed and untyped object binders, the
// array binder, a renamed target, a defaulted member, and an unused sibling beside a rest.
func TestDestructuredParamUnusedMemberRuns(t *testing.T) {
	skipIfShort(t)
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"typed object member unused",
			"function f({ a, b }: { a: number, b: number }): number { return b; }\nconsole.log(f({ a: 1, b: 2 }));\n",
			"2\n",
		},
		{
			"untyped object member unused",
			"function f({ a, b }) { return b; }\nconsole.log(f({ a: 1, b: 2 }));\n",
			"2\n",
		},
		{
			"array member unused",
			"function f([a, b]) { return b; }\nconsole.log(f([1, 2]));\n",
			"2\n",
		},
		{
			"renamed target unused",
			"function f({ a: x, b }: { a: number, b: number }): number { return b; }\nconsole.log(f({ a: 1, b: 2 }));\n",
			"2\n",
		},
		{
			"defaulted member unused",
			"function f({ a = 5, b }: { a?: number, b: number }): number { return b; }\nconsole.log(f({ b: 2 }));\n",
			"2\n",
		},
		{
			"unused sibling beside object rest",
			"function f({ a, ...rest }) { return rest.b; }\nconsole.log(f({ a: 1, b: 2 }));\n",
			"2\n",
		},
		{
			"arrow object member unused",
			"const f = ({ a, b }: { a: number, b: number }): number => b;\nconsole.log(f({ a: 1, b: 2 }));\n",
			"2\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := runProgramGoTolerant(t, tc.src); got != tc.want {
				t.Fatalf("%s printed %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestDestructuredParamUnusedMemberEmitsBlank proves the unused member is blanked rather
// than dropped: the read still runs and a `_ = a` follows it, so the member is marked used
// the way an unused variable declaration is. A member the body does read takes no blank.
func TestDestructuredParamUnusedMemberEmitsBlank(t *testing.T) {
	source := renderProgram(t, "function f({ a, b }: { a: number, b: number }): number { return b; }\nconsole.log(f({ a: 1, b: 2 }));\n")
	if !strings.Contains(source, "a := __0.A") {
		t.Fatalf("the unused member's read was dropped, want it kept:\n%s", source)
	}
	if !strings.Contains(source, "_ = a") {
		t.Fatalf("the unused member was not blanked, want `_ = a`:\n%s", source)
	}
	if strings.Contains(source, "_ = b") {
		t.Fatalf("the used member should take no blank, but `_ = b` was emitted:\n%s", source)
	}
}

// TestDestructuredParamAllUsedNoBlank proves the blank is withheld when every member is
// read, so a fully-used pattern emits no redundant blank assignment.
func TestDestructuredParamAllUsedNoBlank(t *testing.T) {
	source := renderProgram(t, "function f({ a, b }: { a: number, b: number }): number { return a + b; }\nconsole.log(f({ a: 1, b: 2 }));\n")
	if strings.Contains(source, "_ = a") || strings.Contains(source, "_ = b") {
		t.Fatalf("a fully-used pattern should take no blank:\n%s", source)
	}
}
