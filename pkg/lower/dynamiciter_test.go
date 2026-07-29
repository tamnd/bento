package lower

import (
	"strings"
	"testing"
)

// Every for...of path in stmt.go keys off something the checker proved: this is an
// array, this is a string, this is a generator, this class declares [Symbol.iterator].
// A value that came back from a node built-in carries none of that, so it fell through
// them all and handed the build back. These cases pin that such an iterable is walked
// at run time instead, and that the paths which do have a static answer keep it.

// TestForOfOverABoxedIterableDefersToTheRuntime pins the reroute: the emitted loop
// drives value.Iterate rather than ranging a Go slice a box does not have.
func TestForOfOverABoxedIterableDefersToTheRuntime(t *testing.T) {
	got := renderProgram(t, "const os = require('os');\n"+
		"for (const c of os.cpus()) { console.log(1); }\n")
	if !strings.Contains(got, "value.Iterate(") {
		t.Errorf("emitted:\n%s\nwant a loop over value.Iterate", got)
	}
	if strings.Contains(got, ".Elems()") {
		t.Errorf("emitted:\n%s\nwant no range over Elems, which a box does not answer", got)
	}
}

// TestTheIterableIsNamedInTheEmittedCall pins that the source text is passed through.
// It is there only so a non-iterable throws the message Node throws, naming what the
// program wrote, and a runtime handed only the value cannot recover that.
func TestTheIterableIsNamedInTheEmittedCall(t *testing.T) {
	got := renderProgram(t, "const os = require('os');\n"+
		"for (const c of os.cpus()) { console.log(1); }\n")
	if !strings.Contains(got, `value.Iterate(`) || !strings.Contains(got, `"os.cpus()"`) {
		t.Errorf("emitted:\n%s\nwant the iterable's source text handed to value.Iterate", got)
	}
}

// TestALongIterableExpressionIsNotPastedIntoTheMessage pins the trim. Naming the
// expression helps only if it reads at a glance, so an expression too long for a
// one-line error is replaced rather than wrapped across a paragraph.
func TestALongIterableExpressionIsNotPastedIntoTheMessage(t *testing.T) {
	got := renderProgram(t, "const os = require('os');\n"+
		"for (const c of os.cpus().filter(function (x: any) { return x !== null; })) { console.log(1); }\n")
	if !strings.Contains(got, `"the value"`) {
		t.Errorf("emitted:\n%s\nwant a long iterable replaced by a fixed phrase", got)
	}
}

// TestTheLoopVariableOverABoxIsBoxedToo pins the second half of the wall. The binding
// holds whatever the box yielded, so a read off it must dispatch through the value
// model; taking the checker's element type would emit a Go struct field selector the
// box does not carry, which does not compile.
func TestTheLoopVariableOverABoxIsBoxedToo(t *testing.T) {
	got := renderProgram(t, "const os = require('os');\n"+
		"for (const c of os.cpus()) { console.log(c.model); }\n")
	if !strings.Contains(got, ".Get(") {
		t.Errorf("emitted:\n%s\nwant the loop variable read through the dynamic Get", got)
	}
}

// TestSpreadOfABoxDrainsItEagerly pins the expression form: a spread needs every
// element at once rather than a loop it can drive, so it takes IterateToSlice and
// splices the result.
func TestSpreadOfABoxDrainsItEagerly(t *testing.T) {
	got := renderProgram(t, "const os = require('os');\n"+
		"const copy = [...os.cpus()];\n"+
		"console.log(copy.length);\n")
	if !strings.Contains(got, "value.IterateToSlice(") {
		t.Errorf("emitted:\n%s\nwant the spread drained through value.IterateToSlice", got)
	}
}

// TestSpreadOfABoxIntoARestParameterDrainsItToo pins the call-site form, which reaches
// the drain by its own path in calls.go rather than through the array literal.
func TestSpreadOfABoxIntoARestParameterDrainsItToo(t *testing.T) {
	got := renderProgram(t, "function f(...xs: any[]) { return xs.length; }\n"+
		"const a = JSON.parse('[1,2]');\n"+
		"console.log(f(...a));\n")
	if !strings.Contains(got, "value.IterateToSlice(") {
		t.Errorf("emitted:\n%s\nwant the rest spread drained through value.IterateToSlice", got)
	}
}

// TestADestructuringForOfOverABoxTakesTheRuntimeWalk pins the pattern form, which has
// its own path: the pattern binds against the box each step yields.
func TestADestructuringForOfOverABoxTakesTheRuntimeWalk(t *testing.T) {
	got := renderProgram(t, "const o = JSON.parse('{\"a\":1}');\n"+
		"for (const [k, v] of Object.entries(o)) { console.log(k, v); }\n")
	if !strings.Contains(got, "value.Iterate(") {
		t.Errorf("emitted:\n%s\nwant the destructuring loop over value.Iterate", got)
	}
}

// TestAStaticArrayForOfKeepsItsRange pins the guard. The reroute fires only where the
// iterable is a box, so an ordinary array keeps the Go range over its backing slice,
// which is the whole reason the static paths exist.
func TestAStaticArrayForOfKeepsItsRange(t *testing.T) {
	got := renderProgram(t, "const a = [1, 2, 3];\nfor (const x of a) { console.log(x); }\n")
	if strings.Contains(got, "value.Iterate(") {
		t.Errorf("emitted:\n%s\nwant a range over the array, not a runtime walk", got)
	}
}

// TestAStringForOfKeepsItsCodePointRange pins the other guard: a statically known
// string still ranges CodePoints() rather than paying for a runtime kind check.
func TestAStringForOfKeepsItsCodePointRange(t *testing.T) {
	got := renderProgram(t, "const s = 'abc';\nfor (const c of s) { console.log(c); }\n")
	if strings.Contains(got, "value.Iterate(") {
		t.Errorf("emitted:\n%s\nwant a range over CodePoints, not a runtime walk", got)
	}
	if !strings.Contains(got, "CodePoints()") {
		t.Errorf("emitted:\n%s\nwant the string's code-point range kept", got)
	}
}

// TestAGeneratorForOfKeepsItsOwnPath pins that the reroute did not swallow a case that
// already had a better answer. A generator is a coroutine, pulled by its own drive, and
// routing it through the runtime walk would lose that.
func TestAGeneratorForOfKeepsItsOwnPath(t *testing.T) {
	got := renderProgram(t, "function* g() { yield 1; yield 2; }\n"+
		"for (const x of g()) { console.log(x); }\n")
	if strings.Contains(got, "value.Iterate(") {
		t.Errorf("emitted:\n%s\nwant the generator's own drive kept", got)
	}
}
