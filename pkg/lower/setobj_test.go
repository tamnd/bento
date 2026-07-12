package lower

import (
	"strings"
	"testing"
)

// TestSetLoweringShape pins the Go a Set program lowers to: an empty construction
// picks the value constructor for the member kind, a local that holds a set is typed
// *value.Set[T], add/has/delete map to the matching methods, and .size is a Size()
// call. Reading the emitted code directly keeps a change to the shape visible in
// review without running the toolchain.
func TestSetLoweringShape(t *testing.T) {
	const src = `const s = new Set<string>();
s.add("a");
console.log(s.has("a"));
s.delete("a");
console.log(s.size);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"s := value.NewStringSet()",
		"s.Add(value.FromGoString(\"a\"))",
		"s.Has(value.FromGoString(\"a\"))",
		"s.Delete(value.FromGoString(\"a\"))",
		"s.Size()",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("Set lowering missing %q:\n%s", want, source)
		}
	}
}

// TestSetConstructorByMemberKind pins the constructor each member kind selects: a
// number member takes NewNumberSet, a string member NewStringSet, and a boolean
// member NewBoolSet. The member kind is read off the set's own type, so the empty
// new Set<T>() form is enough to fix the constructor, and unlike a Map the call
// carries no type argument because the member type fully determines it.
func TestSetConstructorByMemberKind(t *testing.T) {
	cases := map[string]string{
		`const s = new Set<number>(); console.log(s.size);`:  "value.NewNumberSet()",
		`const s = new Set<string>(); console.log(s.size);`:  "value.NewStringSet()",
		`const s = new Set<boolean>(); console.log(s.size);`: "value.NewBoolSet()",
	}
	for src, want := range cases {
		source := renderProgram(t, src+"\n")
		if !strings.Contains(source, want) {
			t.Errorf("%q did not lower to %q:\n%s", src, want, source)
		}
	}
}

// TestSetHandsBackUnsupportedForms proves the set lowering claims only the subset it
// can emit soundly and hands the rest back. The iterable-argument constructor drains
// an array's backing slice or a user iterable, but a built-in Set or a string source
// needs its own walk a later slice brings, so each routes to the interpreter rather
// than emitting wrong or partial Go.
func TestSetHandsBackUnsupportedForms(t *testing.T) {
	handsBack(t, "const a = new Set<number>(); const s = new Set<number>(a); console.log(s.size);\n")
	handsBack(t, "const s = new Set<string>(\"ab\"); console.log(s.size);\n")
}
