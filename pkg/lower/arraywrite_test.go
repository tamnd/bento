package lower

import (
	"strings"
	"testing"
)

// TestArrayElementWriteEmits pins that a[i] = v on a general array lowers to the
// array's Set, the store half of the At read.
func TestArrayElementWriteEmits(t *testing.T) {
	const src = "export function put(a: number[], i: number, v: number): void { a[i] = v; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "a.Set(i, v)") {
		t.Errorf("array element write did not lower to Set:\n%s", source)
	}
}

// TestArrayElementWriteLiteralIndex pins the append idiom a[a.length] = v lowers to
// Set with the length expression as the index.
func TestArrayElementWriteLiteralIndex(t *testing.T) {
	const src = "export function push(a: number[], v: number): void { a[a.length] = v; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "a.Set(a.Len(), v)") {
		t.Errorf("append-at-length write did not lower to Set:\n%s", source)
	}
}

// TestArrayElementWriteCoercesValue pins that a string write into a string array
// still routes through Set, so the value reaches the element type rather than
// handing back.
func TestArrayElementWriteCoercesValue(t *testing.T) {
	const src = "export function put(a: string[], i: number): void { a[i] = \"x\"; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Set(i, ") {
		t.Errorf("string array write did not lower to Set:\n%s", source)
	}
}

// TestArrayCompoundElementWriteHandsBack proves a compound element write a[i] += v
// hands back rather than dropping the read half the compound needs.
func TestArrayCompoundElementWriteHandsBack(t *testing.T) {
	const src = "export function add(a: number[], i: number, v: number): void { a[i] += v; }\n"
	reason := renderProgramHandBack(t, src)
	if reason == "" {
		t.Fatal("expected a compound array element write to hand back")
	}
}
