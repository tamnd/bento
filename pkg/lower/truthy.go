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
	}
	return nil, &NotYetLowerable{Reason: "truthiness of a non-primitive (object, union, or dynamic value) is a later slice"}
}

// numberTruthy lowers a number in boolean position to its ToBoolean: false at zero
// and NaN, true otherwise. The inlined form is x != 0 && x == x, the zero test with
// the NaN guard riding along (x == x is false only for NaN, which a bare x != 0
// would wrongly call truthy). A side-effecting operand cannot appear twice, so it
// calls value.NumberToBool, the same test behind one evaluation.
func (r *Renderer) numberTruthy(n frontend.Node) (ast.Expr, error) {
	if !r.pureCtorValue(n) {
		x, err := r.lowerExpr(n)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "NumberToBool"), Args: []ast.Expr{x}}, nil
	}
	nonZero, err := r.lowerExpr(n)
	if err != nil {
		return nil, err
	}
	left, err := r.lowerExpr(n)
	if err != nil {
		return nil, err
	}
	right, err := r.lowerExpr(n)
	if err != nil {
		return nil, err
	}
	return &ast.BinaryExpr{
		X:  &ast.BinaryExpr{X: nonZero, Op: token.NEQ, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}},
		Op: token.LAND,
		Y:  &ast.BinaryExpr{X: left, Op: token.EQL, Y: right},
	}, nil
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
	return &ast.BinaryExpr{
		X:  &ast.CallExpr{Fun: &ast.SelectorExpr{X: s, Sel: ident("Length")}},
		Op: token.GTR,
		Y:  &ast.BasicLit{Kind: token.INT, Value: "0"},
	}, nil
}
