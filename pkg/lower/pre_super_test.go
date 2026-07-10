package lower

import (
	"strings"
	"testing"
)

// TestPreSuperStatementsRun pins that this-free statements before super() lower
// before the base assignment, the order JavaScript runs them, and the super
// arguments feed the base constructor after them.
func TestPreSuperStatementsRun(t *testing.T) {
	const src = `class Base {
  label: string;
  constructor(label: string) { this.label = label; }
}
class Derived extends Base {
  constructor(n: number) {
    console.log("preparing");
    super("d" + String(n * 2));
  }
}
const d = new Derived(5);
console.log(d.label);
`
	got := runProgramGo(t, src)
	want := "preparing\nd10\n"
	if got != want {
		t.Errorf("pre-super statements ran wrong\n got: %q\nwant: %q", got, want)
	}
}

// TestPreSuperEmitsBeforeBaseAssign pins the emitted order: the pre-super log
// statement stands before the base assignment inside the constructor body.
func TestPreSuperEmitsBeforeBaseAssign(t *testing.T) {
	const src = `class Base {
  x: number;
  constructor(x: number) { this.x = x; }
}
class Derived extends Base {
  constructor(n: number) {
    console.log("before");
    super(n);
  }
}
new Derived(1);
`
	source := renderProgram(t, src)
	logIdx := strings.Index(source, `value.ConsoleLog(value.FromGoString("before"))`)
	baseIdx := strings.Index(source, ".Base = *NewBase(")
	if logIdx < 0 || baseIdx < 0 {
		t.Fatalf("expected both the pre-super log and the base assignment:\n%s", source)
	}
	if logIdx > baseIdx {
		t.Errorf("pre-super log emitted after the base assignment:\n%s", source)
	}
}

// TestPreSuperHandsBack pins the boundary: a variable declaration before super()
// lowers in its own place ahead of the base assignment rather than through the
// body's var-hoist scope, so it stays a later slice.
func TestPreSuperHandsBack(t *testing.T) {
	const src = `class Base {
  x: number;
  constructor(x: number) { this.x = x; }
}
class Derived extends Base {
  constructor(n: number) {
    const y: number = n * 2;
    super(y);
  }
}
new Derived(1);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "a variable declaration before super() is a later slice") {
		t.Errorf("hand-back reason %q does not name the variable declaration case", reason)
	}
}
