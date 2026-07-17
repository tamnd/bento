package lower

import (
	"errors"
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// TestDynamicImportComputedSpecifierHandsBack pins the honest ceiling: a dynamic
// import whose specifier is computed at run time cannot be resolved to a
// compiled module by an ahead-of-time compiler, so it hands back with the reason
// naming the computed side of the split rather than the misleading
// function-value reason the callee path would give.
func TestDynamicImportComputedSpecifierHandsBack(t *testing.T) {
	const src = `
async function f(spec: string): Promise<void> {
  const p = import(spec);
  void p;
}
f("a");
`
	if reason := renderProgramHandBack(t, src); reason != dynImportComputedReason {
		t.Fatalf("reason = %q, want %q", reason, dynImportComputedReason)
	}
}

// TestDynamicImportConcatSpecifierHandsBack proves a specifier built by
// concatenation is computed too, not static, so it takes the computed reason.
func TestDynamicImportConcatSpecifierHandsBack(t *testing.T) {
	const src = `
async function f(name: string): Promise<void> {
  const p = import("./" + name);
  void p;
}
f("a");
`
	if reason := renderProgramHandBack(t, src); reason != dynImportComputedReason {
		t.Fatalf("reason = %q, want %q", reason, dynImportComputedReason)
	}
}

// TestDynamicImportStaticSpecifierHandsBack proves a bare static import used as a
// value hands back: the specifier names a compiled sibling, but the promise it
// returns has no runtime value here, so `const p = import("./mod")` consumed as a
// value takes the value-use reason rather than the computed ceiling. Only the
// awaited-and-member-consumed form lowers (the run tests below).
func TestDynamicImportStaticSpecifierHandsBack(t *testing.T) {
	prog := compileWithSibling(t,
		`
async function f(): Promise<void> {
  const p = import("./mod");
  void p;
}
f();
`,
		`export function inc(n: number): number { return n + 1; }`)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(rootFile(t, prog, "/m.ts"))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if nyl.Reason != dynImportStaticReason {
		t.Fatalf("reason = %q, want %q", nyl.Reason, dynImportStaticReason)
	}
}

// TestStaticDynamicImportAwaitedNamespaceCallRuns is the positive slice: an
// awaited static dynamic import whose binding is used only through a member call,
// const m = await import("./mod"); m.inc(1), lowers and runs. The binding carries
// no Go var; the declaration lowers to the await suspension alone, and m.inc(1)
// resolves to the composed sibling's package-level Go func, so the program prints
// the incremented value after the synchronous run drains.
func TestStaticDynamicImportAwaitedNamespaceCallRuns(t *testing.T) {
	prog := compileWithSibling(t,
		`
async function f(): Promise<number> {
  const m = await import("./mod");
  return m.inc(1);
}
f().then((v) => console.log("v:" + v));
console.log("sync");
`,
		`export function inc(n: number): number { return n + 1; }`)
	got := goRunSource(t, renderModulesSource(t, prog, "/m.ts"))
	want := "sync\nv:2\n"
	if got != want {
		t.Fatalf("awaited dynamic-import member call ran wrong\n got: %q\nwant: %q", got, want)
	}
}

// TestStaticDynamicImportAwaitedNamespaceValueReadRuns proves a member value read
// off the awaited namespace resolves too: const g = m.inc reads the sibling's Go
// func as a bare value, and calling g runs it, the same resolution a static
// namespace import gives m.inc.
func TestStaticDynamicImportAwaitedNamespaceValueReadRuns(t *testing.T) {
	prog := compileWithSibling(t,
		`
async function f(): Promise<number> {
  const m = await import("./mod");
  const g = m.inc;
  return g(41);
}
f().then((v) => console.log("v:" + v));
`,
		`export function inc(n: number): number { return n + 1; }`)
	got := goRunSource(t, renderModulesSource(t, prog, "/m.ts"))
	want := "v:42\n"
	if got != want {
		t.Fatalf("awaited dynamic-import member value read ran wrong\n got: %q\nwant: %q", got, want)
	}
}

// TestStaticDynamicImportNamespaceUsedAsValueHandsBack pins the ceiling: the same
// awaited binding used as a whole value, returned or passed on, has no runtime
// namespace to stand for, so the unit hands back at the use site rather than
// miscompile. The declaration recognizer never makes an unbacked value; it only
// drops the binding for the member-consumed form.
func TestStaticDynamicImportNamespaceUsedAsValueHandsBack(t *testing.T) {
	prog := compileWithSibling(t,
		`
async function f(): Promise<unknown> {
  const m = await import("./mod");
  return m;
}
f();
`,
		`export function inc(n: number): number { return n + 1; }`)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(rootFile(t, prog, "/m.ts"))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
}

// renderModulesSource assembles the entry source file at entryPath with every
// other lowerable source as a composed dep, the way the build's entryAndDeps does,
// and renders them to one Go program. It lets a module test build and run a
// multi-file program without the single-entry expectation renderProgram carries.
func renderModulesSource(t *testing.T, prog *frontend.Program, entryPath string) string {
	t.Helper()
	var entry frontend.Node
	var deps []frontend.Node
	for _, sf := range prog.SourceFiles() {
		if sf.File().Kind == frontend.FileDTS {
			continue
		}
		if sf.File().Path == entryPath {
			entry = sf
			continue
		}
		deps = append(deps, sf)
	}
	if entry == nil {
		t.Fatalf("no entry source at %q", entryPath)
	}
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	r.SetGoConstants(testGoConstants())
	r.SetGoErrorVars(testGoErrorVars())
	p, err := r.RenderProgramModules(entry, deps)
	if err != nil {
		t.Fatalf("RenderProgramModules: %v", err)
	}
	return p.Source
}

// rootFile returns the loaded source file at path, so a multi-file program can
// pick its entry explicitly rather than through entryFile, which expects a
// single non-library source.
func rootFile(t *testing.T, prog *frontend.Program, path string) frontend.Node {
	t.Helper()
	for _, sf := range prog.SourceFiles() {
		if sf.File().Path == path {
			return sf
		}
	}
	t.Fatalf("no source file at %q", path)
	return nil
}

// compileWithSibling loads an entry that imports a sibling module, so a
// string-literal specifier resolves through the checker instead of drawing a
// missing-module error. The entry is the root; the sibling exists only to make
// the specifier statically resolvable.
func compileWithSibling(t *testing.T, entry, sibling string) *frontend.Program {
	t.Helper()
	prog, err := frontend.Load(frontend.LoadOptions{
		Dir:   "/",
		Roots: []string{"/m.ts"},
		FS: realFS{files: map[string]string{
			"/m.ts":   entry,
			"/mod.ts": sibling,
		}},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, d := range prog.Diagnostics() {
		if d.Category == frontend.CategoryError {
			t.Fatalf("unexpected type error in snippet: %s", d.Message)
		}
	}
	return prog
}
