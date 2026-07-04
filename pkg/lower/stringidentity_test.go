package lower

import (
	"strings"
	"testing"
)

// TestStringToStringIsIdentity pins that s.toString() on a string lowers to the
// receiver itself with no method call, since toString on a string returns the
// string unchanged.
func TestStringToStringIsIdentity(t *testing.T) {
	const src = "export function id(s: string): string { return s.toString(); }\n"
	source := renderProgram(t, src)
	if strings.Contains(source, ".toString()") || strings.Contains(source, ".ToString()") {
		t.Errorf("toString on a string should lower to the receiver, not a call:\n%s", source)
	}
	if !strings.Contains(source, "return s") {
		t.Errorf("toString on a string should lower to the receiver:\n%s", source)
	}
}

// TestStringValueOfIsIdentity pins that s.valueOf() on a string lowers to the
// receiver itself, since valueOf on a string returns the string unchanged.
func TestStringValueOfIsIdentity(t *testing.T) {
	const src = "export function id(s: string): string { return s.valueOf(); }\n"
	source := renderProgram(t, src)
	if strings.Contains(source, ".valueOf()") || strings.Contains(source, ".ValueOf()") {
		t.Errorf("valueOf on a string should lower to the receiver, not a call:\n%s", source)
	}
	if !strings.Contains(source, "return s") {
		t.Errorf("valueOf on a string should lower to the receiver:\n%s", source)
	}
}

// TestStringToStringEvaluatesReceiverOnce proves the receiver is lowered as the
// whole expression, so a chained call keeps its earlier method and only the
// trailing identity call disappears.
func TestStringToStringEvaluatesReceiverOnce(t *testing.T) {
	const src = "export function up(s: string): string { return s.toUpperCase().toString(); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "s.ToUpperCase()") {
		t.Errorf("chained identity call dropped the earlier method:\n%s", source)
	}
	if strings.Contains(source, ".ToString()") {
		t.Errorf("trailing toString should have been dropped:\n%s", source)
	}
}
