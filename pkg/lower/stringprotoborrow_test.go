package lower

import (
	"strings"
	"testing"
)

// TestStringProtoBorrowEmits pins that String.prototype.<m>.call(recv, ...) lowers
// to the value.BStr method run on the receiver coerced with value.ToString, the
// generic-receiver borrow the String pool exercises.
func TestStringProtoBorrowEmits(t *testing.T) {
	const src = "export function b(x: number): string { return String.prototype.slice.call(x, 1, 3); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ToString(") {
		t.Errorf("borrowed slice did not coerce the receiver with ToString:\n%s", source)
	}
	if !strings.Contains(source, ".Slice(1, 3)") {
		t.Errorf("borrowed slice did not lower to the BStr method:\n%s", source)
	}
}

// TestStringProtoBorrowStringReceiver pins the plain string receiver case, the most
// common borrow form: the receiver coerces through ToString all the same.
func TestStringProtoBorrowStringReceiver(t *testing.T) {
	const src = "export function b(s: string): string { return String.prototype.toUpperCase.call(s); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".ToUpperCase()") {
		t.Errorf("borrowed toUpperCase did not lower to the BStr method:\n%s", source)
	}
}

// TestStringProtoBorrowRuns compiles and runs a borrowed string method over a
// number receiver, proving the ToString coercion and the method agree with the
// engine end to end.
func TestStringProtoBorrowRuns(t *testing.T) {
	const src = "const r = String.prototype.charAt.call(12345, 2);\nconsole.log(r);\n"
	out := runProgramGo(t, src)
	if out != "3\n" {
		t.Errorf("borrowed charAt over a number receiver = %q, want %q", out, "3\n")
	}
}
