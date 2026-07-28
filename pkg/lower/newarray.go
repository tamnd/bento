package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers `new Array<T>(...)`, the array constructor called with new.
//
// It is the same array an array literal builds, a *value.Array[T], so the three
// forms it can take all lower to a value constructor: no argument is the empty
// array, a single numeric argument is a length (the preallocation a benchmark
// or a table writes before filling it by index), and any other argument list is
// the elements, the constructor's second signature. That split is JavaScript's
// own, not a heuristic: `new Array(5)` is five slots and `new Array(5, 6)` is
// two elements.

// newArray lowers a new Array expression. n is the whole new expression, whose
// checker type carries the element type, and args are the constructor's
// arguments. A call whose element type does not lower hands back, the same way
// an array literal of that element type does.
func (r *Renderer) newArray(n frontend.Node, args []frontend.Node) (ast.Expr, error) {
	elemType, ok := r.arrayElem(n)
	if !ok {
		return nil, &NotYetLowerable{Reason: "new Array whose element type does not lower yet"}
	}
	// A written type argument, the <number> of new Array<number>(4), is a child of
	// the new expression ahead of the value arguments, so it is dropped here rather
	// than lowered as if it were one. The element type comes from the checker either
	// way, so the annotation itself has nothing left to say.
	args = r.namedArgs(args)
	r.requireImport(valuePkg)

	// new Array(n) preallocates n slots. The single-argument form is a length only
	// when the argument is a number; `new Array<string>("a")` is a one-element
	// array, so the argument's own type, not the argument count, picks the form.
	if len(args) == 1 && r.isNumber(args[0]) {
		length, err := r.lowerExpr(args[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: index(sel("value", "NewArrayLen"), elemType), Args: []ast.Expr{length}}, nil
	}

	elems := make([]ast.Expr, 0, len(args))
	for _, a := range args {
		e, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		elems = append(elems, e)
	}
	elems, err := r.wrapArrayElems(elems, args, n)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: index(sel("value", "NewArray"), elemType), Args: elems}, nil
}
