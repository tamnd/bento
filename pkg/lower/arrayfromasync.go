package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers Array.fromAsync (08 group 3): the static that collects an async
// or sync source into a promise of an array. fromAsync mints its own async body, a
// value.RunAsync coroutine that awaits each element the source yields and appends it,
// then fulfils the promise with the collected array. A caller awaits that promise the
// same way it awaits any other, so the collected array flows out at the await.
//
// The lowered form covers a sync iterable whose elements are promises (each awaited
// to its fulfilled value) and a sync iterable of plain values (each awaited to
// itself, the one-microtask hop fromAsync still takes). A source with no static
// element type, an async iterable driven through [Symbol.asyncIterator], and the map
// callback are each a later slice and hand back, the same boundary for await draws
// over a sync array.

// arrayFromAsync lowers Array.fromAsync(source) to the promise that collects the
// source. The result type is Promise<T[]>, so its element array type comes from the
// promise's element; the source's element decides how each item is awaited. The whole
// expression is a value.RunAsync over a fresh coroutine, so it is a promise a caller
// awaits, and its inner awaits park that coroutine rather than the caller's.
func (r *Renderer) arrayFromAsync(call frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Array.fromAsync with a map callback or thisArg is a later slice"}
	}
	prom, ok := r.promiseElem(r.prog.TypeAt(call))
	if !ok {
		return nil, &NotYetLowerable{Reason: "Array.fromAsync whose result is not a Promise is a later slice"}
	}
	arrGo, err := r.typeExpr(prom)
	if err != nil {
		return nil, err
	}
	resElem, ok := r.prog.ElementType(prom)
	if !ok {
		return nil, &NotYetLowerable{Reason: "Array.fromAsync whose result is not an array is a later slice"}
	}
	resElemGo, err := r.typeExpr(resElem)
	if err != nil {
		return nil, err
	}
	srcNode := argNodes[0]
	srcElem, ok := r.prog.ElementType(r.prog.TypeAt(srcNode))
	if !ok {
		return nil, &NotYetLowerable{Reason: "Array.fromAsync over an async iterable or a non-array source is a later slice"}
	}
	// The await form follows the source's element type, the same split for await draws:
	// a promise element awaits to its inner value, a definite non-thenable wraps and
	// awaits to itself. An element that might be a thenable or is dynamic hands back.
	coName, eName := r.freshTemp(), r.freshTemp()
	var awaited ast.Expr
	switch {
	case r.isPromiseElem(srcElem):
		inner, _ := r.promiseElem(srcElem)
		innerGo, err := r.typeExpr(inner)
		if err != nil {
			return nil, err
		}
		if same, err := sameGoType(innerGo, resElemGo); err != nil {
			return nil, err
		} else if !same {
			return nil, &NotYetLowerable{Reason: "Array.fromAsync whose awaited element type differs from the result element type is a later slice"}
		}
		awaited = &ast.CallExpr{Fun: sel("value", "Await"), Args: []ast.Expr{ident(coName), ident(eName)}}
	case r.isDefiniteNonThenable(srcElem):
		awaited = &ast.CallExpr{Fun: index(sel("value", "AwaitValue"), resElemGo), Args: []ast.Expr{ident(coName), ident(eName)}}
	default:
		return nil, &NotYetLowerable{Reason: "Array.fromAsync over a source whose element may be a thenable or is dynamic is a later slice"}
	}
	src, err := r.lowerExpr(srcNode)
	if err != nil {
		return nil, err
	}
	r.usesPromise = true
	r.requireImport(valuePkg)

	elemsName, outName := r.freshTemp(), r.freshTemp()
	elemsCall := &ast.CallExpr{Fun: &ast.SelectorExpr{X: src, Sel: ident("Elems")}}
	body := []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ident(elemsName)}, Tok: token.DEFINE, Rhs: []ast.Expr{elemsCall}},
		&ast.AssignStmt{Lhs: []ast.Expr{ident(outName)}, Tok: token.DEFINE, Rhs: []ast.Expr{&ast.CallExpr{
			Fun: ident("make"),
			Args: []ast.Expr{
				&ast.ArrayType{Elt: resElemGo},
				&ast.BasicLit{Kind: token.INT, Value: "0"},
				&ast.CallExpr{Fun: ident("len"), Args: []ast.Expr{ident(elemsName)}},
			},
		}}},
		&ast.RangeStmt{
			Key:   ident("_"),
			Value: ident(eName),
			Tok:   token.DEFINE,
			X:     ident(elemsName),
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.AssignStmt{
				Lhs: []ast.Expr{ident(outName)},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{&ast.CallExpr{Fun: ident("append"), Args: []ast.Expr{ident(outName), awaited}}},
			}}},
		},
		&ast.ReturnStmt{Results: []ast.Expr{&ast.CallExpr{Fun: sel("value", "ArrayFrom"), Args: []ast.Expr{ident(outName)}}}},
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident(coName)}, Type: star(sel("value", "AsyncCo"))}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: arrGo}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
	return &ast.CallExpr{Fun: index(sel("value", "RunAsync"), arrGo), Args: []ast.Expr{lit}}, nil
}
