package lower

import (
	"go/ast"
	"go/token"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers statements: variable declarations, assignments and updates,
// returns, and the control flow (if, for, for..of, while). Each lowering keeps
// the honest boundary of the package: a statement outside the subset hands back
// a NotYetLowerable so the unit routes to the engine.

// lowerBlock lowers a block node's statements to a Go block. It is used for the
// function body and for the arms of the control-flow statements, so a nested
// block lowers the same way the top-level one does.
func (r *Renderer) lowerBlock(block frontend.Node) (*ast.BlockStmt, error) {
	stmts, err := r.lowerStatements(r.prog.Children(block))
	if err != nil {
		return nil, err
	}
	return &ast.BlockStmt{List: stmts}, nil
}

// lowerStatements lowers a sequence of statement nodes, in order. It opens a
// block scope for the duration so a `var` that redeclares a name already declared
// in this block lowers to an assignment rather than a duplicate Go declaration.
func (r *Renderer) lowerStatements(nodes []frontend.Node) ([]ast.Stmt, error) {
	r.blockDeclared = append(r.blockDeclared, map[string]bool{})
	defer func() { r.blockDeclared = r.blockDeclared[:len(r.blockDeclared)-1] }()
	out := make([]ast.Stmt, 0, len(nodes))
	for _, n := range nodes {
		stmts, err := r.lowerStatementMulti(n)
		if err != nil {
			return nil, err
		}
		out = append(out, stmts...)
	}
	return out, nil
}

// blockDeclares reports whether name is already declared in the current block and,
// if not, is only meaningful when a block is open. The empty stack (a for-loop
// initializer lowered outside a block) declares nothing, so it always reports false.
func (r *Renderer) blockDeclares(name string) bool {
	if len(r.blockDeclared) == 0 {
		return false
	}
	return r.blockDeclared[len(r.blockDeclared)-1][name]
}

// markBlockDeclared records that name now has a Go declaration in the current
// block, so a later `var` on the same name in the same block lowers to an
// assignment. It is a no-op with no block open.
func (r *Renderer) markBlockDeclared(name string) {
	if len(r.blockDeclared) == 0 {
		return
	}
	r.blockDeclared[len(r.blockDeclared)-1][name] = true
}

// lowerStatementMulti lowers one statement, letting it expand into more than one Go
// statement. A statement-position ternary flattens here to an if plus the arms that
// return or assign each branch; every other statement lowers to the single Go
// statement lowerStatement builds and comes back as a one-element slice.
func (r *Renderer) lowerStatementMulti(n frontend.Node) ([]ast.Stmt, error) {
	// A return in a catch or finally body of a try whose returns escape expands
	// to the named-result assignment plus the bare return out of the deferred
	// handler, so it routes here where a statement may become two.
	if (r.tryRet == tryRetDefer || r.tryRet == tryRetDeferPlain) && n.Kind() == frontend.NodeReturnStatement {
		return r.deferredReturn(n)
	}
	if stmts, ok, err := r.flattenCallableBinding(n); err != nil {
		return nil, err
	} else if ok {
		return stmts, nil
	}
	if stmts, ok, err := r.flattenConditionalStatement(n); err != nil {
		return nil, err
	} else if ok {
		return stmts, nil
	}
	if stmts, ok, err := r.flattenCommaStatement(n); err != nil {
		return nil, err
	} else if ok {
		return stmts, nil
	}
	if stmts, ok, err := r.flattenChainedAssignStatement(n); err != nil {
		return nil, err
	} else if ok {
		return stmts, nil
	}
	if stmts, ok, err := r.flattenArrayDestructure(n); err != nil {
		return nil, err
	} else if ok {
		return stmts, nil
	}
	if stmts, ok, err := r.flattenObjectDestructure(n); err != nil {
		return nil, err
	} else if ok {
		return stmts, nil
	}
	// A variable statement whose bindings the module never reads expands to the
	// declaration plus a blank assignment per unused binding, so the initializer
	// still runs while the emitted Go does not trip declared-and-not-used.
	if n.Kind() == frontend.NodeVariableStatement {
		return r.lowerVarStatementMulti(n)
	}
	s, err := r.lowerStatement(n)
	if err != nil {
		return nil, err
	}
	return []ast.Stmt{s}, nil
}

// lowerStatement lowers one statement. The covered set is the straight-line and
// control-flow forms a numeric body is built from: a return, a local variable
// declaration, an assignment, an if, and a while. The rest land in later slices,
// each handing back until then.
func (r *Renderer) lowerStatement(n frontend.Node) (ast.Stmt, error) {
	switch n.Kind() {
	case frontend.NodeReturnStatement:
		return r.lowerReturn(n)
	case frontend.NodeVariableStatement:
		return r.lowerVarStatement(n)
	case frontend.NodeExpressionStatement:
		return r.lowerExprStatement(n)
	case frontend.NodeIfStatement:
		return r.lowerIf(n)
	case frontend.NodeWhileStatement:
		return r.lowerWhile(n)
	case frontend.NodeForStatement:
		return r.lowerFor(n)
	case frontend.NodeForOfStatement:
		return r.lowerForOf(n)
	case frontend.NodeForInStatement:
		return r.lowerForIn(n)
	case frontend.NodeThrowStatement:
		return r.lowerThrow(n)
	case frontend.NodeTryStatement:
		return r.lowerTry(n)
	case frontend.NodeSwitchStatement:
		return r.lowerSwitch(n)
	case frontend.NodeBlock:
		// A bare block is a lexical scope, `{ let x = 1; ... }`, which Go spells the
		// same way. It appears as a braced case body and as a standalone scope in a
		// function body, and lowers to a Go block either way.
		return r.lowerBlock(n)
	case frontend.NodeUnknown:
		// The frontend leaves several control-flow statements unnamed, so each surfaces
		// here as an unclassified node the lowering recognizes by its shape: a break or
		// continue by its keyword text, a do...while by a body block and condition, a
		// labeled statement by a label identifier and the statement it labels. Anything
		// else keeps the default hand-back below.
		if s, ok := r.lowerBranch(n); ok {
			return s, nil
		}
		if s, ok, err := r.lowerDoWhile(n); ok || err != nil {
			return s, err
		}
		if s, ok, err := r.lowerLabeled(n); ok || err != nil {
			return s, err
		}
		return nil, &NotYetLowerable{Reason: "statement kind " + kindName(n.Kind()) + " is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "statement kind " + kindName(n.Kind()) + " is a later slice"}
	}
}

// lowerForOf lowers for (const x of arr) over an array to a Go range loop,
// for _, x := range arr.Elems(). Ranging the backing slice visits the elements
// in order, which is exactly what the array iterator does for a dense array, so
// no index arithmetic or bounds check is emitted. Only the array case is
// covered: iterating a string, a Map, a Set, or a general iterable is a later
// slice, and a destructured or already-declared loop variable hands back. The
// loop variable takes the element type from the range, so no explicit type is
// written for it.
func (r *Renderer) lowerForOf(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	// A for await...of leads with an await token before the loop binding, so the parser
	// gives it a fourth child. It awaits each result the async iterator yields, a shape
	// the plain for...of range loop does not model, so it takes its own path.
	if r.isForAwait(n) {
		return r.lowerForAwaitOf(kids[1], kids[2], kids[3])
	}
	if len(kids) != 3 {
		return nil, &NotYetLowerable{Reason: "only for...of with a declaration and a body is lowered yet"}
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, kids[0], &decls)
	if len(decls) != 1 {
		return nil, &NotYetLowerable{Reason: "for...of with other than a single loop binding is a later slice"}
	}
	dkids := r.prog.Children(decls[0])
	if len(dkids) == 1 && dkids[0].Kind() == frontend.NodeUnknown && r.patternNode(dkids[0]) {
		return r.forOfDestructure(kids[1], dkids[0], kids[2])
	}
	if len(dkids) != 1 || dkids[0].Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "for...of with a destructuring or annotated loop variable is a later slice"}
	}
	name, ok := localName(r.prog.Text(dkids[0]))
	if !ok {
		return nil, &NotYetLowerable{Reason: "for...of loop variable is not a Go identifier"}
	}
	// An iterator helper result (IteratorObject, a *value.IterHelper) is pulled with a
	// no-argument Next() until done, distinct from a generator whose Next takes a sent
	// value, so it routes before the generator path which shares the wider iterator type
	// names.
	if r.isIterHelperType(r.prog.TypeAt(kids[1])) {
		return r.forOfIterHelper(kids[1], dkids[0], name, kids[2])
	}
	// A generator iterable is our state-machine closure: for...of pulls it until
	// done rather than ranging a backing slice, so it takes its own path.
	if r.isGeneratorIterable(kids[1]) {
		return r.forOfGenerator(kids[1], dkids[0], name, kids[2])
	}
	// a.values() and a.keys() used directly as the iterable range the receiver rather
	// than build and drive an array iterator object: values binds each element, keys
	// the index as a number. entries with a single binding hands back inside the helper.
	if recv, method, ok := r.arrayIterForOfCall(kids[1]); ok {
		return r.forOfArrayIterSingle(recv, dkids[0], name, method, kids[2])
	}
	// A Set used directly, or a Map/Set keys()/values() call, ranges the runtime's
	// insertion-ordered snapshot rather than build and drive an iterator object. The
	// pair-yielding forms hand back inside the helper, since a [key, value] tuple does
	// not lower.
	if stmt, handled, err := r.forOfMapSetSingle(kids[1], dkids[0], name, kids[2]); handled {
		return stmt, err
	}
	// The iterable is an array (ranged over its Elems) or a string (ranged over its
	// code points). A string yields one substring per Unicode code point, so it
	// lowers to a range over CodePoints() the same way an array ranges over Elems();
	// the loop variable is the string of that code point, which is how the checker
	// types it. The arguments object ranges over the same Elems, off the store the
	// body materialized, yielding each boxed argument. Any other iterable (a Set, a
	// Map, a user iterator) is a later slice and hands back.
	var elemsMethod string
	var iter ast.Expr
	switch {
	case r.argsObjName != "" && r.isArgumentsIdent(kids[1]):
		elemsMethod = "Elems"
		iter = ident(r.argsObjName)
	case isArrayElem(r, kids[1]):
		elemsMethod = "Elems"
	case r.numericTypedArray(kids[1]):
		// A numeric typed array is its own iterable, yielding each element as the
		// Number a read hands out, so for...of ranges its widened elements the same
		// way an array ranges its Elems. The bigint arrays yield bigints, a different
		// element, so they are excluded here and hand back.
		elemsMethod = "Floats"
	case r.isString(kids[1]):
		elemsMethod = "CodePoints"
	default:
		// A user iterable, a class that defines [Symbol.iterator], is walked through
		// the iterator protocol: obtain its iterator and pull it until done, rather
		// than range a backing slice it does not have.
		if shape, ok := r.symbolIteratorShape(r.prog.TypeAt(kids[1])); ok {
			return r.forOfIterator(kids[1], dkids[0], name, kids[2], shape)
		}
		return nil, &NotYetLowerable{Reason: "for...of over a non-array, non-string iterable is a later slice"}
	}
	if iter == nil {
		var err error
		iter, err = r.lowerExpr(kids[1])
		if err != nil {
			return nil, err
		}
	}
	body, err := r.loopBody(kids[2])
	if err != nil {
		return nil, err
	}
	rng := &ast.RangeStmt{
		X:    &ast.CallExpr{Fun: &ast.SelectorExpr{X: iter, Sel: ident(elemsMethod)}},
		Body: body,
	}
	// A JavaScript for...of may bind a loop variable it never reads, the common
	// counting idiom `for (const c of s) n++`, but Go rejects an unused range value
	// and a `for _, _ := range` with no new variable. When the body reads the
	// binding it is ranged into the loop variable; when it does not, the loop drops
	// the binding entirely and ranges only to drive the iteration (`for range xs`).
	if r.bodyUsesName(kids[2], r.prog.Text(dkids[0])) {
		rng.Key = ident("_")
		rng.Value = ident(name)
		rng.Tok = token.DEFINE
	}
	return rng, nil
}

// lowerForIn lowers for (const k in obj) over a dynamic object to a Go range loop,
// for _, k := range obj.ForInKeys().Elems(). ForInKeys reports the names for...in
// visits, the receiver's own then inherited enumerable string keys with shadowing
// applied, so ranging its backing slice binds each name once in the spec's order and
// no key bookkeeping is emitted. The loop variable is the key string, which is how
// the checker types a for...in binding, so no explicit type is written for it.
//
// Only a dynamic (any/unknown) receiver is covered: it lowers to a value.Value that
// carries the ForInKeys method, whereas a statically-shaped object or array lowers to
// a Go struct or slice that does not, so a typed receiver hands back. A destructured
// or already-declared loop variable hands back, the same boundary for...of draws; the
// checker rejects a destructuring for...in head outright, so that form never reaches
// here under the front door.
func (r *Renderer) lowerForIn(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 3 {
		return nil, &NotYetLowerable{Reason: "only for...in with a declaration and a body is lowered yet"}
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, kids[0], &decls)
	if len(decls) != 1 {
		return nil, &NotYetLowerable{Reason: "for...in with other than a single loop binding is a later slice"}
	}
	dkids := r.prog.Children(decls[0])
	if len(dkids) != 1 || dkids[0].Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "for...in with a destructuring or annotated loop variable is a later slice"}
	}
	name, ok := localName(r.prog.Text(dkids[0]))
	if !ok {
		return nil, &NotYetLowerable{Reason: "for...in loop variable is not a Go identifier"}
	}
	// A statically-shaped receiver lowers to a Go struct or slice with no ForInKeys
	// method, so only a dynamic receiver, whose lowering is a value.Value, is enumerated
	// here; a typed object is a later slice.
	if !r.isDynamic(kids[1]) {
		return nil, &NotYetLowerable{Reason: "for...in over a statically-typed object is a later slice"}
	}
	recv, err := r.lowerExpr(kids[1])
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(kids[2])
	if err != nil {
		return nil, err
	}
	rng := &ast.RangeStmt{
		X: &ast.CallExpr{Fun: &ast.SelectorExpr{
			X:   &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ForInKeys")}},
			Sel: ident("Elems"),
		}},
		Body: body,
	}
	// A for...in may bind a key it never reads, so when the body does not use the
	// binding the loop drops it and ranges only to drive the iteration, the same
	// unused-binding handling for...of applies; Go rejects an unused range value.
	if r.bodyUsesName(kids[2], r.prog.Text(dkids[0])) {
		rng.Key = ident("_")
		rng.Value = ident(name)
		rng.Tok = token.DEFINE
	}
	return rng, nil
}

// isGeneratorIterable reports whether the checker types n as one of the built-in
// generator or iterator shapes bento lowers to a next() closure, so for...of
// pulls it rather than ranging a slice. The judgment is the iterable type's
// symbol name, the same built-in family the generator method decl produces.
func (r *Renderer) isGeneratorIterable(n frontend.Node) bool {
	sym, ok := r.prog.TypeSymbol(r.prog.TypeAt(n))
	if !ok {
		return false
	}
	switch sym.Name {
	case "Generator", "IterableIterator", "Iterator", "IteratorObject":
		return true
	}
	return false
}

// forOfGenerator lowers a for...of over a generator to the plain Go loop a
// developer writes against a coroutine: obtain the *value.Gen once so its state is
// shared across the loop, then pull a value and a done flag each turn with
// Next(value.Undefined) and stop when done. The generator is obtained once up front,
// not per turn, so two iterations of the same source expression would each get their
// own fresh coroutine, matching the JavaScript protocol. A binding the body never
// reads drops to the blank identifier, since Go rejects an unused loop value.
//
// A loop the body can break out of early abandons the generator with its goroutine
// still parked on the next yield, so an early break must close it: the loop tracks
// whether it broke with a flag and calls Stop after the loop when it did, which
// resumes the suspended body through its finally blocks and lets the goroutine exit,
// the same shape forOfIterator uses to call an iterator's return(). A body that can
// leave the loop another way, a return, a throw, or a labeled branch, would jump past
// that close and leak the goroutine, so it hands back rather than leak silently. A
// body with no early exit runs the generator to done, which needs no close, so the
// drain machinery is left out and the loop stays the plain pull-until-done form.
func (r *Renderer) forOfGenerator(iterable, bindNode frontend.Node, name string, bodyNode frontend.Node) (ast.Stmt, error) {
	if r.forOfBodyBypassesClose(bodyNode) {
		return nil, &NotYetLowerable{Reason: "stopping a generator on a return, throw, or labeled exit from for...of is a later slice"}
	}
	iter, err := r.lowerExpr(iterable)
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	genName := r.freshTemp()
	doneName := r.freshTemp()
	loopVar := ast.Expr(ident("_"))
	if r.bodyUsesName(bodyNode, r.prog.Text(bindNode)) {
		loopVar = ident(name)
	}
	pull := &ast.AssignStmt{
		Lhs: []ast.Expr{loopVar, ident(doneName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ident(genName), Sel: ident("Next")},
			Args: []ast.Expr{sel("value", "Undefined")},
		}},
	}
	// A break that abandons the generator needs the close, done exhaustion does not.
	// When the body can break, a flag starts true and clears on the done branch, so
	// after the loop a break leaves it true and Stop runs, while a run to done leaves
	// it false and Stop is skipped.
	closes := r.forOfBodyMayBreak(bodyNode)
	block := []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ident(genName)}, Tok: token.DEFINE, Rhs: []ast.Expr{iter}}}
	doneStmts := []ast.Stmt{}
	var brokeName string
	if closes {
		brokeName = r.freshTemp()
		block = append(block, &ast.AssignStmt{Lhs: []ast.Expr{ident(brokeName)}, Tok: token.DEFINE, Rhs: []ast.Expr{ident("true")}})
		doneStmts = append(doneStmts, &ast.AssignStmt{Lhs: []ast.Expr{ident(brokeName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{ident("false")}})
	}
	doneStmts = append(doneStmts, &ast.BranchStmt{Tok: token.BREAK})
	brk := &ast.IfStmt{Cond: ident(doneName), Body: &ast.BlockStmt{List: doneStmts}}
	loop := &ast.ForStmt{Body: &ast.BlockStmt{List: append([]ast.Stmt{pull, brk}, body.List...)}}
	block = append(block, loop)
	if closes {
		block = append(block, &ast.IfStmt{
			Cond: ident(brokeName),
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{X: ident(genName), Sel: ident("Stop")},
			}}}},
		})
	}
	return &ast.BlockStmt{List: block}, nil
}

// forOfIterHelper lowers a for...of over an iterator helper result (an IteratorObject,
// a *value.IterHelper) to the plain pull loop the runtime drives: obtain the helper
// once so its state is shared across the loop, then each turn pull a step with Next()
// and stop when the step is done. A helper's Next takes no sent value, unlike a
// generator's, so this path is distinct from forOfGenerator; and a helper has no goroutine
// to abandon, so an early break needs no close and the drain machinery forOfGenerator
// carries is left out.
//
// A step yields a boxed value.Value in its Value field, so the loop variable coerces the
// box down to its element type: a clean primitive runs the ToNumber family, an any binding
// keeps the box, and any other element type hands back rather than land a box in a slot
// that cannot hold it. A binding the body never reads drops entirely, since Go rejects an
// unused variable, and the loop still pulls to drive the iteration.
func (r *Renderer) forOfIterHelper(iterable, bindNode frontend.Node, name string, bodyNode frontend.Node) (ast.Stmt, error) {
	iter, err := r.lowerExpr(iterable)
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	itName := r.freshTemp()
	stepName := r.freshTemp()
	pull := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(stepName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(itName), Sel: ident("Next")}}},
	}
	brk := &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: ident(stepName), Sel: ident("Done")},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}},
	}
	loopStmts := []ast.Stmt{pull, brk}
	if r.bodyUsesName(bodyNode, r.prog.Text(bindNode)) {
		bound, err := r.bindIterHelperValue(&ast.SelectorExpr{X: ident(stepName), Sel: ident("Value")}, bindNode)
		if err != nil {
			return nil, err
		}
		loopStmts = append(loopStmts, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.DEFINE, Rhs: []ast.Expr{bound}})
	}
	loopStmts = append(loopStmts, body.List...)
	loop := &ast.ForStmt{Body: &ast.BlockStmt{List: loopStmts}}
	block := []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ident(itName)}, Tok: token.DEFINE, Rhs: []ast.Expr{iter}},
		loop,
	}
	return &ast.BlockStmt{List: block}, nil
}

