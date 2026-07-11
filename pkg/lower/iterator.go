package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// The iterator protocol is JavaScript's one way to walk a value: an iterable
// answers a [Symbol.iterator]() call with an iterator, and the iterator answers
// each next() call with a { value, done } result until done is true. bento walks
// an array over its Elems and a string over its code points directly, and a
// built-in generator through its next() closure, but a user iterable, a class
// that defines [Symbol.iterator], goes through this protocol path: for...of, and
// later spread and destructuring, all pull it the same way a developer writes the
// loop by hand.

// symbolIteratorGoName is the Go method name a [Symbol.iterator] member lowers to,
// the name for...of calls to obtain the iterator. It is fixed so the loop can name
// it without threading the class through.
const symbolIteratorGoName = "SymbolIterator"

// symbolIteratorProp is the sentinel property key the [Symbol.iterator] method
// carries in the member map. Its spelling starts with a byte no JavaScript
// property name can, so it never collides with a real member.
const symbolIteratorProp = "[Symbol.iterator]"

// builtinIteratorTypeNames are the library iterator and generator types bento
// lowers to a next() closure rather than a struct with a Go Next method. A
// [Symbol.iterator] whose iterator is one of these is not driven through the
// protocol path, since the closure it lowers to has no Next method to call.
var builtinIteratorTypeNames = map[string]bool{
	"Generator":             true,
	"AsyncGenerator":        true,
	"Iterator":              true,
	"IterableIterator":      true,
	"IteratorObject":        true,
	"AsyncIterator":         true,
	"AsyncIterableIterator": true,
}

// iteratorShape is the lowered view of a type reached through its [Symbol.iterator]
// member: the element type each turn yields, and the Go field names the { value,
// done } result is read through.
type iteratorShape struct {
	elem      frontend.Type
	valueName string
	doneName  string
	// returnName is the Go method name of the iterator's optional return(), the one
	// for...of calls to close the iterator on an early exit, or "" when the iterator
	// has no return() and so needs no close.
	returnName string
}

// symbolAsyncIteratorGoName is the Go method name a [Symbol.asyncIterator] member
// lowers to, the name for await...of calls to obtain the async iterator. Like the
// sync SymbolIterator it is fixed so the loop can name it without threading the class
// through.
const symbolAsyncIteratorGoName = "SymbolAsyncIterator"

// symbolAsyncIteratorProp is the sentinel property key the [Symbol.asyncIterator]
// method carries in the member map, the async mirror of symbolIteratorProp.
const symbolAsyncIteratorProp = "[Symbol.asyncIterator]"

// symbolAsyncIteratorMemberPrefix is the mangled property-name prefix the checker
// gives a [Symbol.asyncIterator] member: the internal-symbol prefix byte, then
// @asyncIterator, then a per-symbol id the prefix match skips over. It never
// collides with the sync @iterator prefix, which differs from its third byte on.
const symbolAsyncIteratorMemberPrefix = "\xFE@asyncIterator"

// isSymbolIteratorName reports whether a class member name node is the well-known
// [Symbol.iterator] computed name, the key an iterable class defines its iterator
// factory under. The parser surfaces it as an unnamed node wrapping the property
// access Symbol.iterator, so it is told apart from a [expr] or a ["str"] computed
// name by that exact shape.
func (r *Renderer) isSymbolIteratorName(nameNode frontend.Node) bool {
	return r.isSymbolMemberName(nameNode, "iterator")
}

// isSymbolAsyncIteratorName reports whether a class member name node is the
// well-known [Symbol.asyncIterator] computed name, the key an async iterable class
// defines its async iterator factory under. It reads the same unnamed-node-wrapping
// -a-property-access shape isSymbolIteratorName matches, but for Symbol.asyncIterator.
func (r *Renderer) isSymbolAsyncIteratorName(nameNode frontend.Node) bool {
	return r.isSymbolMemberName(nameNode, "asyncIterator")
}

// isSymbolMemberName reports whether a class member name node is the well-known
// computed name Symbol.<member>, the shared shape check the sync and async iterator
// name matchers use: an unnamed node wrapping a Symbol.<member> property access.
func (r *Renderer) isSymbolMemberName(nameNode frontend.Node, member string) bool {
	if nameNode.Kind() != frontend.NodeUnknown {
		return false
	}
	kids := r.prog.Children(nameNode)
	if len(kids) != 1 || kids[0].Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	pa := r.prog.Children(kids[0])
	return len(pa) == 2 && r.prog.Text(pa[0]) == "Symbol" && r.prog.Text(pa[1]) == member
}

