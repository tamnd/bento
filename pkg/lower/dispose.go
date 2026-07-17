package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// nestedUsingBranchReason is the hand-back a nested-block `using` reports when the
// rest of its block leaves by a break or continue that targets a loop or switch
// enclosing the whole block. That branch would lower to a Go break or continue inside
// the closure the disposal wraps the block in, which leaves the closure, not the loop
// the branch names, so the case waits for the slice that threads the branch out.
const nestedUsingBranchReason = "a using declaration whose block exits by break or continue is a later slice"

// nestedUsingReturnNestReason is the hand-back a returning nested-block `using`
// reports when it sits inside another escape closure (a returning try, or a catch or
// finally handler). The return-threading below turns a return in the remainder into
// the disposal closure's done result, which the call site hands up as the enclosing
// function's return; threading it further through a second escape closure at the same
// time is a later slice, so the nested combination hands back rather than mislower.
const nestedUsingReturnNestReason = "a using declaration that returns inside another escape closure is a later slice"

// nestedUsingRetNameReason is the hand-back a returning nested-block `using` reports
// when the remainder mentions ret or done, the names the disposal closure's escape
// results carry. A source binding or reference by either name would resolve to the
// closure's result rather than its own variable, so the collision hands back the way
// the try escape's does.
const nestedUsingRetNameReason = "a using declaration whose block names the escape results (ret, done) is a later slice"

// usingDisposeTarget qualifies a `using` node for disposal lowering, returning the
// resource's Go name and its declaration. It reports ok=false, leaving the
// declaration to hand back through lowerVarStatement, for every form the disposal
// paths do not own: an `await using` (its disposal is awaited, gated on the async
// model), a statement binding more than one resource, a name that is not a Go
// identifier, or an initializer whose type carries no [Symbol.dispose] method (a
// nullable or undefined resource, whose disposal must be guarded).
func (r *Renderer) usingDisposeTarget(n frontend.Node) (string, []frontend.Node, bool) {
	kw, ok := r.usingKeyword(n)
	if !ok || kw != "using" {
		return "", nil, false
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, n, &decls)
	if len(decls) != 1 {
		return "", nil, false
	}
	nameNode := r.prog.Children(decls[0])[0]
	name, ok := localName(r.prog.Text(nameNode))
	if !ok {
		return "", nil, false
	}
	if !r.hasSymbolDisposeMember(r.prog.TypeAt(nameNode)) {
		return "", nil, false
	}
	return name, decls, true
}

// deferDispose builds the `defer value.Dispose(func() { name.SymbolDispose() })` a
// disposal path registers so the resource releases when its Go scope exits, the
// closure's for a nested block and the enclosing function's for a top-level `using`.
// It routes the release through the runtime's Dispose rather than defer the method
// directly so a throw from disposal chains into a SuppressedError with the error the
// scope was already unwinding, the explicit-resource-management semantics Go's own
// defer, which would replace the pending panic, does not give.
func (r *Renderer) deferDispose(name string) *ast.DeferStmt {
	r.requireImport(valuePkg)
	release := &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{
			Fun: &ast.SelectorExpr{X: ident(name), Sel: ident(symbolDisposeGoName)},
		}}}},
	}
	return &ast.DeferStmt{Call: &ast.CallExpr{Fun: sel("value", "Dispose"), Args: []ast.Expr{release}}}
}

// lowerUsingDefer lowers a `using` declaration at a function-body or program-body top
// level to its binding plus a Go defer of the resource's SymbolDispose, so disposal
// runs when the enclosing function returns, the scope that coincides with the
// JavaScript block scope there. Go's defer covers every exit the block scope disposes
// on, a normal fall-through, a return, and a thrown value unwinding as a panic, and
// runs multiple defers last-registered-first, which gives the reverse declaration
// order the protocol requires for two `using` bindings in the same block.
//
// It reports ok=false for a form usingDisposeTarget does not own, leaving it to hand
// back through lowerVarStatement. A `using` in a nested block never reaches here,
// since the top-scope flag is false there; it lowers through lowerUsingScope instead.
func (r *Renderer) lowerUsingDefer(n frontend.Node) ([]ast.Stmt, bool, error) {
	name, decls, ok := r.usingDisposeTarget(n)
	if !ok {
		return nil, false, nil
	}
	bind, err := r.varDeclStmt(decls)
	if err != nil {
		return nil, false, err
	}
	return []ast.Stmt{bind, r.deferDispose(name)}, true, nil
}