// bindIterHelperValue coerces a helper step's boxed Value into the loop variable's
// element type, the same three-way choice unboxDynamicRead makes: a clean primitive
// coerces through the ToNumber family, an any or unknown binding keeps the box, and any
// other element type hands back rather than force a box into a slot that cannot hold it.
func (r *Renderer) bindIterHelperValue(valExpr ast.Expr, bindNode frontend.Node) (ast.Expr, error) {
	flags := r.prog.TypeAt(bindNode).Flags
	if flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 &&
		flags&(frontend.TypeNumber|frontend.TypeString|frontend.TypeBoolean) != 0 {
		return r.coerceDynamicToStaticFlags(valExpr, flags)
	}
	if flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
		return valExpr, nil
	}
	return nil, &NotYetLowerable{Reason: "for...of over an iterator helper binding a non-primitive element is a later slice"}
}

// forOfBodyMayBreak reports whether a for...of body can leave the loop through an
// unlabeled break that targets this loop, the one exit that abandons a generator with
// the goroutine still parked. An unlabeled break inside a nested loop or switch targets
// that construct, not this loop, so those are not descended into, and neither is a
// nested function whose own break belongs to a loop of its own. When no such break
// exists the loop can only end by running the generator to done, which needs no close.
func (r *Renderer) forOfBodyMayBreak(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeFunctionDeclaration, frontend.NodeFunctionExpression, frontend.NodeArrowFunction,
		frontend.NodeMethodDeclaration, frontend.NodeGetAccessor, frontend.NodeSetAccessor, frontend.NodeConstructor,
		frontend.NodeForStatement, frontend.NodeForOfStatement, frontend.NodeForInStatement,
		frontend.NodeWhileStatement, frontend.NodeSwitchStatement:
		return false
	case frontend.NodeUnknown:
		if branchKeyword(strings.TrimSpace(r.prog.Text(n))) == "break" {
			// An unlabeled break carries no target identifier and hits this loop; a
			// labeled break is handled as a bypass in forOfBodyBypassesClose.
			return len(r.prog.Children(n)) == 0
		}
	}
	for _, k := range r.prog.Children(n) {
		if r.forOfBodyMayBreak(k) {
			return true
		}
	}
	return false
}

// forOfDestructure lowers a for...of whose loop binding is a destructuring pattern.
// A flat array pattern over a homogeneous array of arrays keeps the optimized path
// that can drop an unused element binding; an object pattern, a nested pattern, or any
// other shape binds through the shared recursive binder against a per-iteration
// temporary, the same struct-field selectors and indexed reads a `const {a} = e` or
// `const [x] = e` statement lowers to. Only an array iterable is ranged over its Elems;
// a Set, a Map, or a user iterator is a later slice, since destructuring one needs the
// iterator protocol the single-variable loop walks, not a backing slice.
func (r *Renderer) forOfDestructure(iterable, pattern, bodyNode frontend.Node) (ast.Stmt, error) {
	// `for (const [i, v] of a.entries())` ranges the receiver array directly, its
	// index and element binding the two pattern names, rather than build entry pairs
	// an array iterator would only hand back one at a time.
	if recv, method, ok := r.arrayIterForOfCall(iterable); ok && method == "entries" {
		if strings.HasPrefix(strings.TrimSpace(r.prog.Text(pattern)), "[") {
			return r.forOfEntriesDestructure(recv, pattern, bodyNode)
		}
	}
	// `for (const [k, v] of map)`, its entries() spelling, and a Set's entries() range
	// the runtime's insertion-ordered snapshot and bind the pattern's two names
	// directly, since the [K, V] tuple a pair would otherwise take does not lower.
	if stmt, handled, err := r.forOfMapSetDestructure(iterable, pattern, bodyNode); handled {
		return stmt, err
	}
	if !isArrayElem(r, iterable) {
		return nil, &NotYetLowerable{Reason: "a destructuring for...of over a non-array iterable is a later slice"}
	}
	outerElem, ok := r.prog.ElementType(r.prog.TypeAt(iterable))
	if !ok {
		return nil, &NotYetLowerable{Reason: "a destructuring for...of whose iterable has no element type is a later slice"}
	}
	// An array whose element is dynamic (a value.Value, the element of an any[]) has no
	// static shape for the typed binder to read, so the pattern binds against each element
	// through the same dynamic protocol an untyped pattern uses in a parameter or a
	// declaration: object properties through Get and positions through GetIndex. The read
	// is only sound when the array's Go storage is a value.Value slice, which a parameter
	// typed any[] takes but a const bound to a typed array literal does not: the checker
	// types both elements any, yet the const's slice holds the literal's shaped element,
	// which carries no Get. So the dynamic head fires only for a provably boxed-element
	// iterable and hands the shaped-storage case back rather than emit a read it cannot make.
	if outerElem.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
		if !r.iterableElemBoxed(iterable) {
			return nil, &NotYetLowerable{Reason: "a for...of over an array the checker types any[] but whose Go storage holds a shaped element is a later slice"}
		}
		return r.forOfDynamicDestructure(iterable, pattern, bodyNode)
	}
	// A flat array pattern over a homogeneous array of arrays keeps the optimized path,
	// which drops an unused element binding and reuses a name the pattern repeats.
	if strings.HasPrefix(strings.TrimSpace(r.prog.Text(pattern)), "[") && r.forOfFlatArrayEligible(pattern, outerElem) {
		return r.forOfArrayDestructure(iterable, pattern, bodyNode)
	}
	// The shared binder declares each bound name with :=, which Go rejects if the body
	// never reads it. The flat path drops such a name; the general binder does not, so a
	// pattern with an unused bound name hands back rather than emit a name that a
	// declared-and-not-used error would reject.
	for _, nm := range r.patternBoundNames(pattern) {
		if !r.bodyUsesName(bodyNode, nm) {
			return nil, &NotYetLowerable{Reason: "a for...of destructuring with an unused bound name is a later slice"}
		}
	}
	iter, err := r.lowerExpr(iterable)
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, err
	}
	// The range value is a fresh binding each iteration, so the temporary needs no
	// reset and each iteration's reads see that element.
	tmp := r.freshTemp()
	binds, err := r.bindSubPattern(pattern, ident(tmp), outerElem, token.DEFINE)
	if err != nil {
		return nil, err
	}
	body.List = append(binds, body.List...)
	return &ast.RangeStmt{
		X:     &ast.CallExpr{Fun: &ast.SelectorExpr{X: iter, Sel: ident("Elems")}},
		Key:   ident("_"),
		Value: ident(tmp),
		Tok:   token.DEFINE,
		Body:  body,
	}, nil
}

// forOfDynamicDestructure lowers a for...of whose element is dynamic, `for (const {a} of
// xs)` where xs is any[]: the array ranges over its Elems the way the typed path does, but
// each element is a boxed value.Value the pattern binds against through the dynamic Get and
// GetIndex protocol rather than the struct-field selectors a shaped element would take. The
// per-iteration temporary holds one element, and the pattern's rest bindings are marked
// dynamic before the body lowers so their reads route the boxed way, the same as an untyped
// parameter's. An unused bound name is blanked by the shared unused-binding pass, so this
// path does not hand back on one the way the typed general binder must.
func (r *Renderer) forOfDynamicDestructure(iterable, pattern, bodyNode frontend.Node) (ast.Stmt, error) {
	iter, err := r.lowerExpr(iterable)
	if err != nil {
		return nil, err
	}
	// The pattern binds before the body lowers so every name it introduces is marked
	// dynamic: an element read off a boxed value.Value is itself a value.Value, and the
	// body's reads of that name must route the dynamic way rather than take the checker's
	// element type. The names come straight off the emitted binds, exact for a leaf the
	// checker typed concretely.
	tmp := r.freshTemp()
	binds, err := r.bindDynamicPattern(pattern, ident(tmp), token.DEFINE)
	if err != nil {
		return nil, err
	}
	prevDyn := r.dynBoundLocals
	m := map[string]bool{}
	for name := range prevDyn {
		m[name] = true
	}
	r.collectAssignedNames(binds, m)
	r.dynBoundLocals = m
	defer func() { r.dynBoundLocals = prevDyn }()
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, err
	}
	body.List = append(binds, body.List...)
	return &ast.RangeStmt{
		X:     &ast.CallExpr{Fun: &ast.SelectorExpr{X: iter, Sel: ident("Elems")}},
		Key:   ident("_"),
		Value: ident(tmp),
		Tok:   token.DEFINE,
		Body:  body,
	}, nil
}

// iterableElemBoxed reports whether a for...of iterable's Go storage is a slice of
// boxed value.Value, the only shape whose element carries the dynamic Get and GetIndex
// the untyped head reads through. A parameter typed any[] takes that slice, since a
// parameter lowers to its declared type; a const or let bound to an array literal does
// not, since foldShortDecl gives it the literal's shaped element even under an any[]
// annotation. The two report the same any element type, so the storage is read off the
// symbol's declaration, not the type: only a parameter declaration is provably boxed.
// Any other iterable (a shaped-storage local, a call result, a member) is not proven
// boxed here and stays on the hand-back branch, honest rather than emitting a read the
// element cannot make.
func (r *Renderer) iterableElemBoxed(iterable frontend.Node) bool {
	if iterable.Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(iterable)
	if !ok {
		return false
	}
	return slices.ContainsFunc(r.prog.Declarations(sym), func(d frontend.Node) bool {
		return d.Kind() == frontend.NodeParameter
	})
}

// forOfFlatArrayEligible reports whether an array pattern loop variable is the flat
// homogeneous shape the optimized forOfArrayDestructure path handles: the iterable's
// element is itself an array, and every pattern element is a plain identifier whose
// type matches that inner element type. Any other shape (an object element, a nested
// pattern, a mixed-type tuple, a default, or a rest) falls to the general binder.
func (r *Renderer) forOfFlatArrayEligible(pattern frontend.Node, outerElem frontend.Type) bool {
	innerElem, ok := r.prog.ElementType(outerElem)
	if !ok {
		return false
	}
	innerGo, err := r.typeExpr(innerElem)
	if err != nil {
		return false
	}
	elems := r.prog.Children(pattern)
	if len(elems) == 0 {
		return false
	}
	for _, el := range elems {
		ec := r.prog.Children(el)
		if len(ec) != 1 || ec[0].Kind() != frontend.NodeIdentifier {
			return false
		}
		nameGo, err := r.typeExpr(r.prog.TypeAt(ec[0]))
		if err != nil {
			return false
		}
		if same, err := sameGoType(nameGo, innerGo); err != nil || !same {
			return false
		}
	}
	return true
}

// patternBoundNames collects the leaf names a binding pattern declares, recursing
// through nested patterns so the for...of unused-name guard sees every name the
// shared binder would declare. A property name in a rename ({a: b}) is not a binding,
// so only the target b is collected; an element whose shape the binder itself hands
// back on is skipped here, since the binder's own error carries the handback.
func (r *Renderer) patternBoundNames(pat frontend.Node) []string {
	isObj := strings.HasPrefix(strings.TrimSpace(r.prog.Text(pat)), "{")
	var out []string
	for _, el := range r.prog.Children(pat) {
		if isObj {
			if _, sub, ok := r.objectNestedElem(el); ok {
				out = append(out, r.patternBoundNames(sub)...)
				continue
			}
			info, err := r.classifyObjectElem(el)
			if err != nil {
				continue
			}
			if nm, ok := localName(r.prog.Text(info.bindNode)); ok {
				out = append(out, nm)
			}
			continue
		}
		ec := r.prog.Children(el)
		if len(ec) == 1 && r.patternNode(ec[0]) {
			out = append(out, r.patternBoundNames(ec[0])...)
			continue
		}
		info, err := r.classifyArrayElem(el)
		if err != nil {
			continue
		}
		if info.nested != nil {
			out = append(out, r.patternBoundNames(info.nested)...)
			continue
		}
		if info.nameNode != nil {
			if nm, ok := localName(r.prog.Text(info.nameNode)); ok {
				out = append(out, nm)
			}
		}
	}
	return out
}

// forOfArrayDestructure lowers `for (const [a, b] of pairs)` over an array of arrays
// to a range loop whose element is bound to a generated temporary and destructured at
// the top of the body, for _, e := range pairs.Elems() { a := e.AtI(0); b := e.AtI(1);
// ... }. The range value is a fresh binding each iteration, so the temporary needs no
// reset and the positional reads see that iteration's element. Only a flat array
// pattern over an array-of-arrays iterable is lowered: the iterable's element must be
// an array so the positional read has something to index, and each name's type must
// match that inner element type. A hole, a default, a rest, a nested pattern, a
// non-array iterable, or a non-array element hands back, each a later slice. A name
// the body never reads is dropped rather than bound, the same unused-binding rule the
// single-variable loop applies, so the Go loop compiles.
func (r *Renderer) forOfArrayDestructure(iterable, pattern, bodyNode frontend.Node) (ast.Stmt, error) {
	if !isArrayElem(r, iterable) {
		return nil, &NotYetLowerable{Reason: "a destructuring for...of over a non-array iterable is a later slice"}
	}
	outerElem, ok := r.prog.ElementType(r.prog.TypeAt(iterable))
	if !ok {
		return nil, &NotYetLowerable{Reason: "a destructuring for...of whose iterable has no element type is a later slice"}
	}
	innerElem, ok := r.prog.ElementType(outerElem)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a destructuring for...of whose element is not itself an array is a later slice"}
	}
	innerGo, err := r.typeExpr(innerElem)
	if err != nil {
		return nil, err
	}
	elems := r.prog.Children(pattern)
	if len(elems) == 0 {
		return nil, &NotYetLowerable{Reason: "an empty for...of destructuring pattern binds nothing"}
	}
	// Each element must be a flat name whose type matches the inner element type, so
	// the AtI read binds it without coercion. The reads are gathered first, so an
	// unsupported shape hands the whole loop back before any temporary is minted.
	type binding struct {
		name string
		idx  int
	}
	var used []binding
	seen := make(map[string]int)
	for i, el := range elems {
		ec := r.prog.Children(el)
		if len(ec) != 1 || ec[0].Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: "a for...of destructuring hole, default, rest, or nested pattern is a later slice"}
		}
		nameNode := ec[0]
		name, ok := localName(r.prog.Text(nameNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "a for...of destructured name is not a Go identifier"}
		}
		nameGo, err := r.typeExpr(r.prog.TypeAt(nameNode))
		if err != nil {
			return nil, err
		}
		if same, err := sameGoType(nameGo, innerGo); err != nil {
			return nil, err
		} else if !same {
			return nil, &NotYetLowerable{Reason: "a for...of destructuring where an element's type differs from the array element type is a later slice"}
		}
		// A name the body never reads is dropped: binding it would leave an unused Go
		// local, and the read it drives has no effect worth keeping.
		if !r.bodyUsesName(bodyNode, r.prog.Text(nameNode)) {
			continue
		}
		// A pattern may repeat a name ([x, x]), and JavaScript binds it once with the
		// last element winning. The element reads are pure AtI lookups, so an earlier
		// duplicate is a dead store; keep one binding per name and point it at the last
		// index, which also avoids emitting a second `x :=` that Go rejects as no new
		// variables on the left. for-of/head-var-bound-names-dup exercises this.
		if pos, dup := seen[name]; dup {
			used[pos].idx = i
			continue
		}
		seen[name] = len(used)
		used = append(used, binding{name: name, idx: i})
	}
	iter, err := r.lowerExpr(iterable)
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, err
	}
	rng := &ast.RangeStmt{
		X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: iter, Sel: ident("Elems")}},
	}
	// With no name read, the loop only drives the iteration and binds nothing, so it
	// ranges without a value the way the counting single-variable loop does.
	if len(used) == 0 {
		rng.Body = body
		return rng, nil
	}
	tmp := r.freshTemp()
	reads := make([]ast.Stmt, 0, len(used))
	for _, b := range used {
		reads = append(reads, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(b.name)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: ident(tmp), Sel: ident("AtI")},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(b.idx)}},
			}},
		})
	}
	body.List = append(reads, body.List...)
	rng.Key = ident("_")
	rng.Value = ident(tmp)
	rng.Tok = token.DEFINE
	rng.Body = body
	return rng, nil
}

// isArrayElem reports whether n is an array-typed for...of iterable, the receiver
// whose element type arrayElem resolves. It wraps the arrayElem probe so the
// for...of dispatch reads as a set of iterable kinds rather than a bare ok check.
func isArrayElem(r *Renderer, n frontend.Node) bool {
	_, ok := r.arrayElem(n)
	return ok
}

// bodyUsesName reports whether the subtree rooted at n contains an identifier
// whose source text is name. lowerForOf uses it to decide whether a for...of loop
// variable is read in the body: a binding the body never mentions ranges into the
// blank identifier so the Go loop compiles. The scan is by source text, the same
// spelling the binding carries, so it sees a read however deeply nested. A body
// that shadows the binding with its own declaration of the same name is a
// pathological case this over-approximates as a use, which only keeps the binding.
func (r *Renderer) bodyUsesName(n frontend.Node, name string) bool {
	if n.Kind() == frontend.NodeIdentifier {
		return r.prog.Text(n) == name
	}
	for _, c := range r.prog.Children(n) {
		if r.bodyUsesName(c, name) {
			return true
		}
	}
	return false
}

// lowerReturn lowers a return, with or without a value.
func (r *Renderer) lowerReturn(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	// A return inside a generator body completes the coroutine, whose func returns a
	// value.Value the { value, done: true } result carries. A bare return completes
	// with undefined; a return that carries a value boxes it into the dynamic value
	// the completion frame holds, so a manual driver reading the done result sees it.
	if r.genCo != "" {
		if len(kids) == 0 {
			r.requireImport(valuePkg)
			return &ast.ReturnStmt{Results: []ast.Expr{sel("value", "Undefined")}}, nil
		}
		boxed, err := r.boxOperand(kids[0])
		if err != nil {
			return nil, err
		}
		return &ast.ReturnStmt{Results: []ast.Expr{boxed}}, nil
	}
	if len(kids) == 0 {
		// Inside a try escape closure the bare return of a void function still has
		// to raise done, or the call site would not return; the closure's own
		// fall-off return is the only bare one.
		if r.tryRet == tryRetBody {
			if !isVoidReturn(r.retType) {
				return nil, &NotYetLowerable{Reason: "a bare return of undefined from a value-returning try is a later slice"}
			}
			return &ast.ReturnStmt{Results: []ast.Expr{ident("true")}}, nil
		}
		return &ast.ReturnStmt{}, nil
	}
	if r.tryRet == tryRetBody && isVoidReturn(r.retType) {
		return nil, &NotYetLowerable{Reason: "a value returned from a void function inside a try is a later slice"}
	}
	// An object literal returned into a declared shape with an optional property
	// builds at that shape, the same contextual typing a binding applies; a return
	// type that is itself T | undefined hands back to a later slice.
	if kids[0].Kind() == frontend.NodeObjectLiteralExpression {
		if shape, wrap, ok := r.contextualObjectShape(r.retType); ok {
			if wrap {
				return nil, &NotYetLowerable{Reason: "an object literal returned into a T | undefined optional slot is a later slice"}
			}
			expr, err := r.objectLiteralContextual(kids[0], shape)
			if err != nil {
				return nil, err
			}
			return &ast.ReturnStmt{Results: []ast.Expr{expr}}, nil
		}
	}
	expr, err := r.lowerExpr(kids[0])
	if err != nil {
		return nil, err
	}
	expr, err = r.coerceReturn(expr, kids[0])
	if err != nil {
		return nil, err
	}
	// In a try escape closure the return also raises done, filling the closure's
	// named results in one statement; a plain function return carries the value
	// alone.
	if r.tryRet == tryRetBody {
		return &ast.ReturnStmt{Results: []ast.Expr{expr, ident("true")}}, nil
	}
	return &ast.ReturnStmt{Results: []ast.Expr{expr}}, nil
}

