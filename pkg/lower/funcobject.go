package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// registerCallableFuncDecl claims a top-level named function declaration that is
// also a callable object: a function whose name later carries its own data
// properties, the `function foo() {...} foo.x = 1` shape Node's util and events
// modules lean on. The callable-object model lowers it to a
// `type Foo struct { Call func(...); X ... }` rather than a bare `func Foo`, so the
// binding is a package-level pointer var the property writes and the call path both
// reach. This reserves that var and marks the name for module-assignment
// construction; buildCallableFuncDeclCtors emits the `foo = &Foo{}; foo.Call = ...`
// pair at the top of main. The deeper cases (async, generator) hand back here; a
// body construct the closure cannot yet lower hands back when the body lowers.
func (r *Renderer) registerCallableFuncDecl(fn frontend.Node) (ast.Decl, error) {
	if r.isAsyncFunc(fn) {
		return nil, &NotYetLowerable{Reason: "an async function that is also a callable object is a later slice"}
	}
	if r.isGeneratorFunc(fn) {
		return nil, &NotYetLowerable{Reason: "a generator function that is also a callable object is a later slice"}
	}
	name, structName, err := r.callableFuncDeclNames(fn)
	if err != nil {
		return nil, err
	}
	// The name is a package-level pointer var, so main and any top-level function
	// name the same object; marking it a module-assign var lands its construction as
	// a plain assignment into that var and keeps it off every storage-narrowing tier,
	// the way a hoisted module binding is kept.
	r.moduleAssignVars[name] = true
	decl := &ast.GenDecl{
		Tok: token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{
			Names: []*ast.Ident{ident(name)},
			Type:  &ast.StarExpr{X: ident(structName)},
		}},
	}
	return decl, nil
}

// buildCallableFuncDeclCtors builds the construction statements for the callable-
// object function declarations registered above, in declaration order. Each emits
// the same two-step pair the var-bound callable form does through
// flattenCallableBinding, but built from the declaration's own parameters and body:
// `foo = &Foo{}` reserves the object and `foo.Call = func(...) {...}` fills the
// reserved call field with the closure the body lowers to. The pair runs at the top
// of main because a function declaration hoists, so the object exists before the
// body's first `foo.x = 1` runs.
func (r *Renderer) buildCallableFuncDeclCtors(fns []frontend.Node) ([]ast.Stmt, error) {
	var stmts []ast.Stmt
	for _, fn := range fns {
		s, err := r.callableFuncDeclCtor(fn)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, s...)
	}
	return stmts, nil
}

func (r *Renderer) callableFuncDeclCtor(fn frontend.Node) ([]ast.Stmt, error) {
	name, structName, err := r.callableFuncDeclNames(fn)
	if err != nil {
		return nil, err
	}
	sig, ok := r.prog.SignatureAt(fn)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a callable-object function with no call signature is a later slice"}
	}
	fields, err := r.closureParamFields(fn, sig, "function")
	if err != nil {
		return nil, err
	}
	lit, err := r.blockBodyArrow(fn, fields)
	if err != nil {
		return nil, err
	}
	declStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(name)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(structName)}}},
	}
	callStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.SelectorExpr{X: ident(name), Sel: ident("Call")}},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{lit},
	}
	return []ast.Stmt{declStmt, callStmt}, nil
}

// callableFuncDeclNames resolves the two names a callable-object function
// declaration needs: the lowercase local the pointer var takes and the interned
// struct type behind it. Both the registration and the construction read the same
// pair, so they share this so the var and its assignment never drift apart.
func (r *Renderer) callableFuncDeclNames(fn frontend.Node) (string, string, error) {
	sym, ok := r.prog.SymbolAt(fn)
	if !ok {
		return "", "", &NotYetLowerable{Reason: "a callable-object function declaration with no symbol is a later slice"}
	}
	name, ok := localName(sym.Name)
	if !ok {
		return "", "", &NotYetLowerable{Reason: "a callable-object function name is not a Go identifier"}
	}
	structName, err := r.decls.internStruct(r, r.prog.TypeAt(fn))
	if err != nil {
		return "", "", err
	}
	return name, structName, nil
}
