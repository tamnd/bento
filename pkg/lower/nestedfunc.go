package lower

import (
	"fmt"
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
//
// A helper the moving body reads that is itself declared below that point comes
// along to the same statement, since the same question has the same answer for it.
// A local the body reads that is declared below that point does not, and there is
// nowhere left to move to.
//
// If the early read only passes the function along as a value, the binding it needs
// is a forwarder: `name` is assigned at the top of the block a closure that does
// nothing but call `nameImpl`, and `nameImpl` takes the real body at the
// declaration's own position, where every local it reads exists. The value handed
// out early is the one that gets called later, which is what the source promised.
// Only an early call is left over, since the forwarder would find no body yet, and
// that hands back.
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
//
// thunk and implName carry the forwarder form, the answer for a body that reads a
// local declared below the statement the binding would have to move to: the local
// the source names holds a closure that calls implName, and implName takes the real
// body at the declaration's own position. needMove lists the sibling function
// declarations this one reads that sit below its destination and so come along to
// it, and insertIdx is that destination's index among the siblings, which is how two
// moves of the same declaration settle on the earlier one.
type nestedFuncPlan struct {
	goName       string
	recursive    bool
	hoisted      bool
	insertBefore frontend.Node
	insertIdx    int
	moved        bool
	emitted      bool
	used         bool
	thunk        bool
	implName     string
	needMove     []frontend.Node
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
	// A closure that moved up the block reads siblings declared below where it landed,
	// and those come along to the same statement so they hold their bodies by the time
	// it runs. Their plans only exist now, once every declaration has been walked, so
	// this is the earliest the move can say so. A declaration two moves want settles on
	// the earlier destination, which is the one both readers see.
	for _, n := range r.nestedFuncOrder {
		plan := r.nestedFuncPlans[n]
		if !plan.moved {
			continue
		}
		for _, dep := range plan.needMove {
			dp, ok := r.nestedFuncPlans[dep]
			if !ok {
				continue
			}
			dp.hoisted = true
			if !dp.moved || plan.insertIdx < dp.insertIdx {
				dp.moved, dp.insertBefore, dp.insertIdx = true, plan.insertBefore, plan.insertIdx
			}
		}
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
	thunk := false
	insertIdx := 0
	var insertBefore frontend.Node
	var needMove []frontend.Node
	for j := range idx {
		if !r.subtreeReferencesSymbol(siblings[j], sym) {
			continue
		}
		hoisted = true
		if moved || thunk || !r.eagerlyReferencesSymbol(siblings[j], sym, false) {
			continue
		}
		// The closure binds where the block has only run the statements above this
		// sibling, so a name declared at or below it is a Go local that does not exist
		// yet. A sibling function declaration is not one of those on its own account,
		// because it can come along to the same place; the rest are, and they are what
		// decides between moving the closure and forwarding to it.
		ok, need := r.movableTo(fn, siblings, j, map[frontend.Node]bool{})
		if ok {
			moved, insertBefore, insertIdx, needMove = true, siblings[j], j, need
			continue
		}
		// The forwarder needs a body by the time it is called, and it is assigned one at
		// the declaration's own position, below here. So a sibling that hands the name
		// along as a value is fine and one that calls it is not, and the call hands back.
		if r.anySiblingEagerlyCalls(siblings, idx, sym) {
			return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested function called before its declaration needs hoisting, a later slice"}
		}
		thunk = true
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
		insertIdx:    insertIdx,
		moved:        moved,
		used:         used,
		thunk:        thunk,
		implName:     goName + "Impl",
		needMove:     needMove,
	}, nil
}

// movableTo reports whether a nested function declaration's binding can move to
// sibling index from, and returns the sibling declarations that have to come along
// with it. A closure captures Go locals lexically, so it can only go somewhere every
// name it reads already exists, and the siblings from that index onward are the ones
// that do not.
//
// A sibling function declaration is the case that does not settle it on its own,
// because it can move to the same place. So it is asked the same question in turn,
// and comes along when the answer is yes. Moving it rather than only declaring it at
// the top of the block is what makes the order safe whatever the moved body does
// with it: a helper handed to a call that runs it on the spot has its body by then,
// where a bare declaration would have left nil.
//
// Anything else a sibling declares, a variable or a class, stops the move. The scan
// is over declared symbols rather than text, so an inner block's own local spelling
// the same name does not count, and seen breaks the cycle two mutually referring
// helpers would otherwise make.
func (r *Renderer) movableTo(fn frontend.Node, siblings []frontend.Node, from int, seen map[frontend.Node]bool) (bool, []frontend.Node) {
	if seen[fn] {
		return true, nil
	}
	seen[fn] = true
	body, ok := r.funcBodyBlock(fn)
	if !ok {
		return false, nil
	}
	selfSym, _ := r.prog.SymbolAt(fn)
	var need []frontend.Node
	for j := from; j < len(siblings); j++ {
		s := siblings[j]
		if s == fn {
			continue
		}
		for _, sym := range r.statementDeclaredSymbols(s) {
			if sym == selfSym || !r.subtreeReferencesSymbol(body, sym) {
				continue
			}
			if s.Kind() != frontend.NodeFunctionDeclaration {
				return false, nil
			}
			ok, more := r.movableTo(s, siblings, from, seen)
			if !ok {
				return false, nil
			}
			need = append(need, s)
			need = append(need, more...)
		}
	}
	return true, need
}

// anySiblingEagerlyCalls reports whether any of the siblings before index before
// calls sym while the block runs, as opposed to handing it along as a value. It is
// what separates the two shapes that read a nested function before its declaration:
// a value read is satisfied by a forwarder assigned at the top of the block, and a
// call is not, because the forwarder has no body to call yet.
func (r *Renderer) anySiblingEagerlyCalls(siblings []frontend.Node, before int, sym frontend.Symbol) bool {
	for j := 0; j < before; j++ {
		if r.eagerlyCallsSymbol(siblings[j], sym, false) {
			return true
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

// eagerlyCallsSymbol reports whether the subtree calls sym at a point that runs
// while the enclosing block runs. It walks the same way eagerlyReferencesSymbol
// does, and asks a narrower question: not whether the name is read there, but
// whether what is read there is immediately invoked.
func (r *Renderer) eagerlyCallsSymbol(n frontend.Node, sym frontend.Symbol, invoked bool) bool {
	if isFunctionLike(n.Kind()) && !invoked {
		return false
	}
	call := n.Kind() == frontend.NodeCallExpression || n.Kind() == frontend.NodeNewExpression
	kids := r.prog.Children(n)
	if call && len(kids) != 0 && r.namesSymbol(kids[0], sym) {
		return true
	}
	for i, c := range kids {
		invokedChild := call && i == 0
		if n.Kind() == frontend.NodeParenthesizedExpression {
			invokedChild = invoked
		}
		if r.eagerlyCallsSymbol(c, sym, invokedChild) {
			return true
		}
	}
	return false
}

// namesSymbol reports whether an expression is exactly the identifier bound to sym,
// looking through the parentheses a callee is often written in.
func (r *Renderer) namesSymbol(n frontend.Node, sym frontend.Symbol) bool {
	for n.Kind() == frontend.NodeParenthesizedExpression {
		kids := r.prog.Children(n)
		if len(kids) == 0 {
			return false
		}
		n = kids[0]
	}
	if n.Kind() != frontend.NodeIdentifier {
		return false
	}
	s, ok := r.prog.SymbolAt(n)
	return ok && s == sym
}

// forwardingFuncLit builds the closure a forwarded nested function binds at the top
// of its block: it takes the same parameters as the real body and does nothing but
// pass them to implName, so the value handed out before the declaration is the one
// that runs the body once the declaration has assigned it. The parameters are named
// afresh rather than reused, since the closure's own body never reads them by the
// source's names and a blank or shadowing name would not forward.
func forwardingFuncLit(implName string, ft *ast.FuncType) *ast.FuncLit {
	params := &ast.FieldList{}
	var args []ast.Expr
	variadic := false
	next := 0
	if ft.Params != nil {
		for _, f := range ft.Params.List {
			count := len(f.Names)
			if count == 0 {
				count = 1
			}
			var names []*ast.Ident
			for range count {
				name := fmt.Sprintf("a%d", next)
				next++
				names = append(names, ident(name))
				args = append(args, ident(name))
			}
			params.List = append(params.List, &ast.Field{Names: names, Type: f.Type})
			if _, ok := f.Type.(*ast.Ellipsis); ok {
				variadic = true
			}
		}
	}
	call := &ast.CallExpr{Fun: ident(implName), Args: args}
	if variadic {
		call.Ellipsis = token.Pos(1)
	}
	var body ast.Stmt = &ast.ExprStmt{X: call}
	if ft.Results != nil && len(ft.Results.List) != 0 {
		body = &ast.ReturnStmt{Results: []ast.Expr{call}}
	}
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: ft.Results},
		Body: &ast.BlockStmt{List: []ast.Stmt{body}},
	}
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
	varDeclNamed := func(name string) ast.Stmt {
		return &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ident(name)}, Type: funcLit.Type}}}}
	}
	varDecl := func() ast.Stmt { return varDeclNamed(plan.goName) }
	switch {
	case plan.thunk:
		// Both bindings and the forwarder go to the top of the block, since that is where
		// the read that could not wait for this position happens. Only the real body
		// stays here, which is the point: it is what reads the locals above it.
		r.nestedFuncHoists = append(r.nestedFuncHoists,
			varDeclNamed(plan.goName),
			varDeclNamed(plan.implName),
			&ast.AssignStmt{Lhs: []ast.Expr{ident(plan.goName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{forwardingFuncLit(plan.implName, funcLit.Type)}},
		)
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(plan.implName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{funcLit}})
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