// deferredReturn lowers a return inside a catch or finally body of a try whose
// returns escape. The body runs inside a deferred function, and the only way a
// deferred function sets its enclosing closure's results is by assigning the
// named ones, so the return becomes `ret, done = x, true` followed by a bare
// return out of the handler; the always form's closure has no done, so there
// the assignment fills ret alone, and a void function's fills just done. The
// object-literal contextual path is left to a later slice, so a returned
// literal that needs it hands back through the plain lowering's shape checks.
func (r *Renderer) deferredReturn(n frontend.Node) ([]ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		if !isVoidReturn(r.retType) {
			return nil, &NotYetLowerable{Reason: "a bare return of undefined from a value-returning try is a later slice"}
		}
		return []ast.Stmt{
			&ast.AssignStmt{Lhs: []ast.Expr{ident("done")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ident("true")}},
			&ast.ReturnStmt{},
		}, nil
	}
	if isVoidReturn(r.retType) {
		return nil, &NotYetLowerable{Reason: "a value returned from a void function inside a try is a later slice"}
	}
	expr, err := r.lowerExpr(kids[0])
	if err != nil {
		return nil, err
	}
	expr, err = r.coerceReturn(expr, kids[0])
	if err != nil {
		return nil, err
	}
	lhs := []ast.Expr{ident("ret"), ident("done")}
	rhs := []ast.Expr{expr, ident("true")}
	if r.tryRet == tryRetDeferPlain {
		lhs, rhs = lhs[:1], rhs[:1]
	}
	return []ast.Stmt{
		&ast.AssignStmt{Lhs: lhs, Tok: token.ASSIGN, Rhs: rhs},
		&ast.ReturnStmt{},
	}, nil
}

// lowerVarStatement lowers a const or let statement to Go var declarations, one
// per binding. Both const and let become a Go var, because a Go const cannot
// hold a runtime float64 initializer and TypeScript already forbids reassigning
// a const, so the mutability distinction is enforced upstream, not here. The Go
// type is always written explicitly from the checker's inferred type rather than
// left to :=, because a bare integer literal would infer Go int where the source
// means float64. A binding with no initializer, or one carrying a type
// annotation node this slice does not read yet, hands back.
func (r *Renderer) lowerVarStatement(n frontend.Node) (ast.Stmt, error) {
	var decls []frontend.Node
	collectVarDecls(r.prog, n, &decls)
	return r.varDeclStmt(decls)
}

// lowerVarStatementMulti lowers a variable statement and follows it with a blank
// assignment for every binding the module never reads. Go rejects a local that is
// declared and not used, but the initializer of an unused `var x = e;` must still
// run, so the binding stays and `_ = x` marks it used without changing behavior.
// A binding that is read anywhere gets no blank, and a destructuring target, whose
// name node is not a plain identifier, is left to its own lowering. A `var` that
// redeclares a name the block or scope already declared lowers to an assignment,
// not a fresh declaration, so its blank belongs with the first declaration alone; a
// name already declared before this statement is skipped here so an unread
// redeclared var gets exactly one blank, not one per `var`.
func (r *Renderer) lowerVarStatementMulti(n frontend.Node) ([]ast.Stmt, error) {
	var decls []frontend.Node
	collectVarDecls(r.prog, n, &decls)
	// Note which names are seeing their first Go declaration in this statement before
	// it lowers, since lowering marks them declared and would otherwise make every
	// later redeclaration look fresh too.
	fresh := make([]bool, len(decls))
	for i, d := range decls {
		kids := r.prog.Children(d)
		if len(kids) == 0 {
			continue
		}
		if name, ok := localName(r.prog.Text(kids[0])); ok {
			fresh[i] = !r.blockDeclares(name) && !r.hoistedVars[name] && !r.moduleAssignVars[name]
		}
	}
	s, err := r.lowerVarStatement(n)
	if err != nil {
		return nil, err
	}
	out := []ast.Stmt{s}
	for i, d := range decls {
		if !fresh[i] {
			continue
		}
		kids := r.prog.Children(d)
		if len(kids) == 0 {
			continue
		}
		name, ok := localName(r.prog.Text(kids[0]))
		if !ok {
			continue
		}
		if r.bindingUnused(kids[0]) {
			out = append(out, &ast.AssignStmt{
				Lhs: []ast.Expr{ident("_")},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{ident(name)},
			})
		}
	}
	return out, nil
}

// varDeclStmt builds a Go statement for a set of variable declaration nodes. It is
// shared by a const/let statement and a for-loop initializer, so both spell a
// binding the same way. A `var` that redeclares a name already declared in this Go
// block lowers to an assignment instead of a second declaration, since Go rejects a
// duplicate short declaration where JavaScript allows the redeclaration; every
// other statement builds a fresh Go declaration and records its names as declared.
func (r *Renderer) varDeclStmt(decls []frontend.Node) (ast.Stmt, error) {
	if len(decls) == 0 {
		return nil, &NotYetLowerable{Reason: "variable declaration has no binding"}
	}
	if stmt, ok, err := r.redeclaredVarAssign(decls); err != nil || ok {
		return stmt, err
	}
	stmt, err := r.buildVarDecl(decls)
	if err != nil {
		return nil, err
	}
	for _, d := range decls {
		if name, ok := localName(r.prog.Text(r.prog.Children(d)[0])); ok {
			r.markBlockDeclared(name)
		}
	}
	return stmt, nil
}

// redeclaredVarAssign turns a `var` statement whose bindings all name variables the
// current block already declared, or that the scope hoisted to its top, into
// assignments to those variables. JavaScript hoists a `var` to one binding per
// scope, so `var x = a; var x = b;` is a single x assigned twice, and a var written
// in a nested block whose scope declared it at the top is likewise one binding, both
// of which Go writes as a declaration and then a plain assignment. A
// binding with no initializer keeps the current value and emits nothing, matching a
// bare `var x;` that does not reset x. A statement that mixes a redeclared name with
// a fresh one, or names a destructuring target, is a later slice and hands back;
// one with no redeclared name reports ok=false and takes the ordinary declaration
// path.
func (r *Renderer) redeclaredVarAssign(decls []frontend.Node) (ast.Stmt, bool, error) {
	redeclared := 0
	for _, d := range decls {
		kids := r.prog.Children(d)
		if len(kids) == 0 {
			return nil, false, nil
		}
		name, ok := localName(r.prog.Text(kids[0]))
		if !ok {
			return nil, false, nil
		}
		if r.blockDeclares(name) || r.hoistedVars[name] || r.moduleAssignVars[name] {
			redeclared++
		}
	}
	if redeclared == 0 {
		return nil, false, nil
	}
	if redeclared != len(decls) {
		return nil, true, &NotYetLowerable{Reason: "a var statement that mixes a redeclared name with a new one is a later slice"}
	}
	var lhs, rhs []ast.Expr
	for _, d := range decls {
		kids := r.prog.Children(d)
		name, _ := localName(r.prog.Text(kids[0]))
		initIdx := -1
		if len(kids) >= 2 && kids[len(kids)-1].Kind() != frontend.NodeUnknown {
			initIdx = len(kids) - 1
		}
		if initIdx < 0 {
			continue
		}
		init, err := r.bindingInit(kids[0], kids[initIdx])
		if err != nil {
			return nil, true, err
		}
		lhs = append(lhs, ident(name))
		rhs = append(rhs, init)
	}
	if len(lhs) == 0 {
		return &ast.EmptyStmt{Implicit: true}, true, nil
	}
	return &ast.AssignStmt{Lhs: lhs, Tok: token.ASSIGN, Rhs: rhs}, true, nil
}

// buildVarDecl builds a Go var declaration statement from a set of variable
// declaration nodes.
func (r *Renderer) buildVarDecl(decls []frontend.Node) (ast.Stmt, error) {
	if len(decls) == 0 {
		return nil, &NotYetLowerable{Reason: "variable declaration has no binding"}
	}
	// A lone float64 binding off a decimal literal reads better as name := 0.0 than as
	// var name float64 = 0, and the two declare the same block-scoped variable. Every
	// shape the fold declines (an int32 counter, a multi-binding statement, a non-decimal
	// initializer) keeps the typed var form below.
	if short, ok := r.foldFloatDecl(decls); ok {
		return short, nil
	}
	// A lone binding whose initializer already carries the binding's own type reads
	// better as name := value than as var name T = value, and infers the same type.
	// This covers a string, an array, or any local built from a constructor or a
	// bridged call, everything past the numeric literals foldFloatDecl keeps.
	if short, ok := r.foldShortDecl(decls); ok {
		return short, nil
	}
	specs := make([]ast.Spec, 0, len(decls))
	for _, d := range decls {
		kids := r.prog.Children(d)
		// A binding is [name], [name, type], [name, initializer], or [name, type,
		// initializer]: the name is always first, an optional type annotation follows,
		// and an optional initializer is last. A type annotation is not an expression,
		// so the adapter leaves it unclassified as NodeUnknown, which tells it apart
		// from an initializer, whose node always carries a real expression kind. The Go
		// type is read from the checker's type for the name either way, so the
		// annotation node itself is only skipped, never lowered.
		name, ok := localName(r.prog.Text(kids[0]))
		if !ok {
			return nil, &NotYetLowerable{Reason: "variable name is not a Go identifier"}
		}
		initIdx := -1
		if len(kids) >= 2 && kids[len(kids)-1].Kind() != frontend.NodeUnknown {
			initIdx = len(kids) - 1
		}
		// A binding with no initializer holds undefined until its first assignment, the
		// way `var x;` reads undefined in JavaScript. A dynamic binding (any or unknown)
		// lowers to value.Value, whose Go zero value is exactly that undefined, so a
		// bare var declaration with no value is correct on its own. A binding with a
		// static type has a Go zero value that is not undefined (0 for a number, "" for
		// a string), so a typed declaration with no initializer would observe the wrong
		// value if it were read before assignment and hands back for a later slice.
		if initIdx < 0 {
			if r.prog.TypeAt(kids[0]).Flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 {
				return nil, &NotYetLowerable{Reason: "a statically typed binding with no initializer is a later slice"}
			}
			r.requireImport(valuePkg)
			specs = append(specs, &ast.ValueSpec{
				Names: []*ast.Ident{ident(name)},
				Type:  sel("value", "Value"),
			})
			continue
		}
		// A local the analysis proved holds only 32-bit integers is declared as a Go
		// int32 and its initializer is lowered in the int32 domain, so the counter or
		// accumulator lives in a register with no float64 coercion on any of its
		// operations. Every other local keeps its float64 (or richer) type and the
		// ordinary boundary-coercing initializer.
		if r.int32Locals[name] {
			init, err := r.int32Of(kids[initIdx])
			if err != nil {
				return nil, err
			}
			specs = append(specs, &ast.ValueSpec{
				Names:  []*ast.Ident{ident(name)},
				Type:   ident("int32"),
				Values: []ast.Expr{init},
			})
			continue
		}
		// A local proven to hold safe integers wider than 32 bits is declared as a
		// Go int64 the same way, its initializer lowered in the int64 domain.
		if r.int64Locals[name] {
			init, err := r.int64Of(kids[initIdx])
			if err != nil {
				return nil, err
			}
			specs = append(specs, &ast.ValueSpec{
				Names:  []*ast.Ident{ident(name)},
				Type:   ident("int64"),
				Values: []ast.Expr{init},
			})
			continue
		}
		// A binding initialized by re.exec(s), str.match(re), or str.split(re) holds the
		// boxed value.Value the match returns, an array or null. The checker types each
		// with a concrete Go shape the box does not have (RegExpExecArray | null,
		// RegExpMatchArray | null, or string[]), so the binding lands in a value.Value
		// slot and is marked dynamic, which routes the later null compare and the element
		// and property reads off the result through the value model rather than the
		// static shape the concrete type would otherwise name.
		if r.regExpBoxedResultCall(kids[initIdx]) {
			execInit, err := r.lowerExpr(kids[initIdx])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			specs = append(specs, &ast.ValueSpec{
				Names:  []*ast.Ident{ident(name)},
				Type:   sel("value", "Value"),
				Values: []ast.Expr{execInit},
			})
			r.markDynBound(name)
			continue
		}
		typ, err := r.typeExpr(r.prog.TypeAt(kids[0]))
		if err != nil {
			return nil, err
		}
		init, err := r.bindingInit(kids[0], kids[initIdx])
		if err != nil {
			return nil, err
		}
		specs = append(specs, &ast.ValueSpec{
			Names:  []*ast.Ident{ident(name)},
			Type:   typ,
			Values: []ast.Expr{init},
		})
	}
	return &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: specs}}, nil
}

// bindingInit lowers a binding's initializer, with one context-sensitive case an
// initializer read on its own cannot get right: an empty array literal. The
// checker types a bare [] as never[], since it has no element to infer from, so
// lowering it in isolation would instantiate value.NewArray at never. The
// annotated binding does carry the element type, so when the initializer is an
// empty array literal and the binding is an array, the element type is taken from
// the binding rather than from the literal, which is exactly the contextual type
// TypeScript itself applies here. Every other initializer lowers on its own.
func (r *Renderer) bindingInit(nameNode, initNode frontend.Node) (ast.Expr, error) {
	// A binding whose declared type is dynamic takes an object or array literal as a
	// boxed value straight from its members, skipping the static struct or slice the
	// literal would otherwise build and intern for a shape the any slot never names.
	// Boxing at the expression's own lowering would not see the slot's type; the
	// binding does, so the short-circuit lives here alongside the other contextual
	// cases.
	if r.prog.TypeAt(nameNode).Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
		if boxed, ok, err := r.boxLiteralToDynamic(initNode); err != nil {
			return nil, err
		} else if ok {
			return boxed, nil
		}
	}
	// An object literal whose shape is not statically fixed, one with a computed key
	// naming a runtime value, has no closed key set a Go struct could declare, so it
	// builds as the dynamic bag even when its binding was not written any. The binding
	// is marked dynamic so it lands in a value.Value slot (foldShortDecl infers it from
	// the boxed initializer) and every later read and write of it routes the dynamic
	// way rather than reach for a struct field the shape never had. A literal whose
	// keys are all plain or constant stays on the struct path above.
	if initNode.Kind() == frontend.NodeObjectLiteralExpression && r.objectLiteralNotFixed(initNode) {
		boxed, err := r.boxObjectLiteral(initNode)
		if err != nil {
			return nil, err
		}
		if name, ok := localName(r.prog.Text(nameNode)); ok {
			r.markDynBound(name)
		}
		return boxed, nil
	}
	// A `const s = Symbol()` binding holds the boxed symbol value.NewSymbol builds, but
	// the checker types it unique symbol, a flagless type the dynamic guards do not see
	// and typeExpr cannot name. The boxed initializer is returned straight, so := infers
	// its value.Value slot without a typed var, and the binding is marked dynamic so every
	// later use of it, above all its use as a computed key `o[s]`, routes through the value
	// model that keys the property bag by symbol identity. An annotated `let s: symbol`
	// binding carries the symbol flag already, so isSymbol routes it without the mark.
	if r.isSymbolConstructorCall(initNode) {
		boxed, err := r.lowerExpr(initNode)
		if err != nil {
			return nil, err
		}
		if name, ok := localName(r.prog.Text(nameNode)); ok {
			r.markDynBound(name)
		}
		return boxed, nil
	}
	// An object literal in a slot whose declared shape has an optional property
	// must build at that shape rather than its own all-required type, the contextual
	// typing objectLiteralContextual applies. A slot that is itself T | undefined
	// (the shape wrapped in an optional) is a later slice and hands back here.
	if initNode.Kind() == frontend.NodeObjectLiteralExpression {
		if shape, wrap, ok := r.contextualObjectShape(r.prog.TypeAt(nameNode)); ok {
			if wrap {
				return nil, &NotYetLowerable{Reason: "an object literal in a T | undefined optional slot is a later slice"}
			}
			return r.objectLiteralContextual(initNode, shape)
		}
	}
	if initNode.Kind() == frontend.NodeArrayLiteralExpression && len(r.prog.Children(initNode)) == 0 {
		if elem, ok := r.prog.ElementType(r.prog.TypeAt(nameNode)); ok {
			elemType, err := r.typeExpr(elem)
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: index(sel("value", "NewArray"), elemType)}, nil
		}
	}
	init, err := r.lowerExpr(initNode)
	if err != nil {
		return nil, err
	}
	// A binding whose declared type crosses the dynamic boundary coerces the
	// initializer to match: a dynamic value into a static binding runs the ToNumber
	// family, and a static value into an any binding boxes. A binding whose two
	// sides agree passes the initializer through unchanged, which is every static
	// binding lowered so far.
	return r.coerceToTarget(init, initNode, nameNode)
}

// collectVarDecls gathers the variable declarations inside a variable statement.
// They sit one level down under the declaration list, which bento does not name,
// so the walk descends through it.
func collectVarDecls(prog *frontend.Program, n frontend.Node, out *[]frontend.Node) {
	for _, c := range prog.Children(n) {
		if c.Kind() == frontend.NodeVariableDeclaration {
			*out = append(*out, c)
			continue
		}
		collectVarDecls(prog, c, out)
	}
}

// flattenCallableBinding expands a callable-object binding into the two steps a
// JavaScript object of this shape needs. The test262 prelude writes
// `const assert = function () { ... } as Assert` and then hangs methods off it,
// so the value is a function that also carries fields. Go has no such value, so
// the binding lowers to a pointer whose reserved Call field holds the function:
// `assert := &Assert{}` then `assert.Call = func (...) { ... }`. The pointer is
// declared first on purpose. The function body reads the object back (the assert
// call reaches for its own message helpers), and the later member assignments
// fill those helpers in, so every alias has to see one shared object. Go gets
// that from the closure capturing the already-declared variable, which matches
// the const being in scope for JavaScript. Anything that is not a single
// callable-object binding reports ok=false and takes the ordinary var path.
func (r *Renderer) flattenCallableBinding(n frontend.Node) ([]ast.Stmt, bool, error) {
	if n.Kind() != frontend.NodeVariableStatement {
		return nil, false, nil
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, n, &decls)
	if len(decls) != 1 {
		return nil, false, nil
	}
	kids := r.prog.Children(decls[0])
	if len(kids) != 2 && len(kids) != 3 {
		return nil, false, nil
	}
	nameNode := kids[0]
	if !r.isCallableObject(r.prog.TypeAt(nameNode)) {
		return nil, false, nil
	}
	name, ok := localName(r.prog.Text(nameNode))
	if !ok {
		return nil, true, &NotYetLowerable{Reason: "a callable object bound to a non-identifier name is a later slice"}
	}
	fnNode, ok := r.callableInitFunc(kids[len(kids)-1])
	if !ok {
		return nil, true, &NotYetLowerable{Reason: "a callable object initialized by something other than a function value is a later slice"}
	}
	structName, err := r.decls.internStruct(r, r.prog.TypeAt(nameNode))
	if err != nil {
		return nil, true, err
	}
	fnLit, err := r.lowerExpr(fnNode)
	if err != nil {
		return nil, true, err
	}
	// Step one declares the pointer, so the closure below and every later member
	// assignment reach the same object. A binding an earlier statement captures in a
	// closure has its pointer declared at the scope top instead, so its site here is
	// a plain assignment into the already-declared variable.
	declTok := token.DEFINE
	if r.fwdHoisted[name] {
		declTok = token.ASSIGN
	}
	declStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(name)},
		Tok: declTok,
		Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(structName)}}},
	}
	// Step two assigns the call itself into the reserved field.
	callStmt := &ast.AssignStmt{
		Lhs: []ast.Expr{&ast.SelectorExpr{X: ident(name), Sel: ident("Call")}},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{fnLit},
	}
	return []ast.Stmt{declStmt, callStmt}, true, nil
}

