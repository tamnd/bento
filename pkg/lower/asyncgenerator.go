package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// An async generator (async function* g()) lowers to a Go function that returns a running
// coroutine, the *value.AsyncGen the runtime drives (pkg/value/asyncgenerator.go). The
// body is both a generator and an async body: it yields values a consumer pulls and awaits
// promises between yields, so its single coroutine handle serves both. A yield in the body
// lowers to _co.Yield the way a plain generator's does, and an await lowers to
// value.AsyncGenAwait(_co, p) so it parks the async generator's driver rather than a plain
// async body's. Each pull returns a promise the consumer awaits, the join between the
// pull-at-a-time generator protocol and the settle-later promise protocol. This file builds
// the function form, the shared coroutine body, and the manual next() drive.

// asyncGeneratorElemType reports the yielded element type of an async generator or async
// iterator type, the Y in the *value.AsyncGen[Y] the runtime drives, so an AsyncGenerator
// typed slot (the return of an async generator function, a `const g = ag()` binding)
// renders to the coroutine pointer rather than expanding structurally. The judgment is the
// type's symbol name, the async mirror of generatorElemType, and the element type is the
// generic's first type argument (AsyncGenerator<T, TReturn, TNext> puts T first). A type
// with no readable argument reports false so the caller keeps its existing handling.
func (r *Renderer) asyncGeneratorElemType(t frontend.Type) (frontend.Type, bool) {
	sym, ok := r.prog.TypeSymbol(t)
	if !ok {
		return frontend.Type{}, false
	}
	switch sym.Name {
	case "AsyncGenerator", "AsyncIterableIterator", "AsyncIterator", "AsyncGeneratorFunction":
	default:
		return frontend.Type{}, false
	}
	args := r.prog.TypeArguments(t)
	if len(args) == 0 || args[0].Flags == 0 {
		return frontend.Type{}, false
	}
	return args[0], true
}

// asyncGeneratorCoroutine builds the value.NewAsyncGen[Y](func(_co *value.AsyncGenCo[Y])
// value.Value { ... }) expression an async generator body lowers to, and returns the Go
// element type Y alongside it so the caller can spell the *value.AsyncGen[Y] the function
// returns. The body lowers with the coroutine handle name in scope as both the generator
// handle and the async handle, and inAsyncGen set so an await routes to the AsyncGen
// helpers; a yield routes to _co.Yield and a return boxes to value.Value. The body's
// fall-off appends `return value.Undefined`, the completion an async generator that runs
// off its end reports.
func (r *Renderer) asyncGeneratorCoroutine(fn frontend.Node) (yieldGo ast.Expr, newGen ast.Expr, err error) {
	block, ok := r.funcBodyBlock(fn)
	if !ok {
		return nil, nil, &NotYetLowerable{Reason: "an async generator without a body is a later slice"}
	}
	// inAsyncGen is set for the whole build, not just the body lowering, so the element-type
	// scan already reads a yield* delegate as an async generator: its delegate carries an
	// AsyncGenerator symbol rather than a Generator one, which generatorYieldType only
	// consults under this flag.
	prevInAG := r.inAsyncGen
	r.inAsyncGen = true
	defer func() { r.inAsyncGen = prevInAG }()
	elemType, elemGo, err := r.generatorYieldType(block)
	if err != nil {
		return nil, nil, err
	}

	// The body-scoped analyses are set up the way generatorCoroutine sets them, off both
	// the signature parameters and the body declarations, so a union-typed or any-typed
	// parameter is tracked inside the body. They are saved and restored so the coroutine's
	// scope does not leak out.
	var params []frontend.Param
	if sig, ok := r.prog.SignatureAt(fn); ok {
		params = sig.Params
	}
	bodyStmts := r.prog.Children(block)
	prevUnion := r.unionLocals
	r.unionLocals = r.unionLocalsOf(params, bodyStmts)
	defer func() { r.unionLocals = prevUnion }()
	prevDyn := r.dynLocals
	r.dynLocals = r.dynLocalsOf(params, bodyStmts)
	defer func() { r.dynLocals = prevDyn }()

	coName := r.freshTemp()
	prevCo, prevYT, prevAsync := r.genCo, r.genYieldType, r.asyncCo
	r.genCo, r.genYieldType, r.asyncCo = coName, elemType, coName
	defer func() {
		r.genCo, r.genYieldType, r.asyncCo = prevCo, prevYT, prevAsync
	}()

	body, err := r.blockOf(fn)
	if err != nil {
		return nil, nil, err
	}
	r.requireImport(valuePkg)
	if n := len(body.List); n == 0 || !isGoReturn(body.List[n-1]) {
		body.List = append(body.List, &ast.ReturnStmt{Results: []ast.Expr{sel("value", "Undefined")}})
	}

	coParam := &ast.Field{
		Names: []*ast.Ident{ident(coName)},
		Type:  star(index(sel("value", "AsyncGenCo"), elemGo)),
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{coParam}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "Value")}}},
		},
		Body: body,
	}
	newGen = &ast.CallExpr{Fun: index(sel("value", "NewAsyncGen"), elemGo), Args: []ast.Expr{lit}}
	yieldGo, err = r.typeExpr(elemType)
	if err != nil {
		return nil, nil, err
	}
	return yieldGo, newGen, nil
}

