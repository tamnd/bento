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
		// An object-literal shorthand, `{ f }`, writes the key and the value with one
		// identifier, and the symbol at that identifier is the property the member
		// declares rather than the binding it reads. So the identifier walk below never
		// sees the escape, and the arrow would drop a default no call site fills:
		// `const box = { f }; box.f(1)` printed undefined for the defaulted parameter.
		// Crediting the member to the binding it reads is what closes that, the same
		// resolution the unused-binding walk already makes for the same spelling.
		if sym, ok := shorthandValueSymbol(r.prog, n); ok {
			if _, isCand := candidates[sym]; isCand {
				unsafe[sym] = true
			}
		}
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

// collectClosureDefaultParams marks the closure parameters whose default the callee
// fills in its own body, which is how a defaulted parameter escapes the handback that
// made "function parameter with a default value is a later slice" the largest single
// refusal in the Node compatibility suite.
//
// The fill is `if p.IsUndefined() { p = <default> }` at body entry, so the parameter's
// Go slot has to be able to hold undefined, which means a value.Value. A parameter the
// checker already types any has one. A parameter with no annotation does not: the
// checker read its type off the default, so `function f(a, b = 2)` in a plain .js file
// types b number while its unannotated sibling a is any. That number is an artifact of
// the default rather than anything the source said, and it is the only thing standing
// between the parameter and a slot that could hold the undefined an omitted argument
// binds. So the mark puts it back where its sibling already is.
//
// It is spelled as a boxed-parameter mark, the same representation collectBoxedSignatures
// uses for a parameter a call site hands a box to, because everything downstream already
// reads that: boxedSig rewrites the type to any at the declaration and at every call
// site, so the field is a value.Value, the body reads the name through the value model,
// an argument boxes on the way in, and an omitted one arrives as value.Undefined. Running
// before the fixpoint lets it propagate, so a body that hands its defaulted parameter on
// is seen to pass a box.
//
// An annotated default (`b: number = 2`) is left alone: there the static type is what the
// source asked for, and taking it away to serve the default would be the tail wagging the
// dog. It keeps its handback and is the next slice. A top-level function is left alone
// too, since it already fills its defaults at the call site and never reached the closure
// path; only functions that lower as closures are marked, which in a Node program is
// every function in a required module, that module's body being a closure itself.
func (r *Renderer) collectClosureDefaultParams(files []frontend.Node) {
	var walk func(n, parent frontend.Node, inFunc bool)
	walk = func(n, parent frontend.Node, inFunc bool) {
		if isFunctionLike(n.Kind()) {
			if (n.Kind() != frontend.NodeFunctionDeclaration || inFunc) && !isCallArgument(parent) {
				r.markClosureDefaultParams(n)
			}
			inFunc = true
		}
		for _, c := range r.prog.Children(n) {
			walk(c, n, inFunc)
		}
	}
	for _, f := range files {
		// A required module runs as its own loader function, so even its top-level
		// declarations lower as closures and take the closure parameter path.
		_, required := r.requiredLoaders[f.File().Path]
		walk(f, nil, required)
	}
}

// isCallArgument reports whether a node's parent is a call or a construction, which is
// where a function literal is written inline as a callback. Such a literal's Go func type
// is not its own to choose: it has to fit the slot the callee declares, `nums.map` taking
// a func(float64) float64, so making a parameter dynamic there would emit a literal the
// call cannot pass. Where the callee's slot is itself dynamic the lowering already forces
// the parameters dynamic on its own (forceCallbackDynParams), scoped to that one call, so
// a callback needs nothing from this pass either way.
func isCallArgument(parent frontend.Node) bool {
	if parent == nil {
		return false
	}
	switch parent.Kind() {
	case frontend.NodeCallExpression, frontend.NodeNewExpression:
		return true
	}
	return false
}

// markClosureDefaultParams marks each parameter of one closure whose default the body
// will fill: it carries a default, it is a plain name with no annotation, and its type is
// not already any. The mark is both halves of the boxed-parameter representation, the
// per-function slot boxedSig reads and the per-name entry the parameter field and the
// boxing wrapper read, so the declaration, the body, and every call site land on the same
// answer.
func (r *Renderer) markClosureDefaultParams(fn frontend.Node) {
	// An escape-safe const-bound arrow already drops its defaults and fills them at each
	// direct call site, which keeps the parameter's static Go type. That is the better
	// lowering where it applies, so it wins here.
	if r.arrowDropDefaults[fn] {
		return
	}
	// A function the program calls with new is a runtime constructor, whose parameters
	// the construction path binds itself. Rewriting one of its slots here would put the
	// two paths on different types, so a constructor keeps its handback.
	if sym, ok := r.prog.SymbolAt(fn); ok && r.ctorFuncs[sym] {
		return
	}
	paramNodes := r.funcParamNodes(fn)
	var marks []bool
	for i, pn := range paramNodes {
		if _, ok := r.paramDefaultNode(paramNodes, i); !ok {
			continue
		}
		pkids := r.prog.Children(pn)
		if pkids[0].Kind() != frontend.NodeIdentifier || r.paramIsAnnotated(pn) {
			continue
		}
		if r.prog.TypeAt(pkids[0]).Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
			continue
		}
		if marks == nil {
			marks = make([]bool, len(paramNodes))
			copy(marks, r.boxedParams[fn])
		}
		marks[i] = true
		if r.forceDynParams == nil {
			r.forceDynParams = map[frontend.Node]bool{}
		}
		r.forceDynParams[pkids[0]] = true
	}
	if marks == nil {
		return
	}
	if r.boxedParams == nil {
		r.boxedParams = map[frontend.Node][]bool{}
	}
	r.boxedParams[fn] = marks
}

// paramIsAnnotated reports whether a parameter carries a written type annotation. The
// shim leaves an annotation as an opaque unknown node among the parameter's children,
// which is the same thing paramDefaultNode skips past to find the default, so the two
// read the one shape from opposite ends.
func (r *Renderer) paramIsAnnotated(pn frontend.Node) bool {
	kids := r.prog.Children(pn)
	for _, c := range kids[1:] {
		if c.Kind() == frontend.NodeUnknown {
			return true
		}
	}
	return false
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