// callableInitFunc peels the type cast off a callable-object initializer and
// reports the function value underneath. The prelude spells the initializer as a
// function expression widened to the object type through an `as` cast or an
// angle-bracket assertion, so the cast is stripped (its inner value sits at
// child 0 for `as` and child 1 for the assertion) and the result is returned
// only when it is a function value. A cast over anything else, or an initializer
// with no cast at all, reports ok=false so the binding hands back.
func (r *Renderer) callableInitFunc(init frontend.Node) (frontend.Node, bool) {
	inner := init
	switch init.Kind() {
	case frontend.NodeAsExpression:
		kids := r.prog.Children(init)
		if len(kids) == 0 {
			return init, false
		}
		inner = kids[0]
	case frontend.NodeTypeAssertion:
		kids := r.prog.Children(init)
		if len(kids) < 2 {
			return init, false
		}
		inner = kids[1]
	}
	switch inner.Kind() {
	case frontend.NodeFunctionExpression, frontend.NodeArrowFunction:
		return inner, true
	}
	return init, false
}

// lowerExprStatement lowers an expression used as a statement. The covered
// forms all update a local: a plain or compound assignment, which the checker
// exposes as a binary expression, and a prefix or postfix ++/--; a call or
// other expression statement is a later slice.
func (r *Renderer) lowerExprStatement(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "expression statement did not expose a single expression"}
	}
	return r.lowerUpdate(kids[0])
}

// flattenCommaStatement lowers a statement-position comma expression, a = 1, b = 2
// or f(), g(), to the Go statements its operands spell, evaluated left to right
// with each value discarded. That is exactly the comma operator's meaning when its
// result is thrown away, which is every use of it in statement position. It
// returns ok=false when the statement is not a comma expression, so a plain
// expression statement falls through to the single-statement path below. An
// operand the update lowering does not cover (a bare value with no effect, say)
// makes the whole statement hand back rather than emit a partial flatten.
func (r *Renderer) flattenCommaStatement(n frontend.Node) ([]ast.Stmt, bool, error) {
	if n.Kind() != frontend.NodeExpressionStatement {
		return nil, false, nil
	}
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, false, nil
	}
	ops, ok := commaOperands(r.prog, kids[0])
	if !ok {
		return nil, false, nil
	}
	stmts := make([]ast.Stmt, 0, len(ops))
	for _, op := range ops {
		s, err := r.lowerUpdate(op)
		if err != nil {
			return nil, false, err
		}
		stmts = append(stmts, s)
	}
	return stmts, true, nil
}

// flattenChainedAssignStatement lowers a statement-position chained assignment,
// a = b = 5, to the Go statements it means: the innermost assignment runs, then
// each outer target takes the value the one inside it just held. So a = b = 5
// becomes b = 5; a = b, which evaluates the right side once and settles every
// target, the same as the chain does. It returns ok=false for a plain single
// assignment, which the normal statement path already lowers, and hands back when
// a link in the chain targets something other than a local identifier.
func (r *Renderer) flattenChainedAssignStatement(n frontend.Node) ([]ast.Stmt, bool, error) {
	if n.Kind() != frontend.NodeExpressionStatement {
		return nil, false, nil
	}
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, false, nil
	}
	outer, final, ok := r.chainedAssign(kids[0])
	if !ok || len(outer) == 0 {
		return nil, false, nil
	}
	// The innermost assignment lowers on its own, b = 5. Its target then feeds the
	// next target out, and so on, so the value walks from the inside to the outside
	// exactly as the chain assigns it.
	first, err := r.lowerUpdate(final)
	if err != nil {
		return nil, false, err
	}
	stmts := []ast.Stmt{first}
	source := r.prog.Children(final)[0]
	for i := len(outer) - 1; i >= 0; i-- {
		copyStmt, err := r.assignCopy(outer[i], source)
		if err != nil {
			return nil, false, err
		}
		stmts = append(stmts, copyStmt)
		source = outer[i]
	}
	return stmts, true, nil
}

// chainedAssign walks a chain of plain assignments a = b = ... = value, returning
// the outer targets in source order and the innermost assignment node that carries
// the value. Assignment is right associative, so a = b = 5 nests as a = (b = 5);
// the walk descends the right side while it stays a plain "=" assignment. A single
// assignment has no outer targets and reports ok=false so the caller leaves it to
// the ordinary path. A compound link like a = (b += 5) stops the walk, since its
// value is not a plain copy, and that link lowers (and hands back) on its own.
func (r *Renderer) chainedAssign(n frontend.Node) ([]frontend.Node, frontend.Node, bool) {
	if n.Kind() != frontend.NodeBinaryExpression {
		return nil, n, false
	}
	kids := r.prog.Children(n)
	if len(kids) != 3 || r.prog.Text(kids[1]) != "=" {
		return nil, n, false
	}
	if inner, final, ok := r.chainedAssign(kids[2]); ok {
		return append([]frontend.Node{kids[0]}, inner...), final, true
	}
	return nil, n, true
}

// flattenArrayDestructure lowers `const [a, b] = src` to one `:=` binding per
// element, a := src.AtI(0) and b := src.AtI(1), the same indexed read a written-out
// element access lowers to. It owns the statement once it sees an array binding
// pattern, so every shape it cannot yet lower returns an error and hands the unit
// back rather than falling through to the plain binding path with a misleading
// diagnostic. The bounded cases: the source must be a plain variable, since any
// other source would be re-evaluated once per element without a temporary to hold
// it; the pattern is flat names only, so a hole, a default, a rest, or a nested
// pattern hands back; and each name's type must match the array's element type, so
// a heterogeneous tuple, whose element read does not produce the narrowed type the
// name is declared with, hands back. An object pattern is a separate slice and is
// left for the plain path, which recognizes it is not a Go identifier and hands back.
func (r *Renderer) flattenArrayDestructure(n frontend.Node) ([]ast.Stmt, bool, error) {
	if n.Kind() != frontend.NodeVariableStatement {
		return nil, false, nil
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, n, &decls)
	if len(decls) != 1 {
		return nil, false, nil
	}
	kids := r.prog.Children(decls[0])
	if len(kids) != 2 {
		return nil, false, nil
	}
	patNode, initNode := kids[0], kids[1]
	if patNode.Kind() != frontend.NodeUnknown {
		return nil, false, nil
	}
	if !strings.HasPrefix(strings.TrimSpace(r.prog.Text(patNode)), "[") {
		return nil, false, nil
	}
	// The pattern is an array binding, so from here the statement is ours: an early
	// return reports ok=true with an error, not a fall-through.
	// A dynamic source has no static array shape, so the pattern reads each position
	// through the boxed value's index protocol rather than a typed AtI, the declaration
	// sibling of the untyped parameter's dynamic slot.
	if r.isDynamic(initNode) {
		return r.dynamicSourceDestructure(patNode, initNode)
	}
	initType := r.prog.TypeAt(initNode)
	elemT, ok := r.prog.ElementType(initType)
	// A source that is not an array or tuple but is a user iterable destructures
	// through the iterator protocol: it is drained into a value.Array once, then each
	// target reads off that array by index the same way an array source does, so the
	// bounds-checked AtI read and the whole binding loop below are shared. The element
	// type is the iterable's yield type.
	iterShape, iterOK := iteratorShape{}, false
	if !ok {
		if iterShape, iterOK = r.symbolIteratorShape(initType); !iterOK {
			return nil, true, &NotYetLowerable{Reason: "array destructuring on a non-array or tuple source is a later slice"}
		}
		elemT = iterShape.elem
	}
	elemGo, err := r.typeExpr(elemT)
	if err != nil {
		return nil, true, err
	}
	elems := r.prog.Children(patNode)
	if len(elems) == 0 {
		return nil, true, &NotYetLowerable{Reason: "an empty array destructuring pattern binds nothing"}
	}
	// A trailing rest gathers the elements past the fixed slots into a fresh array,
	// so it is split off and the fixed elements are classified as usual; the rest is
	// bound after them from the same receiver.
	fixedElems, restNode, hasRest, err := r.splitArrayRest(elems)
	if err != nil {
		return nil, true, err
	}
	// The pattern is validated before the source is lowered, so an unsupported shape
	// hands back before a temporary is minted for the source.
	infos := make([]arrayDefaultElem, len(fixedElems))
	for i, el := range fixedElems {
		info, err := r.classifyArrayElem(el)
		if err != nil {
			return nil, true, err
		}
		// A nested pattern binds against the slot the outer element selects, so its
		// inner names are validated when the sub-pattern is bound, not here.
		if info.nested != nil {
			infos[i] = info
			continue
		}
		name, ok := localName(r.prog.Text(info.nameNode))
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "destructured name is not a Go identifier"}
		}
		nameGo, err := r.typeExpr(r.prog.TypeAt(info.nameNode))
		if err != nil {
			return nil, true, err
		}
		// A defaulted element fills from its default when the slot is undefined, so
		// its binding type is the element type the source read yields, the same match
		// a plain element needs; an optional-element source, whose read is an Opt the
		// default would have to peel, is a later slice.
		if same, err := sameGoType(nameGo, elemGo); err != nil {
			return nil, true, err
		} else if !same {
			if info.hasDefault {
				return nil, true, &NotYetLowerable{Reason: "an array destructuring default over an optional-element source is a later slice"}
			}
			return nil, true, &NotYetLowerable{Reason: "array destructuring where an element's type differs from the array element type is a later slice"}
		}
		info.name = name
		infos[i] = info
	}
	var prefix []ast.Stmt
	var recv func() (ast.Expr, error)
	if iterOK {
		// The iterable is drained into a value.Array bound once, so each index read
		// selects off the held array and the iterator is walked a single time.
		src, err := r.lowerExpr(initNode)
		if err != nil {
			return nil, true, err
		}
		r.requireImport(valuePkg)
		drained := &ast.CallExpr{Fun: sel("value", "ArrayFrom"), Args: []ast.Expr{r.iterableToSliceExpr(src, elemGo, iterShape)}}
		tmp := r.freshTemp()
		prefix = []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{drained}}}
		recv = func() (ast.Expr, error) { return ident(tmp), nil }
	} else {
		prefix, recv, err = r.destructureSource(initNode)
		if err != nil {
			return nil, true, err
		}
	}
	stmts := prefix
	for i, info := range infos {
		rc, err := recv()
		if err != nil {
			return nil, true, err
		}
		if info.nested != nil {
			tmp := r.freshTemp()
			read := &ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: rc, Sel: ident("AtI")},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}},
			}
			stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{read}})
			inner, err := r.bindSubPattern(info.nested, ident(tmp), elemT, token.DEFINE)
			if err != nil {
				return nil, true, err
			}
			stmts = append(stmts, inner...)
			continue
		}
		if info.hasDefault {
			nameGo, err := r.typeExpr(r.prog.TypeAt(info.nameNode))
			if err != nil {
				return nil, true, err
			}
			def, err := r.lowerExpr(info.defNode)
			if err != nil {
				return nil, true, err
			}
			def, err = r.coerceToType(def, info.defNode, r.prog.TypeAt(info.nameNode))
			if err != nil {
				return nil, true, err
			}
			stmts = append(stmts, r.defaultFillStmts(info.name, nameGo, arrayOptRead(rc, i), def)...)
			continue
		}
		read := &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: rc, Sel: ident("AtI")},
			Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}},
		}
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(info.name)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{read},
		})
	}
	if hasRest {
		rc, err := recv()
		if err != nil {
			return nil, true, err
		}
		bind, err := r.arrayRestBinding(restNode, elemT, rc, len(infos), token.DEFINE)
		if err != nil {
			return nil, true, err
		}
		stmts = append(stmts, bind)
	}
	return stmts, true, nil
}

// destructureSource lowers the source of a destructuring binding into a receiver the
// element reads select through. A plain variable is re-lowered on each read, since a
// bare identifier reference has no cost and no effect to repeat, so each read carries
// a fresh copy of the receiver expression; any other source is evaluated once into a
// generated temporary bound at the front of the expansion, so a call or a member read
// runs a single time and each read selects off the held value. The returned prefix is
// the temporary's binding, empty for a variable source, and the receiver thunk yields
// the receiver expression for one read.
func (r *Renderer) destructureSource(initNode frontend.Node) ([]ast.Stmt, func() (ast.Expr, error), error) {
	if initNode.Kind() == frontend.NodeIdentifier {
		return nil, func() (ast.Expr, error) { return r.lowerExpr(initNode) }, nil
	}
	lowered, err := r.lowerExpr(initNode)
	if err != nil {
		return nil, nil, err
	}
	tmp := r.freshTemp()
	prefix := []ast.Stmt{&ast.AssignStmt{
		Lhs: []ast.Expr{ident(tmp)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{lowered},
	}}
	return prefix, func() (ast.Expr, error) { return ident(tmp), nil }, nil
}

// dynamicSourceDestructure lowers a declaration whose source is a dynamic value, `const
// {a} = x` or `const [a] = x` where x is any: the source has no static shape, so each name
// reads through the boxed value's member and index protocol the untyped parameter's slot
// uses. The source is lowered once into a temporary when it is not a plain variable, so it
// is evaluated a single time, then the pattern binds against it.
func (r *Renderer) dynamicSourceDestructure(patNode, initNode frontend.Node) ([]ast.Stmt, bool, error) {
	prefix, recv, err := r.destructureSource(initNode)
	if err != nil {
		return nil, true, err
	}
	recvExpr, err := recv()
	if err != nil {
		return nil, true, err
	}
	stmts, err := r.bindDynamicPattern(patNode, recvExpr, token.DEFINE)
	if err != nil {
		return nil, true, err
	}
	// An object rest this declaration binds is a boxed value the checker did not type any,
	// so a read of it later in the scope must dispatch dynamically. Statements lower in
	// order, so marking it here reaches every read below. The map is lazily created since
	// a body with no destructured parameter never built one.
	if r.dynBoundLocals == nil {
		r.dynBoundLocals = map[string]bool{}
	}
	r.collectDynRestNames(patNode, r.dynBoundLocals)
	return append(prefix, stmts...), true, nil
}

// isSymbolConstructorCall reports whether n is a call to the ambient Symbol
// constructor, `Symbol()` or `Symbol(desc)`, whose result is a boxed symbol value.
// A user binding named Symbol shadows the global and is not this call, so the
// ambient-global check keeps the two apart.
func (r *Renderer) isSymbolConstructorCall(n frontend.Node) bool {
	if n.Kind() != frontend.NodeCallExpression {
		return false
	}
	kids := r.prog.Children(n)
	return len(kids) >= 1 && r.prog.Text(kids[0]) == "Symbol" && r.isAmbientGlobal(kids[0])
}

// markDynBound records that a local's Go slot holds a boxed value.Value, so every
// later read and write of it routes through the dynamic value model rather than the
// static shape the checker gave the name. Statements lower in source order, so
// marking a binding as it lowers reaches every use below it. The map is created
// lazily since a body with no dynamic binding never needs one.
func (r *Renderer) markDynBound(name string) {
	if r.dynBoundLocals == nil {
		r.dynBoundLocals = map[string]bool{}
	}
	r.dynBoundLocals[name] = true
}

// flattenObjectDestructure lowers `const {x, y} = src` to one `:=` binding per
// property, x := src.X and y := src.Y, the same struct-field selector a written-out
// property access lowers to. It is the object sibling of flattenArrayDestructure and
// owns the statement once it sees an object binding pattern, so every shape it cannot
// yet lower returns an error and hands the unit back. The bounded cases: the source
// must be a plain variable, so the read repeats without re-evaluating it; the source
// must be a fixed-shape object, since a struct field is what the selector reads, so a
// map-like record or an array hands back; and the pattern is shorthand names only, so
// a rename (`{x: a}`), a default, a rest, or a nested pattern hands back, each a later
// slice. A shorthand name binds the property of the same name, so its type is the
// property's type and the selector read needs no coercion.
func (r *Renderer) flattenObjectDestructure(n frontend.Node) ([]ast.Stmt, bool, error) {
	if n.Kind() != frontend.NodeVariableStatement {
		return nil, false, nil
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, n, &decls)
	if len(decls) != 1 {
		return nil, false, nil
	}
	kids := r.prog.Children(decls[0])
	if len(kids) != 2 {
		return nil, false, nil
	}
	patNode, initNode := kids[0], kids[1]
	if patNode.Kind() != frontend.NodeUnknown {
		return nil, false, nil
	}
	if !strings.HasPrefix(strings.TrimSpace(r.prog.Text(patNode)), "{") {
		return nil, false, nil
	}
	// The pattern is an object binding, so from here the statement is ours: an early
	// return reports ok=true with an error, not a fall-through.
	// A dynamic source has no static shape, so the pattern reads each property through the
	// boxed value's member protocol rather than a struct-field selector, the declaration
	// sibling of the untyped parameter's dynamic slot.
	if r.isDynamic(initNode) {
		return r.dynamicSourceDestructure(patNode, initNode)
	}
	objType := r.prog.TypeAt(initNode)
	if objType.Flags&frontend.TypeObject == 0 || r.isTypedArray(initNode) {
		return nil, true, &NotYetLowerable{Reason: "object destructuring on a non-object source is a later slice"}
	}
	if _, isArray := r.prog.ElementType(objType); isArray {
		return nil, true, &NotYetLowerable{Reason: "object destructuring on an array source is a later slice"}
	}
	if _, err := r.decls.internStruct(r, objType); err != nil {
		return nil, true, err
	}
	elems := r.prog.Children(patNode)
	if len(elems) == 0 {
		return nil, true, &NotYetLowerable{Reason: "an empty object destructuring pattern binds nothing"}
	}
	// The pattern is validated before the source is lowered, so an unsupported shape
	// hands back before a temporary is minted for the source.
	type binding struct {
		info       objectDefaultElem
		name       string
		field      string
		optional   bool
		nested     frontend.Node
		nestedType frontend.Type
		rest       bool
		restStruct string
		restType   frontend.Type
	}
	optionalField := map[string]bool{}
	propType := map[string]frontend.Type{}
	for _, pr := range r.prog.Properties(objType) {
		optionalField[pr.Name] = pr.Optional
		propType[pr.Name] = pr.Type
	}
	fields := make([]binding, len(elems))
	for i, el := range elems {
		// A rest property gathers the own properties the pattern did not name into a new
		// object. On a fixed-shape source the rest's checker type is that object minus the
		// named properties, so it interns as its own struct and the gather is a struct
		// literal copying each remaining field off the receiver, built in the emit loop.
		if restIdent, ok := r.objectRestElem(el); ok {
			name, ok := localName(r.prog.Text(restIdent))
			if !ok {
				return nil, true, &NotYetLowerable{Reason: "an object destructuring rest target is not a Go identifier"}
			}
			restType := r.prog.TypeAt(restIdent)
			if restType.Flags&frontend.TypeObject == 0 {
				return nil, true, &NotYetLowerable{Reason: "an object destructuring rest whose type is not a fixed-shape object is a later slice"}
			}
			if _, isArray := r.prog.ElementType(restType); isArray {
				return nil, true, &NotYetLowerable{Reason: "an object destructuring rest typed as an array is a later slice"}
			}
			structName, err := r.decls.internStruct(r, restType)
			if err != nil {
				return nil, true, err
			}
			fields[i] = binding{rest: true, name: name, restStruct: structName, restType: restType}
			continue
		}
		// A nested pattern renames a source property into an inner pattern that binds
		// against the value the property holds; its inner names are validated when the
		// sub-pattern is bound, so it is routed straight through.
		if source, sub, ok := r.objectNestedElem(el); ok {
			prop := r.prog.Text(source)
			srcName, nok := localName(prop)
			pt, known := propType[prop]
			if !nok || !known {
				return nil, true, &NotYetLowerable{Reason: "a nested object pattern over an unknown property is a later slice"}
			}
			field, fok := exportedField(srcName)
			if !fok {
				return nil, true, &NotYetLowerable{Reason: "destructured property is not a Go field name"}
			}
			fields[i] = binding{field: field, nested: sub, nestedType: pt}
			continue
		}
		info, err := r.classifyObjectElem(el)
		if err != nil {
			return nil, true, err
		}
		prop := r.elemSourceProp(info)
		srcName, ok := localName(prop)
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "destructured name is not a Go identifier"}
		}
		field, ok := exportedField(srcName)
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "destructured property is not a Go field name"}
		}
		name, ok := localName(r.prog.Text(info.bindNode))
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "destructured target is not a Go identifier"}
		}
		fields[i] = binding{info: info, name: name, field: field, optional: optionalField[prop]}
	}
	prefix, recv, err := r.destructureSource(initNode)
	if err != nil {
		return nil, true, err
	}
	stmts := prefix
	for _, b := range fields {
		rc, err := recv()
		if err != nil {
			return nil, true, err
		}
		if b.rest {
			elts := make([]ast.Expr, 0, len(r.prog.Properties(b.restType)))
			for _, pr := range r.prog.Properties(b.restType) {
				field, ok := exportedField(pr.Name)
				if !ok {
					return nil, true, &NotYetLowerable{Reason: "an object destructuring rest property is not a Go field name"}
				}
				elts = append(elts, &ast.KeyValueExpr{Key: ident(field), Value: &ast.SelectorExpr{X: rc, Sel: ident(field)}})
			}
			stmts = append(stmts, &ast.AssignStmt{
				Lhs: []ast.Expr{ident(b.name)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(b.restStruct), Elts: elts}}},
			})
			continue
		}
		if b.nested != nil {
			tmp := r.freshTemp()
			stmts = append(stmts, &ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.SelectorExpr{X: rc, Sel: ident(b.field)}}})
			inner, err := r.bindSubPattern(b.nested, ident(tmp), b.nestedType, token.DEFINE)
			if err != nil {
				return nil, true, err
			}
			stmts = append(stmts, inner...)
			continue
		}
		read := &ast.SelectorExpr{X: rc, Sel: ident(b.field)}
		// A default over an optional field fills when the property is undefined; the
		// field read is an Opt the fill peels. A default over a required field can
		// never fire, since the property is always present, so it binds the read
		// directly and the default is dead.
		if b.info.hasDefault && b.optional {
			nameGo, err := r.typeExpr(r.prog.TypeAt(b.info.bindNode))
			if err != nil {
				return nil, true, err
			}
			def, err := r.lowerExpr(b.info.defNode)
			if err != nil {
				return nil, true, err
			}
			def, err = r.coerceToType(def, b.info.defNode, r.prog.TypeAt(b.info.bindNode))
			if err != nil {
				return nil, true, err
			}
			stmts = append(stmts, r.defaultFillStmts(b.name, nameGo, read, def)...)
			continue
		}
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(b.name)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{read},
		})
		// A named property bound only to be excluded from a rest is a common idiom,
		// `{ a, ...rest }`, so a binding nothing reads takes the blank the ordinary
		// variable path gives, keeping the Go declaration used.
		if r.bindingUnused(b.info.bindNode) {
			stmts = append(stmts, &ast.AssignStmt{
				Lhs: []ast.Expr{ident("_")},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{ident(b.name)},
			})
		}
	}
	return stmts, true, nil
}

