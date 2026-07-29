package build

import "testing"

// A compiled program printed "[object Object]" for every logged object, which is
// what a string coercion of an object gives and what no one logging an object wants
// to read. console.log does not coerce, it inspects, and the inspector is now the
// real one ported from Node. What a unit test on the value model cannot show is that
// a built binary gets it, so these compile and run real programs and hold their
// stdout to what Node v24.18.0 printed for the same source.

// TestConsoleLogsAnObject is the reproducer. Every expectation here came from
// running the same JavaScript under Node.
func TestConsoleLogsAnObject(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log({ a: 1, b: 'x' });\n"+
			"const o = { n: 1, s: 'two', t: true, u: undefined, z: null };\n"+
			"console.log(o);\n"+
			"console.log({ nested: { deep: { deeper: { gone: 1 } } } });\n"+
			"console.log({ 'a-b': 1, valid_id: 2 });\n")
	want := "{ a: 1, b: 'x' }\n" +
		"{ n: 1, s: 'two', t: true, u: undefined, z: null }\n" +
		"{ nested: { deep: { deeper: [Object] } } }\n" +
		"{ 'a-b': 1, valid_id: 2 }\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestConsoleLogsAnArray pins the array form, brackets and spaces and all. An
// array coerced to a string is "1,2,3", which is not what the console prints.
func TestConsoleLogsAnArray(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log([1, 2, 3]);\n"+
			"const nums = [1, 2, 3];\n"+
			"console.log(nums);\n"+
			"const words = ['a', 'b'];\n"+
			"console.log(words);\n"+
			"console.log([]);\n")
	want := "[ 1, 2, 3 ]\n[ 1, 2, 3 ]\n[ 'a', 'b' ]\n[]\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestConsoleLogsAStringRaw pins the argument the console does not inspect. A
// logged string is its own text, and a program's output would be unreadable if
// every line it printed came back wrapped in quotes.
func TestConsoleLogsAStringRaw(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log('hi');\n"+
			"console.log('a', 'b');\n"+
			"console.log(['quoted here']);\n")
	if want := "hi\na b\n[ 'quoted here' ]\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestConsoleLogsAFunction pins the label a logged function reads as, including
// the name. A function boxed into a dynamic slot used to lose its name, so this
// would have printed "[Function (anonymous)]" for both of the named ones.
func TestConsoleLogsAFunction(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"function foo(a) { return a; }\n"+
			"console.log(foo);\n"+
			"const bar = (a) => a;\n"+
			"console.log(bar);\n")
	if want := "[Function: foo]\n[Function: bar]\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestConsoleLogsASymbol pins the kind that used to throw. A symbol has no string
// coercion by design, so the old console path could not print one at all, while
// Node prints its description.
func TestConsoleLogsASymbol(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log(Symbol('s'));\n"+
			"console.log(Symbol());\n")
	if want := "Symbol(s)\nSymbol()\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestConsoleLogsNegativeZero pins the one number the console prints differently
// from a string coercion. A program logging a zero to find out which zero it got
// used to be told "0" either way.
func TestConsoleLogsNegativeZero(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log(-0);\n"+
			"console.log(0);\n"+
			"console.log([-0]);\n"+
			"console.log(1e21, NaN, Infinity);\n")
	if want := "-0\n0\n[ -0 ]\n1e+21 NaN Infinity\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
