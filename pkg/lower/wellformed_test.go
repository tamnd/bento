package lower

import (
	"strings"
	"testing"
)

// TestIsWellFormedEmits pins that s.isWellFormed() lowers to the value.BStr method
// of the same name, a no-argument boolean method dispatched like trim.
func TestIsWellFormedEmits(t *testing.T) {
	const src = "export function ok(s: string): boolean { return s.isWellFormed(); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "s.IsWellFormed()") {
		t.Errorf("isWellFormed did not lower to the BStr method:\n%s", source)
	}
}

// TestToWellFormedEmits pins that s.toWellFormed() lowers to the value.BStr method,
// a no-argument string-returning method.
func TestToWellFormedEmits(t *testing.T) {
	const src = "export function fix(s: string): string { return s.toWellFormed(); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "s.ToWellFormed()") {
		t.Errorf("toWellFormed did not lower to the BStr method:\n%s", source)
	}
}
