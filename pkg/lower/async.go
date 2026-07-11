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
