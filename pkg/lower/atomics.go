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

// isInt32Array reports whether a node is an Int32Array, the waitable integer array
// Atomics.wait and Atomics.notify run over in the covered subset. The spec allows a
// BigInt64Array too, but that stores a *big.Int outside the float AtomicView, so it
// hands back.
func (r *Renderer) isInt32Array(n frontend.Node) bool {
	name, ok := r.typedArrayName(r.prog.TypeAt(n))
	return ok && name == "Int32Array"
}

// atomicsCall lowers a call on the global Atomics namespace to the matching value
// package function (25 §25.4). Most operations take an integer typed array, an index,
// and value operands; isLockFree takes a byte size and pause takes an optional
// iteration count, neither carrying a typed array. The Atomics receiver is a namespace,
// not a value, so it is not lowered: it selects the function here. A form outside the
// covered subset hands back with a reason naming it.
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
	case "compareExchange":
		return r.atomicViewOp("AtomicCompareExchange", 4, method, argNodes)
	case "isLockFree":
		return r.atomicIsLockFree(argNodes)
	case "notify":
		return r.atomicWaitNotify("AtomicNotify", method, 2, 3, argNodes)
	case "wait":
		return r.atomicWaitNotify("AtomicWait", method, 3, 4, argNodes)
	case "pause":
		return r.atomicPause(argNodes)
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

// atomicIsLockFree lowers Atomics.isLockFree(size) to value.AtomicIsLockFree, the one
// Atomics operation that takes no typed array: a byte size returning a boolean. Exactly
// one number argument is covered; anything else hands back.
func (r *Renderer) atomicIsLockFree(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Atomics.isLockFree with this argument count is a later slice"}
	}
	if !r.isNumber(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Atomics.isLockFree with a non-number size is a later slice"}
	}
	size, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "AtomicIsLockFree"), Args: []ast.Expr{size}}, nil
}

// atomicWaitNotify lowers Atomics.wait and Atomics.notify to the named value function.
// Both run over an Int32Array, the waitable integer array the float AtomicView covers,
// take an index, and carry a trailing optional argument (a timeout for wait, a count
// for notify) that lowers to the variadic Go parameter. In a single agent wait cannot
// block to an "ok", since no second agent sends a notify, and notify wakes zero, since
// there is never a waiter; the value functions carry that single-agent semantics. A
// receiver that is not an Int32Array, a wrong argument count, or a non-number argument
// hands back.
func (r *Renderer) atomicWaitNotify(goName, method string, minArity, maxArity int, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) < minArity || len(argNodes) > maxArity {
		return nil, &NotYetLowerable{Reason: "Atomics." + method + " with this argument count is a later slice"}
	}
	if !r.isInt32Array(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Atomics." + method + " over a receiver that is not an Int32Array is a later slice"}
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

// atomicPause lowers Atomics.pause to value.AtomicPause, a spin-wait hint that takes an
// optional iteration count and returns undefined. No argument or a single number
// argument is covered; the number lowers to the variadic Go parameter. Anything else
// hands back.
func (r *Renderer) atomicPause(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) > 1 {
		return nil, &NotYetLowerable{Reason: "Atomics.pause takes at most an iteration count"}
	}
	var args []ast.Expr
	if len(argNodes) == 1 {
		if !r.isNumber(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Atomics.pause with a non-number iteration count is a later slice"}
		}
		lowered, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "AtomicPause"), Args: args}, nil
}
