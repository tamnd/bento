package build

import "testing"

// Note 352 recorded three walls between a program and the node built-ins it calls.
// This file covers the last of them: walking what a built-in answered. Every other
// for...of path in the lowerer keys off something the checker proved, and a value that
// came back from a built-in carries none of that, so `for (const c of os.cpus())` fell
// through every case and failed the build.
//
// These build and run real binaries. What the loop yields is decided at run time by
// value.Iterate, so a test that only inspected the emitted Go would not show that the
// walk visits the right elements.

// TestForOfOverWhatABuiltinAnsweredWalksIt is the reproducer, `for (const c of
// os.cpus())`, plus a read off the loop variable. The read is the second half of the
// wall: the binding holds a box, so `c.model` has to be a dynamic property read rather
// than the Go struct field selector the checker's element type would suggest.
func TestForOfOverWhatABuiltinAnsweredWalksIt(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const os = require('os');\n"+
			"let n = 0;\n"+
			"let kinds = '';\n"+
			"for (const c of os.cpus()) { n = n + 1; kinds = typeof c.model; }\n"+
			"console.log(n === os.cpus().length);\n"+
			"console.log(kinds);\n")
	if want := "true\nstring\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestForOfOverABoxedArrayVisitsEveryElement pins what the walk yields rather than
// only that it ran, over an array of known contents. The array comes from JSON.parse
// because an array literal takes the lowerer's static path, which is a different
// implementation.
func TestForOfOverABoxedArrayVisitsEveryElement(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = JSON.parse('[1,2,3]');\n"+
			"let sum = 0;\n"+
			"let order = '';\n"+
			"for (const x of a) { sum = sum + x; order = order + x; }\n"+
			"console.log(sum, order);\n"+
			"let first = 0;\n"+
			"for (const x of a) { first = x; break; }\n"+
			"console.log(first);\n"+
			"let s = '';\n"+
			"for (const x of a) { if (x === 2) { continue; } s = s + x; }\n"+
			"console.log(s);\n")
	if want := "6 123\n1\n13\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestForOfOverABoxedStringWalksCodePoints pins that a box holding a string walks its
// characters rather than throwing or walking nothing. The kind is settled at run time,
// so the same emitted loop has to handle it.
func TestForOfOverABoxedStringWalksCodePoints(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const o = JSON.parse('{\"s\":\"abc\"}');\n"+
			"let out = '';\n"+
			"for (const ch of o.s) { out = out + ch + '-'; }\n"+
			"console.log(out);\n")
	if want := "a-b-c-\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestDestructuringForOfOverABoxedSourceBindsBothNames pins the pattern form, which
// takes its own path: the pattern binds against the box each step yields, and every
// name it introduces holds a box in turn. Object.entries over a dynamic object is the
// shape a program that walks a parsed JSON record is written in.
func TestDestructuringForOfOverABoxedSourceBindsBothNames(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const o = JSON.parse('{\"a\":1,\"b\":2}');\n"+
			"let out = '';\n"+
			"for (const [k, v] of Object.entries(o)) { out = out + k + '=' + v + ';'; }\n"+
			"console.log(out);\n"+
			"const os = require('os');\n"+
			"for (const [i, c] of os.cpus().entries()) { console.log(i, typeof c.model); break; }\n")
	if want := "a=1;b=2;\n0 string\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestSpreadingWhatABuiltinAnsweredCopiesIt pins the expression form of the same walk.
// A spread needs every element at once rather than a loop it can drive, so it takes the
// eager drain, and the two have to agree about what the source holds.
func TestSpreadingWhatABuiltinAnsweredCopiesIt(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const os = require('os');\n"+
			"const copy = [...os.cpus()];\n"+
			"console.log(copy.length === os.cpus().length);\n"+
			"const a = JSON.parse('[1,2,3]');\n"+
			"console.log([0, ...a, 9].join(','));\n"+
			"console.log([...a, ...a].length);\n")
	if want := "true\n0,1,2,3,9\n6\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestSpreadingABoxIntoARestParameterPassesEveryElement pins the call-site form. The
// arguments reach a rest parameter one per element, which is what a program forwarding
// what a built-in answered into its own helper does.
func TestSpreadingABoxIntoARestParameterPassesEveryElement(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"function count(...xs) { return xs.length; }\n"+
			"function first(...xs) { return xs[0]; }\n"+
			"const a = JSON.parse('[7,8,9]');\n"+
			"console.log(count(...a), first(...a));\n"+
			"console.log(count(1, ...a));\n")
	if want := "3 7\n4\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestIteratingANonIterableThrowsTheNodeMessage pins the failure case end to end. The
// build succeeds, because what the value turns out to be is not knowable until it runs,
// so the program has to throw the way Node throws, naming the expression it was written
// with rather than describing the box.
func TestIteratingANonIterableThrowsTheNodeMessage(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const o = JSON.parse('{\"n\":5}');\n"+
			"try {\n"+
			"  for (const x of o.n) { console.log(x); }\n"+
			"} catch (e) {\n"+
			"  console.log(e.name, e.message);\n"+
			"}\n")
	if want := "TypeError o.n is not iterable\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
