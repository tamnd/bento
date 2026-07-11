package lower

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// A default on a destructuring pattern element fills the target when the source
// slot is undefined and only then, and JavaScript evaluates the default at most
// once and lazily, so a default that calls a function or reads another binding
// runs solely on the undefined path. This file lowers that fill for the array and
// object declaration forms; the assignment and parameter forms reuse the same
// shape from their own paths.

// arrayDefaultElem describes one element of an array binding pattern once its
// shape is classified: a plain name binds the slot directly, a defaulted name
// fills from its default when the slot is undefined. nameNode carries the
// binding's type and defNode the default expression, present only when hasDefault.
type arrayDefaultElem struct {
	name       string
	nameNode   frontend.Node
	hasDefault bool
	defNode    frontend.Node
}

// classifyArrayElem reads one array binding pattern element into an
// arrayDefaultElem. A single identifier child is a plain name; an identifier
// followed by an expression is a defaulted name, `[a = d]`. A hole, a rest, or a
// nested pattern is a later slice, so it hands back rather than mislowering.
func (r *Renderer) classifyArrayElem(el frontend.Node) (arrayDefaultElem, error) {
	ec := r.prog.Children(el)
	switch {
	case len(ec) == 1 && ec[0].Kind() == frontend.NodeIdentifier:
		return arrayDefaultElem{nameNode: ec[0]}, nil
	case len(ec) == 2 && ec[0].Kind() == frontend.NodeIdentifier:
		return arrayDefaultElem{nameNode: ec[0], hasDefault: true, defNode: ec[1]}, nil
	default:
		return arrayDefaultElem{}, &NotYetLowerable{Reason: "an array destructuring hole, rest, or nested pattern is a later slice"}
	}
}

// defaultFillStmts emits the lazy default fill for one binding: the target is
// declared with its own type, the source slot is read once through a bounds-aware
// AtOpt into a temporary, and the default rides the undefined branch while the
// present branch takes the read value. The default is lowered by the caller so it
// is only placed on the undefined path, evaluating at most once and only when the
// slot is missing, the order JavaScript's default fill takes.
func (r *Renderer) defaultFillStmts(name string, nameGo ast.Expr, read ast.Expr, def ast.Expr) []ast.Stmt {
	opt := r.freshTemp()
	decl := &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{
		Names: []*ast.Ident{ident(name)},
		Type:  nameGo,
	}}}}
	present := &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(opt), Sel: ident("Get")}}
	fill := &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ident(opt)}, Tok: token.DEFINE, Rhs: []ast.Expr{read}},
		Cond: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(opt), Sel: ident("IsUndefined")}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{def}}}},
		Else: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{present}}}},
	}
	return []ast.Stmt{decl, fill}
}

// arrayOptRead builds the bounds-aware read for a defaulted array element,
// recv.AtOpt(i), whose Opt is undefined exactly when the source has no element at
// that index. It is the read defaultFillStmts tests, the optional sibling of the
// plain AtI read a non-defaulted element takes.
func arrayOptRead(recv ast.Expr, index int) ast.Expr {
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: recv, Sel: ident("AtOpt")},
		Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(index)}},
	}
}
