package partition

import (
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// realFS is an in-memory FileSystem for compiling a snippet through the real
// checker, the partition package's own copy since frontend's helper is
// unexported.
type realFS struct{ files map[string]string }

func (m realFS) ReadFile(path string) (string, bool) { s, ok := m.files[path]; return s, ok }
func (m realFS) FileExists(path string) bool         { _, ok := m.files[path]; return ok }

func (m realFS) DirectoryExists(path string) bool {
	prefix := path
	if prefix != "" && prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	for name := range m.files {
		if len(name) > len(prefix) && name[:len(prefix)] == prefix {
			return true
		}
	}
	return false
}

func partitionReal(t *testing.T, src string) []Result {
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
		if d.Category == frontend.CategoryError {
			t.Fatalf("unexpected type error: %s", d.Message)
		}
	}
	return New(prog).PassA()
}

func resultNamed(results []Result, name string) (Result, bool) {
	for _, r := range results {
		if r.Unit.Name == name {
			return r, true
		}
	}
	return Result{}, false
}

// TestRealFullyTypedFunctionCompiles drives the partitioner over a real compile:
// a function with concrete parameter and return types and a plain arithmetic
// body classifies as Compiled.
func TestRealFullyTypedFunctionCompiles(t *testing.T) {
	results := partitionReal(t, "export function taxed(cents: number, rate: number): number { return cents * rate; }\n")
	r, ok := resultNamed(results, "taxed")
	if !ok {
		t.Fatalf("no unit named taxed in %v", results)
	}
	if r.Verdict != Compiled {
		t.Fatalf("taxed verdict = %v, reasons = %v, want Compiled", r.Verdict, r.Reasons)
	}
}

// TestRealEvalIsInterpreted proves a hard blocker survives a real compile: a
// function that calls eval cannot be compiled and is not a speculation
// candidate.
func TestRealEvalIsInterpreted(t *testing.T) {
	results := partitionReal(t, "export function runs(): number { return eval(\"1\"); }\n")
	r, ok := resultNamed(results, "runs")
	if !ok {
		t.Fatalf("no unit named runs in %v", results)
	}
	if r.Verdict != Interpreted {
		t.Fatalf("runs verdict = %v, want Interpreted", r.Verdict)
	}
	if r.SpeculationCandidate() {
		t.Error("eval is a hard blocker; the unit must not be a speculation candidate")
	}
}
