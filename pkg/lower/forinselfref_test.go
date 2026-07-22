package lower

import (
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// loadForInSelfRef loads a for...in self-reference snippet, admitting 7022 (a binding
// that implicitly has type any because it is referenced in its own initializer). The
// ts-compat corpus reaches these cases under // @strict: false, where noImplicitAny is
// off and 7022 is never raised; this loader mirrors that admission so the same program
// reaches the renderer here it reaches through build.Compile.
func loadForInSelfRef(t *testing.T, src string) *frontend.Program {
	t.Helper()
	prog, err := frontend.Load(frontend.LoadOptions{
		Dir:   "/",
		Roots: []string{"/m.ts"},
		FS:    realFS{files: map[string]string{"/m.ts": src}},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, d := range prog.Diagnostics() {
		if d.Category != frontend.CategoryError || d.Code == 7022 {
			continue
		}
		t.Fatalf("unexpected type error in snippet: %s", d.Message)
	}
	return prog
}

func renderForInSelfRef(t *testing.T, src string) string {
	t.Helper()
	prog := loadForInSelfRef(t, src)
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

// TestForInSelfReferentialIterable pins that a for...in whose iterable references the
// loop variable itself, `for (var of in of)`, declares the binding undefined ahead of
// the loop so the self-reference names a real Go variable. The `var` binding is
// initialized before the head is evaluated, so the enumerated object is undefined and
// the loop runs zero turns.
func TestForInSelfReferentialIterable(t *testing.T) {
	out := renderForInSelfRef(t, "for (var of in of) { }\n")
	if !strings.Contains(out, "var of value.Value = value.Undefined") {
		t.Fatalf("self-referential for...in did not declare the hoisted binding:\n%s", out)
	}
	if !strings.Contains(out, "for range of.ForInKeys().Elems()") {
		t.Fatalf("self-referential for...in did not enumerate the hoisted value:\n%s", out)
	}
}

// TestForInSelfReferentialIterableRuns builds and runs the shape to prove the enumerated
// undefined yields no keys and the program completes: a statement after the loop runs,
// so the loop neither panics nor spins.
func TestForInSelfReferentialIterableRuns(t *testing.T) {
	skipIfShort(t)
	out := renderForInSelfRef(t, "for (var of in of) { }\nconsole.log(\"done\");\n")
	if got, want := goRunSource(t, out), "done\n"; got != want {
		t.Fatalf("self-referential for...in run mismatch:\n got %q\nwant %q", got, want)
	}
}
