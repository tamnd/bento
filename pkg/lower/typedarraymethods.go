package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// typedArrayMethodCall lowers a method call on a numeric typed-array receiver to a
// value.TypedArray method. It is the typed-array sibling of arrayMethodCall: the
// two share the copy and search shapes, but a typed array runs over a view that
// clamps to its length rather than over a growable array, and every element is a
// Number, so the search and join methods take a float64 target and NumberToString
// directly rather than the element-type closures the Array methods thread in. The
// methods that grow or shrink an array (push, pop, splice) have no typed-array
// analogue and never reach here, since the checker rejects them on a typed array.
// A method outside the covered set hands back so the engine runs it rather than
// emitting a call to a method the view does not have.
func (r *Renderer) typedArrayMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "fill":
		if len(argNodes) < 1 || len(argNodes) > 3 {
			return nil, &NotYetLowerable{Reason: "typed array fill takes a value and up to two bounds"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		if !r.isNumber(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "typed array fill with a non-number value is a later slice"}
		}
		val, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		args := []ast.Expr{val}
		for _, b := range argNodes[1:] {
			if !r.isNumber(b) {
				return nil, &NotYetLowerable{Reason: "typed array fill with a non-number bound is a later slice"}
			}
			lowered, err := r.lowerExpr(b)
			if err != nil {
				return nil, err
			}
			args = append(args, lowered)
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Fill")}, Args: args}, nil
	case "slice":
		return r.typedArrayRangeMethod(recvNode, "Slice", argNodes)
	case "subarray":
		return r.typedArrayRangeMethod(recvNode, "Subarray", argNodes)
	case "copyWithin":
		if len(argNodes) < 1 || len(argNodes) > 3 {
			return nil, &NotYetLowerable{Reason: "typed array copyWithin takes a target and up to two bounds"}
		}
		return r.typedArrayRangeMethod(recvNode, "CopyWithin", argNodes)
	case "at":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "typed array at takes exactly one argument"}
		}
		if !r.isNumber(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "typed array at with a non-number index is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("AtOpt")}, Args: []ast.Expr{arg}}, nil
	case "indexOf":
		return r.typedArraySearch(recvNode, "IndexOf", argNodes)
	case "lastIndexOf":
		return r.typedArraySearch(recvNode, "LastIndexOf", argNodes)
	case "includes":
		return r.typedArraySearch(recvNode, "Includes", argNodes)
	case "join":
		return r.typedArrayJoin(recvNode, argNodes)
	case "set":
		return r.typedArraySet(recvNode, argNodes)
	default:
		return nil, &NotYetLowerable{Reason: "typed array method ." + method + " is a later slice"}
	}
}

// typedArrayRangeMethod lowers slice, subarray, and copyWithin, which share the
// bound shape: zero to three Number bounds that go straight through to the value
// method once each is a number. slice and subarray both take zero to two bounds
// and copyWithin one to three, but the caller has already checked its own count,
// so this only verifies each bound is a Number and lowers it. The value method
// clamps the bounds against the view's length, so no range guard is emitted here.
func (r *Renderer) typedArrayRangeMethod(recvNode frontend.Node, goMethod string, argNodes []frontend.Node) (ast.Expr, error) {
	if goMethod != "CopyWithin" && len(argNodes) > 2 {
		return nil, &NotYetLowerable{Reason: "typed array " + goMethod + " takes at most two bounds"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for _, b := range argNodes {
		if !r.isNumber(b) {
			return nil, &NotYetLowerable{Reason: "typed array " + goMethod + " with a non-number bound is a later slice"}
		}
		lowered, err := r.lowerExpr(b)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goMethod)}, Args: args}, nil
}

