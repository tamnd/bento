package lower

import (
	"go/ast"
	"go/format"
	"go/token"
	"sort"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file assembles the lowered pieces of one entry module into a single
// runnable Go program: a package main file holding the module's top-level
// functions as Go functions and its top-level statements as the body of main.
// It is the step that turns a checked .ts of real size into a .go with a main,
// the shape the native build compiles and links (doc 05 section 27, doc 17). Like
// every other lowering step it hands back a NotYetLowerable rather than emit
// partial or wrong Go, so a program the subset does not fully cover routes to the
// engine instead.

// Program is the assembled Go source for one compiled entry module. Source is a
// complete, gofmt-clean package main file the Go toolchain builds directly.
type Program struct {
	Source string
}

// RenderProgram lowers one entry source file to a runnable Go program. Top-level
// function declarations become package-level Go functions, and the remaining
// top-level statements become the body of main in source order, so the module's
// side effects run when the binary runs. Top-level classes become a struct, a
// NewX constructor, and pointer-receiver methods (classes.go). A construct the
// statement subset does not cover, or a top-level form that is neither a
// function, a class, nor a lowerable statement (an import, an export), hands
// back.
//
// The module's own top-level bindings are locals of main, so a top-level function
// that reads one is not yet supported: the function is a separate Go declaration
// that cannot see main's locals, which would fail the Go build rather than emit
// wrong output. Hoisting shared module bindings to package-level vars is a later
// slice; today a program whose functions are self-contained (the common shape of
// the compute workloads, which are a single top-level body) compiles.
func (r *Renderer) RenderProgram(entry frontend.Node) (Program, error) {
	return r.RenderProgramModules(entry, nil)
}

// RenderProgramModules lowers an entry source file together with the sibling
// modules it imports, which the build composed and staged alongside it, into one
// runnable Go program. The entry lowers exactly as it does alone: its top-level
// statements become main's body and its declarations become package-level Go. A
// sibling contributes its declarations to the same package, so an import of one
// of its exports resolves to a package-level Go name the entry references
// directly. This slice composes a sibling's declarations, not its top-level
// evaluation: a sibling that carries runtime, a variable statement or a
// side-effecting statement whose order the composed unit would have to preserve,
// hands back (see collectModules). With no siblings this is exactly the
// single-file path.
func (r *Renderer) RenderProgramModules(entry frontend.Node, deps []frontend.Node) (Program, error) {
	// A module reached through require runs as its own loader function, not as
	// flattened package declarations, so the require targets across the whole file
	// set are discovered before anything lowers: a require call site then resolves to
	// a loader name already in hand, and the deps split into the import-composed
	// siblings and the require-loaded modules. A dep reached only by require leaves
	// the collectModules path, which composes declarations, for the loader path.
	r.discoverRequiredModules(append([]frontend.Node{entry}, deps...))
	var esDeps, reqDeps []frontend.Node
	for _, dep := range deps {
		if _, required := r.requiredLoaders[dep.File().Path]; required {
			reqDeps = append(reqDeps, dep)
		} else {
			esDeps = append(esDeps, dep)
		}
	}
	// The sibling modules register first: their classes, enums, and generic
	// instantiations join the shared pre-pass state so an entry call site resolves
	// against them, and their top-level functions come back to emit as package
	// funcs beside the entry's. A sibling this slice cannot compose hands back here
	// before the entry lowers.
	depFuncs, err := r.collectModules(esDeps)
	if err != nil {
		return Program{}, err
	}
	// Names that are not Go identifiers mangle through a pure function of the
	// name (ident.go), so declaration and reference agree with no shared table.
	// The one spelling that scheme cannot make safe is a module that speaks
	// both a mangled name and its mangled form verbatim; that clashes in the
	// emitted Go, and renaming one side would make emission order-dependent,
	// so the whole module hands back before anything lowers.
	if err := r.checkMangleCollisions(entry); err != nil {
		return Program{}, err
	}
	// Record the module's node: import bindings before lowering any body, since a
	// function or a top-level statement may call an imported builtin and the call
	// lowering needs the binding already in hand. An import bento does not lower
	// hands back here, routing the whole unit to the engine.
	if err := r.collectNodeImports(entry); err != nil {
		return Program{}, err
	}
	// A binding introduced by an awaited static dynamic import, const m = await
	// import("./mod"), names the same compile-time namespace a static import * as m
	// does. Recording it in the same pre-pass, before any body lowers, lets a member
	// call on it resolve to the composed sibling's Go declaration wherever it appears.
	r.collectDynamicImportNamespaces(entry)
	// Classes register before any body lowers, the same hoisting the imports get,
	// so a function above a class can construct its instances.
	if err := r.collectClasses(entry); err != nil {
		return Program{}, err
	}
	// Enums register in the same pre-pass so a member read A.B that appears above
	// the enum's declaration still resolves to the member's constant or value.
	if err := r.collectEnums(entry); err != nil {
		return Program{}, err
	}
	// Generic functions register their monomorphizations in the same pre-pass so a
	// call above a generic declaration, and the declaration itself, agree on which
	// specializations to emit and what Go name each call resolves to. It reads only
	// the checker and never lowers, so it cannot fail the way the collectors above
	// can; a generic no call site monomorphizes simply records nothing and hands back
	// at its declaration.
	r.collectMono(entry)
	// A generic method cannot lower to one Go method, since a Go method carries no
	// type parameter, so the same pre-pass records the instantiations each generic
	// method's call sites ask for. A class then emits one mangled Go method per
	// instantiation, and a call site rewrites to the one it resolves to.
	r.collectMonoMethods(entry)
	// A const-bound arrow whose binding never escapes as a value can carry a default
	// parameter, filled at each direct call site the way a top-level function's is. The
	// same pre-pass records which arrows are escape-safe so the arrow's declaration
	// lowers the default away and its call sites fill it, both reading one map.
	r.collectArrowDefaults(entry)

	// A module-level binding a top-level function or class body reads cannot stay a
	// local of main, since a separate Go function cannot see main's locals; it hoists
	// to a package-level var the function and main both reference. The set is computed
	// before any statement lowers so the loop below can route a hoisted binding's
	// declaration out of the main body.
	hoisted := r.crossBoundaryModuleNames(entry)
	// The source ordinal of each module binding, so a binding hoisted by in-place
	// assignment can be checked against a forward reference: an initializer that reads
	// a module binding declared later would, in main's source order, read an unset
	// value, so that statement hands back rather than emit a wrong answer.
	moduleOrder := moduleBindingOrder(r.prog, entry)
	// Reset the in-place module-assignment set for this program: a binding hoisted to
	// a zero-valued package var, whose statement stays in main to run as an assignment.
	r.moduleAssignVars = map[string]bool{}

	// Count how many identifiers resolve to each binding so a local declared and
	// never read can be spotted when its statement lowers. A symbol is unique to
	// its binding, so one walk over the module settles the count for every scope.
	r.bindingUses = countBindingUses(r.prog, entry)

	// Count the identifier reads a fold drops so the blank decision below sees the
	// emitted-Go read count, not the source one. Object.keys(o) reads only o's shape
	// and never lowers o, and typeof x over a statically typed x folds to a constant
	// and drops x, so without this a binding the fold orphaned would keep its source
	// read and go unblanked, and the emitted Go would not build.
	r.elidedUses = countElidedReads(r, entry)
	// Count the write-only references, a plain `x = e` left side, so a local that is
	// declared and then only written registers as unread here and gets the blank Go
	// needs; a variable Go never reads is declared-and-not-used even when it is
	// written, since only a read counts toward use.
	r.writeUses = countWriteUses(r.prog, entry)
	// Count the declaration name nodes per binding so bindingUnused can tell a
	// redeclared `var` (two `var f` name nodes, one binding) from a binding declared
	// once and read once. Without this a redeclaration reads as a use and a never-read
	// redeclared var goes unblanked, so the emitted Go trips declared-and-not-used.
	r.bindingDecls = countBindingDecls(r.prog, entry)
	// Record the stored Map and Set iterators a local holds and one for...of drives, so
	// their declarations emit nothing and each loop ranges the receiver directly. It
	// reads the use tallies above to prove the local is referenced exactly once.
	r.collectStoredCollIters(entry)
	// Record every mutating call on a Map or Set identifier so a manual iterator drive can
	// prove its receiver is not mutated after the iterator is minted, the snapshot
	// faithfulness bar the for...of drive holds against its loop body.
	r.collectCollMutations(entry)

	r.programStrict = r.hasUseStrictPrologue(entry)
	var funcs []ast.Decl
	var moduleVars []ast.Decl
	var mainBody []frontend.Node
	// mainItems is the main body in source order with the static-init calls a class
	// declaration contributes spliced in at its own position, the order JavaScript
	// runs the top-level statements and each class's static blocks. mainBody keeps
	// only the statement nodes the analysis passes below walk; the calls carry no
	// binding, so they ride mainItems alone.
	var mainItems []mainItem
	pushStmt := func(n frontend.Node) {
		mainBody = append(mainBody, n)
		mainItems = append(mainItems, mainItem{node: n})
	}
	for _, stmt := range r.prog.Children(entry) {
		switch stmt.Kind() {
		case frontend.NodeFunctionDeclaration:
			// A function overload set is several declarations under one symbol: the bodyless
			// signatures carry no runtime code and the implementation body runs. Skip a
			// signature declaration so only the implementation lowers, once, and the set
			// emits a single Go func. An overload set whose implementation this slice does
			// not claim (a concrete or optional parameter, so the call could not box) hands
			// the whole unit back rather than emit a partial function.
			if sym, ok := r.prog.SymbolAt(stmt); ok {
				if _, isOverload := r.overloadImplNode(sym); isOverload {
					if _, claimed := r.overloadedFuncImpl(sym); !claimed {
						return Program{}, &NotYetLowerable{Reason: "an overloaded function whose implementation is not all-dynamic is a later slice"}
					}
					if _, hasBody := r.funcBodyBlock(stmt); !hasBody {
						continue
					}
				}
			}
			fds, err := r.funcDecls(stmt)
			if err != nil {
				return Program{}, err
			}
			funcs = append(funcs, fds...)
		case frontend.NodeVariableStatement:
			// A variable statement whose bindings a function reads becomes package-level
			// state; one whose bindings stay inside main is an ordinary main local. A
			// hoisted binding whose initializer is not safe to evaluate at package-init
			// time hands back, so the program routes to the interpreter rather than emit
			// Go that reads a name main declared but a function cannot see.
			decl, mode, err := r.hoistModuleVar(stmt, hoisted, moduleOrder)
			if err != nil {
				return Program{}, err
			}
			switch mode {
			case hoistInit:
				// The whole statement moves to package scope, initializer and all, since
				// it is safe to evaluate at package-init time.
				moduleVars = append(moduleVars, decl)
			case hoistAssign:
				// The binding is a zero-valued package var so a top-level function can
				// read it, and the statement stays in main to assign it at its source
				// position, keeping the module top-level evaluation order.
				moduleVars = append(moduleVars, decl)
				pushStmt(stmt)
			default:
				pushStmt(stmt)
			}
		case frontend.NodeClassDeclaration:
			// Already registered by collectClasses; the declarations render after
			// every body lowers so a method body's interned shapes are collected. A
			// class with static initialization steps also contributes a call to its
			// init function here, at the declaration's position, which is when the
			// ordered static work runs.
			if info, ok := r.classInfoForDecl(stmt); ok && len(info.staticInit) > 0 {
				mainItems = append(mainItems, mainItem{initClass: info})
			}
			continue
		case frontend.NodeInterfaceDeclaration, frontend.NodeTypeAliasDeclaration:
			// Type-level declarations carry no runtime code, so they emit nothing.
			continue
		case frontend.NodeEnumDeclaration:
			// Already registered by collectEnums; a plain enum's const block renders
			// with the other package-level declarations below, and a const enum emits
			// nothing since its members inline at each use.
			continue
		case frontend.NodeUnknown:
			// The parser ends a source file with an empty end-of-file token bento
			// does not name; it is skipped. An import declaration is an unnamed node
			// too, already validated and recorded by collectNodeImports above, and it
			// carries no runtime code, so it is skipped here. Several control-flow
			// statements (a do...while, a labeled loop, a break or continue) are left
			// unnamed as well, and lowerStatement recognizes each by its shape, so a
			// non-empty non-import unknown routes to the main body like any other
			// statement; if lowerStatement does not know it either, it hands back there.
			text := strings.TrimSpace(r.prog.Text(stmt))
			if text != "" && !strings.HasPrefix(text, "import") {
				pushStmt(stmt)
			}
		default:
			pushStmt(stmt)
		}
	}

	// The module top-level is a body like a function's, so its integer locals are
	// specialized the same way: the analysis runs over the whole main body before it
	// is lowered, and the counters and accumulators of a compute loop are given a Go
	// int32 type. The set is scoped to this lowering and needs no restore, since the
	// program has no enclosing body.
	r.constInt = r.constIntsOf(mainBody)
	r.int32Locals = r.int32LocalsOf(mainBody)
	// A hoisted binding is a package-level var of its declared type, so it must not
	// also be int32-specialized: main and the functions read it at one Go type, and
	// the int32 form is reserved for a loop-local counter that never leaves main.
	for name := range hoisted {
		delete(r.int32Locals, name)
	}
	r.counterIvl = r.counterIvlOf(mainBody)
	// The int64 tier runs after the int32 set and the counter ranges, since its
	// interval proof reads both, and a hoisted binding stays at its package-level
	// type for the same reason it cannot be int32.
	r.int64Locals = r.int64LocalsOf(mainBody)
	for name := range hoisted {
		delete(r.int64Locals, name)
	}
	r.fixedTArr = r.fixedTypedArraysOf(mainBody)
	r.optLocals = r.optLocalsOf(mainBody)
	r.definiteLocals = r.definiteLocalsOf(mainBody)
	r.unionLocals = r.unionLocalsOf(nil, mainBody)
	r.dynLocals = r.dynLocalsOf(nil, mainBody)
	r.bigOwned = r.bigOwnedLocalsOf(mainBody)
	// A binding hoisted by in-place assignment keeps its statement in main, so the
	// specialization passes above see it; but it is a package-level var of its declared
	// type, read at that one type by main and the functions, so it is kept off every
	// storage-narrowing tier the way an initializer-hoisted binding is by never being
	// in mainBody at all. Without this the pass would read it through .Get() or a
	// union tag while the package var holds the plain value.
	for name := range r.moduleAssignVars {
		delete(r.int32Locals, name)
		delete(r.int64Locals, name)
		delete(r.optLocals, name)
		delete(r.unionLocals, name)
		delete(r.dynLocals, name)
		delete(r.fixedTArr, name)
		delete(r.bigOwned, name)
	}
	r.strBuilders = nil
	// A var written in a nested block of the module body and read outside it hoists
	// to a package-visible top-of-main declaration. A hoisted binding reads at one Go
	// type, so like a module-hoisted one it is kept off the int32 and int64 tiers,
	// which are reserved for a loop-local counter.
	hoistDecls, restoreHoist, err := r.enterVarHoistScope(mainBody)
	if err != nil {
		return Program{}, err
	}
	for name := range r.hoistedVars {
		delete(r.int32Locals, name)
		delete(r.int64Locals, name)
	}
	// A callable-object binding an earlier statement captures in a closure declares
	// its pointer at the top of main too, so the closure closes over a variable
	// already in scope the way the const is for JavaScript.
	fwdDecls, restoreFwd, err := r.enterFwdHoistScope(mainBody)
	if err != nil {
		restoreHoist()
		return Program{}, err
	}
	stmts, err := r.lowerMainItems(mainItems)
	restoreHoist()
	restoreFwd()
	if err != nil {
		return Program{}, err
	}
	stmts = append(hoistDecls, stmts...)
	stmts = append(fwdDecls, stmts...)
	stmts = r.hoistStrBuilders(stmts)

	// The required modules lower after the entry body, so the per-module analysis
	// state each loader overwrites is already spent, and before the throw and promise
	// checks below, so a module body that throws or mints a promise sets the same
	// program-level flag the entry's body would: the entry's main then defers the
	// uncaught reporter or drains the microtask queue for work a loader runs.
	requiredDecls, err := r.renderRequiredModules(reqDeps)
	if err != nil {
		return Program{}, err
	}

	// A program that can raise a thrown value defers the uncaught-error reporter as
	// its first statement, so a throw that escapes every catch prints an
	// uncaught-error line and exits non-zero rather than crashing with a Go stack.
	// The defer is first so it is the last to run and thus sees any panic the body
	// raises. A program that cannot throw defers nothing, leaving its main and its
	// imports as they were.
	if r.usesThrow {
		r.requireImport(valuePkg)
		report := &ast.DeferStmt{Call: &ast.CallExpr{Fun: sel("value", "ReportUncaught")}}
		stmts = append([]ast.Stmt{report}, stmts...)
	}

	mainDecl := &ast.FuncDecl{
		Name: ident("main"),
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: stmts},
	}

	// Class declarations render last so every use site has already lowered, but
	// they print before the functions: each class emits its struct, constructor,
	// and methods together, the order a hand-written Go file keeps.
	classDecls, err := r.renderClasses()
	if err != nil {
		return Program{}, err
	}

	// A program that minted a promise drains the microtask queue once at the end of
	// main, doc 11's run-to-completion point collapsed to the single turn a compiled
	// job has. Every promise 6a mints is already settled, so this one drain runs each
	// then callback the synchronous body queued, in order, after that body. It is the
	// last statement so a callback observes the finished synchronous run, the ordering
	// JavaScript gives a microtask. The drain is appended after the classes render so a
	// promise minted or observed inside a class method body, which lowers there, still
	// sets the flag in time; a program that minted no promise drains nothing.
	if r.usesPromise {
		r.requireImport(valuePkg)
		drain := &ast.ExprStmt{X: &ast.CallExpr{Fun: sel("value", "RunMicrotasks")}}
		mainDecl.Body.List = append(mainDecl.Body.List, drain)
		// After the drain, any promise that rejected and was never observed is an
		// unhandled rejection: JavaScript runs the unhandledrejection path once the
		// microtask checkpoint is clear. Reporting it (to stderr, with a non-zero exit)
		// is what lets a test that asserts a rejection observe it, rather than the
		// rejection vanishing into a false pass.
		report := &ast.ExprStmt{X: &ast.CallExpr{Fun: sel("value", "ReportUnhandledRejections")}}
		mainDecl.Body.List = append(mainDecl.Body.List, report)
	}

	file := &ast.File{Name: ident("main")}
	// The generated struct types the functions and the main body referred to are
	// collected after lowering, since interning happens as a use is lowered, and
	// emitted before the functions, the conventional Go order of types then code.
	file.Decls = append(file.Decls, r.DeclNodes()...)
	// The tagged-sum union types the functions and the main body construct and
	// narrow emit here, beside the interned structs, as a tag type, const block,
	// struct, and per-arm constructors, before the code that refers to them.
	file.Decls = append(file.Decls, r.renderUnions()...)
	// The wide bigint literals the bodies interned emit as package vars, each parsed
	// once at init, so a constant past int64 named in a loop costs one parse for the
	// program's life. They follow the types and precede the code like any other
	// package-level state.
	file.Decls = append(file.Decls, r.bigLitDecls()...)
	// Numeric enums emit their float64-backed const blocks with the other
	// package-level state, before the classes and functions that read them.
	file.Decls = append(file.Decls, r.renderEnums()...)
	// The CommonJS module object and its exports alias, when the program read either
	// global, emit as package-level vars before the module bindings so both main and a
	// top-level function that closes over module or exports name the same variable. A
	// program that named neither emits nothing here.
	file.Decls = append(file.Decls, r.commonjsModuleDecls()...)
	// Module bindings a function reads emit as package-level vars beside the other
	// state, so both main and the functions name the same variable.
	file.Decls = append(file.Decls, moduleVars...)
	file.Decls = append(file.Decls, classDecls...)
	// A composed sibling's functions emit as package funcs before the entry's, the
	// order a hand-written Go file keeps a dependency above its user.
	file.Decls = append(file.Decls, depFuncs...)
	// Each required module's cache slot var and loader function emit here, before the
	// entry's functions and main, so a require call in either resolves to a loader
	// name already declared in the package.
	file.Decls = append(file.Decls, requiredDecls...)
	file.Decls = append(file.Decls, funcs...)
	file.Decls = append(file.Decls, mainDecl)

	// The value runtime import is decided from the finished declarations rather than
	// the requireImport calls scattered through lowering. A type or expression that
	// reaches value.X without its path having flagged the import would otherwise name
	// an unimported package, and a requireImport a later-discarded lowering left behind
	// would import a package no statement references; both emit Go that does not build.
	// Reading the flag off the assembled AST closes each.
	r.reconcileValueImport(file.Decls)
	if specs := r.importSpecs(); len(specs) > 0 {
		file.Decls = append([]ast.Decl{importDecl(specs)}, file.Decls...)
	}

	// Two composed modules can each declare a binding whose Go name is the same,
	// distinct TypeScript symbols the checker never compares because they live in
	// different modules, but one Go identifier the build would reject. Within a
	// single file the per-file mangle-collision check already proved names unique;
	// across the composed unit this final scan catches a cross-module clash and
	// hands the whole unit back rather than emit Go that does not build.
	if len(deps) > 0 {
		if err := composedNameCollision(file.Decls); err != nil {
			return Program{}, err
		}
	}

	// Every not-assignable diagnostic the front door admitted (2345 for a call
	// argument, 2322 for an assignment or initializer) must have flowed through a
	// guarded bridge, one that lowered the value safely or handed it back. A site no
	// bridge reached was lowered by a path with no representation guard and would emit
	// Go that does not compile, so the whole unit hands back here rather than ship it
	// (see unguardedNotAssign). This runs after the body lowers, since only lowering
	// records which sites the guards reached.
	if err := r.unguardedNotAssign(); err != nil {
		return Program{}, err
	}

	src, err := printFile(file)
	if err != nil {
		return Program{}, err
	}
	return Program{Source: src}, nil
}

