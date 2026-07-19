package lower

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

var update = flag.Bool("update", false, "rewrite testdata golden files")

// realFS is an in-memory FileSystem for compiling a single snippet through the
// real checker. Every lowering test drives the renderer from a real compile,
// which is what proves the mapping table holds against the checker's own types
// rather than a hand-built stand-in.
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

// compile loads src as /m.ts through the real checker and fails on any type
// error, so a test that reaches the renderer knows the program was well typed.
func compile(t *testing.T, src string) *frontend.Program {
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
	return prog
}

// compileTolerant loads src like compile but admits the "property does not
// exist" diagnostics (2339 and its "did you mean" variant 2551) the AOT front
// door tolerates in build.firstError. A program that reads or writes a property
// the fixed shape never declared draws 2339, so compiling it strictly would fail
// the test before the renderer ran; tolerating it here reaches the renderer on
// the exact terms build.Compile does, which is where the harness meets these
// programs.
func compileTolerant(t *testing.T, src string) *frontend.Program {
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
		if d.Category != frontend.CategoryError {
			continue
		}
		if d.Code == 2339 || d.Code == 2551 {
			continue
		}
		// 2695 (a comma whose left side has no side effect) is admitted by the AOT
		// front door because the comma still lowers, so mirror it here.
		if d.Code == 2695 {
			continue
		}
		// 2554 and 2555 (a call whose argument count does not match the callee's
		// arity) are admitted because the call still lowers or safely hands back, so
		// mirror the front door here too.
		if d.Code == 2554 || d.Code == 2555 {
			continue
		}
		// 2362 and 2363 (a string or boolean operand of an arithmetic operator) are
		// admitted because the operand coerces through ToNumber and the operator
		// still lowers, so mirror the front door here too.
		if d.Code == 2362 || d.Code == 2363 {
			continue
		}
		// 2703 (a delete over a non-reference operand) is admitted because delete
		// still yields true, folded or handed back, so mirror the front door here.
		if d.Code == 2703 {
			continue
		}
		// 2367 (a comparison the checker judges to have no type overlap) is admitted
		// because the equality still runs at runtime: a static primitive pair lowers
		// through value.LooseEquals and anything else hands back, so mirror the front
		// door here too.
		if d.Code == 2367 {
			continue
		}
		// 2464 (a computed property name typed outside string/number/symbol/any) is
		// admitted because the key still runs ToPropertyKey at runtime: a not-fixed
		// literal boxes and emits SetKeyed over the boxed key, and a key the renderer
		// cannot box hands back, so mirror the front door here too.
		if d.Code == 2464 {
			continue
		}
		// 7031 (a binding element with no inferable type) is admitted because the
		// untyped destructured member lowers to a dynamic value slot the same way an
		// untyped parameter does, so mirror the front door (build.go toleratedCode) here.
		if d.Code == 7031 {
			continue
		}
		// 2345 (an argument not assignable to its parameter) and 2322 (an assignment or
		// initializer not assignable to its slot) are admitted because the argument,
		// constructor, and binding bridges land a representation-safe value and hand back
		// a mismatched one, and the end-of-render reconciliation hands back any site an
		// unguarded path reached, so mirror the front door (build.go toleratedAssignability)
		// here too.
		if d.Code == 2345 || d.Code == 2322 {
			continue
		}
		// 2769 (no overload matches this call) is admitted because a call to a user-defined
		// overloaded function lowers through the implementation's boxed dispatch and the
		// end-of-render reconciliation hands back any 2769 site that path did not reach, so
		// mirror the front door (build.go toleratedOverload) here too.
		if d.Code == 2769 {
			continue
		}
		// 2683 (free `this` with no annotation) is admitted because a free `this`
		// reaches the renderer's this path where thisName is empty and hands back, so
		// mirror the front door (build.go toleratedImplicitThis) here too.
		if d.Code == 2683 {
			continue
		}
		// 7009 (a `new` whose target lacks a construct signature) is admitted because
		// `new f()` over a plain-function value reaches the renderer, which recognizes
		// only class and built-in constructor targets and hands back the rest, so mirror
		// the front door (build.go toleratedConstructAny) here too.
		if d.Code == 7009 {
			continue
		}
		t.Fatalf("unexpected type error in snippet: %s", d.Message)
	}
	return prog
}

