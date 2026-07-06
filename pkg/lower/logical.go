package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the value-returning forms of && and || (05_type_lowering, the
// boolean section: "keep && and || value-returning"). On two booleans the result is
// a boolean and Go's own && and || carry it, evaluating and short-circuiting the
// same way, so those stay on the operator table. But JavaScript's && and || return
// an operand, not a boolean: a || b is a when a is truthy and b otherwise, and
// a && b is a when a is falsy and b otherwise. On two numbers or two strings that
// operand is the value itself, not a bool Go could hand back, so the expression
// lowers to the temporary-with-an-if the doc names, spelled as the immediately
// invoked function the ternary keeps in expression position.
//
// The left operand decides the result and is also the value returned when it
// short-circuits, so it appears in both the truthiness test and the returned
// value. This slice takes the form only when the left operand is repeatable, a
// value with no side effect that reads the same each time, so naming it in both
// places is sound without a temporary; a left operand with a side effect needs the
// temporary a later slice hoists and hands back. Both operands must widen to the
// same primitive, since the result is one Go type; a mixed or non-primitive pair
// hands back for the tagged union the ternary defers the same way.

// valueLogical lowers a value-returning && or || to the if a person writes, wrapped
// in a func so it stands where a value is wanted. It reports handled=false for the
// two-boolean case, so that keeps the Go operator, and for an operator that is not
// && or ||. A same-primitive pair whose left has a side effect hands back with a
// reason; a mixed or non-primitive pair reports handled=false and hands back
// through the operator table.
func (r *Renderer) valueLogical(opText string, left, right frontend.Node) (ast.Expr, bool, error) {
	if opText != "&&" && opText != "||" {
		return nil, false, nil
	}
	// A left operand the checker proved always truthy or always falsy fixes which
	// operand the result is, so the expression collapses to that operand with no test
	// and no func: obj || x is obj and obj && x is x when obj is a non-null object.
	// || yields the left when it is truthy and && when it is falsy, so the left is
	// the result exactly when (op is ||) matches the left's static truthiness; the
	// other operand is dropped, which is the short-circuit, so it need not lower. The
	// left is dropped in the other case, sound only for a side-effect-free operand.
	if v, known := r.staticTruthy(left); known && r.repeatableOperand(left) {
		chosen := right
		if (opText == "||") == v {
			chosen = left
		}
		expr, err := r.lowerExpr(chosen)
		if err != nil {
			return nil, false, err
		}
		return expr, true, nil
	}
	// Two booleans give a boolean result, which Go's && and || return directly with
	// the same evaluation order and short-circuit, so that stays on the operator
	// table rather than growing a func around a value it already has.
	if r.isBool(left) && r.isBool(right) {
		return nil, false, nil
	}
	// A dynamic operand makes the result dynamic: which operand comes back is a
	// runtime truthiness question, so both sides box and the value.Or or value.And
	// helper picks one. The helper takes both operands already evaluated, so the
	// right side must be effect-free for the evaluation the short-circuit skips to
	// be unobservable; the left evaluates exactly once either way. The common
	// shape this serves is a default over a maybe-missing dynamic, message || "".
	if r.isDynamic(left) || r.isDynamic(right) {
		if !r.repeatableOperand(right) {
			return nil, false, &NotYetLowerable{Reason: "value-returning " + opText + " on a dynamic operand whose right side has a side effect needs a lazy form, a later slice"}
		}
		leftBoxed, err := r.boxOperand(left)
		if err != nil {
			return nil, false, err
		}
		rightBoxed, err := r.boxOperand(right)
		if err != nil {
			return nil, false, err
		}
		helper := "Or"
		if opText == "&&" {
			helper = "And"
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", helper), Args: []ast.Expr{leftBoxed, rightBoxed}}, true, nil
	}
	retType, kind, ok := r.condBranchType(left)
	_, otherKind, otherOK := r.condBranchType(right)
	if !ok || !otherOK || kind != otherKind {
		return nil, false, nil
	}
	if !r.repeatableOperand(left) {
		return nil, false, &NotYetLowerable{Reason: "value-returning " + opText + " whose left operand has a side effect needs a temporary, a later slice"}
	}
	guardOperand, err := r.lowerExpr(left)
	if err != nil {
		return nil, false, err
	}
	guard := truthyOfKind(guardOperand, kind)
	// || returns the left operand when it is truthy, && when it is falsy, so && tests
	// the negated truthiness. The returned value is the left operand lowered again,
	// sound because a repeatable operand reads the same both times.
	if opText == "&&" {
		guard = &ast.UnaryExpr{Op: token.NOT, X: guard}
	}
	whenShort, err := r.lowerExpr(left)
	if err != nil {
		return nil, false, err
	}
	whenLong, err := r.lowerExpr(right)
	if err != nil {
		return nil, false, err
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: retType}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.IfStmt{
				Cond: guard,
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{whenShort}}}},
			},
			&ast.ReturnStmt{Results: []ast.Expr{whenLong}},
		}},
	}
	return &ast.CallExpr{Fun: lit}, true, nil
}

// repeatableOperand reports whether an operand reads the same value every time with
// no side effect, so it can be named more than once without a temporary. A pure
// constructor value (an identifier, a literal, or an operator over repeatable
// operands) qualifies, and so does a property or element read, which in this typed
// subset is a struct field or an array index with no getter to fire, so reading it
// twice is the read once repeated.
func (r *Renderer) repeatableOperand(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodePropertyAccessExpression:
		kids := r.prog.Children(n)
		return len(kids) == 2 && r.repeatableOperand(kids[0])
	case frontend.NodeElementAccessExpression:
		kids := r.prog.Children(n)
		return len(kids) == 2 && r.repeatableOperand(kids[0]) && r.repeatableOperand(kids[1])
	}
	return r.pureCtorValue(n)
}
