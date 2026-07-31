package lower

import (
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// compileJS loads src as a JavaScript module, the mode a node builtin factory is
// checked in, so the checker performs the assignment-declaration analysis that adds
// an expando ClassName.member = ... as a static member of the constructor. A .ts
// root would instead report the member absent (2339), the reason a static function
// expando is a JS-mode feature.
func compileJS(t *testing.T, src string) *frontend.Program {
	t.Helper()
	yes, no := true, false
	prog, err := frontend.Load(frontend.LoadOptions{
		Dir:       "/",
		Roots:     []string{"/m.js"},
		Overrides: frontend.ConfigOverrides{AllowJS: &yes, CheckJS: &yes, NoImplicitAny: &no},
		FS:        realFS{files: map[string]string{"/m.js": src}},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// A JS module is checked without noImplicitAny, so the implicit-any diagnostics
	// an untyped parameter or function expression draws are tolerated the same way
	// the AOT front door tolerates them (build.go toleratedImplicitAny).
	for _, d := range prog.Diagnostics() {
		if d.Category != frontend.CategoryError {
			continue
		}
		switch d.Code {
		case 7005, 7006, 7008, 7010, 7011, 7018, 7019, 7031, 7043:
			continue
		}
		t.Fatalf("unexpected type error in JS snippet: %s", d.Message)
	}
	return prog
}

// renderUncheckedJS renders src the way the AOT front door builds a .js entry: allowJs
// on so the checker types the file, checkJs off so its JavaScript-specific reports never
// arise (build.go compileProgram). That is the mode a form TypeScript rejects but
// JavaScript runs has to be read in, arithmetic on a date being one: with checkJs on the
// checker reports 2362 and the renderer hands the whole unit back for it, which is the
// deliberate rule for a program TypeScript does not accept, while a plain .js program
// carries no such report and lowers.
func renderUncheckedJS(t *testing.T, src string) string {
	t.Helper()
	yes, no := true, false
	prog, err := frontend.Load(frontend.LoadOptions{
		Dir:       "/",
		Roots:     []string{"/m.js"},
		Overrides: frontend.ConfigOverrides{AllowJS: &yes, CheckJS: &no, NoImplicitAny: &no},
		FS:        realFS{files: map[string]string{"/m.js": src}},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	r.SetGoConstants(testGoConstants())
	r.SetGoErrorVars(testGoErrorVars())
	p, err := r.RenderProgram(entryFile(t, prog))
	if err != nil {
		t.Fatalf("RenderProgram: %v", err)
	}
	return p.Source
}

func renderExpandoJS(t *testing.T, src string) string {
	t.Helper()
	prog := compileJS(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	r.SetGoConstants(testGoConstants())
	r.SetGoErrorVars(testGoErrorVars())
	p, err := r.RenderProgram(entryFile(t, prog))
	if err != nil {
		t.Fatalf("RenderProgram: %v", err)
	}
	return p.Source
}

const staticExpandoSrc = `class C {
  x;
  constructor(x) { this.x = x; }
  val() { return this.x; }
}
C.make = function (x) { return new C(x); };
const c = C.make(42);
console.log(c.val());
`

// TestClassStaticExpandoLowers pins that a top-level ClassName.member = function ()
// registers as a static field: the class emits the package var, the assignment stores
// the closure into it, and the later ClassName.member(args) call lowers to that var
// applied, the same three shapes a body-declared static function field takes.
func TestClassStaticExpandoLowers(t *testing.T) {
	source := renderExpandoJS(t, staticExpandoSrc)
	if !strings.Contains(source, "var cMake func(value.Value) *C") {
		t.Errorf("static expando did not emit a package var closure:\n%s", source)
	}
	if !strings.Contains(source, "cMake = func(") {
		t.Errorf("static expando assignment did not store into the package var:\n%s", source)
	}
	if !strings.Contains(source, "cMake(value.Number(42))") {
		t.Errorf("call of the static expando did not lower to the var applied:\n%s", source)
	}
}

// TestClassStaticExpandoRuns runs the emitted Go end to end through the AOT path: the
// expando factory builds and calls an instance.
func TestClassStaticExpandoRuns(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, staticExpandoSrc))
	if got != "42\n" {
		t.Errorf("static expando ran wrong\n got: %q\nwant: %q", got, "42\n")
	}
}

// TestClassStaticNonFunctionExpandoLeftAlone pins the boundary: the discovery pass
// claims only a function-valued expando, so a value expando stays unregistered and
// its store hands back at the existing static-store boundary rather than being
// silently registered with a non-callable Go type.
func TestClassStaticNonFunctionExpandoLeftAlone(t *testing.T) {
	const src = `class C {
  x;
  constructor(x) { this.x = x; }
}
C.tag = 5;
console.log(String(C.tag));
`
	prog := compileJS(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	r.SetGoConstants(testGoConstants())
	r.SetGoErrorVars(testGoErrorVars())
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		t.Fatalf("a value expando lowered; want a hand-back at the static-store boundary")
	}
	if !strings.Contains(err.Error(), "storing into static .tag of class C is a later slice") {
		t.Errorf("hand-back reason %q does not name the static-store boundary", err.Error())
	}
}
