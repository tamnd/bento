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
// program asks it. It named crypto until the WebCrypto surface was built
// (webcrypto.go), and WebSocket is the same case crypto was: a global Node installs
// and bento has not modeled.
func TestUnhostedGlobalStillRefuses(t *testing.T) {
	src := "const f = (x: any) => { console.log(typeof x); };\n" +
		"f(WebSocket);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "the ambient global WebSocket read as a value") {
		t.Fatalf("reason = %q, want the WebSocket refusal", reason)
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

// TestAMemberNameIsNotACollision pins the other half of the collision test. The name
// in console.log(x) is an identifier whose symbol is Console's method, declared only
// in a .d.ts, so a file that also binds a top-level `log` looks like a collision to a
// walk that reads every identifier. It is not one: a property and a binding of the
// same spelling are unrelated, and refusing here took seven conformance fixtures down.
func TestAMemberNameIsNotACollision(t *testing.T) {
	src := "const log: string[] = [];\n" +
		"log.push(\"a\");\n" +
		"console.log(log.join(\",\"));\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "value.ConsoleLog") {
		t.Fatalf("want the unit to lower rather than hand back:\n%s", got)
	}
}

// TestAStaticMemberOnAHostedGlobalStillLowersThroughRenderFunc pins the single-function
// entry point. It runs no program pre-pass of its own, so without one of its own every
// global inside the body reads as a value and Number.MAX_VALUE loses the static member
// path it has always had.
func TestAStaticMemberOnAHostedGlobalStillLowersThroughRenderFunc(t *testing.T) {
	got := renderProgram(t, "function f(): number { return Number.MAX_VALUE; }\nconsole.log(f());\n")
	if !strings.Contains(got, "value.NumberMaxValue") {
		t.Fatalf("want the static member:\n%s", got)
	}
}

// The Node classes bento's runtime builds are hosted the same way the codec and
// scheduling globals are, which is what lets test/common/index.js:272 lower. That
// line names twelve globals in one Set literal, and AbortController was the first of
// them with no value form, so it was the first refusal for 1004 of the suite's 3822
// tests.

// TestAClassGlobalReadsAsItsValue pins the name at the head of that list. Every
// other entry already had a value form, so this one line is what the whole family
// turned on.
func TestAClassGlobalReadsAsItsValue(t *testing.T) {
	src := "const f = (x: any) => { console.log(typeof x); };\n" +
		"f(AbortController);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.GlobalValue("AbortController")`) {
		t.Fatalf("want AbortController read as its runtime value:\n%s", got)
	}
}

// TestTheKnownGlobalsSetLowers is the gate itself, the twelve names test/common
// collects to tell its own leaks from the host's. It is pinned whole rather than one
// name at a time because what mattered was that every entry answered, not that any
// particular one did.
func TestTheKnownGlobalsSetLowers(t *testing.T) {
	src := `const knownGlobals = new Set([
  AbortController,
  atob,
  btoa,
  clearImmediate,
  clearInterval,
  clearTimeout,
  global,
  setImmediate,
  setInterval,
  setTimeout,
  queueMicrotask,
  structuredClone,
]);
console.log(String(knownGlobals.size));
`
	got := renderUncheckedJS(t, src)
	for _, want := range []string{
		`value.GlobalValue("AbortController")`,
		`value.GlobalValue("atob")`,
		`value.GlobalValue("structuredClone")`,
		bentoGlobalThisName,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("the known-globals set is missing %s:\n%s", want, got)
		}
	}
}

// TestConstructingAHostedClassGlobalStaysDirect pins that hosting the value form did
// not take the construction away from the static path. new AbortController() still
// lowers to the runtime constructor, which is the shape a program actually writes;
// the value form is only what naming it hands over.
func TestConstructingAHostedClassGlobalStaysDirect(t *testing.T) {
	got := renderProgram(t, "const c = new AbortController();\nc.abort();\nconsole.log(typeof c);\n")
	if !strings.Contains(got, "value.NewAbortController()") {
		t.Fatalf("want the direct constructor:\n%s", got)
	}
	if strings.Contains(got, `value.GlobalValue("AbortController")`) {
		t.Fatalf("the construction went through the value form:\n%s", got)
	}
}

// TestAStaticOnAClassGlobalStaysRefused pins the ceiling the value form is under. A
// hosted global carries its name and its call and nothing else, so a member read off
// one is a read of a value that has no members, and it keeps refusing at compile time
// naming the global rather than answering the undefined the value holds.
func TestAStaticOnAClassGlobalStaysRefused(t *testing.T) {
	reason := renderProgramHandBack(t, "console.log(AbortController.name);\n")
	if !strings.Contains(reason, "the ambient global AbortController read as a value") {
		t.Fatalf("reason = %q, want the AbortController refusal", reason)
	}
}

