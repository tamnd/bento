package lower

import (
	"go/ast"
	"go/token"
	"sort"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the CommonJS module system reached through require. A sibling
// composed by static import contributes its declarations to one shared package
// (modules.go); a sibling reached by require is different, because require runs
// the target module's body once, on the first call, and hands back whatever that
// body left on module.exports. So a required module cannot flatten into the
// entry's package declarations: it needs a body that runs, a place to cache the
// result, and a value the caller receives.
//
// bento models each required module as a loader function guarding a package-level
// cache slot (value.ModuleSlot). The loader runs the module's top-level statements
// once, wrapped so module and exports are the loader's own locals rather than the
// entry's package globals, and returns the module's exports. A require of that
// module lowers to a direct call on its loader, so the module cache, the run-once
// rule, and the partial-exports behaviour of a circular require all come from the
// slot the loader consults. A specifier require cannot resolve statically, or one
// whose target this slice cannot lower, keeps the throwing runtime require, so a
// program that reaches for a module bento does not compose fails honestly rather
// than resolving to a wrong value.

// discoverRequiredModules records every module reached by a require('<literal>')
// call anywhere in the composed file set, keyed by its resolved absolute path and
// mapped to the Go name of the loader function that runs it. It runs before any
// body lowers so a require call site in the entry, or in one required module for
// another, resolves to a loader name already in hand. The loader names are assigned
// in sorted path order, so the mapping is deterministic across runs and two modules
// never share a name.
func (r *Renderer) discoverRequiredModules(files []frontend.Node) {
	paths := map[string]bool{}
	for _, f := range files {
		for _, p := range r.requireTargets(f) {
			paths[p] = true
		}
	}
	if len(paths) == 0 {
		return
	}
	sorted := make([]string, 0, len(paths))
	for p := range paths {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	if r.requiredLoaders == nil {
		r.requiredLoaders = map[string]string{}
	}
	for i, p := range sorted {
		r.requiredLoaders[p] = "bentoModRun_" + strconv.Itoa(i)
	}
}

// requireTargets returns the resolved absolute paths of every module the file
// reaches through a require call whose specifier is a string literal. It walks the
// whole file so a require nested in a function body or a conditional is found too,
// and resolves each literal specifier against the file's import edges, the same map
// a static import resolves through, so the path is the one the frontend loaded. A
// require whose argument is not a static string, or whose specifier resolves to
// nothing or to a declaration file, contributes no target and keeps the throwing
// runtime require at its call site.
func (r *Renderer) requireTargets(file frontend.Node) []string {
	var out []string
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		if n.Kind() == frontend.NodeCallExpression {
			kids := r.prog.Children(n)
			if len(kids) == 2 && r.isGlobalRef(kids[0], "require") {
				if path, ok := r.resolveRequire(file, kids[1]); ok {
					out = append(out, path)
				}
			}
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	walk(file)
	return out
}

// resolveRequire resolves a require call's specifier argument to the absolute path
// of the module it names, or reports ok=false when the call is not a static require
// of a composed module. The argument must be a string literal, and its value must
// match one of the file's relative import edges that resolved to a real source
// file, not a declaration file. The file the call sits in is passed rather than
// read off the argument, since a require in a nested scope still resolves against
// its own module's edges.
func (r *Renderer) resolveRequire(file, arg frontend.Node) (string, bool) {
	if arg.Kind() != frontend.NodeStringLiteral {
		return "", false
	}
	specifier := unquote(r.prog.Text(arg))
	for _, imp := range r.prog.Imports(file.File()) {
		if imp.Specifier != specifier || imp.Kind != frontend.ImportRelative {
			continue
		}
		if imp.Resolved.Path == "" || imp.Resolved.Kind == frontend.FileDTS {
			continue
		}
		return imp.Resolved.Path, true
	}
	return "", false
}

// requireModuleCall lowers require('<literal>') to a direct call on the target
// module's loader when the specifier resolves to a composed required module,
// returning handled=false when it does not so the caller keeps the throwing runtime
// require. The loader returns the module's exports as a value.Value, the value the
// require expression takes, and guards its own module cache, so the run-once and
// caching behaviour needs nothing at the call site beyond the call itself.
func (r *Renderer) requireModuleCall(callee frontend.Node, argNodes []frontend.Node) (ast.Expr, bool, error) {
	if len(argNodes) != 1 {
		return nil, false, nil
	}
	path, ok := r.resolveRequire(callee, argNodes[0])
	if !ok {
		return nil, false, nil
	}
	loader, ok := r.requiredLoaders[path]
	if !ok {
		return nil, false, nil
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: ident(loader)}, true, nil
}

// isRequireModuleCall reports whether a node is a require('<literal>') call whose
// specifier resolves to a module this build composed as a loader. It is how a
// binding initialized straight from a require tells a composed-module result, the
// boxed value.Value the loader returns, apart from any other initializer, so the
// binding lands in a value.Value slot rather than the static struct the module's
// inferred exports type would otherwise name. Any node in the call's file resolves
// the same import edges, so the call node stands in for the file the require sits in.
func (r *Renderer) isRequireModuleCall(n frontend.Node) bool {
	if n.Kind() != frontend.NodeCallExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 || !r.isGlobalRef(kids[0], "require") {
		return false
	}
	path, ok := r.resolveRequire(n, kids[1])
	if !ok {
		return false
	}
	_, ok = r.requiredLoaders[path]
	return ok
}

// renderRequiredModules emits, for each module reached by require, its cache slot
// package var and its loader function, or nil when the program required nothing.
// It runs after the entry body lowers, so the per-module analysis state it
// overwrites for each loader is already spent, and before the assembled file reads
// the interner, so a struct or union a module body interns still emits. A module
// this slice cannot lower (one carrying a top-level function, class, or enum
// declaration) hands the whole unit back here, the same bar the import-composed
// siblings meet.
func (r *Renderer) renderRequiredModules(reqDeps []frontend.Node) ([]ast.Decl, error) {
	if len(r.requiredLoaders) == 0 {
		return nil, nil
	}
	byPath := make(map[string]frontend.Node, len(reqDeps))
	for _, dep := range reqDeps {
		byPath[dep.File().Path] = dep
	}
	paths := make([]string, 0, len(r.requiredLoaders))
	for p := range r.requiredLoaders {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var decls []ast.Decl
	for _, path := range paths {
		file, ok := byPath[path]
		if !ok {
			return nil, &NotYetLowerable{Reason: "a required module the frontend did not stage is a later slice"}
		}
		loaderDecls, err := r.renderRequiredModule(path, file)
		if err != nil {
			return nil, err
		}
		decls = append(decls, loaderDecls...)
	}
	return decls, nil
}

// renderRequiredModule builds the cache slot var and loader function for one
// required module. The loader consults the slot, runs the module body once with
// module and exports bound to its own locals, and returns the module's exports.
func (r *Renderer) renderRequiredModule(path string, file frontend.Node) ([]ast.Decl, error) {
	slot := r.requiredLoaders[path]
	// The run function is named bentoModRun_N; its slot shares the number under
	// bentoMod_N, so a reader pairs the two by eye.
	slotVar := strings.Replace(slot, "bentoModRun_", "bentoMod_", 1)

	body, usesExports, err := r.lowerRequiredModuleBody(file)
	if err != nil {
		return nil, err
	}

	r.requireImport(valuePkg)
	// var bentoMod_N = value.NewModuleSlot()
	slotDecl := &ast.GenDecl{
		Tok: token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{
			Names:  []*ast.Ident{ident(slotVar)},
			Values: []ast.Expr{&ast.CallExpr{Fun: sel("value", "NewModuleSlot")}},
		}},
	}

	slotCall := func(method string, args ...ast.Expr) *ast.CallExpr {
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(slotVar), Sel: ident(method)}, Args: args}
	}

	var list []ast.Stmt
	// if bentoMod_N.Loaded() { return bentoMod_N.Exports() }
	list = append(list, &ast.IfStmt{
		Cond: slotCall("Loaded"),
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.ReturnStmt{Results: []ast.Expr{slotCall("Exports")}},
		}},
	})
	// module := bentoMod_N.Init()
	list = append(list, &ast.AssignStmt{
		Lhs: []ast.Expr{ident("module")},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{slotCall("Init")},
	})
	// exports := module.Get(value.FromGoString("exports")) only when the body read it.
	if usesExports {
		list = append(list, &ast.AssignStmt{
			Lhs: []ast.Expr{ident("exports")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: ident("module"), Sel: ident("Get")},
				Args: []ast.Expr{r.goStringValue("exports")},
			}},
		})
	}
	list = append(list, body...)
	// return bentoMod_N.Finish(module)
	list = append(list, &ast.ReturnStmt{Results: []ast.Expr{slotCall("Finish", ident("module"))}})

	loader := &ast.FuncDecl{
		Name: ident(slot),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "Value")}}},
		},
		Body: &ast.BlockStmt{List: list},
	}
	return []ast.Decl{slotDecl, loader}, nil
}

