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
		if expr, ok, err := r.nullableUnionCoalesce(left, right); ok || err != nil {
			return expr, err
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

// nullableUnionCoalesce lowers a ?? b when the left is a tagged-sum nullable union
// (T | null or T | null | undefined) with a single primitive value arm, the read-side
// mirror of the ??= nullable-union store. The left is bound once to a temp, its tag is
// tested against the sentinel arms, and the result is the fallback on a nullish tag or
// the value arm's field otherwise, so the short-circuit is exact even for a
// side-effecting fallback. The whole expression's Go type is the value arm's type, so
// the fallback must be a definite value of that same arm: a fallback that widens the
// type (a string fallback for a number arm) or is itself nullable would make the result
// a wider union the value arm's field cannot hold, and hands back. ok=false means the
// shape is not a single-primitive-arm nullable union, so the caller keeps its own
// handback.
func (r *Renderer) nullableUnionCoalesce(left, right frontend.Node) (ast.Expr, bool, error) {
	info, ok := r.unionInfoOrIntern(r.prog.TypeAt(left))
	if !ok {
		return nil, false, nil
	}
	// Find the single value arm. An object arm (a pointer-field nullable object union)
	// or more than one value arm is a different representation this slice does not model.
	var varm unionArm
	valueArms := 0
	for _, a := range info.arms {
		if a.tagOnly {
			continue
		}
		if a.isObject || a.field == "" {
			return nil, false, nil
		}
		varm = a
		valueArms++
	}
	if valueArms != 1 {
		return nil, false, nil
	}
	// The union must carry the null sentinel: a T | undefined-only union is the value.Opt
	// shape the optional path handles and never reaches here.
	nullArm, hasNull := info.armForFlags(frontend.TypeNull)
	if !hasNull {
		return nil, false, nil
	}
	undefArm, hasUndef := info.armForFlags(frontend.TypeUndefined)
	// The fallback must be a definite value of the value arm: a non-union type whose
	// primitive selects the same arm. A union fallback (a wider or still-nullable type),
	// a dynamic fallback, or a fallback of a different primitive would not fit the value
	// arm's Go field, so it hands back rather than emit a widening the field cannot hold.
	rt := r.prog.TypeAt(right)
	if rt.Flags&frontend.TypeUnion != 0 {
		return nil, true, &NotYetLowerable{Reason: "?? on a T | null with a union or nullable fallback is a later slice"}
	}
	if fbArm, ok := info.armForFlags(rt.Flags); !ok || fbArm.field != varm.field {
		return nil, true, &NotYetLowerable{Reason: "?? on a T | null whose fallback is not a definite value of the union's value arm is a later slice"}
	}
	l, err := r.lowerExpr(left)
	if err != nil {
		return nil, true, err
	}
	fallback, err := r.lowerExpr(right)
	if err != nil {
		return nil, true, err
	}
	// The nullish guard is the tag against the sentinel arms, null ored with undefined
	// when the union carries both, the same tag test the sentinel compare emits.
	tmp := r.freshTemp()
	guard := ast.Expr(&ast.BinaryExpr{
		X:  &ast.SelectorExpr{X: ident(tmp), Sel: ident("tag")},
		Op: token.EQL,
		Y:  ident(info.tagConst(nullArm)),
	})
	if hasUndef {
		guard = &ast.BinaryExpr{
			X:  guard,
			Op: token.LOR,
			Y: &ast.BinaryExpr{
				X:  &ast.SelectorExpr{X: ident(tmp), Sel: ident("tag")},
				Op: token.EQL,
				Y:  ident(info.tagConst(undefArm)),
			},
		}
	}
	body := []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{l}},
		&ast.IfStmt{
			Cond: guard,
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{fallback}}}},
		},
		&ast.ReturnStmt{Results: []ast.Expr{&ast.SelectorExpr{X: ident(tmp), Sel: ident(varm.field)}}},
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: varm.goType}}}},
		Body: &ast.BlockStmt{List: body},
	}
	return &ast.CallExpr{Fun: lit}, true, nil
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
