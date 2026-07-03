package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the exception surface: constructing a built-in error and
// throwing it. A throw becomes a Go panic carrying a value.Error, and the
// assembled main defers value.ReportUncaught so a throw that escapes every catch
// prints an uncaught-error line and exits non-zero rather than crashing with a Go
// stack (10_value_model.md, and 16_go_interop.md section 7.7 for the boundary
// errors that share the same reporting). Recovering a throw into a catch binding
// is a later slice; this one covers raising and reporting, the half a program
// needs before try/catch can catch anything.

// errorCtors maps a built-in error constructor name to the value constructor its
// new expression lowers to. Only the errors bento's own lowerings raise (a plain
// Error, a TypeError from a failed guard, a RangeError from a range check) are
// covered; the DOM and Node error subclasses are a later slice, and an unlisted
// name hands back so a user class named Error-something is never mistaken for a
// built-in.
var errorCtors = map[string]string{
	"Error":      "NewError",
	"TypeError":  "NewTypeError",
	"RangeError": "NewRangeError",
}

// newExpr lowers a new expression. Only the built-in error constructors are
// covered: new Error(message) and its TypeError and RangeError siblings lower to
// the matching value constructor, taking a single string message or none. A
// constructor bento does not recognize, or an error constructor called with a
// non-string or with more than one argument, hands back to the engine.
func (r *Renderer) newExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		return nil, &NotYetLowerable{Reason: "new expression did not expose a constructor"}
	}
	ctor, ok := errorCtors[r.prog.Text(kids[0])]
	if !ok {
		return nil, &NotYetLowerable{Reason: "new of a constructor other than a built-in error is a later slice"}
	}
	args := kids[1:]
	if len(args) > 1 {
		return nil, &NotYetLowerable{Reason: "an error constructed with more than a message is a later slice"}
	}
	var message ast.Expr
	if len(args) == 0 {
		// new Error() with no argument carries an empty message, the same shape as
		// new Error(""), so it lowers to an empty bento string.
		message = r.bstrLit(nil)
	} else {
		if !r.isString(args[0]) {
			return nil, &NotYetLowerable{Reason: "an error constructed from a non-string message is a later slice"}
		}
		lowered, err := r.lowerExpr(args[0])
		if err != nil {
			return nil, err
		}
		message = lowered
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", ctor), Args: []ast.Expr{message}}, nil
}

// lowerThrow lowers a throw statement to a panic carrying the thrown error, the
// raise half of the exception model. Only throwing a built-in error is covered
// (throw new Error("..."), or throwing a variable bound to one), because the
// thrown value must be a value.Error for the panic to round-trip through the
// runtime's recover; throwing a bare string or number is a later slice that boxes
// the operand. Emitting the throw records that the program can raise, so the
// assembled main defers the uncaught-error reporter.
func (r *Renderer) lowerThrow(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "throw did not expose a single operand"}
	}
	if !r.isThrowable(kids[0]) {
		return nil, &NotYetLowerable{Reason: "throwing a value that is not a built-in error is a later slice"}
	}
	operand, err := r.lowerExpr(kids[0])
	if err != nil {
		return nil, err
	}
	r.usesThrow = true
	return &ast.ExprStmt{X: &ast.CallExpr{Fun: sel("value", "Throw"), Args: []ast.Expr{operand}}}, nil
}

// isThrowable reports whether a throw operand lowers to a value.Error the runtime
// can raise and recover. A new expression for a built-in error qualifies directly;
// an identifier qualifies when the checker types it as an Error, which is how a
// caught-and-rethrown or a locally-constructed error variable throws. Anything
// else hands back until arbitrary thrown values are boxed.
func (r *Renderer) isThrowable(n frontend.Node) bool {
	if n.Kind() == frontend.NodeNewExpression {
		kids := r.prog.Children(n)
		if len(kids) == 0 {
			return false
		}
		_, ok := errorCtors[r.prog.Text(kids[0])]
		return ok
	}
	return false
}
