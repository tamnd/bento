package lower

import (
	"strings"
	"testing"
)

// TestUntypedMapHoldsEveryKeyKind is the JavaScript spelling of a Map: nothing
// narrows the key, so one map holds a string, a number, and an object key at once and
// each is found again by SameValueZero. A missing key reads undefined.
func TestUntypedMapHoldsEveryKeyKind(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const m = new Map();
const key = {};
key.id = 7;
m.set("a", 1);
m.set(2, "two");
m.set(key, true);
console.log(m.size, m.get("a"), m.get(2), m.get(key), m.get("missing"));
`))
	want := "3 1 two true undefined\n"
	if got != want {
		t.Errorf("untyped map\n got: %q\nwant: %q", got, want)
	}
}

// TestAnObjectKeyMatchesByIdentity is the case that ruled out boxing an argument that
// is already a box. Building a second box for the same object would give the map a key
// nothing matches, so the get that follows the set would miss.
func TestAnObjectKeyMatchesByIdentity(t *testing.T) {
	source := renderExpandoJS(t, `const m = new Map();
const key = {};
key.id = 1;
m.set(key, "v");
console.log(m.get(key));
`)
	if strings.Contains(source, "ObjectFromStruct") {
		t.Errorf("an object key was boxed again on its way into the map:\n%s", source)
	}
	if got := goRunSource(t, source); got != "v\n" {
		t.Errorf("object key\n got: %q\nwant: %q", got, "v\n")
	}
}

// TestUntypedMapDeletesAndIterates pins the rest of the surface a JavaScript module
// reaches for, and that iteration still runs in insertion order after a delete.
func TestUntypedMapDeletesAndIterates(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const m = new Map();
m.set("a", 1);
m.set("b", 2);
m.set("c", 3);
console.log(m.has("b"), m.delete("b"), m.has("b"));
for (const [k, v] of m) {
  console.log(k, v);
}
`))
	want := "true true false\na 1\nc 3\n"
	if got != want {
		t.Errorf("delete and iterate\n got: %q\nwant: %q", got, want)
	}
}

// TestUntypedSetComparesBySameValueZero pins the Set half. 1 and "1" are different
// members, a repeated member is not added twice, and NaN is a single member, which is
// the one place SameValueZero and === disagree.
func TestUntypedSetComparesBySameValueZero(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `const s = new Set();
s.add(1);
s.add("1");
s.add(1);
s.add(NaN);
s.add(NaN);
console.log(s.size, s.has("1"), s.has(9), s.has(NaN));
`))
	want := "3 true false true\n"
	if got != want {
		t.Errorf("untyped set\n got: %q\nwant: %q", got, want)
	}
}

// TestATypedMapKeepsItsMonomorphicKey pins the boundary. The dynamic constructor is
// for a key type nothing narrowed; a Map the program did type still keys on the Go
// value directly, with no boxing on its entries.
func TestATypedMapKeepsItsMonomorphicKey(t *testing.T) {
	source := renderProgram(t, `const m = new Map<string, number>();
m.set("a", 1);
console.log(m.get("a"));
`)
	if strings.Contains(source, "NewDynMap") {
		t.Errorf("a typed map took the dynamic constructor:\n%s", source)
	}
	if !strings.Contains(source, "NewStringMap") {
		t.Errorf("a string-keyed map did not take the string constructor:\n%s", source)
	}
}
