package lower

import (
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// maxBoxedSigRounds bounds the fixpoint below. Each round can only add marks, and a
// program has finitely many parameters and functions, so the loop terminates on its
// own; the bound is there so a pathological file cannot make the pass the slowest
// thing in the compiler.
const maxBoxedSigRounds = 8

// collectBoxedSignatures decides, before any body lowers, which declared parameters
// hold a box and which declared functions answer one.
//
// A box is a value.Value the walk produced, `Object.values(m)[0]` being the everyday
// one, and it has no Go fields for the shape the checker projects onto it. Note 384
// settled what happens when such a value is bound to a name: the name takes the boxed
// slot and every read off it dispatches. A declared signature is the same question
// asked from the other side. `function f(r: Row) { return r.id }` called as
// `f(Object.values(m)[0])` has nowhere to put the box, since filling a *ObjIdTag from
// it could only copy properties into a struct and a copy aliases nothing where
// JavaScript aliases everything.
//
// So the signature gives way rather than the value: a parameter a call site hands a
// box to takes a value.Value slot, and a function whose every return is a box answers
// value.Value. Both are expressed by rewriting the checker's type for that position to
// any, which is the type the rest of the lowering already knows how to carry end to
// end. The field becomes value.Value through typeExpr, the body reads the name through
// the value model because dynLocalsOf sees an any parameter, and every call site boxes
// its argument because bridgeArg sees an any target. Nothing downstream needs to learn
// a new shape.
//
// It has to be a decision about the function taken from all its call sites at once,
// because a Go function has one signature: `f(Object.values(m)[0])` and `f({id: 7, tag:
// 'z'})` in the same program must agree, and they agree on the boxed slot, which the
// static literal reaches by boxing on the way in.
//
// The pass runs to a fixpoint because one function's answer feeds another's. In
//
//	function inner(r: Row) { return r.tag }
//	function outer(r: Row) { return inner(r) }
//	outer(Object.values(m)[0])
//
// the first round learns that outer's r is a box from the top-level call, and only the
// second round can see that `inner(r)` therefore passes one too. Marks are only ever
// added, so reading the half-built maps mid-round is safe: a later round can confirm a
// mark but never take one back.
func (r *Renderer) collectBoxedSignatures(files []frontend.Node) {
	// There is no cheap guard left that would skip the pass. A signature is not the only
	// Go type it rewrites any more: a field's is not a signature, and neither is a local's,
	// so a program with no function and no class at all can still have a store that hands a
	// box to a binding. Each round is an AST walk, and a program with nothing to mark
	// changes nothing in the first one and leaves the loop there.
	cands := r.boxableFuncs(files)
	if r.boxedParams == nil {
		r.boxedParams = map[frontend.Node][]bool{}
	}
	if r.boxedReturnFns == nil {
		r.boxedReturnFns = map[frontend.Node]bool{}
	}
	if r.boxedParamSyms == nil {
		r.boxedParamSyms = map[frontend.Symbol]bool{}
	}
	if r.boxedFields == nil {
		r.boxedFields = map[frontend.Node]bool{}
	}
	if r.boxedLocals == nil {
		r.boxedLocals = map[frontend.Symbol]bool{}
	}
	for round := 0; round < maxBoxedSigRounds; round++ {
		changed := r.markBoxedParams(files, cands)
		if r.markBoxedReturns(cands) {
			changed = true
		}
		if r.markBoxedFields(files) {
			changed = true
		}
		if r.markBoxedLoopVars(files) {
			changed = true
		}
		if r.markBoxedLocals(files) {
			changed = true
		}
		if r.markBoxedCallbackParams(files) {
			changed = true
		}
		if r.markBoxedCollections(files) {
			changed = true
		}
		if !changed {
			break
		}
	}
	r.forceBoxedParams(cands)
}

// boxedSetElem reports whether a Set receiver holds its members boxed, and boxedMapKey
// and boxedMapVal the same for a Map's two slots. They are the read side of the marks
// markBoxedCollections lays down: what goes into a boxed collection boxes on its way in
// through the argument coercion that already runs, and what comes out of one is a box,
// which the callback a forEach drives, the binding a for...of makes, and the result of a
// get each have to be told.
func (r *Renderer) boxedSetElem(recvNode frontend.Node) bool {
	return len(r.boxedSetElems) > 0 && (r.isSet(recvNode) || r.isWeakSet(recvNode)) &&
		r.boxedSetElems[r.prog.TypeAt(recvNode).Identity()]
}

func (r *Renderer) boxedMapKey(recvNode frontend.Node) bool {
	return len(r.boxedMapKeys) > 0 && (r.isMap(recvNode) || r.isWeakMap(recvNode)) &&
		r.boxedMapKeys[r.prog.TypeAt(recvNode).Identity()]
}

func (r *Renderer) boxedMapVal(recvNode frontend.Node) bool {
	return len(r.boxedMapVals) > 0 && (r.isMap(recvNode) || r.isWeakMap(recvNode)) &&
		r.boxedMapVals[r.prog.TypeAt(recvNode).Identity()]
}

// readOfBoxedCollection reports whether a call reads a box out of a collection whose slot
// this pass boxed. Only get answers a member directly; the iterating reads bind their box
// through the loop and the callback rather than through an expression, so each of those is
// told at its own lowering.
func (r *Renderer) readOfBoxedCollection(n frontend.Node) bool {
	recvNode, method, _, ok := r.methodCallParts(n)
	if !ok || method != "get" {
		return false
	}
	return r.boxedMapVal(recvNode)
}

// boxedCollSource reports whether an iterable is a Map or Set spelling whose members
// this pass gave the value model, so what it yields is a box per member and a collection
// of it is a boxed array however the checker types the result. A plain Set yields its
// members; a keys() or values() call yields whichever of the two slots it reads, which
// are marked apart; a pair spelling yields an entry with a box in one half, which is a
// box itself.
func (r *Renderer) boxedCollSource(n frontend.Node) bool {
	if r.boxedCollPairSource(n) {
		return true
	}
	if r.isSet(n) {
		return r.boxedSetElem(n)
	}
	recv, accessor, _, ok := r.collIterAccessor(n)
	if !ok {
		return false
	}
	switch accessor {
	case "Keys":
		return r.boxedMapKey(recv)
	case "Values":
		return r.boxedMapVal(recv)
	}
	return r.boxedSetElem(recv)
}

// boxedCollPairSource reports whether an iterable is a pair-yielding spelling, a Map used
// directly or either kind's entries(), of a collection this pass boxed on either side.
//
// An entry holding a box is a box itself. The interned tuple such an entry otherwise
// materializes into is a Go struct, and a struct field has no room for a box, so the
// entry gives way and becomes the two-element array JavaScript says it is. That is the
// same answer the for...of over one pair takes, and it is why the pair spellings answer
// here rather than being a slot of their own: there is no tuple left to give a slot to.
func (r *Renderer) boxedCollPairSource(n frontend.Node) bool {
	if recv, method, _, ok := r.mapSetIterForOfCall(n); ok && method == "entries" {
		return r.boxedCollPair(recv)
	}
	return r.isMap(n) && r.boxedCollPair(n)
}

// arrayFromBoxedSource reports whether n is an Array.from whose one source is a box: a
// Map or Set whose member slot this pass boxed, or any value whose Go slot already holds
// a value.Value. Either way the result is one boxed array, so the sites that ask what an
// expression lowers to have to see through the concrete array type the checker gave the
// call the same way they see through a boxed collection's declared type argument.
//
// It is the one rule both the routing in arrayFrom and the readers of that result go
// through, which is what keeps the emit and the reads from disagreeing about whether the
// collected array is a box.
func (r *Renderer) arrayFromBoxedSource(n frontend.Node) bool {
	if n.Kind() != frontend.NodeCallExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) < 2 || kids[0].Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	parts := r.prog.Children(kids[0])
	if len(parts) != 2 || !r.isGlobalRef(parts[0], "Array") || r.prog.Text(parts[1]) != "from" {
		return false
	}
	// The children after the callee carry any written type arguments beside the value
	// arguments, so the source is read off the named ones rather than off kids[1].
	// A map callback is allowed alongside the source, since the array-like walk that
	// applies it produces a box just the same.
	args := r.namedArgs(kids[1:])
	if len(args) < 1 || len(args) > 2 {
		return false
	}
	return r.boxedCollSource(args[0]) || r.isDynamic(args[0])
}

