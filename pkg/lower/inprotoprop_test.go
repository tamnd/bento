package lower

import (
	"strings"
	"testing"
)

// TestInObjectProtoNameHandsBack pins that `"valueOf" in obj` hands back rather than
// emit a wrong answer. Every ordinary object inherits Object.prototype, so the key is
// present, but bento stores a default plain object and a null-proto object with the
// same nil [[Prototype]] slot and installs no Object.prototype methods, so the runtime
// InOperator would answer false, a wrong result. Until a modeled default prototype
// tells the two apart, an `in` test naming an Object.prototype property is a safe
// handback. This is the expressions/in/S8.12.6_A2_T1 shape.
func TestInObjectProtoNameHandsBack(t *testing.T) {
	const src = `var o: any = {};
console.log(String("valueOf" in o));`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "Object.prototype property") {
		t.Fatalf("in on an Object.prototype name did not hand back for that reason: %q", reason)
	}
}

// TestInOwnPropertyStillLowers pins the fix is narrow: an `in` test whose key does not
// name an Object.prototype property still lowers to the runtime InOperator, which
// answers correctly for an own property, so the common `"a" in obj` is untouched.
func TestInOwnPropertyStillLowers(t *testing.T) {
	const src = `var o: any = {a: 1};
console.log(String("a" in o));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.InOperator(") {
		t.Fatalf("in on an ordinary own-name key did not lower to InOperator:\n%s", out)
	}
}
