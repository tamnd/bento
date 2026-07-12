package lower

import (
	"go/ast"
	"go/token"

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

// arrayIterForOfCall reports whether a for...of iterable is a.values(), a.keys(),
// or a.entries() over an array receiver, the form for...of consumes without ever
// building the iterator object. When it is, it returns the receiver node and the
// method so the loop ranges the receiver directly, the idiomatic Go a developer
// writes rather than a pull-until-done drive of an iterator it can range past. A
// stored array iterator, or one produced any other way, is not this shape and takes
// the general iterator path (a later slice), which keeps the rewrite to the common
// literal form. The call must take no argument, and its receiver must be an array.
func (r *Renderer) arrayIterForOfCall(iterable frontend.Node) (frontend.Node, string, bool) {
	if iterable.Kind() != frontend.NodeCallExpression {
		return nil, "", false
	}
	kids := r.prog.Children(iterable)
	if len(kids) != 1 || kids[0].Kind() != frontend.NodePropertyAccessExpression {
		return nil, "", false
	}
	parts := r.prog.Children(kids[0])
	if len(parts) != 2 {
		return nil, "", false
	}
	method := r.prog.Text(parts[1])
	switch method {
	case "values", "keys", "entries":
	default:
		return nil, "", false
	}
	if _, ok := r.arrayElem(parts[0]); !ok {
		return nil, "", false
	}
	return parts[0], method, true
}

// forOfArrayIterSingle lowers `for (const x of a.values())` and `for (const i of
// a.keys())` to a range over the receiver array, the loop the iterator object would
// only wrap. values() binds each element to the loop variable, the same range the
// plain for...of over the array emits; keys() ranges the index and converts it to
// the number the loop variable holds. A binding the body never reads drops the way
// the plain array loop drops its unused value, so the Go loop compiles. entries()
// with a single binding yields a pair the range does not have on hand, so it hands
// back rather than materialize one; a destructuring [i, v] takes the entries path.
func (r *Renderer) forOfArrayIterSingle(recvNode, bindNode frontend.Node, name, method string, bodyNode frontend.Node) (ast.Stmt, error) {
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, err
	}
	elems := &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Elems")}}
	used := r.bodyUsesName(bodyNode, r.prog.Text(bindNode))
	rng := &ast.RangeStmt{X: elems, Body: body}
	switch method {
	case "values":
		if used {
			rng.Key = ident("_")
			rng.Value = ident(name)
			rng.Tok = token.DEFINE
		}
		return rng, nil
	case "keys":
		if used {
			idx := r.freshTemp()
			rng.Key = ident(idx)
			rng.Tok = token.DEFINE
			conv := &ast.AssignStmt{
				Lhs: []ast.Expr{ident(name)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{&ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{ident(idx)}}},
			}
			body.List = append([]ast.Stmt{conv}, body.List...)
		}
		return rng, nil
	default:
		return nil, &NotYetLowerable{Reason: "a for...of over an array iterator's entries() with a single binding is a later slice"}
	}
}

// forOfEntriesDestructure lowers `for (const [i, v] of a.entries())` to a range over
// the receiver array whose index and element bind the two pattern names directly: the
// index converts to the number i holds, the element binds to v as the plain array
// loop binds its value. Only a flat two-name pattern is lowered; a hole, a default, a
// rest, or a nested pattern hands back, each a later slice. Either name the body never
// reads drops, the same unused-binding rule the single-variable loop applies, so the
// Go loop compiles rather than leave an unused local.
func (r *Renderer) forOfEntriesDestructure(recvNode, pattern, bodyNode frontend.Node) (ast.Stmt, error) {
	elems := r.prog.Children(pattern)
	if len(elems) != 2 {
		return nil, &NotYetLowerable{Reason: "a for...of over entries() with other than a two-name pattern is a later slice"}
	}
	var names [2]string
	var used [2]bool
	for i, el := range elems {
		ec := r.prog.Children(el)
		if len(ec) != 1 || ec[0].Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: "a for...of over entries() with a hole, default, rest, or nested pattern is a later slice"}
		}
		nm, ok := localName(r.prog.Text(ec[0]))
		if !ok {
			return nil, &NotYetLowerable{Reason: "a for...of over entries() bound name is not a Go identifier"}
		}
		names[i] = nm
		used[i] = r.bodyUsesName(bodyNode, r.prog.Text(ec[0]))
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, err
	}
	rng := &ast.RangeStmt{X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Elems")}}, Body: body}
	if !used[0] && !used[1] {
		return rng, nil
	}
	rng.Tok = token.DEFINE
	if used[0] {
		idx := r.freshTemp()
		rng.Key = ident(idx)
		conv := &ast.AssignStmt{
			Lhs: []ast.Expr{ident(names[0])},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{ident(idx)}}},
		}
		body.List = append([]ast.Stmt{conv}, body.List...)
	} else {
		rng.Key = ident("_")
	}
	if used[1] {
		rng.Value = ident(names[1])
	}
	return rng, nil
}