// reconcileValueImport sets the value runtime import from whether the finished
// declarations actually name the value package. The import is aliased value on every
// path, so a selector whose qualifier is the identifier value is a reference to it;
// bento never emits a local named value, so no user binding shadows the qualifier.
// Deriving the flag here, rather than trusting the requireImport calls a lowering makes
// as it interns a use, imports the package exactly when a statement reads it: a path
// that reached value.X without flagging the import still imports it, and a flag a
// discarded lowering left behind no longer imports a package nothing references.
func (r *Renderer) reconcileValueImport(decls []ast.Decl) {
	used := false
	for _, d := range decls {
		ast.Inspect(d, func(n ast.Node) bool {
			if sel, ok := n.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "value" {
					used = true
					return false
				}
			}
			return true
		})
		if used {
			break
		}
	}
	if used {
		r.imports[valuePkg] = true
	} else {
		delete(r.imports, valuePkg)
	}
}

// mainItem is one entry of the main body in source order: either a top-level
// statement node to lower, or a class whose static-init function the main body
// calls at the class declaration's position. Exactly one field is set.
type mainItem struct {
	node      frontend.Node
	initClass *classInfo
}

// lowerMainItems lowers the main body in source order, emitting each statement
// through the ordinary path and each class's static-init call at the class
// declaration's position, the interleaving JavaScript runs. It opens one block
// scope for the whole body, the same scope lowerStatements gives a plain list.
func (r *Renderer) lowerMainItems(items []mainItem) ([]ast.Stmt, error) {
	r.blockDeclared = append(r.blockDeclared, map[string]bool{})
	defer func() { r.blockDeclared = r.blockDeclared[:len(r.blockDeclared)-1] }()
	out := make([]ast.Stmt, 0, len(items))
	for _, it := range items {
		if it.initClass != nil {
			out = append(out, &ast.ExprStmt{X: &ast.CallExpr{Fun: ident(staticInitName(it.initClass))}})
			continue
		}
		// The program body is a top-level scope like a function body, so a `using`
		// among its statements defers its disposal to main's return, the scope that
		// matches the JavaScript block scope. A `using` in a nested block hands back
		// through lowerVarStatement the same way it does inside a function.
		if stmts, ok, err := r.lowerUsingDefer(it.node); err != nil {
			return nil, err
		} else if ok {
			out = append(out, stmts...)
			continue
		}
		stmts, err := r.lowerStatementMulti(it.node)
		if err != nil {
			return nil, err
		}
		out = append(out, stmts...)
	}
	return out, nil
}

