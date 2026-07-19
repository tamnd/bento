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
// path a named function expression's recursive call takes. A call that sits before
// the declaration would read the Go local before it is bound, which does not
// compile, so a forward reference hands the whole unit back to the engine rather
// than emit code Go rejects; the same holds for a name that collides with an
// enclosing parameter, a `this` read whose binding a plain closure does not carry,
// and the async, generator, generic, and callable-object shapes the top-level path
// also declines.

// nestedFuncPlan records how one nested function declaration lowers: the Go local
// it binds, whether its own body calls it (so the two-step var-first form is used
// instead of a plain :=), and whether anything reads the local (so an unread helper
// gets a blank assignment rather than trip Go's declared-and-not-used).
type nestedFuncPlan struct {
	goName    string
	recursive bool
	used      bool
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
	restore := func() {
		for _, s := range undo {
			if s.had {
				r.funcExprSelf[s.sym] = s.prev
			} else {
				delete(r.funcExprSelf, s.sym)
			}
			delete(r.nestedFuncPlans, s.node)
		}
	}
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
		undo = append(undo, saved{sym: sym, prev: prev, had: had, node: n})
	}
	if len(undo) == 0 {
		return noop, nil
	}
	return restore, nil
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
	body, _ := r.funcBodyBlock(fn)
	if subtreeHasKind(r.prog, fn, frontend.NodeThisKeyword) {
		return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested function that reads this needs its own this binding, a later slice"}
	}
	// A sibling that sits before this declaration and reads its name is a call before
	// the Go local is bound, which does not compile, so the whole unit hands back and
	// runs on the engine where the source's hoisting holds.
	for j := range idx {
		if r.subtreeReferencesSymbol(siblings[j], sym) {
			return frontend.Symbol{}, nil, &NotYetLowerable{Reason: "a nested function called before its declaration needs hoisting, a later slice"}
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
	return sym, &nestedFuncPlan{goName: goName, recursive: recursive, used: used}, nil
}

// lowerNestedFuncDecl emits a nested function declaration enterNestedFuncScope
// registered. It builds the closure the same way a block-bodied arrow does, then
// binds it to the Go local: a plain `name := closure` when the body does not call
// itself, or the two-step `var name funcType; name = closure` when it does, since a
// Go func literal cannot name itself. An unread helper takes a trailing blank
// assignment so the binding does not trip declared-and-not-used. It reports ok
// false for a declaration the pass did not register, so the caller falls to the
// existing handback.
func (r *Renderer) lowerNestedFuncDecl(fn frontend.Node) ([]ast.Stmt, bool, error) {
	plan, ok := r.nestedFuncPlans[fn]
	if !ok {
		return nil, false, nil
	}
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
	if plan.recursive {
		out = append(out,
			&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ident(plan.goName)}, Type: funcLit.Type}}}},
			&ast.AssignStmt{Lhs: []ast.Expr{ident(plan.goName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{funcLit}},
		)
	} else {
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(plan.goName)}, Tok: token.DEFINE, Rhs: []ast.Expr{funcLit}})
	}
	if !plan.used {
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ident(plan.goName)}})
	}
	return out, true, nil
}
