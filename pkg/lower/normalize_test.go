package lower

import (
	"strings"
	"testing"
)

// TestNormalizeDefaultEmits pins that s.normalize() with no form argument lowers
// to a no-argument value.BStr.Normalize call, which defaults to NFC at runtime.
func TestNormalizeDefaultEmits(t *testing.T) {
	const src = "export function n(s: string): string { return s.normalize(); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "s.Normalize()") {
		t.Errorf("normalize did not lower to the BStr method:\n%s", source)
	}
}

// TestNormalizeFormEmits pins that s.normalize(form) forwards the form name to the
// value.BStr method, which validates it and throws a RangeError on a bad name.
func TestNormalizeFormEmits(t *testing.T) {
	const src = "export function n(s: string): string { return s.normalize(\"NFKD\"); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "s.Normalize(") || !strings.Contains(source, "NFKD") {
		t.Errorf("normalize did not forward the form to the BStr method:\n%s", source)
	}
}

// TestNormalizeNumberFormHandsBack proves a non-string form argument is not
// lowerable: normalize takes a string, so a number form is the wrong kind and the
// unit hands back to the interpreter rather than emitting a mistyped call.
func TestNormalizeNumberFormHandsBack(t *testing.T) {
	const src = "export function n(s: string): string { return s.normalize(1 as any); }\n"
	reason := renderProgramHandBack(t, src)
	if reason == "" {
		t.Fatal("expected normalize with a number form to hand back")
	}
}
