package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers binary operators over dynamic operands, the expressions an
// any-typed value reaches that + alone did not cover. Strict equality goes
// through the runtime's StrictEquals, since the operand kinds are only known at
// runtime; a compare against the undefined or null literal collapses to the
// box's own presence predicate, which is both faster and the form a person
// would write. The multiplicative operators coerce each side with ToNumber and
// stay native float64 arithmetic, the same shape the checker's number result
// promises. test262's assert.js is the canonical consumer: mustBeTrue === true,
// message === undefined, a !== a, and the 1 / a distinguishing signed zeros.

// dynamicBinary lowers a binary operator where either operand is dynamic. It
// reports handled=false when neither side is dynamic or the operator has no
// dynamic lowering yet, so those keep their hand-back through the operator table.
func (r *Renderer) dynamicBinary(opText string, left, right frontend.Node) (ast.Expr, bool, error) {
	if !r.isDynamic(left) && !r.isDynamic(right) {
		return nil, false, nil
	}
	switch opText {
	case "==", "!=":
		// Loose equality over a dynamic operand runs the Abstract Equality
		// Comparison, which coerces across kinds before it compares: 1 == "1" is
		// true and null == undefined is true. The value model spells that as
		// value.LooseEquals, so a == b lowers to the call and a != b negates it, the
		// coercing sibling of the StrictEquals path below.
		l, err := r.boxOperand(left)
		if err != nil {
			return nil, false, err
		}
		rr, err := r.boxOperand(right)
		if err != nil {
			return nil, false, err
		}
		r.requireImport(valuePkg)
		eq := &ast.CallExpr{Fun: sel("value", "LooseEquals"), Args: []ast.Expr{l, rr}}
		if opText == "!=" {
			return &ast.UnaryExpr{Op: token.NOT, X: eq}, true, nil
		}
		return eq, true, nil
	case "===", "!==":
		if expr, ok, err := r.dynamicPresenceCompare(opText, left, right); err != nil {
			return nil, false, err
		} else if ok {
			return expr, true, nil
		}
		l, err := r.boxOperand(left)
		if err != nil {
			return nil, false, err
		}
		rr, err := r.boxOperand(right)
		if err != nil {
			return nil, false, err
		}
		r.requireImport(valuePkg)
		eq := &ast.CallExpr{Fun: sel("value", "StrictEquals"), Args: []ast.Expr{l, rr}}
		if opText == "!==" {
			return &ast.UnaryExpr{Op: token.NOT, X: eq}, true, nil
		}
		return eq, true, nil
	case "-", "*", "/", "%":
		l, err := r.operandToNumber(left)
		if err != nil {
			return nil, false, err
		}
		rr, err := r.operandToNumber(right)
		if err != nil {
			return nil, false, err
		}
		if opText == "%" {
			r.requireImport("math")
			return &ast.CallExpr{Fun: sel("math", "Mod"), Args: []ast.Expr{l, rr}}, true, nil
		}
		ops := map[string]token.Token{"-": token.SUB, "*": token.MUL, "/": token.QUO}
		return &ast.BinaryExpr{X: l, Op: ops[opText], Y: rr}, true, nil
	case "**":
		// Exponentiation over a dynamic operand coerces each side with ToNumber and
		// runs value.Pow, the same helper the static number ** lowers to, so a dynamic
		// base or exponent raises to a power with the JavaScript edge cases (a NaN
		// exponent, base 1 to an infinite power) kept identical.
		l, err := r.operandToNumber(left)
		if err != nil {
			return nil, false, err
		}
		rr, err := r.operandToNumber(right)
		if err != nil {
			return nil, false, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "Pow"), Args: []ast.Expr{l, rr}}, true, nil
	case "&", "|", "^", "<<", ">>", ">>>":
		// The bitwise operators over a dynamic operand coerce each side with ToNumber
		// and then run the same ToInt32-based construction the static number path uses:
		// each operand narrows to a 32-bit integer, the Go operator runs, and the
		// result casts back to float64. A shift masks its count to five bits. This is
		// the shared bitwiseFromFloat tail reached with two ToNumber-coerced values.
		goOp, shift, unsignedLeft, _ := bitwiseOp(opText)
		l, err := r.operandToNumber(left)
		if err != nil {
			return nil, false, err
		}
		rr, err := r.operandToNumber(right)
		if err != nil {
			return nil, false, err
		}
		return r.bitwiseFromFloat(goOp, shift, unsignedLeft, l, rr), true, nil
	case "<", "<=", ">", ">=":
		// The four relational operators over a dynamic operand run the Abstract
		// Relational Comparison, which boxes each side and coerces through
		// ToPrimitive: two strings order by code unit and everything else compares
		// as numbers, with NaN making the result false. The value model spells that
		// operation as one helper per operator, so a < b lowers to value.Less(a, b)
		// and reads the way a person would write it.
		l, err := r.boxOperand(left)
		if err != nil {
			return nil, false, err
		}
		rr, err := r.boxOperand(right)
		if err != nil {
			return nil, false, err
		}
		helpers := map[string]string{"<": "Less", "<=": "LessEqual", ">": "Greater", ">=": "GreaterEqual"}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", helpers[opText]), Args: []ast.Expr{l, rr}}, true, nil
	}
	return nil, false, nil
}

// dynamicPresenceCompare recognizes a strict equality between a dynamic operand
// and the undefined or null literal and lowers it to the box's IsUndefined or
// IsNull predicate: the literal has no Go value to box, and the predicate is
// the one-tag test the comparison means. The undefined side is skipped rather
// than lowered; only the dynamic operand emits.
func (r *Renderer) dynamicPresenceCompare(opText string, left, right frontend.Node) (ast.Expr, bool, error) {
	pred := ""
	var operand frontend.Node
	for _, pair := range [2][2]frontend.Node{{left, right}, {right, left}} {
		dyn, lit := pair[0], pair[1]
		if !r.isDynamic(dyn) {
			continue
		}
		switch r.prog.TypeAt(lit).Flags {
		case frontend.TypeUndefined:
			pred = "IsUndefined"
		case frontend.TypeNull:
			pred = "IsNull"
		default:
			continue
		}
		operand = dyn
		break
	}
	if pred == "" {
		return nil, false, nil
	}
	lowered, err := r.lowerExpr(operand)
	if err != nil {
		return nil, false, err
	}
	check := &ast.CallExpr{Fun: &ast.SelectorExpr{X: lowered, Sel: ident(pred)}}
	if opText == "!==" {
		return &ast.UnaryExpr{Op: token.NOT, X: check}, true, nil
	}
	return check, true, nil
}

// operandToNumber lowers an operand of a numeric operator to its float64: a
// number is already one, and anything else boxes and coerces through ToNumber,
// the same conversion the language runs on a non-number reaching arithmetic. A
// dynamic bigint reaching this path throws the runtime's TypeError; the
// bigint-preserving dynamic arithmetic is a later slice.
func (r *Renderer) operandToNumber(n frontend.Node) (ast.Expr, error) {
	if r.isNumber(n) {
		return r.lowerExpr(n)
	}
	boxed, err := r.boxOperand(n)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "ToNumber"), Args: []ast.Expr{boxed}}, nil
}
