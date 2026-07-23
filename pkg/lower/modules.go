package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file composes the sibling modules a module-goal entry imports into the one
// Go program the entry lowers to. A static import in TypeScript names a binding
// another module exports; bento lowers that binding to a package-level Go
// declaration, so an import of it needs no runtime indirection: the reference
// spells the same Go name the declaration takes, the way a call to a top-level
// function in the same file does. The build stages the entry and its siblings and
// hands them all to RenderProgramModules; this pass registers each sibling's
// declarations into the shared renderer state and returns its functions to emit
// beside the entry's.
//
// The slice composes a sibling's declarations, not its top-level evaluation. A
// module's exported functions, classes, enums, and type aliases are position
// independent at package scope, so they compose cleanly. A module's top-level
// side effects (a variable initializer that runs, a bare statement, a class
// static block) run in a defined order relative to the modules that import it,
// and threading that order across the composed boundary is a later slice, so a
// sibling that carries one hands back.

// derefAlias resolves an import-alias symbol to the symbol it names, so a
// reference to a binding imported from a sibling module carries the flags and the
// name its declaration took. A name that is not an alias is returned unchanged, so
// the call is safe on any symbol. It is what lets a reference in the entry spell
// the same Go declaration a call within the sibling would.
func (r *Renderer) derefAlias(sym frontend.Symbol) frontend.Symbol {
	if sym.Flags&frontend.SymbolAlias != 0 {
		return r.prog.Aliased(sym)
	}
	return sym
}

// internalNamespaceCall lowers a call whose callee is a member of a sibling
// namespace import, m.inc(1) where m is import * as m from "./m". The sibling's
// exports are package-level Go declarations, so the call resolves to the export's
// Go func and lowers to a direct call, the same spelling a named import of inc
// would take. It returns handled=false when the object is not a namespace binding,
// so the caller keeps its other member-callee paths, and a hand-back for a member
// this slice does not resolve to a plain function call: a member that is not an
// exported function, an overloaded or generic or defaulting export whose call needs
// the machinery the bare-identifier path threads, keeps the whole unit truthful by
// routing to the engine rather than emitting a call the composition cannot back.
func (r *Renderer) internalNamespaceCall(n, access frontend.Node, argNodes []frontend.Node) (ast.Expr, bool, error) {
	kids := r.prog.Children(access)
	if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier || kids[1].Kind() != frontend.NodeIdentifier {
		return nil, false, nil
	}
	if !r.internalNamespaces[r.prog.Text(kids[0])] {
		return nil, false, nil
	}
	name, err := r.namespaceMemberFunc(kids[1])
	if err != nil {
		return nil, true, err
	}
	// A sibling export that reads its arguments object needs the same arity guard a
	// same-module call gets, so the member symbol is resolved to the export it names
	// and checked before the direct call is built.
	calleeReadsArgs := false
	calleeThreadsArgs := false
	if sym, ok := r.prog.SymbolAt(kids[1]); ok {
		target := r.derefAlias(sym)
		// A member access such as m.f is not a direct call of f, so funcSymCallShape sees
		// a non-call reference and never threads a namespace-called export; this stays
		// false and the export keeps the snapshot arity guard. The predicate is consulted
		// anyway so the declaration and this call site read the same answer.
		calleeThreadsArgs = r.funcSymThreadsArgs(target)
		calleeReadsArgs = !calleeThreadsArgs && r.funcSymReadsArguments(target)
	}
	expr, err := r.finishCall(n, ident(name), argNodes, nil, false, calleeReadsArgs, calleeThreadsArgs)
	return expr, true, err
}

