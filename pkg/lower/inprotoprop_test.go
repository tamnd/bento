package lower

import (
	"strings"
	"testing"
)

// TestInObjectProtoMethodLowers pins the capability. `"valueOf" in obj` used to hand
// back, because bento stored a default plain object and a null-proto object with the
// same nil [[Prototype]] slot and installed no Object.prototype methods, so the runtime
// InOperator would have answered false for a name every ordinary object carries. The
// value model answers the methods now, under a gate that reads the end of the receiver's
// prototype chain, so the test lowers and the runtime tells the two receivers apart.
// This is the expressions/in/S8.12.6_A2_T1 shape.
func TestInObjectProtoMethodLowers(t *testing.T) {
	names := []string{
		"valueOf", "toString", "toLocaleString", "hasOwnProperty", "isPrototypeOf",
		"propertyIsEnumerable", "__lookupGetter__", "__lookupSetter__",
		"__defineGetter__", "__defineSetter__",
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			src := "var o: any = {};\nconsole.log(String(\"" + name + "\" in o));"
			out := renderProgram(t, src)
			if !strings.Contains(out, "value.InOperator(") {
				t.Fatalf("in on %q did not lower to InOperator:\n%s", name, out)
			}
		})
	}
}

// TestInUnmodeledProtoSlotHandsBack pins what is left. constructor and __proto__ are
// data slots rather than methods, and their values are objects the value model does not
// build: an Object constructor function and the Object.prototype object itself. An `in`
// test naming one still hands back rather than report an absence the engine contradicts.
func TestInUnmodeledProtoSlotHandsBack(t *testing.T) {
	for _, name := range []string{"constructor", "__proto__"} {
		t.Run(name, func(t *testing.T) {
			src := "var o: any = {};\nconsole.log(String(\"" + name + "\" in o));"
			reason := renderProgramHandBack(t, src)
			if !strings.Contains(reason, "constructor or __proto__") {
				t.Fatalf("in on %q did not hand back for that reason: %q", name, reason)
			}
		})
	}
}

// TestInOwnPropertyStillLowers pins that the common `"a" in obj` is untouched: it
// lowers to the runtime InOperator, which answers correctly for an own property.
func TestInOwnPropertyStillLowers(t *testing.T) {
	const src = `var o: any = {a: 1};
console.log(String("a" in o));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.InOperator(") {
		t.Fatalf("in on an ordinary own-name key did not lower to InOperator:\n%s", out)
	}
}
