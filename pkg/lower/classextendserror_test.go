package lower

import (
	"strings"
	"testing"
)

// TestClassExtendsErrorLowers pins the shape of a class that extends the built-in
// Error: the struct carries the inherited message and name as value.BStr fields
// beside its own declared field, and the constructor fills them from super() before
// the body's this.name override runs. There is no embedded base struct, since the
// built-in Error is not a registered class to embed.
func TestClassExtendsErrorLowers(t *testing.T) {
	const src = `class AppError extends Error {
  code: number;
  constructor(message: string, code: number) {
    super(message);
    this.name = "AppError";
    this.code = code;
  }
}
const e = new AppError("boom", 42);
console.log(e.message);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "Message value.BStr `json:\"message\"`") {
		t.Fatalf("no synthesized message field:\n%s", out)
	}
	if !strings.Contains(out, "Name    value.BStr `json:\"name\"`") {
		t.Fatalf("no synthesized name field:\n%s", out)
	}
	if !strings.Contains(out, "a.Message = message") {
		t.Fatalf("super(message) did not fill the inherited message:\n%s", out)
	}
	if !strings.Contains(out, "a.Name = value.FromGoString(\"Error\")") {
		t.Fatalf("super() did not default the inherited name to Error:\n%s", out)
	}
	if !strings.Contains(out, "a.Name = value.FromGoString(\"AppError\")") {
		t.Fatalf("this.name override did not lower:\n%s", out)
	}
	if strings.Contains(out, "AppError\n\tError") || strings.Contains(out, "\tError\n") {
		t.Fatalf("an Error subclass should embed no base struct:\n%s", out)
	}
}

// TestClassExtendsErrorRuns builds and runs a subclass end to end: the inherited
// message and name read back through the instance, the own field carries its value,
// and instanceof Error folds to true. Node prints the same four lines.
func TestClassExtendsErrorRuns(t *testing.T) {
	skipIfShort(t)
	const src = `class AppError extends Error {
  code: number;
  constructor(message: string, code: number) {
    super(message);
    this.name = "AppError";
    this.code = code;
  }
}
const e = new AppError("boom", 42);
console.log(e.message);
console.log(e.name);
console.log(String(e.code));
console.log(String(e instanceof Error));
`
	got := runProgramGo(t, src)
	want := "boom\nAppError\n42\ntrue\n"
	if got != want {
		t.Fatalf("extends Error run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestThrowClassExtendsErrorRuns throws a subclass and reads its message off the
// caught binding, proving the synthesized message satisfies the throwable surface
// the panic path needs the same way a declared string message does.
func TestThrowClassExtendsErrorRuns(t *testing.T) {
	skipIfShort(t)
	const src = `class AppError extends Error {
  code: number;
  constructor(message: string, code: number) {
    super(message);
    this.name = "AppError";
    this.code = code;
  }
}
try {
  throw new AppError("boom", 42);
} catch (e) {
  if (e instanceof Error) {
    console.log(e.message);
  }
}
`
	got := runProgramGo(t, src)
	want := "boom\n"
	if got != want {
		t.Fatalf("throw extends Error run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestExtendsBuiltinErrorHandsBack pins that extending a built-in error other than
// Error, TypeError here, stays a later slice rather than lowering as a plain class.
func TestExtendsBuiltinErrorHandsBack(t *testing.T) {
	const src = `class B extends TypeError {
  constructor(m: string) { super(m); }
}
const b = new B("x");
console.log(b.message);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "TypeError") {
		t.Fatalf("handback reason = %q, want it to name TypeError", reason)
	}
}

// TestExtendsErrorNoCtorHandsBack pins that an Error subclass without its own
// constructor, which would inherit the built-in's, stays a later slice.
func TestExtendsErrorNoCtorHandsBack(t *testing.T) {
	const src = `class B extends Error {}
const b = new B();
console.log(String(b instanceof Error));
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "without its own constructor") {
		t.Fatalf("handback reason = %q, want the no-constructor phrasing", reason)
	}
}

// TestExtendsErrorOwnMessageFieldHandsBack pins that a subclass declaring its own
// message field, which would collide with the synthesized one, stays a later slice.
func TestExtendsErrorOwnMessageFieldHandsBack(t *testing.T) {
	const src = `class B extends Error {
  message: string = "x";
  constructor() { super(); }
}
const b = new B();
console.log(b.message);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "declares its own .message field") {
		t.Fatalf("handback reason = %q, want the own-message phrasing", reason)
	}
}

// TestClassExtendsErrorDynamicMessageLowers pins that a subclass whose super()
// argument is dynamic rather than statically a string, the shape assert's
// AssertionError hands (options.message || fallback off an any-typed options),
// coerces through value.ErrorMessageString into the inherited message field.
func TestClassExtendsErrorDynamicMessageLowers(t *testing.T) {
	const src = `class E extends Error {
  constructor(o: any) {
    super(o.message || "def");
    this.name = "E";
  }
}
const e = new E({ message: "boom" });
console.log(e.message);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "e.Message = value.ErrorMessageString(") {
		t.Fatalf("dynamic super message did not route through ErrorMessageString:\n%s", out)
	}
	if !strings.Contains(out, "e.Name = value.FromGoString(\"Error\")") {
		t.Fatalf("super() did not default the inherited name to Error:\n%s", out)
	}
}

// TestClassExtendsErrorDynamicMessageRuns builds a subclass with a dynamic super()
// message and reads the stored value on three arms: a present string, a fallback
// where the dynamic read is undefined, and a present non-string the constructor
// coerces with ToString.
func TestClassExtendsErrorDynamicMessageRuns(t *testing.T) {
	skipIfShort(t)
	const src = `class E extends Error {
  constructor(o: any) {
    super(o.message || "def");
  }
}
class N extends Error {
  constructor(o: any) {
    super(o.count);
  }
}
console.log(new E({ message: "boom" }).message);
console.log(new E({}).message);
console.log(new N({ count: 42 }).message);
console.log(new N({}).message === "" ? "empty" : "nonempty");
`
	got := runProgramGo(t, src)
	want := "boom\ndef\n42\nempty\n"
	if got != want {
		t.Fatalf("dynamic super message run mismatch:\n got %q\nwant %q", got, want)
	}
}
