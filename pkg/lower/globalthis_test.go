package lower

import (
	"strings"
	"testing"
)

// globalThis is the name a Node test reaches the global scope through, and line 23 of
// test/common/index.js, `const process = globalThis.process`, is where 665 of the
// suite's tests stopped. The tests below are the four shapes that line and its
// neighbours need: a member read that names a global bento models, an alias binding,
// a keyed read, and a write the program's own code makes. The last one carries the
// leak check common runs, `for (const val in globalThis)`, which only reports what the
// program itself put there because everything bento installs is non-enumerable.

// These render through the .js front door rather than the .ts one, because that is
// what a Node test is and the difference shows here: a write to a global the source
// did not declare, `globalThis.marker = 7`, draws the checker's no-index-signature
// report on `typeof globalThis` in a .ts file and draws nothing at all in the .js mode
// the AOT build reads a test in. Pinning these shapes through the .ts door would pin a
// handback the suite never meets.

// runJS compiles and runs a JavaScript snippet and returns what it printed.
func runJS(t *testing.T, src string) string {
	t.Helper()
	return goRunSource(t, renderUncheckedJS(t, src))
}

// handsBackJS is handsBack for a JavaScript snippet, the contract for a construct
// outside the covered subset.
func handsBackJS(t *testing.T, src string) {
	t.Helper()
	prog := compileUncheckedJS(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	if _, err := r.RenderProgram(entryFile(t, prog)); err == nil {
		t.Fatalf("snippet lowered, want a hand-back:\n%s", src)
	}
}

// TestGlobalThisMemberOfAModelledGlobalReadsThatGlobal is the gate itself: the member
// read means the same thing the bare name does, so the binding holds the process
// object rather than a property looked up on an object at run time.
func TestGlobalThisMemberOfAModelledGlobalReadsThatGlobal(t *testing.T) {
	out := renderUncheckedJS(t, "const p = globalThis.process; console.log(p.platform);\n")
	if !strings.Contains(out, bentoProcessName) {
		t.Errorf("globalThis.process did not read the process object:\n%s", out)
	}
	if strings.Contains(out, bentoGlobalThisName) {
		t.Errorf("globalThis.process went through the global object, want the bare name:\n%s", out)
	}
}

// TestGlobalThisMemberOfAnUnmodelledGlobalHandsBack keeps the read honest. crypto is a
// global Node has and bento has not built, so the answer the global object holds for
// it is undefined, which is not the answer. Naming it is the handback.
func TestGlobalThisMemberOfAnUnmodelledGlobalHandsBack(t *testing.T) {
	handsBackJS(t, "console.log(typeof globalThis.crypto);\n")
}

// TestGlobalThisReferenceEmitsTheGlobalObject pins the emit: one package-level object
// built by the runtime, which every read and write off the name shares.
func TestGlobalThisReferenceEmitsTheGlobalObject(t *testing.T) {
	out := renderUncheckedJS(t, "globalThis.count = 1; console.log(globalThis.count);\n")
	if !strings.Contains(out, "var "+bentoGlobalThisName+" = value.GlobalThisValue()") {
		t.Errorf("the global object was not emitted once at package level:\n%s", out)
	}
	if !strings.Contains(out, ".Set(") {
		t.Errorf("the write did not dispatch through the value model:\n%s", out)
	}
}

// TestGlobalThisAliasBindsToTheSameObject covers `const g = globalThis`, which the
// checker types as the whole global scope. That type has no Go shape, so the binding
// has to take the value slot the reference itself lowers to.
func TestGlobalThisAliasBindsToTheSameObject(t *testing.T) {
	out := renderUncheckedJS(t, "const g = globalThis; g.n = 1; console.log(g.n);\n")
	if !strings.Contains(out, "g := "+bentoGlobalThisName) {
		t.Errorf("the alias did not bind to the global object:\n%s", out)
	}
}

// TestGlobalThisKeyedReadDispatchesAtRunTime covers the bracket spelling. A key that
// is constant in the source is still a key the object answers, so the read is the
// runtime one rather than a Go field selected off the global scope's type.
func TestGlobalThisKeyedReadDispatchesAtRunTime(t *testing.T) {
	out := renderUncheckedJS(t, "const k = \"process\"; console.log(typeof globalThis[k]);\n")
	if !strings.Contains(out, bentoGlobalThisName) {
		t.Errorf("the keyed read did not go through the global object:\n%s", out)
	}
}

// TestGlobalThisRunsAsTheGlobalScope is the behaviour end to end: a write is readable
// under both names, the process global is the one object under both spellings, and a
// global bento installed does not show up in the leak check common runs.
func TestGlobalThisRunsAsTheGlobalScope(t *testing.T) {
	skipIfShort(t)
	src := `globalThis.marker = 7;
const g = globalThis;
console.log(typeof g, g.marker, globalThis.marker);
console.log(globalThis.process === process);
for (const key in globalThis) { console.log("own", key); }
`
	got := runJS(t, src)
	want := "object 7 7\ntrue\nown marker\n"
	if got != want {
		t.Errorf("global scope behaviour: got %q, want %q", got, want)
	}
}
