package lower

import (
	"go/ast"
	"go/token"

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
}

// isSymbolIteratorName reports whether a class member name node is the well-known
// [Symbol.iterator] computed name, the key an iterable class defines its iterator
// factory under. The parser surfaces it as an unnamed node wrapping the property
// access Symbol.iterator, so it is told apart from a [expr] or a ["str"] computed
// name by that exact shape.
func (r *Renderer) isSymbolIteratorName(nameNode frontend.Node) bool {
	if nameNode.Kind() != frontend.NodeUnknown {
		return false
	}
	kids := r.prog.Children(nameNode)
	if len(kids) != 1 || kids[0].Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	pa := r.prog.Children(kids[0])
	return len(pa) == 2 && r.prog.Text(pa[0]) == "Symbol" && r.prog.Text(pa[1]) == "iterator"
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
	return iteratorShape{elem: valueProp.Type, valueName: valueName, doneName: doneName}, true
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
	loop := &ast.ForStmt{Body: &ast.BlockStmt{List: loopStmts}}
	return &ast.BlockStmt{List: []ast.Stmt{getIter, loop}}, nil
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
