package build

import (
	"regexp"
	"testing"
)

// The console started as a set of names the lowerer recognized and turned into a
// write: console.log(x) emitted a call to a helper, and nothing else about the
// name existed. A program that read the name rather than calling through it, or
// that called a member with state behind it, did not build.
//
// So the console is now a value as well, an object the runtime builds whose
// members are the same helpers the lowerer emits a direct call to. These tests
// compile and run real programs over both halves and hold their output to what
// Node v24.4.1 printed for the same source.

// pidWarning rewrites the process id in a console warning, which is the only part
// of the line that differs between two runs of the same program.
var pidWarning = regexp.MustCompile(`\(node:\d+\)`)

// TestConsoleStatefulMembers runs the members that carry state between calls. The
// counts, the group indent, and the assertions all came from Node, and the point
// of running them in one program is that the state is what one call leaves for
// the next: the second count reads the first, and every line inside a group is
// indented by the group that opened before it.
func TestConsoleStatefulMembers(t *testing.T) {
	got := pidWarning.ReplaceAllString(buildAndRunFile(t, "main.js",
		"console.count();\n"+
			"console.count();\n"+
			"console.count('a');\n"+
			"console.countReset();\n"+
			"console.count();\n"+
			"console.countReset('nope');\n"+
			"console.group('outer');\n"+
			"console.log('inside');\n"+
			"console.group();\n"+
			"console.log('two\\nlines');\n"+
			"console.groupEnd();\n"+
			"console.groupEnd();\n"+
			"console.groupEnd();\n"+
			"console.log('back');\n"+
			"console.assert(true, 'never');\n"+
			"console.assert(false);\n"+
			"console.assert(false, 'with %d', 7);\n"), "(node:PID)")
	want := "default: 1\n" +
		"default: 2\n" +
		"a: 1\n" +
		"default: 1\n" +
		"(node:PID) Warning: Count for 'nope' does not exist\n" +
		"outer\n" +
		"  inside\n" +
		"    two\n" +
		"    lines\n" +
		"back\n" +
		"Assertion failed\n" +
		"Assertion failed: with 7\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestConsoleAssertWithoutAMessageString pins the assert case whose first
// argument is not a string. Node puts the prefix in front as its own argument
// there rather than gluing it on with a colon, so the value is inspected the way
// a logged value is instead of coerced to "[object Object]".
func TestConsoleAssertWithoutAMessageString(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.assert(false, { a: 1 }, 2);\n"+
			"console.assert(false, 42, 'tail');\n")
	want := "Assertion failed { a: 1 } 2\nAssertion failed 42 tail\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestConsoleDirInspectsItsArgument pins the one place dir and log differ on the
// same value. log prints a string as its own text and dir inspects it, quotes and
// all, which is why dir is its own helper rather than log with one argument.
func TestConsoleDirInspectsItsArgument(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.dir('text');\n"+
			"console.log('text');\n"+
			"console.dir({ a: 1, b: 'x' });\n"+
			"console.dir([1, 'two']);\n")
	want := "'text'\ntext\n{ a: 1, b: 'x' }\n[ 1, 'two' ]\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestConsoleTimeReportsASpan pins that a timer started under a label reports
// under it and is gone after, and that a label with no timer warns rather than
// printing a duration it does not have. The duration itself is a real elapsed
// span, so the test holds the shape of the line rather than its digits.
func TestConsoleTimeReportsASpan(t *testing.T) {
	got := pidWarning.ReplaceAllString(buildAndRunFile(t, "main.js",
		"console.time('t');\n"+
			"console.timeLog('t', 'mark');\n"+
			"console.timeEnd('t');\n"+
			"console.timeEnd('t');\n"), "(node:PID)")
	want := regexp.MustCompile(`^t: [0-9.]+m?s mark\nt: [0-9.]+m?s\n` +
		`\(node:PID\) Warning: No such label 't' for console\.timeEnd\(\)\n$`)
	if !want.MatchString(got) {
		t.Fatalf("want a mark line, a duration line, and the warning, got %q", got)
	}
}

// TestConsoleIsAValue pins the half of the console that is not a call the lowerer
// recognizes. The name reads as the object the runtime holds, an alias bound to
// it reaches the same members, a member pulled off it is a function value of its
// own, and the object is the one globalThis carries, so the identity holds both
// ways round.
func TestConsoleIsAValue(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const c = console;\n"+
			"c.log('through the alias');\n"+
			"const l = console.log;\n"+
			"l('through the member alias');\n"+
			"globalThis.console.log('through globalThis');\n"+
			"console.log(typeof console, console === globalThis.console);\n")
	want := "through the alias\n" +
		"through the member alias\n" +
		"through globalThis\n" +
		"object true\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestConsoleValueMembersCarryTheSameState pins that the two paths to a member
// are one member. A count made through the object continues the tally a direct
// console.count() started, and a group opened through the object indents a line
// the direct helper writes, which they could not do if the object held a second
// copy of the state.
func TestConsoleValueMembersCarryTheSameState(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const c = console;\n"+
			"console.count();\n"+
			"c.count();\n"+
			"c.group('g');\n"+
			"console.log('inside');\n"+
			"console.groupEnd();\n"+
			"c.log('out');\n")
	want := "default: 1\ndefault: 2\ng\n  inside\nout\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
