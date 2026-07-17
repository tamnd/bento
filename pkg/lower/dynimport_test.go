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

// TestDynamicImportStaticSpecifierHandsBack proves a string-literal specifier
// naming a real sibling module is classified as static: it names a compiled
// module the loader could resolve, so it takes the static reason marking the
// compiled-module load as a later slice, not the computed ceiling.
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
