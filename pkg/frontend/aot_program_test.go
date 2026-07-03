package frontend

import (
	"testing"

	"github.com/tamnd/bento/pkg/frontend/adapter"
)

// loadOne compiles src as /m.ts through the real checker and fails on any type
// error, so a test that reaches its queries knows the program was well typed.
func loadOne(t *testing.T, src string) *Program {
	t.Helper()
	prog, err := Load(LoadOptions{
		Dir:   "/",
		Roots: []string{"/m.ts"},
		FS:    mapFS{files: map[string]string{"/m.ts": src}},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, d := range prog.Diagnostics() {
		if d.Category == CategoryError {
			t.Fatalf("unexpected type error: %s", d.Message)
		}
	}
	return prog
}

// collectKind gathers every node of a kind, depth first, in source order.
func collectKind(p *Program, nodes []Node, kind NodeKind, out *[]Node) {
	for _, n := range nodes {
		if n.Kind() == kind {
			*out = append(*out, n)
		}
		collectKind(p, p.Children(n), kind, out)
	}
}

// firstOfKind returns the first node of a kind anywhere under nodes.
func firstOfKind(t *testing.T, p *Program, nodes []Node, kind NodeKind) Node {
	t.Helper()
	var out []Node
	collectKind(p, nodes, kind, &out)
	if len(out) == 0 {
		t.Fatalf("no node of kind %v found", kind)
	}
	return out[0]
}

// TestProgramQueriesOverRealCompile drives every query the partitioner and
// lowering rely on over a real checked program, proving the frontend vocabulary
// round-trips against the checker's own answers: handles go in, bento value
// types come out, and the structural queries reach the real shapes.
func TestProgramQueriesOverRealCompile(t *testing.T) {
	p := loadOne(t, "export function dist(p: { x: number; y: number }, tag: string | number): number { const q = p.x; return q; }\n")

	roots := p.SourceFiles()
	if len(roots) != 1 {
		t.Fatalf("SourceFiles = %d roots, want 1", len(roots))
	}
	fn := firstOfKind(t, p, roots, NodeFunctionDeclaration)
	if fn.Kind() != NodeFunctionDeclaration {
		t.Fatalf("no function declaration found under the source file")
	}

	sym, ok := p.SymbolAt(fn)
	if !ok {
		t.Fatal("SymbolAt returned no symbol for the function")
	}
	if sym.Name != "dist" {
		t.Errorf("symbol name = %q, want dist", sym.Name)
	}
	if sym.Flags&SymbolFunction == 0 {
		t.Errorf("symbol flags = %b, want SymbolFunction set", sym.Flags)
	}

	sig, ok := p.SignatureAt(fn)
	if !ok {
		t.Fatal("SignatureAt returned no signature")
	}
	if len(sig.Params) != 2 || sig.Params[0].Name != "p" || sig.Params[1].Name != "tag" {
		t.Fatalf("signature params = %+v, want p and tag", sig.Params)
	}
	if sig.MinArgs != 2 {
		t.Errorf("MinArgs = %d, want 2", sig.MinArgs)
	}
	if sig.Return.Flags&TypeNumber == 0 {
		t.Errorf("return flags = %b, want TypeNumber set", sig.Return.Flags)
	}
	if sig.Params[0].Type.Flags&TypeObject == 0 {
		t.Errorf("param p type flags = %b, want TypeObject set", sig.Params[0].Type.Flags)
	}

	props := p.Properties(sig.Params[0].Type)
	if len(props) != 2 || props[0].Name != "x" || props[1].Name != "y" {
		t.Fatalf("properties = %+v, want x and y", props)
	}
	for _, pr := range props {
		if pr.Type.Flags&TypeNumber == 0 {
			t.Errorf("property %q type = %b, want number", pr.Name, pr.Type.Flags)
		}
	}

	// The union parameter round-trips through UnionMembers.
	members := p.UnionMembers(sig.Params[1].Type)
	if len(members) != 2 {
		t.Fatalf("union members = %d, want 2", len(members))
	}
	strFirst := members[0].Flags&TypeString != 0 && members[1].Flags&TypeNumber != 0
	numFirst := members[0].Flags&TypeNumber != 0 && members[1].Flags&TypeString != 0
	if !strFirst && !numFirst {
		t.Errorf("union members = %b,%b, want string and number in some order", members[0].Flags, members[1].Flags)
	}

	// The property access p.x inside the body carries the number type.
	pa := firstOfKind(t, p, p.Children(fn), NodePropertyAccessExpression)
	if pa.Kind() != NodePropertyAccessExpression {
		t.Fatal("no property access expression found in the body")
	}
	if got := p.TypeAt(pa); got.Flags&TypeNumber == 0 {
		t.Errorf("TypeAt(p.x) flags = %b, want number", got.Flags)
	}
}

// TestDeclaredTypeAtReportsNarrowing proves the partitioner can tell a narrowed
// use from its declared type, which is how it decides a union parameter is still
// lowerable when a guard narrows it to a concrete type. The checker narrows x to
// number inside the typeof guard while its declared type stays string | number.
func TestDeclaredTypeAtReportsNarrowing(t *testing.T) {
	p := loadOne(t, "export function f(x: string | number): number { if (typeof x === \"number\") { return x; } return 0; }\n")
	fn := firstOfKind(t, p, p.SourceFiles(), NodeFunctionDeclaration)

	var idents []Node
	collectKind(p, []Node{fn}, NodeIdentifier, &idents)

	found := false
	for _, id := range idents {
		sym, ok := p.SymbolAt(id)
		if !ok || sym.Name != "x" {
			continue
		}
		declared, narrow, ok := p.DeclaredTypeAt(id)
		if !ok {
			continue
		}
		// The narrowed use inside the guard: declared union, narrowed to number.
		if declared.Flags&TypeUnion != 0 && narrow.Flags&TypeNumber != 0 && narrow.Flags&TypeUnion == 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("no narrowed use of x reported number against a declared union")
	}
}

// TestClassExposure proves the two frontend facts class lowering stands on: the
// this keyword inside a method surfaces as its own node kind rather than
// NodeUnknown, and TypeSymbol walks from a class instance type back to the class
// declaration that names it. A primitive type has no symbol and reports false.
func TestClassExposure(t *testing.T) {
	p := loadOne(t, `class Point {
  x: number;
  constructor(x: number) {
    this.x = x;
  }
  double(): number {
    return this.x * 2;
  }
}
export const pt = new Point(3);
export const n = pt.double();
`)
	roots := p.SourceFiles()

	cls := firstOfKind(t, p, roots, NodeClassDeclaration)
	this := firstOfKind(t, p, []Node{cls}, NodeThisKeyword)
	if got := p.Text(this); got != "this" {
		t.Errorf("this keyword text = %q, want this", got)
	}

	// The new expression types as the Point instance, and TypeSymbol walks from
	// that type to the class symbol and its declaration.
	ne := firstOfKind(t, p, roots, NodeNewExpression)
	inst := p.TypeAt(ne)
	if inst.Flags&TypeObject == 0 {
		t.Fatalf("instance type flags = %b, want TypeObject set", inst.Flags)
	}
	sym, ok := p.TypeSymbol(inst)
	if !ok {
		t.Fatal("TypeSymbol returned no symbol for a class instance type")
	}
	if sym.Name != "Point" {
		t.Errorf("class symbol name = %q, want Point", sym.Name)
	}
	if sym.Flags&SymbolClass == 0 {
		t.Errorf("class symbol flags = %b, want SymbolClass set", sym.Flags)
	}
	foundDecl := false
	for _, d := range p.Declarations(sym) {
		if d.Kind() == NodeClassDeclaration {
			foundDecl = true
		}
	}
	if !foundDecl {
		t.Error("class symbol declarations contain no class declaration")
	}

	// A primitive type is intrinsic and carries no declaring symbol.
	lit := firstOfKind(t, p, roots, NodeNumericLiteral)
	if _, ok := p.TypeSymbol(p.TypeAt(lit)); ok {
		t.Error("TypeSymbol reported a symbol for a number type, want none")
	}
}

// TestLoadRealAdapterIsAvailable pins the post-fork reality: a real, checker-
// backed adapter is always constructible, so Load never returns
// ErrRealAdapterUnavailable. The remaining precondition is an explicit root set,
// which TestLoadRequiresRoots covers.
func TestLoadRealAdapterIsAvailable(t *testing.T) {
	if !adapter.RealAdapterAvailable() {
		t.Fatal("RealAdapterAvailable is false, but the fork pins a revision")
	}
	if _, err := Load(LoadOptions{Dir: "."}); err == ErrRealAdapterUnavailable {
		t.Error("Load reported the real adapter unavailable, but the fork is wired in")
	}
}
