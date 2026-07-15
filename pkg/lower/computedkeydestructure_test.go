package lower

import (
	"strings"
	"testing"
)

// An object destructuring computed key over a fixed-shape source reads a slot by a key
// the pattern names with brackets, `const { [k]: v } = o`. When k is a const the checker
// gave a literal string type, the key folds to that string at compile time, so the element
// reads the source's Go struct field the same way a named property `const { a: v } = o`
// does. The underlying static string-keyed element access `o[k]` folds the same way, both
// through the one pureConstStringKey helper, so a bracket read and a computed-key
// destructuring over a fixed shape agree on the field they select.

func TestComputedKeyDestructureReadsField(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const o = { a: 1, b: 2 }; const k = "b"; const { [k]: v } = o; console.log(v);`); got != "2\n" {
		t.Fatalf("const { [k]: v } = o, k=\"b\" = %q, want 2", got)
	}
}

func TestComputedKeyDestructureFirstSlot(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const o = { a: 1, b: 2 }; const k = "a"; const { [k]: v } = o; console.log(v);`); got != "1\n" {
		t.Fatalf("const { [k]: v } = o, k=\"a\" = %q, want 1", got)
	}
}

// A string-literal computed key names the slot directly and has no binding behind it, so
// it folds without needing the const-binding path.
func TestStringLiteralComputedKeyDestructure(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const o = { a: 5 }; const { ["a"]: v } = o; console.log(v);`); got != "5\n" {
		t.Fatalf(`const { ["a"]: v } = o = %q, want 5`, got)
	}
}

// A computed key sits beside named properties in the same pattern, and each reads its own
// folded field.
func TestComputedKeyDestructureMixedWithNamed(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const o = { a: 1, b: 2 }; const k = "b"; const { a, [k]: v } = o; console.log(a, v);`); got != "1 2\n" {
		t.Fatalf("const { a, [k]: v } = o = %q, want 1 2", got)
	}
}

// The static string-keyed element access the computed key rides folds a const key to the
// same field selector on its own, `const k = "a"; o[k]` reading o.a.
func TestStaticStringKeyedElementAccess(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const o = { a: 1, b: 2 }; const k = "a"; console.log(o[k]);`); got != "1\n" {
		t.Fatalf("const k = \"a\"; o[k] = %q, want 1", got)
	}
}

// The fold emits a Go struct-field selector, not a run-time dynamic read, so no boxed key
// or GetElem call reaches the emit for a fixed-shape source.
func TestComputedKeyDestructureEmitsFieldSelector(t *testing.T) {
	got := renderProgramTolerant(t, `const o = { a: 1, b: 2 }; const k = "b"; const { [k]: v } = o; console.log(v);`)
	if !strings.Contains(got, "v := o.B") {
		t.Fatalf("computed key did not fold to a field selector:\n%s", got)
	}
	if strings.Contains(got, "GetElem") {
		t.Fatalf("a fixed-shape computed key must not route through GetElem:\n%s", got)
	}
}

// The const the key folded away is the binding's only use, so it would be declared and not
// used in Go; the elided-read tally blanks it, and the program compiles and runs. A const
// still read elsewhere keeps its use and takes no blank, so `console.log(v, k)` compiles.
func TestComputedKeyFoldedConstStillCompiles(t *testing.T) {
	skipIfShort(t)
	if got := runProgramGoTolerant(t, `const o = { a: 1, b: 2 }; const k = "b"; const { [k]: v } = o; console.log(v, k);`); got != "2 b\n" {
		t.Fatalf("const still read elsewhere = %q, want 2 b", got)
	}
}

// A computed key whose expression runs a side effect, `[(n++, "a")]`, must not fold even
// though the checker types it the literal "a": folding would drop the n++ the language
// runs. It hands back so the side effect is not silently lost.
func TestComputedKeySideEffectHandsBack(t *testing.T) {
	reason := renderProgramTolerantHandBack(t, `const o = { a: 1 }; let n = 0; const { [(n++, "a")]: v } = o; console.log(v);`)
	if !strings.Contains(reason, "run time") && !strings.Contains(reason, "later slice") {
		t.Fatalf("side-effecting key reason = %q, want a handback", reason)
	}
}

// The same purity guard covers the bare element access: `o[(n++, "a")]` keeps its side
// effect and hands back rather than fold to o.a.
func TestStaticStringKeyedElementAccessSideEffectHandsBack(t *testing.T) {
	reason := renderProgramTolerantHandBack(t, `const o = { a: 1 }; let n = 0; console.log(o[(n++, "a")]);`)
	if !strings.Contains(reason, "later slice") {
		t.Fatalf("side-effecting element access reason = %q, want a handback", reason)
	}
}
