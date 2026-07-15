package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// A function overload set is one symbol with several declarations: one or more
// bodyless signature declarations (function f(x: number): number;) and a single
// implementation with a body (function f(x: any): any { ... }). Only the
// implementation runs; the signatures are compile-time views the checker resolves
// each call against. So the whole set lowers to one Go function for the
// implementation body, and every call routes through it.
//
// This slice claims only the representation-safe subset: an implementation whose
// parameters are all required and all dynamic (any or unknown) and whose return is
// dynamic or void. Such an implementation lowers to func F(x value.Value) value.Value,
// so a call boxes each argument into a value.Value and reads the result back as a box,
// the same dynamic contract a call through a value.Value slot already uses. An
// implementation with a concrete or optional parameter is a later slice, since the
// call site would have to reconstruct the exact Go parameter type the impl lowered to.

// overloadImplNode reports whether sym is a function overload set and, when it is,
// returns its single implementation declaration, the one carrying a body block. A set
// has at least one bodyless signature declaration and exactly one implementation; any
// other shape (an ordinary single-declaration function, a merged non-function
// declaration, or two bodies) is not claimed here.
func (r *Renderer) overloadImplNode(sym frontend.Symbol) (frontend.Node, bool) {
	decls := r.prog.Declarations(sym)
	var impl frontend.Node
	sigCount := 0
	for _, d := range decls {
		if d.Kind() != frontend.NodeFunctionDeclaration {
			return nil, false
		}
		if _, ok := r.funcBodyBlock(d); ok {
			if impl != nil {
				return nil, false
			}
			impl = d
			continue
		}
		sigCount++
	}
	if impl == nil || sigCount == 0 {
		return nil, false
	}
	return impl, true
}

// implSigAllDynamic reports whether an overload implementation's own signature is the
// all-dynamic shape this slice lowers: no type parameters, no rest parameter, every
// parameter required and typed any or unknown, and a return typed any, unknown, void,
// or undefined. Only then does the implementation lower to a Go func over value.Value
// parameters and a value.Value result, which is what lets the call box its arguments
// and read the result back as a box.
func (r *Renderer) implSigAllDynamic(impl frontend.Node) bool {
	sig, ok := r.prog.SignatureAt(impl)
	if !ok {
		return false
	}
	if len(sig.TypeParams) != 0 || sig.RestParam != nil {
		return false
	}
	for _, p := range sig.Params {
		if p.Optional {
			return false
		}
		if p.Type.Flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 {
			return false
		}
	}
	ret := sig.Return
	return ret.Flags&(frontend.TypeAny|frontend.TypeUnknown|frontend.TypeVoid|frontend.TypeUndefined) != 0
}

// overloadedFuncImpl returns the implementation declaration of an overloaded function
// symbol this slice claims: a function overload set whose implementation is the
// all-dynamic shape. It is the single predicate the top-level loop, the call site, and
// isDynamic share, so the three agree on exactly which functions the overload path owns.
func (r *Renderer) overloadedFuncImpl(sym frontend.Symbol) (frontend.Node, bool) {
	impl, ok := r.overloadImplNode(sym)
	if !ok {
		return nil, false
	}
	if !r.implSigAllDynamic(impl) {
		return nil, false
	}
	return impl, true
}

// overloadedCall lowers a call to a user-defined overloaded function through its
// implementation. The implementation lowered to a Go func over value.Value parameters,
// so each argument boxes into a value.Value rather than bridging against the matched
// overload's parameter type, which SignatureAt reports at the call and which the impl's
// Go signature does not use. The result is the value.Value the impl returns; isDynamic
// recognizes the call by shape so the surrounding context coerces the box the same way
// it coerces any other dynamic value. A checker-rejected call (code 2769) reaches here
// too, since the front door admits the code, and lowers to the same boxed dispatch,
// which runs the implementation with the argument it was actually given, the run-time
// behavior JavaScript has for a call no overload signature accepts.
func (r *Renderer) overloadedCall(n frontend.Node, name string, argNodes []frontend.Node) (ast.Expr, error) {
	impl, ok := r.overloadedFuncImpl(r.mustSymbolAt(n))
	if !ok {
		return nil, &NotYetLowerable{Reason: "overloaded call lost its implementation between dispatch and lowering"}
	}
	sig, _ := r.prog.SignatureAt(impl)
	if len(argNodes) != len(sig.Params) {
		// The implementation has a fixed Go arity, so a call must supply exactly that
		// many arguments. An all-required implementation means every overload shares the
		// implementation's arity, so a valid call always matches; a call that does not
		// hands back rather than emit a Go call with the wrong argument count.
		return nil, &NotYetLowerable{Reason: "a call whose argument count does not match the overloaded implementation is a later slice"}
	}
	r.markOverloadCallSeen(n)
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		if a.Kind() == frontend.NodeSpreadElement {
			return nil, &NotYetLowerable{Reason: "a spread argument in an overloaded call is a later slice"}
		}
		boxed, err := r.boxOperand(a)
		if err != nil {
			return nil, err
		}
		args = append(args, boxed)
	}
	return &ast.CallExpr{Fun: ident(name), Args: args}, nil
}

// callOfOverloadedFunc reports whether n is a call to a user-defined overloaded
// function this slice lowers. isDynamic reads it to keep the call's boxed result on the
// dynamic path: with overloads the checker narrows the call to the matched overload's
// return, not any, so the type-flag test at the end of isDynamic would miss the box.
func (r *Renderer) callOfOverloadedFunc(n frontend.Node) bool {
	if n.Kind() != frontend.NodeCallExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(kids[0])
	if !ok || sym.Flags&frontend.SymbolFunction == 0 {
		return false
	}
	_, ok = r.overloadedFuncImpl(sym)
	return ok
}

// mustSymbolAt returns the symbol of a call expression's callee identifier, the symbol
// overloadedCall dispatched on. The dispatch already proved the callee is an overloaded
// function identifier, so a miss here is a lowering bug, not a program the checker let
// through; it returns the zero symbol, which overloadedFuncImpl rejects.
func (r *Renderer) mustSymbolAt(n frontend.Node) frontend.Symbol {
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		return frontend.Symbol{}
	}
	sym, _ := r.prog.SymbolAt(kids[0])
	return sym
}
