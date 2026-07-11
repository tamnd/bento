package lower

import (
	"go/ast"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// An async function (async function f()) returns a promise: its body runs and its
// completion settles the promise the caller awaits or chains. bento lowers an
// await-free async body the way asyncMethodDecl does for a method, wrapping the body
// in value.Async so a normal return resolves the promise and a thrown value rejects
// it. This file builds the free-standing async function forms, the declaration, the
// expression, and the arrow, that mirror the class-method async lowering in
// classes.go and share its asyncBody core.

// isAsyncFunc reports whether a function-like node carries the async modifier, the
// async keyword the parser leaves as the leading word of the node's source text. It
// is read the same way the class-method scan reads a method's modifiers, off the
// declaration text rather than a distinct node kind, since the shim folds the
// modifier into the leading token.
func (r *Renderer) isAsyncFunc(fn frontend.Node) bool {
	return firstWord(strings.TrimSpace(r.prog.Text(fn))) == "async"
}

// asyncFuncDecl lowers a top-level async function declaration to a package function
// returning a settled promise: func F(params) *value.Promise[T] { return
// value.Async(func() T { <body> }) }. It is the free-function form of
// asyncStaticFuncDecl, sharing asyncBody so the body wraps in value.Async (or
// value.AsyncVoid for a Promise<void>) exactly as a static async method does. A
// generic or rest-parameter async function is a later slice, the same boundary the
// method form draws.
func (r *Renderer) asyncFuncDecl(fn frontend.Node, sig frontend.Signature, name string) (*ast.FuncDecl, error) {
	if len(sig.TypeParams) != 0 {
		return nil, &NotYetLowerable{Reason: "a generic async function needs monomorphization, a later slice"}
	}
	if sig.RestParam != nil {
		return nil, &NotYetLowerable{Reason: "an async function with a rest parameter is a later slice"}
	}
	params, err := r.funcParamFields(fn, sig)
	if err != nil {
		return nil, err
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return nil, err
	}
	body, err := r.asyncBody(sig.Return, fn)
	if err != nil {
		return nil, err
	}
	return &ast.FuncDecl{
		Name: ident(name),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}, nil
}

// asyncFuncExpr lowers an async function expression, the async function(){} form
// used as a value, to a closure returning a settled promise. A function expression
// always has a block body, so it reuses asyncBody the way the declaration form does;
// the caller has already ruled out a body that reads this, which a Go closure carries
// no receiver for. fields are the closure's parameters, built by the shared
// closureParamFields.
func (r *Renderer) asyncFuncExpr(n frontend.Node, fields []*ast.Field) (ast.Expr, error) {
	sig, ok := r.prog.SignatureAt(n)
	if !ok {
		return nil, &NotYetLowerable{Reason: "an async function expression has no call signature"}
	}
	if len(sig.TypeParams) != 0 {
		return nil, &NotYetLowerable{Reason: "a generic async function expression needs monomorphization, a later slice"}
	}
	if sig.RestParam != nil {
		return nil, &NotYetLowerable{Reason: "an async function expression with a rest parameter is a later slice"}
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return nil, err
	}
	body, err := r.asyncBody(sig.Return, n)
	if err != nil {
		return nil, err
	}
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{List: fields}, Results: results},
		Body: body,
	}, nil
}

// asyncArrow lowers an async arrow function to a closure returning a settled promise.
// An arrow takes either a block body, which reuses asyncBody, or a concise body, a
// single expression the arrow returns; the concise form wraps that one expression in
// value.Async (or value.AsyncVoid for a Promise<void>) directly, since there is no
// block for blockOf to lower. fields are the arrow's parameters.
func (r *Renderer) asyncArrow(n frontend.Node, fields []*ast.Field) (ast.Expr, error) {
	sig, ok := r.prog.SignatureAt(n)
	if !ok {
		return nil, &NotYetLowerable{Reason: "an async arrow has no call signature"}
	}
	if len(sig.TypeParams) != 0 {
		return nil, &NotYetLowerable{Reason: "a generic async arrow needs monomorphization, a later slice"}
	}
	if sig.RestParam != nil {
		return nil, &NotYetLowerable{Reason: "an async arrow with a rest parameter is a later slice"}
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return nil, err
	}
	kids := r.prog.Children(n)
	bodyNode := kids[len(kids)-1]
	var body *ast.BlockStmt
	if bodyNode.Kind() == frontend.NodeBlock {
		body, err = r.asyncBody(sig.Return, n)
	} else {
		body, err = r.asyncConciseBody(sig.Return, bodyNode)
	}
	if err != nil {
		return nil, err
	}
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{List: fields}, Results: results},
		Body: body,
	}, nil
}

