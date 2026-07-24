package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the void operator. void x evaluates its operand and then
// evaluates to undefined, always, whatever the operand yields. The shim gives void
// no distinct kind, so it surfaces as the catch-all NodeUnknown with the operand as
// its one child and the operator keyword leading its source text, the same
// shape-plus-text recognition typeof and delete use.

// isVoidExpr reports whether n is a void expression, recognized the way isTypeofExpr
// recognizes typeof: a NodeUnknown with a single operand child whose source text
// leads with the void keyword. A binding named something like voidx never matches,
// because it lexes as an identifier node, not this catch-all, and because the
// leading keyword run would not equal "void".
func (r *Renderer) isVoidExpr(n frontend.Node) bool {
	if n.Kind() != frontend.NodeUnknown || len(r.prog.Children(n)) != 1 {
		return false
	}
	return leadingKeyword(r.prog.Text(n)) == "void"
}

// voidExpr lowers void operand. The whole expression is always undefined, so it
// lowers to the value.Undefined singleton, the same value the undefined literal
// reads. void otherwise evaluates its operand, and folding to the singleton drops
// the operand from the output, so an operand that could run a call or an assignment
// hands back rather than lose that effect, the discipline typeof and delete take for
// their own constant folds. A side-effect-free operand (void 0 the canonical one)
// folds cleanly, since dropping it changes nothing.
func (r *Renderer) voidExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "void did not expose a single operand"}
	}
	if !r.repeatableOperand(r.unwrapParens(kids[0])) {
		return nil, &NotYetLowerable{Reason: "void over an operand with a side effect is a later slice"}
	}
	r.requireImport(valuePkg)
	return sel("value", "Undefined"), nil
}
