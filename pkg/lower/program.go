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

	// A module-level binding a top-level function or class body reads cannot stay a
	// local of main, since a separate Go function cannot see main's locals; it hoists
	// to a package-level var the function and main both reference. The set is computed
	// before any statement lowers so the loop below can route a hoisted binding's
	// declaration out of the main body.
	hoisted := r.crossBoundaryModuleNames(entry)

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

	var funcs []ast.Decl
	var moduleVars []ast.Decl
	var mainBody []frontend.Node
	for _, stmt := range r.prog.Children(entry) {
		switch stmt.Kind() {
		case frontend.NodeFunctionDeclaration:
			fd, err := r.funcDecl(stmt)
			if err != nil {
				return Program{}, err
			}
			funcs = append(funcs, fd)
		case frontend.NodeVariableStatement:
			// A variable statement whose bindings a function reads becomes package-level
			// state; one whose bindings stay inside main is an ordinary main local. A
			// hoisted binding whose initializer is not safe to evaluate at package-init
			// time hands back, so the program routes to the interpreter rather than emit
			// Go that reads a name main declared but a function cannot see.
			decl, hoist, err := r.hoistModuleVar(stmt, hoisted)
			if err != nil {
				return Program{}, err
			}
			if hoist {
				moduleVars = append(moduleVars, decl)
			} else {
				mainBody = append(mainBody, stmt)
			}
		case frontend.NodeClassDeclaration:
			// Already registered by collectClasses; the declarations render after
			// every body lowers so a method body's interned shapes are collected.
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
				mainBody = append(mainBody, stmt)
			}
		default:
			mainBody = append(mainBody, stmt)
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
	r.unionLocals = r.unionLocalsOf(nil, mainBody)
	r.dynLocals = r.dynLocalsOf(nil, mainBody)
	r.bigOwned = r.bigOwnedLocalsOf(mainBody)
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
	stmts, err := r.lowerStatements(mainBody)
	restoreHoist()
	restoreFwd()
	if err != nil {
		return Program{}, err
	}
	stmts = append(hoistDecls, stmts...)
	stmts = append(fwdDecls, stmts...)
	stmts = r.hoistStrBuilders(stmts)

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
	}

	file := &ast.File{Name: ident("main")}
	if specs := r.importSpecs(); len(specs) > 0 {
		file.Decls = append(file.Decls, importDecl(specs))
	}
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
	// Module bindings a function reads emit as package-level vars beside the other
	// state, so both main and the functions name the same variable.
	file.Decls = append(file.Decls, moduleVars...)
	file.Decls = append(file.Decls, classDecls...)
	file.Decls = append(file.Decls, funcs...)
	file.Decls = append(file.Decls, mainDecl)

	src, err := printFile(file)
	if err != nil {
		return Program{}, err
	}
	return Program{Source: src}, nil
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
	}
	return nil
}

// crossBoundaryModuleNames returns the module-level binding names a top-level
// function or class body reads. Those cannot be locals of main, since a separate Go
// function has no access to main's locals, so the assembler hoists them to
// package-level vars. A reference counts only when its identifier resolves to the
// module binding's own symbol, so a parameter or local that merely shares the name
// does not force a hoist; the module binding it shadows stays a main local when no
// body actually reads it.
func (r *Renderer) crossBoundaryModuleNames(entry frontend.Node) map[string]bool {
	module := map[frontend.Symbol]string{}
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
	if len(used) == 0 {
		return nil
	}
	return used
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
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
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
		for _, c := range prog.Children(n) {
			walk(c)
		}
	}
	walk(entry)
	return uses
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
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		if n.Kind() == frontend.NodeCallExpression {
			if arg, ok := elidedObjectReceiver(r, n); ok {
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
		for _, c := range prog.Children(n) {
			walk(c)
		}
	}
	walk(entry)
	return uses
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
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		if n.Kind() == frontend.NodeExpressionStatement {
			kids := prog.Children(n)
			if len(kids) == 1 {
				if sym, ok := plainAssignTarget(prog, kids[0]); ok {
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
	return arg, true
}

// hoistModuleVar decides whether a module-level variable statement holds a binding a
// function reads and, if so, lowers the whole statement to a package-level var
// declaration. It returns hoist=false for a statement whose bindings all stay inside
// main, which the caller then lowers as an ordinary main local. When a binding does
// need hoisting, every binding in the statement moves with it, so each must carry a
// package-init-safe initializer; one that does not hands back rather than split the
// statement or evaluate a main-ordered side effect at init time.
func (r *Renderer) hoistModuleVar(stmt frontend.Node, hoisted map[string]bool) (ast.Decl, bool, error) {
	if len(hoisted) == 0 {
		return nil, false, nil
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
		return nil, false, nil
	}
	specs := make([]ast.Spec, 0, len(decls))
	for _, d := range decls {
		spec, err := r.moduleVarSpec(d)
		if err != nil {
			return nil, false, err
		}
		specs = append(specs, spec)
	}
	return &ast.GenDecl{Tok: token.VAR, Specs: specs}, true, nil
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