// classInfoForDecl finds the registered class a class-declaration node belongs
// to, so the main body can splice its static-init call in at the declaration's
// position.
func (r *Renderer) classInfoForDecl(decl frontend.Node) (*classInfo, bool) {
	for _, info := range r.classes {
		if info.decl == decl {
			return info, true
		}
	}
	return nil, false
}

// checkMangleCollisions declines a module that spells both a name needing the
// mangle and that name's mangled form verbatim, like $DONE next to D_DONE.
// Both would emit as D_DONE, and picking a fresh name for either side would
// make the mapping depend on what else the module declares, breaking the
// pure-function rule every call site relies on. The scan walks every
// identifier once up front so the guard holds for declarations and references
// alike, wherever they sit.
func (r *Renderer) checkMangleCollisions(entry frontend.Node) error {
	texts := map[string]bool{}
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		if n.Kind() == frontend.NodeIdentifier {
			texts[r.prog.Text(n)] = true
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	walk(entry)
	names := make([]string, 0, len(texts))
	for t := range texts {
		names = append(names, t)
	}
	sort.Strings(names)
	for _, t := range names {
		if m, ok := mangleIdent(t); ok && m != t && texts[m] {
			return &NotYetLowerable{Reason: "the module already speaks " + m + ", which " + t + " mangles to"}
		}
		// The CommonJS module object and exports alias emit under reserved Go names. A
		// user binding whose Go spelling is one of them would share the identifier with
		// the synthetic var, so the unit hands back rather than emit a redeclaration. A
		// reference to the module or exports global itself never reaches here as its own
		// text, since those spell module and exports, not the reserved Go names.
		if m, ok := localName(t); ok && (m == bentoModuleName || m == bentoExportsName || m == bentoRequireName) {
			return &NotYetLowerable{Reason: "the CommonJS module object reserves the Go name " + m + ", which " + t + " takes"}
		}
	}
	return nil
}

// crossBoundaryModuleNames returns the module-level binding names a package-scope
// body reads. Those cannot be locals of main, since a separate Go function has no
// access to main's locals, so the assembler hoists them to package-level vars. A
// reference counts only when its identifier resolves to the module binding's own
// symbol, so a parameter or local that merely shares the name does not force a hoist;
// the module binding it shadows stays a main local when no body actually reads it.
//
// A top-level function or class body is one package-scope reader. A closure held by a
// binding that is itself hoisted is another: its Go func literal is built in main at
// the binding's position but may be called after init, when a captured main local of
// a later binding is not in scope, so every module binding that closure reads must be
// a package var too. That relationship is transitive, since a hoisted binding's
// closure can name a second binding whose own closure names a third, so the set is
// grown to a fixpoint over the closures inside each hoisted binding's initializer.
func (r *Renderer) crossBoundaryModuleNames(entry frontend.Node) map[string]bool {
	module := map[frontend.Symbol]string{}
	initOf := map[string]frontend.Node{}
	for _, stmt := range r.prog.Children(entry) {
		if stmt.Kind() != frontend.NodeVariableStatement {
			continue
		}
		var decls []frontend.Node
		collectVarDecls(r.prog, stmt, &decls)
		for _, d := range decls {
			kids := r.prog.Children(d)
			if len(kids) == 0 {
				continue
			}
			name, ok := localName(r.prog.Text(kids[0]))
			if !ok {
				continue
			}
			if sym, ok := r.prog.SymbolAt(kids[0]); ok {
				module[sym] = name
			}
			if len(kids) == 2 || len(kids) == 3 {
				initOf[name] = kids[len(kids)-1]
			}
		}
	}
	if len(module) == 0 {
		return nil
	}
	used := map[string]bool{}
	for _, stmt := range r.prog.Children(entry) {
		switch stmt.Kind() {
		case frontend.NodeFunctionDeclaration, frontend.NodeClassDeclaration:
			collectModuleRefs(r.prog, stmt, module, used)
		}
	}
	// Grow the set: a hoisted binding's initializer closures cross the boundary too,
	// so the module bindings they read hoist alongside it. Repeat until no new name
	// is added, which closes the set over chains of closures naming one another.
	for {
		grew := false
		for name := range used {
			init, ok := initOf[name]
			if !ok {
				continue
			}
			before := len(used)
			collectClosureModuleRefs(r.prog, init, module, used)
			if len(used) != before {
				grew = true
			}
		}
		if !grew {
			break
		}
	}
	if len(used) == 0 {
		return nil
	}
	return used
}

// collectClosureModuleRefs records the module bindings read from inside a function
// literal within n, the reads a hoisted binding's initializer defers to call time. It
// descends the initializer's immediate expression without recording its reads, since
// those run in main where a main local is still reachable, and switches to the whole
// subtree collector at each function, arrow, or method literal it meets, since that
// body is what may run after init at package scope.
func collectClosureModuleRefs(prog *frontend.Program, n frontend.Node, module map[frontend.Symbol]string, out map[string]bool) {
	switch n.Kind() {
	case frontend.NodeArrowFunction, frontend.NodeFunctionExpression, frontend.NodeFunctionDeclaration,
		frontend.NodeMethodDeclaration, frontend.NodeGetAccessor, frontend.NodeSetAccessor, frontend.NodeConstructor:
		collectModuleRefs(prog, n, module, out)
		return
	}
	for _, c := range prog.Children(n) {
		collectClosureModuleRefs(prog, c, module, out)
	}
}

// collectModuleRefs records every identifier in n's subtree that resolves to a
// module-level binding's symbol, so the caller learns which module bindings a
// function or class body reads. Resolving through the symbol, rather than matching
// the identifier text, means a parameter or local that shadows a module binding is
// not mistaken for a read of it, since the shadow has its own symbol.
func collectModuleRefs(prog *frontend.Program, n frontend.Node, module map[frontend.Symbol]string, out map[string]bool) {
	if n.Kind() == frontend.NodeIdentifier {
		if sym, ok := prog.SymbolAt(n); ok {
			if name, ok := module[sym]; ok {
				out[name] = true
			}
		}
		return
	}
	// An object-literal shorthand `{ x }` reads the outer binding x, but its identifier
	// resolves to the property the shorthand declares rather than to x, so SymbolAt above
	// would miss the read. Resolving the shorthand's value symbol credits it, so a module
	// binding a function reads only through a shorthand still crosses the boundary and
	// hoists to package scope instead of staying a main local the function cannot see.
	if sym, ok := shorthandValueSymbol(prog, n); ok {
		if name, ok := module[sym]; ok {
			out[name] = true
		}
	}
	for _, c := range prog.Children(n) {
		collectModuleRefs(prog, c, module, out)
	}
}

// countBindingUses walks the module and tallies, per symbol, how many identifiers
// resolve to it. A binding declared and never read shows a count of one, the
// declaration's own name; every read or write adds another. Resolving through the
// symbol rather than the identifier text keeps a shadowing local from inflating an
// outer binding's count, since each binding carries its own symbol.
//
// An object-literal shorthand needs a second lookup. The checker resolves a `{ x }`
// member's identifier to the property it declares, not to the outer `x` it copies,
// so the plain symbol walk would miss the read and blank a binding the emit still
// references. Each shorthand member credits its value symbol, the outer binding, so
// the emit for `{ x }` reads `x` without a redundant blank beside it.
func countBindingUses(prog *frontend.Program, entry frontend.Node) map[frontend.Symbol]int {
	uses := map[frontend.Symbol]int{}
	var walk func(n, parent frontend.Node, idx int)
	walk = func(n, parent frontend.Node, idx int) {
		if skipAsTypePosition(prog, n, parent, idx) {
			return
		}
		if n.Kind() == frontend.NodeIdentifier {
			if sym, ok := prog.SymbolAt(n); ok {
				uses[sym]++
			}
		}
		if n.Kind() == frontend.NodeObjectLiteralExpression {
			for _, member := range prog.Children(n) {
				if sym, ok := shorthandValueSymbol(prog, member); ok {
					uses[sym]++
				}
			}
		}
		for i, c := range prog.Children(n) {
			walk(c, n, i)
		}
	}
	walk(entry, frontend.Node(nil), 0)
	return uses
}

// skipAsTypePosition reports whether a walk that counts runtime references should
// prune n and its subtree because n is a type-level construct the emit never lowers.
// It fires on a type alias or interface, whose whole body renderTopLevel drops; on a
// `typeof x` type query, the unnamed type node the parser produces for the annotation
// in `let a: typeof o`; and on the type annotation of a declaration, the unnamed type
// node that follows a binding's name, as the `baz` in `let baz: baz` or the `I` in
// `var k: I`. Each names a binding only to spell a type, so counting the reference
// would keep a binding the runtime never touches alive and leave a real local declared
// and not used in the Go. Pruning a type position can only ever drop a reference the
// emit does not make, so at worst it adds a harmless `_ = x` beside a real read; it
// never hides one the emit does read.
func skipAsTypePosition(prog *frontend.Program, n, parent frontend.Node, idx int) bool {
	switch n.Kind() {
	case frontend.NodeTypeAliasDeclaration, frontend.NodeInterfaceDeclaration:
		return true
	}
	if isTypeQuery(prog, n) {
		return true
	}
	return isDeclTypeAnnotation(n, parent, idx)
}

// isDeclTypeAnnotation reports whether n is the type-annotation child of a variable
// declaration, parameter, or property declaration. The parser wraps a type node in an
// unnamed node, and it also wraps a destructuring binding pattern in one, but the
// binding target is always the first child while the annotation follows it, so an
// unnamed child past the first is the type annotation and nothing else. An initializer
// is a named expression, never unnamed, so it is never mistaken for the annotation.
func isDeclTypeAnnotation(n, parent frontend.Node, idx int) bool {
	if parent == nil || idx == 0 || n.Kind() != frontend.NodeUnknown {
		return false
	}
	switch parent.Kind() {
	case frontend.NodeVariableDeclaration, frontend.NodeParameter, frontend.NodePropertyDeclaration:
		return true
	}
	return false
}

// isTypeQuery reports whether n is a `typeof x` type-query node in a type position,
// the unnamed type node the parser produces for the annotation in `let a: typeof o`
// or a `typeof o` operand inside a larger type. It never fires on a value-position
// `typeof x`, which parses as a named prefix-unary expression, so a walk can drop the
// type-level reference without touching the runtime one. The match is the unnamed
// node whose own token range opens with the `typeof` keyword.
func isTypeQuery(prog *frontend.Program, n frontend.Node) bool {
	if n.Kind() != frontend.NodeUnknown {
		return false
	}
	text := strings.TrimSpace(prog.Text(n))
	return text == "typeof" || strings.HasPrefix(text, "typeof ")
}

// shorthandValueSymbol returns the outer binding an object-literal member reads when
// the member is a shorthand `{ x }`, and false otherwise. A shorthand carries a
// single identifier child and does not open with the spread token; a `{ ...o }` copy
// and a `{ k: v }` pair both fail that shape and read as not-a-shorthand here.
func shorthandValueSymbol(prog *frontend.Program, member frontend.Node) (frontend.Symbol, bool) {
	kids := prog.Children(member)
	if len(kids) != 1 || kids[0].Kind() != frontend.NodeIdentifier {
		return frontend.Symbol{}, false
	}
	if strings.HasPrefix(strings.TrimSpace(prog.Text(member)), "...") {
		return frontend.Symbol{}, false
	}
	return prog.ShorthandValueSymbolAt(member)
}

// bindingUnused reports whether the binding named by nameNode is declared and
// never read: only its declaration name nodes resolve to the binding's symbol, and
// nothing reads it. Resolving through the symbol counts only references to this
// binding, so a shadowing local of the same name elsewhere in the module does not
// keep it alive. A binding whose symbol does not resolve is treated as used, so the
// conservative answer only ever withholds the blank assignment. countBindingUses
// credits an object-literal shorthand to the outer binding it reads, so a `{ x }`
// keeps x used and no redundant blank lands beside the value the struct literal
// copies.
func (r *Renderer) bindingUnused(nameNode frontend.Node) bool {
	sym, ok := r.prog.SymbolAt(nameNode)
	if !ok {
		return false
	}
	// Subtract the reads a fold drops and the write-only references from the emitted
	// Go, so a binding whose only non-declaration reads were dropped by a fold (an
	// Object.keys(o) receiver, a typeof x tag) or were plain writes reads as unused
	// here and gets the blank the emit needs. What remains is the binding's own
	// declaration name nodes plus any real read; a `var` redeclared in the same scope
	// has more than one declaration name node for one binding, so the baseline is the
	// declaration count rather than a hardcoded one. Equal means no read survives.
	decls := r.bindingDecls[sym]
	if decls == 0 {
		decls = 1
	}
	uses := r.bindingUses[sym] - r.elidedUses[sym] - r.writeUses[sym]
	return uses == decls
}

// countBindingDecls walks the module and tallies, per symbol, the declaration name
// nodes that introduce it: the first identifier child of each variable declaration.
// A binding declared once counts one; a `var` redeclared in the same scope, which
// JavaScript folds to a single binding, counts one per `var` name node. bindingUnused
// uses the tally as the baseline it compares the surviving reference count against, so
// a redeclared-but-unread binding is still recognized as unused. Resolving through the
// symbol keeps a shadowing local of the same name from inflating an outer binding's
// count, since each binding carries its own symbol.
func countBindingDecls(prog *frontend.Program, entry frontend.Node) map[frontend.Symbol]int {
	decls := map[frontend.Symbol]int{}
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		if n.Kind() == frontend.NodeVariableDeclaration {
			kids := prog.Children(n)
			if len(kids) > 0 && kids[0].Kind() == frontend.NodeIdentifier {
				if sym, ok := prog.SymbolAt(kids[0]); ok {
					decls[sym]++
				}
			}
		}
		for _, c := range prog.Children(n) {
			walk(c)
		}
	}
	walk(entry)
	return decls
}

