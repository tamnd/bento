package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// integerAtomicReceiver reports whether a node is a typed array Atomics may run over
// through bento's float-based AtomicView: the integer numeric family Int8Array,
// Uint8Array, Int16Array, Uint16Array, Int32Array, and Uint32Array. A bigint-element
// array is a valid Atomics receiver in the language but stores a *big.Int rather than a
// Number, so it does not fit the float64 read-modify-write the covered set shares and
// hands back. Uint8ClampedArray and the float arrays are not Atomics receivers at all,
// so the checker rules them out before lowering.
func (r *Renderer) integerAtomicReceiver(n frontend.Node) bool {
	name, ok := r.typedArrayName(r.prog.TypeAt(n))
	if !ok {
		return false
	}
	switch name {
	case "Int8Array", "Uint8Array", "Int16Array", "Uint16Array", "Int32Array", "Uint32Array":
		return true
	default:
		return false
	}
}

// atomicsCall lowers a call on the global Atomics namespace to the matching value
// package function (25 §25.4). The read, write, and read-modify-write operations each
// take an integer typed array, an index, and value operands. The Atomics receiver is a
// namespace, not a value, so it is not lowered: it selects the function here. A form
// outside the covered subset hands back with a reason naming it.
func (r *Renderer) atomicsCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "load":
		return r.atomicViewOp("AtomicLoad", 2, method, argNodes)
	case "store":
		return r.atomicViewOp("AtomicStore", 3, method, argNodes)
	case "add":
		return r.atomicViewOp("AtomicAdd", 3, method, argNodes)
	case "sub":
		return r.atomicViewOp("AtomicSub", 3, method, argNodes)
	case "and":
		return r.atomicViewOp("AtomicAnd", 3, method, argNodes)
	case "or":
		return r.atomicViewOp("AtomicOr", 3, method, argNodes)
	case "xor":
		return r.atomicViewOp("AtomicXor", 3, method, argNodes)
	case "exchange":
		return r.atomicViewOp("AtomicExchange", 3, method, argNodes)
	default:
		return nil, &NotYetLowerable{Reason: "Atomics." + method + " is a later slice"}
	}
}

// atomicViewOp lowers an Atomics operation over an integer typed array to the named
// value function: the receiver typed array followed by an index and, past load, the
// value operands. The argument count must match the operation's arity exactly, the
// receiver must be an integer typed array the float AtomicView covers, and every
// argument past the receiver must type as a Number; anything else hands back rather
// than emit a mistyped call.
func (r *Renderer) atomicViewOp(goName string, arity int, method string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != arity {
		return nil, &NotYetLowerable{Reason: "Atomics." + method + " with this argument count is a later slice"}
	}
	if !r.integerAtomicReceiver(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Atomics." + method + " over a receiver that is not an integer typed array is a later slice"}
	}
	view, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	args := []ast.Expr{view}
	for _, n := range argNodes[1:] {
		if !r.isNumber(n) {
			return nil, &NotYetLowerable{Reason: "Atomics." + method + " with a non-number argument is a later slice"}
		}
		lowered, err := r.lowerExpr(n)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", goName), Args: args}, nil
}
