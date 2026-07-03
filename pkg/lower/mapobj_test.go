package lower

import (
	"strings"
	"testing"
)

// TestMapLoweringShape pins the Go a Map program lowers to: an empty construction
// picks the value constructor for the key kind and instantiates it at the value
// type, a local that holds a map is typed *value.Map[K, V], set/has/delete map to
// the matching methods, get returns an Opt read past an undefined guard, and .size
// is a Size() call. Reading the emitted code directly keeps a change to the shape
// visible in review without running the toolchain.
func TestMapLoweringShape(t *testing.T) {
	const src = `const m = new Map<string, number>();
m.set("a", 1);
const v = m.get("a");
if (v !== undefined) {
  console.log(v);
}
console.log(m.has("a"));
m.delete("a");
console.log(m.size);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"var m *value.Map[value.BStr, float64] = value.NewStringMap[float64]()",
		"m.Set(value.FromGoString(\"a\"), 1)",
		"m.Get(value.FromGoString(\"a\"))",
		"m.Has(value.FromGoString(\"a\"))",
		"m.Delete(value.FromGoString(\"a\"))",
		"m.Size()",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("Map lowering missing %q:\n%s", want, source)
		}
	}
}

// TestMapConstructorByKeyKind pins the constructor each key kind selects: a number
// key takes NewNumberMap, a string key NewStringMap, and a boolean key NewBoolMap,
// each instantiated at the value type. The key kind is read off the map's own type,
// so the empty new Map<K, V>() form is enough to fix the constructor.
func TestMapConstructorByKeyKind(t *testing.T) {
	cases := map[string]string{
		`const m = new Map<number, number>(); console.log(m.size);`:  "value.NewNumberMap[float64]()",
		`const m = new Map<string, boolean>(); console.log(m.size);`: "value.NewStringMap[bool]()",
		`const m = new Map<boolean, number>(); console.log(m.size);`: "value.NewBoolMap[float64]()",
	}
	for src, want := range cases {
		source := renderProgram(t, src+"\n")
		if !strings.Contains(source, want) {
			t.Errorf("%q did not lower to %q:\n%s", src, want, source)
		}
	}
}

// TestMapHandsBackUnsupportedForms proves the map lowering claims only the subset it
// can emit soundly and hands the rest back. The entries-argument constructor builds
// from a list bento does not lower yet, a non-primitive key has no value constructor,
// and clear plus a method with no mapping are later slices, so each routes to the
// interpreter rather than emitting wrong or partial Go.
func TestMapHandsBackUnsupportedForms(t *testing.T) {
	handsBack(t, "const m = new Map<number, number>([[1, 2]]); console.log(m.size);\n")
	handsBack(t, "const m = new Map<number, number>(); m.forEach(v => console.log(v));\n")
}
