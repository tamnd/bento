package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers nullish coalescing, a ?? b (05_type_lowering, the null and
// undefined section). ?? yields a when a is neither null nor undefined and b
// otherwise, so it is a presence test on the left, never js.Truthy: an empty
// string or a zero, which are falsy but not nullish, keep the left value.
//
// The one nullish shape this slice models is the optional value.Opt[T], the
// lowering of T | undefined, whose only nullish value is undefined and whose
// presence flag is exactly the test ?? runs. A T | null or a dynamic operand is
// a different representation and hands back. Because the fallback lowers to a
// plain function argument, it is evaluated eagerly, so the form is taken only
// when the fallback is side-effect free; a pure fallback run early cannot be
// observed out of order, which keeps ??'s short-circuit intact. A side-effecting
// fallback needs the statement hoisting a later slice builds.

// nullishCoalesce lowers a ?? b. The left must be an optional, so its presence
// flag carries the nullish test; the right must be a pure expression, so eager
// evaluation is sound. When the right is itself optional the whole expression is
// optional and lowers through OrOpt, keeping the Opt result; otherwise the right
// is the element type and Or returns the bare value.
func (r *Renderer) nullishCoalesce(left, right frontend.Node) (ast.Expr, error) {
	if !r.isOptional(left) {
		return nil, &NotYetLowerable{Reason: "nullish coalescing whose left is not the optional T | undefined (a T | null or dynamic operand) is a later slice"}
	}
	if !r.pureCtorValue(right) {
		return nil, &NotYetLowerable{Reason: "nullish coalescing with a side-effecting fallback needs statement hoisting, a later slice"}
	}
	if r.isDynamic(right) {
		return nil, &NotYetLowerable{Reason: "nullish coalescing with a dynamic fallback is a later slice"}
	}
	opt, err := r.lowerExpr(left)
	if err != nil {
		return nil, err
	}
	fallback, err := r.lowerExpr(right)
	if err != nil {
		return nil, err
	}
	// A fallback that is itself optional keeps the result optional, so the two
	// Opt values combine through OrOpt; both lower to the bare Opt because their
	// type still carries undefined and so is never unwrapped.
	if r.isOptional(right) {
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: opt, Sel: ident("OrOpt")}, Args: []ast.Expr{fallback}}, nil
	}
	// The fallback is the element type. Bridge it against the optional's inner so
	// a derived-class fallback upcasts to the base the slot declares, the same
	// way any binding into the element type does; a primitive passes through.
	inner, ok := r.optionalInner(r.prog.UnionMembers(r.prog.TypeAt(left)))
	if ok {
		fallback, err = r.bridgeClassBinding(fallback, right, inner)
		if err != nil {
			return nil, err
		}
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: opt, Sel: ident("Or")}, Args: []ast.Expr{fallback}}, nil
}