// assignCopy builds target = source for one link of a chained assignment, where
// source is the target one step inside it. The source lowers as a read and coerces
// to the target's type, so a chain that crosses the dynamic boundary boxes or
// unboxes at each step the same way a written-out assignment would.
func (r *Renderer) assignCopy(target, source frontend.Node) (ast.Stmt, error) {
	if target.Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "chained assignment to a non-identifier target is a later slice"}
	}
	name, ok := localName(r.prog.Text(target))
	if !ok {
		return nil, &NotYetLowerable{Reason: "chained assignment target is not a Go identifier"}
	}
	src, err := r.lowerExpr(source)
	if err != nil {
		return nil, err
	}
	src, err = r.coerceToTarget(src, source, target)
	if err != nil {
		return nil, err
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{src}}, nil
}

// commaOperands returns the operands of a comma expression in source order, or
// ok=false when n is not a comma expression. The comma operator is left
// associative, so a, b, c parses as (a, b), c; the walk flattens that left spine
// so the operands come back in the order they were written.
func commaOperands(prog *frontend.Program, n frontend.Node) ([]frontend.Node, bool) {
	if n.Kind() != frontend.NodeBinaryExpression {
		return nil, false
	}
	kids := prog.Children(n)
	if len(kids) != 3 || prog.Text(kids[1]) != "," {
		return nil, false
	}
	var out []frontend.Node
	if left, ok := commaOperands(prog, kids[0]); ok {
		out = append(out, left...)
	} else {
		out = append(out, kids[0])
	}
	out = append(out, kids[2])
	return out, true
}

// lowerUpdate lowers a statement-position expression that mutates a local: a
// plain assignment (=), a compound assignment (+=, -=, and the rest), or a
// prefix/postfix increment or decrement. It is shared by an expression
// statement and a for-loop's post clause, both of which discard the value, so
// the prefix and postfix forms of ++/-- lower the same way.
func (r *Renderer) lowerUpdate(n frontend.Node) (ast.Stmt, error) {
	switch n.Kind() {
	case frontend.NodeBinaryExpression:
		if stmt, ok, err := r.arrayDestructureAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.argumentsElementAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.bytesElementAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.arrayElementAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.dynamicElementAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.bigIntInPlaceAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.classFieldAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.objectFieldAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.logicalAssign(n); ok || err != nil {
			return stmt, err
		}
		// A binary expression in statement position that is not an assignment is a
		// bare value written for effect, a === b or a + b with the result thrown
		// away. None of the assignment shapes above matched it, so it takes the
		// same discard a value-shaped statement does rather than lowerAssign, which
		// only knows the assignment operators.
		if parts := r.prog.Children(n); len(parts) == 3 {
			opText := r.prog.Text(parts[1])
			if _, compound := compoundBaseOp(opText); opText != "=" && !compound {
				return r.discardExprStatement(n)
			}
		}
		assign, err := r.lowerAssign(n)
		if err != nil {
			return nil, err
		}
		// A compound step of exactly one, x += 1 or x -= 1, is the increment a person
		// writes x++ or x--. Both discard the value here (a statement or a for-loop's
		// post clause), so the IncDecStmt is the same operation spelled the shorter way.
		if inc, ok := incDecFromStep(assign); ok {
			return inc, nil
		}
		return assign, nil
	case frontend.NodeParenthesizedExpression:
		// An object destructuring assignment is parenthesized in statement position,
		// ({ x, y } = o), since a bare { on the left would open a block. The paren is
		// the only thing between the assignment and lowerUpdate, so unwrap it for the
		// one shape we lower and hand back the rest.
		if stmt, ok, err := r.objectDestructureAssign(n); ok || err != nil {
			return stmt, err
		}
		// An array destructuring assignment can also be parenthesized in statement
		// position, ([a, b] = xs), so unwrap the paren and route the inner assignment
		// through the array path the bare form takes.
		if inner := r.prog.Children(n); len(inner) == 1 && inner[0].Kind() == frontend.NodeBinaryExpression {
			if stmt, ok, err := r.arrayDestructureAssign(inner[0]); ok || err != nil {
				return stmt, err
			}
		}
		return nil, &NotYetLowerable{Reason: "a parenthesized expression statement that is not an object destructuring assignment is a later slice"}
	case frontend.NodePrefixUnaryExpression, frontend.NodePostfixUnaryExpression:
		return r.lowerIncDec(n)
	case frontend.NodeCallExpression:
		// A call used as a statement discards its result, which Go allows for any
		// function call, so it lowers to the call wrapped in an expression
		// statement. This is how a mutating method like an array push is invoked
		// for its effect rather than its value.
		call, err := r.lowerExpr(n)
		if err != nil {
			return nil, err
		}
		return &ast.ExprStmt{X: call}, nil
	default:
		return r.discardExprStatement(n)
	}
}

// discardExprStatement lowers a bare expression statement, one that evaluates its
// operand for any side effect and throws the value away. Go does not let a value
// stand alone as a statement the way a call does, so the discard is spelled
// _ = expr, which evaluates the operand and drops the result. The call form is
// lowered on its own branch, so this covers the value-shaped statements a program
// writes for effect or leaves inert: a member read that runs a getter, a
// comparison written for its effect, a discarded conditional, a lone identifier or
// literal. An operand lowerExpr cannot lower yet hands back through its own error,
// so only a faithfully lowered value reaches the discard.
func (r *Renderer) discardExprStatement(n frontend.Node) (ast.Stmt, error) {
	expr, err := r.lowerExpr(n)
	if err != nil {
		return nil, err
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{expr}}, nil
}

// arrayDestructureAssign lowers an array destructuring assignment `[a, b] = rhs` to a
// single Go parallel assignment, `a, b = rhs0, rhs1`. Go evaluates every right-hand
// side before it assigns any target, which is exactly the destructuring assignment's
// order and is what makes the swap idiom `[a, b] = [b, a]` fall out as `a, b = b, a`.
// It reports ok=false when the statement is not an array destructuring assignment, so
// an ordinary assignment falls through to the paths below. Once it sees the array
// pattern on the left it owns the statement, so a shape it cannot lower hands back:
// the targets must be plain identifiers, an element-access or nested-pattern target
// is a later slice; the right side is either a plain array variable, read element by
// element through AtI with each element type matching its target, or an array literal
// of the same arity whose elements lower and coerce to their targets; any other right
// side needs a temporary and hands back.
func (r *Renderer) arrayDestructureAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" {
		return nil, false, nil
	}
	lhs, rhs := parts[0], parts[2]
	if lhs.Kind() != frontend.NodeArrayLiteralExpression {
		return nil, false, nil
	}
	// The left side is an array pattern, so from here the statement is ours.
	targets := r.prog.Children(lhs)
	if len(targets) == 0 {
		return nil, true, &NotYetLowerable{Reason: "an empty array assignment pattern binds nothing"}
	}
	// A trailing rest gathers the tail into the rest target, so it is split off and
	// the fixed targets take their per-index reads before the rest is assigned.
	fixedTargets, restNode, hasRest, err := r.splitArrayRest(targets)
	if err != nil {
		return nil, true, err
	}
	// A nested pattern in any target position, `[[a, b], c] = m`, turns the flat
	// parallel assignment into a recursive tree of reads: the whole pattern routes
	// through the assignment recursion, which holds each nested slot in a temporary and
	// stores each leaf into its existing target. It needs the source to be a plain array
	// variable so the repeated reads do not re-evaluate it.
	for _, tgt := range fixedTargets {
		if !r.assignPatternNode(tgt) {
			continue
		}
		if rhs.Kind() != frontend.NodeIdentifier {
			return nil, true, &NotYetLowerable{Reason: "a nested array assignment from anything but a plain array variable needs a temporary, a later slice"}
		}
		recv, err := r.lowerExpr(rhs)
		if err != nil {
			return nil, true, err
		}
		stmts, err := r.bindSubArrayAssign(lhs, recv, r.prog.TypeAt(rhs))
		if err != nil {
			return nil, true, err
		}
		return &ast.BlockStmt{List: stmts}, true, nil
	}
	elems := make([]arrayAssignElem, len(fixedTargets))
	anyDefault := false
	for i, tgt := range fixedTargets {
		el, err := r.classifyArrayAssignElem(tgt)
		if err != nil {
			return nil, true, err
		}
		elems[i] = el
		anyDefault = anyDefault || el.hasDefault
	}
	// A default or a rest turns the flat parallel assignment into a per-target fill,
	// so a pattern with either takes its own path; the plain pattern keeps the single
	// parallel assignment that makes the swap idiom fall out.
	if anyDefault || hasRest {
		for _, el := range elems {
			if el.memberNode != nil {
				return nil, true, &NotYetLowerable{Reason: "an array assignment that combines a member target with a default or rest is a later slice"}
			}
		}
		return r.arrayDestructureAssignFill(elems, restNode, hasRest, rhs)
	}
	names := make([]ast.Expr, 0, len(fixedTargets))
	for _, el := range elems {
		if el.memberNode != nil {
			lhs, err := r.memberAssignTarget(el.memberNode)
			if err != nil {
				return nil, true, err
			}
			names = append(names, lhs)
			continue
		}
		name, ok := localName(r.prog.Text(el.nameNode))
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "array assignment target is not a Go identifier"}
		}
		names = append(names, ident(name))
	}
	values, err := r.arrayDestructureValues(fixedTargets, rhs)
	if err != nil {
		return nil, true, err
	}
	return &ast.AssignStmt{Lhs: names, Tok: token.ASSIGN, Rhs: values}, true, nil
}

// arrayDestructureAssignFill lowers an array destructuring assignment that carries a
// default or a rest, `[a = d, b, ...rest] = arr`, to a block of per-target fills: a
// plain target takes its element read, a defaulted target fills from its default when
// the slot is undefined, and a trailing rest gathers the tail past the fixed targets.
// A default or a rest breaks the flat parallel assignment, so this reads element by
// element instead, which needs the source to be a plain array variable; a literal or
// other source hands back as a later slice. Each fixed target's type must match the
// array element type, the same guard the default-free path applies.
func (r *Renderer) arrayDestructureAssignFill(elems []arrayAssignElem, restNode frontend.Node, hasRest bool, rhs frontend.Node) (ast.Stmt, bool, error) {
	if rhs.Kind() != frontend.NodeIdentifier {
		return nil, true, &NotYetLowerable{Reason: "an array assignment with a default or rest from anything but a plain array variable needs a temporary, a later slice"}
	}
	elemT, ok := r.prog.ElementType(r.prog.TypeAt(rhs))
	if !ok {
		return nil, true, &NotYetLowerable{Reason: "array destructuring assignment from a non-array or tuple source is a later slice"}
	}
	elemGo, err := r.typeExpr(elemT)
	if err != nil {
		return nil, true, err
	}
	out := make([]ast.Stmt, 0, len(elems))
	for i, el := range elems {
		name, ok := localName(r.prog.Text(el.nameNode))
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "array assignment target is not a Go identifier"}
		}
		tgtGo, err := r.typeExpr(r.prog.TypeAt(el.nameNode))
		if err != nil {
			return nil, true, err
		}
		if same, err := sameGoType(tgtGo, elemGo); err != nil {
			return nil, true, err
		} else if !same {
			return nil, true, &NotYetLowerable{Reason: "array destructuring assignment where a target's type differs from the array element type is a later slice"}
		}
		recv, err := r.lowerExpr(rhs)
		if err != nil {
			return nil, true, err
		}
		if el.hasDefault {
			def, err := r.lowerExpr(el.defNode)
			if err != nil {
				return nil, true, err
			}
			def, err = r.coerceToType(def, el.defNode, r.prog.TypeAt(el.nameNode))
			if err != nil {
				return nil, true, err
			}
			out = append(out, r.defaultFillAssign(ident(name), arrayOptRead(recv, i), def))
			continue
		}
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(name)},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{&ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: recv, Sel: ident("AtI")},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}},
			}},
		})
	}
	if hasRest {
		recv, err := r.lowerExpr(rhs)
		if err != nil {
			return nil, true, err
		}
		bind, err := r.arrayRestBinding(restNode, elemT, recv, len(elems), token.ASSIGN)
		if err != nil {
			return nil, true, err
		}
		out = append(out, bind)
	}
	return &ast.BlockStmt{List: out}, true, nil
}

// arrayDestructureValues lowers the right side of an array destructuring assignment
// into one Go expression per target, ready for the parallel assignment. A plain array
// variable reads each target's element through AtI, guarded so the element type
// matches the target and no coercion is needed; an array literal of the same arity
// lowers each element and coerces it to its target the way a written-out assignment
// would, which covers the swap idiom.
func (r *Renderer) arrayDestructureValues(targets []frontend.Node, rhs frontend.Node) ([]ast.Expr, error) {
	switch rhs.Kind() {
	case frontend.NodeIdentifier:
		elemT, ok := r.prog.ElementType(r.prog.TypeAt(rhs))
		if !ok {
			return nil, &NotYetLowerable{Reason: "array destructuring assignment from a non-array or tuple source is a later slice"}
		}
		elemGo, err := r.typeExpr(elemT)
		if err != nil {
			return nil, err
		}
		values := make([]ast.Expr, 0, len(targets))
		for i, tgt := range targets {
			tgtGo, err := r.typeExpr(r.prog.TypeAt(tgt))
			if err != nil {
				return nil, err
			}
			if same, err := sameGoType(tgtGo, elemGo); err != nil {
				return nil, err
			} else if !same {
				return nil, &NotYetLowerable{Reason: "array destructuring assignment where a target's type differs from the array element type is a later slice"}
			}
			recv, err := r.lowerExpr(rhs)
			if err != nil {
				return nil, err
			}
			values = append(values, &ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: recv, Sel: ident("AtI")},
				Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}},
			})
		}
		return values, nil
	case frontend.NodeArrayLiteralExpression:
		elems := r.prog.Children(rhs)
		if len(elems) != len(targets) {
			return nil, &NotYetLowerable{Reason: "array destructuring assignment with a different number of literal elements than targets is a later slice"}
		}
		values := make([]ast.Expr, 0, len(elems))
		for i, el := range elems {
			if el.Kind() == frontend.NodeSpreadElement {
				return nil, &NotYetLowerable{Reason: "a spread element in an array assignment source is a later slice"}
			}
			v, err := r.lowerExpr(el)
			if err != nil {
				return nil, err
			}
			v, err = r.coerceToTarget(v, el, targets[i])
			if err != nil {
				return nil, err
			}
			values = append(values, v)
		}
		return values, nil
	default:
		return nil, &NotYetLowerable{Reason: "array destructuring assignment from anything but a plain variable or an array literal needs a temporary, a later slice"}
	}
}

