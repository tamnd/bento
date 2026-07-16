package lower

import (
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
// no-initializer typed local qualifies unless a nested function captures it, in
// which case it keeps handing back. The exclusion is conservative, a name spelled
// the same inside any nested function drops the local whether or not that mention
// truly aliases it, so a wrong answer only costs a handback, never a wrong value.
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
		if declCount[name] == 1 && !captured[name] {
			out[name] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
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
