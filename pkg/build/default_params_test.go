package build

import "testing"

// A defaulted closure parameter whose Go slot is a value.Value fills its own default at
// body entry rather than at the call site. These run the shapes end to end against the
// Node oracle, since the whole point of filling in the callee is that every way of
// reaching the function agrees without the call site knowing anything.

// TestDefaultedParameterInARequiredModule is the shape the Node compatibility suite
// gates on. A required module's body is a closure, so its `function f(a, b = ...)` is a
// closure parameter, and the export flows into a dynamic slot, so the call comes back
// through the boxed wrapper. Both ends see undefined for the argument the call omitted
// and the callee's own body puts the default in.
func TestDefaultedParameterInARequiredModule(t *testing.T) {
	files := map[string]string{
		"lib.js": `function label(a, b = "d") {
  return String(a) + "|" + String(b);
}
module.exports = { label };
`,
		"main.js": `const { label } = require("./lib");
console.log(label(1));
console.log(label(2, "z"));
console.log(label(3, undefined));
`,
	}
	got := buildAndRun(t, "main.js", files)
	want := "1|d\n2|z\n3|d\n"
	if got != want {
		t.Fatalf("a defaulted export printed %q, want %q", got, want)
	}
}

// TestDefaultReadingAnEarlierParameter runs the default the call site could never fill,
// one that reads a parameter bound only in the callee's own scope.
func TestDefaultReadingAnEarlierParameter(t *testing.T) {
	const src = `const pair = function (a, b = a) {
  return String(a) + "|" + String(b);
};
const call = pair;
console.log(call(1), call(2, 3));
`
	got := buildAndRunFile(t, "pair.js", src)
	want := "1|1 2|3\n"
	if got != want {
		t.Fatalf("a default reading an earlier parameter printed %q, want %q", got, want)
	}
}

// TestDefaultedArrowReachedThroughAnObject pins the wrong answer this slice closes. The
// escape walk that drops an arrow's defaults for a direct-call-only binding could not
// see an object-literal shorthand as a use, so `const box = { f }` dropped the default
// and the call through box printed undefined for it.
func TestDefaultedArrowReachedThroughAnObject(t *testing.T) {
	const src = `const greet = (a, b = "d") => String(a) + "|" + String(b);
const box = { greet };
console.log(box.greet(1), box.greet(2, "z"));
`
	got := buildAndRunFile(t, "greet.js", src)
	want := "1|d 2|z\n"
	if got != want {
		t.Fatalf("a defaulted arrow reached through an object printed %q, want %q", got, want)
	}
}