// namespaceMemberFunc resolves the member of a sibling namespace import, the name
// node of `m.f`, to the Go func name its export lowered to, or a hand-back when the
// member is not a plain exported function this composition can name. The call form
// `m.inc(1)` and the value read `const f = m.inc` both resolve to the same
// package-level Go func, the call spelling `Inc(1)` and the read the bare `Inc`, so
// both go through here.
//
// The member resolves through the checker to the sibling export it names; the alias
// it carries derefs to that export's own symbol, which holds the function flag and
// the name the declaration lowered to. An overloaded, generic, or defaulting export
// needs the argument bridging the bare-identifier path threads (a boxed all-dynamic
// implementation, a monomorphized name, a call-site default fill), which a bare Go
// name reference does not carry, so any of those hands back. A member that is not an
// exported function, a const export with no Go value behind it, hands back too.
// namespaceMemberValue resolves a member of a sibling namespace import read as a
// value, m.f or m.K, to the Go name its export lowered to. A function member reads
// as the bare package func name, the same the call form m.f(1) resolves. A const or
// let member reads as the export's package-level Go var, the value the const-export
// composition (#589) materializes: an exported literal const or let composes as a
// package var under its localName, so m.K reads that same localName the way a named
// import of K does. The variable path spells localName because the composition keeps
// the variable convention (a function capitalizes, a variable is preserved), and a
// non-literal const export never reaches here because the sibling hands the whole
// unit back at moduleFuncs before the entry lowers this read. A member that is
// neither a plain function nor a composable variable, a class or an export with no
// Go value behind it, hands back through the function resolver.
func (r *Renderer) namespaceMemberValue(nameNode frontend.Node) (string, error) {
	if sym, ok := r.prog.SymbolAt(nameNode); ok {
		if target := r.derefAlias(sym); target.Flags&frontend.SymbolVariable != 0 && target.Flags&frontend.SymbolFunction == 0 {
			if goName, ok := localName(target.Name); ok {
				return goName, nil
			}
			return "", &NotYetLowerable{Reason: "a namespace member variable whose name is not a Go identifier is a later slice"}
		}
	}
	return r.namespaceMemberFunc(nameNode)
}

func (r *Renderer) namespaceMemberFunc(nameNode frontend.Node) (string, error) {
	sym, ok := r.prog.SymbolAt(nameNode)
	if !ok {
		return "", &NotYetLowerable{Reason: "a namespace member that does not resolve to an export is a later slice"}
	}
	sym = r.derefAlias(sym)
	if sym.Flags&frontend.SymbolFunction == 0 {
		return "", &NotYetLowerable{Reason: "a namespace member that is not an exported function is a later slice"}
	}
	if _, ok := r.overloadedFuncImpl(sym); ok {
		return "", &NotYetLowerable{Reason: "a namespace member that is an overloaded export is a later slice"}
	}
	if len(r.monoSpecs[sym]) > 0 {
		return "", &NotYetLowerable{Reason: "a namespace member that is a generic export is a later slice"}
	}
	if r.funcOmittable(sym) {
		return "", &NotYetLowerable{Reason: "a namespace member that is an export with an omittable parameter is a later slice"}
	}
	name, ok := exportedField(sym.Name)
	if !ok {
		return "", &NotYetLowerable{Reason: "a namespace member whose name is not a Go identifier is a later slice"}
	}
	return name, nil
}

// collectModules registers the composed sibling modules and returns their
// top-level function declarations. Each sibling runs the same declaration
// pre-passes the entry does, so its classes, enums, and generic instantiations
// join the shared renderer state and an entry call site resolves against them;
// its functions come back to emit as package funcs. A sibling this slice cannot
// compose hands back, routing the whole unit to the engine before the entry
// lowers.
func (r *Renderer) collectModules(deps []frontend.Node) ([]ast.Decl, error) {
	for _, dep := range deps {
		if err := r.checkMangleCollisions(dep); err != nil {
			return nil, err
		}
		// A sibling's own imports record here too, so a sibling that imports another
		// sibling, or a node: or go: builtin, binds the same way the entry does; an
		// import the sibling makes that bento does not lower hands back.
		if err := r.collectNodeImports(dep); err != nil {
			return nil, err
		}
		if err := r.collectClasses(dep); err != nil {
			return nil, err
		}
		if err := r.collectEnums(dep); err != nil {
			return nil, err
		}
		r.collectMono(dep)
		r.collectMonoMethods(dep)
		r.collectArrowDefaults(dep)
	}

	var funcs []ast.Decl
	for _, dep := range deps {
		fs, err := r.moduleFuncs(dep)
		if err != nil {
			return nil, err
		}
		funcs = append(funcs, fs...)
	}
	return funcs, nil
}

