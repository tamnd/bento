package build

import "testing"

// These build and run the Boolean() argument kinds the lowerer stopped refusing when
// booleanCoercion started delegating to lowerTruthy. The lowerer tests pin the emit;
// these pin that the emit compiles and prints what Node prints, which is the half the
// Boolean() slice got wrong twice. The always-truthy collapse in particular drops the
// argument's read, and a dropped read that nothing accounts for leaves a Go binding
// declared and not used, which fails the build rather than the lowering.

// TestBooleanOfADynamicValue runs the shape the Node compatibility suite's own harness
// uses, Boolean() over a property read off a dynamic object. Every falsy member of the
// value model has to come back false and everything else true, which is the whole point
// of routing through value.ToBoolean rather than a per-type list.
func TestBooleanOfADynamicValue(t *testing.T) {
	got := buildAndRunFile(t, "main.js", `const o = JSON.parse('{"zero":0,"empty":"","text":"x","nil":null,"nested":{}}');
console.log(Boolean(o.zero), Boolean(o.empty), Boolean(o.text));
console.log(Boolean(o.nil), Boolean(o.missing), Boolean(o.nested), Boolean(o));
`)
	want := "false false true\nfalse false true true\n"
	if got != want {
		t.Fatalf("Boolean() over dynamic reads printed %q, want %q", got, want)
	}
}

// TestBooleanOfAnUnreadObjectBuilds pins the collapse's build hole. Boolean(a) over an
// array folds to the constant true and never lowers a, so a is declared and not used
// unless the elided read is recorded. Both spellings of the fold are here, the call and
// the negation, because each reaches the collapse through a different node kind.
func TestBooleanOfAnUnreadObjectBuilds(t *testing.T) {
	got := buildAndRunFile(t, "main.js", `const a = [];
const o = { k: 1 };
console.log(Boolean(a), Boolean(o), !a, !o);
`)
	want := "true true false false\n"
	if got != want {
		t.Fatalf("Boolean() over unread objects printed %q, want %q", got, want)
	}
}

// TestBooleanOfAnUnreadObjectStillReadsIt pins the guard on that recording: a binding
// the fold drops one read of but the program reads again is still live, so it keeps its
// real declaration and the length read below works.
func TestBooleanOfAnUnreadObjectStillReadsIt(t *testing.T) {
	got := buildAndRunFile(t, "main.js", "const a = [];\nconsole.log(Boolean(a), a.length);\n")
	want := "true 0\n"
	if got != want {
		t.Fatalf("Boolean() beside a real read printed %q, want %q", got, want)
	}
}

// TestBooleanOfAnOptionalArgument runs the optional path end to end. An absent argument
// and a present falsy one are both false, which the presence-plus-inner test has to get
// right in one expression.
func TestBooleanOfAnOptionalArgument(t *testing.T) {
	got := buildAndRunFile(t, "main.js", `function f(x) {
  return Boolean(x);
}
console.log(f('a'), f(''), f(undefined), f(0), f({}));
`)
	want := "true false false false true\n"
	if got != want {
		t.Fatalf("Boolean() over an optional parameter printed %q, want %q", got, want)
	}
}
