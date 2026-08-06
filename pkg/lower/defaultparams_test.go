package lower

import (
	"strings"
	"testing"
)

// A closure parameter with a default value used to hand back outright, which made
// "function parameter with a default value is a later slice" the single largest refusal
// in the Node compatibility suite. Every module body a require lowers is a closure, so
// every module-level `function f(a, b = ...)` in the suite landed here.
//
// The answer for a parameter whose Go slot is a value.Value is that the callee fills its
// own default at body entry. A value.Value holds the undefined an omitted argument
// binds, buildCall already passes value.Undefined for a dynamic argument the call does
// not supply, and value.Arg already yields undefined for a position a boxed call did not
// supply, so both ends already agree and nothing has to be threaded to the call site.
// `if p.IsUndefined() { p = <default> }` is also the language's own rule spelled
// straight: a default fires for an explicit undefined too, and it is evaluated in the
// callee's scope, where a default that reads an earlier parameter can see it.
//
// A parameter whose slot is a static Go type has no undefined to test and keeps its
// handback; that is the next slice.

// TestClosureDynamicDefaultFillsAtBodyEntry pins the fill on a function expression, the
// form a module-level function declaration takes once a require wraps the module body.
func TestClosureDynamicDefaultFillsAtBodyEntry(t *testing.T) {
	src := "const f = function (a: any, b: any = \"d\"): string { return String(a) + \"|\" + String(b); };\nconst box = { f };\nconsole.log(box.f(1));\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "if b.IsUndefined() {") {
		t.Errorf("a defaulted closure parameter emitted:\n%s\nwant an entry fill guarded by IsUndefined", got)
	}
}

// TestConciseArrowDynamicDefaultFillsAtBodyEntry pins the same for a concise-body arrow,
// whose func literal grows a block so the fill can stand above the single return.
func TestConciseArrowDynamicDefaultFillsAtBodyEntry(t *testing.T) {
	src := "const f = (a: any, b: any = \"d\"): string => String(a) + \"|\" + String(b);\nconst box = { f };\nconsole.log(box.f(1));\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "if b.IsUndefined() {") {
		t.Errorf("a defaulted arrow parameter emitted:\n%s\nwant an entry fill guarded by IsUndefined", got)
	}
}

// TestDefaultReadingAnEarlierParameterFillsInTheCallee pins what the body-entry fill
// gets for free. A default that reads an earlier parameter is evaluated in the callee's
// scope, so no call site could reconstruct it; filling at body entry is the one place
// that parameter is bound.
func TestDefaultReadingAnEarlierParameterFillsInTheCallee(t *testing.T) {
	src := "const f = function (a: any, b: any = a): string { return String(a) + \"|\" + String(b); };\nconst box = { f };\nconsole.log(box.f(1));\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "if b.IsUndefined() {") || !strings.Contains(got, "b = a") {
		t.Errorf("a default reading an earlier parameter emitted:\n%s\nwant an entry fill assigning a", got)
	}
}

// TestBoxedClosureWithADefaultPassesTheArgumentThrough pins the other end. A function
// with a defaulted tail flowing into a dynamic slot is wrapped in a value.NewFunc that
// hands value.Arg straight to the lowered func: the wrapper does no arity work, and the
// undefined value.Arg answers for a position the call did not supply is exactly what the
// body's fill tests for. Before this the whole box handed back.
func TestBoxedClosureWithADefaultPassesTheArgumentThrough(t *testing.T) {
	src := "const f = function (a: any, b: any = \"d\"): string { return String(a) + \"|\" + String(b); };\nconst g: any = f;\nconsole.log(g(1));\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "value.NewFunc(") || !strings.Contains(got, "value.Arg(__a, 1)") {
		t.Errorf("boxing a defaulted closure emitted:\n%s\nwant a wrapper passing value.Arg(__a, 1) through", got)
	}
}

// TestClosureStaticDefaultStillHandsBack pins the boundary. A defaulted parameter whose
// Go slot is a float64 has nowhere to hold undefined, so it cannot fill itself and the
// refusal stands rather than emit a call that silently reads a Go zero.
func TestClosureStaticDefaultStillHandsBack(t *testing.T) {
	src := "const f = function (a: number, b: number = 2): number { return a + b; };\nconst box = { f };\nconsole.log(box.f(1));\n"
	renderProgramHandBack(t, src)
}

// TestShorthandEscapeKeepsAnArrowsDefault pins a wrong-answer bug this slice closes.
// collectArrowDefaults drops an arrow's defaults when every use is a direct call, since
// each call site can fill them. Its escape walk read the symbol at each identifier, and
// for an object-literal shorthand, `{ f }`, that symbol is the property the member
// declares rather than the binding it reads, so the escape was invisible: the defaults
// were dropped and `const box = { f }; box.f(1)` passed undefined straight through and
// printed it. The walk now credits a shorthand to the binding it reads, so this arrow
// keeps its default and fills it at body entry.
func TestShorthandEscapeKeepsAnArrowsDefault(t *testing.T) {
	src := "const f = (a: any, b: any = \"d\"): string => String(a) + \"|\" + String(b);\nconst box = { f };\nconsole.log(box.f(1));\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "if b.IsUndefined() {") {
		t.Errorf("an arrow escaping through a shorthand emitted:\n%s\nwant its default kept as an entry fill", got)
	}
}

// TestDirectOnlyArrowStillDropsItsDefault pins that the escape walk did not get so
// conservative that it lost the case it was written for. An arrow whose every use is a
// direct call still drops its default and lets each call site fill it, which keeps the
// parameter's static Go type.
func TestDirectOnlyArrowStillDropsItsDefault(t *testing.T) {
	src := "const f = (a: number, b: number = 2): number => a + b;\nconsole.log(f(1), f(3, 4));\n"
	got := renderProgram(t, src)
	if strings.Contains(got, "IsUndefined()") {
		t.Errorf("a direct-only arrow emitted:\n%s\nwant its default filled at the call site", got)
	}
	if !strings.Contains(got, "f(1, 2)") {
		t.Errorf("a direct-only arrow emitted:\n%s\nwant the omitted argument filled with the default", got)
	}
}

// TestClosureDefaultsRun builds and runs the shapes against the Node oracle: a supplied
// argument wins, an omitted one takes the default, and an explicitly passed undefined
// takes it too, which is the rule that makes the body-entry fill the right spelling
// rather than a call-site one.
func TestClosureDefaultsRun(t *testing.T) {
	skipIfShort(t)
	const src = `const f = function (a: any, b: any = "d"): string {
  return String(a) + "|" + String(b);
};
const g: any = f;
const box = { f };
console.log(f(1), f(2, "z"), f(3, undefined));
console.log(g(4), g(5, "y"));
console.log(box.f(6), box.f(7, "x"));
`
	got := runProgramGo(t, src)
	want := "1|d 2|z 3|d\n4|d 5|y\n6|d 7|x\n"
	if got != want {
		t.Fatalf("defaulted closures printed %q, want %q", got, want)
	}
}
