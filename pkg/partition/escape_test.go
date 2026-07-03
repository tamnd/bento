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

// TestBoxingAliasEscapesTransitively pins the closure: a value that escapes
// carries the escape to every binding that aliases it. copy is stringified and
// escapes directly; rec, which copy was declared from, names the same object and
// must box too.
func TestBoxingAliasEscapesTransitively(t *testing.T) {
	src := "export function audit(rec: { id: string; total: number }): void {\n" +
		"  const copy = rec;\n" +
		"  console.log(JSON.stringify(copy));\n" +
		"}\n"
	b := realBoxing(t, src)

	if !escapesNamed(b, "copy") {
		t.Errorf("copy did not escape through JSON.stringify; escaping = %v", b.Escaping)
	}
	if !escapesNamed(b, "rec") {
		t.Errorf("rec did not inherit copy's escape across the alias; escaping = %v", b.Escaping)
	}
}

// TestBoxingAssignmentAliasEscapes pins that an assignment alias carries escape
// the same way a declaration alias does. other is stringified and escapes, and rec
// was assigned into other, so rec names the same object and boxes.
func TestBoxingAssignmentAliasEscapes(t *testing.T) {
	src := "export function audit(rec: { id: string }, other: { id: string }): void {\n" +
		"  other = rec;\n" +
		"  console.log(JSON.stringify(other));\n" +
		"}\n"
	b := realBoxing(t, src)

	if !escapesNamed(b, "rec") {
		t.Errorf("rec did not inherit other's escape across the assignment; escaping = %v", b.Escaping)
	}
}

// TestBoxingUntypedCrossingEscapes pins the boundary-crossing sink beyond the
// reflective walk: an object handed to an any parameter crosses into code that may
// retain or shape-mutate it, so it boxes. order flows into sink's any parameter and
// escapes.
func TestBoxingUntypedCrossingEscapes(t *testing.T) {
	src := "export function sink(x: any): void {}\n" +
		"export function feed(order: { id: string; total: number }): void { sink(order); }\n"
	b := realBoxing(t, src)

	if !escapesNamed(b, "order") {
		t.Errorf("order did not escape crossing into an any parameter; escaping = %v", b.Escaping)
	}
}

// TestBoxingTypedCrossingStaysMonomorphic pins the tight side: an object handed to
// a concretely typed parameter stays monomorphic, because the callee is bound by
// the type and cannot treat it dynamically. order flows only into keep's typed
// parameter and does not escape.
func TestBoxingTypedCrossingStaysMonomorphic(t *testing.T) {
	src := "export function keep(rec: { total: number }): number { return rec.total; }\n" +
		"export function pass(order: { total: number }): number { return keep(order); }\n"
	b := realBoxing(t, src)

	if escapesNamed(b, "order") {
		t.Errorf("order escaped crossing into a typed parameter, but a typed crossing stays monomorphic; escaping = %v", b.Escaping)
	}
}

// TestBoxingEscapesAcrossCallEdge pins the section 13.5 worked example: an object
// handed to a function that JSON.stringifies it escapes at the call site, not only
// inside the callee. au is stringified inside audit and escapes; hd, the argument
// handle passes to audit, inherits that escape across the call edge; sm, summarize's
// parameter, inherits it too because the same hd flows into summarize, so summarize
// must read that instance through the boxed representation.
func TestBoxingEscapesAcrossCallEdge(t *testing.T) {
	src := "interface Order { id: string; total: number }\n" +
		"function summarize(sm: Order): number { return sm.total * 1.1; }\n" +
		"function audit(au: Order): void { console.log(JSON.stringify(au)); }\n" +
		"function handle(hd: Order): number { audit(hd); return summarize(hd); }\n"
	b := realBoxing(t, src)

	for _, name := range []string{"au", "hd", "sm"} {
		if !escapesNamed(b, name) {
			t.Errorf("%s did not escape, but the stringified object reaches it across a call edge; escaping = %v", name, b.Escaping)
		}
	}
}

// TestBoxingUnconnectedObjectStaysMonomorphic pins that the call-edge flow does not
// spill onto a value with no path to a sink. z is only read through a typed field
// access and is passed nowhere, so it stays monomorphic even while an unrelated
// object in the same program escapes.
func TestBoxingUnconnectedObjectStaysMonomorphic(t *testing.T) {
	src := "interface Order { id: string; total: number }\n" +
		"function priceOnly(z: Order): number { return z.total; }\n" +
		"function audit(au: Order): void { console.log(JSON.stringify(au)); }\n"
	b := realBoxing(t, src)

	if escapesNamed(b, "z") {
		t.Errorf("z escaped, but it reaches no sink and is passed nowhere; escaping = %v", b.Escaping)
	}
	if !escapesNamed(b, "au") {
		t.Errorf("au did not escape through JSON.stringify; escaping = %v", b.Escaping)
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
