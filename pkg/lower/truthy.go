package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers JavaScript truthiness, the ToBoolean an operand undergoes when
// it stands in boolean position (05_type_lowering, the boolean section). A Go if,
// for, or ! wants a real bool, but JavaScript takes any value there and reads it
// through the falsy set (false, 0, -0, NaN, "", null, undefined), so a non-boolean
// operand lowers to the test that reproduces that set for its type rather than to a
// bare Go truth value it does not have.
//
// A boolean operand already is the bool the position wants, so it passes through. A
// number is falsy only at zero and NaN, a string only when empty, a bigint only at 0n,
// and those tests are the ones a Go comparison does not spell on its own: a bare x != 0
// keeps NaN, which is falsy, a Go string has no direct emptiness idiom for value.BStr,
// and a bigint is a *big.Int with no == against a literal. A non-primitive the checker
// proved always truthy or always falsy (an object is always truthy, a
// null/undefined/void-only type always falsy) collapses to the Go boolean constant, or
// to that constant behind the operand's evaluation when the operand has a side effect
// that must still fire. A tagged-sum union reads its truth through the ToBoolean method
// the renderer emits for it, which switches the tag to the active arm's falsy rule.
//
// What is left is a union whose arms include an object, which has no interned form to
// switch on yet, and a side-effecting operand whose type is only null, undefined, or
// void, which may lower to a Go call that returns nothing to discard. Those hand back.

