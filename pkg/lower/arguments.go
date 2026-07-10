package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file materializes the arguments object a function body reads. TypeScript
// types arguments as IArguments, a static shape whose .length and indexing would
// otherwise hand back, so a body that reads arguments needs a real backing store.
//
// The store is a *value.Array[value.Value] materialized at body entry from the
// declared parameters. This is faithful only when the call site always passes
// exactly one argument per parameter, which the checker guarantees for a function
// whose parameters are all required and carries no rest: too few or too many
// arguments is a type error, so arguments.length equals the parameter count and
// arguments[i] is the i-th parameter at every call. A function with an optional,
// defaulted, or rest parameter sees a call-site-varying arity the body cannot
// reconstruct from its parameters, so it hands back to a later slice.

// argumentsPlan decides whether a function declaration materializes an arguments
// object. It returns the materialization statement to prepend to the body and the
// Go name of the backing local, or ok=false when the body does not read arguments.
// It returns a NotYetLowerable when the body reads arguments in a way this slice
// does not back yet, so the whole function hands back rather than emit a body that
// reads a store the guards cannot fill soundly.
func (r *Renderer) argumentsPlan(fn frontend.Node, sig frontend.Signature) (ast.Stmt, string, bool, error) {
	block, ok := r.funcBodyBlock(fn)
	if !ok {
		return nil, "", false, nil
	}
	reads, supported := false, true
	for _, stmt := range r.prog.Children(block) {
		r.scanArguments(stmt, &reads, &supported)
	}
	if !reads {
		return nil, "", false, nil
	}
	if !supported {
		return nil, "", false, &NotYetLowerable{Reason: "this read of arguments is a later slice"}
	}
	// The parameters must exactly capture the call arity for the store to stand in
	// for the passed arguments: a rest gathers a call-varying tail, and an optional
	// or defaulted parameter lets a call omit a slot, so either makes arguments.length
	// depend on the call site the body cannot see.
	if sig.RestParam != nil {
		return nil, "", false, &NotYetLowerable{Reason: "arguments in a function with a rest parameter needs the call-site arity, a later slice"}
	}
	if sig.MinArgs != len(sig.Params) {
		return nil, "", false, &NotYetLowerable{Reason: "arguments in a function with an omittable parameter needs the call-site arity, a later slice"}
	}
	// Each parameter is boxed into the store, so each must lower to a plain Go local
	// whose static type boxes into a value.Value. A destructured parameter has no
	// single Go name, and a parameter typed as an object or array has no primitive
	// box, so either hands back.
	boxes := make([]ast.Expr, 0, len(sig.Params))
	for _, p := range sig.Params {
		pname, ok := localName(p.Name)
		if !ok {
			return nil, "", false, &NotYetLowerable{Reason: "arguments over a parameter with no plain Go name is a later slice"}
		}
		box, err := r.boxStaticToDynamicFlags(ident(pname), p.Type.Flags)
		if err != nil {
			return nil, "", false, err
		}
		boxes = append(boxes, box)
	}
	r.requireImport(valuePkg)
	name := r.freshTemp()
	mat := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(name)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun:  index(sel("value", "NewArray"), sel("value", "Value")),
			Args: boxes,
		}},
	}
	return mat, name, true, nil
}

// scanArguments walks a body node and records whether it reads the arguments object
// and whether every such read is a shape this slice backs. It descends into the
// receiver of a member or index access but never into its name, so obj.arguments
// (a property named arguments) is not mistaken for the arguments object, and it
// stops at a nested function or method, each of which binds its own arguments. It
// does not stop at an arrow, which has no arguments of its own and reads the
// enclosing function's, so an arrow's read counts toward this body. A bare
// reference to arguments that no backed shape consumed marks the body unsupported,
// so the plan hands the whole function back.
func (r *Renderer) scanArguments(n frontend.Node, reads, supported *bool) {
	switch n.Kind() {
	case frontend.NodeFunctionDeclaration, frontend.NodeFunctionExpression,
		frontend.NodeMethodDeclaration, frontend.NodeGetAccessor,
		frontend.NodeSetAccessor, frontend.NodeConstructor:
		// A nested function binds its own arguments, so its reads are not this body's.
		// An arrow is not stopped: it has no arguments of its own and reads the
		// enclosing function's, so its read counts here and its lowered closure captures
		// the store this body materializes.
		return
	case frontend.NodeIdentifier:
		if r.prog.Text(n) == "arguments" {
			*reads = true
			*supported = false
		}
		return
	case frontend.NodePropertyAccessExpression:
		kids := r.prog.Children(n)
		if len(kids) == 2 && r.isArgumentsIdent(kids[0]) && r.prog.Text(kids[1]) == "length" {
			*reads = true
			return
		}
		if len(kids) == 2 {
			// Descend only into the receiver: the name child is a property key, never a
			// value reference to the arguments object.
			r.scanArguments(kids[0], reads, supported)
			return
		}
	case frontend.NodeElementAccessExpression:
		kids := r.prog.Children(n)
		if len(kids) == 2 && r.isArgumentsIdent(kids[0]) {
			*reads = true
			// The receiver is the arguments object, backed by the store; scan only the
			// index expression, which is an ordinary read.
			r.scanArguments(kids[1], reads, supported)
			return
		}
	case frontend.NodeForOfStatement:
		kids := r.prog.Children(n)
		if len(kids) == 3 && r.isArgumentsIdent(kids[1]) {
			*reads = true
			// The iterable is the arguments object, ranged over the store; scan the loop
			// binding and body but not the iterable identifier.
			r.scanArguments(kids[0], reads, supported)
			r.scanArguments(kids[2], reads, supported)
			return
		}
	}
	for _, c := range r.prog.Children(n) {
		r.scanArguments(c, reads, supported)
	}
}

// isArgumentsIdent reports whether a node is a bare identifier reading the
// arguments object. In the strict-mode code bento compiles a binding named
// arguments is a syntax error, so any identifier spelled arguments in a function
// body is the implicit arguments object.
func (r *Renderer) isArgumentsIdent(n frontend.Node) bool {
	return n.Kind() == frontend.NodeIdentifier && r.prog.Text(n) == "arguments"
}