// countElidedReads tallies, per binding, the identifier reads a fold drops from the
// emitted Go. Object.keys(o), Object.getOwnPropertyNames(o), and Object.hasOwn(o, k)
// read only o's compile-time shape and never lower o, and typeof x over a statically
// typed x folds to a constant tag and drops x, so the source counts a read the emit
// does not make. Recording those reads lets bindingUnused blank a binding the fold
// orphaned. Each pattern is matched syntactically, on a bare identifier; if it does
// not actually fold the whole program hands back and no Go is emitted, and even a
// stray over-count only adds a harmless `_ = x` beside a real read, so it never
// unbalances a binding the emit does read.
func countElidedReads(r *Renderer, entry frontend.Node) map[frontend.Symbol]int {
	uses := map[frontend.Symbol]int{}
	prog := r.prog
	var walk func(n, parent frontend.Node, idx int)
	walk = func(n, parent frontend.Node, idx int) {
		// Stay in step with countBindingUses: a type position carries no runtime
		// code, so it counts no read inside one. Crediting an elided read here that
		// it never counted would subtract below a real binding's baseline and leave
		// a live local unblanked.
		if skipAsTypePosition(prog, n, parent, idx) {
			return
		}
		if n.Kind() == frontend.NodeCallExpression {
			if arg, ok := elidedObjectReceiver(r, n); ok {
				if sym, ok := prog.SymbolAt(arg); ok {
					uses[sym]++
				}
			}
			for _, arg := range elidedTemporalStringArgs(r, n) {
				if sym, ok := prog.SymbolAt(arg); ok {
					uses[sym]++
				}
			}
		}
		if arg, ok := elidedTypeofOperand(r, n); ok {
			if sym, ok := prog.SymbolAt(arg); ok {
				uses[sym]++
			}
		}
		if arg, ok := elidedTruthyOperand(r, n); ok {
			if sym, ok := prog.SymbolAt(arg); ok {
				uses[sym]++
			}
		}
		if arg, ok := elidedComputedKey(r, n, parent); ok {
			if sym, ok := prog.SymbolAt(arg); ok {
				uses[sym]++
			}
		}
		if arg, ok := elidedInReceiver(r, n); ok {
			if sym, ok := prog.SymbolAt(arg); ok {
				uses[sym]++
			}
		}
		for _, arg := range elidedOverriddenSpreadSources(r, n) {
			if sym, ok := prog.SymbolAt(arg); ok {
				uses[sym]++
			}
		}
		for i, c := range prog.Children(n) {
			walk(c, n, i)
		}
	}
	walk(entry, entry, 0)
	return uses
}