// lowerTruthy lowers an operand standing in boolean position to a Go bool: the
// operand itself when it is already boolean, and the type's ToBoolean test
// otherwise. A pure operand inlines the comparison, the readable form a person
// writes; an operand with a side effect routes through the shared value helper so
// it is evaluated once, since the inlined form names the operand twice.
func (r *Renderer) lowerTruthy(n frontend.Node) (ast.Expr, error) {
	if r.isBool(n) {
		lowered, err := r.lowerExpr(n)
		if err != nil {
			return nil, err
		}
		// A boolean-typed operand usually lowers to a Go bool the position already
		// wants, but one whose runtime form is a value.Value box (obj.hasOwnProperty(k),
		// typed boolean by the checker yet dispatched through the runtime Call) would
		// leave a value.Value where an if or a ! needs a bool. coerceBoxedBooleanOperand
		// reads such a box back through value.ToBoolean and passes a native bool through
		// untouched, so if (obj.hasOwnProperty(k)) compiles the same as any other guard.
		return r.coerceBoxedBooleanOperand(n, lowered), nil
	}
	// A non-primitive operand the checker proved always truthy or always falsy
	// collapses to that Go boolean constant: an object, array, function, or class
	// instance is always truthy, and a type that is only null, undefined, or void is
	// always falsy. This is the object-in-boolean-position case, where the value has
	// no falsy member to test. The collapse is taken only for a repeatable operand,
	// so dropping its evaluation loses no side effect; an operand with a side effect
	// keeps its runtime test and falls through to the per-kind handling below.
	if val, known := r.staticTruthy(n); known && r.repeatableOperand(n) {
		if val {
			return ident("true"), nil
		}
		return ident("false"), nil
	}
	switch {
	case r.isNumber(n):
		return r.numberTruthy(n)
	case r.isString(n):
		return r.stringTruthy(n)
	case r.isBigInt(n):
		return r.bigIntTruthy(n)
	case r.isDynamic(n):
		// A dynamic operand's kind is only known at runtime, so the whole falsy
		// set is one call into the value model's ToBoolean, the same test Or and
		// And run on their left operand.
		x, err := r.lowerExpr(n)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "ToBoolean"), Args: []ast.Expr{x}}, nil
	}
	// An optional operand (T | undefined, a value.Opt[T]) is falsy two ways: it is
	// undefined, or it is present but its inner value is falsy. So if (x) over an
	// optional tests both, presence and the inner ToBoolean: !x.IsUndefined() && the
	// inner test on x.Get(). The inner test is spelled two ways: for a primitive inner
	// a Go comparison reproduces (a number, string, or boolean) it is the inline
	// ToBoolean on x.Get(), and for an inner the checker proved always truthy when
	// present (an object, array, or class-instance shape, which carries no falsy
	// member) it drops away, leaving the presence test alone. The operand is named
	// twice, so a repeatable one inlines here while a wider or side-effecting optional
	// keeps the handback below.
	if opt, kind, ok, err := r.optionalTruthy(n); err != nil {
		return nil, err
	} else if ok {
		present := &ast.UnaryExpr{Op: token.NOT, X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: opt, Sel: ident("IsUndefined")}}}
		if kind == "present" {
			return present, nil
		}
		get := &ast.CallExpr{Fun: &ast.SelectorExpr{X: opt, Sel: ident("Get")}}
		if kind == "bigint" {
			r.requireImport(valuePkg)
		}
		return &ast.BinaryExpr{X: present, Op: token.LAND, Y: truthyOfKind(get, kind)}, nil
	}
	// A tagged-sum union operand reads its truth through the ToBoolean method the
	// renderer emits for it: the method switches the tag to the active arm's falsy rule,
	// so if (x) over a number | string | undefined is falsy for undefined, a zero or NaN
	// number, and an empty string, and truthy otherwise. The union is evaluated once by
	// the method call, so a side-effecting operand keeps its effect, unlike the inlined
	// primitive forms that name the operand twice.
	if _, ok := r.unionTruthy(n); ok {
		x, err := r.lowerExpr(n)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: x, Sel: ident("ToBoolean")}}, nil
	}
	// A non-primitive the checker proved always truthy but whose evaluation has a side
	// effect cannot collapse to the Go constant the way the repeatable form above does,
	// since dropping the operand would drop the effect. It lowers to the constant behind
	// the evaluation instead, the immediately invoked func the logical path already uses
	// to hold a statement in expression position: `if (build())` runs build, discards
	// what it built, and takes the branch, which is what an object in boolean position
	// means.
	//
	// Only the always-truthy half takes this. An operand whose type is only null,
	// undefined, or void may lower to a Go call that returns nothing, which cannot stand
	// on the right of the blank assignment, so it keeps the handback below.
	if val, known := r.staticTruthy(n); known && val {
		x, err := r.lowerExpr(n)
		if err != nil {
			return nil, err
		}
		lit := &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  &ast.FieldList{},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ident("bool")}}},
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{
				&ast.AssignStmt{Lhs: []ast.Expr{ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{x}},
				&ast.ReturnStmt{Results: []ast.Expr{ident("true")}},
			}},
		}
		return &ast.CallExpr{Fun: lit}, nil
	}
	return nil, r.notLowerable(n, "truthiness of a union or a side-effecting non-primitive is a later slice")
}

// unionTruthy reports whether an operand in boolean position is a tagged-sum union
// whose truthiness the ToBoolean method can spell, and marks the union so the renderer
// emits that method. It fires for a primitive union all of whose arms are a number,
// string, boolean, or a tag-only sentinel; a union carrying a bigint or object arm has
// no inline truthiness yet and reports false, keeping the caller on its handback. An
// operand whose type is not an interned union, or an optional the earlier path already
// took, reports false too.
func (r *Renderer) unionTruthy(n frontend.Node) (*unionInfo, bool) {
	info, ok := r.unionInfoOrIntern(r.prog.TypeAt(n))
	if !ok || !unionToBooleanSupported(info) {
		return nil, false
	}
	info.needsToBoolean = true
	// A bigint arm's case calls value.BigIntToBool, and a union of a bigint with a
	// sentinel names nothing else out of the value package, so the import is asked for
	// here rather than left to whatever else the file happens to reach.
	for _, a := range info.arms {
		if a.flag&frontend.TypeBigInt != 0 {
			r.requireImport(valuePkg)
		}
	}
	return info, true
}

