package lower

import (
	"strings"
	"testing"
)

// TestGeneratorFuncCoroutineShape pins that a top-level generator function lowers to
// a Go function returning the running coroutine value.NewGen builds: g() hands the
// caller a *value.Gen[T], and the body is the goroutine func wrapped in NewGen.
func TestGeneratorFuncCoroutineShape(t *testing.T) {
	const src = `function* g(): Generator<number> { yield 1; yield 2; }
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func G() *value.Gen[float64]") {
		t.Errorf("generator function did not lower to a *value.Gen coroutine:\n%s", source)
	}
	if !strings.Contains(source, "value.NewGen[float64](") {
		t.Errorf("generator body was not wrapped in value.NewGen:\n%s", source)
	}
}

// TestGeneratorFuncExprShape pins that a generator function expression bound to a
// const lowers to a closure returning the running coroutine, the value form of the
// generator function declaration.
func TestGeneratorFuncExprShape(t *testing.T) {
	const src = `const g = function* (): Generator<number> { yield 1; yield 2; };
g();
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func() *value.Gen[float64]") {
		t.Errorf("generator function expression did not lower to a *value.Gen closure:\n%s", source)
	}
	if !strings.Contains(source, "value.NewGen[float64](") {
		t.Errorf("generator expression body was not wrapped in value.NewGen:\n%s", source)
	}
}

// TestGeneratorFuncExprForOf pins that a generator function expression drives a
// for...of the same as the declaration form, pulling its coroutine until done.
func TestGeneratorFuncExprForOf(t *testing.T) {
	const src = `const g = function* (): Generator<number> { yield 5; yield 6; yield 7; };
let out = "";
for (const x of g()) { out += String(x) + " "; }
console.log(out);
`
	if got, want := runProgramGo(t, src), "5 6 7 \n"; got != want {
		t.Fatalf("generator expression for...of printed %q, want %q", got, want)
	}
}

// TestGeneratorNamedFuncExprHandsBack pins that a named generator function
// expression keeps its own reason until that slice lands.
func TestGeneratorNamedFuncExprHandsBack(t *testing.T) {
	const src = `const g = function* gen(): Generator<number> { yield 1; };
g();
`
	if reason, want := renderProgramHandBack(t, src), "a named generator function expression is a later slice"; reason != want {
		t.Fatalf("handback reason = %q, want %q", reason, want)
	}
}

// TestGeneratorFuncForOf pins that a for...of over a generator function pulls its
// coroutine until done, printing each yielded value in order.
func TestGeneratorFuncForOf(t *testing.T) {
	const src = `function* g(): Generator<number> { yield 10; yield 20; yield 30; }
for (const x of g()) { console.log(String(x)); }
`
	if got, want := runProgramGo(t, src), "10\n20\n30\n"; got != want {
		t.Fatalf("generator for...of printed %q, want %q", got, want)
	}
}

// TestGeneratorFuncYieldInControlFlow pins that a yield inside a loop suspends the
// coroutine wherever it sits, so the loop advances one turn per pull.
func TestGeneratorFuncYieldInControlFlow(t *testing.T) {
	const src = `function* g(): Generator<number> {
  for (let i = 0; i < 4; i++) { yield i * i; }
}
let out = "";
for (const x of g()) { out += String(x) + " "; }
console.log(out);
`
	if got, want := runProgramGo(t, src), "0 1 4 9 \n"; got != want {
		t.Fatalf("control-flow generator printed %q, want %q", got, want)
	}
}

// TestGeneratorFuncYieldString pins that the element type follows the yielded
// values: a generator of strings lowers to a *value.Gen[value.BStr] and its for...of
// binds each string.
func TestGeneratorFuncYieldString(t *testing.T) {
	const src = `function* g(): Generator<string> { yield "a"; yield "b"; }
let out = "";
for (const s of g()) { out += s; }
console.log(out);
`
	if got, want := runProgramGo(t, src), "ab\n"; got != want {
		t.Fatalf("string generator printed %q, want %q", got, want)
	}
}

