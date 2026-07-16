package lower

import (
	"strings"
	"testing"
)

// TestCollIterSpreadSplicesAccessor proves a spread of a Map or Set keys()/values()
// call in an array literal splices the runtime's insertion-ordered snapshot slice off
// the receiver: a Map's keys() and values() splice Keys() and Values(), a Set's
// values() splices its de-duplicated Members().
func TestCollIterSpreadSplicesAccessor(t *testing.T) {
	skipIfShort(t)
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"map keys",
			"const m = new Map<string, number>([[\"a\", 1], [\"b\", 2]]);\n" +
				"console.log([...m.keys()].join(\",\"));\n",
			"a,b\n",
		},
		{
			"map values",
			"const m = new Map<string, number>([[\"a\", 1], [\"b\", 2]]);\n" +
				"console.log([...m.values()].join(\",\"));\n",
			"1,2\n",
		},
		{
			"set values dedup",
			"const s = new Set<number>([10, 20, 30, 20]);\n" +
				"console.log([...s.values()].join(\",\"));\n",
			"10,20,30\n",
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

// TestCollIterSpreadIntoRestParam proves a spread of a Map or Set keys()/values() call
// into a rest parameter splices the same accessor snapshot the array literal does.
func TestCollIterSpreadIntoRestParam(t *testing.T) {
	skipIfShort(t)
	const src = "function sum(...xs: number[]): number { let t = 0; for (const x of xs) t += x; return t; }\n" +
		"const m = new Map<string, number>([[\"a\", 1], [\"b\", 2], [\"c\", 3]]);\n" +
		"const s = new Set<number>([10, 20, 30, 20]);\n" +
		"console.log(sum(...m.values()), sum(...s.values()));\n"
	if got := runProgramGo(t, src); got != "6 60\n" {
		t.Fatalf("got %q, want %q", got, "6 60\n")
	}
}

// TestCollIterDestructure proves array destructuring off a Map or Set keys()/values()
// call drains the accessor snapshot into a value.Array once and reads each target by
// index, the trailing rest gathering the tail.
func TestCollIterDestructure(t *testing.T) {
	skipIfShort(t)
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"map keys two names",
			"const m = new Map<string, number>([[\"a\", 1], [\"b\", 2], [\"c\", 3]]);\n" +
				"const [k0, k1] = m.keys();\nconsole.log(k0, k1);\n",
			"a b\n",
		},
		{
			"map values rest",
			"const m = new Map<string, number>([[\"a\", 1], [\"b\", 2], [\"c\", 3]]);\n" +
				"const [v0, ...rest] = m.values();\nconsole.log(v0, rest.join(\",\"));\n",
			"1 2,3\n",
		},
		{
			"set values",
			"const s = new Set<number>([10, 20, 30, 20]);\n" +
				"const [a, b] = s.values();\nconsole.log(a, b);\n",
			"10 20\n",
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

// TestCollIterSpreadEmitsAccessor pins the lowering: a Map values() spread splices
// Values() off the receiver, not the call, wrapped by the array literal's ArrayFrom.
func TestCollIterSpreadEmitsAccessor(t *testing.T) {
	got := renderProgram(t, "const m = new Map<string, number>([[\"a\", 1]]);\n"+
		"export function k(): number { const xs = [...m.values()]; return xs.length; }\n")
	if !strings.Contains(got, ".Values()") {
		t.Errorf("map values spread did not splice the Values() accessor:\n%s", got)
	}
}

// TestCollIterEntriesBuildsTupleSlice pins the lowering: entries() yields [key, value]
// pairs, so spreading it collects the Keys/Values snapshots into a slice of the interned
// tuple the append splices, rather than staying on the old handback.
func TestCollIterEntriesBuildsTupleSlice(t *testing.T) {
	const src = "const m = new Map<string, number>([[\"a\", 1]]);\n" +
		"export function k(): number { const xs = [...m.entries()]; return xs.length; }\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, ".Keys()") || !strings.Contains(got, ".Values()") {
		t.Errorf("map entries spread did not collect the Keys/Values snapshots:\n%s", got)
	}
	if !strings.Contains(got, "Tuple_str_num{") {
		t.Errorf("map entries spread did not build the interned tuple:\n%s", got)
	}
}