// lowerRequiredModuleBody lowers a required module's top-level statements to the
// body of its loader, returning whether the body read the exports local so the
// loader declares it only when it is used. It runs the same per-statement analysis
// the entry body runs (the storage tiers, the use counts, the string-builder and
// var hoists), so a module body computes the same way whether it is the entry or a
// required module; the difference is only that module and exports are loader-locals
// here, which requiredModuleActive routes in commonjs.go. A module carrying a
// top-level function, class, or enum declaration hands back, since those would need
// a Go declaration that cannot see the loader's locals, a later slice.
func (r *Renderer) lowerRequiredModuleBody(file frontend.Node) ([]ast.Stmt, bool, error) {
	r.requiredModuleActive = true
	r.reqUsesExports = false
	defer func() { r.requiredModuleActive = false }()

	var mainBody []frontend.Node
	var mainItems []mainItem
	pushStmt := func(n frontend.Node) {
		mainBody = append(mainBody, n)
		mainItems = append(mainItems, mainItem{node: n})
	}
	for _, stmt := range r.prog.Children(file) {
		switch stmt.Kind() {
		case frontend.NodeFunctionDeclaration:
			return nil, false, &NotYetLowerable{Reason: "a required module with a top-level function declaration is a later slice"}
		case frontend.NodeClassDeclaration:
			return nil, false, &NotYetLowerable{Reason: "a required module with a top-level class declaration is a later slice"}
		case frontend.NodeEnumDeclaration:
			return nil, false, &NotYetLowerable{Reason: "a required module with a top-level enum declaration is a later slice"}
		case frontend.NodeInterfaceDeclaration, frontend.NodeTypeAliasDeclaration:
			// Type-level declarations carry no runtime code.
			continue
		case frontend.NodeUnknown:
			// An import declaration is recorded in the pre-pass and carries no code; the
			// end-of-file token is empty. Any other unnamed statement routes to the body
			// like the entry's does, and hands back there if the statement subset does not
			// cover it.
			text := strings.TrimSpace(r.prog.Text(stmt))
			if text != "" && !strings.HasPrefix(text, "import") {
				pushStmt(stmt)
			}
		default:
			pushStmt(stmt)
		}
	}

	// The loader body has no top-level function or class (they hand back above), so no
	// binding crosses a declaration boundary and nothing hoists to package scope; the
	// module's bindings are all loader locals. The in-place module-assignment set is
	// reset for the same reason the entry resets it.
	r.moduleAssignVars = map[string]bool{}
	r.programStrict = r.hasUseStrictPrologue(file)

	// The use counts drive the blank-declaration decision the same way they do for the
	// entry: a local declared and never read gets Go's blank identifier so the emitted
	// function builds.
	r.bindingUses = countBindingUses(r.prog, file)
	r.elidedUses = countElidedReads(r, file)
	r.writeUses = countWriteUses(r.prog, file)
	r.bindingDecls = countBindingDecls(r.prog, file)
	r.collectStoredCollIters(file)
	r.collectCollMutations(file)

	// The storage tiers specialize the loader body's integer locals the same way the
	// entry's are, reading the whole body before it lowers. No binding is hoisted, so
	// none is held off a tier the way a package-level entry var is.
	r.constInt = r.constIntsOf(mainBody)
	r.int32Locals = r.int32LocalsOf(mainBody)
	r.counterIvl = r.counterIvlOf(mainBody)
	r.int64Locals = r.int64LocalsOf(mainBody)
	r.fixedTArr = r.fixedTypedArraysOf(mainBody)
	r.optLocals = r.optLocalsOf(mainBody)
	r.definiteLocals = r.definiteLocalsOf(mainBody)
	r.unionLocals = r.unionLocalsOf(nil, mainBody)
	r.dynLocals = r.dynLocalsOf(nil, mainBody)
	r.bigOwned = r.bigOwnedLocalsOf(mainBody)
	r.strBuilders = nil

	hoistDecls, restoreHoist, err := r.enterVarHoistScope(mainBody)
	if err != nil {
		return nil, false, err
	}
	fwdDecls, restoreFwd, err := r.enterFwdHoistScope(mainBody)
	if err != nil {
		restoreHoist()
		return nil, false, err
	}
	stmts, err := r.lowerMainItems(mainItems)
	restoreHoist()
	restoreFwd()
	if err != nil {
		return nil, false, err
	}
	stmts = append(hoistDecls, stmts...)
	stmts = append(fwdDecls, stmts...)
	stmts = r.hoistStrBuilders(stmts)

	return stmts, r.reqUsesExports, nil
}