// TestGeneratorYieldExprValue pins that a yield used as an expression binds the
// value the consumer sends back through next(v). A for...of drive always sends
// undefined, so each yield expression evaluates to undefined here.
func TestGeneratorYieldExprValue(t *testing.T) {
	const src = `function* g(): Generator<number> {
  const a = yield 1;
  console.log("a=" + String(a));
  const b = yield 2;
  console.log("b=" + String(b));
}
for (const v of g()) { console.log("v=" + String(v)); }
`
	if got, want := runProgramGo(t, src), "v=1\na=undefined\nv=2\nb=undefined\n"; got != want {
		t.Fatalf("yield-expression generator printed %q, want %q", got, want)
	}
}

// TestGeneratorYieldExprShape pins that a plain yield expression lowers to a bare
// Yield call on the coroutine, whose result is the dynamic value the consumer sent.
func TestGeneratorYieldExprShape(t *testing.T) {
	const src = `function* g(): Generator<number> {
  const a = yield 1;
  console.log(String(a));
}`
	got := renderProgram(t, src)
	if !strings.Contains(got, "a := _bt0.Yield(1)") {
		t.Fatalf("yield expression did not bind the sent value:\n%s", got)
	}
}

// TestGeneratorYieldTypedNextCoerces pins that when the next type is a concrete
// primitive, the dynamic Yield result is coerced to it so the surrounding Go stays
// typed: total += yield 1 becomes total += value.ToNumber(_co.Yield(1)).
func TestGeneratorYieldTypedNextCoerces(t *testing.T) {
	const src = `function* g(): Generator<number, void, number> {
  let total = 0;
  total += yield 1;
  console.log(String(total));
}`
	got := renderProgram(t, src)
	if !strings.Contains(got, "total += value.ToNumber(_bt0.Yield(1))") {
		t.Fatalf("typed next value was not coerced:\n%s", got)
	}
}

// TestGeneratorEmptyHandsBack pins that a generator with no yielded value has no
// element type to name yet and hands back with that reason.
func TestGeneratorEmptyHandsBack(t *testing.T) {
	const src = `function* g(): Generator<number> { return; }
g();
`
	if reason, want := renderProgramHandBack(t, src), "a generator with no yielded value has no element type here, a later slice"; reason != want {
		t.Fatalf("handback reason = %q, want %q", reason, want)
	}
}

// TestGeneratorReturnValueBoxes pins that a valued return inside a generator boxes
// into the dynamic value the completion frame carries, so a { value, done: true }
// result reports it. The body func returns a value.Value, so return 2 becomes
// return value.Number(2).
func TestGeneratorReturnValueBoxes(t *testing.T) {
	const src = `function* g(): Generator<number> { yield 1; return 2; }
g();`
	got := renderProgram(t, src)
	if !strings.Contains(got, "return value.Number(2)") {
		t.Fatalf("valued return did not box into the completion value:\n%s", got)
	}
}

// TestGeneratorReturnValueForOf pins that a for...of over a generator with a valued
// return still yields each value and completes: for...of discards the return value,
// matching the JavaScript rule, so the loop prints the yields and stops.
func TestGeneratorReturnValueForOf(t *testing.T) {
	const src = `function* g(): Generator<number> { yield 1; yield 2; return 99; }
for (const x of g()) { console.log(String(x)); }
`
	if got, want := runProgramGo(t, src), "1\n2\n"; got != want {
		t.Fatalf("generator with valued return printed %q, want %q", got, want)
	}
}