// isSymbolIteratorExpr reports whether an expression node is the well-known
// Symbol.iterator property access, the key a manual `obj[Symbol.iterator]()`
// reference reads the iterator factory through. Unlike isSymbolIteratorName, which
// matches the computed member name in a class body (an unnamed node wrapping the
// access), this matches the access expression itself, the shape it takes as an
// element-access index.
func (r *Renderer) isSymbolIteratorExpr(node frontend.Node) bool {
	if node.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	pa := r.prog.Children(node)
	return len(pa) == 2 && r.prog.Text(pa[0]) == "Symbol" && r.prog.Text(pa[1]) == "iterator"
}

// isSymbolAsyncIteratorExpr reports whether an expression node is the well-known
// Symbol.asyncIterator property access, the key a manual `obj[Symbol.asyncIterator]()`
// reference reads the async iterator factory through, the async mirror of
// isSymbolIteratorExpr.
func (r *Renderer) isSymbolAsyncIteratorExpr(node frontend.Node) bool {
	if node.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	pa := r.prog.Children(node)
	return len(pa) == 2 && r.prog.Text(pa[0]) == "Symbol" && r.prog.Text(pa[1]) == "asyncIterator"
}

// memberByPrefix returns the first property of a type whose name starts with the
// given prefix, the way the [Symbol.iterator] member is reached: the checker
// mangles a well-known symbol member to an internal name that leads with a prefix
// byte and the symbol's description, so the member is found by that lead rather
// than an exact name that carries a per-symbol id.
func (r *Renderer) memberByPrefix(t frontend.Type, prefix string) (frontend.Property, bool) {
	for _, p := range r.prog.Properties(t) {
		if len(p.Name) >= len(prefix) && p.Name[:len(prefix)] == prefix {
			return p, true
		}
	}
	return frontend.Property{}, false
}

// memberByName returns the named property of a type, the ordinary lookup the
// protocol navigation uses for the iterator's next and the result's value and
// done.
func (r *Renderer) memberByName(t frontend.Type, name string) (frontend.Property, bool) {
	for _, p := range r.prog.Properties(t) {
		if p.Name == name {
			return p, true
		}
	}
	return frontend.Property{}, false
}

// symbolIteratorMemberPrefix is the mangled property-name prefix the checker gives
// a [Symbol.iterator] member: the internal-symbol prefix byte, then @iterator,
// then a per-symbol id the prefix match skips over.
const symbolIteratorMemberPrefix = "\xFE@iterator"

// symbolIteratorShape returns how a type drives the iterator protocol, or false
// when it does not. It navigates the [Symbol.iterator] member to its iterator
// type, the iterator's next() to the { value, done } result, and reads the
// element type off the result's value. The path is taken only when the iterator
// is a user type bento lowers to a struct with a Go Next method, not a built-in
// generator closure, and only when the result's done is a Go bool, since the
// lowered loop breaks on `if r.Done`, so the emitted reads all resolve.
func (r *Renderer) symbolIteratorShape(t frontend.Type) (iteratorShape, bool) {
	iterMethod, ok := r.memberByPrefix(t, symbolIteratorMemberPrefix)
	if !ok {
		return iteratorShape{}, false
	}
	call, _ := r.prog.Signatures(iterMethod.Type)
	if len(call) == 0 {
		return iteratorShape{}, false
	}
	iterType := call[0].Return
	if sym, ok := r.prog.TypeSymbol(iterType); ok && builtinIteratorTypeNames[sym.Name] {
		return iteratorShape{}, false
	}
	next, ok := r.memberByName(iterType, "next")
	if !ok {
		return iteratorShape{}, false
	}
	nextCall, _ := r.prog.Signatures(next.Type)
	if len(nextCall) == 0 {
		return iteratorShape{}, false
	}
	result := nextCall[0].Return
	valueProp, ok := r.memberByName(result, "value")
	if !ok {
		return iteratorShape{}, false
	}
	doneProp, ok := r.memberByName(result, "done")
	if !ok {
		return iteratorShape{}, false
	}
	if r.primitiveFlagsOfType(doneProp.Type)&frontend.TypeBoolean == 0 {
		return iteratorShape{}, false
	}
	valueName, ok := exportedField("value")
	if !ok {
		return iteratorShape{}, false
	}
	doneName, ok := exportedField("done")
	if !ok {
		return iteratorShape{}, false
	}
	shape := iteratorShape{elem: valueProp.Type, valueName: valueName, doneName: doneName}
	// An iterator may define an optional return(), which for...of calls to close it on
	// an early exit. When it does, the Go method name is recorded so the loop can call
	// it; when it does not, returnName stays empty and the loop needs no close.
	if _, ok := r.memberByName(iterType, "return"); ok {
		if name, ok := exportedField("return"); ok {
			shape.returnName = name
		}
	}
	return shape, true
}

