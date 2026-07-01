package frontend

import "testing"

// mapFS is an in-memory FileSystem for exercising Load without touching disk.
type mapFS struct{ files map[string]string }

func (m mapFS) ReadFile(path string) (string, bool) { s, ok := m.files[path]; return s, ok }
func (m mapFS) FileExists(path string) bool         { _, ok := m.files[path]; return ok }

func (m mapFS) DirectoryExists(path string) bool {
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

// TestLoadCompilesRealProgram drives the whole frontend: Load builds a real
// typescript-go program through the fork's shim, follows a relative import, and
// the resulting Program answers a type query with the checker's own answer.
func TestLoadCompilesRealProgram(t *testing.T) {
	fs := mapFS{files: map[string]string{
		"/app/main.ts": "import { area } from \"./geo\";\nexport const a = area(2);\n",
		"/app/geo.ts":  "export function area(r: number): number { return 3.14 * r * r; }\n",
	}}

	prog, err := Load(LoadOptions{Dir: "/app", Roots: []string{"/app/main.ts"}, FS: fs})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if rev := prog.Revision(); rev == "" {
		t.Fatal("Revision is empty; expected the pinned fork commit")
	}

	// The program sees both files, resolved through bento's own resolver.
	imps := prog.Imports(SourceFile{Path: "/app/main.ts", Kind: FileTS})
	if len(imps) != 1 || imps[0].Resolved.Path != "/app/geo.ts" {
		t.Fatalf("imports = %+v, want one edge to /app/geo.ts", imps)
	}

	// No diagnostics: the program type-checks clean.
	for _, d := range prog.Diagnostics() {
		if d.Category == CategoryError {
			t.Errorf("unexpected error diagnostic: %s", d.Message)
		}
	}
}

// TestLoadReportsTypeErrors proves the checker's semantic errors flow all the way
// out through Load, not just parse errors.
func TestLoadReportsTypeErrors(t *testing.T) {
	fs := mapFS{files: map[string]string{
		"/app/main.ts": "export const n: number = \"nope\";\n",
	}}
	prog, err := Load(LoadOptions{Dir: "/app", Roots: []string{"/app/main.ts"}, FS: fs})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	found := false
	for _, d := range prog.Diagnostics() {
		if d.Category == CategoryError {
			found = true
		}
	}
	if !found {
		t.Fatal("expected an error diagnostic for the type mismatch")
	}
}

// TestLoadRequiresRoots holds the documented contract that Load needs explicit
// roots until tsconfig include discovery lands.
func TestLoadRequiresRoots(t *testing.T) {
	if _, err := Load(LoadOptions{Dir: "/app"}); err != ErrNoRoots {
		t.Fatalf("err = %v, want ErrNoRoots", err)
	}
}
