package partition

import "github.com/tamnd/bento/pkg/frontend"

// classifyLocal is Pass A for a single unit: it walks the unit's own body in
// isolation, ignoring who calls it and what it calls, and decides whether the
// unit's own syntax and its own local types are compilable
// (06_compile_vs_interpret.md section 4.3). It returns the reasons it could not
// cleanly compile; an empty slice means Compiled-eligible.
//
// The walk stops at nested function boundaries: a nested function is its own
// unit and is classified on its own, so a with statement inside a nested
// closure does not block the outer unit.
func (pt *Partitioner) classifyLocal(u Unit) []Reason {
	c := &collector{seen: map[ReasonKind]bool{}}

	// The unit's own signature: every parameter and the return must be
	// lowerable. A declared any parameter surfaces here as an untyped value, a
	// bare type parameter as an unlowerable type.
	if sig, ok := pt.prog.SignatureAt(u.Root); ok {
		for _, param := range sig.Params {
			pt.checkType(param.Type, u.Root, c)
		}
		if sig.RestParam != nil {
			pt.checkType(sig.RestParam.Type, u.Root, c)
		}
		pt.checkType(sig.Return, u.Root, c)
	}

	// The body: walk every node that belongs to this unit and check both its
	// syntax (hard and soft blockers) and the type it evaluates to.
	pt.walkBody(u.Root, c)

	return c.reasons
}

// walkBody visits the children of a unit root, and their children, stopping at
// any nested function boundary so the outer unit is judged on its own code only.
func (pt *Partitioner) walkBody(root frontend.Node, c *collector) {
	for _, child := range pt.prog.Children(root) {
		pt.inspect(child, c)
		if functionLike(child.Kind()) {
			continue // nested function: its own unit, do not descend
		}
		pt.walkBody(child, c)
	}
}

// inspect checks one node for the blockers Pass A can see locally.
func (pt *Partitioner) inspect(n frontend.Node, c *collector) {
	switch n.Kind() {
	case frontend.NodeWithStatement:
		c.add(Reason{Kind: ReasonWith, Node: n, Message: "with statement defeats static scoping"})
	case frontend.NodeCallExpression:
		if pt.calleeName(n) == "eval" {
			c.add(Reason{Kind: ReasonEval, Node: n, Message: "eval defeats static scoping"})
		}
	case frontend.NodeNewExpression:
		if pt.calleeName(n) == "Function" {
			c.add(Reason{Kind: ReasonNewFunction, Node: n, Message: "new Function builds an opaque body from a string"})
		}
	}

	// Any observable any or unknown at a use site is a soft blocker. The zero
	// Type (a statement, a void position) reports no flags and is skipped.
	t := pt.prog.TypeAt(n)
	if t.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
		c.add(Reason{Kind: ReasonUntypedValue, Node: n, Message: "any or unknown value used without narrowing"})
	}
}

// checkType records a reason when a declared type is not lowerable, splitting an
// untyped any/unknown from a merely-unrendered shape so Pass C can tell a
// guardable edge from a plain gap.
func (pt *Partitioner) checkType(t frontend.Type, at frontend.Node, c *collector) {
	if t.Flags == 0 {
		return
	}
	if t.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
		c.add(Reason{Kind: ReasonUntypedValue, Node: at, Message: "any or unknown in the unit signature"})
		return
	}
	if !lowerable(pt.prog, t) {
		c.add(Reason{Kind: ReasonUnlowerableType, Node: at, Message: "signature type is outside the lowerable set"})
	}
}

// calleeName returns the name the call or new expression's callee resolves to,
// or the empty string when the callee is not a bound identifier. A call to eval
// or new Function is detected by this name, the way the checker would resolve
// the global symbol.
func (pt *Partitioner) calleeName(call frontend.Node) string {
	kids := pt.prog.Children(call)
	if len(kids) == 0 {
		return ""
	}
	callee := kids[0]
	if callee.Kind() != frontend.NodeIdentifier {
		return ""
	}
	if sym, ok := pt.prog.SymbolAt(callee); ok {
		return sym.Name
	}
	return ""
}

// collector accumulates reasons for a unit, deduplicating by kind so a unit with
// many any uses reports one untyped-value reason, not one per node. The first
// occurrence's node is kept so a diagnostic still points somewhere useful.
type collector struct {
	reasons []Reason
	seen    map[ReasonKind]bool
}

func (c *collector) add(r Reason) {
	if c.seen[r.Kind] {
		return
	}
	c.seen[r.Kind] = true
	c.reasons = append(c.reasons, r)
}