// TestGeneratorManualNextShape pins that a manual it.next() lowers to the value.GenNext
// drive that packs the { value, done } result, with a boxer closure lifting the yielded
// number into a value.Value.
func TestGeneratorManualNextShape(t *testing.T) {
	const src = `function* g(): Generator<number> { yield 1; }
const it = g();
const r = it.next();
console.log(String(r.done));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.GenNext(it,") {
		t.Errorf("manual next did not lower to value.GenNext:\n%s", source)
	}
	if !strings.Contains(source, "value.Number(") {
		t.Errorf("manual next boxer did not box the yielded number:\n%s", source)
	}
	if !strings.Contains(source, ".Done") {
		t.Errorf("r.done did not lower to the IterResult .Done field:\n%s", source)
	}
}

// TestGeneratorManualNextDrive pins the end-to-end hand drive: pulling with next() and
// reading .value and .done off each result walks the generator exactly as a for...of
// would, one value at a time until done latches.
func TestGeneratorManualNextDrive(t *testing.T) {
	const src = `function* g(): Generator<number> { yield 10; yield 20; yield 30; }
const it = g();
let out = "";
let r = it.next();
while (!r.done) { out += String(r.value) + " "; r = it.next(); }
console.log(out);
`
	if got, want := runProgramGo(t, src), "10 20 30 \n"; got != want {
		t.Fatalf("manual next drive printed %q, want %q", got, want)
	}
}

// TestGeneratorManualNextSends pins that a value passed to next(v) reaches the yield it
// resumes, the same threading a for...of cannot express because it always sends
// undefined. The first next starts the body, the second sends 5 into the first yield.
func TestGeneratorManualNextSends(t *testing.T) {
	const src = `function* g(): Generator<number, void, number> {
  const a = yield 1;
  console.log("got " + String(a));
}
const it = g();
it.next();
it.next(5);
`
	if got, want := runProgramGo(t, src), "got 5\n"; got != want {
		t.Fatalf("manual next(v) send printed %q, want %q", got, want)
	}
}

// TestGeneratorManualReturnShape pins that it.return(v) lowers to the value.GenReturn
// helper, the runtime that closes the generator early and packs the { value, done }
// completion the caller reads.
func TestGeneratorManualReturnShape(t *testing.T) {
	const src = `function* g(): Generator<number> { yield 1; }
const it = g();
it.return(0);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.GenReturn(it,") {
		t.Errorf("manual return did not lower to value.GenReturn:\n%s", source)
	}
}

// TestGeneratorManualReturnRunsFinally pins that return(v) closes a suspended generator
// through its finally block, the JavaScript rule that an early close runs cleanup, and
// that the completion the return packs carries the value and a done of true.
func TestGeneratorManualReturnRunsFinally(t *testing.T) {
	const src = `function* g(): Generator<number> {
  try {
    yield 1;
    yield 2;
  } finally {
    console.log("cleanup");
  }
}
const it = g();
const a = it.next();
console.log(String(a.value));
const b = it.return(99);
console.log(String(b.value) + " " + String(b.done));
`
	if got, want := runProgramGo(t, src), "1\ncleanup\n99 true\n"; got != want {
		t.Fatalf("return early-close printed %q, want %q", got, want)
	}
}

// TestGeneratorManualThrowShape pins that it.throw(e) lowers to the value.GenThrow
// helper, the runtime that raises the thrown value at the suspended yield.
func TestGeneratorManualThrowShape(t *testing.T) {
	const src = `function* g(): Generator<number> { try { yield 1; } catch (e) {} }
const it = g();
it.throw(new Error("x"));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.GenThrow(it,") {
		t.Errorf("manual throw did not lower to value.GenThrow:\n%s", source)
	}
}

// TestGeneratorManualThrowCaught pins that throw(e) raises the error at the suspended
// yield so a try/catch in the body catches it and the generator resumes, yielding again
// past the catch, the JavaScript rule that an injected throw a body catches is recoverable.
func TestGeneratorManualThrowCaught(t *testing.T) {
	const src = `function* g(): Generator<number> {
  try {
    yield 1;
  } catch (e) {
    console.log("caught");
    yield 2;
  }
}
const it = g();
console.log(String(it.next().value));
const r = it.throw(new Error("boom"));
console.log(String(r.value));
`
	if got, want := runProgramGo(t, src), "1\ncaught\n2\n"; got != want {
		t.Fatalf("throw caught printed %q, want %q", got, want)
	}
}

// TestGeneratorYieldStarNonGeneratorHandsBack pins that yield* over a plain iterable
// such as an array is still a later slice: only a generator delegate is lowerable, so
// the array form keeps a reason until the iterator-protocol delegation lands.
func TestGeneratorYieldStarNonGeneratorHandsBack(t *testing.T) {
	const src = `function* g(): Generator<number> { yield* [1, 2]; }
g();
`
	if reason, want := renderProgramHandBack(t, src), "a yield* over a non-generator iterable is a later slice"; reason != want {
		t.Fatalf("handback reason = %q, want %q", reason, want)
	}
}

// TestGeneratorYieldStarShape pins that yield* over a generator delegate lowers to a
// YieldFrom drive on the coroutine handle, the runtime that forwards the delegate's
// values and threads the sent value into it.
func TestGeneratorYieldStarShape(t *testing.T) {
	const src = `function* inner(): Generator<number> { yield 1; yield 2; }
function* g(): Generator<number> { yield* inner(); }
g();
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".YieldFrom(Inner())") {
		t.Errorf("yield* did not lower to a YieldFrom drive of the delegate:\n%s", source)
	}
}