// spreadOfBoxedColl reports whether a spread element splices a collection whose member
// slot this pass boxed. Every spelling that yields boxes counts, which is what
// boxedCollSource already answers: a Set spliced straight, a keys() or values() call, and
// a Map or an entries() call, whose entries are boxes once one of their halves is.
func (r *Renderer) spreadOfBoxedColl(n frontend.Node) bool {
	if n.Kind() != frontend.NodeSpreadElement {
		return false
	}
	kids := r.prog.Children(n)
	return len(kids) == 1 && r.boxedCollSource(kids[0])
}

// spreadOfUnlandableBox reports whether a spread element splices a box whose elements
// have no Go slot to come down to, which is what makes the literal around it a box too.
//
//	const rows = JSON.parse(s) as Row[]
//	const copy = [...rows]
//
// rows is a value.Value, so the splice is a run-time drain that yields value.Value
// elements. A number or string element type takes those elements down to a Go primitive
// one by one, which is what the ordinary array path now does, and the copy stays the
// typed array the checker named. A shape has no such conversion: there is no value to
// put in a *ObjIdTag that is the same object the box already is. So the copy gives way
// and becomes a boxed array, the same answer a spread of a boxed collection takes.
//
// An any[] or unknown[] source stays out of this. Its elements are already value.Value
// on the ordinary path, so the literal it splices into is a typed array of boxes and
// reading it as a boxed literal would change what that spelling has always built.
func (r *Renderer) spreadOfUnlandableBox(n frontend.Node) bool {
	if n.Kind() != frontend.NodeSpreadElement {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) != 1 || !r.spreadsABox(kids[0]) {
		return false
	}
	elemT, ok := r.prog.ElementType(r.prog.TypeAt(kids[0]))
	if !ok || elemT.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
		return false
	}
	return elemT.Flags&(frontend.TypeNumber|frontend.TypeString|frontend.TypeBoolean) == 0
}

// markBoxedCollections marks the slots of a Set or a Map that some call hands a box, so
// the collection holds its members boxed rather than in the Go element type the checker's
// type argument interns.
//
//	const s = new Set<Row>()
//	s.add(Object.values(m)[0])
//
// value.NewRefSet[*ObjIdTag] keys its members on a Go struct pointer and a box is not one,
// so this was Go that did not build. The rule is the one every other slot takes: the
// collection gives way, since a box cannot be brought to the struct without a copy that
// aliases nothing, while a static member boxes on its way in.
//
// The mark is on the collection type rather than on any one expression because a Go
// collection has one element type, and the type is what both spellings of it read: the
// constructor that mints it and the Go type of every variable that holds it. A program
// with two unrelated Set<Row> gets one answer for both, which is coarser than it needs to
// be and is still right, since a static member has somewhere to go in the boxed set and a
// box has nowhere to go in the static one.
//
// Only a shape is marked. A member the checker already calls any or unknown holds a box
// already, and one it calls a number, a string, or a boolean has a Go value the box comes
// down to through the coercion that already runs.
func (r *Renderer) markBoxedCollections(files []frontend.Node) bool {
	changed := false
	r.walkInClasses(files, func(n frontend.Node) bool {
		// A collection built from a boxed iterable holds boxes from the start, since the
		// runtime drains such a source into value.Value elements and there is no shape to
		// bring them to. `new Set<Row>(Object.values(m))` is the everyday spelling.
		if coll, iter, ok := r.collFromIterable(n); ok {
			if r.isBoxedChain(iter) && r.isSetType(coll) &&
				r.markCollSlot(&r.boxedSetElems, coll, r.setElemRaw) {
				changed = true
			}
			// A Map built from a literal of pairs fills itself with one Set per pair, so each
			// pair's two halves land in the same two slots a written set() fills.
			if r.isMapType(coll) && iter.Kind() == frontend.NodeArrayLiteralExpression {
				for _, pair := range r.prog.Children(iter) {
					kv := r.prog.Children(pair)
					if pair.Kind() != frontend.NodeArrayLiteralExpression || len(kv) != 2 {
						continue
					}
					if r.isBoxedChain(kv[0]) && r.markCollSlot(&r.boxedMapKeys, coll, r.mapKeyRaw) {
						changed = true
					}
					if r.isBoxedChain(kv[1]) && r.markCollSlot(&r.boxedMapVals, coll, r.mapValRaw) {
						changed = true
					}
				}
			}
		}
		recvNode, method, argNodes, ok := r.methodCallParts(n)
		if !ok {
			return true
		}
		switch {
		// A WeakSet and a WeakMap take the same three and four method names against the
		// same two slot readers, so each is marked beside the strong collection it mirrors.
		// Whether the runtime can hold a boxed member weakly is renderWeakSet's own
		// question, and it answers with a hand-back.
		case r.isSet(recvNode) || r.isWeakSet(recvNode):
			// add, has, and delete each take one member, so a boxed argument to any of them
			// is a box the set has to hold: has and delete compare against what is stored,
			// which only answers right if the stored form is the boxed one.
			switch method {
			case "add", "has", "delete":
			default:
				return true
			}
			if len(argNodes) != 1 || !r.isBoxedChain(argNodes[0]) {
				return true
			}
			if r.markCollSlot(&r.boxedSetElems, r.prog.TypeAt(recvNode), r.setElemRaw) {
				changed = true
			}
		case r.isMap(recvNode) || r.isWeakMap(recvNode):
			// set takes a key then a value; get, has, and delete take a key alone. A boxed
			// argument marks the slot it fills, and the two slots are decided apart because
			// a Map<Row, number> boxes its keys and keeps its values float64.
			var keyed, valued bool
			switch method {
			case "set":
				keyed, valued = len(argNodes) > 0, len(argNodes) > 1
			case "get", "has", "delete":
				keyed = len(argNodes) > 0
			default:
				return true
			}
			if keyed && r.isBoxedChain(argNodes[0]) {
				if r.markCollSlot(&r.boxedMapKeys, r.prog.TypeAt(recvNode), r.mapKeyRaw) {
					changed = true
				}
			}
			if valued && r.isBoxedChain(argNodes[1]) {
				if r.markCollSlot(&r.boxedMapVals, r.prog.TypeAt(recvNode), r.mapValRaw) {
					changed = true
				}
			}
		}
		return true
	})
	return changed
}

