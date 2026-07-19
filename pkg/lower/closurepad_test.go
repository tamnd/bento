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
