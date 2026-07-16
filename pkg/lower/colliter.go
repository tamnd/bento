package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers a manual drive of a stored Map or Set iterator (03 group 1, item
// 66): `const it = m.values()` (or keys(), and the Set siblings) followed by a hand-
// rolled `it.next()` loop. The for...of and spread consumers of such a stored iterator
// already see through to the receiver and range its snapshot; a manual next() drive
// cannot, because the iterator object is read many times and stepped by hand, so it
// needs a real iterator value. That value is the runtime *value.ArrayIter minted over
// the receiver's insertion-ordered snapshot slice: values() and a Set's members box
// through Values or Members, keys() through Keys, and each step packs the same
// value.IterResult a generator and an array iterator hand back, so `.value` and
// `.done` read off it the same way.
//
// The snapshot is taken when the iterator is minted, the moment map.values() runs. A
// live iterator would observe a mutation made to the collection before it is drained;
// the snapshot cannot, so construction hands back when the receiver is mutated at any
// point after the mint, the same faithfulness bar the for...of drive holds against its
// loop body. entries() yields a [key, value] pair, which this drive does not
// materialize, so it stays on the handback.

// isCollIteratorType reports whether t is the MapIterator or SetIterator a Map or
// Set's keys(), values(), or entries() produces, the type a stored manual drive binds
// and this file lowers to the *value.ArrayIter slot. The checker names it MapIterator
// or SetIterator with its element type argument, so it is recognized by that symbol
// name, the way the array iterator is recognized by ArrayIterator.
func (r *Renderer) isCollIteratorType(t frontend.Type) bool {
	sym, ok := r.prog.TypeSymbol(t)
	return ok && (sym.Name == "MapIterator" || sym.Name == "SetIterator")
}

// isCollIteratorReceiver reports whether obj holds a Map or Set iterator value, the
// receiver a manual it.next() drives. It reads the node's type first and then, for a
// plain identifier, the type its symbol was declared with, the same two-step the array
// iterator and generator receiver checks make so a binding whose narrowed type moved
// still resolves.
func (r *Renderer) isCollIteratorReceiver(obj frontend.Node) bool {
	if r.isCollIteratorType(r.prog.TypeAt(obj)) {
		return true
	}
	if obj.Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(obj)
	if !ok {
		return false
	}
	return r.isCollIteratorType(r.prog.TypeOfSymbol(sym))
}

// collIterMethodCall lowers a manual drive of a Map or Set iterator, it.next(), to the
// runtime's Next, which packs the { value, done } result the keys or values kind
// yields off the snapshot the iterator was minted over. The receiver is the
// *value.ArrayIter the construction produced; Next takes no argument, the way the
// array iterator's does, since a snapshot walk has no suspended yield to resume. A
// receiver that is not a collection iterator returns ok false so the caller keeps
// looking; a method other than next is a later slice, as return and throw on it wait
// on the same drive the array iterator's do.
func (r *Renderer) collIterMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, bool, error) {
	if !r.isCollIteratorReceiver(recvNode) {
		return nil, false, nil
	}
	if method != "next" {
		return nil, false, &NotYetLowerable{Reason: "a Map or Set iterator's ." + method + "() is a later slice"}
	}
	if len(argNodes) != 0 {
		return nil, false, &NotYetLowerable{Reason: "a Map or Set iterator's next() with an argument is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, false, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Next")}}, true, nil
}

// collIterConstructor lowers m.values(), m.keys(), and the Set siblings, when they are
// not consumed by a for...of or spread that ranges the receiver directly, to the
// runtime that mints an *value.ArrayIter over the receiver's snapshot slice. The
// accessor whose typed snapshot holds the yielded members comes from collIterAccessor:
// a Map's keys() and values() read Keys and Values, a Set's keys() and values() both
// read Members. Each element boxes through the same closure a generator's yield does,
// since the iterator yields dynamic values whatever the member type. entries() yields
// a pair this drive does not materialize, and a member type with no dynamic boxing,
// both hand back. A receiver mutated after this mint would diverge from the snapshot,
// so that hands back too.
func (r *Renderer) collIterConstructor(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 0 {
		return nil, &NotYetLowerable{Reason: "a Map or Set iterator method takes no arguments"}
	}
	if method == "entries" {
		return nil, &NotYetLowerable{Reason: "a manual drive of a Map or Set entries() iterator yields a pair, a later slice"}
	}
	accessor, elem, ok := r.collIterMemberAccessor(recvNode, method)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a manual drive of this Map or Set iterator is a later slice"}
	}
	if hb := r.collIterMutationHandback(recvNode); hb != nil {
		return nil, hb
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
		Fun:  sel("value", "ArrayIterFromSlice"),
		Args: []ast.Expr{collCall(recv, accessor), sel("value", "ArrayIterValues"), box},
	}, nil
}

