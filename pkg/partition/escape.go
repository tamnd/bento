package partition

import "github.com/tamnd/bento/pkg/frontend"

// Boxing is the result of escape analysis: the set of bindings whose values must
// be represented as a boxed canonical Value rather than a monomorphic Go
// representation, because each reaches a dynamic sink where interpreted code or a
// reflective operation observes or mutates it (06_compile_vs_interpret.md section
// 13). A binding not in the set can stay in its cheap monomorphic representation
// for its whole lifetime.
//
// Boxing is the second lattice Pass B propagates (section 4.2). It is a monotone
// data-flow analysis: a value escapes if it flows to a sink or flows to another
// value that escapes, computed to a fixpoint. This analysis seeds the sinks it can
// see directly and closes escape over the aliasing it can see within a unit; the
// inter-procedural flow that carries escape across a return or through a retained
// argument is a later slice, and until it lands the result is a sound lower bound
// that lowering must not yet rely on to stop boxing.
type Boxing struct {
	// Escaping holds every binding symbol that must be boxed.
	Escaping map[frontend.Symbol]bool
}

// Escapes reports whether a binding must be boxed.
func (b Boxing) Escapes(sym frontend.Symbol) bool { return b.Escaping[sym] }

// Boxing runs escape analysis over the whole program and returns the boxing
// lattice. It walks every unit body to seed the direct sinks and collect the
// aliasing edges, then closes escape over those edges to a fixpoint so a value
// that escapes reaches every binding that aliases it.
func (pt *Partitioner) Boxing() Boxing {
	units := pt.Units()
	index := make(map[nodeKey]int, len(units))
	for i, u := range units {
		index[keyOf(u.Root)] = i
	}

	seeds := map[frontend.Symbol]bool{}
	edges := map[frontend.Symbol][]frontend.Symbol{}
	for _, u := range units {
		pt.scanEscapes(u.Root, index, seeds, edges)
	}

	closeEscapes(seeds, edges)
	return Boxing{Escaping: seeds}
}

// scanEscapes walks one unit's body, stopping at nested function boundaries the
// way the other Pass B walks do. It seeds an escape at every dynamic-sink argument
// and records an aliasing edge at every binding-to-binding declaration or
// assignment, so the closure can carry an escape from one alias to the other.
func (pt *Partitioner) scanEscapes(node frontend.Node, index map[nodeKey]int, seeds map[frontend.Symbol]bool, edges map[frontend.Symbol][]frontend.Symbol) {
	for _, child := range pt.prog.Children(node) {
		switch child.Kind() {
		case frontend.NodeCallExpression, frontend.NodeNewExpression:
			pt.seedSinks(child, index, seeds)
		case frontend.NodeVariableDeclaration:
			pt.collectDeclarationAlias(child, edges)
		case frontend.NodeBinaryExpression:
			pt.collectAssignmentAlias(child, edges)
		}
		if functionLike(child.Kind()) {
			continue
		}
		pt.scanEscapes(child, index, seeds, edges)
	}
}

// seedSinks marks the binding behind every argument that crosses into a position
// the type system cannot vouch for. A value handed to an any or unknown parameter
// can be walked reflectively, retained, or shape-mutated by the receiver, none of
// which a fixed Go layout survives, so it must box. This one rule covers the
// reflective walk (JSON.stringify, whose parameter is any) and the boundary
// crossing into interpreted or unknown code alike. Primitives are immutable and
// cross by value with no identity concern (section 9.3), so only object-shaped
// arguments seed an escape.
func (pt *Partitioner) seedSinks(call frontend.Node, index map[nodeKey]int, seeds map[frontend.Symbol]bool) {
	kids := pt.prog.Children(call)
	if len(kids) < 2 {
		return
	}
	args := kids[1:]
	sig, hasSig := pt.prog.SignatureAt(call)
	calleeExternal := pt.calleeIsExternal(kids[0], index)

	for k, arg := range args {
		if !isBoxable(pt.prog.TypeAt(arg)) {
			continue
		}
		if !pt.positionUntyped(sig, hasSig, k, calleeExternal) {
			continue
		}
		if sym, ok := pt.bindingOf(arg); ok {
			seeds[sym] = true
		}
	}
}

