package frontend

import "testing"

import "github.com/tamnd/bento/pkg/frontend/adapter"

// TestProgramQueriesOverFake builds a small typed program by hand through the
// fake adapter and drives every query the partitioner and lowering rely on. It
// proves the frontend vocabulary round-trips: handles go in, bento value types
// come out, and the structural queries reach the shapes a real checker would
// report. When the real typescript-go adapter lands behind the same interface,
// this same surface answers the same way.
func TestProgramQueriesOverFake(t *testing.T) {
	f := adapter.NewFake()

	num := f.Prim(TypeNumber)
	str := f.Prim(TypeString)
	point := f.Object(f.Prop("x", num), f.Prop("y", num))

	// A use of `p.x` inside the body narrowed to number, and a union-typed
	// identifier, both children of the function so a consumer reaches them by
	// walking the body, the real partitioner/lowering path.
	use := f.Node(adapter.NodePropertyAccessExpression, num)
	un := f.Node(adapter.NodeIdentifier, f.Union(str, num))
	fn := f.Func("dist", f.Sig([]adapter.ParamInfo{f.Param("p", point)}, num), use, un)

	a, handle := f.Program(fn)
	p := Wrap(a, handle)

	roots := p.SourceFiles()
	if len(roots) != 1 {
		t.Fatalf("SourceFiles = %d roots, want 1", len(roots))
	}
	if k := roots[0].Kind(); k != NodeFunctionDeclaration {
		t.Fatalf("root kind = %v, want function declaration", k)
	}

	sym, ok := p.SymbolAt(roots[0])
	if !ok {
		t.Fatal("SymbolAt returned no symbol for the function")
	}
	if sym.Name != "dist" {
		t.Errorf("symbol name = %q, want dist", sym.Name)
	}
	if sym.Flags&SymbolFunction == 0 {
		t.Errorf("symbol flags = %b, want SymbolFunction set", sym.Flags)
	}

	sig, ok := p.SignatureAt(roots[0])
	if !ok {
		t.Fatal("SignatureAt returned no signature")
	}
	if len(sig.Params) != 1 || sig.Params[0].Name != "p" {
		t.Fatalf("signature params = %+v, want one param named p", sig.Params)
	}
	if sig.MinArgs != 1 {
		t.Errorf("MinArgs = %d, want 1", sig.MinArgs)
	}
	if sig.Return.Flags&TypeNumber == 0 {
		t.Errorf("return flags = %b, want TypeNumber set", sig.Return.Flags)
	}
	if sig.Params[0].Type.Flags&TypeObject == 0 {
		t.Errorf("param type flags = %b, want TypeObject set", sig.Params[0].Type.Flags)
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

	// Walk the body the way a consumer does, then query the wrapped nodes.
	body := p.Children(roots[0])
	if len(body) != 2 {
		t.Fatalf("body = %d nodes, want 2", len(body))
	}

	// The narrowed type at the first use is number.
	if got := p.TypeAt(body[0]); got.Flags&TypeNumber == 0 {
		t.Errorf("TypeAt(use) flags = %b, want number", got.Flags)
	}

	// The union identifier round-trips through UnionMembers.
	members := p.UnionMembers(p.TypeAt(body[1]))
	if len(members) != 2 {
		t.Fatalf("union members = %d, want 2", len(members))
	}
	if members[0].Flags&TypeString == 0 || members[1].Flags&TypeNumber == 0 {
		t.Errorf("union members = %b,%b, want string,number", members[0].Flags, members[1].Flags)
	}
}

// TestDeclaredTypeAtReportsNarrowing proves the partitioner can tell a narrowed
// use from its declared type, which is how it decides a union parameter is still
// lowerable when every branch narrows to a concrete type.
func TestDeclaredTypeAtReportsNarrowing(t *testing.T) {
	f := adapter.NewFake()
	num := f.Prim(TypeNumber)
	str := f.Prim(TypeString)

	// A narrowed view: the value is number here, but its declared type is
	// string | number.
	narrowed := f.Prim(TypeNumber)
	narrowed.Declared = f.Union(str, num)
	use := f.Node(adapter.NodeIdentifier, narrowed)

	a, handle := f.Program(f.Func("f", f.Sig(nil, num), use))
	p := Wrap(a, handle)

	body := p.Children(p.SourceFiles()[0])
	declared, narrow, ok := p.DeclaredTypeAt(body[0])
	if !ok {
		t.Fatal("DeclaredTypeAt reported no type")
	}
	if declared.Flags&TypeUnion == 0 {
		t.Errorf("declared flags = %b, want union", declared.Flags)
	}
	if narrow.Flags&TypeNumber == 0 {
		t.Errorf("narrowed flags = %b, want number", narrow.Flags)
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
