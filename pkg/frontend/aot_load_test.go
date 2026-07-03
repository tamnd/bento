package frontend

import (
	"strings"
	"testing"
)

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

// TestLoadResolvesBentoGoVocabulary proves the ambient overlay serves the
// bento:go module: a program that imports a helper from it type-checks clean and
// the checker sees the helper's real shape, so a correct use of GoReader.Read
// carries no diagnostic. This is what makes a generated .d.ts's
// `import { GoReader } from "bento:go"` resolve instead of erroring on a missing
// module.
func TestLoadResolvesBentoGoVocabulary(t *testing.T) {
	fs := mapFS{files: map[string]string{
		"/app/main.ts": "import { GoReader } from \"bento:go\";\n" +
			"export function first(r: GoReader): number { return r.Read(new Uint8Array(8)); }\n",
	}}
	prog, err := Load(LoadOptions{Dir: "/app", Roots: []string{"/app/main.ts"}, FS: fs})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, d := range prog.Diagnostics() {
		if d.Category == CategoryError {
			t.Errorf("unexpected error diagnostic against bento:go: %s", d.Message)
		}
	}
}

// TestLoadBentoGoVocabularyIsTyped proves the served module is a real typed shape,
// not an untyped escape hatch: reaching for a method GoReader does not have is a
// checker error, so the vocabulary constrains a program the way its declarations
// promise.
func TestLoadBentoGoVocabularyIsTyped(t *testing.T) {
	fs := mapFS{files: map[string]string{
		"/app/main.ts": "import { GoReader } from \"bento:go\";\n" +
			"export function bad(r: GoReader): number { return r.Write(new Uint8Array(8)); }\n",
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
		t.Fatal("expected an error for calling GoReader.Write, which the vocabulary does not declare")
	}
}

// TestLoadTypeChecksGoImport proves the whole go: interop type path: the resolver
// classifies go:crypto/sha256, the frontend generates that package's declarations
// from the real Go package and serves them at the virtual path the import resolves
// to, and the checker binds Sum256 against its genuine signature. A call that
// matches the signature carries no diagnostic. This test loads a real standard
// library package, so it needs the Go toolchain but no network.
func TestLoadTypeChecksGoImport(t *testing.T) {
	fs := mapFS{files: map[string]string{
		"/app/main.ts": "import { Sum256 } from \"go:crypto/sha256\";\n" +
			"export const digest = Sum256(new Uint8Array([1, 2, 3]));\n",
	}}
	prog, err := Load(LoadOptions{Dir: "/app", Roots: []string{"/app/main.ts"}, FS: fs})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// The go: import resolves to its virtual declaration path and is pulled into the
	// program as a typed input.
	imps := prog.Imports(SourceFile{Path: "/app/main.ts", Kind: FileTS})
	if len(imps) != 1 {
		t.Fatalf("imports = %+v, want one go: edge", imps)
	}
	if imps[0].Kind != ImportGo {
		t.Errorf("go: import kind = %v, want ImportGo", imps[0].Kind)
	}
	if imps[0].Resolved.Path != "/__bento_go__/crypto/sha256.d.ts" {
		t.Errorf("go: import resolved to %q, want the virtual declaration path", imps[0].Resolved.Path)
	}

	for _, d := range prog.Diagnostics() {
		if d.Category == CategoryError {
			t.Errorf("unexpected error diagnostic against go:crypto/sha256: %s", d.Message)
		}
	}
}

// TestLoadGoImportIsTyped proves the served Go declarations are the real API and
// not an untyped escape hatch: passing a number where Sum256 wants a Uint8Array is
// a checker error, so a go: import constrains a program the way the package's true
// signature does.
func TestLoadGoImportIsTyped(t *testing.T) {
	fs := mapFS{files: map[string]string{
		"/app/main.ts": "import { Sum256 } from \"go:crypto/sha256\";\n" +
			"export const digest = Sum256(42);\n",
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
		t.Fatal("expected an error for passing a number to Sum256, which wants a Uint8Array")
	}
}

// TestLoadUnsupportedGoSignatureIsCallSiteError proves the section 6.14 promise:
// a Go function whose signature requires a type the bridge cannot cross is not
// dropped from the declarations, it is projected with a GoUnsupported marker, so an
// author who reaches for it gets a clear checker error at the call site rather than
// a mysterious missing export or a runtime surprise. math/cmplx.Abs takes a
// complex128, which has no faithful TypeScript meaning and projects as
// GoUnsupported, so passing a number to it must be a checker error. Like the other
// go: interop tests this loads a real standard library package, so it needs the Go
// toolchain but no network.
func TestLoadUnsupportedGoSignatureIsCallSiteError(t *testing.T) {
	fs := mapFS{files: map[string]string{
		"/app/main.ts": "import { Abs } from \"go:math/cmplx\";\n" +
			"export const m = Abs(1);\n",
	}}
	prog, err := Load(LoadOptions{Dir: "/app", Roots: []string{"/app/main.ts"}, FS: fs})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// The symbol is projected, not omitted: Abs binds, so the diagnostic is a type
	// error at the call, never a "no exported member Abs" that would mean the
	// declaration silently swallowed the function.
	found := false
	for _, d := range prog.Diagnostics() {
		if d.Category != CategoryError {
			continue
		}
		found = true
		if strings.Contains(d.Message, "Abs") && strings.Contains(strings.ToLower(d.Message), "no exported member") {
			t.Errorf("Abs was omitted from the declarations instead of projected as GoUnsupported: %s", d.Message)
		}
	}
	if !found {
		t.Fatal("expected a checker error for calling a go: function whose parameter projects as GoUnsupported")
	}
}

// TestLoadRequiresRoots holds the documented contract that Load needs explicit
// roots until tsconfig include discovery lands.
func TestLoadRequiresRoots(t *testing.T) {
	if _, err := Load(LoadOptions{Dir: "/app"}); err != ErrNoRoots {
		t.Fatalf("err = %v, want ErrNoRoots", err)
	}
}