// collectDeclarationAlias records the aliasing edge in a `const w = v` or
// `let w = v` whose initializer is a bare reference to another binding. Both name
// the same object at runtime, so if either escapes the other must box too; the
// edge is symmetric. A type annotation between the name and the initializer names
// a type, not a value binding, so it is skipped by the value-binding check.
func (pt *Partitioner) collectDeclarationAlias(decl frontend.Node, edges map[frontend.Symbol][]frontend.Symbol) {
	kids := pt.prog.Children(decl)
	if len(kids) < 2 {
		return
	}
	name := kids[0]
	w, ok := pt.valueBindingSymbol(name)
	if !ok || !isBoxable(pt.prog.TypeAt(name)) {
		return
	}
	for _, rhs := range kids[1:] {
		v, ok := pt.valueBindingSymbol(rhs)
		if !ok || !isBoxable(pt.prog.TypeAt(rhs)) {
			continue
		}
		addAliasEdge(edges, w, v)
	}
}

// collectAssignmentAlias records the aliasing edge in an assignment `w = v` where
// both sides are bindings of object type. Like a declaration alias, the two name
// the same object, so escape flows between them.
func (pt *Partitioner) collectAssignmentAlias(bin frontend.Node, edges map[frontend.Symbol][]frontend.Symbol) {
	kids := pt.prog.Children(bin)
	if len(kids) != 3 || pt.prog.Text(kids[1]) != "=" {
		return
	}
	l, ok := pt.valueBindingSymbol(kids[0])
	if !ok || !isBoxable(pt.prog.TypeAt(kids[0])) {
		return
	}
	r, ok := pt.valueBindingSymbol(kids[2])
	if !ok || !isBoxable(pt.prog.TypeAt(kids[2])) {
		return
	}
	addAliasEdge(edges, l, r)
}

// valueBindingSymbol returns the symbol a bare identifier names when that symbol
// is a value binding, a parameter or a variable, as opposed to a type, a function,
// or a class. Only a value binding holds a value that can alias another and be
// boxed, so a name that resolves to anything else is not an alias endpoint.
func (pt *Partitioner) valueBindingSymbol(n frontend.Node) (frontend.Symbol, bool) {
	if n.Kind() != frontend.NodeIdentifier {
		return frontend.Symbol{}, false
	}
	sym, ok := pt.prog.SymbolAt(n)
	if !ok {
		return frontend.Symbol{}, false
	}
	sym = pt.prog.Aliased(sym)
	for _, decl := range pt.prog.Declarations(sym) {
		switch decl.Kind() {
		case frontend.NodeParameter, frontend.NodeVariableDeclaration:
			return sym, true
		}
	}
	return frontend.Symbol{}, false
}

// bindingOf returns the symbol a bare identifier names, the binding whose stored
// value a sink observes. An argument that is not a plain identifier names no
// single binding to box.
func (pt *Partitioner) bindingOf(arg frontend.Node) (frontend.Symbol, bool) {
	if arg.Kind() != frontend.NodeIdentifier {
		return frontend.Symbol{}, false
	}
	sym, ok := pt.prog.SymbolAt(arg)
	if !ok {
		return frontend.Symbol{}, false
	}
	return pt.prog.Aliased(sym), true
}

// isBoxable reports whether a type is one whose values carry mutable identity, an
// object, array, class, or function shape, as opposed to an immutable primitive.
// Only these need boxing when they reach a sink; primitives cross by value.
func isBoxable(t frontend.Type) bool {
	return t.Flags&frontend.TypeObject != 0
}

// addAliasEdge adds a symmetric edge between two aliasing bindings, so the closure
// can carry an escape in either direction.
func addAliasEdge(edges map[frontend.Symbol][]frontend.Symbol, a, b frontend.Symbol) {
	if a == b {
		return
	}
	edges[a] = append(edges[a], b)
	edges[b] = append(edges[b], a)
}

// closeEscapes propagates escape to a fixpoint over the aliasing edges: once a
// binding escapes, every binding it aliases escapes too, transitively. The lattice
// is finite and monotone, so the worklist drains and the fixpoint is reached.
func closeEscapes(seeds map[frontend.Symbol]bool, edges map[frontend.Symbol][]frontend.Symbol) {
	work := make([]frontend.Symbol, 0, len(seeds))
	for sym := range seeds {
		work = append(work, sym)
	}
	for len(work) > 0 {
		sym := work[len(work)-1]
		work = work[:len(work)-1]
		for _, next := range edges[sym] {
			if !seeds[next] {
				seeds[next] = true
				work = append(work, next)
			}
		}
	}
}
