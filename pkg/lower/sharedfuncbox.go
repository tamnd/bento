package lower

import (
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// collectSharedFuncBoxes records the module-level bindings whose value exists exactly
// once in a run, so a function boxed off one of them can be boxed to the same
// value.Value every time rather than to a fresh wrapper per site.
//
// Boxing wraps a Go func in a value.NewFunc closure built at the site that needs it, so
// the same function boxed at two sites produces two wrappers with no relation to each
// other. In JavaScript those are one object:
//
//	const h = () => {}
//	process.on('x', h)
//	process.off('x', h)      // finds it, because both are the same function
//
// With a wrapper per site the off compares two different values and removes nothing,
// and the same gap is under `fns.indexOf(h)`, `set.has(h)`, and a plain `h === h` read
// through a dynamic slot. The fix is to give the two sites one box, which is what the
// key recorded here buys: the runtime memoizes the first wrapper under it and every
// later site gets that one back.
//
// The condition is that the binding holds one function for the life of the run, and the
// case that plainly does is a declaration written directly in a module's top-level
// statement list: it is evaluated once when the module runs, and a module runs once. A
// declaration inside a function or a loop is deliberately left out, since each call or
// each turn makes its own closure and one box for all of them would say two different
// functions are the same. Those keep a wrapper per site, which is what every box did
// before this.
func (r *Renderer) collectSharedFuncBoxes(files []frontend.Node) {
	if r.sharedFuncBoxes == nil {
		r.sharedFuncBoxes = map[frontend.Symbol]string{}
	}
	// The key names the module by its position in the file set rather than by its path,
	// so the emitted Go says the same thing wherever the source tree happens to sit. It
	// only has to be unique across the program and stable across a rebuild.
	for i, f := range files {
		for _, stmt := range r.prog.Children(f) {
			for _, nameNode := range r.moduleLevelBindingNames(stmt) {
				sym, ok := r.prog.SymbolAt(nameNode)
				if !ok {
					continue
				}
				r.sharedFuncBoxes[sym] = strconv.Itoa(i) + "#" + sym.Name
			}
		}
	}
}

// moduleLevelBindingNames returns the name nodes a top-level statement binds once for
// the life of the module: a function declaration's own name, and the identifiers a
// const statement binds. A let or a var is left out because a later assignment would
// put a different function behind the name, and the shared box would then hand back the
// one the first site saw.
//
// An export modifier changes nothing about how often the statement runs, so
// `export function f() {}` is here too, read through the same statement kinds.
func (r *Renderer) moduleLevelBindingNames(stmt frontend.Node) []frontend.Node {
	switch stmt.Kind() {
	case frontend.NodeFunctionDeclaration:
		kids := r.prog.Children(stmt)
		if len(kids) > 0 && kids[0].Kind() == frontend.NodeIdentifier {
			return kids[:1]
		}
		return nil
	case frontend.NodeVariableStatement:
		if !r.isConstStatement(stmt) {
			return nil
		}
		var out []frontend.Node
		for _, nn := range r.varNameNodes(stmt) {
			if nn.Kind() == frontend.NodeIdentifier {
				out = append(out, nn)
			}
		}
		return out
	}
	return nil
}

// sharedFuncBoxKey answers the key a boxed reference shares its wrapper under, and
// whether it has one. Only a plain reference to a recorded binding does: a call result
// or an inline literal is a new function each time it is evaluated, which is exactly
// what a wrapper per site already says.
func (r *Renderer) sharedFuncBoxKey(src frontend.Node) (string, bool) {
	if src == nil || src.Kind() != frontend.NodeIdentifier || len(r.sharedFuncBoxes) == 0 {
		return "", false
	}
	sym, ok := r.prog.SymbolAt(src)
	if !ok {
		return "", false
	}
	key, ok := r.sharedFuncBoxes[sym]
	return key, ok
}