// moduleFuncs walks a composed sibling's top level and returns its function
// declarations, guarding that the module is declaration-only. A function lowers
// to a package Go func the entry can call. A class, enum, interface, or type
// alias already registered in the pre-pass and emits with the entry's
// declarations, so it contributes nothing here. An import recorded already and
// carries no code. Anything else, a variable statement or a runtime statement
// whose evaluation order the composed unit would have to preserve, hands the
// whole unit back, since this slice composes a module's declarations, not its top
// level running.
func (r *Renderer) moduleFuncs(dep frontend.Node) ([]ast.Decl, error) {
	var funcs []ast.Decl
	for _, stmt := range r.prog.Children(dep) {
		switch stmt.Kind() {
		case frontend.NodeFunctionDeclaration:
			// An overload set lowers like the entry's: the bodyless signatures carry no
			// code, and a set whose implementation this slice cannot claim hands back
			// rather than emit a partial function.
			if sym, ok := r.prog.SymbolAt(stmt); ok {
				if _, isOverload := r.overloadImplNode(sym); isOverload {
					if _, claimed := r.overloadedFuncImpl(sym); !claimed {
						return nil, &NotYetLowerable{Reason: "an overloaded function in a sibling module is a later slice"}
					}
					if _, hasBody := r.funcBodyBlock(stmt); !hasBody {
						continue
					}
				}
			}
			fds, err := r.funcDecls(stmt)
			if err != nil {
				return nil, err
			}
			funcs = append(funcs, fds...)
		case frontend.NodeClassDeclaration:
			// A plain class registered in the pre-pass and renders with the entry's
			// classes. One with static initialization runs that work at its declaration
			// position, which is a top-level side effect the composed unit would have to
			// order, so it hands back.
			if info, ok := r.classInfoForDecl(stmt); ok && len(info.staticInit) > 0 {
				return nil, &NotYetLowerable{Reason: "a sibling module class with static initialization is a later slice"}
			}
		case frontend.NodeVariableStatement:
			// An exported const or let whose initializer is safe to evaluate at
			// package-init time composes as a package-level Go var the entry reads
			// through its import. A literal initializer has no observable evaluation
			// order, so it composes the way a function does; a non-literal initializer,
			// a non-exported binding, or a destructuring pattern still hands back, since
			// running it would need the module top-level order this slice does not thread.
			decl, err := r.moduleExportVarDecl(stmt)
			if err != nil {
				return nil, err
			}
			funcs = append(funcs, decl)
		case frontend.NodeInterfaceDeclaration, frontend.NodeTypeAliasDeclaration, frontend.NodeEnumDeclaration:
			// Type-level declarations carry no runtime code; a plain enum's const block
			// renders with the entry's package-level state and a const enum inlines.
			continue
		case frontend.NodeUnknown:
			// An import declaration recorded in the pre-pass above and carries no code;
			// the end-of-file token is empty. A re-export (`export { x } from`, `export *
			// from`) forwards another module's binding and declares nothing of its own:
			// the forwarded binding lives in the module the re-export names, which the
			// build composes separately, so an import of the re-exported name derefs the
			// alias chain through to that declaration and the re-export statement itself
			// carries no code. Any other unnamed statement (a bare local `export { x }`, an
			// `export default`, a side-effecting expression) is a top level the composed
			// unit would have to run, so it hands back.
			text := strings.TrimSpace(r.prog.Text(stmt))
			if text == "" || strings.HasPrefix(text, "import") || r.isReExport(stmt) {
				continue
			}
			return nil, &NotYetLowerable{Reason: "a top-level statement in a sibling module is a later slice"}
		default:
			return nil, &NotYetLowerable{Reason: "a top-level statement in a sibling module is a later slice"}
		}
	}
	return funcs, nil
}

