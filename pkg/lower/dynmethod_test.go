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

// TestDynamicToStringWithArgRuns pins the other half: a dynamic .toString() carrying
// an argument does not go through ToStringMethod, which is the no-argument
// Object.prototype dispatch and has nowhere to put one. It takes the generic dynamic
// path instead, reading toString off the receiver and invoking it with what the call
// passed, so the radix form answers what the receiver's own prototype says. This shape
// pinned a handback while nothing on a primitive answered such a read.
func TestDynamicToStringWithArgRuns(t *testing.T) {
	skipIfShort(t)
	src := `
let m: any = 255;
console.log(m.toString(16));
console.log(m.toString(2));
let s: any = "abc";
console.log(s.toString());
`
	got := runProgramGo(t, src)
	want := "ff\n11111111\nabc\n"
	if got != want {
		t.Fatalf("dynamic toString with a radix run mismatch:\n got %q\nwant %q", got, want)
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

// TestConsoleLogSymbolToStringUnboxes pins that console.log(sym.toString()) over a
// symbol prints the string toString gives without wrapping it in value.ConsoleValue.
// The symbol binding is a boxed value, so callOfDynamicMember once claimed the call
// produced a box and routed the console argument through ConsoleValue, but the
// toString lowering unboxes a known-kind receiver to its concrete value.BStr, which
// ConsoleValue, wanting a value.Value, cannot take: the emitted Go did not compile.
// The receiver kind is known, so the call is a bstr, and console.log takes it direct.
func TestConsoleLogSymbolToStringUnboxes(t *testing.T) {
	src := `const a = Symbol("hello"); console.log(a.toString());`
	out := renderProgram(t, src)
	if strings.Contains(out, "ConsoleValue(") {
		t.Fatalf("console.log(symbol.toString()) wrapped a bstr in value.ConsoleValue:\n%s", out)
	}
	if !strings.Contains(out, ".ToStringMethod().AsString()") {
		t.Fatalf("symbol .toString() did not unbox to ToStringMethod().AsString():\n%s", out)
	}
}

// TestConsoleLogSymbolToStringRuns builds and runs the same shape end to end, proving
// the emitted Go compiles now that the console argument is the unboxed bstr.
func TestConsoleLogSymbolToStringRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const a = Symbol("hello");
const b = Symbol();
console.log(a.toString());
console.log(b.toString());
`
	got := runProgramGo(t, src)
	want := "Symbol(hello)\nSymbol()\n"
	if got != want {
		t.Fatalf("console.log(symbol.toString()) run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestDynamicValueOfLowers pins that x.valueOf() on a dynamic receiver lowers to the
// runtime dispatch rather than handing back: the call becomes recv.ValueOfMethod(),
// which returns the receiver value the way Object.prototype.valueOf and the primitive
// wrappers do. The result stays boxed, since valueOf on an any receiver is itself any.
func TestDynamicValueOfLowers(t *testing.T) {
	src := `let m: any = 42; let v: any = m.valueOf();`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".ValueOfMethod()") {
		t.Fatalf("dynamic .valueOf() did not lower to ValueOfMethod:\n%s", out)
	}
	if strings.Contains(out, ".ValueOfMethod().As") {
		t.Fatalf("dynamic .valueOf() should keep the boxed result, not unbox:\n%s", out)
	}
}

// TestDynamicValueOfWithArgRuns is the valueOf half of the same rule. ValueOfMethod is
// the no-argument dispatch, so a call carrying an argument reads valueOf off the
// receiver and invokes it, and Number.prototype.valueOf ignores what it was passed and
// answers the number, which is what the language says too.
func TestDynamicValueOfWithArgRuns(t *testing.T) {
	skipIfShort(t)
	src := `
let m: any = 42;
console.log(m.valueOf(1));
console.log((m.valueOf(1) as number) + 8);
`
	got := runProgramGo(t, src)
	want := "42\n50\n"
	if got != want {
		t.Fatalf("dynamic valueOf with an argument run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestDynamicValueOfRuns builds and runs a dynamic .valueOf() over each primitive kind
// and an object, checking the result is the receiver value unchanged, then reads it
// back through a following operation to prove the box is usable.
func TestDynamicValueOfRuns(t *testing.T) {
	skipIfShort(t)
	src := `
let n: any = 42;
let b: any = true;
let s: any = "hi";
console.log(n.valueOf());
console.log(b.valueOf());
console.log(s.valueOf());
console.log((n.valueOf() as number) + 8);
`
	got := runProgramGo(t, src)
	want := "42\ntrue\nhi\n50\n"
	if got != want {
		t.Fatalf("dynamic valueOf run mismatch:\n got %q\nwant %q", got, want)
	}
}
