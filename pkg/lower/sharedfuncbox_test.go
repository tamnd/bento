package lower

import (
	"strings"
	"testing"
)

// A function boxed at two sites has to be one value, since that is what the language
// says: the h handed to process.on and the h handed to process.off are the same object.
// Boxing builds a wrapper at each site, so the two sites are tied together by a key the
// runtime memoizes under. These pin which sites get a key and which deliberately do not.

// TestSharedFuncBoxTiesTwoSitesTogether pins the case the memo exists for: a module-level
// const holding a function, boxed once where it is registered and once where it is
// removed, carries the same key at both sites.
func TestSharedFuncBoxTiesTwoSitesTogether(t *testing.T) {
	src := "const h = (x: number) => { console.log(x); };\n" +
		"process.on(\"ping\", h);\n" +
		"process.off(\"ping\", h);\n"
	got := renderProgram(t, src)
	if n := strings.Count(got, `value.SharedFunc("0#h"`); n != 2 {
		t.Fatalf("want both sites keyed as 0#h, found %d:\n%s", n, got)
	}
}

// TestSharedFuncBoxKeysAFunctionDeclaration pins that a function declaration is the same
// case as a const arrow. It is bound once when the module runs, so the two forms a program
// picks between for the same job behave alike.
func TestSharedFuncBoxKeysAFunctionDeclaration(t *testing.T) {
	src := "function h(x: number) {\n  console.log(x);\n}\n" +
		"process.on(\"ping\", h);\n" +
		"process.off(\"ping\", h);\n"
	got := renderProgram(t, src)
	if n := strings.Count(got, `value.SharedFunc("0#h"`); n != 2 {
		t.Fatalf("want both sites keyed as 0#h, found %d:\n%s", n, got)
	}
}

// TestSharedFuncBoxSkipsAnInlineLiteral pins that a literal written at the site keeps its
// own wrapper. There is no second site to tie it to, and a key would only cost a map read.
func TestSharedFuncBoxSkipsAnInlineLiteral(t *testing.T) {
	src := "process.on(\"ping\", (x: number) => { console.log(x); });\n"
	got := renderProgram(t, src)
	if strings.Contains(got, "value.SharedFunc(") {
		t.Fatalf("an inline literal was given a shared box:\n%s", got)
	}
}

// TestSharedFuncBoxSkipsALetBinding pins the reason the pre-pass reads const and not let:
// a later assignment puts a different function behind the name, and a shared box would
// hand back the one the first site happened to see.
func TestSharedFuncBoxSkipsALetBinding(t *testing.T) {
	src := "let h = (x: number) => { console.log(x); };\n" +
		"process.on(\"ping\", h);\n" +
		"h = (x: number) => { console.log(x + 1); };\n" +
		"process.off(\"ping\", h);\n"
	got := renderProgram(t, src)
	if strings.Contains(got, "value.SharedFunc(") {
		t.Fatalf("a let binding was given a shared box:\n%s", got)
	}
}

// TestSharedFuncBoxSkipsAClosurePerCall pins the other exclusion: a function declared
// inside a function is a new closure on every call, so tying two of its boxes together
// would say two different functions are the same one.
func TestSharedFuncBoxSkipsAClosurePerCall(t *testing.T) {
	src := "function reg(n: number) {\n" +
		"  const h = () => { console.log(n); };\n" +
		"  process.on(\"ping\", h);\n" +
		"  process.off(\"ping\", h);\n" +
		"}\n" +
		"reg(1);\n"
	got := renderProgram(t, src)
	if strings.Contains(got, "value.SharedFunc(") {
		t.Fatalf("a per-call closure was given a shared box:\n%s", got)
	}
}

// TestProcessOnWithARunTimeEventNameLowers pins the relaxation that came with the emitter:
// an event named by an expression used to be a handback, and now goes through the process
// object like every other member call, so the name is read where the call happens.
func TestProcessOnWithARunTimeEventNameLowers(t *testing.T) {
	src := "const ev = \"pi\" + \"ng\";\n" +
		"process.on(ev, (x: number) => { console.log(x); });\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, `bentoProcess.Get(value.FromGoString("on"))`) {
		t.Fatalf("want the on call to go through the process object, got:\n%s", got)
	}
}

// TestProcessOnALiteralEventKeepsTheDirectCall pins that the literal form did not lose
// its direct lowering: a static event name still emits the runtime call by name, with no
// member read on the process object at run time.
func TestProcessOnALiteralEventKeepsTheDirectCall(t *testing.T) {
	src := "process.on(\"ping\", (x: number) => { console.log(x); });\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, `value.OnProcessEvent("ping"`) {
		t.Fatalf("want the direct OnProcessEvent call, got:\n%s", got)
	}
}
