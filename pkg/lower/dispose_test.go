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

// TestUsingDisposalLowers proves a `using` at a body top level lowers to its binding
// plus a deferred disposal that runs when the enclosing function returns, rather than
// handing back or dropping the disposal. The disposal routes through the runtime's
// Dispose so a throw from the release chains into a SuppressedError with the pending
// error, so the emitted defer names value.Dispose over the resource's SymbolDispose.
func TestUsingDisposalLowers(t *testing.T) {
	src := disposeClass + "using r = new R(\"a\");\nconsole.log(\"body\");\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "defer value.Dispose(func() {") || !strings.Contains(source, "r.SymbolDispose()") {
		t.Errorf("using declaration did not defer its disposal through value.Dispose:\n%s", source)
	}
}

// TestUsingSuppressedErrorChainRuns proves the explicit-resource-management error
// chain: when the block body throws and the resource's disposal also throws, the
// disposal's error wraps the body's error in a SuppressedError, so a catch around the
// scope binds an error named SuppressedError rather than losing one throw to the
// other the way a plain Go defer would.
func TestUsingSuppressedErrorChainRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
class R {
  name: string;
  constructor(n: string) { this.name = n; }
  [Symbol.dispose]() { throw new Error("dispose " + this.name); }
}
try {
  using r = new R("a");
  throw new Error("body");
} catch (e) {
  if (e instanceof Error) { console.log(e.name); }
}
`
	if got, want := runProgramGo(t, src), "SuppressedError\n"; got != want {
		t.Fatalf("suppressed-error chain printed %q, want %q", got, want)
	}
}

// TestUsingDisposalThrowPropagatesRuns proves the other half: when the block body
// completes and only the disposal throws, the disposal's error propagates on its own,
// so a catch around the scope binds that error rather than a suppression chain.
func TestUsingDisposalThrowPropagatesRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
class R {
  name: string;
  constructor(n: string) { this.name = n; }
  [Symbol.dispose]() { throw new Error("dispose " + this.name); }
}
try {
  using r = new R("a");
  console.log("body");
} catch (e) {
  if (e instanceof Error) { console.log(e.message); }
}
`
	if got, want := runProgramGo(t, src), "body\ndispose a\n"; got != want {
		t.Fatalf("disposal-throw propagation printed %q, want %q", got, want)
	}
}

// TestUsingDisposalRuns builds and runs a top-level `using`, so the disposal is proven
// against the JavaScript order: the body runs, then the resource is released at scope
// exit, printing "body" then "dispose a".
func TestUsingDisposalRuns(t *testing.T) {
	skipIfShort(t)
	src := disposeClass + "console.log(\"before\");\nusing r = new R(\"a\");\nconsole.log(\"body\");\n"
	if got, want := runProgramGo(t, src), "before\nbody\ndispose a\n"; got != want {
		t.Fatalf("using disposal printed %q, want %q", got, want)
	}
}

// TestUsingReverseDisposalRuns proves two `using` bindings in one block dispose in
// reverse declaration order, the protocol's rule, which the last-registered-first
// order of Go defers gives for free: b is released before a.
func TestUsingReverseDisposalRuns(t *testing.T) {
	skipIfShort(t)
	src := disposeClass + "using a = new R(\"a\");\nusing b = new R(\"b\");\nconsole.log(\"body\");\n"
	if got, want := runProgramGo(t, src), "body\ndispose b\ndispose a\n"; got != want {
		t.Fatalf("reverse disposal printed %q, want %q", got, want)
	}
}

// TestUsingInFunctionDisposesOnReturn proves the defer covers an early return: a
// `using` in a function body releases the resource when the function returns, before
// control leaves it, so the release prints between the body and the caller's line.
func TestUsingInFunctionDisposesOnReturn(t *testing.T) {
	skipIfShort(t)
	const body = `
function f(): void {
  using r = new R("a");
  console.log("in f");
  return;
}
f();
console.log("after f");
`
	src := disposeClass + body
	if got, want := runProgramGo(t, src), "in f\ndispose a\nafter f\n"; got != want {
		t.Fatalf("using-on-return printed %q, want %q", got, want)
	}
}

