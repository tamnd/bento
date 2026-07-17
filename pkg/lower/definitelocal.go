package lower

import (
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file finds the statically typed locals declared with no initializer that
// are safe to lower to a plain Go var of the declared type. A JavaScript binding
// with no initializer reads undefined until its first assignment, and the Go zero
// value of a number is 0, of a string "", neither of them undefined, so lowering
// such a binding to `var x T` is sound only when no read can observe the slot
// before a real value lands in it.
//
// The TypeScript checker, under strict mode, already proves that for every direct
// in-flow read: it rejects `let x: number; use(x)` before an assignment as
// "Variable 'x' is used before being assigned", so any such program hands back at
// the front door before lowering runs. The one read shape its definite-assignment
// analysis does not police is a read from inside a closure, `let x: number; const
// g = () => x`, which it accepts even though g may run while x is still undefined.
// So the analysis here reproduces the checker's guarantee by exclusion: a
// no-initializer typed local qualifies unless a nested function captures it. The
// exclusion is conservative, a name spelled the same inside any nested function drops
// the local whether or not that mention truly aliases it, so a wrong answer only
// costs a handback, never a wrong value.
//
// One captured shape rejoins the plain-var set: a local assigned by unconditional
// top-level code before any capturing closure is defined. Every such closure then
// sits after the assignment and cannot run before the slot holds a real value, so
// Go's by-reference capture reads that value and the zero is never observed as one.
// assignedBeforeAnyCapture proves that flow shape, again conservatively, so a shape it
// cannot positively clear keeps handing back.
//
// An optional (T | undefined) or dynamic (any, unknown) binding is not this set's
// concern: its Go zero value already is undefined (value.Opt's None, value.Value's
// nil box), so buildVarDecl lowers it on its own no-initializer branches with no
// definite-assignment reasoning needed.

// definiteLocalsOf analyzes a body and returns the set of local names declared with
// a non-optional, non-dynamic static type and no initializer that no nested closure
// captures, so a no-initializer declaration of one lowers to a plain `var x T`. A
// name declared more than once is dropped, since the flat name set cannot tell two
// scopes apart, the same soundness rule optLocalsOf follows. A nil map means nothing
// to lower this way.
func (r *Renderer) definiteLocalsOf(body []frontend.Node) map[string]bool {
	cand := map[string]bool{}
	declCount := map[string]int{}
	for _, n := range body {
		r.collectDefiniteDecls(n, cand, declCount)
	}
	if len(cand) == 0 {
		return nil
	}
	captured := map[string]bool{}
	for _, n := range body {
		r.collectClosureCaptures(n, captured)
	}
	out := map[string]bool{}
	for name := range cand {
		if declCount[name] != 1 {
			continue
		}
		// A local no closure captures reproduces the checker's own definite-assignment
		// guarantee and lowers to a plain var. A captured one normally hands back, since
		// the checker does not police a closure read, but it is safe all the same when it
		// is assigned by unconditional top-level code before any capturing closure is even
		// defined: assignedBeforeAnyCapture proves that shape.
		if !captured[name] || r.assignedBeforeAnyCapture(name, body) {
			out[name] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// assignedBeforeAnyCapture reports whether a closure-captured no-initializer typed
// local is assigned by unconditional top-level code before any closure that could
// capture it is defined, the one flow shape under which a plain `var x T` is sound
// despite the capture. The checker does not police a read of the local from inside a
// closure, so a plain var would read Go's zero (0 for a number, "" for a string)
// where a closure that ran before the first assignment observes JavaScript undefined.
// When the local is assigned first, every capturing closure sits after that
// assignment and so cannot run before the slot holds a real value, and Go's
// by-reference capture reads that value, so the zero is never observed as a value.
// The scan walks the scope's top-level statement list in order: it skips the
// declaration, and the first later statement that mentions the name decides it, which
// must be an unconditional top-level `x = rhs` whose right side neither reads the name
// nor holds any function. Anything else (a read, a closure, a compound or
// destructuring write, a conditional assignment) leaves the pre-assignment window open
// and hands back. The proof is conservative, so a shape it cannot positively clear
// costs a handback, never a wrong value.
func (r *Renderer) assignedBeforeAnyCapture(name string, body []frontend.Node) bool {
	for _, s := range body {
		// The declaration `let x: T;` mentions the name once, as its own binding, and is
		// not a read: skip it. A declaration that also reads the name in a sibling
		// initializer (let x: T, y = x) mentions it more than once, so it is not skipped
		// and is caught below as a pre-assignment read.
		if r.isNoInitDeclOf(name, s) {
			continue
		}
		if r.countIdentAnywhere(s, name) == 0 {
			continue
		}
		return r.isCaptureSafeFirstAssign(name, s)
	}
	return false
}

// isNoInitDeclOf reports whether a statement is the variable declaration of name with
// no initializer and no other mention of the name, the `let x: T;` shape the capture
// proof skips over on its way to the first assignment. A statement that reads the name
// in a sibling initializer mentions it more than once and is not this shape.
func (r *Renderer) isNoInitDeclOf(name string, s frontend.Node) bool {
	if s.Kind() != frontend.NodeVariableStatement {
		return false
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, s, &decls)
	found := false
	for _, d := range decls {
		kids := r.prog.Children(d)
		if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier || r.prog.Text(kids[0]) != name {
			continue
		}
		// A binding is [name], [name, type], [name, initializer], or [name, type,
		// initializer]. An initializer is the last child carrying a real expression kind.
		if len(kids) >= 2 && kids[len(kids)-1].Kind() != frontend.NodeUnknown {
			return false
		}
		found = true
	}
	return found && r.countIdentAnywhere(s, name) == 1
}

// isCaptureSafeFirstAssign reports whether a statement is an unconditional top-level
// `name = rhs` whose right side neither reads name nor contains a function, the one
// first-mention shape that proves name is assigned before any capturing closure is
// defined. A compound assignment, a destructuring target, or a right side holding a
// closure or a read of name all fail it, so only the plain store the assignment path
// already lowers reaches the plain-var admission.
func (r *Renderer) isCaptureSafeFirstAssign(name string, s frontend.Node) bool {
	if s.Kind() != frontend.NodeExpressionStatement {
		return false
	}
	kids := r.prog.Children(s)
	if len(kids) != 1 || kids[0].Kind() != frontend.NodeBinaryExpression {
		return false
	}
	parts := r.prog.Children(kids[0])
	if len(parts) != 3 {
		return false
	}
	if parts[0].Kind() != frontend.NodeIdentifier || r.prog.Text(parts[0]) != name {
		return false
	}
	if strings.TrimSpace(r.prog.Text(parts[1])) != "=" {
		return false
	}
	rhs := parts[2]
	if r.countIdentAnywhere(rhs, name) != 0 {
		return false
	}
	return !subtreeHasFunctionLike(r.prog, rhs)
}

// subtreeHasFunctionLike reports whether a subtree holds any function-like node, so a
// right-hand side that could capture a name is kept off the capture-safe assignment
// path.
func subtreeHasFunctionLike(prog *frontend.Program, n frontend.Node) bool {
	if isFunctionLike(n.Kind()) {
		return true
	}
	for _, c := range prog.Children(n) {
		if subtreeHasFunctionLike(prog, c) {
			return true
		}
	}
	return false
}

// collectDefiniteDecls walks one node, recording each variable declaration whose name
// is a plain identifier typed as a non-optional, non-dynamic static type and that
// carries no initializer, and recurses so a binding in a nested block or loop is seen.
// It counts declarations per name alongside so definiteLocalsOf can drop a name
// declared in more than one scope. A binding with an initializer, a destructuring
// target, or an optional or dynamic type is left to its own lowering path.
func (r *Renderer) collectDefiniteDecls(n frontend.Node, cand map[string]bool, declCount map[string]int) {
	if n.Kind() == frontend.NodeVariableDeclaration {
		kids := r.prog.Children(n)
		if len(kids) > 0 && kids[0].Kind() == frontend.NodeIdentifier {
			if name, ok := localName(r.prog.Text(kids[0])); ok {
				declCount[name]++
				// A binding is [name], [name, type], [name, initializer], or [name, type,
				// initializer]. An initializer is the last child carrying a real expression
				// kind, so its absence is the last child being a type annotation node (left
				// unclassified as NodeUnknown) or no second child at all.
				hasInit := len(kids) >= 2 && kids[len(kids)-1].Kind() != frontend.NodeUnknown
				if !hasInit {
					t := r.prog.TypeAt(kids[0])
					if t.Flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 && !r.isOptionalType(t) {
						cand[name] = true
					}
				}
			}
		}
	}
	for _, c := range r.prog.Children(n) {
		r.collectDefiniteDecls(c, cand, declCount)
	}
}

// collectClosureCaptures walks one node and, at every nested function literal, marks
// every identifier under it as captured, then recurses through non-function nodes so a
// closure at any depth is reached. Marking every same-spelled identifier inside a
// closure is deliberately blunt: it never misses a genuine capture and only ever drops
// an extra local from the definite set, which costs a handback and not a wrong value.
func (r *Renderer) collectClosureCaptures(n frontend.Node, captured map[string]bool) {
	switch n.Kind() {
	case frontend.NodeArrowFunction, frontend.NodeFunctionExpression, frontend.NodeFunctionDeclaration,
		frontend.NodeMethodDeclaration, frontend.NodeGetAccessor, frontend.NodeSetAccessor, frontend.NodeConstructor:
		r.markCapturedIdents(n, captured)
		return
	}
	for _, c := range r.prog.Children(n) {
		r.collectClosureCaptures(c, captured)
	}
}

// markCapturedIdents records every identifier name in a subtree as captured, the
// blanket rule for a nested function body.
func (r *Renderer) markCapturedIdents(n frontend.Node, captured map[string]bool) {
	if n.Kind() == frontend.NodeIdentifier {
		if name, ok := localName(r.prog.Text(n)); ok {
			captured[name] = true
		}
		return
	}
	for _, c := range r.prog.Children(n) {
		r.markCapturedIdents(c, captured)
	}
}
