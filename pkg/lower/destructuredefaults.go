package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

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

// objectDefaultElem describes one element of an object binding pattern once its
// shape is classified: a plain shorthand name binds the property of the same name,
// a defaulted shorthand name fills from its default when the property is undefined.
type objectDefaultElem struct {
	name       string
	nameNode   frontend.Node
	hasDefault bool
	defNode    frontend.Node
}

// classifyObjectElem reads one object binding pattern element into an
// objectDefaultElem. A single identifier is a plain shorthand, `{x}`; an
// identifier followed by an expression under an `=` separator is a shorthand
// default, `{x = d}`. A rename (`{a: b}`), a rename carrying a default, a rest, or
// a nested pattern is a later slice, so it hands back. The separator between the
// name and the second child tells a default (`=`) from a rename (`:`), which the
// child kinds alone cannot when the default is itself an identifier.
func (r *Renderer) classifyObjectElem(el frontend.Node) (objectDefaultElem, error) {
	ec := r.prog.Children(el)
	switch {
	case len(ec) == 1 && ec[0].Kind() == frontend.NodeIdentifier:
		return objectDefaultElem{nameNode: ec[0]}, nil
	case len(ec) == 2 && ec[0].Kind() == frontend.NodeIdentifier && !strings.Contains(r.elemSeparator(el, ec[0], ec[1]), ":"):
		return objectDefaultElem{nameNode: ec[0], hasDefault: true, defNode: ec[1]}, nil
	default:
		return objectDefaultElem{}, &NotYetLowerable{Reason: "an object destructuring rename, default, rest, or nested pattern is a later slice"}
	}
}

// elemSeparator returns the source text between two children of a pattern element,
// the operator that joins them: `=` for a default, `:` for a rename. It reads the
// gap by the children's spans relative to the element, so it sees only the joining
// token and never the default expression's own text, which may itself contain a
// colon.
func (r *Renderer) elemSeparator(el, first, second frontend.Node) string {
	txt := r.prog.Text(el)
	lo := int(first.End()) - int(el.Pos())
	hi := int(second.Pos()) - int(el.Pos())
	if lo < 0 || hi > len(txt) || lo > hi {
		return ""
	}
	return txt[lo:hi]
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