// markCollSlot records that one slot of a collection type holds a box, and reports whether
// that was news. The slot's own type comes from the raw reader, the one that does not
// consult these marks, so the shape test reads what the checker wrote rather than what an
// earlier round already rewrote.
func (r *Renderer) markCollSlot(marks *map[int]bool, coll frontend.Type, raw func(frontend.Type) (frontend.Type, bool)) bool {
	slot, ok := raw(coll)
	if !ok {
		return false
	}
	const settled = frontend.TypeAny | frontend.TypeUnknown |
		frontend.TypeNumber | frontend.TypeString | frontend.TypeBoolean
	if slot.Flags&settled != 0 {
		return false
	}
	id := coll.Identity()
	if (*marks)[id] {
		return false
	}
	if *marks == nil {
		*marks = map[int]bool{}
	}
	(*marks)[id] = true
	return true
}

// methodCallParts splits a plain method call, recv.name(args), into its three parts. It is
// the shape the collection walk above looks for and nothing else: a call whose callee is a
// property access with a receiver and a name.
func (r *Renderer) methodCallParts(n frontend.Node) (frontend.Node, string, []frontend.Node, bool) {
	if n.Kind() != frontend.NodeCallExpression {
		return nil, "", nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodePropertyAccessExpression {
		return nil, "", nil, false
	}
	callee := r.prog.Children(kids[0])
	if len(callee) != 2 || callee[1].Kind() != frontend.NodeIdentifier {
		return nil, "", nil, false
	}
	return callee[0], r.prog.Text(callee[1]), kids[1:], true
}

// markBoxedCallbackParams marks the parameters of an inline callback that a dynamic call
// hands a box, so the pass sees what the lowering already does.
//
// A callback going into a call on a box is handed its arguments boxed, and note 383's
// forceCallbackDynParams gives every parameter that cannot hold one a value.Value slot
// instead. That happens as the callback lowers, which is long after this pass has decided,
// so `Object.values(m).map((r: Row) => label(r))` had a box in `r` that nothing on this
// side knew about and label kept a parameter the box could not fill.
//
// The condition is forceCallbackDynParams' own, read here rather than there: the receiver
// is a box, so the call dispatches through the value model, and a parameter takes the value
// slot unless it is one of the primitives a box coerces into. Only the symbol is recorded.
// The callback's own Go signature is not this pass's to write, since the closure renders
// from the forced set; what the pass needs is only that a read of the name answers as a box.
func (r *Renderer) markBoxedCallbackParams(files []frontend.Node) bool {
	changed := false
	r.walkInClasses(files, func(n frontend.Node) bool {
		if n.Kind() != frontend.NodeCallExpression {
			return true
		}
		kids := r.prog.Children(n)
		if len(kids) < 2 || kids[0].Kind() != frontend.NodePropertyAccessExpression {
			return true
		}
		recv := r.prog.Children(kids[0])
		if len(recv) != 2 || !r.isBoxedChain(recv[0]) {
			return true
		}
		for _, a := range kids[1:] {
			if a.Kind() != frontend.NodeArrowFunction && a.Kind() != frontend.NodeFunctionExpression {
				continue
			}
			for _, pn := range r.funcParamNodes(a) {
				pkids := r.prog.Children(pn)
				if len(pkids) == 0 || pkids[0].Kind() != frontend.NodeIdentifier {
					continue
				}
				if r.takesADynamicArg(r.prog.TypeAt(pkids[0])) {
					continue
				}
				sym, ok := r.prog.SymbolAt(pkids[0])
				if !ok || r.boxedParamSyms[sym] {
					continue
				}
				r.boxedParamSyms[sym] = true
				changed = true
			}
		}
		return true
	})
	return changed
}

// markBoxedFields decides which class fields hold a box, and joins the fixpoint because
// the three answers feed each other: a method that hands back a box fills a field, a
// field that holds one is read into a call, and that call's parameter takes the value
// slot in turn.
//
// A field is boxed when some store hands it a box. That is the field's reading of the
// rule the parameter and the result take: a Go struct field has one type, and of the two
// candidates the box is the only one the other stores can be brought to, since a static
// value boxes on its way in through the coercion that already runs where a box has no way
// to become the struct.
//
// A hierarchy is allowed here, where note 387 had to stop for a method. A field's Go type
// is written into the struct of the class that declares it and nowhere else, since a
// derived struct embeds its base and reaches the field through Go's own promotion, and
// registration rejects a derived member sharing a base member's name, so at most one class
// on a chain owns any property. The one-place condition the whole pass rests on therefore
// holds for a field however deep the chain is; it is the vtable and the interface a method
// is written into again that this pass still cannot rewrite.
//
// A private field is left out because its own boxing rule already exists for the static
// case, and a synthesized Error field has no declared type to rewrite.
func (r *Renderer) markBoxedFields(files []frontend.Node) bool {
	if len(r.classes) == 0 {
		return false
	}
	changed := false
	for _, info := range r.classes {
		for _, f := range info.fields {
			if f.ident == nil || f.synthBStr || strings.HasPrefix(f.prop, "#") {
				continue
			}
			if r.boxedFields[f.ident] || !r.fieldTakesABox(files, info, f) {
				continue
			}
			r.boxedFields[f.ident] = true
			changed = true
		}
	}
	return changed
}

// markBoxedLocals marks a local binding some assignment hands a box, so its Go slot is a
// value.Value however the checker types the declaration.
//
// This is the field's rule read at a binding. Note 384 already gave a binding the value
// slot when its own initializer was a box; what was left is the box that arrives later.
//
//	let cur: Row = { id: 0, tag: 'z' }
//	cur = m['b']
//
// The declaration says Row and the store hands back a value.Value, and there is one Go
// variable to hold both. Only the box is a candidate the other side can be brought to: the
// literal boxes on its way in through the coercion that already runs, and the box has no
// way to become the struct without a copy that aliases nothing.
//
// A local passes the one-place condition the way a field does. Its Go type is written into
// exactly one declaration, and every read of the name resolves to that one binding, so
// there is no second spelling to keep in step. The store itself can sit anywhere, which is
// why this is decided here rather than by looking at the declaration alone: the store that
// boxes `seen` is inside a callback in
//
//	Object.values(m).forEach((r: Row) => { seen = r })
//
// and the callback's parameter is only known to hold a box because a round of this same
// fixpoint said so.
func (r *Renderer) markBoxedLocals(files []frontend.Node) bool {
	changed := false
	r.walkInClasses(files, func(n frontend.Node) bool {
		target, value, ok := r.localStoreParts(n)
		if !ok {
			return true
		}
		sym, ok := r.prog.SymbolAt(target)
		if !ok || r.boxedLocals[sym] || !r.boxableLocal(sym) {
			return true
		}
		if r.isBoxedChain(value) {
			r.boxedLocals[sym] = true
			changed = true
		}
		return true
	})
	return changed
}

// localStoreParts splits a plain assignment to a bare name into its target and its value.
// A compound assignment is left out the way it is for a field: `cur += x` reads the slot
// before it writes it, so it is an operator's question rather than a store's.
func (r *Renderer) localStoreParts(n frontend.Node) (frontend.Node, frontend.Node, bool) {
	if n.Kind() != frontend.NodeBinaryExpression {
		return nil, nil, false
	}
	parts := r.prog.Children(n)
	if len(parts) != 3 || strings.TrimSpace(r.prog.Text(parts[1])) != "=" {
		return nil, nil, false
	}
	if parts[0].Kind() != frontend.NodeIdentifier {
		return nil, nil, false
	}
	return parts[0], parts[2], true
}

// boxableLocal reports whether a symbol names a binding this pass may move into the value
// slot: one variable declaration and nothing else.
//
// A second declaration would mean two Go slots for one name and the one-place condition
// would not hold. A parameter is left out because its own half of the pass decides it from
// the call sites, which is a stronger reading than a store inside the body.
//
// The type test is boxedChainBinding's, read here so the walk does not mark a binding that
// path would then decline. One the checker already calls any or unknown holds a box
// already, and one it calls a number, a string, or a boolean has a Go value the store can
// be brought down to through the coercion that already runs.
func (r *Renderer) boxableLocal(sym frontend.Symbol) bool {
	decls := r.prog.Declarations(sym)
	if len(decls) != 1 || decls[0].Kind() != frontend.NodeVariableDeclaration {
		return false
	}
	kids := r.prog.Children(decls[0])
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return false
	}
	const settled = frontend.TypeAny | frontend.TypeUnknown |
		frontend.TypeNumber | frontend.TypeString | frontend.TypeBoolean
	return r.prog.TypeAt(kids[0]).Flags&settled == 0
}

