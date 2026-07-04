package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers an array or typed-array index expression to a native Go int
// when the checker proved the index is an integer. A JavaScript index is a
// Number, so the ordinary lowering passes a float64 to At/SetAt, which truncate
// it back to an int and fold a NaN on every access. A loop counter never leaves
// the int32 range and never holds a NaN, so paying that per-access conversion is
// pure overhead. When the index is driven by an int32-specialized local, the
// access lowers through the AtI/SetAtI form that takes the index already narrowed,
// and the counter itself stays a native int through int32Of. The bounds check and
// the out-of-range behavior are unchanged, so the sped-up access reads the same
// element and drops the same out-of-range write the float form does; only the
// index conversion is removed. Eliding the bounds check itself needs the proof the
// index stays inside the array, which is a later slice (05 §M5); this optimization
// keeps the check and wins only the index conversion, which is always sound.

// intLoopIndex reports whether an index expression should lower to the native-int
// AtI/SetAtI form. It must be int32-producing, so int32Of yields a native int
// expression rather than a float coercion, and it must mention an int32-specialized
// local, which restricts the rewrite to the loop-counter case the optimization
// targets. A constant index like a[0] is int32-producing but gains nothing from the
// int form, so it keeps the float At and its golden is left untouched.
func (r *Renderer) intLoopIndex(n frontend.Node) bool {
	return r.int32Producing(n) && r.mentionsInt32Local(n)
}

// mentionsInt32Local reports whether an expression reads an int32-specialized
// local anywhere in its tree, the test that tells a counter-driven index apart
// from a pure-literal one.
func (r *Renderer) mentionsInt32Local(n frontend.Node) bool {
	if n.Kind() == frontend.NodeIdentifier {
		name, ok := localName(r.prog.Text(n))
		return ok && r.int32Locals[name]
	}
	for _, c := range r.prog.Children(n) {
		if r.mentionsInt32Local(c) {
			return true
		}
	}
	return false
}

// intIndexExpr lowers an index expression to a Go int. It takes the int32 lowering
// of the index, which keeps a counter and its arithmetic in native integer
// registers, and converts it to int for the slice index the AtI/SetAtI methods
// take. The caller has already checked intLoopIndex, so int32Of stays on its
// native path rather than falling back to a value.ToInt32 coercion.
func (r *Renderer) intIndexExpr(n frontend.Node) (ast.Expr, error) {
	x, err := r.int32Of(n)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: ident("int"), Args: []ast.Expr{x}}, nil
}
