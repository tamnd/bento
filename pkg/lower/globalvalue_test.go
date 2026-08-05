package lower

import (
	"strings"
	"testing"
)

// An ambient global named rather than called used to hand the whole unit back. The
// globals bento hosts now read as the interned value the runtime holds for the name,
// and the ones it does not host go on refusing at compile time. These pin both
// halves, and the file-scope rule that decides whether a name is a global at all.

// TestHostedGlobalReadsAsItsValue pins the shape the suite writes constantly: a
// constructor global handed to a helper as a value, which needs an object to hand
// over rather than a Go symbol that was never declared.
func TestHostedGlobalReadsAsItsValue(t *testing.T) {
	src := "const f = (x: any) => { console.log(typeof x); };\n" +
		"f(Symbol);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.GlobalValue("Symbol")`) {
		t.Fatalf("want Symbol read as its runtime value:\n%s", got)
	}
}

// TestHostedFunctionGlobalReadsAsItsValue pins the other half of the family, a bare
// function global used as a value rather than called. It reaches a different
// handback in the identifier path than a constructor does, so both are pinned.
func TestHostedFunctionGlobalReadsAsItsValue(t *testing.T) {
	src := "const f = (x: any) => { console.log(typeof x); };\n" +
		"f(atob);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.GlobalValue("atob")`) {
		t.Fatalf("want atob read as its runtime value:\n%s", got)
	}
}

// TestUnhostedGlobalStillRefuses pins the rule the hosted table exists under: a
// global whose behavior the runtime has not built keeps its compile-time refusal,
// rather than being handed over as a value that answers undefined for everything a
// program asks it.
func TestUnhostedGlobalStillRefuses(t *testing.T) {
	src := "const f = (x: any) => { console.log(typeof x); };\n" +
		"f(crypto);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "the ambient global crypto read as a value") {
		t.Fatalf("reason = %q, want the crypto refusal", reason)
	}
}

// TestAFileBindingThatCollidesWithALibraryGlobalHandsBack pins the file-scope rule.
// `name` is a DOM global in the standard library bento checks against, and a
// top-level const in a script does not shadow it, it collides with it: the
// declaration gets the file's symbol and every reference gets the library's. Reading
// that reference as the global answers for a binding the program never meant, so the
// unit hands back and says so.
func TestAFileBindingThatCollidesWithALibraryGlobalHandsBack(t *testing.T) {
	src := "const name = \"ev\" + 1;\n" +
		"console.log(name);\n"
	reason := renderUncheckedJSHandBack(t, src)
	if !strings.Contains(reason, "a top-level binding of name collides with the standard library") {
		t.Fatalf("reason = %q, want the collision refusal", reason)
	}
}

// TestAFileBindingThatCollidesWithAModeledGlobalHandsBack pins the same rule where it
// matters most. A program that binds its own parseInt means its own function, and
// lowering the call to the runtime's would be a wrong answer rather than a refusal.
func TestAFileBindingThatCollidesWithAModeledGlobalHandsBack(t *testing.T) {
	src := "const parseInt = (s) => s.length;\n" +
		"console.log(parseInt(\"abc\"));\n"
	reason := renderUncheckedJSHandBack(t, src)
	if !strings.Contains(reason, "a top-level binding of parseInt collides with the standard library") {
		t.Fatalf("reason = %q, want the collision refusal", reason)
	}
}

// TestABindingOfABentoGlobalIsNotACollision pins the exception the rule needs to be
// usable. `const process = require('node:process')` re-binds the object bento's own
// ambient declaration already names, so the reference resolving to that declaration
// is the answer the program wants and the unit lowers as it always did.
func TestABindingOfABentoGlobalIsNotACollision(t *testing.T) {
	src := "const process = require(\"node:process\");\n" +
		"console.log(process.argv.length > 0);\n"
	got := renderUncheckedJS(t, src)
	if !strings.Contains(got, `value.RequireBuiltin("node:process")`) {
		t.Fatalf("want the binding to lower rather than hand back:\n%s", got)
	}
}

// TestAGlobalIsStillAGlobalInAFileThatDoesNotBindIt pins the other side of the scope
// rule, so the collision test cannot pass by turning every global off.
func TestAGlobalIsStillAGlobalInAFileThatDoesNotBindIt(t *testing.T) {
	got := renderProgram(t, "console.log(parseInt(\"12\", 10));\n")
	if !strings.Contains(got, "value.ParseInt") {
		t.Fatalf("want the runtime parseInt:\n%s", got)
	}
}

// TestAStaticMemberOnAHostedGlobalStaysStaticThroughTheBoxedPass pins the ordering the
// two pre-passes need. isBoxedChain is asked about the same expressions twice, once by
// the boxed-signature pass and once while lowering, and the answer has to be the same
// both times. With the receiver set collected after the boxed pass, Object.keys(m) read
// as a box there and as a static member read here, so the loop emitted a bento string
// into a variable the body had already been told was a value.Value.
func TestAStaticMemberOnAHostedGlobalStaysStaticThroughTheBoxedPass(t *testing.T) {
	src := "const m = JSON.parse('{\"a\":1}') as Record<string, number>;\n" +
		"for (const k of Object.keys(m)) { console.log(k, m[k]); }\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "value.ConsoleFormat(value.StringValue(k)") {
		t.Fatalf("want the loop key to stay a bento string:\n%s", got)
	}
	if strings.Contains(got, `value.GlobalValue("Object")`) {
		t.Fatalf("want the receiver to take the static member path:\n%s", got)
	}
}
