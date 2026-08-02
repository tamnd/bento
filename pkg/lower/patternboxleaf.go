package lower

import (
	"github.com/tamnd/bento/pkg/frontend"
)

// A destructuring leaf off a boxed source holds a box, and which name it is has nothing
// to do with where the question is asked from.
//
// `const { a } = raw` over a boxed record binds a to a value.Value. Inside the body that
// destructured it that was already known: the binder records the names it assigned in
// dynBoundLocals, so every later read in that body dispatches through the value model.
// But that set is keyed by name and scoped to one body, and a top-level function reading
// a is not inside it, so from there the read lowered against the checker's shape for the
// name and the compiler saw a value.Value where a struct was wanted.
//
// The plain path answers this from the declaration instead. identifierBindsABox walks a
// binding's own declaration and asks boxedChainBinding, which is why a module binding
// holding a box reads correctly from anywhere. It could not answer for a leaf, since it
// requires the declaration's first child to be an identifier and a pattern's first child
// is the pattern. This file is that same answer for a leaf: which of a pattern's leaves
// is this symbol, and does the source hand it a box.

// patternLeafBindsABox reports whether the leaf a destructuring pattern binds holds a
// boxed value.Value rather than a Go value of the type the checker gave it.
//
// The rule is the binder's own, read at the declaration rather than off the emitted
// binds. A source that is not boxed destructures into ordinary Go values, so only a
// dynamic or boxed source can hand a leaf a box at all. Of the leaves it does reach, one
// the checker types number, string, or boolean comes down to its Go primitive at the
// bind, which is what dynLeafUnboxes says and what unboxDynLeaf emits, so it holds no
// box. One typed any or unknown has a boxed slot already and every read of it routes
// through isDynamic without help, so marking it changes nothing and it is left alone.
// What is left is the shapes, which have no Go value to come down to and keep the box.
func (r *Renderer) patternLeafBindsABox(leaf, init frontend.Node) bool {
	flags := r.prog.TypeAt(leaf).Flags
	if flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
		return false
	}
	if r.dynLeafUnboxes(leaf) {
		return false
	}
	// A leaf the boxed-signature pass gave a value.Value slot holds a box whatever its
	// source is, the same way boxedChainBinding reads that mark ahead of its own test.
	if r.isBoxedLocalRead(leaf) {
		return true
	}
	return r.isDynamic(init) || r.isBoxedChain(init)
}

// markPatternBoxLeaves records, by symbol, every module-level destructuring leaf whose Go
// slot holds a box, and reports whether it added anything.
//
// It is a round of the boxed-signature fixpoint rather than a pass of its own, because it
// both reads that pass's answers and feeds them. It reads them where a leaf's source is a
// call whose return the pass boxed; it feeds them where the leaf is passed to a function,
// since the parameter's slot is decided by what flows into it and a leaf whose box was
// still unknown would have driven a coercion into a static struct that has none. Marks are
// only added here, never taken back, which is the property the fixpoint rests on.
//
// Only module-level patterns are walked. A leaf of a pattern inside a function body is read
// inside that body, where dynBoundLocals already answers, and the question this set exists
// for is a read that leaves the body that bound the name.
//
// The symbol is the key rather than the name, since two patterns in two scopes can bind the
// same spelling and only one of them is this binding.
func (r *Renderer) markPatternBoxLeaves(files []frontend.Node) bool {
	changed := false
	for _, file := range files {
		for _, stmt := range r.prog.Children(file) {
			if stmt.Kind() != frontend.NodeVariableStatement {
				continue
			}
			var decls []frontend.Node
			collectVarDecls(r.prog, stmt, &decls)
			for _, d := range decls {
				kids := r.prog.Children(d)
				if len(kids) < 2 || !r.patternNode(kids[0]) {
					continue
				}
				var leaves []frontend.Node
				if !r.patternLeafNodes(kids[0], &leaves) {
					continue
				}
				init := kids[len(kids)-1]
				for _, leaf := range leaves {
					sym, ok := r.prog.SymbolAt(leaf)
					if !ok || r.patternBoxLeaves[sym] || !r.patternLeafBindsABox(leaf, init) {
						continue
					}
					if r.patternBoxLeaves == nil {
						r.patternBoxLeaves = map[frontend.Symbol]bool{}
					}
					r.patternBoxLeaves[sym] = true
					changed = true
				}
			}
		}
	}
	return changed
}

// isPatternBoxLeaf reports whether an identifier names a module-level destructuring leaf
// whose slot holds a box, the answer markModulePatternBoxLeaves settled.
func (r *Renderer) isPatternBoxLeaf(n frontend.Node) bool {
	if len(r.patternBoxLeaves) == 0 || n.Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(n)
	return ok && r.patternBoxLeaves[sym]
}