// objectDestructureAssign lowers an object destructuring assignment `({ x, y } = o)`
// to a single Go parallel assignment, `x, y = o.X, o.Y`, one struct-field selector per
// target. It is the assignment sibling of flattenObjectDestructure, which binds fresh
// names with `:=`; here the targets are already-declared locals, so it assigns with
// `=`. The statement is parenthesized in source since a leading `{` would open a block,
// so the node handed in is the parenthesized expression. It reports ok=false when that
// wraps anything but an object-pattern assignment, so an ordinary parenthesized
// statement falls through. Once it sees the object pattern on the left it owns the
// statement: the source must be a plain variable of a fixed-shape object read field by
// field, and every target must be a shorthand identifier whose field is the property of
// the same name, so a rename (`{x: a}`), a default, a rest, a nested pattern, a member
// target, or a non-variable source hands back, each a later slice.
func (r *Renderer) objectDestructureAssign(paren frontend.Node) (ast.Stmt, bool, error) {
	inner := r.prog.Children(paren)
	if len(inner) != 1 || inner[0].Kind() != frontend.NodeBinaryExpression {
		return nil, false, nil
	}
	parts := r.prog.Children(inner[0])
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" {
		return nil, false, nil
	}
	lhs, rhs := parts[0], parts[2]
	if lhs.Kind() != frontend.NodeObjectLiteralExpression {
		return nil, false, nil
	}
	// The left side is an object pattern, so from here the statement is ours.
	if rhs.Kind() != frontend.NodeIdentifier {
		return nil, true, &NotYetLowerable{Reason: "object destructuring assignment from anything but a plain variable needs a temporary, a later slice"}
	}
	objType := r.prog.TypeAt(rhs)
	if objType.Flags&frontend.TypeObject == 0 || r.isTypedArray(rhs) {
		return nil, true, &NotYetLowerable{Reason: "object destructuring assignment from a non-object source is a later slice"}
	}
	if _, isArray := r.prog.ElementType(objType); isArray {
		return nil, true, &NotYetLowerable{Reason: "object destructuring assignment from an array source is a later slice"}
	}
	if _, err := r.decls.internStruct(r, objType); err != nil {
		return nil, true, err
	}
	props := r.prog.Children(lhs)
	if len(props) == 0 {
		return nil, true, &NotYetLowerable{Reason: "an empty object assignment pattern binds nothing"}
	}
	// A nested pattern renamed onto a property, `({ p: { x } } = o)`, routes the whole
	// pattern through the assignment recursion, which holds each nested property in a
	// temporary and stores each leaf into its existing target. The source is already a
	// plain variable, so the repeated field reads do not re-evaluate it.
	for _, prop := range props {
		if _, _, ok := r.objectAssignNestedElem(prop); !ok {
			continue
		}
		recv, err := r.lowerExpr(rhs)
		if err != nil {
			return nil, true, err
		}
		stmts, err := r.bindSubObjectAssign(lhs, recv, objType)
		if err != nil {
			return nil, true, err
		}
		return &ast.BlockStmt{List: stmts}, true, nil
	}
	optionalField := map[string]bool{}
	for _, pr := range r.prog.Properties(objType) {
		optionalField[pr.Name] = pr.Optional
	}
	elems := make([]objectAssignElem, len(props))
	anyDefault := false
	anyRest := false
	for i, prop := range props {
		// A rest property gathers the properties the pattern did not name into the existing
		// target. On a fixed-shape source the target's type is that object minus the named
		// properties, so it interns as its own struct and the gather is a struct literal
		// copying each remaining field off the source, the assignment sibling of the
		// declaration rest.
		if restIdent, ok := r.objectRestElem(prop); ok {
			if _, ok := localName(r.prog.Text(restIdent)); !ok {
				return nil, true, &NotYetLowerable{Reason: "an object assignment rest target is not a Go identifier"}
			}
			restType := r.prog.TypeAt(restIdent)
			if restType.Flags&frontend.TypeObject == 0 {
				return nil, true, &NotYetLowerable{Reason: "an object assignment rest whose type is not a fixed-shape object is a later slice"}
			}
			if _, isArray := r.prog.ElementType(restType); isArray {
				return nil, true, &NotYetLowerable{Reason: "an object assignment rest typed as an array is a later slice"}
			}
			structName, err := r.decls.internStruct(r, restType)
			if err != nil {
				return nil, true, err
			}
			elems[i] = objectAssignElem{rest: true, bindNode: restIdent, restStruct: structName, restType: restType}
			anyRest = true
			continue
		}
		el, err := r.classifyObjectAssignElem(prop)
		if err != nil {
			return nil, true, err
		}
		elems[i] = el
		anyDefault = anyDefault || el.hasDefault
	}
	// A rest alongside a default combines the per-property fill with the gather, whose
	// interleaving is a later slice; the rest with plain and renamed properties takes the
	// parallel path below.
	if anyRest && anyDefault {
		return nil, true, &NotYetLowerable{Reason: "an object assignment that combines a rest with a default is a later slice"}
	}
	// A default turns the flat parallel assignment into a per-property fill, so the
	// pattern with any default takes the block path; the default-free pattern keeps
	// the single parallel assignment.
	if anyDefault {
		for _, el := range elems {
			if el.bindMember != nil {
				return nil, true, &NotYetLowerable{Reason: "an object assignment that combines a member target with a default is a later slice"}
			}
		}
		return r.objectDestructureAssignDefaults(elems, optionalField, rhs)
	}
	names := make([]ast.Expr, 0, len(props))
	values := make([]ast.Expr, 0, len(props))
	for _, el := range elems {
		if el.rest {
			recv, err := r.lowerExpr(rhs)
			if err != nil {
				return nil, true, err
			}
			name, ok := localName(r.prog.Text(el.bindNode))
			if !ok {
				return nil, true, &NotYetLowerable{Reason: "object assignment rest target is not a Go identifier"}
			}
			elts := make([]ast.Expr, 0, len(r.prog.Properties(el.restType)))
			for _, pr := range r.prog.Properties(el.restType) {
				field, ok := exportedField(pr.Name)
				if !ok {
					return nil, true, &NotYetLowerable{Reason: "an object assignment rest property is not a Go field name"}
				}
				elts = append(elts, &ast.KeyValueExpr{Key: ident(field), Value: &ast.SelectorExpr{X: recv, Sel: ident(field)}})
			}
			names = append(names, ident(name))
			values = append(values, &ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(el.restStruct), Elts: elts}})
			continue
		}
		srcName, ok := localName(r.prog.Text(el.nameNode))
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "object assignment property is not a Go identifier"}
		}
		field, ok := exportedField(srcName)
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "object assignment property is not a Go field name"}
		}
		recv, err := r.lowerExpr(rhs)
		if err != nil {
			return nil, true, err
		}
		var lhs ast.Expr
		if el.bindMember != nil {
			lhs, err = r.memberAssignTarget(el.bindMember)
			if err != nil {
				return nil, true, err
			}
		} else {
			name, ok := localName(r.prog.Text(el.bindNode))
			if !ok {
				return nil, true, &NotYetLowerable{Reason: "object assignment target is not a Go identifier"}
			}
			lhs = ident(name)
		}
		names = append(names, lhs)
		values = append(values, &ast.SelectorExpr{X: recv, Sel: ident(field)})
	}
	return &ast.AssignStmt{Lhs: names, Tok: token.ASSIGN, Rhs: values}, true, nil
}

// objectDestructureAssignDefaults lowers an object destructuring assignment that
// carries a default on at least one property, `({x = d, y} = o)`, to a block of
// per-property fills. A default over an optional field fills when the property is
// undefined, reading the field's Opt; a default over a required field can never
// fire, so it binds the read directly and the default is dead. A plain property
// takes its field read, the same as the default-free path.
func (r *Renderer) objectDestructureAssignDefaults(elems []objectAssignElem, optionalField map[string]bool, rhs frontend.Node) (ast.Stmt, bool, error) {
	out := make([]ast.Stmt, 0, len(elems))
	for _, el := range elems {
		prop := r.prog.Text(el.nameNode)
		srcName, ok := localName(prop)
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "object assignment property is not a Go identifier"}
		}
		field, ok := exportedField(srcName)
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "object assignment property is not a Go field name"}
		}
		name, ok := localName(r.prog.Text(el.bindNode))
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "object assignment target is not a Go identifier"}
		}
		recv, err := r.lowerExpr(rhs)
		if err != nil {
			return nil, true, err
		}
		read := &ast.SelectorExpr{X: recv, Sel: ident(field)}
		if el.hasDefault && optionalField[prop] {
			def, err := r.lowerExpr(el.defNode)
			if err != nil {
				return nil, true, err
			}
			def, err = r.coerceToType(def, el.defNode, r.prog.TypeAt(el.bindNode))
			if err != nil {
				return nil, true, err
			}
			out = append(out, r.defaultFillAssign(ident(name), read, def))
			continue
		}
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{read}})
	}
	return &ast.BlockStmt{List: out}, true, nil
}

// argumentsElementAssign lowers a write to arguments[i] to the backing store's Set,
// the store half of the At read elementAccess lowers for an arguments index. It
// claims the statement only when a store is in scope (argsObjName set) and the
// receiver is the arguments object, so an ordinary array or typed-array write falls
// through to the paths below. The store is a snapshot of the parameters, so writing
// it is the unmapped (strict) rule where arguments does not alias the named
// parameters; that is faithful only when no parameter is read by name in the body
// (argsWriteSafe), so a body that observes a parameter makes the mapped difference
// visible and hands the whole function back rather than emit an unfaithful write.
// Only a plain "=" is covered; a compound write reads and writes the element and is
// a later slice.
func (r *Renderer) argumentsElementAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	if r.argsObjName == "" {
		return nil, false, nil
	}
	parts := r.prog.Children(bin)
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" {
		return nil, false, nil
	}
	target := parts[0]
	if target.Kind() != frontend.NodeElementAccessExpression {
		return nil, false, nil
	}
	idxParts := r.prog.Children(target)
	if len(idxParts) != 2 || !r.isArgumentsIdent(idxParts[0]) {
		return nil, false, nil
	}
	if !r.argsWriteSafe {
		return nil, false, &NotYetLowerable{Reason: "a write to arguments while a parameter is read by name is the mapped-arguments aliasing corner the snapshot store does not mirror, a later slice"}
	}
	if !r.isNumber(idxParts[1]) {
		return nil, false, &NotYetLowerable{Reason: "a write to arguments with a non-number index is a later slice"}
	}
	idx, err := r.lowerExpr(idxParts[1])
	if err != nil {
		return nil, false, err
	}
	val, err := r.lowerExpr(parts[2])
	if err != nil {
		return nil, false, err
	}
	// The store holds value.Value, so the written value boxes into a dynamic slot the
	// same way the parameters did when the store was materialized.
	boxed, err := r.boxStaticToDynamicFlags(val, r.prog.TypeAt(parts[2]).Flags)
	if err != nil {
		return nil, false, err
	}
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ident(r.argsObjName), Sel: ident("Set")},
		Args: []ast.Expr{idx, boxed},
	}}, true, nil
}

// bytesElementAssign lowers a typed-array element write a[i] = v to the buffer's
// SetAt, the store half of the typed-array indexing lowered as a read by
// elementAccess (section 6.3). It reports ok=false when the statement is not an
// element write into a typed array, so lowerUpdate falls through to lowerAssign,
// which handles the local-identifier assignments; only when the target is an
// element access whose receiver the checker types a numeric typed array does this
// claim the statement. The value coerces to the element inside SetAt with the
// element kind's store rule, so the lowering passes it as the Number the checker
// typed it. Only a plain "=" is covered: a compound write a[i] += v reads and
// writes the element and is a later slice, so it hands back for the engine rather
// than dropping the read.
func (r *Renderer) bytesElementAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" {
		return nil, false, nil
	}
	target := parts[0]
	if target.Kind() != frontend.NodeElementAccessExpression {
		return nil, false, nil
	}
	idxParts := r.prog.Children(target)
	if len(idxParts) != 2 {
		return nil, false, nil
	}
	recvNode, idxNode := idxParts[0], idxParts[1]
	// A bigint typed-array write a[i] = v stores through the view's SetAt, which
	// truncates the bigint to the element's 64-bit width and drops an out-of-range or
	// non-canonical index. The value is a bigint (a *big.Int), not the Number the
	// numeric family stores, so this claims the write before the numeric path and
	// hands back a non-bigint value.
	if r.bigintTypedArray(recvNode) {
		if !r.isNumber(idxNode) {
			return nil, false, &NotYetLowerable{Reason: "a bigint typed-array write with a non-number index is a later slice"}
		}
		if !r.isBigInt(parts[2]) {
			return nil, false, &NotYetLowerable{Reason: "a bigint typed-array write of a non-bigint value is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, false, err
		}
		idx, err := r.lowerExpr(idxNode)
		if err != nil {
			return nil, false, err
		}
		val, err := r.lowerExpr(parts[2])
		if err != nil {
			return nil, false, err
		}
		return &ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ident("SetAt")},
			Args: []ast.Expr{idx, val},
		}}, true, nil
	}
	if !r.numericTypedArray(recvNode) {
		return nil, false, nil
	}
	if !r.isNumber(idxNode) {
		return nil, false, &NotYetLowerable{Reason: "a typed-array write with a non-number index is a later slice"}
	}
	if !r.isNumber(parts[2]) {
		return nil, false, &NotYetLowerable{Reason: "a typed-array write of a non-number value is a later slice"}
	}
	// A write into a fixed-length integer typed array at a proven-in-range index whose
	// value is itself an integer stores straight into the backing slice,
	// recv.Data()[idx] = int8(v). The Go conversion to the element width wraps exactly
	// as the store coercion does for an integer value, so it drops the coerce function
	// pointer, the bounds branch, and the float round trip the read-modify-write loop
	// otherwise pays every iteration. A non-integer value keeps SetAt, whose coercion
	// this native store cannot stand in for.
	if info, idxNode2, ok := r.provenTypedRead(target); ok && r.int32Producing(parts[2]) {
		stmt, err := r.typedSliceStore(recvNode, idxNode2, parts[2], info)
		if err != nil {
			return nil, false, err
		}
		return stmt, true, nil
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, false, err
	}
	val, err := r.lowerExpr(parts[2])
	if err != nil {
		return nil, false, err
	}
	// A proven-integer loop index writes through SetAtI, the integer-index store that
	// takes the index already narrowed and coerces and bounds-checks exactly as SetAt
	// does. A dynamic or fractional index keeps SetAt, which truncates the Number.
	if r.intLoopIndex(idxNode) {
		idx, err := r.intIndexExpr(idxNode)
		if err != nil {
			return nil, false, err
		}
		return &ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ident("SetAtI")},
			Args: []ast.Expr{idx, val},
		}}, true, nil
	}
	idx, err := r.lowerExpr(idxNode)
	if err != nil {
		return nil, false, err
	}
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: recv, Sel: ident("SetAt")},
		Args: []ast.Expr{idx, val},
	}}, true, nil
}

// arrayElementAssign lowers a general array element write a[i] = v to the array's
// Set, the store half of the At read elementAccess lowers (section 4.0). It reports
// ok=false when the statement is not an element write into an array, so lowerUpdate
// falls through to the byte-buffer, class-field, and local-identifier paths that
// own the other targets; only when the target is an element access whose receiver
// the checker types an array does this claim the statement. A byte buffer is
// handled by bytesElementAssign before this and is not an array in the checker's
// vocabulary, so it never reaches here. The value coerces to the element type the
// same way a local assignment coerces to its target, so a widening or a boxing the
// element type needs is applied rather than dropped. Only a plain "=" is covered: a
// compound write a[i] += v reads and writes the element and is a later slice, so it
// hands back for the engine rather than dropping the read.
func (r *Renderer) arrayElementAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" {
		return nil, false, nil
	}
	target := parts[0]
	if target.Kind() != frontend.NodeElementAccessExpression {
		return nil, false, nil
	}
	idxParts := r.prog.Children(target)
	if len(idxParts) != 2 {
		return nil, false, nil
	}
	recvNode, idxNode := idxParts[0], idxParts[1]
	if _, ok := r.arrayElem(recvNode); !ok {
		return nil, false, nil
	}
	if !r.isNumber(idxNode) {
		return nil, false, &NotYetLowerable{Reason: "an array write with a non-number index is a later slice"}
	}
	if hugeLiteralArrayIndex(idxNode, r.prog) {
		return nil, false, &NotYetLowerable{Reason: "an array write at a literal index far past the end (a[2**32 - 2] = v) needs the sparse representation, a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, false, err
	}
	idx, err := r.lowerExpr(idxNode)
	if err != nil {
		return nil, false, err
	}
	val, err := r.lowerExpr(parts[2])
	if err != nil {
		return nil, false, err
	}
	val, err = r.coerceToTarget(val, parts[2], target)
	if err != nil {
		return nil, false, err
	}
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: recv, Sel: ident("Set")},
		Args: []ast.Expr{idx, val},
	}}, true, nil
}

// objectFieldAssign lowers a property write o.k = v on a plain fixed-shape object
// to the Go struct field assignment o.K = v, the store half of the o.k read
// propertyAccess lowers (section 4.1). A plain object lowers to a pointer to its
// interned struct, so the field write shows through every reference to the object,
// which is what a mutation on a JavaScript object must be. It reports ok=false when
// the statement is not a property write into a plain object, so lowerUpdate falls
// through to the accessor and local-identifier paths that own the other targets. A
// class instance is claimed by classFieldAssign before this and is excluded again
// here for good measure, and an array, a byte buffer, a Map, and a Set are not
// plain objects, so none of them reach the field store. The value coerces to the
// field type the same way a local assignment coerces to its target. Only a plain
// "=" is covered: a compound write o.k += v is a later slice, so it hands back
// rather than emitting a store that skips the read.
func (r *Renderer) objectFieldAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" {
		return nil, false, nil
	}
	target := parts[0]
	if target.Kind() != frontend.NodePropertyAccessExpression {
		return nil, false, nil
	}
	tParts := r.prog.Children(target)
	if len(tParts) != 2 {
		return nil, false, nil
	}
	obj := tParts[0]
	// A write re.lastIndex = v sets the offset a global or sticky match resumes from
	// (22 §22.2.7). lastIndex is a property on value.RegExp reached through a method,
	// not a struct field, so it lowers to a SetLastIndex call. It routes before the
	// dynamic and fixed-shape gates below, which would box the receiver or fail to find
	// a field of that name on a shape a RegExp is not. The value coerces to the number
	// the property holds, the same float64 a read reports.
	if r.prog.Text(tParts[1]) == "lastIndex" && r.isRegExp(obj) {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, false, err
		}
		rhs, err := r.lowerExpr(parts[2])
		if err != nil {
			return nil, false, err
		}
		rhs, err = r.coerceToTarget(rhs, parts[2], target)
		if err != nil {
			return nil, false, err
		}
		return &ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ident("SetLastIndex")},
			Args: []ast.Expr{rhs},
		}}, true, nil
	}
	// A write o.k = v on a dynamic receiver (one typed any or unknown, new Object()
	// being the first source) has no static field to assign, so it dispatches at
	// runtime through the boxed value's Set, the mirror of the dynamic Get a read
	// takes. Set writes the property in place through the object's pointer and
	// returns the receiver, which the statement discards, so it lowers to a bare
	// call. The value boxes through boxOperand so a primitive rides its constructor
	// and a nested dynamic passes through. This routes before the static-shape gate
	// below, which expects a receiver whose object type the checker pinned down.
	if r.isDynamic(obj) {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, false, err
		}
		val, err := r.boxOperand(parts[2])
		if err != nil {
			return nil, false, err
		}
		r.requireImport(valuePkg)
		// A write o.__proto__ = v is the legacy prototype accessor, not an own property
		// of that name: an object or null retargets the slot honoring extensibility, and
		// any other value is left alone. It routes to SetProtoAssign rather than Set.
		if r.prog.Text(tParts[1]) == "__proto__" {
			return &ast.ExprStmt{X: &ast.CallExpr{
				Fun:  &ast.SelectorExpr{X: recv, Sel: ident("SetProtoAssign")},
				Args: []ast.Expr{val},
			}}, true, nil
		}
		key := &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{
			&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(r.prog.Text(tParts[1]))},
		}}
		return &ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ident("Set")},
			Args: []ast.Expr{key, val},
		}}, true, nil
	}
	objType := r.prog.TypeAt(obj)
	if objType.Flags&frontend.TypeObject == 0 {
		return nil, false, nil
	}
	if _, isArray := r.prog.ElementType(objType); isArray {
		return nil, false, nil
	}
	if r.isTypedArray(obj) || r.isMap(obj) || r.isSet(obj) {
		return nil, false, nil
	}
	if _, ok := r.classReceiver(obj); ok {
		return nil, false, nil
	}
	// A write o.k = v to a property the fixed shape never declared has no Go field
	// to land in: the struct interns exactly the shape's declared fields, so a read
	// of an absent property folds to value.MissingProperty (member.go), and putting
	// that read on the left of an assignment emits value.MissingProperty(o) = v,
	// which is not addressable and fails the go build. Adding a property a shape
	// never declared is a runtime shape mutation this path does not model, so it
	// hands back rather than assign to a non-lvalue.
	if _, present := r.shapeProp(objType, r.prog.Text(tParts[1])); !present {
		return nil, false, &NotYetLowerable{Reason: "a write o.k = v that adds a property the fixed-shape object never declared needs the object's runtime shape, a later slice"}
	}
	lhs, err := r.lowerExpr(target)
	if err != nil {
		return nil, false, err
	}
	rhs, err := r.lowerExpr(parts[2])
	if err != nil {
		return nil, false, err
	}
	rhs, err = r.coerceToTarget(rhs, parts[2], target)
	if err != nil {
		return nil, false, err
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{rhs}}, true, nil
}

