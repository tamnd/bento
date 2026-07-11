package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the array iterator (08 group 3): the object arr.values(),
// arr.keys(), and arr.entries() hand back, which the language drives through a
// hand-rolled next() and through for...of. The iterator is the runtime
// *value.ArrayIter, minted over the receiver array and walked by index; its next()
// packs the same value.IterResult a generator's next() does, so a caller reads
// .value and .done off it the same way.

// isArrayIteratorType reports whether t is the ArrayIterator an array's values,
// keys, or entries produces, the type that lowers to the *value.ArrayIter slot. The
// checker names it ArrayIterator with a single type argument (the element for
// values, the number index for keys, the [index, element] pair for entries), so it
// is recognized by that symbol name rather than by shape.
func (r *Renderer) isArrayIteratorType(t frontend.Type) bool {
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "ArrayIterator"
}

// isArrayIteratorReceiver reports whether obj is an ArrayIterator value, the
// receiver a manual it.next() drives. It reads the node's type first and then, for a
// plain identifier, the type its symbol was declared with, the same two-step a
// generator's IterResult receiver check makes so a narrowed binding still resolves.
func (r *Renderer) isArrayIteratorReceiver(obj frontend.Node) bool {
	if r.isArrayIteratorType(r.prog.TypeAt(obj)) {
		return true
	}
	if obj.Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(obj)
	if !ok {
		return false
	}
	return r.isArrayIteratorType(r.prog.TypeOfSymbol(sym))
}

// iterResultBoxedValueRead reports whether n is a `.value` read off an
// IteratorResult whose static type is not a clean primitive the box coerces down to,
// the array iterator's `number | undefined` value being the first. The IterResult
// stores its value as a boxed value.Value, so a read the checker types as a union
// (a value that may be the yielded element or the undefined a done result carries)
// has no single Go type to coerce to and stays the box on the dynamic path. A
// generator whose value is a clean number, string, or boolean keeps the static
// coercion the member read applies, so this leaves the generator path unchanged.
func (r *Renderer) iterResultBoxedValueRead(n frontend.Node) bool {
	if n.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	parts := r.prog.Children(n)
	if len(parts) != 2 || r.prog.Text(parts[1]) != "value" {
		return false
	}
	if !r.isIterResultReceiver(parts[0]) {
		return false
	}
	f := r.prog.TypeAt(n).Flags
	return f&frontend.TypeUnion != 0 || f&(frontend.TypeNumber|frontend.TypeString|frontend.TypeBoolean) == 0
}

// arrayIterMethodCall lowers a manual drive of an array iterator, it.next(), to the
// runtime's Next, which packs the { value, done } result the values, keys, or
// entries kind yields. The receiver is the *value.ArrayIter the iterator produced;
// Next takes no argument, unlike a generator's next(v), since an array walk has no
// suspended yield to resume. A receiver that is not an array iterator returns ok
// false so the caller keeps looking; a method other than next is a later slice, as
// return and throw on an array iterator wait on the same drive the generator has.
func (r *Renderer) arrayIterMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, bool, error) {
	if !r.isArrayIteratorReceiver(recvNode) {
		return nil, false, nil
	}
	if method != "next" {
		return nil, false, &NotYetLowerable{Reason: "an array iterator's ." + method + "() is a later slice"}
	}
	if len(argNodes) != 0 {
		return nil, false, &NotYetLowerable{Reason: "an array iterator's next() with an argument is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, false, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Next")}}, true, nil
}

// arrayIterConstructor lowers arr.values(), arr.keys(), and arr.entries() to the
// runtime that mints an array iterator over the receiver. A statically typed array
// boxes its elements once through value.ArrayIterFromTyped, whose box closure lifts
// each typed element into the dynamic value the iterator yields; the element type
// comes from the receiver. The three methods differ only in the kind constant they
// pass. An element type with no dynamic boxing (an array of objects) hands back the
// way a generator's yield of the same type does.
func (r *Renderer) arrayIterConstructor(recvNode frontend.Node, kind string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 0 {
		return nil, &NotYetLowerable{Reason: "an array iterator method takes no arguments"}
	}
	elem, ok := r.prog.ElementType(r.prog.TypeAt(recvNode))
	if !ok {
		return nil, &NotYetLowerable{Reason: "an array iterator over a non-array receiver is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	box, err := r.genElemBoxer(elem)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun:  sel("value", "ArrayIterFromTyped"),
		Args: []ast.Expr{recv, sel("value", kind), box},
	}, nil
}
