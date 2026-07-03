package partition

import "testing"

// realBoxing compiles a snippet and runs escape analysis over it.
func realBoxing(t *testing.T, src string) Boxing {
	t.Helper()
	return New(loadReal(t, src, false)).Boxing()
}

// escapesNamed reports whether any binding named name is in the escaping set. The
// tests give each binding under test a distinct name so a name uniquely identifies
// the binding, which keeps the assertion off the unexported symbol id.
func escapesNamed(b Boxing, name string) bool {
	for sym := range b.Escaping {
		if sym.Name == name {
			return true
		}
	}
	return false
}

// TestBoxingStringifiedObjectEscapes pins the section 13.5 sink: an object handed
// to JSON.stringify is walked dynamically, so it can no longer keep a fixed Go
// layout and must be boxed. dumped is stringified and escapes; kept, an object of
// the same shape that is only read through a typed field access, does not.
func TestBoxingStringifiedObjectEscapes(t *testing.T) {
	src := "export function audit(dumped: { id: string; total: number }): void { console.log(JSON.stringify(dumped)); }\n" +
		"export function summarize(kept: { id: string; total: number }): number { return kept.total; }\n"
	b := realBoxing(t, src)

	if !escapesNamed(b, "dumped") {
		t.Errorf("dumped did not escape, but JSON.stringify walks it dynamically; escaping = %v", b.Escaping)
	}
	if escapesNamed(b, "kept") {
		t.Errorf("kept escaped, but it is only read through a typed field access; escaping = %v", b.Escaping)
	}
}

// TestBoxingDirectStringifyEscapes pins that the sink is detected when the
// stringify is the whole expression, not only when it is nested inside another
// call. rec is returned straight from JSON.stringify and must box.
func TestBoxingDirectStringifyEscapes(t *testing.T) {
	src := "export function dump(rec: { a: number }): string { return JSON.stringify(rec); }\n"
	b := realBoxing(t, src)

	if !escapesNamed(b, "rec") {
		t.Errorf("rec did not escape through a direct JSON.stringify; escaping = %v", b.Escaping)
	}
}

// TestBoxingPrimitiveDoesNotEscape pins that a primitive handed to JSON.stringify
// does not box: primitives are immutable and cross by value with no identity
// concern (section 9.3), so there is nothing to keep monomorphic. n is a number
// and stays out of the set.
func TestBoxingPrimitiveDoesNotEscape(t *testing.T) {
	src := "export function show(n: number): void { console.log(JSON.stringify(n)); }\n"
	b := realBoxing(t, src)

	if escapesNamed(b, "n") {
		t.Errorf("n escaped, but a stringified primitive needs no boxing; escaping = %v", b.Escaping)
	}
}

// TestBoxingNoSinkNoEscape pins that a value which never reaches a sink stays
// monomorphic. rec is only read through a typed field access, so escape analysis
// leaves it out of the boxing set.
func TestBoxingNoSinkNoEscape(t *testing.T) {
	src := "export function priceOf(rec: { a: number; b: number }): number { return rec.a + rec.b; }\n"
	b := realBoxing(t, src)

	if escapesNamed(b, "rec") {
		t.Errorf("rec escaped with no sink in sight; escaping = %v", b.Escaping)
	}
	if len(b.Escaping) != 0 {
		t.Errorf("escaping set is non-empty for a program with no sink: %v", b.Escaping)
	}
}
