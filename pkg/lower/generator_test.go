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

// TestGeneratorValuedReturnHandsBack pins that a generator that returns a value keeps
// its own reason until the return-value slice lands (item 5).
func TestGeneratorValuedReturnHandsBack(t *testing.T) {
	const src = `function* g(): Generator<number> { yield 1; return 2; }
g();
`
	if reason, want := renderProgramHandBack(t, src), "a generator return value is a later slice"; reason != want {
		t.Fatalf("handback reason = %q, want %q", reason, want)
	}
}

// TestGeneratorYieldStarHandsBack pins that a yield* delegation keeps its own reason
// until the delegation slice lands (item 6).
func TestGeneratorYieldStarHandsBack(t *testing.T) {
	const src = `function* g(): Generator<number> { yield* [1, 2]; }
g();
`
	if reason, want := renderProgramHandBack(t, src), "a yield* delegation is a later slice"; reason != want {
		t.Fatalf("handback reason = %q, want %q", reason, want)
	}
}
