package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// newSharedArrayBuffer lowers a SharedArrayBuffer construction, the shared backing
// store of 25 §25.2. new SharedArrayBuffer(byteLength) lowers to
// value.NewSharedArrayBuffer(n), a zeroed shared run of that many bytes. The growable
// form new SharedArrayBuffer(n, { maxByteLength: m }) lowers to
// value.NewGrowableSharedArrayBuffer(n, m), which records the maximum the buffer may
// grow to. Either way the byte length must be a Number, and the options argument must
// be an object literal carrying the maxByteLength property, the same shape the
// ArrayBuffer constructor takes, so any other second argument hands back.
func (r *Renderer) newSharedArrayBuffer(args []frontend.Node) (ast.Expr, error) {
	if len(args) == 0 || len(args) > 2 {
		return nil, &NotYetLowerable{Reason: "only new SharedArrayBuffer(byteLength) and new SharedArrayBuffer(byteLength, { maxByteLength }) are lowered"}
	}
	if !r.isNumber(args[0]) {
		return nil, &NotYetLowerable{Reason: "a SharedArrayBuffer byte length that is not a number is a later slice"}
	}
	length, err := r.lowerExpr(args[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	if len(args) == 1 {
		return &ast.CallExpr{Fun: sel("value", "NewSharedArrayBuffer"), Args: []ast.Expr{length}}, nil
	}
	max, err := r.arrayBufferMaxByteLength(args[1])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: sel("value", "NewGrowableSharedArrayBuffer"), Args: []ast.Expr{length, max}}, nil
}

// isViewBuffer reports whether a node is a buffer a typed array or a DataView may
// view: an ArrayBuffer or a SharedArrayBuffer. Both back a view the same way, since a
// view aliases the underlying byte run either kind holds, so the view constructors
// accept both and read the bytes through lowerViewBuffer.
func (r *Renderer) isViewBuffer(n frontend.Node) bool {
	return r.isArrayBuffer(n) || r.isSharedArrayBuffer(n)
}

// lowerViewBuffer lowers a buffer argument to the *value.ArrayBuffer a view
// constructor takes. An ArrayBuffer lowers straight through; a SharedArrayBuffer lowers
// to its .Buffer(), the underlying run its views share, so a typed array or a DataView
// over a shared buffer aliases the same bytes and observes writes made through any
// other view of it.
func (r *Renderer) lowerViewBuffer(n frontend.Node) (ast.Expr, error) {
	buf, err := r.lowerExpr(n)
	if err != nil {
		return nil, err
	}
	if r.isSharedArrayBuffer(n) {
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: buf, Sel: ident("Buffer")}}, nil
	}
	return buf, nil
}

// sharedArrayBufferMethodCall lowers a method call whose receiver is a
// SharedArrayBuffer to a value.SharedArrayBuffer method (25 §25.2.4). It covers grow,
// which enlarges the shared run within its maximum, and slice, which copies a byte span
// into a fresh shared buffer. A method the surface does not carry hands back with a
// reason naming it, so it reaches a clear handback rather than the generic receiver
// error.
func (r *Renderer) sharedArrayBufferMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "grow":
		return r.sharedArrayBufferGrow(recvNode, argNodes)
	case "slice":
		return r.sharedArrayBufferSlice(recvNode, argNodes)
	default:
		return nil, &NotYetLowerable{Reason: "SharedArrayBuffer method ." + method + " is a later slice"}
	}
}

// sharedArrayBufferGrow lowers SharedArrayBuffer.prototype.grow to the runtime Grow,
// which enlarges the backing run to the new byte length within the buffer's maximum
// (25 §25.2.4.4). The new length is a required Number, so exactly one number argument
// is covered; a different count or a non-number length hands back rather than emit a
// call the runtime method does not take. Grow returns nothing, matching the undefined
// the method yields, so the call stands as the statement it appears as.
func (r *Renderer) sharedArrayBufferGrow(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "SharedArrayBuffer grow takes exactly one length argument"}
	}
	if !r.isNumber(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "SharedArrayBuffer grow with a non-number length is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	arg, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Grow")}, Args: []ast.Expr{arg}}, nil
}

// sharedArrayBufferSlice lowers SharedArrayBuffer.prototype.slice to the runtime
// Slice, which copies the bytes in [start, end) into a fresh shared buffer (25
// §25.2.4.3). Both bounds are optional Numbers, so the call lowers to the variadic Go
// method: none for the whole buffer, one for a start with the end running to the
// current length, and two for an explicit span. A non-number bound is a later slice,
// held back rather than coerced here so the covered subset stays the one the number
// path proves.
func (r *Renderer) sharedArrayBufferSlice(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) > 2 {
		return nil, &NotYetLowerable{Reason: "SharedArrayBuffer slice takes at most a start and an end"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	var args []ast.Expr
	for _, argNode := range argNodes {
		if !r.isNumber(argNode) {
			return nil, &NotYetLowerable{Reason: "SharedArrayBuffer slice with a non-number bound is a later slice"}
		}
		arg, err := r.lowerExpr(argNode)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Slice")}, Args: args}, nil
}
