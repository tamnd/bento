package lower

import (
	"strings"
	"testing"
)

// TestArrayDestructureFromGenerator proves array destructuring off a generator drains
// its coroutine into a value.Array once and reads each target by index: a two-name
// pattern binds the first yields, and a trailing rest gathers the tail.
func TestArrayDestructureFromGenerator(t *testing.T) {
	skipIfShort(t)
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"two names",
			"function* g(): Generator<number> { yield 1; yield 2; yield 3; }\n" +
				"const [a, b] = g();\nconsole.log(a, b);\n",
			"1 2\n",
		},
		{
			"with rest",
			"function* g(): Generator<number> { yield 1; yield 2; yield 3; yield 4; }\n" +
				"const [a, ...rest] = g();\nconsole.log(a, rest.join(\",\"));\n",
			"1 2,3,4\n",
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

// TestArrayDestructureFromSet proves array destructuring off a Set reads its typed
// Members() snapshot in insertion order, binding each target by index.
func TestArrayDestructureFromSet(t *testing.T) {
	skipIfShort(t)
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"number members",
			"const s = new Set<number>([10, 20, 30]);\nconst [a, b] = s;\nconsole.log(a, b);\n",
			"10 20\n",
		},
		{
			"string members",
			"const s = new Set<string>([\"a\", \"b\", \"c\"]);\nconst [x, y] = s;\nconsole.log(x, y);\n",
			"a b\n",
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

// TestBuiltinIterableDestructureShapes pins the lowering shapes: a generator source
// drains through its coroutine Next and a Set source reads Members(), each wrapped in
// value.ArrayFrom so the shared indexed reads apply.
func TestBuiltinIterableDestructureShapes(t *testing.T) {
	gen := renderProgram(t, "function* g(): Generator<number> { yield 1; }\n"+
		"export function k(): number { const [a] = g(); return a; }\n")
	if !strings.Contains(gen, "value.ArrayFrom(") || !strings.Contains(gen, ".Next(value.Undefined)") {
		t.Errorf("generator destructure did not drain the coroutine into an array:\n%s", gen)
	}
	set := renderProgram(t, "export function k(s: Set<number>): number { const [a] = s; return a; }\n")
	if !strings.Contains(set, "value.ArrayFrom(") || !strings.Contains(set, ".Members()") {
		t.Errorf("set destructure did not read Members() into an array:\n%s", set)
	}
}

// TestIterHelperDestructureHandsBack pins the zero-fail edge: an iterator-helper
// result is not a generator and its iterator type is not a protocol shape, so
// destructuring it stays on the non-array handback rather than pull a coroutine.
func TestIterHelperDestructureHandsBack(t *testing.T) {
	const src = "const it = [1, 2, 3].values().map(x => x * 2);\n" +
		"export function k(): number { const [a] = it; return a; }\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "later slice") {
		t.Fatalf("iterator-helper destructure reason = %q, want a later-slice handback", reason)
	}
}