// TestGeneratorYieldStarForOf pins the end-to-end delegation: a for...of over the outer
// generator sees every value the delegate yields, in order, as if the delegate's body
// were spliced into the outer one.
func TestGeneratorYieldStarForOf(t *testing.T) {
	const src = `function* inner(): Generator<number> { yield 2; yield 3; }
function* g(): Generator<number> { yield 1; yield* inner(); yield 4; }
let out = "";
for (const x of g()) { out += String(x) + " "; }
console.log(out);
`
	if got, want := runProgramGo(t, src), "1 2 3 4 \n"; got != want {
		t.Fatalf("yield* delegation for...of printed %q, want %q", got, want)
	}
}

// TestGeneratorYieldStarReturnValue pins that the yield* expression evaluates to the
// delegate's return value, the number a { value, done: true } from the delegate
// carries, so a delegate that returns 5 makes `yield* inner()` read as 5.
func TestGeneratorYieldStarReturnValue(t *testing.T) {
	const src = `function* inner(): Generator<number, number> { yield 1; return 5; }
function* g(): Generator<number> { const r = yield* inner(); yield r + 10; }
let out = "";
for (const x of g()) { out += String(x) + " "; }
console.log(out);
`
	if got, want := runProgramGo(t, src), "1 15 \n"; got != want {
		t.Fatalf("yield* return value printed %q, want %q", got, want)
	}
}

// TestGeneratorForOfBreakStops pins that a for...of that breaks out of a generator
// early lowers a close: the loop tracks that it broke and calls Stop after the loop, so
// the abandoned goroutine unwinds rather than parking on its next yield forever.
func TestGeneratorForOfBreakStops(t *testing.T) {
	const src = `function* g(): Generator<number> { yield 1; yield 2; yield 3; }
for (const x of g()) { console.log(String(x)); break; }
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Stop()") {
		t.Errorf("a breakable generator for...of did not lower a Stop close:\n%s", source)
	}
}

// TestGeneratorForOfNoBreakStaysPlain pins the other side: a for...of the body never
// breaks out of runs the generator to done, which needs no close, so the drain
// machinery is left out and the loop stays the plain pull-until-done form.
func TestGeneratorForOfNoBreakStaysPlain(t *testing.T) {
	const src = `function* g(): Generator<number> { yield 1; yield 2; }
for (const x of g()) { console.log(String(x)); }
`
	source := renderProgram(t, src)
	if strings.Contains(source, ".Stop()") {
		t.Errorf("a generator for...of with no early exit should not lower a Stop close:\n%s", source)
	}
}

// TestGeneratorForOfBreakRunsFinally pins the end-to-end drain: breaking out of a
// generator mid-iteration unwinds its suspended body through its finally block right
// after the loop, before the statement past the loop runs, matching JavaScript's rule
// that abandoning an iterator closes it.
func TestGeneratorForOfBreakRunsFinally(t *testing.T) {
	const src = `function* g(): Generator<number> {
  try {
    yield 1;
    yield 2;
    yield 3;
  } finally {
    console.log("cleanup");
  }
}
for (const x of g()) {
  console.log(String(x));
  if (x === 2) { break; }
}
console.log("after");
`
	if got, want := runProgramGo(t, src), "1\n2\ncleanup\nafter\n"; got != want {
		t.Fatalf("generator for...of break drain printed %q, want %q", got, want)
	}
}

// TestGeneratorForOfReturnHandsBack pins that a for...of over a generator whose body
// can leave through a return hands back: the return would jump past the after-loop
// close and leak the goroutine, so the loop declines rather than leak silently.
func TestGeneratorForOfReturnHandsBack(t *testing.T) {
	const src = `function* g(): Generator<number> { yield 1; yield 2; }
function h(): void {
  for (const x of g()) {
    if (x === 1) { return; }
    console.log(String(x));
  }
}
h();
`
	if reason, want := renderProgramHandBack(t, src), "stopping a generator on a return, throw, or labeled exit from for...of is a later slice"; reason != want {
		t.Fatalf("handback reason = %q, want %q", reason, want)
	}
}