// asyncIteratorShape returns how a type drives the async iterator protocol, or false
// when it does not. It is the async mirror of symbolIteratorShape: it navigates the
// [Symbol.asyncIterator] member to its iterator type, the iterator's next() to the
// promise it returns, and through that promise to the awaited { value, done } result.
// The one extra step over the sync path is the promiseElem unwrap: an async iterator's
// next() returns Promise<{ value, done }>, so the result the loop reads is what awaiting
// that promise yields. The path is taken only when the iterator is a user type bento
// lowers to a struct with Go Next and (optionally) Return methods, not a built-in async
// generator closure, and only when the result's done is a Go bool the loop breaks on.
func (r *Renderer) asyncIteratorShape(t frontend.Type) (iteratorShape, bool) {
	iterMethod, ok := r.memberByPrefix(t, symbolAsyncIteratorMemberPrefix)
	if !ok {
		return iteratorShape{}, false
	}
	call, _ := r.prog.Signatures(iterMethod.Type)
	if len(call) == 0 {
		return iteratorShape{}, false
	}
	iterType := call[0].Return
	if sym, ok := r.prog.TypeSymbol(iterType); ok && builtinIteratorTypeNames[sym.Name] {
		return iteratorShape{}, false
	}
	next, ok := r.memberByName(iterType, "next")
	if !ok {
		return iteratorShape{}, false
	}
	nextCall, _ := r.prog.Signatures(next.Type)
	if len(nextCall) == 0 {
		return iteratorShape{}, false
	}
	result, ok := r.promiseElem(nextCall[0].Return)
	if !ok {
		return iteratorShape{}, false
	}
	valueProp, ok := r.memberByName(result, "value")
	if !ok {
		return iteratorShape{}, false
	}
	doneProp, ok := r.memberByName(result, "done")
	if !ok {
		return iteratorShape{}, false
	}
	if r.primitiveFlagsOfType(doneProp.Type)&frontend.TypeBoolean == 0 {
		return iteratorShape{}, false
	}
	valueName, ok := exportedField("value")
	if !ok {
		return iteratorShape{}, false
	}
	doneName, ok := exportedField("done")
	if !ok {
		return iteratorShape{}, false
	}
	shape := iteratorShape{elem: valueProp.Type, valueName: valueName, doneName: doneName}
	// An async iterator may define an optional return(), which for await...of awaits to
	// close it on an early exit, the same close the sync path records for the sync loop.
	if _, ok := r.memberByName(iterType, "return"); ok {
		if name, ok := exportedField("return"); ok {
			shape.returnName = name
		}
	}
	return shape, true
}

// isForAwait reports whether a for...of node is a for await...of: the parser leads such
// a loop with an await token before the loop binding, giving it a fourth child the plain
// for...of lacks. It is read both to route the statement to the async path and to mark an
// enclosing async body as suspending.
func (r *Renderer) isForAwait(n frontend.Node) bool {
	kids := r.prog.Children(n)
	return len(kids) == 4 && kids[0].Kind() == frontend.NodeUnknown &&
		strings.TrimSpace(r.prog.Text(kids[0])) == "await"
}

// lowerForAwaitOf lowers a for await...of loop, the async iteration statement that
// awaits each result before the body runs. It resolves the loop binding the way
// lowerForOf does, then dispatches on the iterable: a user async iterable, a class that
// defines [Symbol.asyncIterator], is driven through the async iterator protocol. The
// loop must sit inside a lowered async body, whose coroutine handle each await parks on;
// a for await outside one, or over an iterable this slice does not yet drive, hands back.
func (r *Renderer) lowerForAwaitOf(declNode, iterable, bodyNode frontend.Node) (ast.Stmt, error) {
	if r.asyncCo == "" {
		return nil, &NotYetLowerable{Reason: "a for await...of outside a lowered async body is a later slice"}
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, declNode, &decls)
	if len(decls) != 1 {
		return nil, &NotYetLowerable{Reason: "for await...of with other than a single loop binding is a later slice"}
	}
	dkids := r.prog.Children(decls[0])
	if len(dkids) != 1 || dkids[0].Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "for await...of with a destructuring or annotated loop variable is a later slice"}
	}
	name, ok := localName(r.prog.Text(dkids[0]))
	if !ok {
		return nil, &NotYetLowerable{Reason: "for await...of loop variable is not a Go identifier"}
	}
	if shape, ok := r.asyncIteratorShape(r.prog.TypeAt(iterable)); ok {
		return r.forAwaitOfIterator(iterable, dkids[0], name, bodyNode, shape)
	}
	return nil, &NotYetLowerable{Reason: "for await...of over this iterable is a later slice"}
}