// typedArraySearch lowers indexOf, lastIndexOf, and includes over a typed array.
// The target is a Number the value method compares directly, so unlike the Array
// search methods no element-equality closure is threaded in: a typed array's
// element is always a Number, whose strict equality is Go == and whose
// SameValueZero the Includes method folds in itself. Only the one-argument form is
// covered; a fromIndex is a later slice.
func (r *Renderer) typedArraySearch(recvNode frontend.Node, goMethod string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "typed array " + goMethod + " with a fromIndex argument is a later slice"}
	}
	if !r.isNumber(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "typed array " + goMethod + " with a non-number target is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	target, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goMethod)}, Args: []ast.Expr{target}}, nil
}

// typedArrayJoin lowers a join call to the value.TypedArray Join method. The
// separator is the lowered string argument, or the JavaScript default comma when
// the call has none. Unlike the Array join, no per-element stringify closure is
// passed: a typed array's element is always a Number, so the value method uses
// NumberToString directly. A non-string separator or more than one argument hands
// back.
func (r *Renderer) typedArrayJoin(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) > 1 {
		return nil, &NotYetLowerable{Reason: "typed array join with more than one argument is not valid"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	var sep ast.Expr
	if len(argNodes) == 1 {
		if !r.isString(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "typed array join with a non-string separator is a later slice"}
		}
		sep, err = r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
	} else {
		r.requireImport(valuePkg)
		sep = &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `","`}}}
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Join")}, Args: []ast.Expr{sep}}, nil
}

// typedArraySet lowers a set call to the value.TypedArray Set method. The source
// is read into a []float64 snapshot before the write so an overlapping set from
// another view of the same buffer is correct: a typed-array source lowers through
// its Floats method and a plain number-array source through its Elems slice. The
// optional offset is a Number, defaulting to zero. A source that is neither a
// numeric typed array nor a number array, or a non-number offset, hands back.
func (r *Renderer) typedArraySet(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) < 1 || len(argNodes) > 2 {
		return nil, &NotYetLowerable{Reason: "typed array set takes a source and an optional offset"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	srcExpr, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	var src ast.Expr
	switch {
	case r.numericTypedArray(argNodes[0]):
		src = &ast.CallExpr{Fun: &ast.SelectorExpr{X: srcExpr, Sel: ident("Floats")}}
	case r.isNumberArray(argNodes[0]):
		src = &ast.CallExpr{Fun: &ast.SelectorExpr{X: srcExpr, Sel: ident("Elems")}}
	default:
		return nil, &NotYetLowerable{Reason: "typed array set from a source that is not a numeric typed array or a number array is a later slice"}
	}
	var offset ast.Expr
	if len(argNodes) == 2 {
		if !r.isNumber(argNodes[1]) {
			return nil, &NotYetLowerable{Reason: "typed array set with a non-number offset is a later slice"}
		}
		offset, err = r.lowerExpr(argNodes[1])
		if err != nil {
			return nil, err
		}
	} else {
		offset = &ast.BasicLit{Kind: token.INT, Value: "0"}
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Set")}, Args: []ast.Expr{src, offset}}, nil
}

// genericTypedArray reports whether a node's type is a numeric typed array that
// lowers to the generic *value.TypedArray[T] view, the receiver set the method
// surface here runs over. It is numericTypedArray minus Uint8Array: Uint8Array has
// its own []byte representation without these view methods, so a method call on one
// hands back rather than emitting a call to a method the byte buffer does not have.
// typedArrayElemGo excludes Uint8Array for the same reason, so it is the exact test.
func (r *Renderer) genericTypedArray(n frontend.Node) bool {
	name, ok := r.typedArrayName(r.prog.TypeAt(n))
	if !ok {
		return false
	}
	_, ok = typedArrayElemGo(name)
	return ok
}

// isNumberArray reports whether a node's type is an array whose element lowers to
// the Go float64 a Number takes, the source shape typedArraySet copies from
// through the array's Elems slice. It is the array counterpart of the typed-array
// source check: a number array's Elems is a []float64 the Set method reads
// directly.
func (r *Renderer) isNumberArray(n frontend.Node) bool {
	elem, ok := r.prog.ElementType(r.prog.TypeAt(n))
	if !ok {
		return false
	}
	return elem.Flags&frontend.TypeNumber != 0
}
