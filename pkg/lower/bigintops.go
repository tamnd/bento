package lower

import (
	"go/ast"
	"go/token"
	"math"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file holds the bigint lowerings that sit beside the operator mapping in
// funcgen.go: the in-place update that bigOwnedLocalsOf (bigown.go) makes sound,
// and the BigInt(x) conversion. Both exist so the emitted Go reads like a person
// wrote it: an accumulator loop mutates its own big.Int the way hand-written
// math/big code does, and a conversion is one named runtime call.

// bigIntInPlaceAssign lowers a self-referential update of an owned bigint local
// to the in-place *big.Int form: acc *= i and acc = acc * i both emit
// acc.Mul(acc, i), which stores the product into acc's own backing array with no
// per-iteration allocation. It reports ok=false when the statement is not that
// shape, so lowerUpdate falls through to lowerAssign and its always-fresh form:
// the target must be a bigint local bigOwnedLocalsOf proved unshared, and the
// operator must be one with a *big.Int method (the arithmetic and bitwise
// family); ** and the shifts go through value helpers that return fresh values
// and keep the ordinary assignment. big.Int methods accept a receiver that
// aliases either operand, so acc = acc * acc emits acc.Mul(acc, acc) unchanged.
func (r *Renderer) bigIntInPlaceAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	if len(r.bigOwned) == 0 {
		return nil, false, nil
	}
	parts := r.prog.Children(bin)
	if len(parts) != 3 {
		return nil, false, nil
	}
	target := parts[0]
	if target.Kind() != frontend.NodeIdentifier || !r.isBigInt(target) {
		return nil, false, nil
	}
	name, ok := localName(r.prog.Text(target))
	if !ok || !r.bigOwned[name] {
		return nil, false, nil
	}

	opText := r.prog.Text(parts[1])
	var method string
	var leftNode, rightNode frontend.Node
	if baseOp, compound := compoundBaseOp(opText); compound {
		// acc op= x is acc = acc op x, so the receiver is also the left operand.
		method, ok = bigIntArithMethod(baseOp)
		if !ok || !r.isBigInt(parts[2]) {
			return nil, false, nil
		}
		leftNode, rightNode = target, parts[2]
	} else if opText == "=" {
		// acc = a op b takes the in-place form when acc is one of the operands, the
		// self-reference that makes the old value dead at the store.
		method, leftNode, rightNode, ok = r.bigIntSelfOp(name, parts[2])
		if !ok {
			return nil, false, nil
		}
	} else {
		return nil, false, nil
	}

	l, err := r.lowerExpr(leftNode)
	if err != nil {
		return nil, false, err
	}
	rr, err := r.lowerExpr(rightNode)
	if err != nil {
		return nil, false, err
	}
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ident(name), Sel: ident(method)},
		Args: []ast.Expr{l, rr},
	}}, true, nil
}

// bigIntSelfOp recognizes a right-hand side of the form name op x or x op name
// where op has a *big.Int method, the self-referential shape a plain assignment
// must have to mutate name in place. It returns the method and the two operand
// nodes in call order.
func (r *Renderer) bigIntSelfOp(name string, rhs frontend.Node) (method string, left, right frontend.Node, ok bool) {
	if rhs.Kind() != frontend.NodeBinaryExpression {
		return
	}
	parts := r.prog.Children(rhs)
	if len(parts) != 3 || !r.isBigInt(parts[0]) || !r.isBigInt(parts[2]) {
		return
	}
	method, ok = bigIntArithMethod(r.prog.Text(parts[1]))
	if !ok {
		return
	}
	if !r.bigIdentNamed(parts[0], name) && !r.bigIdentNamed(parts[2], name) {
		return "", left, right, false
	}
	return method, parts[0], parts[2], true
}

// foldBigIntNumber reads a BigInt(x) argument that is an integer numeric literal
// in the int64 range, the shape that folds to big.NewInt at compile time. A
// float64 holds every integer up to 2^53 exactly and larger literals only in the
// spaced-out representable values, so the fold takes the float64 the literal
// denotes, exactly what the runtime conversion would see. The int64 bound keeps
// the emitted form the one-call big.NewInt; a wider integral literal stays a
// runtime conversion rather than intern a shared package var, whose read would
// not be a fresh value.
func (r *Renderer) foldBigIntNumber(arg frontend.Node) (int64, bool) {
	if arg.Kind() != frontend.NodeNumericLiteral {
		return 0, false
	}
	f, ok := numericLiteralValue(r.prog.Text(arg))
	if !ok || f != math.Trunc(f) {
		return 0, false
	}
	// The exact int64 window in float64 terms: -2^63 is representable, 2^63 is the
	// first value past MaxInt64, so the safe test is f >= -2^63 && f < 2^63.
	if f < -math.Ldexp(1, 63) || f >= math.Ldexp(1, 63) {
		return 0, false
	}
	return int64(f), true
}

// bigIdentNamed reports whether a node is an identifier for the given Go local
// name, the self-reference test of the in-place forms.
func (r *Renderer) bigIdentNamed(n frontend.Node, name string) bool {
	if n.Kind() != frontend.NodeIdentifier {
		return false
	}
	got, ok := localName(r.prog.Text(n))
	return ok && got == name
}

// bigIntCoercion lowers BigInt(x) called as a function over a primitive argument,
// the fourth primitive coercion beside String, Number, and Boolean. A bigint
// passes through unchanged, the identity BigInt(b) is. A number goes through
// value.NumberToBigInt, which throws the RangeError JavaScript raises on a
// fractional, NaN, or infinite argument; a string through value.StringToBigInt,
// the ECMAScript StringToBigInt grammar that throws a SyntaxError on anything
// that is not an integer literal; a boolean through value.BoolToBigInt. The two
// throwing paths defer the uncaught reporter. It takes exactly one argument; a
// different arity, or an argument this slice does not convert, hands back.
func (r *Renderer) bigIntCoercion(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "BigInt() with this argument count is a later slice"}
	}
	arg := argNodes[0]
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	switch {
	case r.isBigInt(arg):
		return lowered, nil // BigInt(b) on a bigint is the identity
	case r.isNumber(arg):
		// BigInt(42) on an integer literal is the literal 42n spelled through the
		// constructor, so it folds to the same big.NewInt at compile time instead of
		// a runtime conversion that can never throw. Only an int64-range fold is
		// taken: it emits the fresh one-call form, which keeps bigExprIsFresh's
		// answer for a BigInt(number) truthful. A fractional literal is left to the
		// runtime helper, whose RangeError is the behavior.
		if v, ok := r.foldBigIntNumber(arg); ok {
			r.requireImport("math/big")
			return &ast.CallExpr{
				Fun:  sel("big", "NewInt"),
				Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.FormatInt(v, 10)}},
			}, nil
		}
		r.usesThrow = true
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "NumberToBigInt"), Args: []ast.Expr{lowered}}, nil
	case r.isString(arg):
		r.usesThrow = true
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "StringToBigInt"), Args: []ast.Expr{lowered}}, nil
	case r.isBool(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "BoolToBigInt"), Args: []ast.Expr{lowered}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "BigInt() on this argument type is a later slice"}
	}
}
