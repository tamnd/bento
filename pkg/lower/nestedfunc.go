package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers a function declaration nested inside another function's body,
// the helper a routine declares for its own use (`function outer() { function
// step(x) { ... } return step(1); }`). JavaScript hoists such a declaration to the
// top of its enclosing scope, so a call may sit anywhere in the block; Go has no
// hoisted local function, only a func value bound to a local, which is in scope
// from its declaration onward. The lowering emits the closure at the declaration's
// textual position and routes every call to the Go local, the same funcExprSelf
// path a named function expression's recursive call takes.
//
// An earlier sibling that names the declaration is the hoisting question, and the
// answer turns on when that name is read.
//
// Almost always it is read from inside another function's body, which runs after
// the block has finished, so the binding only has to exist by then: declaring `var
// name funcType` at the top of the Go block and assigning the closure at the
// declaration's own position gives that, and it is what lets a file of mutually
// referring helpers lower at all.
//
// A read that runs while the block itself runs needs more, because the Go local
// would still be nil there where JavaScript already has the function. So the
// closure's binding moves up to just before the statement that reads it, which is
// as early as it can go while still capturing only names Go has already declared.
// If the body reads something declared at or after that point, the move would
// capture a Go local that does not exist yet, and the unit hands back instead.
//
// The same holds for a name that collides with an enclosing parameter, a `this`
// read whose binding a plain closure does not carry, and the async, generator,
// generic, and callable-object shapes the top-level path also declines.

// nestedFuncPlan records how one nested function declaration lowers: the Go local
// it binds, whether its own body calls it (so the two-step var-first form is used
// instead of a plain :=), whether an earlier sibling names it (so the declaration
// goes to the top of the block and the closure binds by assignment), which sibling
// the assignment moves up to when one of them reads the name while the block runs,
// whether that move has already emitted it so its own position emits nothing, and
// whether anything reads the local (so an unread helper gets a blank assignment
// rather than trip Go's declared-and-not-used).
type nestedFuncPlan struct {
	goName       string
	recursive    bool
	hoisted      bool
	insertBefore frontend.Node
	moved        bool
	emitted      bool
	used         bool
}

// paramNameSet gives the set of Go parameter names a signature binds, the guard a
// nested function declaration checks its own Go name against so it does not collide
// with an enclosing parameter in the same Go block. A rest parameter binds a name
// too, so it joins the set. The set is non-nil even when empty, since a non-nil map
// is what switches the nested-function pass on for a body.
func (r *Renderer) paramNameSet(sig frontend.Signature) map[string]bool {
	set := make(map[string]bool, len(sig.Params)+1)
	for _, p := range sig.Params {
		if name, ok := localName(p.Name); ok {
			set[name] = true
		}
	}
	if sig.RestParam != nil {
		if name, ok := localName(sig.RestParam.Name); ok {
			set[name] = true
		}
	}
	return set
}

// pushScopeParams sets the enclosing-parameter name set for a body about to lower
// and returns a restore, so a nested function declaration inside it can vet its Go
// name against the parameters and the pass switches on for the body. It mirrors
// pushOptParams and pushDynBound so a body-entry site reads uniformly.
func (r *Renderer) pushScopeParams(sig frontend.Signature) func() {
	prev := r.scopeParams
	r.scopeParams = r.paramNameSet(sig)
	return func() { r.scopeParams = prev }
}

