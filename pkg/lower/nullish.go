package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers nullish coalescing, a ?? b (05_type_lowering, the null and
// undefined section). ?? yields a when a is neither null nor undefined and b
// otherwise, so it is a presence test on the left, never js.Truthy: an empty
// string or a zero, which are falsy but not nullish, keep the left value.
//
// The one nullish shape this slice models is the optional value.Opt[T], the
// lowering of T | undefined, whose only nullish value is undefined and whose
// presence flag is exactly the test ?? runs, plus the dynamic value whose test
// is the runtime IsNullish. A T | null is a different representation and hands
// back. A pure fallback keeps the compact Or/OrOpt/Coalesce form, where the
// fallback is a plain argument: run early it cannot be observed out of order, so
// ??'s short-circuit is intact. A side-effecting fallback (a call, an
// allocation) must not run when the left is present, so it lowers to a lazy
// closure that binds the left once, tests its presence, and evaluates the
// fallback only on the nullish branch, keeping the short-circuit exactly.

// nullishCoalesce lowers a ?? b. The left must be an optional, so its presence
// flag carries the nullish test; the right must be a pure expression, so eager
// evaluation is sound. When the right is itself optional the whole expression is
// optional and lowers through OrOpt, keeping the Opt result; otherwise the right
// is the element type and Or returns the bare value.
func (r *Renderer) nullishCoalesce(left, right frontend.Node) (ast.Expr, error) {
	if !r.isOptional(left) {
		if r.isDynamic(left) {
			return r.dynamicNullishCoalesce(left, right)
		}
		return nil, &NotYetLowerable{Reason: "nullish coalescing whose left is a T | null, not the optional T | undefined or a dynamic operand, is a later slice"}
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
	rightOptional := r.isOptional(right)
	// The fallback for a definite right is the element type. Bridge it against the
	// optional's inner so a derived-class fallback upcasts to the base the slot
	// declares, the same way any binding into the element type does; a primitive
	// passes through. An optional right keeps the bare Opt and needs no bridge.
	inner, ok := r.optionalInner(r.prog.UnionMembers(r.prog.TypeAt(left)))
	if !ok {
		return nil, &NotYetLowerable{Reason: "nullish coalescing whose left optional has no inner type is a later slice"}
	}
	if !rightOptional {
		fallback, err = r.bridgeClassBinding(fallback, right, inner)
		if err != nil {
			return nil, err
		}
	}
	// A pure fallback is safe to evaluate eagerly, so it stays the compact form: a
	// fallback that is itself optional keeps the result optional and combines
	// through OrOpt, and a definite fallback returns the bare element type through
	// Or. Both lower the operands as plain arguments.
	if r.pureCtorValue(right) {
		if rightOptional {
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: opt, Sel: ident("OrOpt")}, Args: []ast.Expr{fallback}}, nil
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: opt, Sel: ident("Or")}, Args: []ast.Expr{fallback}}, nil
	}
	// A side-effecting fallback rides a lazy closure: the optional is bound once to
	// a temp, and the fallback runs only when the temp is undefined. An optional
	// fallback keeps the result optional and returns the temp itself when present;
	// a definite fallback returns the temp's inner value through Get.
	innerType, err := r.typeExpr(inner)
	if err != nil {
		return nil, err
	}
	tmp := r.freshTemp()
	retType := innerType
	present := ast.Expr(&ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(tmp), Sel: ident("Get")}})
	if rightOptional {
		r.requireImport(valuePkg)
		retType = &ast.IndexExpr{X: sel("value", "Opt"), Index: innerType}
		present = ident(tmp)
	}
	body := []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{opt}},
		&ast.IfStmt{
			Cond: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(tmp), Sel: ident("IsUndefined")}},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{fallback}}}},
		},
		&ast.ReturnStmt{Results: []ast.Expr{present}},
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: retType}}}},
		Body: &ast.BlockStmt{List: body},
	}
	return &ast.CallExpr{Fun: lit}, nil
}

// dynamicNullishCoalesce lowers a ?? b when the left is a dynamic value, whose
// nullish test is the runtime IsNullish rather than an Opt presence flag. It
// returns the left when it is neither null nor undefined and the right otherwise.
// Both sides box to a Value, so the result keeps the left's runtime kind or the
// fallback's, and a dynamic fallback is admitted since the value model works in
// boxed values. A pure fallback keeps the compact value.Coalesce(a, b), whose
// eager argument cannot be observed out of order; a side-effecting fallback rides
// a lazy closure that binds the left once and runs the fallback only when the
// left tests nullish, the same short-circuit the Opt path keeps.
func (r *Renderer) dynamicNullishCoalesce(left, right frontend.Node) (ast.Expr, error) {
	l, err := r.boxOperand(left)
	if err != nil {
		return nil, err
	}
	fallback, err := r.boxOperand(right)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	if r.pureCtorValue(right) {
		return &ast.CallExpr{Fun: sel("value", "Coalesce"), Args: []ast.Expr{l, fallback}}, nil
	}
	tmp := r.freshTemp()
	body := []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{l}},
		&ast.IfStmt{
			Cond: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(tmp), Sel: ident("IsNullish")}},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{fallback}}}},
		},
		&ast.ReturnStmt{Results: []ast.Expr{ident(tmp)}},
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "Value")}}}},
		Body: &ast.BlockStmt{List: body},
	}
	return &ast.CallExpr{Fun: lit}, nil
}