// dynamicElementAssign lowers a bracket write o[k] = v on a dynamic receiver (one
// typed any or unknown) through the runtime store, the write mirror of the dynamic
// element read elementAccess lowers (member.go). arrayElementAssign already claims a
// receiver the checker types an array, so what reaches here is a receiver whose type
// the checker never pinned down, the same any target the dotted write o.x = v routes
// through objectFieldAssign. The key dispatches by its own type exactly as the read
// does: a number index writes through SetIndex, a dynamic key through SetElem, and a
// string key through SetKey, so a[3] = x lands in an array element and o["k"] = x in
// a named property by the same rule the read resolves them. The value boxes through
// boxOperand so a primitive rides its constructor and a nested dynamic passes
// through. The store returns the assigned value, which the statement discards, so it
// lowers to a bare call. A compound write o[k] <op>= v reads the old value once,
// runs the boxed arithmetic, and stores the result: the receiver and key are read
// on both the load and the store, so a side-effecting receiver or key hands back to
// keep the "evaluate the key once" rule, and a repeatable one reads the same slot
// on both sides. It reports ok=false when the statement is not a bracket write on a
// dynamic receiver, so lowerUpdate falls through to the paths that own the other
// targets.
func (r *Renderer) dynamicElementAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 {
		return nil, false, nil
	}
	opText := r.prog.Text(parts[1])
	baseOp, compound := compoundBaseOp(opText)
	if opText != "=" && !compound {
		return nil, false, nil
	}
	target := parts[0]
	if target.Kind() != frontend.NodeElementAccessExpression {
		return nil, false, nil
	}
	idxParts := r.prog.Children(target)
	if len(idxParts) != 2 {
		return nil, false, nil
	}
	recvNode, idxNode := idxParts[0], idxParts[1]
	if !r.isDynamic(recvNode) {
		return nil, false, nil
	}
	storeMethod, err := r.elementStoreMethod(idxNode)
	if err != nil {
		return nil, false, err
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, false, err
	}
	idx, err := r.lowerExpr(idxNode)
	if err != nil {
		return nil, false, err
	}
	var val ast.Expr
	if compound {
		if !r.repeatableOperand(recvNode) || !r.repeatableOperand(idxNode) {
			return nil, true, &NotYetLowerable{Reason: "compound assignment to a computed member with a side-effecting receiver or key is a later slice"}
		}
		loadMethod, err := r.elementLoadMethod(idxNode)
		if err != nil {
			return nil, false, err
		}
		recvLoad, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, false, err
		}
		idxLoad, err := r.lowerExpr(idxNode)
		if err != nil {
			return nil, false, err
		}
		old := &ast.CallExpr{Fun: &ast.SelectorExpr{X: recvLoad, Sel: ident(loadMethod)}, Args: []ast.Expr{idxLoad}}
		val, err = r.dynamicCompoundResult(baseOp, old, parts[2])
		if err != nil {
			return nil, false, err
		}
	} else {
		val, err = r.boxOperand(parts[2])
		if err != nil {
			return nil, false, err
		}
	}
	r.requireImport(valuePkg)
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: recv, Sel: ident(storeMethod)},
		Args: []ast.Expr{idx, val},
	}}, true, nil
}

// lowerIncDec lowers a ++ or -- applied to a local number. In statement
// position the prefix and postfix forms have the same effect since the value
// is discarded, so both map to a Go IncDecStmt. Go's ++ and -- accept a
// float64, so the loop counter keeps its number type. The operand must be a
// local identifier of number type; ++/-- on anything else hands back.
func (r *Renderer) lowerIncDec(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "increment expression did not expose a single operand"}
	}
	operand := kids[0]
	op := strings.TrimSpace(strings.Trim(strings.ReplaceAll(r.prog.Text(n), r.prog.Text(operand), ""), " "))
	var tok token.Token
	switch op {
	case "++":
		tok = token.INC
	case "--":
		tok = token.DEC
	default:
		return nil, &NotYetLowerable{Reason: "update operator " + op + " is a later slice"}
	}
	// this.count++ and p.count++ on a class field are the same statement on a
	// struct field, so the field selector the class lowering builds takes the
	// place of the local identifier.
	if operand.Kind() == frontend.NodePropertyAccessExpression {
		if lhs, ok, err := r.classFieldTarget(operand); err != nil {
			return nil, err
		} else if ok {
			if r.isDynamic(operand) {
				lhs2, _, _ := r.classFieldTarget(operand)
				return r.dynamicIncDec(lhs, lhs2, tok), nil
			}
			if !r.isNumber(operand) {
				return nil, &NotYetLowerable{Reason: "increment of a non-number needs coercion, a later slice"}
			}
			return &ast.IncDecStmt{X: lhs, Tok: tok}, nil
		}
	}
	if operand.Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "increment of a non-identifier target is a later slice"}
	}
	name, ok := localName(r.prog.Text(operand))
	if !ok {
		return nil, &NotYetLowerable{Reason: "increment target is not a Go identifier"}
	}
	// A local bound without an initializer lives in a value.Value slot even after
	// control flow narrows it to number: var count; count = 0; count++ types count
	// number at the ++, but the storage is still the box, so a Go count++ would try
	// to increment a value.Value. The dynLocals check routes on storage, not the
	// narrowed type, so the update goes through value.Inc on the box.
	if r.isDynamic(operand) || r.dynLocals[name] {
		return r.dynamicIncDec(ident(name), ident(name), tok), nil
	}
	if !r.isNumber(operand) {
		return nil, &NotYetLowerable{Reason: "increment of a non-number needs coercion, a later slice"}
	}
	return &ast.IncDecStmt{X: ident(name), Tok: tok}, nil
}

// dynamicIncDec lowers a ++ or -- on a dynamic target in statement position,
// where the discarded result makes the prefix and postfix forms the same effect.
// A dynamic value has no Go ++ to apply, so the update reads the target, runs the
// numeric increment through value.Inc or value.Dec, which is ToNumeric and keeps a
// bigint a bigint, and assigns the result back. The read and the write take
// separate expression nodes so the printed form holds no shared node; both name
// the same side-effect-free target, a local or a field of one, so evaluating it
// on each side is the same access.
func (r *Renderer) dynamicIncDec(write, read ast.Expr, tok token.Token) *ast.AssignStmt {
	fn := "Inc"
	if tok == token.DEC {
		fn = "Dec"
	}
	r.requireImport(valuePkg)
	return &ast.AssignStmt{
		Lhs: []ast.Expr{write},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: sel("value", fn), Args: []ast.Expr{read}}},
	}
}

// lowerAssign lowers a binary assignment to a Go assignment statement. It
// covers both a plain "=" and a compound operator like "+=": a compound
// assignment desugars to x = x <op> rhs and reuses combineBinary, so a "+="
// on strings concatenates and a "%=" becomes math.Mod, exactly as the binary
// operator would. It is shared by an assignment used as a statement and a
// for-loop's post clause. The target must be a local identifier; assigning to
// a property or an element is a later slice.
func (r *Renderer) lowerAssign(bin frontend.Node) (*ast.AssignStmt, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 {
		return nil, &NotYetLowerable{Reason: "binary expression did not expose left, operator, right"}
	}
	opText := r.prog.Text(parts[1])
	baseOp, compound := compoundBaseOp(opText)
	if opText != "=" && !compound {
		return nil, &NotYetLowerable{Reason: "non-assignment expression used as a statement is a later slice"}
	}
	if parts[0].Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "assignment to a non-identifier target is a later slice"}
	}
	// An assignment whose target is an ambient global (NaN = 12, undefined = 1) is
	// not a store into a user slot: the runtime holds no such lvalue, and in strict
	// mode the store throws a TypeError bento does not model, while in sloppy mode it
	// is a silent no-op. Emitting the source name would name an undefined Go symbol.
	// Hand back the way the read path does. global/10.2.1.1.3-4-16-s hits this.
	if r.isAmbientGlobal(parts[0]) {
		return nil, &NotYetLowerable{Reason: "assignment to the ambient global " + r.prog.Text(parts[0]) + " is a later slice"}
	}
	name, ok := localName(r.prog.Text(parts[0]))
	if !ok {
		return nil, &NotYetLowerable{Reason: "assignment target is not a Go identifier"}
	}
	var rhs ast.Expr
	var err error
	if compound {
		rhs, err = r.combineBinary(baseOp, parts[0], parts[2])
		if err != nil {
			return nil, err
		}
		// A compound + on a dynamic operand produces a boxed value.Value, so when the
		// target is a static primitive the result must coerce back to it (total += x
		// keeps total a number even though x is any). The target being dynamic needs
		// no coercion, since the boxed result already fits, and a compound whose result
		// is static reaches a static target directly.
		if r.combineIsDynamic(baseOp, parts[0], parts[2]) && !r.isDynamic(parts[0]) {
			rhs, err = r.coerceDynamicToStatic(rhs, parts[0])
			if err != nil {
				return nil, err
			}
		} else if r.isDynamic(parts[0]) && !r.combineIsDynamic(baseOp, parts[0], parts[2]) {
			// The target is a boxed dynamic binding but the compound result is static: a +
			// with a string operand concatenates, and value.Concat returns a bstr rather
			// than a box, so the bare bstr does not fit the value.Value slot. Wrap it back
			// into a box so the assignment types. Only + reaches here for a dynamic target,
			// since combineBinary hands back every other operator on a dynamic operand.
			// message += ' ' in the test262 prelude takes this shape, message being any and
			// the right side a string literal.
			if baseOp != "+" {
				return nil, &NotYetLowerable{Reason: "compound assignment other than + on a dynamic target is a later slice"}
			}
			r.requireImport(valuePkg)
			rhs = &ast.CallExpr{Fun: sel("value", "StringValue"), Args: []ast.Expr{rhs}}
		} else if !r.combineIsDynamic(baseOp, parts[0], parts[2]) && r.localStorageDynamic(parts[0]) {
			// The target's Go slot is a boxed value.Value, e.g. `var y;` with no
			// initializer, but control-flow analysis narrowed the read that combineBinary
			// lowered, so the compound result came out a static primitive that does not
			// fit the boxed slot. Box it back: a + over a string concatenates to a bstr
			// that StringValue boxes, every arithmetic operator leaves a float64 that
			// value.Number boxes. `var y; y = 1; y /= -1;` in the test262 compound-assign
			// suite takes this shape, y declared value.Value but read as a number.
			r.requireImport(valuePkg)
			if baseOp == "+" && (r.isString(parts[0]) || r.isString(parts[2])) {
				rhs = &ast.CallExpr{Fun: sel("value", "StringValue"), Args: []ast.Expr{rhs}}
			} else {
				rhs = &ast.CallExpr{Fun: sel("value", "Number"), Args: []ast.Expr{rhs}}
			}
		}
	} else if r.int32Locals[name] {
		// The target is an int32-specialized local, so the right-hand side lowers in the
		// int32 domain and the assignment to the int32 variable is the ToInt32 the value
		// would otherwise get from a | 0 or a Math.imul. The analysis only specialized
		// this local because every one of its writes is inherently int32, so int32Of has
		// a native lowering for this right-hand side rather than the coercing fallback.
		rhs, err = r.int32Of(parts[2])
		if err != nil {
			return nil, err
		}
	} else if r.int64Locals[name] {
		// The target is an int64-specialized local, so the right-hand side lowers in
		// the int64 domain. The analysis approved every one of this local's writes
		// through the same walk int64Of lowers, so the native form always exists.
		rhs, err = r.int64Of(parts[2])
		if err != nil {
			return nil, err
		}
	} else {
		rhs, err = r.lowerExpr(parts[2])
		if err != nil {
			return nil, err
		}
		rhs, err = r.coerceToTarget(rhs, parts[2], parts[0])
		if err != nil {
			return nil, err
		}
	}
	// A right-hand side that is exactly "target op value" over a native Go operator
	// collapses to Go's compound assignment, so total = total + i is written total +=
	// i the way a developer would. This fires whether the source wrote += or spelled
	// the self-reference out, and only on a bare binary whose left operand is the
	// target identifier, so a string concat (a .Concat call, not a Go +) or a
	// coercion-wrapped result is left as the plain assignment it needs to be.
	tok := token.ASSIGN
	if bin, ok := rhs.(*ast.BinaryExpr); ok {
		if ctok, ok := compoundAssignToken(bin.Op); ok {
			if x, ok := bin.X.(*ast.Ident); ok && x.Name == name {
				tok = ctok
				rhs = bin.Y
			}
		}
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{ident(name)},
		Tok: tok,
		Rhs: []ast.Expr{rhs},
	}, nil
}

// logicalAssign lowers a logical assignment used as a statement: x ??= y, x ||= y,
// or x &&= y. Unlike an arithmetic compound, these short-circuit: y is evaluated
// and stored only when x has the operator's trigger value, so the whole thing is
// an if that guards a plain assignment rather than x = x <op> y. Putting the
// assignment inside the guard keeps the short-circuit exactly, so y may have a
// side effect the eager forms could not admit.
//
// ??= assigns when x is undefined, so the target must be an optional (the
// T | undefined the Opt models), and the guard is x.IsUndefined(). ||= assigns
// when x is falsy and &&= when x is truthy, so both read x through the same
// JavaScript truthiness lowerTruthy spells for the target's type, negated for
// ||=. The target must be a plain local identifier so it can be named in both the
// guard and the assignment with no repeated side effect.
// A non-logical operator reports ok=false so the caller falls through to the plain
// and arithmetic-compound path.
func (r *Renderer) logicalAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 {
		return nil, false, nil
	}
	op := r.prog.Text(parts[1])
	if op != "??=" && op != "||=" && op != "&&=" {
		return nil, false, nil
	}
	target := parts[0]
	if target.Kind() == frontend.NodePropertyAccessExpression {
		return r.memberLogicalAssign(target, op, parts[2])
	}
	if target.Kind() != frontend.NodeIdentifier {
		return nil, true, &NotYetLowerable{Reason: "logical assignment to a non-identifier target is a later slice"}
	}
	name, ok := localName(r.prog.Text(target))
	if !ok {
		return nil, true, &NotYetLowerable{Reason: "logical assignment target is not a Go identifier"}
	}
	// Build the guard the operator triggers on.
	var cond ast.Expr
	switch op {
	case "??=":
		switch {
		case r.isDynamic(target):
			// A dynamic target is nullish on either null or undefined, so its guard is
			// the runtime IsNullish, the same presence test dynamicNullishCoalesce runs.
			// The slot stays a box, so the store below coerces the right-hand side into a
			// value the same way any assignment into a dynamic slot does.
			cond = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("IsNullish")}}
		case r.isOptional(target):
			// An optional target is nullish only on undefined, so its guard is the Opt
			// presence flag. A definite right-hand side leaves the target present after
			// the store, which the checker narrows to the bare T; the store keeps the
			// slot Opt[T] (coerceToTarget wraps the value in Some), so a later read the
			// checker narrowed must unwrap with .Get() for the slot and the narrowed use
			// to agree. That unwrap only fires for a local the optLocals pre-pass tracks;
			// a parameter is not tracked, so its narrowed read would keep the bare Opt and
			// fail to compile, and a definite right-hand side into one hands back. An
			// optional right-hand side leaves the target still T | undefined, so no
			// narrowing happens and either kind of target lowers.
			if !r.isOptional(parts[2]) && !r.isOptBinding(name) {
				return nil, true, &NotYetLowerable{Reason: "??= with a definite right-hand side into a target the narrowing pre-pass does not track is a later slice"}
			}
			cond = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("IsUndefined")}}
		default:
			return nil, true, &NotYetLowerable{Reason: "??= on a target that is neither the optional T | undefined nor a dynamic value is a later slice"}
		}
	case "||=":
		// ||= assigns when the target is falsy, so the guard is the target's
		// JavaScript truthiness negated. lowerTruthy spells the falsy set for the
		// target's type: the bare identifier for a boolean, x != 0 && x == x for a
		// number, s.Length() > 0 for a string, and value.ToBoolean for a dynamic
		// value. An object or union target still hands back through lowerTruthy.
		truth, err := r.lowerTruthy(target)
		if err != nil {
			return nil, true, err
		}
		cond = &ast.UnaryExpr{Op: token.NOT, X: truth}
	case "&&=":
		// &&= is the mirror: it assigns when the target is truthy, so the guard is
		// the same truthiness test without the negation.
		truth, err := r.lowerTruthy(target)
		if err != nil {
			return nil, true, err
		}
		cond = truth
	}
	rhs, err := r.lowerExpr(parts[2])
	if err != nil {
		return nil, true, err
	}
	rhs, err = r.coerceToTarget(rhs, parts[2], target)
	if err != nil {
		return nil, true, err
	}
	// Named evaluation: an anonymous function assigned to an identifier takes that
	// identifier as its name (value ??= function() {} binds the name "value"). The
	// dynamic target boxes the right-hand side to a function value, so wrap it in
	// value.WithName to record the name a later f.name read returns. A named function
	// keeps its own name and is not wrapped. logical-assignment/
	// lgcl-nullish-assignment-operator-namedevaluation-function and -arrow-function
	// exercise this.
	if r.isDynamic(target) && r.isAnonymousFunctionDef(parts[2]) {
		r.requireImport(valuePkg)
		rhs = &ast.CallExpr{
			Fun:  sel("value", "WithName"),
			Args: []ast.Expr{rhs, &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(strings.TrimSpace(r.prog.Text(target)))}},
		}
	}
	assign := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(name)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{rhs},
	}
	return &ast.IfStmt{
		Cond: cond,
		Body: &ast.BlockStmt{List: []ast.Stmt{assign}},
	}, true, nil
}

// incDecFromStep rewrites a compound step of one, x += 1 or x -= 1, into Go's ++
// or --. It fires only on a bare += or -= whose right-hand side is the literal 1,
// so a step of any other size or a wider expression keeps the compound assignment.
// The value is discarded in every position lowerUpdate serves, so ++ and += 1 are
// the same operation, and ++ is how it reads.
func incDecFromStep(a *ast.AssignStmt) (*ast.IncDecStmt, bool) {
	var tok token.Token
	switch a.Tok {
	case token.ADD_ASSIGN:
		tok = token.INC
	case token.SUB_ASSIGN:
		tok = token.DEC
	default:
		return nil, false
	}
	if len(a.Lhs) != 1 || len(a.Rhs) != 1 {
		return nil, false
	}
	lit, ok := a.Rhs[0].(*ast.BasicLit)
	if !ok || (lit.Value != "1" && lit.Value != "1.0") {
		return nil, false
	}
	return &ast.IncDecStmt{X: a.Lhs[0], Tok: tok}, true
}

// compoundBaseOp maps a compound assignment operator to the binary operator it
// fuses, so combineBinary can build the x <op> rhs half of x <op>= rhs. Every
// arithmetic and bitwise compound is here, including **= which fuses to the **
// combineBinary lowers to value.Pow; the plain "=" is not a compound and returns
// false.
// compoundAssignToken maps a native Go binary operator to its compound-assignment
// form (ADD to +=, SHL to <<=, and so on), reporting whether one exists. It is the
// peephole that lets total = total + i print as total += i: every arithmetic and
// bitwise Go operator has a compound form, so the rewrite is always available when the
// right-hand side is a bare binary over the target. Comparison and logical operators
// have no compound form and return false, so a boolean assignment is left alone.
func compoundAssignToken(op token.Token) (token.Token, bool) {
	switch op {
	case token.ADD:
		return token.ADD_ASSIGN, true
	case token.SUB:
		return token.SUB_ASSIGN, true
	case token.MUL:
		return token.MUL_ASSIGN, true
	case token.QUO:
		return token.QUO_ASSIGN, true
	case token.REM:
		return token.REM_ASSIGN, true
	case token.AND:
		return token.AND_ASSIGN, true
	case token.OR:
		return token.OR_ASSIGN, true
	case token.XOR:
		return token.XOR_ASSIGN, true
	case token.SHL:
		return token.SHL_ASSIGN, true
	case token.SHR:
		return token.SHR_ASSIGN, true
	case token.AND_NOT:
		return token.AND_NOT_ASSIGN, true
	default:
		return token.ILLEGAL, false
	}
}

func compoundBaseOp(op string) (string, bool) {
	switch op {
	case "+=":
		return "+", true
	case "-=":
		return "-", true
	case "*=":
		return "*", true
	case "**=":
		return "**", true
	case "/=":
		return "/", true
	case "%=":
		return "%", true
	case "&=":
		return "&", true
	case "|=":
		return "|", true
	case "^=":
		return "^", true
	case "<<=":
		return "<<", true
	case ">>=":
		return ">>", true
	case ">>>=":
		return ">>>", true
	default:
		return "", false
	}
}

