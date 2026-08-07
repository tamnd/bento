package lower

import (
	"strings"
	"testing"
)

// TestOptionalChainEmits pins the shape of a single optional-property link: a?.b
// on a T | undefined receiver lowers to value.OptMap over the receiver optional,
// the mapping function reading the one field, so the member is read only when the
// receiver is present.
func TestOptionalChainEmits(t *testing.T) {
	const src = "interface Pt { x: number; y: number }\n" +
		"export function px(p: Pt | undefined): number { return p?.x ?? 0; }\n"
	source := renderProgram(t, src)
	for _, want := range []string{"value.OptMap(p, func(v *ObjXY) float64 {", "return v.X", ").Or(0)"} {
		if !strings.Contains(source, want) {
			t.Errorf("optional chain did not print %q:\n%s", want, source)
		}
	}
}

// TestOptionalChainMultiLink pins that a?.b?.c nests one OptMap inside the next:
// the inner link produces an Opt the outer link maps over, so a nullish receiver
// anywhere short-circuits the whole chain.
func TestOptionalChainMultiLink(t *testing.T) {
	const src = "interface Inner { v: number }\n" +
		"interface Outer { inner: Inner }\n" +
		"export function deep(o: Outer | undefined): number { return o?.inner?.v ?? 0; }\n"
	source := renderProgram(t, src)
	if n := strings.Count(source, "value.OptMap("); n != 2 {
		t.Errorf("multi-link chain emitted %d OptMap calls, want 2:\n%s", n, source)
	}
}

// TestOptionalChainClassReceiver pins that an optional class instance reads its
// field through the same OptMap, with the pointer instance type as the mapping
// parameter.
func TestOptionalChainClassReceiver(t *testing.T) {
	const src = "class Box { v: number; constructor() { this.v = 1; } }\n" +
		"export function read(b: Box | undefined): number { return b?.v ?? 0; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.OptMap(b, func(v *Box) float64 {") {
		t.Errorf("optional chain on a class receiver did not print the pointer map:\n%s", source)
	}
}

// TestOptionalChainElement pins that the bracketed spelling reaches the same place the
// dotted one does. It used to be the boundary of this slice and hand back, since a?.[i]
// is a different node shape than a property access; it lowers now, and an optional
// receiver still maps over its inner value rather than dispatching at run time.
func TestOptionalChainElement(t *testing.T) {
	const src = "export function first(a: number[] | undefined): number { return (a?.[0]) ?? 0; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.OptMap(a, func(v *value.Array[float64]) float64 {") {
		t.Errorf("optional element read did not print the array map:\n%s", source)
	}
}
