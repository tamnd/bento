package lower

import (
	"strings"
	"testing"
)

// TestWeakMapLoweringShape pins the Go a WeakMap program lowers to: construction picks
// NewWeakMap over the key pointee and the value type, a local that holds one is typed
// *value.WeakMap[T, V], and get, set, has, and delete map to the matching methods with
// the object key passed as the struct pointer it lowers to. Reading the emitted code
// directly keeps a change to the shape visible in review without running the toolchain.
func TestWeakMapLoweringShape(t *testing.T) {
	const src = `interface Box { id: number }
const k: Box = { id: 1 };
const m = new WeakMap<Box, number>();
m.set(k, 1);
const v = m.get(k);
if (v !== undefined) {
  console.log(v);
}
console.log(m.has(k));
m.delete(k);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.NewWeakMap[ObjId, float64]()",
		"m.Set(k, 1)",
		"m.Get(k)",
		"m.Has(k)",
		"m.Delete(k)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("WeakMap lowering missing %q:\n%s", want, source)
		}
	}
}

// TestWeakMapHandsBackUnsupportedForms proves the WeakMap lowering claims only the
// subset it can emit soundly and hands the rest back. Construction from an iterable of
// entry pairs needs each key built as an object first, which the iterable-drain slice
// brings, and a key whose render is not a pointer is not a weak key, so each routes to
// the interpreter rather than emitting wrong or partial Go.
func TestWeakMapHandsBackUnsupportedForms(t *testing.T) {
	handsBack(t, "interface Box { id: number }\nconst k: Box = { id: 1 };\nconst m = new WeakMap<Box, number>([[k, 1]]);\nconsole.log(m.has(k));\n")
}