// lowerFor lowers a C-style for to Go. The classic three-clause form maps almost
// directly, but Go forbids a var declaration in a for's init clause, so a
// let-initialized loop is emitted as a Go block holding the declaration followed
// by a for with an empty init: the loop variable keeps its block scope and its
// float64 type, which a := init would lose to int inference. An expression
// initializer, one that assigns an existing binding rather than declaring a new
// one, needs no wrapping block and lowers straight into the for's init clause; a
// destructuring initializer still hands back as its own later slice.
// lowerForPost lowers a for loop's post clause. A single update lowers the way it
// does anywhere else. A comma of updates, the i++, j-- a two-pointer loop walks
// with, cannot be two Go statements because a Go post clause holds exactly one, so
// its operands fuse into one parallel assignment, i, j = i + 1, j - 1, which runs
// them together the way the comma sequence does. The fuse needs every operand to
// assign a distinct local; an operand that targets a property, repeats a target,
// or is a call (which cannot sit on the left of an assignment) hands back, since
// those cannot join one parallel assignment.
func (r *Renderer) lowerForPost(n frontend.Node) (ast.Stmt, error) {
	ops, ok := commaOperands(r.prog, n)
	if !ok {
		return r.lowerUpdate(n)
	}
	var lhs, rhs []ast.Expr
	seen := make(map[string]bool, len(ops))
	for _, op := range ops {
		s, err := r.lowerUpdate(op)
		if err != nil {
			return nil, err
		}
		l, r2, ok := postAssignPair(s)
		if !ok {
			return nil, &NotYetLowerable{Reason: "a for post clause comma with an operand that is not a local assignment is a later slice"}
		}
		name, ok := l.(*ast.Ident)
		if !ok || seen[name.Name] {
			return nil, &NotYetLowerable{Reason: "a for post clause comma that repeats or computes its target is a later slice"}
		}
		seen[name.Name] = true
		lhs = append(lhs, l)
		rhs = append(rhs, r2)
	}
	return &ast.AssignStmt{Lhs: lhs, Tok: token.ASSIGN, Rhs: rhs}, nil
}

// postAssignPair rewrites one lowered update statement into the (target, value)
// pair a parallel assignment needs, so an increment, a plain assignment, and a
// compound step all reduce to target = value. An increment becomes target +/- 1;
// a compound step x <op>= v becomes x = x <op> v, the same fuse combineBinary
// undoes going the other way. A statement that is not one of these (a call) has no
// such pair and reports ok=false.
func postAssignPair(s ast.Stmt) (ast.Expr, ast.Expr, bool) {
	switch st := s.(type) {
	case *ast.IncDecStmt:
		op := token.ADD
		if st.Tok == token.DEC {
			op = token.SUB
		}
		return st.X, &ast.BinaryExpr{X: st.X, Op: op, Y: &ast.BasicLit{Kind: token.INT, Value: "1"}}, true
	case *ast.AssignStmt:
		if len(st.Lhs) != 1 || len(st.Rhs) != 1 {
			return nil, nil, false
		}
		if st.Tok == token.ASSIGN {
			return st.Lhs[0], st.Rhs[0], true
		}
		base, ok := compoundAssignBase(st.Tok)
		if !ok {
			return nil, nil, false
		}
		return st.Lhs[0], &ast.BinaryExpr{X: st.Lhs[0], Op: base, Y: st.Rhs[0]}, true
	default:
		return nil, nil, false
	}
}

// compoundAssignBase maps a Go compound-assignment token to the binary operator it
// fuses, the inverse of compoundAssignToken, so x += v can be rewritten as x + v
// for a parallel assignment. A token that is not a compound assignment returns
// false.
func compoundAssignBase(tok token.Token) (token.Token, bool) {
	switch tok {
	case token.ADD_ASSIGN:
		return token.ADD, true
	case token.SUB_ASSIGN:
		return token.SUB, true
	case token.MUL_ASSIGN:
		return token.MUL, true
	case token.QUO_ASSIGN:
		return token.QUO, true
	case token.REM_ASSIGN:
		return token.REM, true
	case token.AND_ASSIGN:
		return token.AND, true
	case token.OR_ASSIGN:
		return token.OR, true
	case token.XOR_ASSIGN:
		return token.XOR, true
	case token.SHL_ASSIGN:
		return token.SHL, true
	case token.SHR_ASSIGN:
		return token.SHR, true
	case token.AND_NOT_ASSIGN:
		return token.AND_NOT, true
	default:
		return token.ILLEGAL, false
	}
}

func (r *Renderer) lowerFor(n frontend.Node) (ast.Stmt, error) {
	// A for statement's three header clauses are each optional in the source, so
	// the roles are read straight off the node rather than by walking children:
	// an omitted clause leaves no child, and a bare child list cannot say which
	// of the surviving nodes is the condition and which the incrementor. An empty
	// condition means the loop runs forever, which Go writes as a bare for with no
	// condition; an empty incrementor means no post clause.
	fc := r.prog.ForClauses(n)

	// The loop opens its own Go scope: the initializer either sits in the for
	// clause or in the block that wraps the loop, so its names belong to the loop,
	// not the enclosing block. A fresh frame keeps those names out of the enclosing
	// block's declared set, so a later loop reusing the same counter names still
	// declares them rather than mistaking them for a redeclaration.
	r.blockDeclared = append(r.blockDeclared, map[string]bool{})
	defer func() { r.blockDeclared = r.blockDeclared[:len(r.blockDeclared)-1] }()

	var decls []frontend.Node
	var exprInit ast.Stmt
	if fc.HasInit {
		collectVarDecls(r.prog, fc.Init, &decls)
		// A `var` counter this loop declares but a later statement reuses hoists to
		// the scope top, so here the init writes the existing binding rather than
		// declaring a fresh loop-local. Lowering it as an assignment keeps the counter
		// one shared binding, the way JavaScript's function scope means it to be.
		if hoisted, ok, err := r.hoistedForInit(decls); err != nil {
			return nil, err
		} else if ok {
			exprInit = hoisted
			decls = nil
		}
	}
	if fc.HasInit {
		if len(decls) == 0 && exprInit == nil {
			// An expression initializer writes to a binding that already exists
			// rather than declaring one, so it needs no wrapping block: it lowers
			// straight into Go's for init clause the way the post clause lowers its
			// own update. A single assignment stands as one statement; a comma of
			// assignments fuses into one parallel assignment.
			s, err := r.lowerForPost(fc.Init)
			if err != nil {
				return nil, err
			}
			exprInit = s
		} else {
			// A destructuring initializer binds a pattern, not a plain name, and the
			// pattern's own lowering is a separate slice. The binding name of a plain
			// counter is an identifier node; an array or object pattern is not, so hand
			// back rather than mangle the pattern text into one Go name.
			for _, d := range decls {
				kids := r.prog.Children(d)
				if len(kids) == 0 {
					continue
				}
				if kids[0].Kind() != frontend.NodeIdentifier {
					return nil, &NotYetLowerable{Reason: "a for loop with a destructuring initializer is a later slice"}
				}
			}
		}
	}

	var cond ast.Expr
	if fc.HasCond {
		c, err := r.lowerCondition(fc.Cond)
		if err != nil {
			return nil, err
		}
		cond = c
	}
	var post ast.Stmt
	if fc.HasIncr {
		p, err := r.lowerForPost(fc.Incr)
		if err != nil {
			return nil, err
		}
		post = p
	}
	body, err := r.loopBody(fc.Body)
	if err != nil {
		return nil, err
	}

	// With no declaration to place, the surviving clauses go straight onto the for.
	// A nil condition is Go's infinite loop, a nil post clause is none, so for(;;),
	// for(;cond;), and for(let-less shapes) all read the way a developer writes them.
	if len(decls) == 0 {
		return &ast.ForStmt{Init: exprInit, Cond: cond, Post: post, Body: body}, nil
	}

	// A loop variable an omitted clause leaves unread is still declared, and Go
	// rejects a declared-and-unused local. A condition or an incrementor usually
	// reads the counter, but for(let i=0; false;) and for(let i=0;;) read none, so
	// each unread binding takes a blank assignment the way an unused let does. A
	// bound blank also keeps such a binding out of the fold, whose := init has no
	// place to hang one.
	blanks := r.forInitBlanks(decls)

	// A single float64 loop variable folds into the for's own init clause, so the
	// loop reads for i := 0.0; i < n; i++ the way a developer writes it rather than a
	// block wrapping a var declaration. The block form stays for everything the fold
	// declines (an int32-specialized counter, a hex or non-literal initializer, more
	// than one loop variable, an unread counter that needs a blank), because Go's :=
	// would infer int for those and lose the declared type the block's var keeps.
	if len(blanks) == 0 {
		if init, ok := r.foldFloatDecl(decls); ok {
			return &ast.ForStmt{Init: init, Cond: cond, Post: post, Body: body}, nil
		}
	}
	initDecl, err := r.varDeclStmt(decls)
	if err != nil {
		return nil, err
	}
	loop := &ast.ForStmt{Cond: cond, Post: post, Body: body}
	list := append([]ast.Stmt{initDecl}, blanks...)
	list = append(list, loop)
	return &ast.BlockStmt{List: list}, nil
}

// forInitBlanks builds a blank assignment for each for-loop binding no clause of
// the loop reads, so a counter an omitted condition or incrementor orphans does
// not trip Go's declared-and-unused rule. A binding read anywhere in the loop
// gets none, the same test lowerVarStatementMulti applies to a plain let.
func (r *Renderer) forInitBlanks(decls []frontend.Node) []ast.Stmt {
	var out []ast.Stmt
	for _, d := range decls {
		kids := r.prog.Children(d)
		if len(kids) == 0 {
			continue
		}
		name, ok := localName(r.prog.Text(kids[0]))
		if !ok {
			continue
		}
		if r.bindingUnused(kids[0]) {
			out = append(out, &ast.AssignStmt{
				Lhs: []ast.Expr{ident("_")},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{ident(name)},
			})
		}
	}
	return out
}

// hoistedForInit turns a for loop's `var` init into an assignment when the counter
// has been hoisted to the scope top, so the loop writes the shared binding instead
// of shadowing it with a loop-local. It fires only when a binding is in the hoisted
// set, so a plain loop-local counter keeps its declaration and its fold. A single
// binding lowers to one assignment, several to a parallel one; a hoisted binding
// mixed with a non-hoisted one, or one with no initializer, is a later slice.
func (r *Renderer) hoistedForInit(decls []frontend.Node) (ast.Stmt, bool, error) {
	anyHoisted := false
	for _, d := range decls {
		kids := r.prog.Children(d)
		if len(kids) == 0 {
			continue
		}
		if name, ok := localName(r.prog.Text(kids[0])); ok && r.hoistedVars[name] {
			anyHoisted = true
		}
	}
	if !anyHoisted {
		return nil, false, nil
	}
	var lhs, rhs []ast.Expr
	for _, d := range decls {
		kids := r.prog.Children(d)
		if len(kids) != 2 && len(kids) != 3 {
			return nil, false, &NotYetLowerable{Reason: "a hoisted for-init var without a single initializer is a later slice"}
		}
		name, ok := localName(r.prog.Text(kids[0]))
		if !ok || !r.hoistedVars[name] {
			return nil, false, &NotYetLowerable{Reason: "a for-init mixing a hoisted var with a non-hoisted binding is a later slice"}
		}
		init, err := r.bindingInit(kids[0], kids[len(kids)-1])
		if err != nil {
			return nil, false, err
		}
		lhs = append(lhs, ident(name))
		rhs = append(rhs, init)
	}
	return &ast.AssignStmt{Lhs: lhs, Tok: token.ASSIGN, Rhs: rhs}, true, nil
}

// foldFloatDecl builds a Go short variable declaration from a single float64
// binding, so name := 0.0 stands in for var name float64 = 0. It serves both a
// for loop's init clause, where the short form folds into the for statement, and a
// plain const or let statement, where it reads the way a person would write it. It
// returns false, and the caller keeps the typed var form, unless there is exactly
// one binding, that binding is a plain float64 (not an int32-specialized counter),
// and its initializer is a decimal literal floatLiteral can retype: Go's := infers
// int from a bare 0, so the fold is sound only when the initializer already denotes,
// or can be spelled as, a float64.
func (r *Renderer) foldFloatDecl(decls []frontend.Node) (ast.Stmt, bool) {
	if len(decls) != 1 {
		return nil, false
	}
	kids := r.prog.Children(decls[0])
	if len(kids) != 2 && len(kids) != 3 {
		return nil, false
	}
	name, ok := localName(r.prog.Text(kids[0]))
	if !ok || r.int32Locals[name] || r.int64Locals[name] {
		return nil, false
	}
	typ, err := r.typeExpr(r.prog.TypeAt(kids[0]))
	if err != nil || !isFloat64Ident(typ) {
		return nil, false
	}
	init, err := r.bindingInit(kids[0], kids[len(kids)-1])
	if err != nil {
		return nil, false
	}
	finit, ok := floatLiteral(init)
	if !ok {
		return nil, false
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.DEFINE, Rhs: []ast.Expr{finit}}, true
}

// foldShortDecl builds a Go short variable declaration for a lone binding whose
// initializer already carries the binding's own Go type, so name := value stands in
// for var name T = value and infers the same T. It reaches every local past the
// numeric literals: a string built from value.FromGoString, an array from
// value.NewArray, a directory from a bridged call. The safety rests on bindingInit,
// which coerces the initializer to the binding's type, so the lowered initializer is
// already of type T and := infers T exactly.
//
// It declines three shapes. More than one binding stays a grouped var. An
// int32-specialized counter keeps its explicit int32, which documents the
// specialization the analysis chose. A bare literal initializer hands back, because a
// Go untyped integer constant infers int under := where the binding means float64;
// foldFloatDecl runs first and floatifies the ones it can, and the rest keep the
// typed var.
func (r *Renderer) foldShortDecl(decls []frontend.Node) (ast.Stmt, bool) {
	if len(decls) != 1 {
		return nil, false
	}
	kids := r.prog.Children(decls[0])
	if len(kids) != 2 && len(kids) != 3 {
		return nil, false
	}
	name, ok := localName(r.prog.Text(kids[0]))
	if !ok || r.int32Locals[name] || r.int64Locals[name] {
		return nil, false
	}
	init, err := r.bindingInit(kids[0], kids[len(kids)-1])
	if err != nil {
		return nil, false
	}
	if _, isLit := init.(*ast.BasicLit); isLit {
		return nil, false
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.DEFINE, Rhs: []ast.Expr{init}}, true
}

// isFloat64Ident reports whether a lowered type expression is the bare float64 type,
// the type every plain JavaScript number local takes. It is how the readability
// rewrites tell a number local apart from a string, boolean, or richer type whose :=
// inference is already exact.
func isFloat64Ident(typ ast.Expr) bool {
	id, ok := typ.(*ast.Ident)
	return ok && id.Name == "float64"
}

// floatLiteral retypes a numeric initializer so Go's := infers float64 from it. A
// literal already written as a float (it carries a point or an exponent) passes
// through, and a plain decimal integer literal gains a trailing .0 so 0 becomes 0.0
// and 5 becomes 5.0. A negative literal arrives as a unary minus wrapping the
// literal (-5 is SUB over 5), so the sign is peeled off, the inner literal retyped,
// and the minus put back, which turns -5 into -5.0 rather than a Go int. A hex,
// binary, or octal integer literal, or anything that is not a bare literal, returns
// false: those have no short float spelling here, so the caller keeps the typed var
// form rather than change the value.
func floatLiteral(init ast.Expr) (ast.Expr, bool) {
	if neg, ok := init.(*ast.UnaryExpr); ok && neg.Op == token.SUB {
		inner, ok := floatLiteral(neg.X)
		if !ok {
			return nil, false
		}
		return &ast.UnaryExpr{Op: token.SUB, X: inner}, true
	}
	lit, ok := init.(*ast.BasicLit)
	if !ok {
		return nil, false
	}
	if lit.Kind == token.FLOAT {
		return lit, true
	}
	if lit.Kind != token.INT {
		return nil, false
	}
	for i := 0; i < len(lit.Value); i++ {
		if lit.Value[i] < '0' || lit.Value[i] > '9' {
			// A prefix (0x, 0b, 0o) or separator means this is not a plain decimal run,
			// so there is no bare float spelling for it.
			return nil, false
		}
	}
	return &ast.BasicLit{Kind: token.FLOAT, Value: lit.Value + ".0"}, true
}

// loopBody lowers the body of a loop, which JavaScript allows to be either a
// braced block or a single unbraced statement. A block lowers as its statements;
// a lone statement is lowered on its own and wrapped in a Go block, since a Go
// for always takes a block. This is what lets a one-line loop like
// `for (...) xs.push(f(i));` lower without the source having to add braces.
func (r *Renderer) loopBody(n frontend.Node) (*ast.BlockStmt, error) {
	if n.Kind() == frontend.NodeBlock {
		return r.lowerBlock(n)
	}
	stmts, err := r.lowerStatementMulti(n)
	if err != nil {
		return nil, err
	}
	return &ast.BlockStmt{List: stmts}, nil
}

// lowerIf lowers an if, with an optional else that is itself a block or a
// chained else-if. The condition must be a boolean expression, so a truthy
// number or object condition (JavaScript coercion) hands back until the truthy
// slice lands.
func (r *Renderer) lowerIf(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) < 2 {
		return nil, &NotYetLowerable{Reason: "if statement did not expose a condition and body"}
	}
	cond, err := r.lowerCondition(kids[0])
	if err != nil {
		return nil, err
	}
	body, err := r.lowerBodyBlock(kids[1])
	if err != nil {
		return nil, err
	}
	stmt := &ast.IfStmt{Cond: cond, Body: body}
	if len(kids) >= 3 {
		els, err := r.lowerArm(kids[2])
		if err != nil {
			return nil, err
		}
		stmt.Else = els
	}
	return stmt, nil
}

// lowerArm lowers one arm of an if: a block, a chained if for an else-if, or a
// single unbraced statement such as `else return x`, which becomes a one
// statement block so the Go if keeps its required block form.
func (r *Renderer) lowerArm(n frontend.Node) (ast.Stmt, error) {
	switch n.Kind() {
	case frontend.NodeBlock:
		return r.lowerBlock(n)
	case frontend.NodeIfStatement:
		return r.lowerIf(n)
	default:
		return r.lowerBodyBlock(n)
	}
}

// lowerBodyBlock lowers a statement that stands where a block is expected: an if
// or loop body written with braces stays a block, while a single unbraced
// statement (`if (c) return x;`) becomes a one statement block, since a Go if or
// for body is always a block. It is how the lowerer accepts the brace-optional
// bodies JavaScript allows without a special case at every use.
func (r *Renderer) lowerBodyBlock(n frontend.Node) (*ast.BlockStmt, error) {
	if n.Kind() == frontend.NodeBlock {
		return r.lowerBlock(n)
	}
	stmts, err := r.lowerStatementMulti(n)
	if err != nil {
		return nil, err
	}
	return &ast.BlockStmt{List: stmts}, nil
}

// lowerWhile lowers a while to a Go for with only a condition, Go's spelling of
// the same loop. The condition must be boolean, as for an if.
func (r *Renderer) lowerWhile(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return nil, &NotYetLowerable{Reason: "while statement did not expose a condition and body"}
	}
	cond, err := r.lowerCondition(kids[0])
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(kids[1])
	if err != nil {
		return nil, err
	}
	return &ast.ForStmt{Cond: cond, Body: body}, nil
}

// lowerCondition lowers a control-flow condition to a Go bool. A boolean operand
// lowers as itself; a non-boolean rides JavaScript truthiness through lowerTruthy,
// which reproduces the falsy set for its type, so if (n), while (s), and a ternary
// condition over a number or string lower the way the source reads them.
func (r *Renderer) lowerCondition(n frontend.Node) (ast.Expr, error) {
	return r.lowerTruthy(n)
}
