package lower

import "github.com/tamnd/bento/pkg/frontend"

// A CommonJS file's top level is its own scope. Node wraps every module in a
// function before it runs, so a top-level `const name = ...` is a local of that
// wrapper and has nothing to do with any global of the same name.
//
// The checker does not see it that way. A .js file with no import and no export is
// a script, and a script's top level is the global scope, so a top-level
// declaration whose name a library already declares collides with it. The standard
// library bento checks against is lib.es2022.full, which includes the DOM globals,
// and that is where `name`, `length`, `origin`, `status`, `self`, `top`, `event`
// and a dozen more come from. Every one of those is a name an ordinary Node program
// binds without a second thought:
//
//	const name = 'ev' + 1
//	process.on(name, fn)
//
// What the checker does with that is not a shadow, it is two symbols. The
// declaration gets a symbol whose declaration is the file's own const, and every
// reference gets the library's symbol, whose only declaration is a .d.ts. So the
// program says `name` and the checker answers with `declare var name: string`.
//
// Nothing downstream can repair that. Lowering the reference as the global answers
// for a binding the program never meant, which is a wrong answer; lowering it as
// the file's binding means overriding the checker's resolution at every site that
// reads a symbol, which is the checker's job and not a lowering rule. So the unit
// hands back, naming what happened, and stays honest until the declared global
// surface matches what a Node program actually has (dropping dom from the lib set
// and declaring the web globals bento hosts in aot_ambient.go, which is a slice of
// its own).
//
// A global bento declares itself is left alone. `const process = require('node:process')`
// re-binds the same object bento's ambient process names, so the reference resolving
// to the ambient declaration is the right answer rather than a collision, and that
// pattern is everywhere in Node code.

// collectFileScopeNames records, per file, the names the file binds at its top
// level, and then finds the references in that file the checker resolved to a
// standard library declaration of the same name. The first such reference is kept
// as the collision the render hands back on.
func (r *Renderer) collectFileScopeNames(files []frontend.Node) {
	if r.fileScopeNames == nil {
		r.fileScopeNames = map[string]map[string]bool{}
	}
	for _, f := range files {
		path := f.File().Path
		names := r.fileScopeNames[path]
		if names == nil {
			names = map[string]bool{}
			r.fileScopeNames[path] = names
		}
		for _, stmt := range r.prog.Children(f) {
			for _, name := range r.topLevelDeclaredNames(stmt) {
				names[name] = true
			}
		}
	}
	for _, f := range files {
		r.findShadowedGlobal(f)
	}
}

// topLevelDeclaredNames returns the names a top-level statement binds in its file's
// scope. Every binding form counts, not just the ones that hold a single value for
// the life of the run: the question here is which name the program means, and a
// `let name` collides with the library global exactly as firmly as a `const name`
// does.
//
// A destructuring pattern is read through the same name-node walk the variable
// paths use, so `const { name } = o` is in here too.
func (r *Renderer) topLevelDeclaredNames(stmt frontend.Node) []string {
	switch stmt.Kind() {
	case frontend.NodeFunctionDeclaration, frontend.NodeClassDeclaration:
		kids := r.prog.Children(stmt)
		if len(kids) > 0 && kids[0].Kind() == frontend.NodeIdentifier {
			return []string{r.prog.Text(kids[0])}
		}
		return nil
	case frontend.NodeVariableStatement:
		var out []string
		for _, nn := range r.varNameNodes(stmt) {
			if nn.Kind() == frontend.NodeIdentifier {
				out = append(out, r.prog.Text(nn))
			}
		}
		return out
	}
	return nil
}

// findShadowedGlobal walks a file for a reference the checker sent to the standard
// library while the file binds that same name at its own top level. It records the
// first one it finds; one is enough, since the whole unit hands back on it.
func (r *Renderer) findShadowedGlobal(n frontend.Node) {
	if r.shadowedGlobal != "" {
		return
	}
	if n.Kind() == frontend.NodeIdentifier && r.referenceMissesFileBinding(n) {
		r.shadowedGlobal = r.prog.Text(n)
		return
	}
	for _, c := range r.prog.Children(n) {
		r.findShadowedGlobal(c)
	}
}

// referenceMissesFileBinding reports whether n names something its own file binds at
// top level while resolving to a standard library declaration instead. The
// declaration node itself is not one: its symbol is the file's own binding, so it
// fails the library test below.
func (r *Renderer) referenceMissesFileBinding(n frontend.Node) bool {
	if !r.fileBindsName(n) {
		return false
	}
	sym, ok := r.prog.SymbolAt(n)
	if !ok {
		return false
	}
	decls := r.prog.Declarations(sym)
	if len(decls) == 0 {
		return false
	}
	for _, d := range decls {
		// A declaration bento wrote is not a collision: the binding re-names the object
		// bento's global already stands for, so resolving to the ambient declaration is
		// the answer the program wants.
		if d.File().Kind != frontend.FileDTS || frontend.IsBentoAmbientPath(d.File().Path) {
			return false
		}
	}
	return true
}

// fileBindsName reports whether the file holding n binds n's name at its own top
// level, the test that separates a program's own binding from a library global that
// happens to share the name.
func (r *Renderer) fileBindsName(n frontend.Node) bool {
	if len(r.fileScopeNames) == 0 {
		return false
	}
	return r.fileScopeNames[n.File().Path][r.prog.Text(n)]
}