// asyncGeneratorFuncDecl lowers a top-level async generator function to a Go function that
// returns a running coroutine: g(params) *value.AsyncGen[Y] { return
// value.NewAsyncGen[Y](...) }. The body is the coroutine func asyncGeneratorCoroutine
// builds, so the function's only statement hands the caller the *value.AsyncGen a for
// await...of or a manual next() drives. A generic async generator or one with a rest
// parameter is a later slice.
func (r *Renderer) asyncGeneratorFuncDecl(fn frontend.Node, sig frontend.Signature, name string) (*ast.FuncDecl, error) {
	if len(sig.TypeParams) != 0 {
		return nil, &NotYetLowerable{Reason: "a generic async generator needs monomorphization, a later slice"}
	}
	if sig.RestParam != nil {
		return nil, &NotYetLowerable{Reason: "an async generator with a rest parameter is a later slice"}
	}
	// An async generator is called through the shared finishCall path, which fills
	// value.None for an omitted bare optional the same way a plain function's call
	// does, so the body tracks the full optParamsOf, both the bare x?: T and a
	// required x: T | undefined. The push runs before funcParamFields builds the
	// fields, so a bare optional renders its value.Opt[T] field instead of handing
	// back, and a read the checker narrowed to T unwraps it with .Get().
	defer r.pushOptParams(r.optParamsOf(fn, sig))()
	params, err := r.funcParamFields(fn, sig)
	if err != nil {
		return nil, err
	}
	yieldGo, newGen, err := r.asyncGeneratorCoroutine(fn)
	if err != nil {
		return nil, err
	}
	result := &ast.Field{Type: star(index(sel("value", "AsyncGen"), yieldGo))}
	return &ast.FuncDecl{
		Name: ident(name),
		Type: &ast.FuncType{Params: params, Results: &ast.FieldList{List: []*ast.Field{result}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{newGen}}}},
	}, nil
}

// asyncGeneratorFuncExpr lowers an async generator function expression used as a value to
// a closure that returns the running coroutine: func(params) *value.AsyncGen[Y] { return
// value.NewAsyncGen[Y](...) }. It is the expression form of asyncGeneratorFuncDecl, the
// shape a const bound to an async function* takes, and shares the same coroutine builder.
func (r *Renderer) asyncGeneratorFuncExpr(n frontend.Node, fields []*ast.Field) (ast.Expr, error) {
	yieldGo, newGen, err := r.asyncGeneratorCoroutine(n)
	if err != nil {
		return nil, err
	}
	result := &ast.Field{Type: star(index(sel("value", "AsyncGen"), yieldGo))}
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{List: fields}, Results: &ast.FieldList{List: []*ast.Field{result}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{newGen}}}},
	}, nil
}

// asyncGeneratorMethodCall lowers a manual drive of an async generator, it.next(v), to the
// runtime pull that returns the promise for the { value, done } result. The receiver is the
// *value.AsyncGen[Y] the async generator produced; g.Next resumes the body to its next
// yield or completion and settles the returned promise with the result, which the consumer
// awaits. next(v) boxes its argument into the generator's dynamic in-channel, and a boxer
// closure lifts the yielded Y back into a value.Value the way the plain generator drive
// does. A receiver that is not an async generator returns ok false; return and throw are
// later slices.
func (r *Renderer) asyncGeneratorMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, bool, error) {
	elem, ok := r.asyncGeneratorElemType(r.prog.TypeAt(recvNode))
	if !ok {
		return nil, false, nil
	}
	switch method {
	case "next":
	case "return", "throw":
		return nil, false, &NotYetLowerable{Reason: "an async generator's ." + method + "() is a later slice"}
	default:
		return nil, false, &NotYetLowerable{Reason: "an async generator's ." + method + "() is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, false, err
	}
	boxer, err := r.genElemBoxer(elem)
	if err != nil {
		return nil, false, err
	}
	sent, err := r.genSentArg(argNodes)
	if err != nil {
		return nil, false, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: recv, Sel: ident("Next")},
		Args: []ast.Expr{sent, boxer},
	}, true, nil
}
