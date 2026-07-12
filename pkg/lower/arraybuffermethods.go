package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// arrayBufferMethodCall lowers a method call whose receiver is an ArrayBuffer to a
// value.ArrayBuffer method (25 §25.1). It runs after the view paths in methodCall,
// since a buffer is the backing store a view aliases rather than a view itself, and
// covers the transfer pair here: the resize surface is a later slice under the same
// group. A method the surface does not carry hands back with a reason naming it, so
// it reaches a clear handback rather than the generic receiver error.
//
// Ceiling: transfer and transferToFixedLength share one runtime body and so lower
// the same way, because the resizable-versus-fixed distinction transferToFixedLength
// draws has no observable effect until the resizable buffer lands. Once it does, the
// fixed-length variant must pin its result non-resizable while transfer may carry the
// source's maxByteLength, and the two lowerings split.
func (r *Renderer) arrayBufferMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "transfer":
		return r.arrayBufferTransfer(recvNode, "Transfer", argNodes)
	case "transferToFixedLength":
		return r.arrayBufferTransfer(recvNode, "TransferToFixedLength", argNodes)
	default:
		return nil, &NotYetLowerable{Reason: "ArrayBuffer method ." + method + " is a later slice"}
	}
}

// arrayBufferTransfer lowers ArrayBuffer.prototype.transfer and its fixed-length
// sibling to the named runtime method. The new byte length is an optional Number, so
// it lowers to the variadic argument the runtime method takes: none for the default,
// which keeps the receiver's current length, and the single lowered number when the
// call gives one. A non-number length is a later slice, held back rather than coerced
// here so the covered subset stays the one the number path proves.
func (r *Renderer) arrayBufferTransfer(recvNode frontend.Node, runtimeName string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) > 1 {
		return nil, &NotYetLowerable{Reason: "ArrayBuffer transfer takes at most one length argument"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	var args []ast.Expr
	if len(argNodes) == 1 {
		if !r.isNumber(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "ArrayBuffer transfer with a non-number length is a later slice"}
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(runtimeName)}, Args: args}, nil
}
