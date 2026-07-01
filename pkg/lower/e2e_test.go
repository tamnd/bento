package lower_test

import (
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/lower"
	"github.com/tamnd/bento/pkg/partition"
)

// e2eFS is an in-memory FileSystem for the end-to-end test.
type e2eFS struct{ files map[string]string }

func (m e2eFS) ReadFile(path string) (string, bool) { s, ok := m.files[path]; return s, ok }
func (m e2eFS) FileExists(path string) bool         { _, ok := m.files[path]; return ok }

func (m e2eFS) DirectoryExists(path string) bool {
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

// TestEndToEndRealSourceCompilesAndLowers runs a real TypeScript function through
// the whole M4 front half: the real checker types it, the partitioner rules it
// Compiled, and the type renderer lowers its signature to Go types. Nothing here
// is a stand-in; the checker is the real one, reached through the fork's shim.
func TestEndToEndRealSourceCompilesAndLowers(t *testing.T) {
	prog, err := frontend.Load(frontend.LoadOptions{
		Dir:   "/",
		Roots: []string{"/geo.ts"},
		FS: e2eFS{files: map[string]string{
			"/geo.ts": "export function area(width: number, height: number): number { return width * height; }\n",
		}},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, d := range prog.Diagnostics() {
		if d.Category == frontend.CategoryError {
			t.Fatalf("unexpected type error: %s", d.Message)
		}
	}

	// Partition: the function is fully typed with a plain body, so it compiles.
	var area partition.Unit
	var verdict partition.Verdict
	for _, r := range partition.New(prog).PassA() {
		if r.Unit.Name == "area" {
			area, verdict = r.Unit, r.Verdict
		}
	}
	if area.Name != "area" {
		t.Fatal("partitioner did not surface the area unit")
	}
	if verdict != partition.Compiled {
		t.Fatalf("area verdict = %v, want Compiled", verdict)
	}

	// Lower: the compiled unit's signature renders to Go types, number to float64.
	sig, ok := prog.SignatureAt(area.Root)
	if !ok {
		t.Fatal("no signature for the area unit")
	}
	r := lower.NewRenderer(prog)
	for _, param := range sig.Params {
		got, err := r.RenderType(param.Type)
		if err != nil {
			t.Fatalf("RenderType(%s): %v", param.Name, err)
		}
		if got != "float64" {
			t.Errorf("param %s rendered as %q, want float64", param.Name, got)
		}
	}
	ret, err := r.RenderType(sig.Return)
	if err != nil {
		t.Fatalf("RenderType(return): %v", err)
	}
	if ret != "float64" {
		t.Errorf("return rendered as %q, want float64", ret)
	}
}
