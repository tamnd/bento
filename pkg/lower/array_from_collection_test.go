package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestArrayFromCollectionCollectsTheSnapshot pins the shape: Array.from over a Map or
// Set collects the same insertion-ordered snapshot slice a spread of that collection
// splices, handed to value.ArrayFrom as a fresh array. A Set and either kind's values()
// read Members or Values, a Map's keys() reads Keys, and a Set's keys() is its members
// again, since a Set's two accessors are the same walk.
func TestArrayFromCollectionCollectsTheSnapshot(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"set",
			"const s = new Set<number>([1, 2]);\n" +
				"export function k(): number { return Array.from(s).length; }\n",
			"value.ArrayFrom(s.Members())",
		},
		{
			"set values",
			"const s = new Set<number>([1, 2]);\n" +
				"export function k(): number { return Array.from(s.values()).length; }\n",
			"value.ArrayFrom(s.Members())",
		},
		{
			"set keys",
			"const s = new Set<number>([1, 2]);\n" +
				"export function k(): number { return Array.from(s.keys()).length; }\n",
			"value.ArrayFrom(s.Members())",
		},
		{
			"map keys",
			"const m = new Map<string, number>([[\"a\", 1]]);\n" +
				"export function k(): number { return Array.from(m.keys()).length; }\n",
			"value.ArrayFrom(m.Keys())",
		},
		{
			"map values",
			"const m = new Map<string, number>([[\"a\", 1]]);\n" +
				"export function k(): number { return Array.from(m.values()).length; }\n",
			"value.ArrayFrom(m.Values())",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("Array.from over a collection did not print %q:\n%s", c.want, got)
			}
		})
	}
}

// TestArrayFromCollectionPairsBuildTheTuple pins the pair-yielding spellings: a Map used
// directly and either kind's entries() yield [key, value], so the collection walks the
// two snapshots together into a slice of the interned tuple the result's element type
// names, the same slice the spread of those spellings splices.
func TestArrayFromCollectionPairsBuildTheTuple(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{
			"map direct",
			"const m = new Map<string, number>([[\"a\", 1]]);\n" +
				"export function k(): number { return Array.from(m).length; }\n",
		},
		{
			"map entries",
			"const m = new Map<string, number>([[\"a\", 1]]);\n" +
				"export function k(): number { return Array.from(m.entries()).length; }\n",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			if !strings.Contains(got, ".Keys()") || !strings.Contains(got, ".Values()") {
				t.Errorf("Array.from over pairs did not read both snapshots:\n%s", got)
			}
			if !strings.Contains(got, "Tuple_str_num{") {
				t.Errorf("Array.from over pairs did not build the interned tuple:\n%s", got)
			}
		})
	}
}

// TestArrayFromCollectionTakesAMapCallback pins that the collected array is the
// callback's receiver: the members come off the collection first and the callback then
// maps that array the way it maps a written one. A callback back to the member's own type
// stays on the array method, one that changes the type takes the free function where both
// Go types are named, and a callback reading the position takes the index-aware variant.
func TestArrayFromCollectionTakesAMapCallback(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"same type",
			"const s = new Set<number>([1, 2]);\n" +
				"export function k(): number { return Array.from(s, (x) => x * 2).length; }\n",
			"value.ArrayFrom(s.Members()).Map(",
		},
		{
			"type changing",
			"const s = new Set<number>([1, 2]);\n" +
				"export function k(): number { return Array.from(s, (x) => `${x}!`).length; }\n",
			"value.MapArray[float64, value.BStr](value.ArrayFrom(s.Members())",
		},
		{
			"index aware",
			"const s = new Set<number>([1, 2]);\n" +
				"export function k(): number { return Array.from(s, (x, i) => x + i).length; }\n",
			"value.ArrayFrom(s.Members()).MapIndex(",
		},
		{
			"map keys",
			"const m = new Map<string, number>([[\"a\", 1]]);\n" +
				"export function k(): number { return Array.from(m.keys(), (v) => v.length).length; }\n",
			"value.MapArray[value.BStr, float64](value.ArrayFrom(m.Keys())",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			if !strings.Contains(got, c.want) {
				t.Errorf("Array.from with a map callback did not print %q:\n%s", c.want, got)
			}
		})
	}
}

// TestArrayFromBoxedCollectionBuildsABoxedArray pins the collection whose member slot the
// boxed-signature pass rewrote: the runtime's snapshot is already a []value.Value, so the
// collection is the append into a fresh slice a spread of it makes, wrapped as the one
// array value every read off the result then dispatches through, whatever concrete array
// type the checker gave the call.
func TestArrayFromBoxedCollectionBuildsABoxedArray(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{}') as Record<string, Row>;\n" +
		"const s = new Set<Row>();\n" +
		"s.add(raw['a']);\n" +
		"export function k(): number { return Array.from(s).length; }\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "value.NewArrayValue(append([]value.Value{}, s.Members()...))") {
		t.Errorf("Array.from over a boxed set did not build a boxed array:\n%s", got)
	}
}

// TestArrayFromBoxedCollectionResultStaysDynamic pins the other half of that: the call's
// result is a box, so a method read off it dispatches through the value model rather than
// reaching for the Go array method the checker's element type would name.
func TestArrayFromBoxedCollectionResultStaysDynamic(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{}') as Record<string, Row>;\n" +
		"const s = new Set<Row>();\n" +
		"s.add(raw['a']);\n" +
		"export function k(): string { return Array.from(s).map((r) => r.tag).join(','); }\n"
	got := renderProgram(t, src)
	if strings.Contains(got, "value.MapArray[") {
		t.Errorf("a map off a boxed collected array took the static array path:\n%s", got)
	}
	if !strings.Contains(got, "value.NewArrayValue(append([]value.Value{}, s.Members()...))") {
		t.Errorf("a map off a boxed collected array did not collect the boxes:\n%s", got)
	}
}

// TestArrayFromGeneratorDrains pins the generator source: Array.from drains the coroutine
// into a slice of its yielded type, the same pull a spread of the generator makes, rather
// than falling through to the array-like walk that reads a length the generator has not
// got.
func TestArrayFromGeneratorDrains(t *testing.T) {
	const src = "function* pair(): Generator<number> { yield 7; yield 8; }\n" +
		"export function k(): number { return Array.from(pair()).length; }\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "value.ArrayFrom(") {
		t.Errorf("Array.from over a generator did not collect a fresh array:\n%s", got)
	}
	if !strings.Contains(got, "var _bt1 []float64") || !strings.Contains(got, ".Next(value.Undefined)") {
		t.Errorf("Array.from over a generator did not drain into a slice of its yield type:\n%s", got)
	}
}

// TestArrayFromCollectionHandsBack pins the boundary this slice leaves: a map callback
// over a collection whose members are boxed has to decide what the callback's parameter
// slot is before it can run, which is the boxed-signature question and its own later
// slice, so it hands back with a named reason rather than mixing a box into a typed
// callback.
func TestArrayFromCollectionHandsBack(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{}') as Record<string, Row>;\n" +
		"const s = new Set<Row>();\n" +
		"s.add(raw['a']);\n" +
		"export function k(): string { return Array.from(s, (r) => r.tag).join(','); }\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "boxed") {
		t.Errorf("hand-back reason %q does not name the boxed members", nyl.Reason)
	}
}
