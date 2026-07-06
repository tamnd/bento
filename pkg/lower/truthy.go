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
// number is falsy only at zero and NaN, a string only when empty, and those two
// tests are the ones a Go comparison does not spell on its own: a bare x != 0 keeps
// NaN, which is falsy, and a Go string has no direct emptiness idiom for value.BStr.
// An object, a union, or a dynamic value has a falsy rule this slice does not model
// yet and hands the unit back rather than guess one.

// lowerTruthy lowers an operand standing in boolean position to a Go bool: the
// operand itself when it is already boolean, and the type's ToBoolean test
// otherwise. A pure operand inlines the comparison, the readable form a person
// writes; an operand with a side effect routes through the shared value helper so
// it is evaluated once, since the inlined form names the operand twice.
func (r *Renderer) lowerTruthy(n frontend.Node) (ast.Expr, error) {
	if r.isBool(n) {
		return r.lowerExpr(n)
	}
	switch {
	case r.isNumber(n):
		return r.numberTruthy(n)
	case r.isString(n):
		return r.stringTruthy(n)
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
	return nil, &NotYetLowerable{Reason: "truthiness of a non-primitive (object or union) is a later slice"}
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
	f := r.prog.TypeAt(n).Flags
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

// truthyOfKind builds the inlined ToBoolean test for a Go expression whose kind is
// already known, the falsy set spelled out for each primitive: a number is truthy
// when non-zero and not NaN (x != 0 && x == x), a string when non-empty
// (s.Length() > 0), and a boolean is its own truth. The expression is named more
// than once in the number form, so a caller passes one it can safely repeat, a
// literal, an identifier, or a lowered pure operand. It returns nil for a kind
// without an inline test, which no caller reaches.
func truthyOfKind(x ast.Expr, kind string) ast.Expr {
	switch kind {
	case "bool":
		return x
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