// enterNestedFuncScope registers every nested function declaration among a block's
// statements before any of them lowers, so a sibling call resolves to the Go local
// the declaration binds wherever in the block it sits. It mirrors the forward-hoist
// scope: it records the routing in funcExprSelf and the emission plan keyed by node,
// and returns a restore that clears both. It runs only for a body that populated
// scopeParams (a plain function or a block-bodied closure), so a method, generator,
// async, or constructor body leaves its nested declarations to the older handback.
// A nested declaration outside the lowerable subset makes the whole call return an
// error, which hands the unit back rather than emit a partial block.
func (r *Renderer) enterNestedFuncScope(nodes []frontend.Node) (func(), error) {
	noop := func() {}
	if r.scopeParams == nil {
		return noop, nil
	}
	type saved struct {
		sym  frontend.Symbol
		prev string
		had  bool
		node frontend.Node
	}
	var undo []saved
	prevOrder := r.nestedFuncOrder
	restore := func() {
		for _, s := range undo {
			if s.had {
				r.funcExprSelf[s.sym] = s.prev
			} else {
				delete(r.funcExprSelf, s.sym)
			}
			delete(r.nestedFuncPlans, s.node)
		}
		r.nestedFuncOrder = prevOrder
	}
	r.nestedFuncOrder = nil
	for i, n := range nodes {
		if n.Kind() != frontend.NodeFunctionDeclaration {
			continue
		}
		sym, plan, err := r.nestedFuncLowerable(n, nodes, i)
		if err != nil {
			restore()
			return noop, err
		}
		prev, had := r.funcExprSelf[sym]
		r.funcExprSelf[sym] = plan.goName
		if r.nestedFuncPlans == nil {
			r.nestedFuncPlans = map[frontend.Node]*nestedFuncPlan{}
		}
		r.nestedFuncPlans[n] = plan
		r.nestedFuncOrder = append(r.nestedFuncOrder, n)
		undo = append(undo, saved{sym: sym, prev: prev, had: had, node: n})
	}
	if len(undo) == 0 {
		r.nestedFuncOrder = prevOrder
		return noop, nil
	}
	return restore, nil
}

// emitMovedNestedFuncs emits the nested function declarations whose binding moves up
// to just before stmt, in the order they are declared, and returns the statements to
// splice in ahead of it. Every block that lowers a statement list calls it before
// each statement, so a helper a statement reads while the block runs is already bound
// by the time that statement does. A block with nothing moved returns nothing, which
// is every block that does not have a forward call in it.
func (r *Renderer) emitMovedNestedFuncs(stmt frontend.Node) ([]ast.Stmt, error) {
	var out []ast.Stmt
	for _, fn := range r.nestedFuncOrder {
		plan, ok := r.nestedFuncPlans[fn]
		if !ok || !plan.moved || plan.emitted || plan.insertBefore != stmt {
			continue
		}
		stmts, ok, err := r.lowerNestedFuncDecl(fn)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, stmts...)
		}
	}
	return out, nil
}

// nestedFuncLowerable decides whether one nested function declaration lowers to a
// closure bound to a Go local, and if so returns its symbol and emission plan. It
// declines the same shapes the top-level path declines (async, generator, generic,
// callable object, a name that is not a Go identifier) plus the two a nested binding
// adds: a call that sits before the declaration, which would read the Go local
// before it is bound, and a name that collides with an enclosing parameter, which
// would redeclare it. A `this` read is declined too, since a plain closure does not
// carry the enclosing `this` the way the source's hoisted function would.
func (r *Renderer) nestedFuncLowerable(fn frontend.Node, siblings []frontend.Node, idx int) (frontend.Symbol, *nestedFuncPlan, error) {
	sym, ok := r.prog.SymbolAt(fn)
	if !ok {
		return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested function declaration with no symbol is a later slice"}
	}
	nameNode, ok := r.funcExprNameNode(fn)
	if !ok {
		return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested function declaration with no name is a later slice"}
	}
	goName, ok := localName(r.prog.Text(nameNode))
	if !ok {
		return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested function name that is not a Go identifier is a later slice"}
	}
	if r.isAsyncFunc(fn) {
		return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested async function declaration is a later slice"}
	}
	if r.isGeneratorFunc(fn) {
		return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested generator function declaration is a later slice"}
	}
	sig, ok := r.prog.SignatureAt(fn)
	if !ok {
		return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested function with no call signature is a later slice"}
	}
	if len(sig.TypeParams) != 0 {
		return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested generic function declaration needs monomorphization, a later slice"}
	}
	if r.isCallableObject(r.prog.TypeAt(fn)) {
		return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested function with own properties is a callable object, a later slice"}
	}
	if r.scopeParams[goName] {
		return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested function whose name collides with an enclosing parameter is a later slice"}
	}
	// A nested function overload signature carries no body block, only a later
	// implementation does. There is nothing to lower for the signature and its body
	// scan below would read a nil node, so the whole unit hands back and runs on the
	// engine where the overload set resolves.
	body, ok := r.funcBodyBlock(fn)
	if !ok {
		return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested function overload signature has no body to lower, a later slice"}
	}
	if subtreeHasKind(r.prog, fn, frontend.NodeThisKeyword) {
		return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested function that reads this needs its own this binding, a later slice"}
	}
	// A sibling above this declaration that names it decides where the binding goes.
	// If it only names it from inside a function body, the read happens after the
	// block has run, so declaring the Go local at the top of the block and assigning
	// the closure at this position is enough. If the read runs while the block runs,
	// the assignment moves up to just before that sibling, which is the earliest point
	// that still sees everything the source declared above it.
	hoisted := false
	moved := false
	var insertBefore frontend.Node
	for j := range idx {
		if !r.subtreeReferencesSymbol(siblings[j], sym) {
			continue
		}
		hoisted = true
		if !moved && r.eagerlyReferencesSymbol(siblings[j], sym, false) {
			moved, insertBefore = true, siblings[j]
			// The closure binds where the block has only run the statements above this
			// sibling, so a name declared at or below it is a Go local that does not exist
			// yet. JavaScript's hoisting has no such limit, so this hands back rather than
			// pick between a Go build failure and a binding that reads the wrong thing.
			if r.capturesSiblingDeclaredFrom(body, siblings, j, fn, sym) {
				return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested function called before its declaration needs hoisting, a later slice"}
			}
		}
	}
	recursive := r.subtreeReferencesSymbol(body, sym)
	used := recursive
	if !used {
		for j := range siblings {
			if j == idx {
				continue
			}
			if r.subtreeReferencesSymbol(siblings[j], sym) {
				used = true
				break
			}
		}
	}
	return sym, &nestedFuncPlan{
		goName:       goName,
		recursive:    recursive,
		hoisted:      hoisted,
		insertBefore: insertBefore,
		moved:        moved,
		used:         used,
	}, nil
}

