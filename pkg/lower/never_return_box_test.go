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

// TestCtorOptionalParamNarrowsInTernary runs a bare optional constructor parameter
// narrowed by a message === undefined ? ... : message conditional, both supplied and
// omitted, proving the narrowing reaches a ternary branch and the new-Box call site
// fills value.Some or value.None.
func TestCtorOptionalParamNarrowsInTernary(t *testing.T) {
	skipIfShort(t)
	const src = `class Box {
  message: string;
  constructor(message?: string) { this.message = message === undefined ? "none" : message; }
}
console.log(new Box("hi").message);
console.log(new Box().message);
`
	if got, want := runProgramGo(t, src), "hi\nnone\n"; got != want {
		t.Fatalf("constructor optional parameter in ternary printed %q, want %q", got, want)
	}
}
