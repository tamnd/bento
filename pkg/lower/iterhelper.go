package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the iterator helpers (10_advanced group 5): the lazy methods
// ES2024 hangs off Iterator.prototype. map, filter, take, drop, and flatMap return a
// new iterator; reduce, toArray, forEach, some, every, and find drive the source and
// return a value. Every helper lowers to a value.Iter* free function over the
// receiver's Next method, so an array iterator (arr.values(), *value.ArrayIter) and a
// helper result (Iterator.from(...), *value.IterHelper) feed the same path: both
// expose a no-argument Next() value.IterResult the helper closes over.
//
// The checker types a helper result as IteratorObject, which lower.go maps to
// *value.IterHelper, distinct from the ArrayIterator an array's values() produces and
// from the Generator a generator function returns, so a helper chain renders to a
// pointer the runtime pulls rather than expanding structurally.

// isIterHelperType reports whether t is the IteratorObject a helper produces, the
// type that lowers to the *value.IterHelper slot. The checker names the result of
// Iterator.from and of every lazy helper IteratorObject, distinct from the
// ArrayIterator a values()/keys()/entries() call yields and the Generator a generator
// function returns, so a chained helper is recognized by that symbol name.
func (r *Renderer) isIterHelperType(t frontend.Type) bool {
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "IteratorObject"
}

// isIterHelperReceiver reports whether obj is a value the helpers drive: an
// IteratorObject (a *value.IterHelper) or an ArrayIterator (a *value.ArrayIter). Both
// expose a no-argument Next() value.IterResult, so a helper call closes over the
// receiver's Next the same way whichever it is. It reads the node's type first and
// then, for a plain identifier, the type its symbol was declared with, the same
// two-step the array iterator and generator receiver checks make.
func (r *Renderer) isIterHelperReceiver(obj frontend.Node) bool {
	if r.isIterHelperType(r.prog.TypeAt(obj)) || r.isArrayIteratorType(r.prog.TypeAt(obj)) {
		return true
	}
	if obj.Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(obj)
	if !ok {
		return false
	}
	t := r.prog.TypeOfSymbol(sym)
	return r.isIterHelperType(t) || r.isArrayIteratorType(t)
}

// iterHelperMethodCall lowers a helper method on an iterator receiver to the matching
// value.Iter* free function over the receiver's Next. It fires for an IteratorObject
// or an ArrayIterator receiver and returns ok false otherwise so the caller keeps
// looking. The lazy helpers (map, filter) return a new *value.IterHelper; a manual
// next() on an IteratorObject returns its Next result. A method outside the set this
// slice covers hands back with a named reason.
func (r *Renderer) iterHelperMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, bool, error) {
	if !r.isIterHelperReceiver(recvNode) {
		return nil, false, nil
	}
	// A manual next() on an IteratorObject drives the helper directly, recv.Next(); an
	// ArrayIterator's next() stays on its own path (arrayIterMethodCall), so next is
	// handled here only for the IteratorObject receiver and otherwise returns ok false.
	if method == "next" {
		if !r.isIterHelperType(r.prog.TypeAt(recvNode)) && !(recvNode.Kind() == frontend.NodeIdentifier && r.iterHelperSymbol(recvNode)) {
			return nil, false, nil
		}
		if len(argNodes) != 0 {
			return nil, false, &NotYetLowerable{Reason: "an iterator helper's next() with an argument is a later slice"}
		}
		next, err := r.iterReceiverNext(recvNode)
		if err != nil {
			return nil, false, err
		}
		return &ast.CallExpr{Fun: next}, true, nil
	}
	switch method {
	case "map", "filter":
		if len(argNodes) != 1 {
			return nil, false, &NotYetLowerable{Reason: "an iterator helper's ." + method + "() takes exactly a callback"}
		}
		next, err := r.iterReceiverNext(recvNode)
		if err != nil {
			return nil, false, err
		}
		fn, err := r.boxOperand(argNodes[0])
		if err != nil {
			return nil, false, err
		}
		r.requireImport(valuePkg)
		goName := map[string]string{"map": "IterMap", "filter": "IterFilter"}[method]
		return &ast.CallExpr{Fun: sel("value", goName), Args: []ast.Expr{next, fn}}, true, nil
	default:
		return nil, false, &NotYetLowerable{Reason: "an iterator helper's ." + method + "() is a later slice"}
	}
}

// iterHelperSymbol reports whether a plain-identifier receiver's declared symbol type
// is an IteratorObject, the second half of the helper-receiver test for a next() call
// so a narrowed binding still resolves to the helper path.
func (r *Renderer) iterHelperSymbol(obj frontend.Node) bool {
	sym, ok := r.prog.SymbolAt(obj)
	if !ok {
		return false
	}
	return r.isIterHelperType(r.prog.TypeOfSymbol(sym))
}

// iterReceiverNext lowers the receiver and returns its Next method value, the
// func() value.IterResult closure every helper free function takes. Both a
// *value.ArrayIter and a *value.IterHelper expose Next, so this is the one point the
// two receiver representations meet.
func (r *Renderer) iterReceiverNext(recvNode frontend.Node) (*ast.SelectorExpr, error) {
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	return &ast.SelectorExpr{X: recv, Sel: ident("Next")}, nil
}