// blockHasAwait reports whether n contains an await expression that belongs to the
// current async body, descending its statements but stopping at a nested function,
// whose own awaits belong to its own async body. It is the await-side mirror of
// collectYields: it decides whether a body suspends, which routes it to the coroutine
// path instead of the synchronous value.Async wrapping.
func (r *Renderer) blockHasAwait(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeFunctionDeclaration, frontend.NodeFunctionExpression, frontend.NodeArrowFunction,
		frontend.NodeMethodDeclaration, frontend.NodeGetAccessor, frontend.NodeSetAccessor, frontend.NodeConstructor:
		return false
	case frontend.NodeAwaitExpression:
		return true
	}
	for _, k := range r.prog.Children(n) {
		if r.blockHasAwait(k) {
			return true
		}
	}
	return false
}

// bodyHasAwait reports whether the block body of fn awaits, so an async function whose
// body suspends lowers through the coroutine rather than the synchronous value.Async
// path. A concise-bodied arrow has no block for funcBodyBlock to find and reports
// false, so it keeps the synchronous path; an await in a concise body is not lowered
// yet and hands back cleanly at the await site.
func (r *Renderer) bodyHasAwait(fn frontend.Node) bool {
	block, ok := r.funcBodyBlock(fn)
	if !ok {
		return false
	}
	return r.blockHasAwait(block)
}

// asyncCoroutineBody lowers an async body that awaits into the single return that mints
// its pending promise: return value.RunAsync[T](func(_co *value.AsyncCo) T { <body> })
// for a valued promise, or value.RunAsyncVoid(func(_co *value.AsyncCo) { <body> }) for a
// Promise<void>. The body lowers with the coroutine handle in scope, so each await in it
// routes to awaitExpr and suspends on _co; the body's returns carry the element type T
// the coroutine fulfills the promise with. It is the suspending counterpart of asyncBody,
// which asyncBody dispatches to when the body awaits.
func (r *Renderer) asyncCoroutineBody(ret frontend.Type, retNode frontend.Node) (*ast.BlockStmt, error) {
	elem, ok := r.promiseElem(ret)
	if !ok {
		return nil, &NotYetLowerable{Reason: "an async function whose return is not a Promise is a later slice"}
	}
	coName := r.freshTemp()
	prevCo := r.asyncCo
	r.asyncCo = coName
	defer func() { r.asyncCo = prevCo }()

	prevRet := r.retType
	if isVoidReturn(elem) {
		r.retType = frontend.Type{}
	} else {
		r.retType = elem
	}
	defer func() { r.retType = prevRet }()

	inner, err := r.blockOf(retNode)
	if err != nil {
		return nil, err
	}
	r.usesPromise = true
	r.requireImport(valuePkg)

	coParam := &ast.Field{
		Names: []*ast.Ident{ident(coName)},
		Type:  star(sel("value", "AsyncCo")),
	}
	if isVoidReturn(elem) {
		lit := &ast.FuncLit{
			Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{coParam}}},
			Body: inner,
		}
		call := &ast.CallExpr{Fun: sel("value", "RunAsyncVoid"), Args: []ast.Expr{lit}}
		return &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{call}}}}, nil
	}
	et, err := r.typeExpr(elem)
	if err != nil {
		return nil, err
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{coParam}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: et}}},
		},
		Body: inner,
	}
	call := &ast.CallExpr{Fun: index(sel("value", "RunAsync"), et), Args: []ast.Expr{lit}}
	return &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{call}}}}, nil
}