// elidedOverriddenSpreadSources reports the spread-source identifiers of an object
// literal every one of whose fields a later member of the same literal overrides,
// so the spread copies nothing into the emitted struct. objectLiteral resolves the
// left-to-right override in its field map, and a source all of whose fields a later
// member re-sets contributes no surviving src.Field to the composite literal, yet
// the source identifier was still read once by the spread. Recording that read lets
// bindingUnused blank the source, an ambient `declare const` among them, the
// override orphaned. A node that is not an object literal, a spread whose source is
// not a plain identifier, and a literal carrying a member whose field set cannot be
// resolved statically all return nothing, keeping the match to the case it is sure
// of, where even a stray over-count only adds a harmless _ = x beside a real read.
func elidedOverriddenSpreadSources(r *Renderer, n frontend.Node) []frontend.Node {
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return nil
	}
	prog := r.prog
	members := prog.Children(n)
	// contributed[i] is the set of Go field names member i sets, left nil when the
	// member's fields cannot be resolved statically. A nil later member could hide a
	// field an earlier spread's source still supplies, so it blocks claiming any
	// earlier spread fully overridden.
	contributed := make([]map[string]bool, len(members))
	type spreadRef struct {
		idx  int
		src  frontend.Node
		flds map[string]bool
	}
	var spreads []spreadRef
	for i, p := range members {
		if p.Kind() != frontend.NodeUnknown {
			continue
		}
		kids := prog.Children(p)
		switch len(kids) {
		case 1:
			if strings.HasPrefix(strings.TrimSpace(prog.Text(p)), "...") {
				src := kids[0]
				flds := spreadSourceFields(r, src)
				contributed[i] = flds
				if src.Kind() == frontend.NodeIdentifier && flds != nil {
					spreads = append(spreads, spreadRef{idx: i, src: src, flds: flds})
				}
				continue
			}
			if f, ok := plainMemberField(r, kids[0]); ok {
				contributed[i] = map[string]bool{f: true}
			}
		case 2:
			if f, ok := plainMemberField(r, kids[0]); ok {
				contributed[i] = map[string]bool{f: true}
			}
		}
	}
	var out []frontend.Node
	for _, s := range spreads {
		later := map[string]bool{}
		unresolved := false
		for j := s.idx + 1; j < len(members); j++ {
			if contributed[j] == nil {
				unresolved = true
				break
			}
			for f := range contributed[j] {
				later[f] = true
			}
		}
		if unresolved {
			continue
		}
		covered := true
		for f := range s.flds {
			if !later[f] {
				covered = false
				break
			}
		}
		if covered {
			out = append(out, s.src)
		}
	}
	return out
}

// spreadSourceFields reports the Go field names an object spread of src copies,
// mirroring objectSpread: the source must be a fixed-shape object, not an array,
// with at least one static property, each of whose names is a Go identifier. It
// returns nil when any of those fail, the same conditions under which objectSpread
// hands the literal back rather than emit a field read.
func spreadSourceFields(r *Renderer, src frontend.Node) map[string]bool {
	t := r.prog.TypeAt(src)
	if t.Flags&frontend.TypeObject == 0 {
		return nil
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return nil
	}
	props := r.prog.Properties(t)
	if len(props) == 0 {
		return nil
	}
	flds := map[string]bool{}
	for _, p := range props {
		f, ok := exportedField(p.Name)
		if !ok {
			return nil
		}
		flds[f] = true
	}
	return flds
}

// plainMemberField resolves the Go field name a plain object-literal member sets,
// the same memberName then exportedField objectLiteral keys its field map on, so
// the override check compares names the emit agrees with. It reports false for a
// key that is not a plain property name.
func plainMemberField(r *Renderer, keyNode frontend.Node) (string, bool) {
	prop, ok := r.memberName(keyNode)
	if !ok {
		return "", false
	}
	return exportedField(prop)
}

// elidedTemporalStringArgs reports the bare-identifier arguments a Temporal call folds to
// a constant string and drops from the emit. A const of a literal string type passed
// straight to a Temporal from-string or time-zone lowering (Temporal.PlainDate.from(s),
// zdt.withTimeZone(tz), and the sibling per-type folds) is read by stringLiteralValue at
// compile time and emitted as the quoted literal, so the source names the const but the
// emit never lowers the identifier, leaving it declared and not used. Recording the read
// lets bindingUnused blank the const the fold orphaned. The match is gated on the callee
// being a Temporal static (Temporal.<Type>.<method>) or a method on a Temporal-typed
// receiver, so a user function named from(s) is untouched; within that gate an argument a
// particular method does not fold lowers and reads the identifier for real, where an
// over-count only adds a harmless _ = s beside it, and a widened or non-literal argument
// has no literal type and is skipped.
func elidedTemporalStringArgs(r *Renderer, n frontend.Node) []frontend.Node {
	kids := r.prog.Children(n)
	if len(kids) < 2 {
		return nil
	}
	callee := kids[0]
	if callee.Kind() != frontend.NodePropertyAccessExpression {
		return nil
	}
	parts := r.prog.Children(callee)
	if len(parts) < 1 {
		return nil
	}
	recv := parts[0]
	temporal := false
	if recv.Kind() == frontend.NodePropertyAccessExpression {
		// A static call Temporal.<Type>.<method>(...) whose receiver is the two-level
		// access Temporal.<Type>.
		if sub := r.prog.Children(recv); len(sub) == 2 && r.isGlobalRef(sub[0], "Temporal") {
			temporal = true
		}
	}
	if !temporal && r.isTemporalReceiver(recv) {
		temporal = true
	}
	if !temporal {
		return nil
	}
	var out []frontend.Node
	for _, arg := range kids[1:] {
		if arg.Kind() != frontend.NodeIdentifier {
			continue
		}
		if _, ok := r.stringLiteralValue(arg); ok {
			out = append(out, arg)
		}
	}
	return out
}

