package lower

import (
	"strings"
	"testing"
)

// TestSetSpreadSplicesMembers proves a spread of a Set into an array literal splices
// its members in insertion order: the bare spread, a spread mixed with head and tail
// elements, a string-member Set, and the Set's own de-duplication carried through so
// a member added twice appears once.
func TestSetSpreadSplicesMembers(t *testing.T) {
	skipIfShort(t)
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"bare",
			"const s = new Set<number>([1, 2, 3]);\nconst a = [...s];\nconsole.log(a.join(\",\"), a.length);\n",
			"1,2,3 3\n",
		},
		{
			"with head and tail, dedup carried",
			"const s = new Set<number>([1, 2, 2, 3]);\nconst a = [0, ...s, 9];\nconsole.log(a.join(\",\"));\n",
			"0,1,2,3,9\n",
		},
		{
			"string members",
			"const s = new Set<string>([\"a\", \"b\"]);\nconst a = [\"x\", ...s];\nconsole.log(a.join(\",\"));\n",
			"x,a,b\n",
		},
		{
			"empty",
			"const s = new Set<number>();\nconst a = [...s];\nconsole.log(a.length);\n",
			"0\n",
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

// TestSetSpreadIntoRestParam proves a spread of a Set into a rest parameter splices
// its Members() snapshot, so the callee receives the members as the collected rest
// slice, alone and mixed with fixed arguments, with the Set's de-duplication carried.
func TestSetSpreadIntoRestParam(t *testing.T) {
	skipIfShort(t)
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"bare, dedup carried",
			"function sum(...ns: number[]): number { let t = 0; for (const n of ns) t += n; return t; }\n" +
				"const s = new Set<number>([1, 2, 3, 3]);\nconsole.log(sum(...s));\n",
			"6\n",
		},
		{
			"mixed with fixed args",
			"function sum(...ns: number[]): number { let t = 0; for (const n of ns) t += n; return t; }\n" +
				"const s = new Set<number>([1, 2, 3]);\nconsole.log(sum(10, ...s, 20));\n",
			"36\n",
		},
		{
			"string members",
			"function join(...ss: string[]): string { return ss.join(\"-\"); }\n" +
				"const s = new Set<string>([\"a\", \"b\"]);\nconsole.log(join(\"x\", ...s));\n",
			"x-a-b\n",
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

// TestSetSpreadEmitsMembers pins the lowering shape: a spread of a Set splices its
// Members() slice, the typed insertion-ordered snapshot, rather than route through a
// per-element drain.
func TestSetSpreadEmitsMembers(t *testing.T) {
	const src = "export function k(s: Set<number>): number[] { return [...s]; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Members()") {
		t.Errorf("set spread did not splice Members():\n%s", source)
	}
}