// renderProgramTolerant compiles src through the tolerant front door and
// assembles it to Go source, the counterpart to renderProgram for programs that
// only reach the renderer because a tolerated diagnostic was admitted. A hand-back
// is a test failure: the case is inside the lowerable subset by construction.
func renderProgramTolerant(t *testing.T, src string) string {
	t.Helper()
	prog := compileTolerant(t, src)
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

// renderProgramTolerantHandBack compiles src through the tolerant front door and
// asserts the assembler hands the whole program back as NotYetLowerable,
// returning the reason. It is the handback counterpart for programs that only
// reach the renderer because a dynamic-member diagnostic was admitted.
func renderProgramTolerantHandBack(t *testing.T, src string) string {
	t.Helper()
	prog := compileTolerant(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	return nyl.Reason
}

// typeOfDecl compiles src and returns the type of the first variable
// declaration's name, the type the renderer lowers.
func typeOfDecl(t *testing.T, src string) (*frontend.Program, frontend.Type) {
	t.Helper()
	prog := compile(t, src)
	var decls []frontend.Node
	collectKind(prog, prog.SourceFiles(), frontend.NodeVariableDeclaration, &decls)
	if len(decls) == 0 {
		t.Fatal("no variable declaration in snippet")
	}
	name := prog.Children(decls[0])[0]
	return prog, prog.TypeAt(name)
}

// collectKind gathers every node of a kind, depth first, in source order.
func collectKind(prog *frontend.Program, nodes []frontend.Node, kind frontend.NodeKind, out *[]frontend.Node) {
	for _, n := range nodes {
		if n.Kind() == kind {
			*out = append(*out, n)
		}
		collectKind(prog, prog.Children(n), kind, out)
	}
}

// renderReal renders the type of the first variable declaration in src.
func renderReal(t *testing.T, src string) (*Renderer, string, error) {
	t.Helper()
	prog, ty := typeOfDecl(t, src)
	r := NewRenderer(prog)
	got, err := r.RenderType(ty)
	return r, got, err
}

// renderEachDecl renders the type of every top-level variable declaration in
// source order through one renderer, so an interning or naming test can see how
// several shapes in one program share (or do not share) generated declarations.
func renderEachDecl(t *testing.T, src string) (*Renderer, []string) {
	t.Helper()
	prog := compile(t, src)
	var decls []frontend.Node
	collectKind(prog, prog.SourceFiles(), frontend.NodeVariableDeclaration, &decls)
	r := NewRenderer(prog)
	got := make([]string, 0, len(decls))
	for _, d := range decls {
		name := prog.Children(d)[0]
		s, err := r.RenderType(prog.TypeAt(name))
		if err != nil {
			t.Fatalf("RenderType: %v", err)
		}
		got = append(got, s)
	}
	return r, got
}

// TestRealPrimitivesRender pins the section 3 to 8 primitive mappings against the
// real checker: each annotated primitive lowers to its Go counterpart.
func TestRealPrimitivesRender(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"number", "const x: number = 1;", "float64"},
		{"bigint", "const x: bigint = 1n;", "*big.Int"},
		{"string", "const x: string = \"s\";", "value.BStr"},
		{"boolean", "const x: boolean = true;", "bool"},
		{"symbol", "const x: symbol = Symbol();", "value.Value"},
		// any and unknown have no static shape, so they lower to the boxed value.Value
		// the dynamic world uses; the operations on such a value dispatch on its
		// runtime kind through the value package.
		{"any", "const x: any = 1;", "value.Value"},
		{"unknown", "const x: unknown = 1;", "value.Value"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
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
// real string[][] nests it (section 11), against the checker's own array types.
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
	if want := "*value.Array[*value.Array[value.BStr]]"; got != want {
		t.Errorf("RenderType(string[][]) = %q, want %q", got, want)
	}
}

// TestRealObjectRendersToStructPointer proves a real inferred object type lowers
// to a pointer to a generated struct with exported fields in a stable order
// (section 12), pinned by a golden.
func TestRealObjectRendersToStructPointer(t *testing.T) {
	r, got, err := renderReal(t, "const x = { x: 1, y: 2 };")
	if err != nil {
		t.Fatalf("RenderType(point): %v", err)
	}
	if want := "*ObjXY"; got != want {
		t.Errorf("RenderType(point) = %q, want %q", got, want)
	}
	decls := r.Decls()
	if len(decls) != 1 {
		t.Fatalf("got %d decls, want 1", len(decls))
	}
	checkGolden(t, "point_struct.golden", decls[0].Source)
}

// TestRealObjectFieldTypesLower proves a nested object field lowers to a pointer
// to its own generated struct and a primitive field to its Go type, pinned by a
// golden over both declarations.
func TestRealObjectFieldTypesLower(t *testing.T) {
	r, got, err := renderReal(t, `const x = { origin: { x: 1, y: 2 }, label: "s" };`)
	if err != nil {
		t.Fatalf("RenderType(shape): %v", err)
	}
	if want := "*ObjLabelOrigin"; got != want {
		t.Errorf("RenderType(shape) = %q, want %q", got, want)
	}
	decls := r.Decls()
	if len(decls) != 2 {
		t.Fatalf("got %d decls, want 2 (the outer shape and the nested point)", len(decls))
	}
	var all strings.Builder
	for _, d := range decls {
		all.WriteString(d.Source)
	}
	checkGolden(t, "nested_struct.golden", all.String())
}

// TestRealSameShapeInternsToOneStruct pins the interning rule of section 12: one
// object type used in two fields, so both fields share a single type identity,
// lowers to one Go struct, not two.
func TestRealSameShapeInternsToOneStruct(t *testing.T) {
	r, _ := renderEachDecl(t, "type Point = { x: number; y: number };\ndeclare const pair: { a: Point; b: Point };")
	names := map[string]int{}
	for _, d := range r.Decls() {
		names[d.Name]++
	}
	if names["ObjXY"] != 1 {
		t.Errorf("ObjXY emitted %d times, want exactly 1 (interned)", names["ObjXY"])
	}
}

// TestRealDistinctShapesShareBaseNameGetSuffix pins the collision rule of section
// 29: two different shapes that derive the same base name get distinct Go names.
func TestRealDistinctShapesShareBaseNameGetSuffix(t *testing.T) {
	r, _ := renderEachDecl(t, "declare const outer: { a: { x: number }; b: { x: string } };")
	seen := map[string]bool{}
	for _, d := range r.Decls() {
		if seen[d.Name] {
			t.Fatalf("duplicate generated name %q", d.Name)
		}
		seen[d.Name] = true
	}
	if !seen["ObjX"] || !seen["ObjX_2"] {
		t.Errorf("want both ObjX and ObjX_2, got names %v", seen)
	}
}

// TestRealStringLiteralUnionRendersBStr pins the contract for a closed
// string-literal union: it is a plain string at run time, one of a fixed set of
// strings, so it renders to value.BStr and every operation reads it through the
// ordinary string machinery rather than a separate tag representation.
func TestRealStringLiteralUnionRendersBStr(t *testing.T) {
	_, got, err := renderReal(t, `const x: "circle" | "rect" = "circle";`)
	if err != nil {
		t.Fatalf("RenderType(string-literal union) err = %v, want value.BStr", err)
	}
	if got != "value.BStr" {
		t.Fatalf("RenderType(string-literal union) = %q, want value.BStr", got)
	}
}

// TestRealSameStringUnionRendersBStr pins that a named string-literal union renders
// the same value.BStr, not just an inline one at a single use site.
func TestRealSameStringUnionRendersBStr(t *testing.T) {
	_, got, err := renderReal(t, "type Dir = \"north\" | \"south\";\nconst d: Dir = \"north\";")
	if err != nil {
		t.Fatalf("RenderType(named string-literal union) err = %v, want value.BStr", err)
	}
	if got != "value.BStr" {
		t.Fatalf("RenderType(named string-literal union) = %q, want value.BStr", got)
	}
}

// TestRealNonIdentifierStringUnionRendersBStr pins that a string-literal union
// whose member is not a Go identifier still renders value.BStr; the member is a
// run-time string value, so a spaced member needs no name mangle at all.
func TestRealNonIdentifierStringUnionRendersBStr(t *testing.T) {
	_, got, err := renderReal(t, `const x: "north" | "due east" = "north";`)
	if err != nil {
		t.Fatalf("RenderType(union with a spaced member) err = %v, want value.BStr", err)
	}
	if got != "value.BStr" {
		t.Fatalf("RenderType(union with a spaced member) = %q, want value.BStr", got)
	}
}

// TestRealUnlowerableHandsBack pins the section 30 hand-back contract against the
// checker: constructs whose lowering slice has not landed return NotYetLowerable
// rather than a wrong Go type.
func TestRealUnlowerableHandsBack(t *testing.T) {
	cases := []struct{ name, src string }{
		{"union", "const x: number | number[] = 1;"},
		{"typeParameter", "function f<T>(a: T) { const x = a; return x; }"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, _, err := renderReal(t, tc.src)
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderType(%s) err = %v, want *NotYetLowerable", tc.name, err)
			}
		})
	}
}