// isTemporalReceiver reports whether a node is a value of one of the Temporal wrapper
// types, the receivers whose methods route to the per-type Temporal method lowerings.
func (r *Renderer) isTemporalReceiver(n frontend.Node) bool {
	return r.isPlainDate(n) || r.isPlainTime(n) || r.isPlainDateTime(n) ||
		r.isDuration(n) || r.isPlainYearMonth(n) || r.isPlainMonthDay(n) ||
		r.isInstant(n) || r.isZonedDateTime(n)
}

// elidedComputedKey reports the identifier a static const string key folds away, in the
// two places a fixed-shape read folds one: an object destructuring computed key,
// `const { [k]: v } = o`, and a string-keyed element access, `o[k]`. Both fold k to the
// source's field at compile time when k is a const of a literal string type, so the
// source names k but the emit never lowers it, which would leave a const k whose only use
// was the key declared and not used. Recording the read lets bindingUnused blank the
// orphaned const. Only an identifier key with a constant string value is matched; a
// string-literal key has no binding to orphan, and a run-time or widened key hands back.
// The match is syntactic and does not check the receiver is static: a dynamic receiver
// lowers and reads k for real, where an over-count only adds a harmless _ = k beside it.
// A computed member of an object literal, `{ [k]: 1 }`, shares the destructuring element's
// two-child shape but keeps its key, since boxObjectLiteral lowers it through SetKeyed, so
// it is excluded by its parent being an object-literal expression rather than a pattern.
func elidedComputedKey(r *Renderer, n, parent frontend.Node) (frontend.Node, bool) {
	if parent.Kind() != frontend.NodeObjectLiteralExpression {
		if key, _, ok := r.objectComputedElem(n); ok {
			if key.Kind() == frontend.NodeIdentifier {
				if _, ok := r.pureConstStringKey(key); ok {
					return key, true
				}
			}
		}
	}
	if n.Kind() == frontend.NodeElementAccessExpression {
		kids := r.prog.Children(n)
		if len(kids) == 2 && kids[1].Kind() == frontend.NodeIdentifier {
			if _, ok := r.pureConstStringKey(kids[1]); ok {
				return kids[1], true
			}
		}
	}
	return nil, false
}

// elidedTruthyOperand reports the bare-identifier condition of a control-flow node
// that lowerTruthy collapses to a constant and drops from the emit. An object in a
// boolean position (for (; obj; ), while (obj), if (obj), obj ? a : b, obj && x) is
// always truthy, so lowerTruthy folds it to the Go constant true and never lowers
// the read, which leaves a var whose only use was that condition declared and not
// used. Recording the read lets bindingUnused blank the binding the fold orphaned.
// The match is the same one lowerTruthy makes: a bare identifier the checker proved
// always truthy or always falsy and that is repeatable, so its evaluation is safe to
// drop. A parenthesized condition unwraps to the identifier inside so ((obj)) counts
// too. A dynamic or primitive condition keeps its runtime test and is not matched.
func elidedTruthyOperand(r *Renderer, n frontend.Node) (frontend.Node, bool) {
	var cond frontend.Node
	switch n.Kind() {
	case frontend.NodeForStatement:
		fc := r.prog.ForClauses(n)
		if !fc.HasCond {
			return nil, false
		}
		cond = fc.Cond
	case frontend.NodeWhileStatement, frontend.NodeIfStatement, frontend.NodeConditionalExpression:
		kids := r.prog.Children(n)
		if len(kids) < 1 {
			return nil, false
		}
		cond = kids[0]
	case frontend.NodeBinaryExpression:
		// The left operand of && or || rides the same truthiness fold (logical.go),
		// so an always-truthy object on the left collapses and drops its read.
		kids := r.prog.Children(n)
		if len(kids) != 3 {
			return nil, false
		}
		switch r.prog.Text(kids[1]) {
		case "&&", "||":
			cond = kids[0]
		default:
			return nil, false
		}
	default:
		return nil, false
	}
	cond = r.unwrapParens(cond)
	if cond.Kind() != frontend.NodeIdentifier {
		return nil, false
	}
	if r.isBool(cond) || r.isDynamic(cond) {
		return nil, false
	}
	if _, known := r.staticTruthy(cond); !known {
		return nil, false
	}
	if !r.repeatableOperand(cond) {
		return nil, false
	}
	return cond, true
}

// elidedTypeofOperand reports the bare-identifier operand of a typeof expression
// that folds to a constant tag and drops the operand from the emit. typeof x folds
// when x is not dynamic and its type pins one tag (typeof.go), which is exactly the
// non-dynamic bare identifier this matches; a dynamic operand keeps its runtime read
// and so is left uncounted.
func elidedTypeofOperand(r *Renderer, n frontend.Node) (frontend.Node, bool) {
	if !r.isTypeofExpr(n) {
		return nil, false
	}
	operand := r.prog.Children(n)[0]
	if operand.Kind() != frontend.NodeIdentifier {
		return nil, false
	}
	if r.isDynamic(operand) {
		return nil, false
	}
	if _, ok := r.staticTypeofTag(operand); !ok {
		return nil, false
	}
	return operand, true
}

// countWriteUses tallies, per binding, the identifier references that write it
// without reading it: the left side of a plain `x = e` assignment whose value is
// discarded, which the checker exposes as a binary expression whose operator token is
// `=` and whose left operand is a bare identifier, sitting alone as an expression
// statement. Only a discarded assignment is write-only: when the assignment's value is
// consumed (console.log(s = "set")) the emit synthesizes a read of the target, so an
// assignment nested inside a larger expression keeps the binding read and is not
// counted. A compound assignment (x += e) and a ++/-- read the binding before storing,
// so those are left out too. Go counts only a read toward use, so recording the
// write-only references lets bindingUnused blank a local that is declared and then only
// written.
func countWriteUses(prog *frontend.Program, entry frontend.Node) map[frontend.Symbol]int {
	uses := map[frontend.Symbol]int{}
	creditIdent := func(target frontend.Node) {
		if target == nil || target.Kind() != frontend.NodeIdentifier {
			return
		}
		if sym, ok := prog.SymbolAt(target); ok {
			uses[sym]++
		}
	}
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		switch n.Kind() {
		case frontend.NodeExpressionStatement:
			kids := prog.Children(n)
			if len(kids) == 1 {
				if sym, ok := plainAssignTarget(prog, kids[0]); ok {
					uses[sym]++
				}
				// A destructuring assignment standing alone, `({ p: x } = o)` or
				// `([a] = pair)`, writes each pattern target and discards the result, so
				// a name it only stores into is write-only the same way a plain `x = e`
				// is. Crediting each bare-identifier target here lets bindingUnused blank
				// a `let x` a later pattern assignment stores into but nothing reads,
				// which Go otherwise rejects as declared-and-not-used.
				if lhs, ok := destructureAssignPattern(prog, kids[0]); ok {
					var creditTargets func(p frontend.Node)
					creditTargets = func(p frontend.Node) {
						for _, c := range prog.Children(p) {
							if c.Kind() == frontend.NodeIdentifier {
								creditIdent(c)
							} else {
								creditTargets(c)
							}
						}
					}
					creditTargets(lhs)
				}
			}
		case frontend.NodeForOfStatement, frontend.NodeForInStatement:
			// A head that assigns each element to an existing binding, `for (v of it)`,
			// writes v every iteration without reading it, the same write-only shape a
			// standalone `v = e` is. A declaring head, `for (const v of it)`, is a fresh
			// binding whose head child is a declaration list, not a bare name, so it does
			// not match here and is not miscounted as a write of an outer v.
			if kids := prog.Children(n); len(kids) >= 1 {
				creditIdent(kids[0])
			}
		case frontend.NodeForStatement:
			// A for loop's incrementor, `for (...; ...; n = c)`, writes n once per step and
			// never reads it back, so a counter the loop only advances is write-only the
			// same way a discarded assignment is.
			if fc := prog.ForClauses(n); fc.HasIncr {
				if sym, ok := plainAssignTarget(prog, fc.Incr); ok {
					uses[sym]++
				}
			}
		}
		for _, c := range prog.Children(n) {
			walk(c)
		}
	}
	walk(entry)
	return uses
}

// plainAssignTarget reports the binding written by a plain `x = e` assignment whose
// left operand is a bare identifier, and false for anything else (a compound assign, a
// member or index target, a non-assignment expression). The caller only treats it as
// write-only when the assignment stands alone as an expression statement, so its value
// is discarded and the emit never reads the target back.
func plainAssignTarget(prog *frontend.Program, n frontend.Node) (frontend.Symbol, bool) {
	if n.Kind() != frontend.NodeBinaryExpression {
		return frontend.Symbol{}, false
	}
	kids := prog.Children(n)
	if len(kids) != 3 || strings.TrimSpace(prog.Text(kids[1])) != "=" || kids[0].Kind() != frontend.NodeIdentifier {
		return frontend.Symbol{}, false
	}
	return prog.SymbolAt(kids[0])
}