// optionalTruthy reports whether an operand in boolean position is an optional whose
// inner test lowerTruthy can spell inline, and returns the lowered optional alongside a
// kind that names that inner test so the caller can build !x.IsUndefined() and, unless
// the inner is always truthy, the inner ToBoolean on x.Get(). It fires only for a
// repeatable operand, since the presence test and the inner test each name the optional
// once. The inner qualifies two ways: a primitive with an inline ToBoolean (a number,
// string, or boolean, kind "number", "string", or "bool"), or a shape the checker
// proved always truthy when present (an object, array, or class instance, kind
// "present"), which needs no inner test at all. A non-optional, a non-repeatable
// optional, or an optional over a wider inner (a null-or-undefined inner with no truthy
// case, or a dynamic one) reports ok false and leaves the caller on its handback.
func (r *Renderer) optionalTruthy(n frontend.Node) (opt ast.Expr, kind string, ok bool, err error) {
	if !r.isOptionalType(r.prog.TypeAt(n)) || !r.repeatableOperand(n) {
		return nil, "", false, nil
	}
	inner, isOpt := r.optionalInner(r.prog.UnionMembers(r.prog.TypeAt(n)))
	if !isOpt {
		return nil, "", false, nil
	}
	switch {
	case primitiveTruthyKindOK(inner.Flags):
		kind, _ = primitiveTruthyKind(inner.Flags)
	case staticTruthyIsTrue(inner.Flags):
		kind = "present"
	default:
		return nil, "", false, nil
	}
	opt, err = r.lowerExpr(n)
	if err != nil {
		return nil, "", false, err
	}
	return opt, kind, true, nil
}

// primitiveTruthyKindOK reports whether a type's flags name a primitive with an inline
// ToBoolean, the precondition primitiveTruthyKind's ok return states.
func primitiveTruthyKindOK(f frontend.TypeFlags) bool {
	_, ok := primitiveTruthyKind(f)
	return ok
}

// staticTruthyIsTrue reports whether a type's flags mark it always truthy when present,
// so an optional over it is truthy exactly when present and its inner ToBoolean drops
// away. It reuses the always-truthy half of staticTruthyFlags.
func staticTruthyIsTrue(f frontend.TypeFlags) bool {
	val, known := staticTruthyFlags(f)
	return known && val
}

// primitiveTruthyKind maps a primitive type's flags to the kind truthyOfKind spells an
// inline ToBoolean for: a number, a string, a boolean, or a bigint. A union (more than
// one flag past the primitive bit) or any other type reports ok false, so the caller
// keeps its handback rather than test a shape with no single inline falsy rule.
func primitiveTruthyKind(f frontend.TypeFlags) (string, bool) {
	if f&frontend.TypeUnion != 0 {
		return "", false
	}
	switch {
	case f&frontend.TypeNumber != 0:
		return "number", true
	case f&frontend.TypeString != 0:
		return "string", true
	case f&frontend.TypeBoolean != 0:
		return "bool", true
	case f&frontend.TypeBigInt != 0:
		return "bigint", true
	}
	return "", false
}

// staticTruthy reports whether the checker proved an operand's type is always
// truthy or always falsy, so a condition or logical operand over it collapses to
// the branch that runs instead of testing a value whose outcome is already fixed
// (05_type_lowering, the boolean item on collapsing truthiness to a constant). A
// plain object type, an object literal, an array, a function, or a class instance,
// is always truthy: it carries no null or undefined and is not a falsy primitive.
// A type that is only null, undefined, or void is always falsy. Every other type,
// a number or string that could be zero or empty, a boolean, a union, or a dynamic
// value, is not statically known and reports known false, so it keeps its runtime
// test.
func (r *Renderer) staticTruthy(n frontend.Node) (val, known bool) {
	return staticTruthyFlags(r.prog.TypeAt(n).Flags)
}