// lowerUsingScope lowers a `using` declaration in a nested block, where a
// function-scoped defer would release the resource too late. It disposes at the
// block's own exit by wrapping the binding, its defer, and the rest of the block in a
// closure invoked in place: the defer runs when the closure returns, the point the
// block scope ends. rest is the statements after the `using` in the same block, and a
// second `using` among them wraps again inside this closure, so its defer runs first,
// the reverse declaration order the protocol requires.
//
// When the remainder leaves by a return, the disposal closure carries the escape out:
// it takes named results (ret T, done bool), a return in the remainder fills them as
// `return x, true` the way a returning try body does, and the call site turns done
// back into the enclosing function's return, so the resource still releases through the
// closure's defer on the early-return path. A break or continue targeting a loop
// outside the block still hands back through nestedUsingBranchReason, since Go's branch
// cannot leave the closure. It reports ok=false for a form usingDisposeTarget does not
// own, leaving it to hand back through lowerVarStatement.
func (r *Renderer) lowerUsingScope(n frontend.Node, rest []frontend.Node) ([]ast.Stmt, bool, error) {
	name, decls, ok := r.usingDisposeTarget(n)
	if !ok {
		return nil, false, nil
	}
	returns := false
	for _, k := range rest {
		if r.branchEscapesClosure(k) {
			return nil, false, &NotYetLowerable{Reason: nestedUsingBranchReason}
		}
		if r.blockReturns(k) {
			returns = true
		}
	}
	bind, err := r.varDeclStmt(decls)
	if err != nil {
		return nil, false, err
	}
	if returns {
		return r.lowerUsingScopeReturn(name, bind, rest)
	}
	body := []ast.Stmt{bind, r.deferDispose(name)}
	restStmts, err := r.lowerStatements(rest)
	if err != nil {
		return nil, false, err
	}
	body = append(body, restStmts...)
	return []ast.Stmt{&ast.ExprStmt{X: callClosure(body)}}, true, nil
}

// lowerUsingScopeReturn lowers a nested-block `using` whose remainder returns, the
// escape form of lowerUsingScope. It reuses the try escape's return threading: the
// remainder lowers under tryRetBody, so each return becomes `return x, true` (or
// `return true` for a void function), and the closure declares the matching named
// results the call site reads. A tail bare return covers the fall-through path, where
// the closure returns its zero results and the call site runs on past the block.
//
// It hands back when the `using` sits inside another escape closure, since threading a
// return through two at once is a later slice, and when the remainder mentions the ret
// or done result names, the same shadowing hazard the try escape guards.
func (r *Renderer) lowerUsingScopeReturn(name string, bind ast.Stmt, rest []frontend.Node) ([]ast.Stmt, bool, error) {
	if r.tryRet != tryRetNone {
		return nil, false, &NotYetLowerable{Reason: nestedUsingReturnNestReason}
	}
	for _, k := range rest {
		if r.mentionsName(k, "ret") || r.mentionsName(k, "done") {
			return nil, false, &NotYetLowerable{Reason: nestedUsingRetNameReason}
		}
	}
	valued := !isVoidReturn(r.retType)

	body := []ast.Stmt{bind, r.deferDispose(name)}
	prev := r.tryRet
	r.tryRet = tryRetBody
	restStmts, err := r.lowerStatements(rest)
	r.tryRet = prev
	if err != nil {
		return nil, false, err
	}
	body = append(body, restStmts...)
	// The closure's fall-through path returns its zero results; a remainder already
	// ending in a Go return needs no tail return after it.
	if len(body) == 0 || !isGoReturn(body[len(body)-1]) {
		body = append(body, &ast.ReturnStmt{})
	}

	results := &ast.FieldList{}
	if valued {
		rt, err := r.typeExpr(r.retType)
		if err != nil {
			return nil, false, err
		}
		results.List = append(results.List, &ast.Field{Names: []*ast.Ident{ident("ret")}, Type: rt})
	}
	results.List = append(results.List, &ast.Field{Names: []*ast.Ident{ident("done")}, Type: ident("bool")})
	closure := &ast.CallExpr{Fun: &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: results},
		Body: &ast.BlockStmt{List: body},
	}}

	var lhs []ast.Expr
	var thenRet *ast.ReturnStmt
	if valued {
		lhs = []ast.Expr{ident("ret"), ident("done")}
		thenRet = &ast.ReturnStmt{Results: []ast.Expr{ident("ret")}}
	} else {
		lhs = []ast.Expr{ident("done")}
		thenRet = &ast.ReturnStmt{}
	}
	return []ast.Stmt{&ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: lhs, Tok: token.DEFINE, Rhs: []ast.Expr{closure}},
		Cond: ident("done"),
		Body: &ast.BlockStmt{List: []ast.Stmt{thenRet}},
	}}, true, nil
}