// TestTheCryptoGateLowers is the gate the crypto slice closed, the four reads
// test/common runs behind its hasCrypto guard. The guard is false on a bento binary,
// since bento reports no openssl version, but the block still has to compile, and
// until the WebCrypto object was built the first of those four lines was the first
// refusal for 1016 of the suite's tests.
func TestTheCryptoGateLowers(t *testing.T) {
	src := `const knownGlobals = new Set();
knownGlobals.add(globalThis.crypto);
knownGlobals.add(globalThis.Crypto);
knownGlobals.add(globalThis.CryptoKey);
knownGlobals.add(globalThis.SubtleCrypto);
console.log(String(knownGlobals.size));
`
	got := renderUncheckedJS(t, src)
	for _, want := range []string{
		"value.CryptoValue()",
		`value.GlobalValue("Crypto")`,
		`value.GlobalValue("CryptoKey")`,
		`value.GlobalValue("SubtleCrypto")`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("the crypto gate is missing %s:\n%s", want, got)
		}
	}
}

// TestCryptoReadsAsTheRuntimeObject pins which of the two shapes the crypto name
// takes. It is not a hosted global: a hosted global is a name and a call and
// deliberately excludes a member receiver, and every use of crypto is a receiver, so
// it reads as the whole runtime object the way process, console and Buffer do.
func TestCryptoReadsAsTheRuntimeObject(t *testing.T) {
	got := renderUncheckedJS(t, "console.log(typeof crypto);\n")
	if !strings.Contains(got, "value.CryptoValue()") {
		t.Fatalf("crypto did not read as the runtime object:\n%s", got)
	}
	if strings.Contains(got, `value.GlobalValue("crypto")`) {
		t.Fatalf("crypto took the hosted-global route:\n%s", got)
	}
}

// TestACryptoMemberCallDispatchesDynamically pins the reason crypto needs an entry in
// isDynamic. The standard library types the name as a Crypto, an interface bento
// interns no Go shape for, so without the entry the member call would try to resolve
// against that shape instead of dispatching off the object.
func TestACryptoMemberCallDispatchesDynamically(t *testing.T) {
	got := renderUncheckedJS(t, "const a = new Uint8Array(4);\ncrypto.getRandomValues(a);\nconsole.log(crypto.randomUUID().length);\n")
	if !strings.Contains(got, "value.CryptoValue()") {
		t.Fatalf("the receiver did not reach the crypto object:\n%s", got)
	}
	for _, want := range []string{`"getRandomValues"`, `"randomUUID"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("the member call did not dispatch by name on %s:\n%s", want, got)
		}
	}
}

// TestALocalBindingNamedCryptoIsItsOwn pins the shadow rule for the name. A binding a
// function declares is not the ambient global, so it reads the slot the program
// wrote; the top-level case is a different matter and still refuses, since the
// checker resolves every reference in the file to the library's symbol.
func TestALocalBindingNamedCryptoIsItsOwn(t *testing.T) {
	got := renderUncheckedJS(t, "function f() { const crypto = 3; return crypto; }\nconsole.log(f());\n")
	if strings.Contains(got, "value.CryptoValue()") {
		t.Fatalf("a local binding named crypto read the crypto object:\n%s", got)
	}
}

// TestTheCryptoClassNamesRefuseTheirCall pins the ceiling the three class names sit
// under, the same one AbortController sits under: the value carries a name and a call
// and no statics, so a member read off one keeps refusing at compile time.
func TestTheCryptoClassNamesRefuseTheirCall(t *testing.T) {
	reason := renderProgramHandBack(t, "console.log(Crypto.name);\n")
	if !strings.Contains(reason, "the ambient global Crypto read as a value") {
		t.Fatalf("reason = %q, want the Crypto refusal", reason)
	}
}

// TestCryptoRunsAsTheWebCryptoGlobal is the behaviour end to end: the identity the
// known-globals set rests on, the two members that are real, and the refusal the
// subtle surface carries.
func TestCryptoRunsAsTheWebCryptoGlobal(t *testing.T) {
	skipIfShort(t)
	src := `console.log(typeof crypto, crypto === globalThis.crypto);
console.log(typeof crypto.subtle, typeof crypto.randomUUID, typeof crypto.subtle.digest);
const id = crypto.randomUUID();
console.log(id.length, id.charAt(14));
const bytes = new Uint8Array(16);
console.log(crypto.getRandomValues(bytes) === bytes);
try {
  crypto.subtle.digest("SHA-256", bytes);
} catch (e) {
  console.log(e.name);
}
`
	got := runJS(t, src)
	want := "object true\nobject function function\n36 4\ntrue\nTypeError\n"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