// destructureAssignPattern reports the left pattern of a destructuring assignment
// expression, `({ p: x } = o)` or `([a] = pair)`, and false for anything else. An
// assignment target parses as an expression, so the pattern is an object or array
// literal on the left of a `=` binary; the statement may wrap the whole thing in a
// parenthesized expression, which an object-literal target always needs to parse as
// an assignment rather than a block. The caller credits each bare identifier under
// the returned pattern as a write-only target.
func destructureAssignPattern(prog *frontend.Program, n frontend.Node) (frontend.Node, bool) {
	if n.Kind() == frontend.NodeParenthesizedExpression {
		kids := prog.Children(n)
		if len(kids) == 1 {
			return destructureAssignPattern(prog, kids[0])
		}
		return frontend.Node(nil), false
	}
	if n.Kind() != frontend.NodeBinaryExpression {
		return frontend.Node(nil), false
	}
	kids := prog.Children(n)
	if len(kids) != 3 || strings.TrimSpace(prog.Text(kids[1])) != "=" {
		return frontend.Node(nil), false
	}
	switch kids[0].Kind() {
	case frontend.NodeObjectLiteralExpression, frontend.NodeArrayLiteralExpression:
		return kids[0], true
	}
	return frontend.Node(nil), false
}

// elidedObjectReceiver reports the bare-identifier receiver of an Object static call
// that folds to a compile-time answer and drops the receiver from the emit:
// Object.keys, Object.getOwnPropertyNames, and Object.hasOwn. It returns the receiver
// identifier node and true only for those calls with an identifier first argument.
func elidedObjectReceiver(r *Renderer, call frontend.Node) (frontend.Node, bool) {
	kids := r.prog.Children(call)
	if len(kids) < 2 {
		return nil, false
	}
	callee := kids[0]
	if callee.Kind() != frontend.NodePropertyAccessExpression {
		return nil, false
	}
	ckids := r.prog.Children(callee)
	if len(ckids) != 2 {
		return nil, false
	}
	if !r.isGlobalRef(ckids[0], "Object") {
		return nil, false
	}
	switch r.prog.Text(ckids[1]) {
	case "keys", "getOwnPropertyNames", "hasOwn":
	default:
		return nil, false
	}
	arg := kids[1]
	if arg.Kind() != frontend.NodeIdentifier {
		return nil, false
	}
	// A dynamic receiver is not folded away: its Object static walks the runtime bag
	// and lowers the receiver, so its read is real and must not be counted as elided.
	if r.isDynamic(arg) {
		return nil, false
	}
	return arg, true
}

// moduleHoist is how a module-level variable statement a function reads reaches
// package scope. hoistNone keeps the statement a local of main. hoistInit moves the
// whole statement, initializer and all, to a package-level var, since the
// initializer is safe to evaluate at package-init time. hoistAssign declares the
// binding as a zero-valued package var and leaves the statement in main to run as an
// assignment at its source position, for an initializer that is not init-safe (a
// call, or an expression over other module state) but whose in-place evaluation
// keeps the module top-level order.
type moduleHoist int

const (
	hoistNone moduleHoist = iota
	hoistInit
	hoistAssign
)

// hoistModuleVar decides how a module-level variable statement holding a binding a
// function reads reaches package scope. A statement whose bindings all stay inside
// main returns hoistNone and is lowered as an ordinary main local. When a binding
// does need hoisting, every binding in the statement moves with it: if all their
// initializers are safe to evaluate at package-init time the statement hoists whole
// (hoistInit), otherwise each binding becomes a zero-valued package var and the
// statement stays in main to assign it in source order (hoistAssign). A binding with
// no initializer, or one whose initializer forward-references a module binding
// declared later, hands back rather than emit a var that reads an unset value.
func (r *Renderer) hoistModuleVar(stmt frontend.Node, hoisted map[string]bool, order map[frontend.Symbol]int) (ast.Decl, moduleHoist, error) {
	if len(hoisted) == 0 {
		return nil, hoistNone, nil
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, stmt, &decls)
	needsHoist := false
	for _, d := range decls {
		kids := r.prog.Children(d)
		if len(kids) == 0 {
			continue
		}
		if name, ok := localName(r.prog.Text(kids[0])); ok && hoisted[name] {
			needsHoist = true
			break
		}
	}
	if !needsHoist {
		return nil, hoistNone, nil
	}
	// Every binding must carry an initializer to hoist; a bare `let x;` a function
	// reads is a later slice, not this group.
	for _, d := range decls {
		if kids := r.prog.Children(d); len(kids) != 2 && len(kids) != 3 {
			return nil, hoistNone, &NotYetLowerable{Reason: "a module binding a function reads needs an initializer to hoist to a package var"}
		}
	}
	// If every initializer is package-init-safe the statement hoists whole, the
	// cleaner form with no zero-value window and no in-main assignment.
	allSafe := true
	for _, d := range decls {
		kids := r.prog.Children(d)
		if !packageSafeInit(r.prog, kids[len(kids)-1]) {
			allSafe = false
			break
		}
	}
	if allSafe {
		specs := make([]ast.Spec, 0, len(decls))
		for _, d := range decls {
			spec, err := r.moduleVarSpec(d)
			if err != nil {
				return nil, hoistNone, err
			}
			specs = append(specs, spec)
		}
		return &ast.GenDecl{Tok: token.VAR, Specs: specs}, hoistInit, nil
	}
	// Otherwise the binding hoists by in-place assignment. Each initializer must read
	// only module bindings declared before this one, so main's source-order assignment
	// never reads an unset package var; a forward or cyclic reference hands back.
	for _, d := range decls {
		kids := r.prog.Children(d)
		sym, ok := r.prog.SymbolAt(kids[0])
		if !ok {
			return nil, hoistNone, &NotYetLowerable{Reason: "a hoisted module binding has no resolved symbol"}
		}
		if r.forwardModuleRef(kids[len(kids)-1], order, order[sym]) {
			return nil, hoistNone, &NotYetLowerable{Reason: "a module binding a function reads forward-references a later module binding, which the hoist cannot order"}
		}
	}
	specs := make([]ast.Spec, 0, len(decls))
	for _, d := range decls {
		spec, err := r.moduleZeroVarSpec(d)
		if err != nil {
			return nil, hoistNone, err
		}
		specs = append(specs, spec)
		if name, ok := localName(r.prog.Text(r.prog.Children(d)[0])); ok {
			r.moduleAssignVars[name] = true
		}
	}
	return &ast.GenDecl{Tok: token.VAR, Specs: specs}, hoistAssign, nil
}

// moduleVarSpec lowers one binding of a hoisted variable statement to a Go value
// spec, var name T = init, typed by the checker the same way a local binding is. The
// initializer must be safe to evaluate at package-init time; one that reads a name
// or makes a call could depend on main's order or a side effect, so it hands back.
func (r *Renderer) moduleVarSpec(d frontend.Node) (ast.Spec, error) {
	kids := r.prog.Children(d)
	if len(kids) != 2 && len(kids) != 3 {
		return nil, &NotYetLowerable{Reason: "a module binding a function reads needs an initializer to hoist to a package var"}
	}
	name, ok := localName(r.prog.Text(kids[0]))
	if !ok {
		return nil, &NotYetLowerable{Reason: "a hoisted module binding name is not a Go identifier"}
	}
	initNode := kids[len(kids)-1]
	if !packageSafeInit(r.prog, initNode) {
		return nil, &NotYetLowerable{Reason: "a module binding a function reads has an initializer that is not yet hoistable to a package var"}
	}
	typ, err := r.typeExpr(r.prog.TypeAt(kids[0]))
	if err != nil {
		return nil, err
	}
	init, err := r.bindingInit(kids[0], initNode)
	if err != nil {
		return nil, err
	}
	return &ast.ValueSpec{
		Names:  []*ast.Ident{ident(name)},
		Type:   typ,
		Values: []ast.Expr{init},
	}, nil
}

// moduleZeroVarSpec lowers one binding of an in-place-hoisted statement to a
// zero-valued package var, var name T, typed by the checker. The value is written
// in main at the statement's source position, so the spec carries the type only; Go
// zero-initializes it, which stands in for the module-eval window before the
// assignment runs. The type render is what proves the binding hoistable here, since
// a type the lowerer cannot spell hands back before the statement stays in main.
func (r *Renderer) moduleZeroVarSpec(d frontend.Node) (ast.Spec, error) {
	kids := r.prog.Children(d)
	name, ok := localName(r.prog.Text(kids[0]))
	if !ok {
		return nil, &NotYetLowerable{Reason: "a hoisted module binding name is not a Go identifier"}
	}
	typ, err := r.typeExpr(r.prog.TypeAt(kids[0]))
	if err != nil {
		return nil, err
	}
	return &ast.ValueSpec{
		Names: []*ast.Ident{ident(name)},
		Type:  typ,
	}, nil
}

