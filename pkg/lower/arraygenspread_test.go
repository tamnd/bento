package lower

import (
	"strings"
	"testing"
)

// TestGeneratorSpreadDrainsCoroutine proves a spread of a generator into an array
// literal drains its coroutine into the array: the yielded values land in order, a
// spread mixes with head and tail elements, and two spreads of the same source each
// run a fresh coroutine rather than share one exhausted run.
func TestGeneratorSpreadDrainsCoroutine(t *testing.T) {
	skipIfShort(t)
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"bare",
			"function* g(): Generator<number> { yield 1; yield 2; yield 3; }\n" +
				"const a = [...g()];\nconsole.log(a.join(\",\"));\n",
			"1,2,3\n",
		},
		{
			"with head and tail",
			"function* g(): Generator<number> { yield 2; yield 3; }\n" +
				"const a = [1, ...g(), 4];\nconsole.log(a.join(\",\"));\n",
			"1,2,3,4\n",
		},
		{
			"string element",
			"function* g(): Generator<string> { yield \"a\"; yield \"b\"; }\n" +
				"const a = [\"x\", ...g()];\nconsole.log(a.join(\",\"));\n",
			"x,a,b\n",
		},
		{
			"two spreads run fresh coroutines",
			"function* g(): Generator<number> { yield 1; yield 2; }\n" +
				"const a = [...g(), ...g()];\nconsole.log(a.join(\",\"));\n",
			"1,2,1,2\n",
		},
		{
			"delegation flows through",
			"function* inner(): Generator<number> { yield 1; yield 2; }\n" +
				"function* g(): Generator<number> { yield* inner(); yield 3; }\n" +
				"const a = [...g()];\nconsole.log(a.join(\",\"));\n",
			"1,2,3\n",
		},
		{
			"generator held in a var",
			"function* g(): Generator<number> { yield 5; yield 6; }\n" +
				"const it = g();\nconst a = [...it];\nconsole.log(a.join(\",\"));\n",
			"5,6\n",
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

// TestGeneratorSpreadIntoRestParam proves a spread of a generator into a rest
// parameter drains the same coroutine, so the callee receives the yielded values as
// the collected rest slice, alone and mixed with fixed arguments.
func TestGeneratorSpreadIntoRestParam(t *testing.T) {
	skipIfShort(t)
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"bare",
			"function sum(...ns: number[]): number { let t = 0; for (const n of ns) t += n; return t; }\n" +
				"function* g(): Generator<number> { yield 1; yield 2; yield 3; }\n" +
				"console.log(sum(...g()));\n",
			"6\n",
		},
		{
			"mixed with fixed args",
			"function sum(...ns: number[]): number { let t = 0; for (const n of ns) t += n; return t; }\n" +
				"function* g(): Generator<number> { yield 1; yield 2; yield 3; }\n" +
				"console.log(sum(10, ...g(), 20));\n",
			"36\n",
		},
		{
			"string element",
			"function join(...ss: string[]): string { return ss.join(\"-\"); }\n" +
				"function* g(): Generator<string> { yield \"a\"; yield \"b\"; }\n" +
				"console.log(join(\"x\", ...g()));\n",
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

// TestGeneratorSpreadEmitsDrain pins the lowering shape: a spread of a generator
// pulls its Next(value.Undefined) until done and appends the yields, so the emitted
// output carries the coroutine pull rather than an Elems splice.
func TestGeneratorSpreadEmitsDrain(t *testing.T) {
	const src = "function* g(): Generator<number> { yield 1; }\n" +
		"export function k(): number[] { return [...g()]; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Next(value.Undefined)") {
		t.Errorf("generator spread did not pull the coroutine with Next:\n%s", source)
	}
}

// TestGeneratorSpreadIterHelperHandsBack pins the zero-fail edge: an iterator-helper
// result (a *value.IterHelper, whose Next takes no sent value) is not a generator, so
// spreading it stays on the non-array handback rather than emit a Next call it does
// not answer.
func TestGeneratorSpreadIterHelperHandsBack(t *testing.T) {
	const src = "const it = [1, 2, 3].values().map(x => x * 2);\n" +
		"export function k(): number[] { return [...it]; }\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "later slice") {
		t.Fatalf("iterator-helper spread reason = %q, want a later-slice handback", reason)
	}
}