// markBoxedLoopVars marks the binding of a for...of whose iterable is a box, so the pass
// sees the box the loop lowering already binds.
//
//	for (const r of Object.values(m)) { cur = r }
//
// The iterable is a box, so what comes out of it is a box, and forOfDynamic marks the
// binding dynBound before it lowers the body for exactly that reason. That mark is made
// while the loop lowers, which is after this pass has run, so nothing here saw it and a
// store fed by the loop variable read as a store of the checker's shape.
//
// The two tests are the two the lowering applies in sequence, isBoxedChain in lowerForOf
// and then forOfDynamic's own, so a loop marked here is a loop that really does take the
// dynamic path. A loop that takes one of the static paths binds a Go value and nothing
// about it is boxed.
//
// This joins the fixpoint rather than running once because the iterable can be a box for a
// reason an earlier round established, a call of a function the pass gave a boxed result
// among them. A destructuring head is left out; its names bind through a different path
// and are their own slice.
func (r *Renderer) markBoxedLoopVars(files []frontend.Node) bool {
	changed := false
	r.walkInClasses(files, func(n frontend.Node) bool {
		bind, iterable, ok := r.forOfSingleBinding(n)
		if !ok {
			return true
		}
		// A Set whose members the pass boxed yields a box each turn the same way a boxed
		// iterable does, which rangeCollSingle is what tells the body. The Set itself is not
		// a boxed chain, so it is read on its own here.
		if !r.boxedSetElem(iterable) {
			if !r.isBoxedChain(iterable) {
				return true
			}
			if !r.isDynamic(iterable) && !r.producesBoxedValue(iterable) {
				return true
			}
		}
		sym, ok := r.prog.SymbolAt(bind)
		if !ok || r.boxedLoopVars[sym] {
			return true
		}
		if r.boxedLoopVars == nil {
			r.boxedLoopVars = map[frontend.Symbol]bool{}
		}
		r.boxedLoopVars[sym] = true
		changed = true
		return true
	})
	return changed
}

// forOfSingleBinding answers the name a for...of binds and the expression it iterates,
// for the plain head that declares one identifier. It is lowerForOf's own reading of the
// statement's children, kept to the single-binding shape forOfDynamic handles.
func (r *Renderer) forOfSingleBinding(n frontend.Node) (frontend.Node, frontend.Node, bool) {
	if n.Kind() != frontend.NodeForOfStatement || r.isForAwait(n) {
		return nil, nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) != 3 {
		return nil, nil, false
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, kids[0], &decls)
	if len(decls) != 1 {
		return nil, nil, false
	}
	dkids := r.prog.Children(decls[0])
	if len(dkids) != 1 || dkids[0].Kind() != frontend.NodeIdentifier {
		return nil, nil, false
	}
	return dkids[0], kids[1], true
}

// isBoxedLoopVar reports whether an identifier names a for...of binding the pass found
// iterating a box.
func (r *Renderer) isBoxedLoopVar(n frontend.Node) bool {
	if len(r.boxedLoopVars) == 0 || n.Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(n)
	return ok && r.boxedLoopVars[sym]
}

// isBoxedLocalRead reports whether an identifier names a local this pass gave a value.Value
// slot, so a read of it is a box however the checker types the declaration.
func (r *Renderer) isBoxedLocalRead(n frontend.Node) bool {
	if len(r.boxedLocals) == 0 || n.Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(n)
	return ok && r.boxedLocals[sym]
}

// fieldTakesABox reports whether any store to a field hands it a box: its own declared
// initializer, a this.f = v inside the class it belongs to or any class below it, or a
// recv.f = v anywhere the receiver reaches that field.
//
// Both sides of a store need the class `this` names: the receiver of `this.f = ...` to
// know the store is this field's, and the value of `... = this.head()` to know a sibling
// method handed it a box. The walk carries it through curClass, which is what classReceiver
// reads, so the declaration supplies what the receiver does not.
//
// The receiver is matched by what its property resolves to rather than by the class
// itself, so a store written in a derived class, or through a derived receiver, lands on
// the base's field the way Go's promotion lands it.
func (r *Renderer) fieldTakesABox(files []frontend.Node, info *classInfo, f classField) bool {
	if f.init != nil {
		prev := r.curClass
		r.curClass = info
		boxed := r.isBoxedChain(f.init)
		r.curClass = prev
		if boxed {
			return true
		}
	}
	found := false
	r.walkInClasses(files, func(n frontend.Node) bool {
		if found {
			return false
		}
		lhs, rhs, ok := r.propStoreParts(n)
		if !ok || r.prog.Text(r.prog.Children(lhs)[1]) != f.prop {
			return true
		}
		if r.storeReaches(r.prog.Children(lhs)[0], f) && r.isBoxedChain(rhs) {
			found = true
			return false
		}
		return true
	})
	return found
}

// storeReaches reports whether a store's receiver names the field f, following the base
// chain the way a read of the property does.
func (r *Renderer) storeReaches(recv frontend.Node, f classField) bool {
	info, ok := r.classReceiver(recv)
	if !ok {
		return false
	}
	got, ok := info.lookupField(f.prop)
	return ok && got.ident == f.ident
}