// forwardModuleRef reports whether an initializer subtree reads a module binding
// declared at or after ordinal self, the case an in-place assignment cannot order:
// main runs the assignments in source order, so a read of a binding declared later,
// or of the binding itself, would see an unset package var. A read of an earlier
// module binding is fine, since its assignment has already run.
func (r *Renderer) forwardModuleRef(n frontend.Node, order map[frontend.Symbol]int, self int) bool {
	switch n.Kind() {
	case frontend.NodeIdentifier:
		if sym, ok := r.prog.SymbolAt(n); ok {
			if ord, ok := order[sym]; ok && ord >= self {
				return true
			}
		}
		return false
	case frontend.NodeArrowFunction, frontend.NodeFunctionExpression, frontend.NodeFunctionDeclaration,
		frontend.NodeMethodDeclaration, frontend.NodeGetAccessor, frontend.NodeSetAccessor, frontend.NodeConstructor:
		// A nested function body, and its default parameter values, run when the
		// closure is called, not while this initializer evaluates. A read of a later
		// module binding from inside is therefore deferred past init and cannot see an
		// unset package var, so it is not a forward reference. The checker rejects any
		// forward reference that is read immediately (an IIFE or a plain read) as a
		// use-before-declaration before lowering, so only these deferred reads reach
		// here. The one form the checker cannot trace, a helper invoked mid-init that
		// transitively reads a not-yet-assigned binding, is a documented approximation.
		return false
	case frontend.NodeCallExpression, frontend.NodeNewExpression:
		// A function literal in callee position is invoked now, so its body does run at
		// init time. Descend into it as an immediate read even though the opaque case
		// above would otherwise skip it, keeping the guard sound on its own.
		kids := r.prog.Children(n)
		if len(kids) > 0 && isInvokedFunctionLiteral(kids[0]) {
			for _, c := range r.prog.Children(kids[0]) {
				if r.forwardModuleRef(c, order, self) {
					return true
				}
			}
			for _, c := range kids[1:] {
				if r.forwardModuleRef(c, order, self) {
					return true
				}
			}
			return false
		}
	}
	for _, c := range r.prog.Children(n) {
		if r.forwardModuleRef(c, order, self) {
			return true
		}
	}
	return false
}

// isInvokedFunctionLiteral reports whether a node is a bare function literal, the
// callee shape of an immediately invoked function expression. Its body runs at the
// call site rather than being deferred, so forwardModuleRef must look through it.
func isInvokedFunctionLiteral(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeArrowFunction, frontend.NodeFunctionExpression:
		return true
	}
	return false
}

// moduleBindingOrder assigns each module-level variable binding a source ordinal, so
// an in-place hoist can tell a backward reference (safe, already assigned) from a
// forward or self reference (unsafe, an unset package var). The ordinal increases in
// declaration order across every top-level variable statement.
func moduleBindingOrder(prog *frontend.Program, entry frontend.Node) map[frontend.Symbol]int {
	order := map[frontend.Symbol]int{}
	next := 0
	for _, stmt := range prog.Children(entry) {
		if stmt.Kind() != frontend.NodeVariableStatement {
			continue
		}
		var decls []frontend.Node
		collectVarDecls(prog, stmt, &decls)
		for _, d := range decls {
			kids := prog.Children(d)
			if len(kids) == 0 {
				continue
			}
			if sym, ok := prog.SymbolAt(kids[0]); ok {
				order[sym] = next
			}
			next++
		}
	}
	return order
}

// packageSafeInit reports whether an initializer can be evaluated at package-init
// time. A subtree with no identifier read and no call references no other binding
// and has no observable side effect, so a Go package-var initializer runs it with
// the same result main would, whatever the init order. A numeric, string, boolean,
// or bigint literal, a sign on one, and arithmetic over them all qualify; an
// initializer that names a variable or calls a function does not.
func packageSafeInit(prog *frontend.Program, n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeIdentifier, frontend.NodeCallExpression:
		return false
	case frontend.NodeObjectLiteralExpression:
		return objectLiteralSafeInit(prog, n)
	}
	for _, c := range prog.Children(n) {
		if !packageSafeInit(prog, c) {
			return false
		}
	}
	return true
}

// objectLiteralSafeInit reports whether an object literal { k: v, ... } is safe to
// evaluate at package-init time. A property name is a fixed label, not a binding
// read, so a plain-identifier key does not make the literal unsafe the way the
// generic walk would judge it; only the values, and a computed key's own
// expression, carry a read or a call that could depend on main's order. A
// shorthand { x } or a spread { ...o } does read an outer binding, and a method or
// accessor member holds a body the generic walk cannot vet here, so each of those
// keeps the literal off the package-init path.
func objectLiteralSafeInit(prog *frontend.Program, n frontend.Node) bool {
	for _, member := range prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return false
		}
		kids := prog.Children(member)
		switch len(kids) {
		case 2:
			// A plain-identifier key names a property and is skipped; a computed key,
			// [expr], is not an identifier node and is vetted like any other subtree.
			if kids[0].Kind() != frontend.NodeIdentifier && !packageSafeInit(prog, kids[0]) {
				return false
			}
			if !packageSafeInit(prog, kids[1]) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// bigLitDecls emits one package-level var per wide bigint literal the bodies
// interned, in first-use order so the output is deterministic:
//
//	var bigLit1 = value.BigIntMustParse("36893488147419103232")
//
// Each parses once at init, so a loop that names the constant re-reads the same
// *big.Int. The var is shared across every site that named the value, which is
// why bigown.go treats a read of one as not fresh: a local initialized from it
// must never be mutated in place.
func (r *Renderer) bigLitDecls() []ast.Decl {
	if len(r.bigLitOrder) == 0 {
		return nil
	}
	out := make([]ast.Decl, 0, len(r.bigLitOrder))
	for _, decimal := range r.bigLitOrder {
		out = append(out, &ast.GenDecl{
			Tok: token.VAR,
			Specs: []ast.Spec{&ast.ValueSpec{
				Names: []*ast.Ident{ident(r.bigLits[decimal])},
				Values: []ast.Expr{&ast.CallExpr{
					Fun:  sel("value", "BigIntMustParse"),
					Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(decimal)}},
				}},
			}},
		})
	}
	return out
}

// goImportSpec is one import the assembled file carries: a Go import path and,
// for a go: interop package, the local alias it is imported under. A plain runtime
// import (the value model, math) has an empty alias and prints unaliased.
type goImportSpec struct {
	alias string
	path  string
}

// importSpecs is the full import set the assembled file needs, the plain runtime
// imports the lowering required and the aliased Go packages a go: call reached,
// sorted by path so the output is deterministic.
func (r *Renderer) importSpecs() []goImportSpec {
	specs := make([]goImportSpec, 0, len(r.imports)+len(r.goAliases))
	for p := range r.imports {
		specs = append(specs, goImportSpec{path: p})
	}
	for p, a := range r.goAliases {
		specs = append(specs, goImportSpec{alias: a, path: p})
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].path < specs[j].path })
	return specs
}

// importDecl builds the import block for the assembled file. The parenthesized
// form is forced (a nonzero Lparen) so a single import prints as an import block
// like every other, which keeps the generated file's shape stable as more
// imports appear. A spec with an alias prints it as the import name, the form a
// go: interop package needs so the call qualifier and the import agree.
func importDecl(specs []goImportSpec) ast.Decl {
	out := make([]ast.Spec, 0, len(specs))
	for _, s := range specs {
		spec := &ast.ImportSpec{
			Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(s.path)},
		}
		if s.alias != "" {
			spec.Name = ident(s.alias)
		}
		out = append(out, spec)
	}
	return &ast.GenDecl{Tok: token.IMPORT, Lparen: token.Pos(1), Specs: out}
}

// printFile renders the assembled file to gofmt-clean Go source. A print failure
// means the file bento built is not valid Go, a lowering bug, so it surfaces as a
// NotYetLowerable rather than a panic, the same boundary printExpr and printDecl
// keep.
func printFile(f *ast.File) (string, error) {
	var b strings.Builder
	if err := format.Node(&b, token.NewFileSet(), f); err != nil {
		return "", &NotYetLowerable{Reason: "generated program did not print: " + err.Error()}
	}
	return b.String(), nil
}

// hasUseStrictPrologue reports whether the entry module opens with a "use strict"
// directive. The directive prologue is the leading run of string-literal expression
// statements, and "use strict" anywhere in that run makes the module strict; a
// statement that is not a bare string literal ends the prologue. Scanning only the
// prologue keeps a later string expression, or a x = "use strict" assignment, from
// falsely marking the module strict.
func (r *Renderer) hasUseStrictPrologue(entry frontend.Node) bool {
	for _, stmt := range r.prog.Children(entry) {
		if stmt.Kind() != frontend.NodeExpressionStatement {
			return false
		}
		kids := r.prog.Children(stmt)
		if len(kids) == 0 || kids[0].Kind() != frontend.NodeStringLiteral {
			return false
		}
		if directiveText(r.prog.Text(kids[0])) == "use strict" {
			return true
		}
	}
	return false
}

// directiveText strips the surrounding quotes a string-literal directive carries in
// source so its body compares against "use strict" directly.
func directiveText(raw string) string {
	if len(raw) >= 2 {
		q := raw[0]
		if (q == '"' || q == '\'') && raw[len(raw)-1] == q {
			return raw[1 : len(raw)-1]
		}
	}
	return raw
}