// This file names the well-known dispose symbols a resource class carries, the
// entry points the explicit-resource-management protocol (`using`, `await using`)
// calls at scope exit. A `[Symbol.dispose]` method lowers to a fixed Go method the
// disposal path names without threading the class through, the way a
// `[Symbol.iterator]` member lowers for for...of.

// symbolDisposeGoName is the Go method name a [Symbol.dispose] member lowers to,
// the name a `using` declaration calls to release the resource at scope exit. It is
// fixed so the disposal path can name it without threading the class through.
const symbolDisposeGoName = "SymbolDispose"

// symbolDisposeProp is the sentinel property key the [Symbol.dispose] method carries
// in the member map. Its spelling starts with a byte no JavaScript property name
// can, so it never collides with a real member.
const symbolDisposeProp = "[Symbol.dispose]"

// symbolAsyncDisposeGoName is the Go method name a [Symbol.asyncDispose] member
// lowers to, the name an `await using` declaration awaits at scope exit, the async
// mirror of symbolDisposeGoName.
const symbolAsyncDisposeGoName = "SymbolAsyncDispose"

// symbolAsyncDisposeProp is the sentinel property key the [Symbol.asyncDispose]
// method carries in the member map, the async mirror of symbolDisposeProp.
const symbolAsyncDisposeProp = "[Symbol.asyncDispose]"

// isSymbolDisposeName reports whether a class member name node is the well-known
// [Symbol.dispose] computed name, the key a disposable class defines its release
// method under. It reads the same unnamed-node-wrapping-a-property-access shape the
// iterator name matchers do, but for Symbol.dispose.
func (r *Renderer) isSymbolDisposeName(nameNode frontend.Node) bool {
	return r.isSymbolMemberName(nameNode, "dispose")
}

// isSymbolAsyncDisposeName reports whether a class member name node is the
// well-known [Symbol.asyncDispose] computed name, the key an async disposable class
// defines its awaited release method under, the async mirror of isSymbolDisposeName.
func (r *Renderer) isSymbolAsyncDisposeName(nameNode frontend.Node) bool {
	return r.isSymbolMemberName(nameNode, "asyncDispose")
}

// symbolDisposeMemberPrefix is the mangled property-name prefix the checker gives a
// [Symbol.dispose] member: the internal-symbol prefix byte, then @dispose, then a
// per-symbol id the prefix match skips over, the same shape the iterator members
// carry. It never collides with @asyncDispose, which differs from its second byte on.
const symbolDisposeMemberPrefix = "\xFE@dispose"

// isSymbolDisposeExpr reports whether an expression node is the well-known
// Symbol.dispose property access, the key a manual `obj[Symbol.dispose]()` reference
// reads the release method through. Unlike isSymbolDisposeName, which matches the
// computed member name in a class body (an unnamed node wrapping the access), this
// matches the access expression itself, the shape it takes as an element-access index.
func (r *Renderer) isSymbolDisposeExpr(node frontend.Node) bool {
	if node.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	pa := r.prog.Children(node)
	return len(pa) == 2 && r.prog.Text(pa[0]) == "Symbol" && r.prog.Text(pa[1]) == "dispose"
}

// hasSymbolDisposeMember reports whether a type defines a [Symbol.dispose] method,
// so a manual `obj[Symbol.dispose]()` lowers to the Go SymbolDispose method only when
// the receiver actually carries it, the way the iterator manual access gates on the
// receiver being a user iterable.
func (r *Renderer) hasSymbolDisposeMember(t frontend.Type) bool {
	_, ok := r.memberByPrefix(t, symbolDisposeMemberPrefix)
	return ok
}