// capturesSiblingDeclaredFrom reports whether a function body reads a name one of
// the siblings from index from onward declares, skipping the function's own
// declaration and its own name. It is the guard on moving a closure's binding up the
// block: the closure captures Go locals lexically, so it can only go somewhere every
// name it reads is already declared, and a name bound at or below the destination is
// not.
//
// The scan is over declared symbols rather than text, so an inner block's own local
// spelling the same name does not count, and it covers a function declaration as well
// as a variable one, since a helper bound below the destination is exactly as absent
// there as a const is.
func (r *Renderer) capturesSiblingDeclaredFrom(body frontend.Node, siblings []frontend.Node, from int, self frontend.Node, selfSym frontend.Symbol) bool {
	for j := from; j < len(siblings); j++ {
		s := siblings[j]
		if s == self {
			continue
		}
		for _, sym := range r.statementDeclaredSymbols(s) {
			if sym == selfSym {
				continue
			}
			if r.subtreeReferencesSymbol(body, sym) {
				return true
			}
		}
	}
	return false
}

// statementDeclaredSymbols returns the symbols a block-level statement binds in the
// block: the leaves of every variable declaration in a variable statement, and the
// name of a function or class declaration. A statement that binds nothing in the
// block, an expression or a control-flow statement whose own body opens a scope of
// its own, returns nothing.
func (r *Renderer) statementDeclaredSymbols(n frontend.Node) []frontend.Symbol {
	var out []frontend.Symbol
	switch n.Kind() {
	case frontend.NodeVariableStatement:
		var decls []frontend.Node
		collectVarDecls(r.prog, n, &decls)
		for _, d := range decls {
			names, ok := r.declBindingNames(d)
			if !ok {
				continue
			}
			for _, nn := range names {
				if sym, ok := r.prog.SymbolAt(nn); ok {
					out = append(out, sym)
				}
			}
		}
	case frontend.NodeFunctionDeclaration, frontend.NodeClassDeclaration:
		if sym, ok := r.prog.SymbolAt(n); ok {
			out = append(out, sym)
		}
	}
	return out
}

