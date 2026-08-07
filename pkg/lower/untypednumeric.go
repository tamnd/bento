package lower

import (
	"go/ast"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers an arithmetic or bitwise operator the checker gives no type at
// all. That happens for one reason: a bigint met a number, or met an any that could
// be holding either. TypeScript reports the mix as an error and hands back no type
// for the expression, because there is no single answer to give. 2n * 2n is 4n,
// 2 * 2 is 4, and 2n * 2 is a TypeError.
//
// The absence of a static type is the signal, not an obstacle. It says the operand
// kinds are only knowable at runtime, so the operator lowers to the runtime's own
// version of itself, which dispatches on the kinds it finds and yields a boxed
// value.Value. The everyday case is node's own test harness:
//
//	function platformTimeout(ms) {
//	  const multipliers = typeof ms === 'bigint' ?
//	    { two: 2n, four: 4n } : { two: 2, four: 4 };
//	  return multipliers.two * ms;
//	}
//
// where multipliers.two is number | bigint and ms is an untyped parameter.
//
// Before this, such an expression coerced both sides through ToNumber, which throws
// on a bigint, so the bigint arm of that function could not run at all; and the
// result, having no static type, could not be boxed into the dynamic slot it flowed
// into, so the whole program handed back.

// valueNumericHelper maps an operator to the runtime function that computes it over
// two boxed values. + is one of them, through value.Add, which is the same helper the
// dynamic-combine path below uses; it reaches here when neither operand is dynamic but
// the pair still disagrees, `b + pick(false)` over two number | bigint. Its coercion is
// the one that differs from the rest: it runs ToPrimitive under the default hint and
// concatenates when either side comes back a string, where every other operator here
// runs the number hint and never concatenates.
func valueNumericHelper(opText string) (string, bool) {
	switch opText {
	case "+":
		return "Add", true
	case "-":
		return "Sub", true
	case "*":
		return "Mul", true
	case "/":
		return "Div", true
	case "%":
		return "Rem", true
	case "**":
		return "Exponentiate", true
	case "&":
		return "BitAnd", true
	case "|":
		return "BitOr", true
	case "^":
		return "BitXor", true
	case "<<":
		return "ShiftLeft", true
	case ">>":
		return "ShiftRight", true
	case ">>>":
		return "UnsignedShiftRight", true
	default:
		return "", false
	}
}

// untypedNumericOp recognizes a binary expression that takes the runtime operator
// and reports the helper and the two operands. It is the single decision both the
// lowering and the already-boxed predicate ask, so the two cannot drift: whatever
// this claims lowers to a value.Value, and whatever lowers to a value.Value enters a
// dynamic slot without a second wrapping.
//
// Two conditions have to hold. The checker gives the expression no type, which is
// how it reports that the operands do not agree on a numeric kind. And at least one
// operand can be holding a bigint, which is the only way that disagreement arises;
// an expression left untyped for some other reason keeps whatever hand-back it had.
func (r *Renderer) untypedNumericOp(n frontend.Node) (helper string, left, right frontend.Node, ok bool) {
	// A compound assignment desugars to x = x <op> rhs and lowers the operator with no
	// node of its own, since the source never wrote one. The checker's type for the
	// operator is the whole decision here, so there is nothing to ask and the compound
	// form keeps whatever it lowered to before; `ms *= 2n` over a union target is its
	// own slice.
	if n == nil {
		return "", nil, nil, false
	}
	n = r.unwrapParens(n)
	if n.Kind() != frontend.NodeBinaryExpression {
		return "", nil, nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) != 3 {
		return "", nil, nil, false
	}
	opText := strings.TrimSpace(r.prog.Text(kids[1]))
	helper, ok = valueNumericHelper(opText)
	if !ok {
		return "", nil, nil, false
	}
	// The checker reports a pair that does not agree in two different ways. For every
	// operator but + it computes no type at all, there being no answer to give. For +,
	// whose result could still have been a string, error recovery answers any. Both say
	// the same thing, and claiming the any case here costs nothing: an operand that
	// really is dynamic would reach the same value.Add through the dynamic combine
	// anyway, and one that is not would otherwise hand back.
	if t := r.prog.TypeAt(n); t.Flags != 0 && (opText != "+" || t.Flags&frontend.TypeAny == 0) {
		return "", nil, nil, false
	}
	if !r.mayHoldBigInt(kids[0]) && !r.mayHoldBigInt(kids[2]) {
		return "", nil, nil, false
	}
	return helper, kids[0], kids[2], true
}

// mayHoldBigInt reports whether an operand can be a bigint when the program runs. A
// type that is a bigint obviously can, and so can a union with a bigint member,
// which is the shape that raises the whole question: number | bigint is what a
// `typeof ms === 'bigint'` ternary produces. A dynamic value can too, since an any
// or an unknown carries whatever it was handed. An operand the checker gave no type
// is itself one of these expressions, the inner * of a * b * c, which answers a
// boxed value that can be a bigint in turn.
func (r *Renderer) mayHoldBigInt(n frontend.Node) bool {
	if r.isDynamic(n) {
		return true
	}
	return r.typeMayHoldBigInt(r.prog.TypeAt(n), 0)
}

// typeMayHoldBigInt is the walk over a type and its union members. The depth guard
// is the usual one for a recursive type query: a union member can be a union in
// turn, and a self-referential alias would otherwise not terminate.
func (r *Renderer) typeMayHoldBigInt(t frontend.Type, depth int) bool {
	if t.Flags&frontend.TypeBigInt != 0 || t.Flags == 0 {
		return true
	}
	if depth > 8 {
		return false
	}
	for _, m := range r.prog.UnionMembers(t) {
		if r.typeMayHoldBigInt(m, depth+1) {
			return true
		}
	}
	return false
}

// untypedNumericBinary lowers the operator to its runtime form, boxing each operand
// so the helper can see the kind it is holding.
func (r *Renderer) untypedNumericBinary(n frontend.Node) (ast.Expr, bool, error) {
	helper, left, right, ok := r.untypedNumericOp(n)
	if !ok {
		return nil, false, nil
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
	return &ast.CallExpr{Fun: sel("value", helper), Args: []ast.Expr{l, rr}}, true, nil
}

// callOfUntypedResult reports whether a call the checker gives no type at all invokes a
// function whose Go result is a box, so the call expression already is a value.Value and
// enters a dynamic slot as itself.
//
// The shape comes out of the same source as the operator above. A function whose only
// return is an operator the checker could not type gets no return type computed either,
// and error recovery reports the function's return as any, so its Go func hands back a
// value.Value. The call site is left with no type at all, which is the one thing every
// consumer downstream reads to decide how to treat the result, so what the call yields
// has to be asked of the callee instead:
//
//	function mul(x, y) { return x * y }   // Go: func Mul(x, y value.Value) value.Value
//	const z: unknown = mul(3n, 4n)        // the checker types this initializer as nothing
//
// Both ways a Go result can be a box are read. A declared any is the everyday one; the
// boxed-signature pass's own mark covers a function whose declared return is a shape it
// rewrote to the value model.
func (r *Renderer) callOfUntypedResult(n frontend.Node) bool {
	if n.Kind() != frontend.NodeCallExpression || r.prog.TypeAt(n).Flags != 0 {
		return false
	}
	fn, ok := r.calleeFuncNode(n)
	if !ok {
		return false
	}
	if r.boxedReturnFns[fn] {
		return true
	}
	sig, ok := r.prog.SignatureAt(fn)
	return ok && r.isDynamicType(sig.Return)
}

// untypedBoxedExpr reports whether an expression the checker gives no type computes a
// value all the same, lowering to a box. It is the two shapes this file makes: the
// operator itself and a call of a function whose only return is one. A site that reads
// an absent type as "nothing to hand back" asks this to tell that case apart from a
// value the checker could not name.
func (r *Renderer) untypedBoxedExpr(n frontend.Node) bool {
	return r.isUntypedNumericBinary(n) || r.callOfUntypedResult(r.unwrapParens(n))
}

// isUntypedNumericBinary reports whether an expression's lowering is already a
// value.Value because it took the runtime operator, so boxing it into a dynamic slot
// is the identity. Without this the box would look at the checker's type, find none,
// and hand the program back on an expression that is already exactly what the slot
// wants.
func (r *Renderer) isUntypedNumericBinary(n frontend.Node) bool {
	_, _, _, ok := r.untypedNumericOp(n)
	return ok
}