// moduleExportVarDecl composes a sibling's exported variable statement, `export
// const K = 7` or `export let total = 3`, into a package-level Go var the entry
// reads through its import. Every binding must be an exported identifier with a
// package-init-safe initializer, the same bar the entry's own hoisted module vars
// meet, so the var evaluates once at package init with no dependence on the module
// top-level order. The binding takes its localName the way the entry's hoisted vars
// do, so an import reference derefs to the same Go name (expr.go) and a same-module
// read spells it unchanged. A non-exported statement, a destructuring pattern, a
// binding with no initializer, or a non-literal initializer hands the whole unit
// back, since composing it would need the top-level evaluation this slice omits.
func (r *Renderer) moduleExportVarDecl(stmt frontend.Node) (ast.Decl, error) {
	if !strings.HasPrefix(strings.TrimSpace(r.prog.Text(stmt)), "export") {
		return nil, &NotYetLowerable{Reason: "a non-exported top-level binding in a sibling module is a later slice"}
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, stmt, &decls)
	if len(decls) == 0 {
		return nil, &NotYetLowerable{Reason: "an exported binding with no declaration in a sibling module is a later slice"}
	}
	specs := make([]ast.Spec, 0, len(decls))
	for _, d := range decls {
		spec, err := r.moduleVarSpec(d)
		if err != nil {
			return nil, err
		}
		specs = append(specs, spec)
	}
	return &ast.GenDecl{Tok: token.VAR, Specs: specs}, nil
}

// isReExport reports whether an unnamed top-level statement forwards another
// module's binding, `export { x } from "./m"`, `export { x as y } from "./m"`, or
// `export * from "./m"`. A re-export names a module (a string-literal specifier
// child) and begins with export, which distinguishes it from a local `export { x }`
// that re-exports a same-module binding and from an ordinary import. It declares no
// runtime value of its own, so the composed module skips it.
func (r *Renderer) isReExport(stmt frontend.Node) bool {
	if !strings.HasPrefix(strings.TrimSpace(r.prog.Text(stmt)), "export") {
		return false
	}
	for _, k := range r.prog.Children(stmt) {
		if k.Kind() == frontend.NodeStringLiteral {
			return true
		}
	}
	return false
}

// composedNameCollision reports whether two top-level declarations in the
// composed program share a Go name. Within one file the mangle-collision
// pre-pass proved names unique, but two modules can each declare a binding that
// mangles to the same Go identifier, distinct TypeScript symbols the checker
// never compares because they live in different modules. The build would reject
// the duplicate, so the whole unit hands back rather than emit Go that does not
// compile.
func composedNameCollision(decls []ast.Decl) error {
	seen := map[string]bool{}
	clash := func(name string) error {
		if name == "" || name == "_" || name == "main" {
			return nil
		}
		if seen[name] {
			return &NotYetLowerable{Reason: "a declaration name collides across the composed modules, a later slice"}
		}
		seen[name] = true
		return nil
	}
	for _, d := range decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			// A method (a func with a receiver) takes its name from its type, not the
			// package namespace, so only a plain function competes for a top-level name.
			if decl.Recv == nil {
				if err := clash(decl.Name.Name); err != nil {
					return err
				}
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					if err := clash(s.Name.Name); err != nil {
						return err
					}
				case *ast.ValueSpec:
					for _, n := range s.Names {
						if err := clash(n.Name); err != nil {
							return err
						}
					}
				}
			}
		}
	}
	return nil
}