// TestRealOptionalPropertyLowers pins that an optional property x?: T, which
// types as the T | undefined optional, lowers to a struct with a value.Opt[T]
// field rather than handing the whole object back.
func TestRealOptionalPropertyLowers(t *testing.T) {
	r, got, err := renderReal(t, "declare const x: { host: string; port?: number };")
	if err != nil {
		t.Fatalf("RenderType(object with optional) err = %v, want nil", err)
	}
	if got == "" {
		t.Fatal("RenderType(object with optional) returned an empty type")
	}
	var src string
	for _, d := range r.decls.emit() {
		src += d.Source + "\n"
	}
	if !strings.Contains(src, "value.Opt[float64]") {
		t.Fatalf("struct decl = %q, want a value.Opt[float64] field for the optional property", src)
	}
}

// TestRealWideOptionalPropertyLowers pins that an optional whose type is not the
// two-member T | undefined shape (port?: number | string adds a third member)
// lowers to a tagged-sum field holding a tag-only undefined arm for the absent
// property, no handback.
func TestRealWideOptionalPropertyLowers(t *testing.T) {
	r, got, err := renderReal(t, "declare const x: { host: string; port?: number | string };")
	if err != nil {
		t.Fatalf("RenderType(object with wide optional) err = %v, want nil via the tagged sum", err)
	}
	if got == "" {
		t.Fatal("RenderType(object with wide optional) returned an empty type")
	}
	var src string
	for _, d := range r.decls.emit() {
		src += d.Source + "\n"
	}
	if !strings.Contains(src, "NumOrStrOrUndef") {
		t.Fatalf("struct decls = %q, want a tagged sum with an undefined arm for the wide optional", src)
	}
}

// TestRealNonIdentifierPropertyLowers pins that a property name Go cannot spell
// verbatim becomes a mangled struct field. Declaration, dotted read, and
// bracket read all route through the same exportedField, so "has space" is
// one field everywhere and the object lowers with a json tag keeping the
// original spelling.
func TestRealNonIdentifierPropertyLowers(t *testing.T) {
	r, got, err := renderReal(t, `declare const x: { "has space": number };`)
	if err != nil {
		t.Fatalf("RenderType(object with non-identifier key) err = %v, want nil via the mangle", err)
	}
	if got == "" {
		t.Fatal("RenderType(object with non-identifier key) returned an empty type")
	}
	var src string
	for _, d := range r.decls.emit() {
		src += d.Source + "\n"
	}
	if !strings.Contains(src, "`json:\"has space\"`") {
		t.Fatalf("struct decl = %q, want a json tag preserving the original key", src)
	}
}

// checkGolden compares got against the named golden file, rewriting it under
// -update.
func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with -update to create)", name, err)
	}
	if got != string(want) {
		t.Errorf("golden %s mismatch:\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}
