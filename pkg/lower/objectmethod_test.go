package lower

import (
	"strings"
	"testing"
)

// TestObjectMethodEmitsFuncField pins that a plain method member lowers to the
// func-valued struct field a function-property assignment would fill, so the
// interned shape carries the method's closure under its exported field name.
func TestObjectMethodEmitsFuncField(t *testing.T) {
	src := `
const o = { add(a: number, b: number) { return a + b; } };
console.log(o.add(2, 3));
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "Add:") {
		t.Fatalf("expected an Add func field for the method, got:\n%s", out)
	}
}

// TestObjectMethodRuns builds and runs the emitted Go and checks a called method
// and a method with no parameters against the Node oracle.
func TestObjectMethodRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const o = {
  add(a: number, b: number) { return a + b; },
  greet() { return "hi"; },
};
console.log(o.add(2, 3));
console.log(o.greet());
`
	got := runProgramGo(t, src)
	want := "5\nhi\n"
	if got != want {
		t.Fatalf("object method run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestObjectMethodAlongsideProperty pins that a method and a plain property share
// one interned shape, the method filling its func field while the property fills
// its value field.
func TestObjectMethodAlongsideProperty(t *testing.T) {
	skipIfShort(t)
	src := `
const o = {
  base: 10,
  plus(n: number) { return n + 1; },
};
console.log(o.base);
console.log(o.plus(4));
`
	got := runProgramGo(t, src)
	want := "10\n5\n"
	if got != want {
		t.Fatalf("mixed method/property run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestObjectMethodThisHandsBack pins the receiver boundary: a method that reads
// this needs the object bound as its receiver, which a plain closure field does
// not carry, so the literal hands back rather than emit a closure whose this is
// unbound.
func TestObjectMethodThisHandsBack(t *testing.T) {
	src := `
const o = {
  v: 3,
  get() { return this.v; },
};
console.log(o.get());
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "reads this or super") {
		t.Fatalf("expected a this-receiver handback, got: %s", reason)
	}
}

// TestObjectAccessorHandsBack pins that a getter member is still handed back to
// the descriptor model, its accessor node kind routing it past the plain-method
// path.
func TestObjectAccessorHandsBack(t *testing.T) {
	src := `
const o = {
  get v() { return 1; },
};
console.log(o.v);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "accessor member") {
		t.Fatalf("expected an accessor handback, got: %s", reason)
	}
}