// walkInClasses visits every node of the program with curClass set to whatever class the
// node sits inside, so `this` resolves during the pass the way it will once the body
// lowers. The visit returns false to stop descending.
//
// The pass needs this wherever a shape it decides can be written with `this`: a field read
// or a sibling call is only recognizable as a box once the receiver has a class, and those
// appear in returns and in arguments as readily as in stores.
func (r *Renderer) walkInClasses(files []frontend.Node, visit func(frontend.Node) bool) {
	byDecl := make(map[frontend.Node]*classInfo, len(r.classes))
	for _, info := range r.classes {
		if info.decl != nil {
			byDecl[info.decl] = info
		}
	}
	var walk func(frontend.Node)
	walk = func(n frontend.Node) {
		if !visit(n) {
			return
		}
		if info, ok := byDecl[n]; ok {
			prev := r.curClass
			r.curClass = info
			for _, c := range r.prog.Children(n) {
				walk(c)
			}
			r.curClass = prev
			return
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	for _, file := range files {
		walk(file)
	}
}

// propStoreParts recognizes a plain `recv.prop = value` assignment expression and hands
// back its target member access and its value. A compound store is not one: it reads the
// field first, so what it writes is whatever the operator produced rather than the value
// as written.
func (r *Renderer) propStoreParts(n frontend.Node) (frontend.Node, frontend.Node, bool) {
	if n.Kind() != frontend.NodeBinaryExpression {
		return nil, nil, false
	}
	parts := r.prog.Children(n)
	if len(parts) != 3 || strings.TrimSpace(r.prog.Text(parts[1])) != "=" {
		return nil, nil, false
	}
	if parts[0].Kind() != frontend.NodePropertyAccessExpression || len(r.prog.Children(parts[0])) != 2 {
		return nil, nil, false
	}
	return parts[0], parts[2], true
}

// readOfBoxedField reports whether a property read reaches a field this pass boxed, so
// the read is a value.Value however the checker types the declaration.
func (r *Renderer) readOfBoxedField(n frontend.Node) bool {
	if len(r.boxedFields) == 0 || n.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 || kids[1].Kind() != frontend.NodeIdentifier {
		return false
	}
	info, ok := r.classReceiver(kids[0])
	if !ok {
		return false
	}
	f, ok := info.lookupField(r.prog.Text(kids[1]))
	return ok && f.ident != nil && r.boxedFields[f.ident]
}

// boxableFunc is one function declaration this pass may rewrite, held with the pieces
// every round needs so they are looked up once rather than per round.
type boxableFunc struct {
	sym    frontend.Symbol
	sig    frontend.Signature
	params []frontend.Node
	// owner is the class a member candidate is declared in, so the return scan can set
	// curClass and a `return this.first` resolves the field it reads.
	owner *classInfo
}

// boxableFuncs collects the function declarations whose signature this pass is allowed
// to rewrite.
//
// A named function is a candidate whether it was written as a `function` declaration or
// bound to a name as an arrow or a function expression, since both lower from one
// signature read at one place. addBoxableMethods adds the class members that meet the
// same condition.
//
// A symbol referenced anywhere but as the callee of a direct call is left alone as
// well. Rewriting the parameter changes the function's Go type, so a reference that
// passes it as a value (`rows.map(pick)`) would hand a func of the new shape to a slot
// expecting the old one. Every call site is rewritten together by construction; a value
// use is not something this pass can rewrite, so it disqualifies the function instead.
//
// A generic function is skipped because its specializations each lower from their own
// substituted signature, and an overload set because the implementation signature is
// not the one a call site resolves against.
func (r *Renderer) boxableFuncs(files []frontend.Node) map[frontend.Node]boxableFunc {
	out := map[frontend.Node]boxableFunc{}
	var walk func(frontend.Node)
	walk = func(n frontend.Node) {
		switch n.Kind() {
		case frontend.NodeFunctionDeclaration:
			if sym, ok := r.prog.SymbolAt(n); ok {
				r.addBoxableFunc(out, n, sym)
			}
		case frontend.NodeVariableDeclaration:
			// A const-bound arrow or function expression is the other shape a named
			// function takes. It lowers as a Go func literal whose fields come from its own
			// signature, which is the one place the overlay has to land, so it is a
			// candidate on the same terms as a declaration. The name it is bound to carries
			// the symbol every call site resolves through.
			kids := r.prog.Children(n)
			if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
				break
			}
			sym, ok := r.prog.SymbolAt(kids[0])
			if !ok {
				break
			}
			if fn, ok := r.declBoundFunc(n); ok && fn.Kind() != frontend.NodeFunctionDeclaration {
				r.addBoxableFunc(out, fn, sym)
			}
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	for _, f := range files {
		walk(f)
	}
	r.addBoxableMethods(out)
	return out
}

// addBoxableMethods adds the methods this pass may rewrite. Classes register before it
// runs, so the receiver of a call resolves to the class that declares the method the
// same way the call lowering resolves it.
//
// A method is a candidate only when its Go signature is written in exactly one place.
// That rules out an instance method in a hierarchy: an override, an abstract
// declaration, or any class with a base or a subclass, since those spell the signature
// again at the vtable and at the interface every derived receiver is called through. A
// static method carries none of that, being a package function the class name routes to
// and not inherited in this slice, so every class offers its statics.
//
// Both kinds still have to clear boxableMethodSig: an async or generator method hands
// back a promise or a coroutine rather than what a return carries, a generic method
// lowers once per specialization from its own substituted signature, and a name some
// expression reads as a value rather than calls takes the method's Go type the rewrite
// would be changing.
func (r *Renderer) addBoxableMethods(out map[frontend.Node]boxableFunc) {
	if len(r.classes) == 0 {
		return
	}
	derived := map[*classInfo]bool{}
	for _, info := range r.classes {
		if info.base != nil {
			derived[info.base] = true
		}
	}
	for _, info := range r.classes {
		for _, m := range info.staticMethods {
			r.addBoxableMethod(out, info, m)
		}
		for _, g := range info.staticGetters {
			r.addBoxableGetter(out, info, g)
		}
		// A constructor is a package function written once per class, with no vtable entry
		// and no interface, so it takes the rewrite in a hierarchy the way a field does.
		r.addBoxableCtor(out, info)
		if info.base != nil || derived[info] {
			continue
		}
		for _, m := range info.methods {
			if info.isVirtual(m.prop) || m.abstract {
				continue
			}
			r.addBoxableMethod(out, info, m)
		}
		for _, g := range info.getters {
			if info.isVirtual(g.prop) {
				continue
			}
			r.addBoxableGetter(out, info, g)
		}
	}
}

// addBoxableCtor records a constructor's parameters as candidates. A constructor's whole
// job is moving its arguments into fields, so it is the half of the field slice that faces
// the call site: `new S(box)` has nowhere to put the box until the parameter holds one.
//
// Only the parameter half can apply. A constructor hands back the receiver rather than
// what a return carries, and ctorBody hands back on any return statement at all, so no
// result mark can arise to be read.
//
// The eligibility is the field's, a class with no base and no subclass, which the caller
// has already checked: the parameter's Go type is written into one function, and the field
// it is stored into is written into one struct.
func (r *Renderer) addBoxableCtor(out map[frontend.Node]boxableFunc, info *classInfo) {
	if info.ctor == nil {
		return
	}
	sig, ok := r.prog.SignatureAt(info.ctor)
	if !ok || len(sig.TypeParams) != 0 || sig.RestParam != nil {
		return
	}
	out[info.ctor] = boxableFunc{sig: sig, params: info.ctorParams, owner: info}
}

// addBoxableGetter records a getter as a candidate. A getter emits through the method
// path and takes no parameters, so only the return half of the rewrite can apply, and it
// applies on the same terms: a body that hands back a box gives the getter a value.Value
// result and the read off it dispatches.
//
// It skips the read-as-a-value check the other two take, because a getter has no shape a
// program can read without calling it. Reading `s.head` is how a getter is invoked, so
// that check would match the getter's own use and leave every getter alone.
func (r *Renderer) addBoxableGetter(out map[frontend.Node]boxableFunc, owner *classInfo, g classMethod) {
	sig, ok := r.prog.SignatureAt(g.node)
	if !ok || len(sig.TypeParams) != 0 {
		return
	}
	out[g.node] = boxableFunc{sig: sig, owner: owner}
}

// addBoxableMethod records one method as a candidate when its Go signature is one this
// pass can rewrite in place. The property-name check is across the whole program, so an
// unrelated object's property of the same name is enough to leave a method alone, which
// costs a rewrite this pass could have made but never makes a wrong one.
func (r *Renderer) addBoxableMethod(out map[frontend.Node]boxableFunc, owner *classInfo, m classMethod) {
	if r.isAsyncFunc(m.node) || r.isGeneratorFunc(m.node) {
		return
	}
	sig, ok := r.prog.SignatureAt(m.node)
	if !ok || len(sig.TypeParams) != 0 || sig.RestParam != nil {
		return
	}
	if r.propReadAsValue(m.prop) {
		return
	}
	out[m.node] = boxableFunc{sig: sig, params: r.funcParamNodes(m.node), owner: owner}
}

// propReadAsValue reports whether any member access to a property name in the program is
// something other than the callee of a call, `rows.map(s.take)` being the shape that
// matters here. Such a read takes the method's Go type, which this pass would be
// rewriting, so one is enough to leave the method alone.
func (r *Renderer) propReadAsValue(prop string) bool {
	found := false
	var walk func(n frontend.Node, isCallee bool)
	walk = func(n frontend.Node, isCallee bool) {
		if found {
			return
		}
		kids := r.prog.Children(n)
		if n.Kind() == frontend.NodePropertyAccessExpression && len(kids) == 2 &&
			kids[1].Kind() == frontend.NodeIdentifier && r.prog.Text(kids[1]) == prop && !isCallee {
			found = true
			return
		}
		for i, c := range kids {
			walk(c, n.Kind() == frontend.NodeCallExpression && i == 0)
		}
	}
	for _, f := range r.prog.SourceFiles() {
		walk(f, false)
	}
	return found
}

// declBoundFunc returns the function a declaration binds its name to, the arrow or
// function expression the initializer itself is. Only parentheses and a type assertion
// sit between a name and the function it names, so those are the only wrappers it looks
// through. declValueFunc, which this used to call, finds the first function-like node
// anywhere under the declaration; for `const out = xs.map((r) => r.tag)` that is the
// inline callback, and treating it as the function `out` names decides the callback by
// how the binding is used, which is a fact about a string.
func (r *Renderer) declBoundFunc(d frontend.Node) (frontend.Node, bool) {
	var none frontend.Node
	kids := r.prog.Children(d)
	// A binding is [name], [name, type], [name, initializer], or [name, type,
	// initializer], so an initializer is a last child past the name carrying a real
	// expression kind rather than an unclassified type annotation.
	if len(kids) < 2 {
		return none, false
	}
	init := kids[len(kids)-1]
	if init.Kind() == frontend.NodeUnknown {
		return none, false
	}
	for {
		peeled := r.unwrapParens(r.assertionOperand(init))
		if peeled == init {
			break
		}
		init = peeled
	}
	if !isFunctionLike(init.Kind()) {
		return none, false
	}
	return init, true
}

func (r *Renderer) addBoxableFunc(out map[frontend.Node]boxableFunc, fn frontend.Node, sym frontend.Symbol) {
	sig, ok := r.prog.SignatureAt(fn)
	if !ok || len(sig.TypeParams) != 0 || sig.RestParam != nil {
		r.markUnclaimedFunc(fn)
		return
	}
	if len(r.prog.Declarations(sym)) != 1 {
		r.markUnclaimedFunc(fn)
		return
	}
	if r.funcSymValueUsed(sym) {
		r.markUnclaimedFunc(fn)
		return
	}
	out[fn] = boxableFunc{sym: sym, sig: sig, params: r.funcParamNodes(fn)}
}

// markUnclaimedFunc records a named function this pass looked at and did not take.
// Whatever the reason, its signature stays as the checker wrote it, so a body that
// hands back a box has nowhere to put it. arrowFunc reads this set to tell such an arrow
// apart from an inline callback, whose result comes from the slot it is passed to rather
// than from its own signature and which this pass never considers.
func (r *Renderer) markUnclaimedFunc(fn frontend.Node) {
	if r.unclaimedFuncs == nil {
		r.unclaimedFuncs = map[frontend.Node]bool{}
	}
	r.unclaimedFuncs[fn] = true
}

// funcSymValueUsed reports whether any reference to a function symbol is something
// other than the callee of a direct call: passed as a value, read through a member such
// as f.call, or boxed. Such a reference reads the function's Go type, which this pass
// would be changing out from under it, so one is enough to leave the function alone.
// The name a declaration binds is the definition rather than a use and is skipped.
func (r *Renderer) funcSymValueUsed(sym frontend.Symbol) bool {
	used := false
	decls := map[frontend.Node]bool{}
	for _, d := range r.prog.Declarations(sym) {
		decls[d] = true
	}
	refersToSym := func(n frontend.Node) bool {
		if n.Kind() != frontend.NodeIdentifier {
			return false
		}
		s, ok := r.prog.SymbolAt(n)
		return ok && r.derefAlias(s) == sym
	}
	var walk func(n frontend.Node, isCallee bool)
	walk = func(n frontend.Node, isCallee bool) {
		if used {
			return
		}
		if n.Kind() == frontend.NodeIdentifier {
			if refersToSym(n) && !isCallee {
				used = true
			}
			return
		}
		kids := r.prog.Children(n)
		if n.Kind() == frontend.NodeCallExpression && len(kids) >= 1 && refersToSym(kids[0]) {
			for _, a := range kids[1:] {
				walk(a, false)
			}
			return
		}
		isDecl := decls[n]
		for _, c := range kids {
			if isDecl && refersToSym(c) {
				continue
			}
			walk(c, false)
		}
	}
	for _, f := range r.prog.SourceFiles() {
		walk(f, false)
	}
	return used
}

// markBoxedParams walks every call in the program and marks the parameter a boxed
// argument lands on. It reports whether it added a mark, which is what drives the
// fixpoint.
func (r *Renderer) markBoxedParams(files []frontend.Node, cands map[frontend.Node]boxableFunc) bool {
	changed := false
	r.walkInClasses(files, func(n frontend.Node) bool {
		if fn, ok := r.superCtorNode(n); ok {
			if c, isCand := cands[fn]; isCand {
				for i, a := range r.prog.Children(n)[1:] {
					if i >= len(c.sig.Params) {
						break
					}
					if !r.boxableParamSlot(c.sig.Params[i]) || !r.isBoxedChain(a) {
						continue
					}
					if r.markBoxedParam(fn, c, i) {
						changed = true
					}
				}
			}
		}
		if fn, ok := r.newCtorNode(n); ok {
			if c, isCand := cands[fn]; isCand {
				for i, a := range r.prog.Children(n)[1:] {
					if i >= len(c.sig.Params) {
						break
					}
					if !r.boxableParamSlot(c.sig.Params[i]) || !r.isBoxedChain(a) {
						continue
					}
					if r.markBoxedParam(fn, c, i) {
						changed = true
					}
				}
			}
		}
		if n.Kind() == frontend.NodeCallExpression {
			if fn, ok := r.calleeFuncNode(n); ok {
				if c, isCand := cands[fn]; isCand {
					args := r.prog.Children(n)[1:]
					for i, a := range args {
						if i >= len(c.sig.Params) {
							break
						}
						if !r.boxableParamSlot(c.sig.Params[i]) || !r.isBoxedChain(a) {
							continue
						}
						if r.markBoxedParam(fn, c, i) {
							changed = true
						}
					}
				}
			}
		}
		return true
	})
	return changed
}

// boxableParamSlot reports whether a parameter is one a box can be moved into.
//
// A parameter the checker already types any or unknown holds the box as it is, so
// there is nothing to rewrite. One typed number, string, or boolean has a Go value the
// box comes down to through the ordinary coercion, which is the same answer a read off
// a box gives, so it keeps its static slot. What is left is the shapes, an object, an
// array, a tuple, a class instance, and none of them can be built from a box. This is
// boxedChainBinding's rule read at a parameter rather than at a binding.
func (r *Renderer) boxableParamSlot(p frontend.Param) bool {
	const holds = frontend.TypeAny | frontend.TypeUnknown |
		frontend.TypeNumber | frontend.TypeString | frontend.TypeBoolean
	return p.Type.Flags != 0 && p.Type.Flags&holds == 0
}

func (r *Renderer) markBoxedParam(fn frontend.Node, c boxableFunc, i int) bool {
	marks := r.boxedParams[fn]
	if marks == nil {
		marks = make([]bool, len(c.sig.Params))
		r.boxedParams[fn] = marks
	}
	if marks[i] {
		return false
	}
	marks[i] = true
	// The parameter's own symbol joins the boxed set so a read of the name inside the
	// body answers isBoxedChain in the next round, which is what lets `inner(r)` be seen
	// as passing a box once outer's r is one.
	if i < len(c.params) {
		// The name is looked up past any leading modifier so a constructor's parameter
		// property, where the name node is also the field's, joins the set too.
		for _, k := range r.prog.Children(c.params[i]) {
			if k.Kind() != frontend.NodeIdentifier {
				continue
			}
			if sym, ok := r.prog.SymbolAt(k); ok {
				r.boxedParamSyms[sym] = true
			}
			break
		}
	}
	return true
}

// markBoxedReturns marks a function that hands back a box on any path, so its Go
// result is a value.Value and a call to it is itself a box.
//
// An async or generator body is left alone: what its Go func hands back is the promise
// or the coroutine rather than the value a return carries, so the result type is not
// this pass's to rewrite.
func (r *Renderer) markBoxedReturns(cands map[frontend.Node]boxableFunc) bool {
	changed := false
	for fn, c := range cands {
		if r.boxedReturnFns[fn] || !r.boxableReturnSlot(c.sig.Return) {
			continue
		}
		if r.isAsyncFunc(fn) || r.isGeneratorFunc(fn) {
			continue
		}
		if !r.returnsBoxedIn(c.owner, fn) {
			continue
		}
		r.boxedReturnFns[fn] = true
		changed = true
	}
	return changed
}

// returnsBoxedIn asks funcReturnsBoxedChain with the declaring class in hand, so a member
// whose body says `return this.first` resolves the receiver the same way the body will
// once it lowers. A plain function has no owner and asks the question as it stands.
func (r *Renderer) returnsBoxedIn(owner *classInfo, fn frontend.Node) bool {
	if owner == nil {
		return r.funcReturnsBoxedChain(fn)
	}
	prev := r.curClass
	r.curClass = owner
	boxed := r.funcReturnsBoxedChain(fn)
	r.curClass = prev
	return boxed
}

// boxableReturnSlot is boxableParamSlot for the result position. A declared return the
// checker types number, string, or boolean is left alone for the same reason: the
// return coercion brings the box down to the Go primitive, which is what a read off a
// box does anyway.
func (r *Renderer) boxableReturnSlot(t frontend.Type) bool {
	return r.boxableParamSlot(frontend.Param{Type: t})
}

// funcReturnsBoxedChain reports whether a function hands back a box on any path.
//
// One such path settles it for the whole function, because a Go function has one result
// type and a box is the only one of the two the other returns can be brought to: a
// struct boxes on its way out through the ordinary return coercion, where a box has no
// way to become the struct. That is the parameter half's rule read at the result, where
// the static literal argument boxes into the slot the boxed call site decided.
//
// A body with no return at all hands back undefined and is not this shape. A concise
// arrow has no return statement to read, so its single body expression is the path.
func (r *Renderer) funcReturnsBoxedChain(fn frontend.Node) bool {
	if block, ok := r.funcBodyBlock(fn); ok {
		for _, ret := range r.returnsOfBody(block) {
			kids := r.prog.Children(ret)
			if len(kids) == 1 && r.returnHandsBackABox(kids[0]) {
				return true
			}
		}
		return false
	}
	kids := r.prog.Children(fn)
	if len(kids) < 2 {
		return false
	}
	body := kids[len(kids)-1]
	return body.Kind() != frontend.NodeBlock && r.returnHandsBackABox(body)
}

// returnHandsBackABox reports whether a returned expression hands back a box.
//
// It is isBoxedChain plus the shape that writes the disagreement into one expression
// rather than across two returns, `return b ? m[k] : { id: 9, tag: 'z' }`. A returned
// ternary does not lower as one value: flattenConditionalReturn turns it into an if with
// a return in each arm, and each of those coerces to the function's result on its own.
// So the arms are what to ask, and the recursion here is the one conditionalReturnStmts
// takes so a chained ternary answers on any of its arms.
func (r *Renderer) returnHandsBackABox(x frontend.Node) bool {
	x = r.unwrapParens(x)
	if x.Kind() != frontend.NodeConditionalExpression {
		return r.isBoxedChain(x)
	}
	arms := r.prog.Children(x)
	if len(arms) < 3 {
		return false
	}
	for _, arm := range arms[1:] {
		if r.returnHandsBackABox(arm) {
			return true
		}
	}
	return false
}

// forceBoxedParams records each boxed parameter's own name or pattern node in the set a
// boxed callback's parameters already use (note 383). Two lowerings read that set rather
// than the signature: a closure's parameter field, which renders from the parameter
// node's checker type and would otherwise spell the shape the box cannot fill, and a
// destructuring pattern's entry bindings, which read their leaves out of the one boxed
// slot through the dynamic protocol.
//
// A pattern that holds a box reads the same whether it got there from a boxed callback
// or from a call site handing a declared signature a box, so both roots meet in the one
// set. It runs after the fixpoint has settled, since a mark added mid-round would change
// what a later round sees through a different door than the one the fixpoint reasons on.
func (r *Renderer) forceBoxedParams(cands map[frontend.Node]boxableFunc) {
	for fn, marks := range r.boxedParams {
		c := cands[fn]
		for i, boxed := range marks {
			if !boxed || i >= len(c.params) {
				continue
			}
			pkids := r.prog.Children(c.params[i])
			if len(pkids) == 0 {
				continue
			}
			if pkids[0].Kind() != frontend.NodeIdentifier && !r.patternNode(pkids[0]) {
				continue
			}
			if r.forceDynParams == nil {
				r.forceDynParams = map[frontend.Node]bool{}
			}
			r.forceDynParams[pkids[0]] = true
		}
	}
}

// calleeFuncNode resolves a call's callee to the single function declaration it names,
// the one this pass may have rewritten. A callee that is not a plain identifier, or one
// whose symbol has no function declaration, is not a call into a rewritten signature.
func (r *Renderer) calleeFuncNode(n frontend.Node) (frontend.Node, bool) {
	if n.Kind() != frontend.NodeCallExpression {
		return nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) != 0 && kids[0].Kind() == frontend.NodePropertyAccessExpression {
		return r.calleeMethodNode(kids[0])
	}
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return nil, false
	}
	sym, ok := r.prog.SymbolAt(kids[0])
	if !ok {
		return nil, false
	}
	for _, d := range r.prog.Declarations(r.derefAlias(sym)) {
		if d.Kind() == frontend.NodeFunctionDeclaration {
			return d, true
		}
		if fn, ok := r.declValueFunc(d); ok {
			return fn, true
		}
	}
	return nil, false
}

// calleeMethodNode resolves a call's member callee, `s.take(...)`, to the method
// declaration it names, through the receiver's own class the way classMethodCall
// resolves it. A receiver whose checker type is not a registered class, or a name the
// class does not declare as an instance method, is not a call into a method this pass
// rewrites.
//
// The receiver resolves through classReceiver so that `this.head()` inside a sibling
// method finds the class it is written in. While the pass is deciding, that answers only
// where a caller has set curClass, which fieldTakesABox does over the class it is asking
// about; a method's own returns are what mark it, so nothing else needs it there. It
// answers everywhere once a body is lowering, which is when the read off the call has to
// know it is holding a box.
func (r *Renderer) calleeMethodNode(callee frontend.Node) (frontend.Node, bool) {
	kids := r.prog.Children(callee)
	if len(kids) != 2 || kids[1].Kind() != frontend.NodeIdentifier {
		return nil, false
	}
	name := r.prog.Text(kids[1])
	// A receiver naming the class itself, `S.head()`, is the static half: it routes to
	// the package function the static method became, so it is looked up in that list
	// rather than among the instance methods.
	if info, ok := r.classNameRef(kids[0]); ok {
		if m, ok := info.staticMethodByName(name); ok {
			return m.node, true
		}
	}
	info, ok := r.classReceiver(kids[0])
	if !ok {
		return nil, false
	}
	m, ok := info.lookupMethod(name)
	if !ok {
		return nil, false
	}
	return m.node, true
}

// boxedSig applies this pass's decision to a function's signature: a parameter a call
// site hands a box to reads as any, which is the type whose Go slot is value.Value and
// whose call-site bridging boxes. Everything downstream (the parameter field, the
// dynamic-locals set the body reads through, the argument coercion) picks its answer
// from the type, so rewriting the type here is the whole of the change.
//
// The result reads as any on the same terms once the function is known to hand back a
// box, which is what gives an arrow and a function expression the value.Value result
// their block body renders straight from the signature. A `function` declaration reaches
// the same answer through funcDeclNamed, beside the growing-object override it already
// carries.
func (r *Renderer) boxedSig(fn frontend.Node, sig frontend.Signature) frontend.Signature {
	marks := r.boxedParams[fn]
	if len(marks) == 0 && !r.boxedReturnFns[fn] {
		return sig
	}
	if len(marks) != 0 {
		params := make([]frontend.Param, len(sig.Params))
		copy(params, sig.Params)
		for i := range params {
			if i < len(marks) && marks[i] {
				params[i].Type = frontend.Type{Flags: frontend.TypeAny}
			}
		}
		sig.Params = params
	}
	if r.boxedReturnFns[fn] {
		sig.Return = frontend.Type{Flags: frontend.TypeAny}
	}
	return sig
}

// newCtorNode resolves `new S(...)` to the constructor declaration it runs, the way
// newExpr resolves it: through the class the identifier names, so a local class shadowing
// a built-in still answers as itself. A class with no written constructor has no node to
// mark, and nothing to mark on it either, since its arguments go nowhere.
func (r *Renderer) newCtorNode(n frontend.Node) (frontend.Node, bool) {
	if n.Kind() != frontend.NodeNewExpression {
		return nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return nil, false
	}
	info, ok := r.classNameRef(kids[0])
	if !ok || info.ctor == nil {
		return nil, false
	}
	return info.ctor, true
}

// superCtorNode resolves a super(...) call to the base constructor it runs, which is how a
// box reaches a base's parameter: a derived class hands its own parameter straight on, and
// only the base's declaration says what the field it fills holds. The walk supplies
// curClass, so the base is the one the enclosing class extends.
func (r *Renderer) superCtorNode(n frontend.Node) (frontend.Node, bool) {
	if n.Kind() != frontend.NodeCallExpression {
		return nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeSuperKeyword {
		return nil, false
	}
	if r.curClass == nil || r.curClass.base == nil || r.curClass.base.ctor == nil {
		return nil, false
	}
	return r.curClass.base.ctor, true
}

// callOfBoxedReturnFunc reports whether a call goes to a function this pass gave a
// value.Value result, so the call is already a box even though the checker types it by
// the shape the function declares.
func (r *Renderer) callOfBoxedReturnFunc(n frontend.Node) bool {
	if len(r.boxedReturnFns) == 0 {
		return false
	}
	fn, ok := r.calleeFuncNode(n)
	return ok && r.boxedReturnFns[fn]
}

// readOfBoxedGetter reports whether a property read runs a getter this pass gave a
// value.Value result. A getter is called by being read, so the read is where its box
// arrives, and this is the getter's answer to the question callOfBoxedReturnFunc answers
// for everything that is called with parentheses.
func (r *Renderer) readOfBoxedGetter(n frontend.Node) bool {
	if len(r.boxedReturnFns) == 0 || n.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 || kids[1].Kind() != frontend.NodeIdentifier {
		return false
	}
	name := r.prog.Text(kids[1])
	// A receiver naming the class itself reads the package function a static getter
	// became, the same split calleeMethodNode makes for a call.
	if info, ok := r.classNameRef(kids[0]); ok {
		if g, ok := info.staticGetterByName(name); ok {
			return r.boxedReturnFns[g.node]
		}
	}
	info, ok := r.classReceiver(kids[0])
	if !ok {
		return false
	}
	g, ok := info.getterByName(name)
	return ok && r.boxedReturnFns[g.node]
}

// isBoxedParamRead reports whether an identifier names a parameter this pass gave a
// value.Value slot. The body's own dynamic-locals bookkeeping answers the same question
// once the body is lowering; this answers it from the symbol, which is what the pass
// itself needs while it is still deciding and what a read outside the current body
// (another function's call to this one) needs too.
func (r *Renderer) isBoxedParamRead(n frontend.Node) bool {
	if len(r.boxedParamSyms) == 0 || n.Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(n)
	return ok && r.boxedParamSyms[sym]
}

// collFromIterable answers the collection type a `new Set(src)` or `new Map(src)` mints
// and the source it is filled from. A construction with no argument fills nothing, so it
// reports false and the slot is left to the calls that reach it later.
func (r *Renderer) collFromIterable(n frontend.Node) (frontend.Type, frontend.Node, bool) {
	if n.Kind() != frontend.NodeNewExpression {
		return frontend.Type{}, nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) < 2 {
		return frontend.Type{}, nil, false
	}
	// The children after the callee carry the written type arguments beside the value
	// arguments, and namedArgs is what tells the two apart.
	args := r.namedArgs(kids[1:])
	if len(args) != 1 {
		return frontend.Type{}, nil, false
	}
	return r.prog.TypeAt(n), args[0], true
}
