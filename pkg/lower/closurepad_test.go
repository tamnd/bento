package lower

import (
	"strings"
	"testing"
)

// TestClosureArgPadsMissingTrailingParam pins that a function literal passed to a
// callback slot that declares more parameters than the literal grows the slot's
// trailing parameters as blank-named fields, so the emitted func value's type equals
// the slot's Go func type. JavaScript lets the callback ignore the argument, but Go
// requires the arity to match, so a zero-parameter literal flowing into a
// (x?: string) => void slot lowers to func(_ value.Opt[value.BStr]).
func TestClosureArgPadsMissingTrailingParam(t *testing.T) {
	const src = `interface ICb { (x?: string): void }
function Load(f: ICb) {}
Load(function () {});
Load(function (z?: string) {});
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "Load(func(_ value.Opt[value.BStr]) {") {
		t.Errorf("zero-arg literal did not pad its callback slot's trailing param:\n%s", source)
	}
	if !strings.Contains(source, "Load(func(z value.Opt[value.BStr]) {") {
		t.Errorf("full-arity literal did not keep its declared param:\n%s", source)
	}
}

// TestArrayMapCallbackPadsElement pins that an array map callback that ignores its
// element, () => expr, grows the element parameter so the emitted func value matches
// the value.MapArray func(T) U it flows into. Without the pad the zero-arg literal
// emitted func() U, which Go rejected against the func(value.Value) U the map expects.
func TestArrayMapCallbackPadsElement(t *testing.T) {
	const src = `const xs = [1, 2, 3];
const ys = xs.map(() => "k");
console.log(ys.length);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func(_ float64) value.BStr {") {
		t.Errorf("map callback ignoring its element did not pad the element param:\n%s", source)
	}
}

// TestArrayMapCallbackPadRuns proves the padded map callback runs: the element is
// ignored exactly as the source arrow ignores it, so a type-changing map over an any
// array builds and prints its mapped values.
func TestArrayMapCallbackPadRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const xs = [1, 2, 3];
const ys = xs.map(() => "k");
console.log(ys.join(","));
`
	if got, want := runProgramGo(t, src), "k,k,k\n"; got != want {
		t.Fatalf("map-callback pad run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestClosureArgPadRuns builds and runs a lower-arity callback end to end: each
// literal ignores the argument the slot supplies exactly as the source function does,
// so the program compiles and prints the callbacks' own output.
func TestClosureArgPadRuns(t *testing.T) {
	skipIfShort(t)
	const src = `interface ICb { (x?: string): void }
function run(f: ICb) { f("hi"); }
run(function () { console.log("no-arg"); });
run(function (z?: string) { console.log(z); });
`
	got := runProgramGo(t, src)
	want := "no-arg\nhi\n"
	if got != want {
		t.Fatalf("padded callback run mismatch:\n got %q\nwant %q", got, want)
	}
}
