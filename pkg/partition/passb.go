package partition

import "github.com/tamnd/bento/pkg/frontend"

// PassB propagates contamination over the program to a fixpoint, taking Pass A's
// local verdicts and returning the verdicts that hold once cross-unit effects are
// accounted for (06_compile_vs_interpret.md section 4.2). It takes the Pass A
// results verbatim and returns a new slice in the same unit order, so a caller
// can diff the two to see exactly what Pass B changed.
//
// The load-bearing property of this pass is containment (section 4.4). The naive
// rule "a compiled unit that calls an interpreted unit must itself be
// interpreted" is wrong and would collapse the whole program into the engine: a
// compiled unit calls an interpreted one perfectly well by paying a boundary
// crossing (section 10), so an ordinary call never contaminates. Pass B therefore
// does nothing along ordinary call edges; a Compiled caller of an Interpreted
// callee stays Compiled. Contamination flows along exactly two edges instead.
//
// This slice realizes the second of those, the control-inversion edge: a compiled
// unit's function handed to an untyped callback position, where interpreted or
// unknown code may invoke it with arguments of the wrong static type, needs entry
// guards, which makes it Speculative rather than Interpreted. The first edge, the
// value-identity edge that drives boxing (section 13), is the escape-analysis
// lattice and lands in its own slice; the call graph this pass is built beside is
// the substrate that slice walks.
func (pt *Partitioner) PassB(pa []Result) []Result {
	out := make([]Result, len(pa))
	copy(out, pa)

	units := pt.Units()
	index := make(map[nodeKey]int, len(units))
	for i, u := range units {
		index[keyOf(u.Root)] = i
	}

	// inverted maps a unit index to the reference node that inverts control into
	// it, so the promotion below can point a diagnostic at the exact callback
	// argument. A unit referenced from several untyped positions inverts once.
	inverted := map[int]frontend.Node{}
	for i := range units {
		pt.scanInversion(units[i].Root, index, out, inverted)
	}

	for j, ref := range inverted {
		// Only a cleanly Compiled unit is promoted. A unit Pass A already left
		// Interpreted stays there, and a unit that is a Pass C speculation
		// candidate for its own reasons is left for Pass C; control inversion adds
		// nothing to a unit that is not otherwise native.
		if out[j].Verdict != Compiled {
			continue
		}
		out[j].Verdict = Speculative
		out[j].Reasons = append(out[j].Reasons, Reason{
			Kind:    ReasonControlInversion,
			Node:    ref,
			Message: "compiled function handed to an untyped callback position needs entry guards",
		})
	}

	return out
}

// scanInversion walks one unit's own body, stopping at nested function
// boundaries the way Pass A's walkBody does, and inspects every call and new
// expression it contains for a control-inversion argument. A call inside a nested
// function is inspected when that nested unit is the walk root, so each call is
// inspected exactly once, under the unit that owns it. Which unit owns a call does
// not matter to the result, only which unit a reference argument targets, so the
// single visit is enough.
func (pt *Partitioner) scanInversion(node frontend.Node, index map[nodeKey]int, results []Result, inverted map[int]frontend.Node) {
	for _, child := range pt.prog.Children(node) {
		switch child.Kind() {
		case frontend.NodeCallExpression, frontend.NodeNewExpression:
			pt.inspectCallbackArgs(child, index, results, inverted)
		}
		if functionLike(child.Kind()) {
			continue
		}
		pt.scanInversion(child, index, results, inverted)
	}
}

// inspectCallbackArgs looks at one call's arguments for a bare reference to a
// compiled function unit passed into an untyped position. The callee is the first
// child; the arguments follow it in order, which lines them up with the resolved
// signature's parameters for the common call shape this slice handles. A call that
// carries explicit type arguments or a spread can misalign that mapping, a known
// imprecision a later slice tightens; the mapping is only ever read to decide
// whether a position is untyped, so a misalignment can add a guard, never drop
// one, which stays on the sound side.
func (pt *Partitioner) inspectCallbackArgs(call frontend.Node, index map[nodeKey]int, results []Result, inverted map[int]frontend.Node) {
	kids := pt.prog.Children(call)
	if len(kids) < 2 {
		return // a callee and at least one argument
	}
	args := kids[1:]
	sig, hasSig := pt.prog.SignatureAt(call)
	calleeExternal := pt.calleeIsExternal(kids[0], index)

	for k, arg := range args {
		j, ok := pt.referencedFunctionUnit(arg, index, results)
		if !ok {
			continue
		}
		if pt.positionUntyped(sig, hasSig, k, calleeExternal) {
			if _, seen := inverted[j]; !seen {
				inverted[j] = arg
			}
		}
	}
}

// referencedFunctionUnit reports the unit index when arg is a bare identifier
// that names a function unit in this program, the shape of a function passed by
// reference rather than called. An argument that is itself a call, a property
// access, or anything but a plain name is not a by-reference handoff and is
// skipped, and a name that resolves to a variable or a class rather than a
// function unit is skipped too.
func (pt *Partitioner) referencedFunctionUnit(arg frontend.Node, index map[nodeKey]int, results []Result) (int, bool) {
	if arg.Kind() != frontend.NodeIdentifier {
		return 0, false
	}
	sym, ok := pt.prog.SymbolAt(arg)
	if !ok {
		return 0, false
	}
	sym = pt.prog.Aliased(sym)
	for _, decl := range pt.prog.Declarations(sym) {
		j, ok := index[keyOf(decl)]
		if !ok {
			continue
		}
		if results[j].Unit.Kind == UnitFunction {
			return j, true
		}
	}
	return 0, false
}

// calleeIsExternal reports whether a call's callee resolves to no unit in this
// program: a node: builtin, a go: import, or an unresolved dynamic target. It is
// the same external notion the call graph records, reused here so the untyped
// fallback below only fires on a genuinely unknown callee.
func (pt *Partitioner) calleeIsExternal(callee frontend.Node, index map[nodeKey]int) bool {
	sym, ok := pt.prog.SymbolAt(callee)
	if !ok {
		return true
	}
	sym = pt.prog.Aliased(sym)
	for _, decl := range pt.prog.Declarations(sym) {
		if _, ok := index[keyOf(decl)]; ok {
			return false
		}
	}
	return true
}

// positionUntyped reports whether argument position k receives an untyped value,
// the callback position that inverts control. When the checker resolved a
// signature for the call, the parameter at k decides it: an any or unknown
// parameter, or an any-element rest parameter absorbing k, is untyped, while a
// parameter with a concrete function type models the callback's own call and is
// not. When no signature resolved, only a genuinely external callee is treated as
// untyped, so an in-program call with a lost signature does not invent a guard.
func (pt *Partitioner) positionUntyped(sig frontend.Signature, hasSig bool, k int, calleeExternal bool) bool {
	if !hasSig {
		return calleeExternal
	}
	if k < len(sig.Params) {
		return isUntyped(sig.Params[k].Type)
	}
	if sig.RestParam != nil {
		if elem, ok := pt.prog.ElementType(sig.RestParam.Type); ok {
			return isUntyped(elem)
		}
		return isUntyped(sig.RestParam.Type)
	}
	return false
}

// isUntyped reports whether a type is any or unknown, the flags that mark a
// position the type system cannot vouch for.
func isUntyped(t frontend.Type) bool {
	return t.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0
}
