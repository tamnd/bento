package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestBreakEmits pins that a bare break inside a loop lowers to a Go break, the same
// keyword targeting the same innermost loop.
func TestBreakEmits(t *testing.T) {
	const src = `function firstMultiple(n: number): number {
  let found = 0;
  for (let i = 1; i < 100; i++) {
    if (i % n === 0) {
      found = i;
      break;
    }
  }
  return found;
}
console.log(firstMultiple(7));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "break") {
		t.Errorf("break did not lower to a Go break:\n%s", source)
	}
}

// TestContinueEmits pins that a bare continue inside a loop lowers to a Go continue.
func TestContinueEmits(t *testing.T) {
	const src = `function sumSkip(n: number): number {
  let s = 0;
  for (let i = 0; i < n; i++) {
    if (i === n - 1) continue;
    s += i;
  }
  return s;
}
console.log(sumSkip(5));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "continue") {
		t.Errorf("continue did not lower to a Go continue:\n%s", source)
	}
}

// TestBranchInSwitch pins that a break inside a switch case lowers away with the
// case (Go breaks for it) while a continue inside a switch nested in a loop survives
// as a Go continue, since Go's continue targets the loop just as JavaScript's does.
// The switch that would otherwise read the trailing continue as a fall-through keeps
// lowering because a continue terminates the case.
func TestBranchInSwitch(t *testing.T) {
	const src = `function label(n: number): string {
  let out = "";
  for (let i = 0; i < n; i++) {
    switch (i) {
      case 0:
        continue;
      case 1:
        out += "a";
        break;
      default:
        out += "b";
    }
    out += ".";
  }
  return out;
}
console.log(label(3));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "continue") {
		t.Errorf("continue in a switch did not survive:\n%s", source)
	}
	if !strings.Contains(source, "switch i {") {
		t.Errorf("switch in a loop did not lower:\n%s", source)
	}
}

// TestLabeledBranchHandsBack pins that a labeled break keeps the unit on the engine
// rather than lowering to an unlabeled Go break that would target the wrong loop. The
// labeled statement carries a target this slice does not model, so the whole unit
// hands back with a NotYetLowerable.
func TestLabeledBranchHandsBack(t *testing.T) {
	const src = `function f(n: number): number {
  let s = 0;
  outer: for (let i = 0; i < n; i++) {
    for (let j = 0; j < n; j++) {
      if (j > i) break outer;
      s++;
    }
  }
  return s;
}
console.log(f(4));
`
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
}

// TestBreakContinueRuns builds and runs break and continue end to end and matches the
// Node oracle: a break stops a loop early, a continue skips to the next iteration, and
// both compose with a switch nested in a loop.
func TestBreakContinueRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function sumUntil(n: number): number {
  let s = 0;
  for (let i = 0; i < n; i++) {
    if (i > 5) break;
    s += i;
  }
  return s;
}
function sumOdd(n: number): number {
  let s = 0;
  for (let i = 0; i < n; i++) {
    if (i % 2 === 0) continue;
    s += i;
  }
  return s;
}
function label(n: number): string {
  let out = "";
  for (let i = 0; i < n; i++) {
    switch (i) {
      case 0:
        continue;
      case 1:
        out += "a";
        break;
      default:
        out += "b";
    }
    out += ".";
  }
  return out;
}
console.log(sumUntil(100));
console.log(sumOdd(10));
console.log(label(3));
`
	got := runProgramGo(t, src)
	want := "15\n" +
		"25\n" +
		"a.b.\n"
	if got != want {
		t.Fatalf("break/continue program printed %q, want %q", got, want)
	}
}
