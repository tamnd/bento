package lower

import (
	"strings"
	"testing"
)

// A built-in required with require('<name>') has no declaration file behind it, so
// the checker gives the module binding no usable type and the lowerer lands it in a
// value.Value slot. Everything read off that binding is in the same position: the
// checker has no type for it either, and asking for one used to fail the whole
// build with "no type at this position (void or statement)". These cases pin the
// rule that took its place, that a binding fed by a read or a call off a boxed
// module binding is itself a box.
//
// The construct is not an edge case. `const os = require('os')` followed by
// `const cpus = os.cpus()` is how every CommonJS program that uses a built-in is
// written, and until this rule landed the second line would not compile.

// TestBindingOffARequiredBuiltinIsABox pins that each of these lowers at all, since
// renderProgram fails on a hand-back and every one of them handed back before the
// rule landed, and that the binding takes the runtime read straight, so its Go type
// is inferred from the box rather than named off a checker type there is none of.
// The chains cover a plain call, a data property read, an index step, and a long
// chain, since the rule walks the whole chain down to its root rather than looking
// only at the last step.
//
// Either spelling of the runtime read counts. A data property is a Get on the module
// box, and a method call is the one receiver-preserving CallMethod, which is the same
// read with the module threaded through as the receiver (ctorfunc.go).
func TestBindingOffARequiredBuiltinIsABox(t *testing.T) {
	for _, init := range []string{
		"os.platform()",
		"os.totalmem()",
		"os.EOL",
		"os.cpus()[0]",
		"os.cpus()[0].times.idle",
		"os.userInfo().homedir",
	} {
		t.Run(init, func(t *testing.T) {
			got := renderProgram(t, "const os = require('os');\nconst v = "+init+";\nconsole.log(typeof v);\n")
			if !strings.Contains(got, "v := os.Get(") && !strings.Contains(got, "v := value.CallMethod(os, ") {
				t.Errorf("const v = %s emitted:\n%s\nwant v bound straight to the runtime read", init, got)
			}
		})
	}
}

// TestBindingOffARequiredBuiltinStaysDynamic pins the second half of the rule: the
// new binding is marked dynamic, so its own later reads route through the value
// model too. Without the mark the binding would hold a box while its reads reached
// for a Go struct field the box does not carry, which the Go compiler rejects with
// the reason lost. The check is that a read off the second binding emits a runtime
// Get rather than a Go selector.
func TestBindingOffARequiredBuiltinStaysDynamic(t *testing.T) {
	got := renderProgram(t, "const os = require('os');\nconst u = os.userInfo();\nconsole.log(u.homedir);\n")
	if !strings.Contains(got, ".Get(") {
		t.Errorf("a read off a binding taken from a required built-in emitted:\n%s\nwant a runtime Get", got)
	}
}

// TestBindingOffATypedReceiverKeepsItsType pins the guard. The rule fires on the
// flagless type the checker reports for a read off a box, so a binding fed by a
// receiver the checker does have a type for must be untouched: it keeps its own Go
// slot, and boxing it would cost every later use of it the static path. A local
// array's length is the plain case, a number.
func TestBindingOffATypedReceiverKeepsItsType(t *testing.T) {
	got := renderProgram(t, "const xs = [1, 2, 3];\nconst n = xs.length;\nconsole.log(n);\n")
	if strings.Contains(got, "var n value.Value") {
		t.Errorf("const n = xs.length emitted:\n%s\nwant a number slot, not a box", got)
	}
}
