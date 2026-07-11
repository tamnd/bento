package lower

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the delete operator. delete obj.k removes a property from an
// object and evaluates to a boolean: true when the slot is gone (or was never
// there), false when the property refused removal. The shim gives delete no
// distinct kind, so it surfaces as the catch-all NodeUnknown with the operand as
// its one child and the operator keyword leading its source text, the same
// shape-plus-text recognition typeof uses.

// isDeleteExpr reports whether n is a delete expression, recognized the way
// isTypeofExpr recognizes typeof: a NodeUnknown with a single operand child whose
// source text leads with the delete keyword. A binding named something like
// deleted never matches, because it lexes as an identifier node, not this
// catch-all, and because the leading keyword run would not equal "delete".
func (r *Renderer) isDeleteExpr(n frontend.Node) bool {
	if n.Kind() != frontend.NodeUnknown || len(r.prog.Children(n)) != 1 {
		return false
	}
	return leadingKeyword(r.prog.Text(n)) == "delete"
}

// deleteExpr lowers delete operand. A property or element access on a dynamic
// receiver removes the named property at runtime through value.Value.Delete,
// which reports the boolean delete yields. Every other operand shape hands back:
// a static-shape receiver has its properties fixed as Go struct fields with no
// runtime slot to remove, and an identifier operand is a binding delete with its
// own strict-mode rules, both later slices.
func (r *Renderer) deleteExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "delete did not expose a single operand"}
	}
	target := r.unwrapParens(kids[0])
	switch target.Kind() {
	case frontend.NodePropertyAccessExpression:
		return r.deleteMember(target)
	case frontend.NodeElementAccessExpression:
		return r.deleteElement(target)
	case frontend.NodeIdentifier:
		return nil, &NotYetLowerable{Reason: "delete of an identifier binding is a later slice"}
	default:
		return r.deleteNonReference(target)
	}
}

// deleteNonReference lowers delete over an operand that is not a property
// reference, such as a literal, an arithmetic expression, or this. The delete
// operator evaluates such an operand and then yields true without removing
// anything, because there is no property slot to remove. A side-effect-free
// operand folds to the constant true, dropping the operand the way JavaScript
// discards its value; an operand that could run a call or an assignment hands back
// rather than lose that effect, since folding to true would drop it.
func (r *Renderer) deleteNonReference(target frontend.Node) (ast.Expr, error) {
	if !r.repeatableOperand(target) {
		return nil, &NotYetLowerable{Reason: "delete of a non-reference operand with a side effect is a later slice"}
	}
	return ident("true"), nil
}

// deleteMember lowers delete obj.k. Only a dynamic receiver lowers: its property
// lives in a runtime object whose slot Delete can remove, so the removal reads the
// same boxed value the matching Get read would. A statically typed receiver keeps
// its properties as Go struct fields with no slot to drop, so it hands back for
// the object descriptor model a later phase builds.
func (r *Renderer) deleteMember(target frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(target)
	if len(kids) != 2 {
		return nil, &NotYetLowerable{Reason: "delete target did not expose an object and a property name"}
	}
	obj, nameNode := kids[0], kids[1]
	if !r.isDynamic(obj) {
		return nil, &NotYetLowerable{Reason: "delete of a statically typed property needs the object descriptor model, a later slice"}
	}
	recv, err := r.lowerExpr(obj)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	key := &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(r.prog.Text(nameNode))}}}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Delete")}, Args: []ast.Expr{key}}, nil
}

// deleteElement lowers delete obj[k] with a computed key. It mirrors the dynamic
// element read: the key is coerced to a property key by its type, a number through
// DeleteIndex, another dynamic value through DeleteElem, and a string used as is
// through Delete, so the removed slot is the same one the matching read would
// reach. Only a dynamic receiver lowers, for the reason deleteMember hands back a
// statically typed one; a key that is neither number, string, nor dynamic is its
// own later slice.
func (r *Renderer) deleteElement(target frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(target)
	if len(kids) != 2 {
		return nil, &NotYetLowerable{Reason: "delete target did not expose an object and an index"}
	}
	obj, idxNode := kids[0], kids[1]
	if !r.isDynamic(obj) {
		return nil, &NotYetLowerable{Reason: "delete of a statically typed property needs the object descriptor model, a later slice"}
	}
	recv, err := r.lowerExpr(obj)
	if err != nil {
		return nil, err
	}
	idx, err := r.lowerExpr(idxNode)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	switch {
	case r.isNumber(idxNode):
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("DeleteIndex")}, Args: []ast.Expr{idx}}, nil
	case r.isDynamic(idxNode):
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("DeleteElem")}, Args: []ast.Expr{idx}}, nil
	case r.isString(idxNode):
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Delete")}, Args: []ast.Expr{idx}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "delete with a non-number, non-string index is a later slice"}
	}
}
