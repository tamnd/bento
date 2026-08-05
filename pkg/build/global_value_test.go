package build

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A global named rather than called is a value, and what a program does with that
// value is compare it, read typeof off it, hand it to something, and call it later.
// These run the whole thing end to end, since the identity a compiled program sees
// is decided by what the emitted Go reaches for and not by what the runtime holds.

// TestGlobalReadAsAValueKeepsItsIdentity pins the comparison Node's suite writes
// most often: the global reached two ways is one object.
func TestGlobalReadAsAValueKeepsItsIdentity(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = atob;\n"+
			"console.log(a === atob, globalThis.atob === atob, typeof a, a.name);\n")
	if want := "true true function atob\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestGlobalCalledThroughItsValue pins that the value is the function and not a
// stand-in: calling it runs the same runtime the bare name calls.
func TestGlobalCalledThroughItsValue(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const enc = btoa;\n"+
			"const dec = atob;\n"+
			"console.log(dec(enc('bento')));\n")
	if want := "bento\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestConstructorGlobalReadAsAValue pins the constructor half of the family. Symbol
// is the one the suite reaches for, and what it wants is the identity and a call.
func TestConstructorGlobalReadAsAValue(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const S = Symbol;\n"+
			"const one = S('tag');\n"+
			"const two = S('tag');\n"+
			"console.log(S === Symbol, typeof S, one === two, typeof one);\n")
	if want := "true function false symbol\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestConstructorGlobalThatNeedsNew pins that a constructor with no call form says
// so, which is what the language does and what a program testing the refusal catches.
func TestConstructorGlobalThatNeedsNew(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const M = Map;\n"+
			"try {\n"+
			"  M();\n"+
			"} catch (e) {\n"+
			"  console.log(e.name, e.message);\n"+
			"}\n")
	if want := "TypeError Constructor Map requires 'new'\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestGlobalHandedToAFunction pins the shape the suite's helpers take: the global
// goes in as an argument and is called from inside, which is what makes it a value
// rather than a name in the first place.
func TestGlobalHandedToAFunction(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"function run(fn, arg) { return fn(arg); }\n"+
			"console.log(run(btoa, 'ok'));\n")
	if want := "b2s=\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestObjectCalledAsAGlobal pins the one call form that had no lowering at all:
// Object(x) of an object is x, which is how a program asserts it was handed an
// object rather than a primitive. The receiver is a parsed value rather than a
// literal because a fixed-shape literal boxes into a fresh wrapper at each site, so
// the identity a literal would compare is a question about boxing and not about
// Object; the suite's own use (Object(process.config) === process.config) is over a
// value that is already a box, which is what this is.
func TestObjectCalledAsAGlobal(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const o = JSON.parse('{\"a\":1}');\n"+
			"const same = Object(o);\n"+
			"console.log(same === o, same.a);\n")
	if want := "true 1\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// compileJSSource is compileSource for a .js entry, the CommonJS script shape whose
// top level the checker treats as the global scope. It is what reaches the collision
// between a file's own binding and a standard library global of the same name; the
// same source in a .ts entry is a hard redeclaration error at the front door.
func compileJSSource(t *testing.T, src string) (string, error) {
	t.Helper()
	dir := t.TempDir()
	entry := filepath.Join(dir, "entry.js")
	if err := os.WriteFile(entry, []byte(src), 0o644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	return Compile(entry)
}

// TestAFileBindingNamedLikeADomGlobalHandsBack pins the file-scope rule end to end.
// `name` is a DOM global in the standard library bento checks against, and a
// top-level const in a CommonJS file collides with it there and with nothing at all
// under Node. The checker gives the declaration one symbol and the reference another,
// so the reference reads as the library's global; the build hands the unit back and
// names the collision rather than answering for a binding the program never meant.
func TestAFileBindingNamedLikeADomGlobalHandsBack(t *testing.T) {
	_, err := compileJSSource(t, "const name = 'ev' + 1;\nconsole.log(name);\n")
	if err == nil {
		t.Fatal("a top-level binding of a DOM global name should hand back")
	}
	if !strings.Contains(err.Error(), "a top-level binding of name collides with the standard library") {
		t.Fatalf("expected the collision hand-back, got: %v", err)
	}
}

// TestAFileBindingShadowingAModeledGlobalHandsBack pins the same rule where getting
// it wrong would be a wrong answer rather than a refusal: a program that binds its
// own parseInt means its own function, and lowering the call to the runtime's would
// silently run something else.
func TestAFileBindingShadowingAModeledGlobalHandsBack(t *testing.T) {
	_, err := compileJSSource(t, "const parseInt = (s) => s.length;\nconsole.log(parseInt('12'));\n")
	if err == nil {
		t.Fatal("a top-level binding of parseInt should hand back")
	}
	if !strings.Contains(err.Error(), "a top-level binding of parseInt collides with the standard library") {
		t.Fatalf("expected the collision hand-back, got: %v", err)
	}
}

// TestABindingOfABentoGlobalStillLowers pins the exception the rule needs to be
// usable. `const process = require('node:process')` re-binds the object bento's own
// ambient declaration already names, so the reference resolving to that declaration
// is the answer the program wants and the program runs as it always did.
func TestABindingOfABentoGlobalStillLowers(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const process = require('node:process');\n"+
			"console.log(typeof process);\n")
	if want := "object\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
