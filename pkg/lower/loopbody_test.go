package lower

import (
	"strings"
	"testing"
)

// JavaScript lets a loop body be a single unbraced statement, not just a braced
// block. A for and an if already accept that form; these tests pin that a for...of
// and a while do too, so `for (const x of xs) s += x;` and `while (c) s++;` lower
// instead of handing the whole unit back to the interpreter.

// TestForOfUnbracedBodyLowers proves a for...of with a lone unbraced statement
// lowers to the same range loop a braced body produces, with the statement as the
// loop's one line.
func TestForOfUnbracedBodyLowers(t *testing.T) {
	const src = "export function f(): number { let s = 0; for (const x of [1, 2, 3]) s = s + x; return s; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "range value.NewArray[float64](1, 2, 3).Elems()") {
		t.Errorf("for...of with an unbraced body did not lower to a range loop:\n%s", source)
	}
	if !strings.Contains(source, "s += x") {
		t.Errorf("for...of unbraced body statement was not lowered into the loop:\n%s", source)
	}
}

// TestWhileUnbracedBodyLowers proves a while with a lone unbraced statement lowers
// to the same condition-only Go for a braced body produces.
func TestWhileUnbracedBodyLowers(t *testing.T) {
	const src = "export function f(): number { let s = 0; while (s < 3) s = s + 1; return s; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "for s < 3 {") {
		t.Errorf("while with an unbraced body did not lower to a condition-only for:\n%s", source)
	}
	if !strings.Contains(source, "s++") {
		t.Errorf("while unbraced body statement was not lowered into the loop:\n%s", source)
	}
}

// TestUnbracedLoopBodiesRun builds and runs the generated Go so the unbraced forms
// are proven to compute the same result as their braced twins, not just to lower.
func TestUnbracedLoopBodiesRun(t *testing.T) {
	skipIfShort(t)
	const src = `
function run(): void {
  let sum = 0;
  for (const x of [1, 2, 3, 4]) sum = sum + x;
  console.log(sum);

  const sq: number[] = [];
  for (const i of [1, 2, 3]) sq.push(i * i);
  console.log(sq.join(","));

  let n = 5;
  while (n > 0) n = n - 1;
  console.log(n);

  let s = 0;
  while (s < 3) s++;
  console.log(s);
}
run();
`
	if got, want := runProgramGo(t, src), "10\n1,4,9\n0\n3\n"; got != want {
		t.Fatalf("unbraced loop bodies printed %q, want %q", got, want)
	}
}
