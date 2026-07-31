package lower

import (
	"go/ast"
	"go/token"
	"sort"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers `import assert from 'node:assert'`, the ESM form of a require of
// a built-in the runtime registry answers. The require form already lands its
// binding in a value.Value slot and reads every member through the value model
// (require_modules.go); this gives the import form the same binding, so the two
// specifier styles a Node program is written in reach one module value and cannot
// answer differently.
//
// It is deliberately not the node:fs, node:os, node:path and node:util path in
// nodefs.go. Those four lower each member to a named value helper, a direct call
// with no boxing at all, which is what a syscall-shaped workload wants. A module
// the registry builds has no such helpers: its members are values hanging off a
// module object, so the binding holds the object and each member read goes through
// Get. Both are right for what they carry, so the two live side by side and
// recordNodeImport asks the static set first.
//
// The bindings emit as package-level vars assigned at the top of main rather than
// as main locals. A top-level function that calls an imported assert is the shape
// half of Node's test files take, and a Go func cannot see main's locals; a package
// var both can name closes that. They are assigned in main rather than initialized
// at package-init time so a module whose load throws (the honest-stub trap on a
// member read) raises where the program's own error reporting can catch it, rather
// than as a Go panic before main starts.

// builtinModuleBinding is what one imported name stands for: the module specifier it
// came from, and the member of that module it names. An empty member is the module
// object itself, the default and namespace forms, since a Node built-in is CommonJS
// and its default export is the module.
type builtinModuleBinding struct {
	module string
	member string
}

// recordBuiltinModuleImport records the bindings of an import from a built-in the
// registry answers. A bare side-effect import binds nothing and lowers to nothing:
// loading a built-in has no effect a program can observe, so there is nothing to
// emit and nothing to hand back for. Every other form binds names, and each name
// becomes a package-level value.Value the emitter fills at the top of main.
func (r *Renderer) recordBuiltinModuleImport(module string, clause frontend.Node, haveClause bool) error {
	if !haveClause {
		return nil
	}
	if r.builtinModuleImports == nil {
		r.builtinModuleImports = map[string]builtinModuleBinding{}
	}
	objects := moduleObjectBindings(r.prog, clause)
	for _, binding := range objects {
		r.builtinModuleImports[binding] = builtinModuleBinding{module: module}
	}
	named, ok := namedImportsNode(r.prog, clause)
	if !ok {
		if len(objects) > 0 {
			return nil
		}
		return &NotYetLowerable{Reason: "import of " + module + " in this form is a later slice"}
	}
	for _, spec := range r.prog.Children(named) {
		names := identChildren(r.prog, spec)
		if len(names) == 0 {
			return &NotYetLowerable{Reason: "import specifier of " + module + " exposed no name"}
		}
		// The first identifier is the exported name and the last is the local binding,
		// so an aliased import (import { ok as assertOk }) reads the export it names and
		// binds the name the program spells.
		r.builtinModuleImports[names[len(names)-1]] = builtinModuleBinding{module: module, member: names[0]}
	}
	return nil
}

// builtinModuleImportNames returns the bound names in sorted order, so the emitted
// declarations and the assignments that fill them come out in the same order on every
// run and a build is reproducible.
func (r *Renderer) builtinModuleImportNames() []string {
	if len(r.builtinModuleImports) == 0 {
		return nil
	}
	names := make([]string, 0, len(r.builtinModuleImports))
	for name := range r.builtinModuleImports {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// builtinModuleImportDecls emits one package-level `var name value.Value` per bound
// name. The var is declared without an initializer and assigned at the top of main by
// builtinModuleImportInit, so the module load runs inside the program's own error
// handling rather than at package-init time.
func (r *Renderer) builtinModuleImportDecls() []ast.Decl {
	names := r.builtinModuleImportNames()
	if len(names) == 0 {
		return nil
	}
	r.requireImport(valuePkg)
	specs := make([]ast.Spec, 0, len(names))
	for _, name := range names {
		specs = append(specs, &ast.ValueSpec{
			Names: []*ast.Ident{ident(name)},
			Type:  sel("value", "Value"),
		})
	}
	return []ast.Decl{&ast.GenDecl{Tok: token.VAR, Specs: specs}}
}

// builtinModuleImportInit emits the assignments that fill those vars, the module load
// itself. A module-object binding takes the registry's module value; a named import
// takes the member read off it, which is what the import means: the name is bound at
// load time, so a member the module does not carry raises the built-in's honest-stub
// error as the program starts rather than at the first call.
func (r *Renderer) builtinModuleImportInit() []ast.Stmt {
	names := r.builtinModuleImportNames()
	if len(names) == 0 {
		return nil
	}
	r.requireImport(valuePkg)
	stmts := make([]ast.Stmt, 0, len(names))
	for _, name := range names {
		binding := r.builtinModuleImports[name]
		var rhs ast.Expr = &ast.CallExpr{
			Fun:  sel("value", "RequireBuiltin"),
			Args: []ast.Expr{stringLit(binding.module)},
		}
		if binding.member != "" {
			rhs = &ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: rhs, Sel: ident("Get")},
				Args: []ast.Expr{r.goStringValue(binding.member)},
			}
		}
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(name)},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{rhs},
		})
	}
	return stmts
}

// isDynBoundName reports whether a name reads as a boxed value.Value: a binding the
// body currently lowering marked dynBound, or one an ESM import of a registry
// built-in bound to a module value. The two are one question at every site that asks
// it, since both hold a box whatever type the checker gave the name.
//
// The built-in half is keyed by name rather than by symbol, the same way the node:
// import bindings beside it are, so a nested binding that shadows an imported name
// would read as the module here. Every form that could shadow it is a later slice's
// problem to key precisely; the import bindings are module-level and a program that
// rebinds one is not the program this path exists for.
func (r *Renderer) isDynBoundName(name string) bool {
	if r.dynBoundLocals[name] {
		return true
	}
	_, ok := r.builtinModuleImports[name]
	return ok
}
