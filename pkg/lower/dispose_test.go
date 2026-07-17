package lower

import (
	"strings"
	"testing"
)

// Explicit resource management binds a disposable with `using` or `await using` and
// runs its [Symbol.dispose] or [Symbol.asyncDispose] at scope exit. The first slice
// is the well-known-symbol method name: a class defines its release method under the
// computed name [Symbol.dispose], and bento lowers that name to a fixed Go method the
// way it lowers [Symbol.iterator], so the class resolves where an ordinary [expr]
// method name would hand back. The `using` declaration that calls the method at scope
// exit is a later slice, and until it lands the declaration hands back rather than
// lower to a plain binding that would drop the disposal.

const disposeClass = `
class R {
  name: string;
  constructor(n: string) { this.name = n; }
  [Symbol.dispose]() { console.log("dispose " + this.name); }
  label(): string { return this.name; }
}
`

// TestClassSymbolDisposeMethodLowers proves a class whose release method carries the
// well-known [Symbol.dispose] computed name lowers to a Go method under the fixed
// SymbolDispose name, rather than handing back the way an ordinary computed method
// name does.
func TestClassSymbolDisposeMethodLowers(t *testing.T) {
	src := disposeClass + "const r = new R(\"a\");\nconsole.log(r.label());\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "SymbolDispose()") {
		t.Errorf("[Symbol.dispose] method did not lower to the SymbolDispose Go method:\n%s", source)
	}
}

// TestClassSymbolAsyncDisposeMethodLowers proves the async mirror: a class whose
// release method carries the well-known [Symbol.asyncDispose] computed name lowers to
// a Go method under the fixed SymbolAsyncDispose name.
func TestClassSymbolAsyncDisposeMethodLowers(t *testing.T) {
	const src = `
class R {
  name: string;
  constructor(n: string) { this.name = n; }
  [Symbol.asyncDispose]() { console.log("async dispose " + this.name); }
  label(): string { return this.name; }
}
const r = new R("a");
console.log(r.label());
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "SymbolAsyncDispose()") {
		t.Errorf("[Symbol.asyncDispose] method did not lower to the SymbolAsyncDispose Go method:\n%s", source)
	}
}

// TestManualDisposeDriveLowers proves a test can invoke disposal by hand:
// obj[Symbol.dispose]() reads the release method as the Go SymbolDispose method, the
// same name the `using` disposal path will call, so a manual dispose lowers to a
// direct method call.
func TestManualDisposeDriveLowers(t *testing.T) {
	src := disposeClass + "const r = new R(\"a\");\nr[Symbol.dispose]();\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".SymbolDispose()") {
		t.Errorf("manual dispose did not lower through the SymbolDispose method:\n%s", source)
	}
}

// TestManualDisposeDriveRuns builds and runs a by-hand disposal: construct the
// resource, then call its [Symbol.dispose] directly, so the SymbolDispose method runs
// and prints its release line.
func TestManualDisposeDriveRuns(t *testing.T) {
	skipIfShort(t)
	src := disposeClass + "const r = new R(\"a\");\nconsole.log(\"before\");\nr[Symbol.dispose]();\nconsole.log(\"after\");\n"
	if got, want := runProgramGo(t, src), "before\ndispose a\nafter\n"; got != want {
		t.Fatalf("manual dispose printed %q, want %q", got, want)
	}
}

// TestUsingDeclarationHandsBack proves the honest leftover: a `using` declaration
// whose scope-exit disposal is not yet lowered hands the unit back rather than lower
// to a plain binding that would drop the dispose call, keeping the zero-fail
// invariant. The resource class's dispose method lowers already; only the declaration
// waits for its slice.
func TestUsingDeclarationHandsBack(t *testing.T) {
	src := disposeClass + "using r = new R(\"a\");\nconsole.log(\"body\");\n"
	reason := renderProgramHandBack(t, src)
	if want := "the using declaration's scope-exit disposal is a later slice"; reason != want {
		t.Fatalf("handback reason = %q, want %q", reason, want)
	}
}

// TestAwaitUsingDeclarationHandsBack proves the async mirror hands back the same way,
// reported by its full `await using` spelling.
func TestAwaitUsingDeclarationHandsBack(t *testing.T) {
	const src = `
class R {
  name: string;
  constructor(n: string) { this.name = n; }
  async [Symbol.asyncDispose](): Promise<void> { console.log("dispose " + this.name); }
}
async function f(): Promise<void> {
  await using r = new R("a");
  console.log("body");
}
f();
`
	reason := renderProgramHandBack(t, src)
	if want := "the await using declaration's scope-exit disposal is a later slice"; reason != want {
		t.Fatalf("handback reason = %q, want %q", reason, want)
	}
}