// TestUsingInNestedBlockDisposesAtBlockExit proves a `using` in a nested block
// releases the resource at that block's exit, not the function's: the disposal prints
// between the block body and the statement after the block, so a closure-scoped defer
// runs at the inner brace, where the JavaScript block scope ends.
func TestUsingInNestedBlockDisposesAtBlockExit(t *testing.T) {
	skipIfShort(t)
	const body = `
function f(): void {
  if (true) {
    using r = new R("a");
    console.log("body");
  }
  console.log("after block");
}
f();
`
	src := disposeClass + body
	if got, want := runProgramGo(t, src), "body\ndispose a\nafter block\n"; got != want {
		t.Fatalf("nested-block disposal printed %q, want %q", got, want)
	}
}

// TestUsingInLoopBodyDisposesEachIteration proves a `using` in a loop body releases
// the resource once per iteration, at the end of each pass, the way the JavaScript
// block scope re-enters and exits each time: the closure the disposal wraps the body
// in is invoked and returns every iteration.
func TestUsingInLoopBodyDisposesEachIteration(t *testing.T) {
	skipIfShort(t)
	const body = `
for (let i = 0; i < 2; i = i + 1) {
  using r = new R(String(i));
  console.log("body " + String(i));
}
`
	src := disposeClass + body
	if got, want := runProgramGo(t, src), "body 0\ndispose 0\nbody 1\ndispose 1\n"; got != want {
		t.Fatalf("loop-body disposal printed %q, want %q", got, want)
	}
}

// TestUsingInNestedBlockReturnDisposesThenReturns proves the escape-threading form: a
// `using` whose block leaves by a return releases the resource before the function
// returns, so the disposal prints before the caller's line and the return still leaves
// f. The disposal closure carries the return out through its named results, running the
// defer on the early-return path.
func TestUsingInNestedBlockReturnDisposesThenReturns(t *testing.T) {
	skipIfShort(t)
	const body = `
function f(cond: boolean): void {
  if (cond) {
    using r = new R("a");
    console.log("body");
    return;
  }
  console.log("after block");
}
f(true);
console.log("done");
`
	src := disposeClass + body
	if got, want := runProgramGo(t, src), "body\ndispose a\ndone\n"; got != want {
		t.Fatalf("nested-block return disposal printed %q, want %q", got, want)
	}
}

// TestUsingInNestedBlockValueReturnDisposesThenReturns proves the valued mirror: a
// `using` whose block returns a value releases the resource, then hands the value up as
// the function's return, so the caller reads the returned value after the disposal ran.
func TestUsingInNestedBlockValueReturnDisposesThenReturns(t *testing.T) {
	skipIfShort(t)
	const body = `
function f(cond: boolean): string {
  if (cond) {
    using r = new R("a");
    console.log("body");
    return "early";
  }
  return "late";
}
console.log(f(true));
`
	src := disposeClass + body
	if got, want := runProgramGo(t, src), "body\ndispose a\nearly\n"; got != want {
		t.Fatalf("nested-block value-return disposal printed %q, want %q", got, want)
	}
}

// TestUsingInNestedBlockFallThroughStillRuns proves the escape form keeps the
// fall-through path: when the guarded return is not taken, control runs on past the
// block, disposing at the block exit, and the statement after the block still runs.
func TestUsingInNestedBlockFallThroughStillRuns(t *testing.T) {
	skipIfShort(t)
	const body = `
function f(cond: boolean): void {
  if (true) {
    using r = new R("a");
    console.log("body");
    if (cond) {
      return;
    }
  }
  console.log("after block");
}
f(false);
console.log("done");
`
	src := disposeClass + body
	if got, want := runProgramGo(t, src), "body\ndispose a\nafter block\ndone\n"; got != want {
		t.Fatalf("nested-block fall-through disposal printed %q, want %q", got, want)
	}
}

// TestUsingInNestedBlockBranchHandsBack proves the honest leftover: a `using` whose
// block leaves by a break targeting the loop enclosing the block hands back, where a Go
// break inside the disposal closure would leave the closure, not the loop it names. The
// branch-escape case waits for its own slice, unlike the return case the closure now
// threads out.
func TestUsingInNestedBlockBranchHandsBack(t *testing.T) {
	const body = `
for (let i = 0; i < 2; i = i + 1) {
  if (i === 0) {
    using r = new R("a");
    console.log("body");
    break;
  }
  console.log("tail");
}
`
	src := disposeClass + body
	reason := renderProgramHandBack(t, src)
	if want := "a using declaration whose block exits by break or continue is a later slice"; reason != want {
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
