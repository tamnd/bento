package lower

import (
	"strings"
	"testing"
)

// TestDynamicToStringLowers pins that x.toString() on a dynamic receiver lowers
// to the runtime dispatch rather than handing back: the call becomes
// recv.ToStringMethod(), which runs the toString the receiver's prototype
// installs at run time.
func TestDynamicToStringLowers(t *testing.T) {
	src := `let m: any = 42; let s: any = m.toString();`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".ToStringMethod()") {
		t.Fatalf("dynamic .toString() did not lower to ToStringMethod:\n%s", out)
	}
}

// TestNarrowedReceiverToStringLowers pins that toString() on a dynamic local a
// typeof guard narrowed to a kind the accessors do not unbox still lowers to the
// runtime dispatch: the binding holds the bare box, so the call reads through
// ToStringMethod, and since the narrowed call is typed string the box unboxes to
// its BStr with AsString. compareArray in the test262 prelude hits this shape
// with message.toString() inside a typeof message === 'symbol' guard.
func TestNarrowedReceiverToStringLowers(t *testing.T) {
	src := `function f(m: any): void { if (typeof m === "symbol") { m = m.toString(); } }`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".ToStringMethod().AsString()") {
		t.Fatalf("narrowed-receiver .toString() did not lower to ToStringMethod().AsString():\n%s", out)
	}
}

// TestDynamicToStringWithArgHandsBack pins that a dynamic .toString() with an
// argument still hands back: the radix form is a later slice, so lowering it to
// the no-argument helper would drop the argument.
func TestDynamicToStringWithArgHandsBack(t *testing.T) {
	src := `let m: any = 42; let s: any = m.toString(16);`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "argument") {
		t.Fatalf("dynamic .toString(16) handed back for the wrong reason: %q", reason)
	}
}

// TestDynamicToStringRuns builds and runs a dynamic .toString() over each kind
// and checks the result matches the toString that kind's prototype installs.
func TestDynamicToStringRuns(t *testing.T) {
	skipIfShort(t)
	src := `
let n: any = 42;
let b: any = true;
let s: any = "hi";
console.log(n.toString());
console.log(b.toString());
console.log(s.toString());
`
	got := runProgramGo(t, src)
	want := "42\ntrue\nhi\n"
	if got != want {
		t.Fatalf("dynamic toString run mismatch:\n got %q\nwant %q", got, want)
	}
}