// eagerlyReferencesSymbol reports whether the subtree reads sym at a point that
// runs while the enclosing block runs, rather than inside a function body that
// runs later. That distinction is the whole of the hoisting question: a helper
// named from inside another helper's body is read after the block has finished
// binding everything, which a Go local declared at the top of the block satisfies,
// while a read in statement position happens before the assignment and would find
// nil.
//
// A function the surrounding call invokes on the spot runs now, so the callee
// position of a call is walked rather than skipped. Everything else that is
// function-like is skipped whole, including a callback passed as an argument: the
// call may run it immediately, but it may also store it, and the shape is common
// enough that treating it as eager would refuse most of what this exists to admit.
// A callback a call runs before the block finishes and that reads a later helper
// is the one shape this can be wrong about, and it is rare enough to be worth the
// rest.
func (r *Renderer) eagerlyReferencesSymbol(n frontend.Node, sym frontend.Symbol, invoked bool) bool {
	if isFunctionLike(n.Kind()) && !invoked {
		return false
	}
	if n.Kind() == frontend.NodeIdentifier {
		if s, ok := r.prog.SymbolAt(n); ok && s == sym {
			return true
		}
	}
	call := n.Kind() == frontend.NodeCallExpression || n.Kind() == frontend.NodeNewExpression
	for i, c := range r.prog.Children(n) {
		// Parentheses are how an IIFE is almost always written, so they pass the callee
		// position through rather than end it. Without that the function inside them
		// reads as a body that runs later, and a helper it calls would be bound after the
		// call that runs it.
		invokedChild := call && i == 0
		if n.Kind() == frontend.NodeParenthesizedExpression {
			invokedChild = invoked
		}
		if r.eagerlyReferencesSymbol(c, sym, invokedChild) {
			return true
		}
	}
	return false
}

// lowerNestedFuncDecl emits a nested function declaration enterNestedFuncScope
// registered. It builds the closure the same way a block-bodied arrow does, then
// binds it to the Go local: a plain `name := closure` when the body does not call
// itself, or the two-step `var name funcType; name = closure` when it does, since a
// Go func literal cannot name itself. A declaration an earlier sibling names takes
// the same two-step form with the `var` moved to the top of the block, which is how
// the source's hoisting reaches Go; the type comes off the closure that was just
// built, so the declaration and the assignment cannot disagree about it. An unread
// helper takes a trailing blank assignment so the binding does not trip
// declared-and-not-used. It reports ok false for a declaration the pass did not
// register, so the caller falls to the existing handback.
func (r *Renderer) lowerNestedFuncDecl(fn frontend.Node) ([]ast.Stmt, bool, error) {
	plan, ok := r.nestedFuncPlans[fn]
	if !ok {
		return nil, false, nil
	}
	// A moved declaration was already emitted ahead of the statement that reads it, so
	// its own position contributes nothing. Reporting ok here rather than falling
	// through is what keeps the statement from lowering a second closure.
	if plan.emitted {
		return nil, true, nil
	}
	plan.emitted = true
	sig, ok := r.prog.SignatureAt(fn)
	if !ok {
		return nil, false, &NotYetLowerable{Reason: "a nested function with no call signature is a later slice"}
	}
	fields, err := r.closureParamFields(fn, sig, "function")
	if err != nil {
		return nil, false, err
	}
	lit, err := r.blockBodyArrow(fn, fields)
	if err != nil {
		return nil, false, err
	}
	funcLit, ok := lit.(*ast.FuncLit)
	if !ok {
		return nil, false, &NotYetLowerable{Reason: "a nested function body did not lower to a closure"}
	}
	var out []ast.Stmt
	varDecl := func() ast.Stmt {
		return &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ident(plan.goName)}, Type: funcLit.Type}}}}
	}
	switch {
	case plan.hoisted:
		r.nestedFuncHoists = append(r.nestedFuncHoists, varDecl())
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(plan.goName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{funcLit}})
	case plan.recursive:
		out = append(out,
			varDecl(),
			&ast.AssignStmt{Lhs: []ast.Expr{ident(plan.goName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{funcLit}},
		)
	default:
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(plan.goName)}, Tok: token.DEFINE, Rhs: []ast.Expr{funcLit}})
	}
	if !plan.used {
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ident(plan.goName)}})
	}
	return out, true, nil
}
