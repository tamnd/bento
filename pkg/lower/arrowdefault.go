package lower

import "github.com/tamnd/bento/pkg/frontend"

// collectArrowDefaults walks the whole program before any body lowers and records
// the const-bound arrow functions whose defaulted parameters can lower as plain Go
// fields, filled at the call site the way a top-level function's defaults are.
//
// An arrow is a Go func value, and a Go func value carries no optional parameter,
// so a defaulted arrow parameter hands back in the general case: a callback slot
// like `cb(1)` cannot reconstruct a default it never saw. The one case that is safe
// is a const binding whose only uses are the binding itself and direct calls on it.
// Such a binding never escapes as a value, so its Go func type is pinned only by the
// `f := func(...)` the initializer emits, and every call is a direct call the call
// site can fill the omitted default at, exactly as a top-level function does.
//
// This mirrors the other RenderProgram pre-passes (collectMono, collectClasses): a
// whole-program walk that fills a Renderer map both the arrow's declaration lowering
// and its call sites read, so a call and the arrow it resolves to agree without a
// shared table. It reads only the checker and the AST, so it cannot fail; an arrow
// that is not provably escape-safe simply records nothing and keeps its handback.
func (r *Renderer) collectArrowDefaults(entry frontend.Node) {
	// First pass: candidate const-bound arrows with at least one call-site-fillable
	// default, keyed by the binding symbol and remembering the arrow node.
	candidates := map[frontend.Symbol]frontend.Node{}
	var find func(n frontend.Node)
	find = func(n frontend.Node) {
		if n.Kind() == frontend.NodeVariableDeclaration {
			if sym, arrow, ok := r.arrowDefaultCandidate(n); ok {
				candidates[sym] = arrow
			}
		}
		for _, c := range r.prog.Children(n) {
			find(c)
		}
	}
	find(entry)
	if len(candidates) == 0 {
		return
	}

	// Second pass: escape analysis. A candidate is disqualified the moment any use of
	// its symbol is neither the binding's own declaration name nor the callee of a
	// direct call, since any other use pins or passes the Go func value. The default
	// answer is conservative: an unclassified use marks the symbol unsafe, so the arrow
	// keeps its handback rather than pass a Go zero value where a default belonged.
	unsafe := map[frontend.Symbol]bool{}
	var walk func(n, parent frontend.Node)
	walk = func(n, parent frontend.Node) {
		if n.Kind() == frontend.NodeIdentifier {
			if sym, ok := r.prog.SymbolAt(n); ok {
				if _, isCand := candidates[sym]; isCand && !r.arrowUseIsSafe(n, parent) {
					unsafe[sym] = true
				}
			}
		}
		for _, c := range r.prog.Children(n) {
			walk(c, n)
		}
	}
	walk(entry, nil)

	for sym, arrow := range candidates {
		if unsafe[sym] {
			continue
		}
		r.arrowDropDefaults[arrow] = true
		r.arrowCallDefaults[sym] = r.arrowDefaultNodes(arrow)
	}
}

// arrowDefaultCandidate reports whether a variable declaration binds a plain
// identifier to an arrow function that carries at least one default the call site
// could fill, and returns the binding symbol and the arrow node. It screens out
// every shape the call-site fill cannot serve: a destructured or renamed binding
// name, an initializer that is not a bare arrow, an async arrow (its body wraps in
// the promise coroutine), a rest or non-identifier parameter, and a default that
// reads an earlier parameter, which is evaluated in the callee's scope where the
// call site cannot reconstruct it.
func (r *Renderer) arrowDefaultCandidate(decl frontend.Node) (frontend.Symbol, frontend.Node, bool) {
	kids := r.prog.Children(decl)
	if len(kids) < 2 {
		return frontend.Symbol{}, nil, false
	}
	nameNode := kids[0]
	if nameNode.Kind() != frontend.NodeIdentifier {
		return frontend.Symbol{}, nil, false
	}
	arrow := kids[len(kids)-1]
	if arrow.Kind() != frontend.NodeArrowFunction {
		return frontend.Symbol{}, nil, false
	}
	sym, ok := r.prog.SymbolAt(nameNode)
	if !ok {
		return frontend.Symbol{}, nil, false
	}
	if r.isAsyncFunc(arrow) {
		return frontend.Symbol{}, nil, false
	}
	sig, ok := r.prog.SignatureAt(arrow)
	if !ok || sig.RestParam != nil {
		return frontend.Symbol{}, nil, false
	}
	paramNodes := r.funcParamNodes(arrow)
	hasDefault := false
	for i, pn := range paramNodes {
		pkids := r.prog.Children(pn)
		if len(pkids) == 0 || pkids[0].Kind() != frontend.NodeIdentifier {
			return frontend.Symbol{}, nil, false
		}
		def, ok := r.paramDefaultNode(paramNodes, i)
		if !ok {
			continue
		}
		if r.defaultReadsOwnParam(sig, def) {
			return frontend.Symbol{}, nil, false
		}
		hasDefault = true
	}
	if !hasDefault {
		return frontend.Symbol{}, nil, false
	}
	return sym, arrow, true
}

// arrowUseIsSafe reports whether one use of a candidate arrow's binding keeps it
// from escaping as a value. A use is safe only when it is the binding's own
// declaration name (the const that introduces it) or the callee position of a direct
// call, `f(...)`, where the call site reconstructs the default. Every other position,
// passing f as an argument, storing it, reading it as a member object, marks the
// binding as escaping, so the arrow keeps its handback.
func (r *Renderer) arrowUseIsSafe(use, parent frontend.Node) bool {
	if parent == nil {
		return false
	}
	kids := r.prog.Children(parent)
	if len(kids) == 0 {
		return false
	}
	switch parent.Kind() {
	case frontend.NodeVariableDeclaration:
		// The binding's own name node, the left of `const f = ...`, is not a read.
		return kids[0] == use
	case frontend.NodeCallExpression:
		// The callee of a direct call is safe; an argument position is an escape.
		return kids[0] == use
	}
	return false
}

// arrowDefaultNodes returns an arrow's parameter defaults aligned to its parameter
// list, with a nil where a parameter has no default, the shape buildCall reads to
// fill an omitted trailing argument. It mirrors calleeDefaults for the arrow form,
// reading the defaults off the arrow's own parameter nodes.
func (r *Renderer) arrowDefaultNodes(arrow frontend.Node) []frontend.Node {
	paramNodes := r.funcParamNodes(arrow)
	out := make([]frontend.Node, len(paramNodes))
	for i := range paramNodes {
		if def, ok := r.paramDefaultNode(paramNodes, i); ok {
			out[i] = def
		}
	}
	return out
}