// awaitExpr lowers an await expression to a suspend on the current async body's
// coroutine handle. Awaiting a promise lowers to value.Await(_co, p), which parks the
// body until p settles, then resumes it with the fulfilled value or raises the
// rejection at the await. Awaiting a definite non-thenable primitive lowers to
// value.AwaitValue(_co, v), which JavaScript wraps in a resolved promise: it defers one
// microtask turn and hands the value straight back. Awaiting a value that might be a
// thenable (an object with a then method a real await adopts) or a dynamic value whose
// shape is hidden hands back, as does an await outside a lowered async body, which has
// no handle to park on.
func (r *Renderer) awaitExpr(n frontend.Node) (ast.Expr, error) {
	if r.asyncCo == "" {
		return nil, &NotYetLowerable{Reason: "an await outside a lowered async body is a later slice"}
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		return nil, &NotYetLowerable{Reason: "an await with no operand is a later slice"}
	}
	operand := kids[len(kids)-1]
	opType := r.prog.TypeAt(operand)
	if _, ok := r.promiseElem(opType); ok {
		p, err := r.lowerExpr(operand)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "Await"), Args: []ast.Expr{ident(r.asyncCo), p}}, nil
	}
	if !r.isDefiniteNonThenable(opType) {
		return nil, &NotYetLowerable{Reason: "an await on a possibly-thenable or dynamic value is a later slice"}
	}
	v, err := r.lowerExpr(operand)
	if err != nil {
		return nil, err
	}
	// The element type is pinned as an explicit type argument so an untyped operand (a
	// numeric literal defaulting to int) crosses to the value's Go type the await
	// expression carries, the same type the surrounding code reads the await as.
	et, err := r.typeExpr(opType)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: index(sel("value", "AwaitValue"), et), Args: []ast.Expr{ident(r.asyncCo), v}}, nil
}

// isDefiniteNonThenable reports whether t is a value that certainly carries no then
// method, so awaiting it only defers a microtask and yields the value straight back. A
// primitive number, string, boolean, or bigint qualifies; an object might be a thenable,
// and any, unknown, a union, an intersection, or a type parameter hide the shape, so
// none of those do.
func (r *Renderer) isDefiniteNonThenable(t frontend.Type) bool {
	if t.Flags&(frontend.TypeObject|frontend.TypeAny|frontend.TypeUnknown|frontend.TypeUnion|frontend.TypeIntersection|frontend.TypeTypeParameter) != 0 {
		return false
	}
	return t.Flags&(frontend.TypeNumber|frontend.TypeString|frontend.TypeBoolean|frontend.TypeBigInt) != 0
}

// asyncConciseBody mints the promise for a concise-bodied async arrow, whose body is
// a single expression rather than a block. It wraps that expression the way asyncBody
// wraps a block: value.Async(func() T { return <expr> }) for a valued promise, or
// value.AsyncVoid(func() { <expr> }) for a Promise<void>, where a void body must be a
// call so it stands in Go statement position. The expression coerces to the promise's
// element type the same way a block body's return does.
func (r *Renderer) asyncConciseBody(ret frontend.Type, bodyNode frontend.Node) (*ast.BlockStmt, error) {
	elem, ok := r.promiseElem(ret)
	if !ok {
		return nil, &NotYetLowerable{Reason: "an async arrow whose return is not a Promise is a later slice"}
	}
	r.usesPromise = true
	r.requireImport(valuePkg)
	if isVoidReturn(elem) {
		expr, err := r.lowerExpr(bodyNode)
		if err != nil {
			return nil, err
		}
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			return nil, &NotYetLowerable{Reason: "an async arrow with a void concise body that is not a call is a later slice"}
		}
		lit := &ast.FuncLit{
			Type: &ast.FuncType{Params: &ast.FieldList{}},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: call}}},
		}
		wrap := &ast.CallExpr{Fun: sel("value", "AsyncVoid"), Args: []ast.Expr{lit}}
		return &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{wrap}}}}, nil
	}
	et, err := r.typeExpr(elem)
	if err != nil {
		return nil, err
	}
	prevRet := r.retType
	r.retType = elem
	defer func() { r.retType = prevRet }()
	expr, err := r.lowerExpr(bodyNode)
	if err != nil {
		return nil, err
	}
	expr, err = r.coerceToType(expr, bodyNode, elem)
	if err != nil {
		return nil, err
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: et}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{expr}}}},
	}
	wrap := &ast.CallExpr{Fun: sel("value", "Async"), Args: []ast.Expr{lit}}
	return &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{wrap}}}}, nil
}