// collIterMemberAccessor reports the Go accessor whose insertion-ordered snapshot slice
// holds the members keys() or values() yields off recvNode, and the member type, given
// the Map or Set receiver and the method. A Map's keys() reads Keys and its values()
// reads Values; a Set's keys() and values() both read Members, since a Set's key is its
// value. A receiver that is neither a Map nor a Set, or whose key, value, or member type
// the checker does not expose, reports ok false so the construction hands back.
func (r *Renderer) collIterMemberAccessor(recvNode frontend.Node, method string) (accessor string, elem frontend.Type, ok bool) {
	switch {
	case r.isSet(recvNode):
		e, eok := r.setElem(r.prog.TypeAt(recvNode))
		if !eok {
			return "", frontend.Type{}, false
		}
		return "Members", e, true
	case r.isMap(recvNode):
		k, v, mok := r.mapKeyVal(r.prog.TypeAt(recvNode))
		if !mok {
			return "", frontend.Type{}, false
		}
		if method == "keys" {
			return "Keys", k, true
		}
		return "Values", v, true
	}
	return "", frontend.Type{}, false
}

// collIterMutationHandback returns a handback when the receiver a manual iterator is
// minted over is mutated after the mint. The iterator walks a snapshot taken when it is
// built, which is faithful only when the collection does not change before the drive
// drains it: a JavaScript iterator would see a later add or delete, the snapshot cannot.
// The receiver must be a plain identifier the module never reassigns, so a mutating call
// on that same identifier at a source position past the mint marks the snapshot stale
// and hands back. A receiver that is not a stable identifier is conservatively treated as
// mutable and hands back too. The live iterator that observes the change is a later slice.
func (r *Renderer) collIterMutationHandback(recvNode frontend.Node) *NotYetLowerable {
	if recvNode.Kind() != frontend.NodeIdentifier {
		return &NotYetLowerable{Reason: "a manual drive of a Map or Set iterator over other than a stable identifier is a later slice"}
	}
	sym, ok := r.prog.SymbolAt(recvNode)
	if !ok || r.writeUses[sym] != 0 {
		return &NotYetLowerable{Reason: "a manual drive of a Map or Set iterator over a reassigned receiver is a later slice"}
	}
	for _, p := range r.collMutations[sym] {
		if p > recvNode.Pos() {
			return &NotYetLowerable{Reason: "a manual drive of a Map or Set iterator whose receiver is mutated before the drive drains it needs the iterator's live view, a later slice"}
		}
	}
	return nil
}

// collectCollMutations records, per Map or Set identifier symbol, the source positions
// of every mutating call on it: add, set, delete, or clear. A manual iterator drive
// reads these to prove its receiver is not mutated after the iterator is minted, the
// snapshot faithfulness bar. It runs once over the module the way the other use tallies
// do, so a construction site can test a receiver against it without re-walking.
func (r *Renderer) collectCollMutations(entry frontend.Node) {
	r.collMutations = map[frontend.Symbol][]frontend.Pos{}
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		if n.Kind() == frontend.NodeCallExpression {
			kids := r.prog.Children(n)
			if len(kids) >= 1 && kids[0].Kind() == frontend.NodePropertyAccessExpression {
				parts := r.prog.Children(kids[0])
				if len(parts) == 2 && parts[0].Kind() == frontend.NodeIdentifier {
					switch r.prog.Text(parts[1]) {
					case "add", "set", "delete", "clear":
						if sym, ok := r.prog.SymbolAt(parts[0]); ok {
							r.collMutations[sym] = append(r.collMutations[sym], parts[0].Pos())
						}
					}
				}
			}
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	walk(entry)
}
