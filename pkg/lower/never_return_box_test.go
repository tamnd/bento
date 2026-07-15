package lower

import (
	"strings"
	"testing"
)

// TestNeverReturnFuncBoxes pins that a function whose body only throws, so its
// inferred return type is never, boxes into a dynamic value slot and runs. This
// is the shape every assert.throws callback takes, a thunk that always throws,
// so it gates the throwing-test prelude.
func TestNeverReturnFuncBoxes(t *testing.T) {
	const src = `function run(func: any): void { func(); }
try {
  run(function () { throw new TypeError("boom"); });
} catch (e: any) {
  console.log("caught: " + e.message);
}
`
	got := runProgramGo(t, src)
	if got != "caught: boom\n" {
		t.Errorf("never-return callback ran wrong\n got: %q\nwant: %q", got, "caught: boom\n")
	}
}

// TestNeverReturnFuncEmitsUndefined pins the wrapper shape: the never-returning
// thunk runs the inner call for effect and returns value.Undefined, the same
// unreachable-but-well-typed tail a void return takes.
func TestNeverReturnFuncEmitsUndefined(t *testing.T) {
	const src = `function run(func: any): void { func(); }
run(function () { throw new TypeError("x"); });
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NewFunc(") {
		t.Fatalf("never-return callback did not box through value.NewFunc:\n%s", source)
	}
	if !strings.Contains(source, "return value.Undefined") {
		t.Errorf("never-return wrapper does not yield value.Undefined:\n%s", source)
	}
}

// TestCtorOptionalParamHandsBack pins the zero-fail guard: a class constructor
// with a static optional parameter hands back rather than emit a value.Opt field
// the body reads as a bare T, which would not compile. A plain function's optional
// parameter lowers to value.Opt[T] now, but a constructor keeps the stricter
// paramFields (no optParams narrowing set is built for it), so it stays a later slice.
func TestCtorOptionalParamHandsBack(t *testing.T) {
	const src = `class Box {
  message: string;
  constructor(message?: string) { this.message = message === undefined ? "" : message; }
}
console.log(new Box("hi").message);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "optional parameter needs call-site defaulting") {
		t.Errorf("hand-back reason %q does not name the optional-parameter case", reason)
	}
}
