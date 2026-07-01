package lower

import (
	"errors"
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// realFS is an in-memory FileSystem for compiling a single snippet through the
// real checker. It is the lower package's own copy because frontend's test
// helper is unexported.
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

// typeOfDecl compiles src as /m.ts and returns the type of the first variable
// declaration's name, the type the renderer lowers. Driving the renderer from a
// real compile is what proves the mapping table holds against the checker's own
// types, not a hand-built stand-in.
func typeOfDecl(t *testing.T, src string) (*frontend.Program, frontend.Type) {
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
			t.Fatalf("unexpected type error in snippet: %s", d.Message)
		}
	}
	decl := findKind(prog, prog.SourceFiles(), frontend.NodeVariableDeclaration)
	if decl == nil {
		t.Fatal("no variable declaration in snippet")
	}
	name := prog.Children(decl)[0]
	return prog, prog.TypeAt(name)
}

func findKind(prog *frontend.Program, nodes []frontend.Node, kind frontend.NodeKind) frontend.Node {
	for _, n := range nodes {
		if n.Kind() == kind {
			return n
		}
		if got := findKind(prog, prog.Children(n), kind); got != nil {
			return got
		}
	}
	return nil
}

func renderReal(t *testing.T, src string) (*Renderer, string, error) {
	t.Helper()
	prog, ty := typeOfDecl(t, src)
	r := NewRenderer(prog)
	got, err := r.RenderType(ty)
	return r, got, err
}

// TestRealPrimitivesRender pins the primitive mappings against the real checker:
// each annotated primitive lowers to its Go counterpart.
func TestRealPrimitivesRender(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"number", "const x: number = 1;", "float64"},
		{"bigint", "const x: bigint = 1n;", "*big.Int"},
		{"string", "const x: string = \"s\";", "bstr"},
		{"boolean", "const x: boolean = true;", "bool"},
		{"symbol", "const x: symbol = Symbol();", "*value.Symbol"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, got, err := renderReal(t, tc.src)
			if err != nil {
				t.Fatalf("RenderType error: %v", err)
			}
			if got != tc.want {
				t.Errorf("RenderType = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRealArrayRenders pins that a real number[] lowers to the Array header and a
// real string[][] nests it, the same rule the fake test asserts, now against the
// checker's own array types.
func TestRealArrayRenders(t *testing.T) {
	_, got, err := renderReal(t, "const x: number[] = [];")
	if err != nil {
		t.Fatalf("RenderType(number[]): %v", err)
	}
	if want := "*value.Array[float64]"; got != want {
		t.Errorf("RenderType(number[]) = %q, want %q", got, want)
	}

	_, got, err = renderReal(t, "const x: string[][] = [];")
	if err != nil {
		t.Fatalf("RenderType(string[][]): %v", err)
	}
	if want := "*value.Array[*value.Array[bstr]]"; got != want {
		t.Errorf("RenderType(string[][]) = %q, want %q", got, want)
	}
}

// TestRealObjectRendersToStructPointer proves a real inferred object type lowers
// to a pointer to a generated struct with one declaration, the fixed-shape rule
// of section 12 holding against the checker's structural type.
func TestRealObjectRendersToStructPointer(t *testing.T) {
	r, got, err := renderReal(t, "const x = { x: 1, y: 2 };")
	if err != nil {
		t.Fatalf("RenderType(point): %v", err)
	}
	if got == "" || got[0] != '*' {
		t.Fatalf("RenderType(point) = %q, want a pointer to a struct", got)
	}
	decls := r.Decls()
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}
}

// TestRealUnlowerableHandsBack pins the section 30 hand-back contract against the
// checker: constructs whose lowering slice has not landed return NotYetLowerable
// rather than a wrong Go type.
func TestRealUnlowerableHandsBack(t *testing.T) {
	cases := []struct{ name, src string }{
		{"any", "const x: any = 1;"},
		{"union", "const x: number | string = 1;"},
		{"typeParameter", "function f<T>(a: T) { const x = a; return x; }"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := renderReal(t, tc.src)
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderType(%s) err = %v, want *NotYetLowerable", tc.name, err)
			}
		})
	}
}
