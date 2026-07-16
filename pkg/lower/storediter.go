package lower

import "github.com/tamnd/bento/pkg/frontend"

// storedCollIter is a Map or Set iterator a local stores, resolved to the receiver
// and accessor the direct-call for...of forms already range: a receiver node, the Go
// accessor whose insertion-ordered snapshot holds the yielded members (Keys, Values,
// or Members, with entries() reported by an empty accessor and driven as a pair), and
// whether the receiver is a "map" or a "set".
type storedCollIter struct {
	recv   frontend.Node
	method string
	kind   string
}

// collectStoredCollIters records every `const it = m.keys()` and its siblings whose
// Map or Set iterator is stored in a local and consumed by exactly one for...of, so
// the declaration can emit nothing and the for...of can range the receiver directly.
// It runs after countBindingUses and countBindingDecls, whose tallies it reads to
// prove single use, and populates r.storedCollIters. A binding it does not record
// keeps the ordinary declaration path, which hands its iterator-object type back.
//
// A stored built-in iterator is stateful and single-use in JavaScript, so recording
// one is sound only when the local is referenced exactly once and that reference is a
// for...of iterable: a second read would replay the receiver's snapshot from the
// start, which the live iterator would not. The receiver must be an identifier the
// module never reassigns, so capturing it at the declaration and ranging it at the
// loop name the same object the direct `m.keys()` at the loop would.
func (r *Renderer) collectStoredCollIters(entry frontend.Node) {
	r.storedCollIters = map[frontend.Symbol]storedCollIter{}
	// The symbols an identifier resolves to where it is consumed as a whole iterable:
	// the iterable of a for...of, or the operand of a spread. A candidate is recorded
	// only when its one reference is one of these, so the see-through path that ranges
	// the receiver's snapshot is the code that handles the reference. A reference in any
	// other position (a manual next(), an argument) is not a consumer, so the binding
	// keeps the ordinary path and hands its iterator-object type back.
	consumerSyms := map[frontend.Symbol]bool{}
	var findConsumers func(n frontend.Node)
	findConsumers = func(n frontend.Node) {
		switch n.Kind() {
		case frontend.NodeForOfStatement:
			kids := r.prog.Children(n)
			if len(kids) >= 2 && kids[1].Kind() == frontend.NodeIdentifier {
				if sym, ok := r.prog.SymbolAt(kids[1]); ok {
					consumerSyms[sym] = true
				}
			}
		case frontend.NodeSpreadElement:
			kids := r.prog.Children(n)
			if len(kids) == 1 && kids[0].Kind() == frontend.NodeIdentifier {
				if sym, ok := r.prog.SymbolAt(kids[0]); ok {
					consumerSyms[sym] = true
				}
			}
		}
		for _, c := range r.prog.Children(n) {
			findConsumers(c)
		}
	}
	findConsumers(entry)

	var findDecls func(n frontend.Node)
	findDecls = func(n frontend.Node) {
		if n.Kind() == frontend.NodeVariableDeclaration {
			if sym, ci, ok := r.storedCollIterDecl(n); ok && consumerSyms[sym] {
				// The binding is read exactly once, and that read is the consumer above:
				// its references past the declaration name nodes total one, which the walk
				// proved is a for...of iterable or a spread operand. A binding read more than
				// once, reassigned (a write is another reference), or read somewhere other
				// than the one consumer fails this and keeps the ordinary path.
				if r.bindingUses[sym]-r.bindingDecls[sym] == 1 {
					r.storedCollIters[sym] = ci
				}
			}
		}
		for _, c := range r.prog.Children(n) {
			findDecls(c)
		}
	}
	findDecls(entry)
}

// storedCollIterDecl reports whether a variable declaration is `name = <call>` whose
// initializer is a Map or Set keys()/values()/entries() call over an identifier
// receiver the module never reassigns, returning the binding symbol and the receiver
// and accessor the for...of ranges. A destructuring or multi-part binding, a non-call
// initializer, a receiver that is not a stable identifier, or a call the direct
// for...of path does not recognize all report ok=false.
func (r *Renderer) storedCollIterDecl(decl frontend.Node) (frontend.Symbol, storedCollIter, bool) {
	kids := r.prog.Children(decl)
	if len(kids) < 2 || kids[0].Kind() != frontend.NodeIdentifier {
		return frontend.Symbol{}, storedCollIter{}, false
	}
	initNode := kids[len(kids)-1]
	if initNode.Kind() == frontend.NodeUnknown {
		return frontend.Symbol{}, storedCollIter{}, false
	}
	recv, method, kind, ok := r.mapSetIterForOfCall(initNode)
	if !ok || recv.Kind() != frontend.NodeIdentifier {
		return frontend.Symbol{}, storedCollIter{}, false
	}
	recvSym, ok := r.prog.SymbolAt(recv)
	if !ok || r.writeUses[recvSym] != 0 {
		return frontend.Symbol{}, storedCollIter{}, false
	}
	sym, ok := r.prog.SymbolAt(kids[0])
	if !ok {
		return frontend.Symbol{}, storedCollIter{}, false
	}
	return sym, storedCollIter{recv: recv, method: method, kind: kind}, true
}

// storedCollIterOf resolves a node to the receiver and accessor of a stored Map or Set
// iterator when the node is an identifier bound to one the pre-pass recorded, so the
// for...of and spread see-through can range the receiver the direct call form ranges.
// Any other node, or an identifier not recorded, reports ok=false.
func (r *Renderer) storedCollIterOf(iterable frontend.Node) (recv frontend.Node, method, kind string, ok bool) {
	if iterable.Kind() != frontend.NodeIdentifier {
		return nil, "", "", false
	}
	sym, symOK := r.prog.SymbolAt(iterable)
	if !symOK {
		return nil, "", "", false
	}
	ci, recorded := r.storedCollIters[sym]
	if !recorded {
		return nil, "", "", false
	}
	return ci.recv, ci.method, ci.kind, true
}
