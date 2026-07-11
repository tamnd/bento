package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the assignment edge cases M4 left handed back: a computed
// member target on a dynamic receiver, both the plain store and the compound
// read-modify-write, and a compound assignment read for its value. The plain
// element store and the identifier assignment-as-value already lower elsewhere;
// what remains is the computed target and the compound result, which need the
// runtime element store and the boxed dynamic arithmetic the value model gives.

// elementStoreMethod picks the runtime store method for a bracket write o[k] = v
// on a dynamic receiver by the key's own type, the write mirror of the dynamic
// element read: a number index writes through SetIndex, another dynamic value
// through SetElem, and a string key through SetKey, so o[3] = x lands in an array
// element and o["k"] = x in a named property by the same rule the read resolves
// them. A key that is none of these is its own later slice.
func (r *Renderer) elementStoreMethod(idxNode frontend.Node) (string, error) {
	switch {
	case r.isNumber(idxNode):
		return "SetIndex", nil
	case r.isDynamic(idxNode):
		return "SetElem", nil
	case r.isString(idxNode):
		return "SetKey", nil
	default:
		return "", &NotYetLowerable{Reason: "a dynamic element write with a non-number, non-string index is a later slice"}
	}
}

// assignValueElement lowers a computed-member assignment read for its value,
// (o[k] = v) in an expression position, on a dynamic receiver. The runtime store
// returns the boxed assigned value, so when the whole expression is itself
// dynamic the bare store call both writes and yields it, evaluating the receiver,
// key, and value once in source order. When the checker types the expression a
// static primitive, that being the assigned value's type, the box is read back to
// its static form through the same coercion a dynamic read into that slot takes,
// still a single evaluation. A non-dynamic receiver has no runtime element slot
// this path writes and hands back, the wall the static array element write hits.
func (r *Renderer) assignValueElement(n, left, right frontend.Node) (ast.Expr, error) {
	parts := r.prog.Children(left)
	if len(parts) != 2 {
		return nil, &NotYetLowerable{Reason: "element assignment value target did not expose a receiver and an index"}
	}
	recvNode, idxNode := parts[0], parts[1]
	if !r.isDynamic(recvNode) {
		return nil, &NotYetLowerable{Reason: "assignment value into a statically typed element needs the object descriptor model, a later slice"}
	}
	method, err := r.elementStoreMethod(idxNode)
	if err != nil {
		return nil, err
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	idx, err := r.lowerExpr(idxNode)
	if err != nil {
		return nil, err
	}
	val, err := r.boxOperand(right)
	if err != nil {
		return nil, err
	}
	store := &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(method)}, Args: []ast.Expr{idx, val}}
	if r.isDynamic(n) {
		return store, nil
	}
	return r.coerceDynamicToStatic(store, n)
}