// staticTruthyFlags is the flags-level core of staticTruthy, shared with the optional
// path so an option's inner type is read for the same always-truthy or always-falsy
// verdict. A plain object type (which covers arrays and class instances too) is always
// truthy; a type that is only null, undefined, or void is always falsy; every other
// type reports known false.
func staticTruthyFlags(f frontend.TypeFlags) (val, known bool) {
	if f == frontend.TypeObject {
		return true, true
	}
	if f != 0 && f&^(frontend.TypeNull|frontend.TypeUndefined|frontend.TypeVoid) == 0 {
		return false, true
	}
	return false, false
}

// numberTruthy lowers a number in boolean position to its ToBoolean: false at zero
// and NaN, true otherwise. The inlined form is x != 0 && x == x, the zero test with
// the NaN guard riding along (x == x is false only for NaN, which a bare x != 0
// would wrongly call truthy). A side-effecting operand cannot appear twice, so it
// calls value.NumberToBool, the same test behind one evaluation.
func (r *Renderer) numberTruthy(n frontend.Node) (ast.Expr, error) {
	x, err := r.lowerExpr(n)
	if err != nil {
		return nil, err
	}
	if !r.pureCtorValue(n) {
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "NumberToBool"), Args: []ast.Expr{x}}, nil
	}
	return truthyOfKind(x, "number"), nil
}

// stringTruthy lowers a string in boolean position to its ToBoolean: false only for
// the empty string, true for any content, so "0" and "false" are both truthy. The
// inlined form is s.Length() > 0, the code-unit count against zero; a side-effecting
// operand calls value.StringToBool, the same test behind one evaluation.
func (r *Renderer) stringTruthy(n frontend.Node) (ast.Expr, error) {
	s, err := r.lowerExpr(n)
	if err != nil {
		return nil, err
	}
	if !r.pureCtorValue(n) {
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "StringToBool"), Args: []ast.Expr{s}}, nil
	}
	return truthyOfKind(s, "string"), nil
}

// bigIntTruthy lowers a bigint in boolean position to its ToBoolean: false only for
// 0n, true for every other value, negative ones included. It is one call either way,
// value.BigIntToBool over the *big.Int, so unlike the number and string forms it needs
// no separate spelling for an operand with a side effect: the operand is named once.
func (r *Renderer) bigIntTruthy(n frontend.Node) (ast.Expr, error) {
	b, err := r.lowerExpr(n)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return truthyOfKind(b, "bigint"), nil
}

// truthyOfKind builds the inlined ToBoolean test for a Go expression whose kind is
// already known, the falsy set spelled out for each primitive: a number is truthy
// when non-zero and not NaN (x != 0 && x == x), a string when non-empty
// (s.Length() > 0), a bigint when it is not 0n, and a boolean is its own truth. The
// expression is named more than once in the number form, so a caller passes one it can
// safely repeat, a literal, an identifier, or a lowered pure operand. It returns nil
// for a kind without an inline test, which no caller reaches.
//
// The bigint form is a call rather than a comparison because a bigint is a *big.Int and
// Go has no == against a zero literal for one; value.BigIntToBool is the sign test, and
// it also answers false for the nil pointer an unset arm carries. A caller that emits
// this kind asks for the value import, since a union of a bigint with a sentinel names
// nothing else out of that package.
func truthyOfKind(x ast.Expr, kind string) ast.Expr {
	switch kind {
	case "bool":
		return x
	case "bigint":
		return &ast.CallExpr{Fun: sel("value", "BigIntToBool"), Args: []ast.Expr{x}}
	case "number":
		return &ast.BinaryExpr{
			X:  &ast.BinaryExpr{X: x, Op: token.NEQ, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}},
			Op: token.LAND,
			Y:  &ast.BinaryExpr{X: x, Op: token.EQL, Y: x},
		}
	case "string":
		return &ast.BinaryExpr{
			X:  &ast.CallExpr{Fun: &ast.SelectorExpr{X: x, Sel: ident("Length")}},
			Op: token.GTR,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
		}
	}
	return nil
}