// forAwaitOfIterator lowers a for await...of over a user async iterable through the
// async iterator protocol: obtain the async iterator once, then each turn await the
// promise next() returns, stop when the settled result is done, and bind its value. It
// is the async mirror of forOfIterator, the same pull-until-done loop but with each
// pull wrapped in value.Await on the enclosing async body's coroutine handle, so the
// loop parks until the result settles before the body runs. The iterator is obtained
// once up front, so a second for await over the same source gets its own fresh iterator,
// matching the protocol. A binding the body never reads is not bound, since Go rejects
// an unused variable.
func (r *Renderer) forAwaitOfIterator(iterable, bindNode frontend.Node, name string, bodyNode frontend.Node, shape iteratorShape) (ast.Stmt, error) {
	// Closing an async iterator on an early exit awaits its return(); that awaited close
	// is a later slice, so a loop over an iterator that defines return() hands back rather
	// than leave it unclosed on a break.
	if shape.returnName != "" {
		return nil, &NotYetLowerable{Reason: "async iterator close via return() on early exit from for await...of is a later slice"}
	}
	src, err := r.lowerExpr(iterable)
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, err
	}
	nextName, ok := exportedField("next")
	if !ok {
		return nil, &NotYetLowerable{Reason: "an async iterator whose next is not a Go method name is a later slice"}
	}
	r.requireImport(valuePkg)
	itName := r.freshTemp()
	resName := r.freshTemp()
	getIter := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(itName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: src, Sel: ident(symbolAsyncIteratorGoName)}}},
	}
	// Each turn awaits the promise next() returns, parking the loop on the coroutine
	// handle until the result settles, then reads done and value off the { value, done }
	// result the await yields.
	pull := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(resName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun: sel("value", "Await"),
			Args: []ast.Expr{
				ident(r.asyncCo),
				&ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(itName), Sel: ident(nextName)}},
			},
		}},
	}
	brk := &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: ident(resName), Sel: ident(shape.doneName)},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}},
	}
	loopStmts := []ast.Stmt{pull, brk}
	if r.bodyUsesName(bodyNode, r.prog.Text(bindNode)) {
		loopStmts = append(loopStmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(name)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.SelectorExpr{X: ident(resName), Sel: ident(shape.valueName)}},
		})
	}
	loopStmts = append(loopStmts, body.List...)
	return &ast.BlockStmt{List: []ast.Stmt{getIter, &ast.ForStmt{Body: &ast.BlockStmt{List: loopStmts}}}}, nil
}

// forOfBodyBypassesClose reports whether a for...of body contains a completion
// that would jump past the after-loop iterator close: a return, a throw, or a
// labeled break or continue, any of which leaves the loop without falling through
// to the `if broke` that calls return(). An unlabeled break or continue is safe,
// since it lands on the after-loop close (break) or re-enters the loop (continue),
// so those do not count. A nested function body is not descended into, since its
// own return belongs to it, not to this loop.
func (r *Renderer) forOfBodyBypassesClose(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeFunctionDeclaration, frontend.NodeFunctionExpression, frontend.NodeArrowFunction,
		frontend.NodeMethodDeclaration, frontend.NodeGetAccessor, frontend.NodeSetAccessor, frontend.NodeConstructor:
		return false
	case frontend.NodeReturnStatement, frontend.NodeThrowStatement:
		return true
	case frontend.NodeUnknown:
		if word := branchKeyword(strings.TrimSpace(r.prog.Text(n))); word == "break" || word == "continue" {
			kids := r.prog.Children(n)
			// A labeled branch names its target, so it may leave a loop enclosing this
			// one; it is conservatively treated as a bypass. An unlabeled branch stays
			// with the nearest loop and is safe.
			return len(kids) == 1 && kids[0].Kind() == frontend.NodeIdentifier
		}
	}
	for _, k := range r.prog.Children(n) {
		if r.forOfBodyBypassesClose(k) {
			return true
		}
	}
	return false
}

