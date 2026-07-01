package adapter

import (
	"path"
	"strings"
	"testing"
)

// memHost is an in-memory Host over a fixed file map. It resolves a relative
// specifier against the containing file's directory and appends .ts when the
// written path has no extension, which is all the real-adapter tests here need.
type memHost struct{ files map[string]string }

func (h memHost) ReadFile(p string) (string, bool) { s, ok := h.files[p]; return s, ok }
func (h memHost) FileExists(p string) bool         { _, ok := h.files[p]; return ok }
func (h memHost) GetCurrentDirectory() string      { return "/" }

func (h memHost) DirectoryExists(p string) bool {
	prefix := strings.TrimSuffix(p, "/") + "/"
	for name := range h.files {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func (h memHost) ResolveModule(specifier, containingFile string) (string, ImportKind, bool) {
	if !strings.HasPrefix(specifier, ".") {
		return "", ImportBare, false
	}
	base := path.Dir(containingFile)
	target := path.Clean(path.Join(base, specifier))
	for _, cand := range []string{target, target + ".ts", target + ".tsx", target + "/index.ts"} {
		if _, ok := h.files[cand]; ok {
			return cand, ImportRelative, true
		}
	}
	return "", ImportRelative, false
}

// findFirst walks the program's AST and returns the first node of the given
// kind, so a test can pin a query to a known construct without hard-coding byte
// offsets.
func findFirst(a *RealAdapter, p ProgramHandle, kind NodeKind) NodeHandle {
	var walk func(n NodeHandle) NodeHandle
	walk = func(n NodeHandle) NodeHandle {
		if a.KindOf(n) == kind {
			return n
		}
		for _, c := range a.ChildrenOf(n) {
			if got := walk(c); got != nil {
				return got
			}
		}
		return nil
	}
	for _, f := range a.SourceFiles(p) {
		if got := walk(f); got != nil {
			return got
		}
	}
	return nil
}

func buildReal(t *testing.T, files map[string]string, roots ...string) (*RealAdapter, ProgramHandle) {
	t.Helper()
	a := NewReal()
	p, err := a.BuildProgram(roots, CompilerOptions{Strict: true}, memHost{files: files})
	if err != nil {
		t.Fatalf("BuildProgram: %v", err)
	}
	return a, p
}

// TestRealInfersPrimitiveType proves the real checker answers the type at a
// position: a variable initialized to a number widens to number.
func TestRealInfersPrimitiveType(t *testing.T) {
	a, p := buildReal(t, map[string]string{
		"/main.ts": "const x = 42;\n",
	}, "/main.ts")
	defer prog(p).prog.Close()

	decl := findFirst(a, p, NodeVariableDeclaration)
	if decl == nil {
		t.Fatal("no variable declaration found")
	}
	// The declaration's name identifier is the first child; its type is number.
	name := a.ChildrenOf(decl)[0]
	ty := a.TypeOfNode(p, name)
	if ty == nil {
		t.Fatal("no type at declaration name")
	}
	widened := a.WidenType(p, ty)
	if f := a.TypeFlagsOf(p, widened); f&TypeNumber == 0 {
		t.Fatalf("widened type flags = %b, want TypeNumber set", f)
	}
}

// TestRealEnumeratesProperties proves the checker's structural view: an object
// literal exposes its members with their inferred types.
func TestRealEnumeratesProperties(t *testing.T) {
	a, p := buildReal(t, map[string]string{
		"/main.ts": "const o = { name: \"bento\", size: 3 };\n",
	}, "/main.ts")
	defer prog(p).prog.Close()

	decl := findFirst(a, p, NodeVariableDeclaration)
	name := a.ChildrenOf(decl)[0]
	ty := a.TypeOfNode(p, name)
	props := a.PropertiesOf(p, ty)
	got := map[string]bool{}
	for _, pr := range props {
		got[pr.Name] = true
	}
	if !got["name"] || !got["size"] {
		t.Fatalf("properties = %v, want name and size", got)
	}
}

// TestRealReportsDiagnostics proves the checker's errors reach bento: a type
// mismatch surfaces as a diagnostic with a code and message.
func TestRealReportsDiagnostics(t *testing.T) {
	_, p := buildReal(t, map[string]string{
		"/main.ts": "const n: number = \"not a number\";\n",
	}, "/main.ts")
	defer prog(p).prog.Close()

	diags := NewReal().Diagnostics(p)
	if len(diags) == 0 {
		t.Fatal("expected a diagnostic for the type mismatch")
	}
	found := false
	for _, d := range diags {
		if d.Category == CategoryError && d.Message != "" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no error diagnostic in %v", diags)
	}
}

// TestRealResolvesImports proves the build discovers a relative import, resolves
// it through the host, and records the edge.
func TestRealResolvesImports(t *testing.T) {
	a, p := buildReal(t, map[string]string{
		"/main.ts": "import { hi } from \"./lib\";\nexport const g = hi();\n",
		"/lib.ts":  "export function hi(): string { return \"hi\"; }\n",
	}, "/main.ts")
	defer prog(p).prog.Close()

	imps := a.ImportsOf(p, "/main.ts")
	if len(imps) != 1 {
		t.Fatalf("imports = %v, want one edge", imps)
	}
	if imps[0].ResolvedFile != "/lib.ts" {
		t.Fatalf("resolved to %q, want /lib.ts", imps[0].ResolvedFile)
	}
}
