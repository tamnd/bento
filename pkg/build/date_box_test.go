package build

import "testing"

// A date that crosses into a dynamic slot is a run-time surface rather than an
// emission, so what matters is what the built binary prints when a program logs a date,
// serializes one, asserts two are equal, or writes through one it passed somewhere
// untyped. These build real binaries and hold their whole output against what Node
// v24.18.0 prints for the same program. Everything here is written to read the same in
// any time zone, since the machine that runs the suite is not the machine the readings
// were taken on.

// TestABoxedDatePrintsLikeNode covers the rendering: a date names its instant in ISO
// form wherever it is printed, nested or not, and one that names no instant prints the
// human text instead.
func TestABoxedDatePrintsLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const d = new Date(0);\n"+
			"console.log(d);\n"+
			"console.log({ when: d }, [d]);\n"+
			"console.log(new Date(NaN));\n"+
			"console.log(new Map([['k', new Date(0)]]));\n"+
			"console.log(new Set([new Date(0)]));\n")
	want := "1970-01-01T00:00:00.000Z\n" +
		"{ when: 1970-01-01T00:00:00.000Z } [ 1970-01-01T00:00:00.000Z ]\n" +
		"Invalid Date\n" +
		"Map(1) { 'k' => 1970-01-01T00:00:00.000Z }\n" +
		"Set(1) { 1970-01-01T00:00:00.000Z }\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestACollectionOfDatesBoxesEachOne covers the element bridge rather than the date: an
// array, a Map and a Set of dates are typed containers of a typed element, so each one
// has to present its members through the date's own box when it crosses. A member read
// back out of one is the date the container holds, not a copy of it, which is what lets
// a program mutate what it found.
func TestACollectionOfDatesBoxesEachOne(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const list = [new Date(0), new Date(1000)];\n"+
			"console.log(list);\n"+
			"console.log(JSON.stringify(list));\n"+
			"const m = new Map();\n"+
			"m.set('a', new Date(0));\n"+
			"console.log(m.get('a'), m);\n"+
			"const s = new Set([new Date(0)]);\n"+
			"console.log(s.size, s);\n")
	want := "[ 1970-01-01T00:00:00.000Z, 1970-01-01T00:00:01.000Z ]\n" +
		"[\"1970-01-01T00:00:00.000Z\",\"1970-01-01T00:00:01.000Z\"]\n" +
		"1970-01-01T00:00:00.000Z Map(1) { 'a' => 1970-01-01T00:00:00.000Z }\n" +
		"1 Set(1) { 1970-01-01T00:00:00.000Z }\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedDateLooksLikeAnObject covers what a date is from the outside once it is
// dynamic: an object whose own property table is empty, since a date carries its
// members on its prototype and none of its own. Three neighbors of this belong here and
// cannot be written yet, each for a reason that has nothing to do with the box: the
// [object Date] tag needs Object.prototype.toString.call, 'getTime' in d needs the in
// operator outside a union narrowing, and both Object.entries(d) and a bare read of
// d.getTime hit the checker's overloaded-function-type gap on Date's own declarations.
// All three are pinned in pkg/value/datebox_test.go instead.
func TestABoxedDateLooksLikeAnObject(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const d = new Date(0);\n"+
			"console.log(typeof d, Object.keys(d).length, Object.values(d).length);\n")
	want := "object 0 0\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedDateCoercesByHint is the split no other built-in has: a date under + is
// text, and a date under every other operator is its instant. It comes from the date's
// own Symbol.toPrimitive answering the string form for the default hint, so getting it
// wrong would silently turn a concatenation into arithmetic.
func TestABoxedDateCoercesByHint(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const d = new Date(0);\n"+
			"console.log(d + 1 === d.toString() + '1');\n"+
			"console.log(String(d) === d.toString());\n"+
			"console.log(d - 0, d * 1, +d);\n"+
			"console.log(new Date(1000) > new Date(0));\n")
	want := "true\n" +
		"true\n" +
		"0 0 0\n" +
		"true\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedDateSerializesLikeNode covers JSON.stringify, which reaches a date through
// its toJSON hook rather than through its properties: a date has none, so without the
// hook every one of these would write the empty object.
func TestABoxedDateSerializesLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const d = new Date(0);\n"+
			"console.log(JSON.stringify(d));\n"+
			"console.log(JSON.stringify({ a: d, b: [d] }));\n"+
			"console.log(JSON.stringify(new Date(NaN)));\n"+
			"console.log(JSON.stringify({ when: d }, null, 2));\n")
	want := "\"1970-01-01T00:00:00.000Z\"\n" +
		"{\"a\":\"1970-01-01T00:00:00.000Z\",\"b\":[\"1970-01-01T00:00:00.000Z\"]}\n" +
		"null\n" +
		"{\n  \"when\": \"1970-01-01T00:00:00.000Z\"\n}\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAWriteThroughABoxedDateIsSeenByTheTypedDate is the point of boxing a date as a
// view: a date handed to something untyped is the same date, so a write made there is
// visible to the typed code that keeps running afterwards.
func TestAWriteThroughABoxedDateIsSeenByTheTypedDate(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const d = new Date(0);\n"+
			"function bump(x) { x.setUTCFullYear(2000); return x.getTime(); }\n"+
			"console.log(bump(d));\n"+
			"console.log(d.getUTCFullYear(), d.toISOString());\n")
	want := "946684800000\n" +
		"2000 2000-01-01T00:00:00.000Z\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAssertOverDatesMatchesNode covers the comparison a test suite makes. A date is
// its moment, so two built apart are equal when they name the same one and two that
// name none are equal to each other, and the diff a failure prints is part of what a
// program's output is.
func TestAssertOverDatesMatchesNode(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const assert = require('assert');\n"+
			"assert.deepStrictEqual(new Date(0), new Date(0));\n"+
			"assert.deepStrictEqual(new Date(NaN), new Date(NaN));\n"+
			"try {\n"+
			"  assert.deepStrictEqual(new Date(0), new Date(1));\n"+
			"} catch (e) {\n"+
			"  console.log(e.message);\n"+
			"}\n"+
			"console.log('done');\n")
	want := "Expected values to be strictly deep-equal:\n" +
		"+ actual - expected\n" +
		"\n" +
		"+ 1970-01-01T00:00:00.000Z\n" +
		"- 1970-01-01T00:00:00.001Z\n" +
		"\ndone\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