// forOfIterator lowers a for...of over a user iterable through the iterator
// protocol: obtain the iterator once, then each turn pull a result, stop when it
// is done, and bind the value. It is the loop a developer writes against next(),
// the same pull-until-done shape forOfGenerator emits for a built-in generator,
// but over the user type's Go SymbolIterator and Next methods and the { value,
// done } struct they return. The iterator is obtained once up front, so a second
// for...of over the same source gets its own fresh iterator, matching the
// protocol. A binding the body never reads is not bound, since Go rejects an
// unused variable.
func (r *Renderer) forOfIterator(iterable, bindNode frontend.Node, name string, bodyNode frontend.Node, shape iteratorShape) (ast.Stmt, error) {
	src, err := r.lowerExpr(iterable)
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, err
	}
	nextName, ok := exportedField("next")
	if !ok {
		return nil, &NotYetLowerable{Reason: "an iterator whose next is not a Go method name is a later slice"}
	}
	// An iterator that defines return() is closed when the loop exits early, so the
	// loop is exited normally only when next() reports done. A `broke` flag starts
	// true and is cleared on the done branch, so after the loop an unbroken run (done)
	// skips the close and a broken run (an unlabeled break) calls it. A body that can
	// leave the loop another way, a return, a throw, or a labeled branch, would jump
	// past the after-loop close, so it hands back rather than skip the close silently.
	closes := shape.returnName != ""
	if closes && r.forOfBodyBypassesClose(bodyNode) {
		return nil, &NotYetLowerable{Reason: "iterator close on a return, throw, or labeled exit from for...of is a later slice"}
	}
	itName := r.freshTemp()
	resName := r.freshTemp()
	getIter := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(itName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: src, Sel: ident(symbolIteratorGoName)}}},
	}
	pull := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(resName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(itName), Sel: ident(nextName)}}},
	}
	doneStmts := []ast.Stmt{}
	block := []ast.Stmt{getIter}
	var brokeName string
	if closes {
		brokeName = r.freshTemp()
		block = append(block, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(brokeName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{ident("true")},
		})
		doneStmts = append(doneStmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(brokeName)},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{ident("false")},
		})
	}
	doneStmts = append(doneStmts, &ast.BranchStmt{Tok: token.BREAK})
	brk := &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: ident(resName), Sel: ident(shape.doneName)},
		Body: &ast.BlockStmt{List: doneStmts},
	}
	loopStmts := []ast.Stmt{pull, brk}
	if r.bodyUsesName(bodyNode, r.prog.Text(bindNode)) {
		loopStmts = append(loopStmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(name)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.SelectorExpr{X: ident(resName), Sel: ident(shape.valueName)}},
		})
	}
	loopStmts = append(loopStmts, body.List...)
	block = append(block, &ast.ForStmt{Body: &ast.BlockStmt{List: loopStmts}})
	if closes {
		block = append(block, &ast.IfStmt{
			Cond: ident(brokeName),
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{X: ident(itName), Sel: ident(shape.returnName)},
			}}}},
		})
	}
	return &ast.BlockStmt{List: block}, nil
}

// iterableToSliceExpr drains a user iterable into a Go slice of its element type,
// the collection spread and destructuring need in expression position. It is the
// same pull-until-done walk forOfIterator emits, wrapped in a func literal that
// returns the collected slice so it stands where a value is expected: a spread
// splices the slice, and destructuring indexes it. The iterator is obtained inside
// the literal, so each spread of the same source gets its own fresh iterator, the
// protocol's rule. The element type is the caller's, already checked to match the
// iterable's, so the appended values need no conversion.
func (r *Renderer) iterableToSliceExpr(src, elemType ast.Expr, shape iteratorShape) ast.Expr {
	nextName, _ := exportedField("next")
	sliceName := r.freshTemp()
	itName := r.freshTemp()
	resName := r.freshTemp()
	decl := &ast.DeclStmt{Decl: &ast.GenDecl{
		Tok:   token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ident(sliceName)}, Type: &ast.ArrayType{Elt: elemType}}},
	}}
	getIter := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(itName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: src, Sel: ident(symbolIteratorGoName)}}},
	}
	pull := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(resName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(itName), Sel: ident(nextName)}}},
	}
	brk := &ast.IfStmt{
		Cond: &ast.SelectorExpr{X: ident(resName), Sel: ident(shape.doneName)},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}},
	}
	grow := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(sliceName)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun:  ident("append"),
			Args: []ast.Expr{ident(sliceName), &ast.SelectorExpr{X: ident(resName), Sel: ident(shape.valueName)}},
		}},
	}
	loop := &ast.ForStmt{Body: &ast.BlockStmt{List: []ast.Stmt{pull, brk, grow}}}
	ret := &ast.ReturnStmt{Results: []ast.Expr{ident(sliceName)}}
	fn := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: &ast.ArrayType{Elt: elemType}}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{decl, getIter, loop, ret}},
	}
	return &ast.CallExpr{Fun: fn}
}
