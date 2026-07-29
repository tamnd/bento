package lower

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// A for...of whose iterable is a boxed value cannot be settled while lowering. Every
// other for...of path in stmt.go keys off something the checker proved: this is an
// array, this is a string, this is a generator, this class defines [Symbol.iterator].
// A value that came back from a node built-in carries none of that. `os.cpus()` is a
// value.Value and the checker's answer stops there, so `for (const c of os.cpus())`
// fell through every case and handed back.
//
// The question is deferred to run time instead. value.Iterate looks at what it was
// actually handed, an array, a string, or anything carrying Symbol.iterator, and
// answers a puller. The loop below drives that puller, so one emitted shape covers
// every iterable a boxed value can turn out to be, and a value that is none of them
// throws the TypeError Node throws rather than compiling into a walk of nothing.

// forOfDynamic lowers a for...of over a boxed iterable to a pull-until-done loop over
// value.Iterate. It reports whether it handled the statement, so the caller keeps its
// hand-back for an iterable that is neither statically known nor boxed.
//
// The loop shape is forOfIterator's, with the runtime's IterResult in place of the
// user type's { value, done } struct. Nothing closes the iterator on an early break:
// value.Iterate answers an IterHelper, which has no return(), so there is nothing to
// call. A source that needs closing reaches forOfIterator instead, where the checker
// found the type that declares it.
func (r *Renderer) forOfDynamic(iterable, bindNode frontend.Node, name string, bodyNode frontend.Node) (ast.Stmt, bool, error) {
	if !r.isDynamic(iterable) && !r.producesBoxedValue(iterable) {
		return nil, false, nil
	}
	src, err := r.lowerExpr(iterable)
	if err != nil {
		return nil, true, err
	}
	// The loop variable holds a value.Value, whatever the checker made of the element
	// type. It has to: the iterable is a box, so what comes out of it is a box, and the
	// checker's element type is a guess at a shape the runtime never built. Marking the
	// binding routes every read of it in the body through the dynamic value model, so
	// `c.model` is a property read on the box rather than a Go struct field selector
	// that would not compile. The mark is dropped again after the body, since the
	// binding's scope is this loop and an outer name of the same spelling is not this.
	prev, had := r.dynBoundLocals[name]
	r.markDynBound(name)
	body, err := r.loopBody(bodyNode)
	if had {
		r.dynBoundLocals[name] = prev
	} else {
		delete(r.dynBoundLocals, name)
	}
	if err != nil {
		return nil, true, err
	}
	itName := r.freshTemp()
	resName := r.freshTemp()
	stmts := []ast.Stmt{&ast.AssignStmt{
		Lhs: []ast.Expr{ident(itName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{r.iterateCall(src, iterable)},
	}}
	loop := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ident(resName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(itName), Sel: ident("Next")}}},
		},
		&ast.IfStmt{
			Cond: &ast.SelectorExpr{X: ident(resName), Sel: ident("Done")},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}},
		},
	}
	// A braced body that re-declares the loop variable shadows it in its own lexical
	// scope, so the body lowers into a nested Go block and the binding is dropped:
	// every mention of the name in the body reaches the inner declaration. A binding
	// the body never reads is not bound at all, since Go rejects an unused variable.
	// Both are the rules forOfIterator follows, for the same reasons.
	if r.bodyBlockShadows(bodyNode, r.prog.Text(bindNode)) {
		loop = append(loop, &ast.BlockStmt{List: body.List})
	} else {
		if r.bodyUsesName(bodyNode, r.prog.Text(bindNode)) {
			loop = append(loop, &ast.AssignStmt{
				Lhs: []ast.Expr{ident(name)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{&ast.SelectorExpr{X: ident(resName), Sel: ident("Value")}},
			})
		}
		loop = append(loop, body.List...)
	}
	stmts = append(stmts, &ast.ForStmt{Body: &ast.BlockStmt{List: loop}})
	return &ast.BlockStmt{List: stmts}, true, nil
}

// forOfDynamicPatternIter lowers a destructuring for...of whose iterable is a boxed
// value: `for (const [i, c] of os.cpus().entries())`. It is forOfDynamicDestructure's
// twin, binding the pattern the same dynamic way against a per-iteration temporary,
// with value.Iterate driving the walk in place of a range over a Go .Elems() slice a
// box does not have.
//
// The pattern binds before the body lowers, so every name it introduces is marked
// dynamic first: an element read off a box is itself a box, and the body's reads of
// those names must route through the value model rather than take the checker's
// element type.
func (r *Renderer) forOfDynamicPatternIter(iterable, pattern, bodyNode frontend.Node) (ast.Stmt, error) {
	src, err := r.lowerExpr(iterable)
	if err != nil {
		return nil, err
	}
	elemName := r.freshTemp()
	binds, err := r.bindDynamicPattern(pattern, ident(elemName), token.DEFINE)
	if err != nil {
		return nil, err
	}
	prevDyn := r.dynBoundLocals
	m := map[string]bool{}
	for name := range prevDyn {
		m[name] = true
	}
	r.collectAssignedNames(binds, m)
	r.dynBoundLocals = m
	defer func() { r.dynBoundLocals = prevDyn }()
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, err
	}
	itName := r.freshTemp()
	resName := r.freshTemp()
	loop := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ident(resName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(itName), Sel: ident("Next")}}},
		},
		&ast.IfStmt{
			Cond: &ast.SelectorExpr{X: ident(resName), Sel: ident("Done")},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}},
		},
		&ast.AssignStmt{
			Lhs: []ast.Expr{ident(elemName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.SelectorExpr{X: ident(resName), Sel: ident("Value")}},
		},
	}
	loop = append(loop, binds...)
	loop = append(loop, body.List...)
	return &ast.BlockStmt{List: []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ident(itName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{r.iterateCall(src, iterable)},
		},
		&ast.ForStmt{Body: &ast.BlockStmt{List: loop}},
	}}, nil
}

// iterateCall builds the value.Iterate(src, "<text>") call. The second argument is
// the source text of the iterable expression, which is there only for the error a
// non-iterable raises: Node says "os.cpus is not iterable", naming what the program
// wrote, and the runtime cannot recover that from a value. This is the one place that
// has both, so it hands the text over rather than let the message degrade to a
// description of the box.
func (r *Renderer) iterateCall(src ast.Expr, iterable frontend.Node) ast.Expr {
	return &ast.CallExpr{
		Fun: sel("value", "Iterate"),
		Args: []ast.Expr{
			src,
			&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(iterSourceText(r.prog.Text(iterable)))},
		},
	}
}

// iterateToSliceCall builds the value.IterateToSlice(src, "<text>") call, the eager
// form for a spread, which needs every element at once rather than a loop it can
// drive. It names the source the same way iterateCall does, so a non-iterable spread
// and a non-iterable for...of report the same expression.
func (r *Renderer) iterateToSliceCall(src ast.Expr, iterable frontend.Node) ast.Expr {
	return &ast.CallExpr{
		Fun: sel("value", "IterateToSlice"),
		Args: []ast.Expr{
			src,
			&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(iterSourceText(r.prog.Text(iterable)))},
		},
	}
}

// iterSourceText trims the iterable's source text down to something that reads well in
// a one-line error. A multi-line or very long expression is replaced by a fixed phrase
// rather than pasted whole, since the point of naming the expression is to identify it
// at a glance and a wrapped paragraph does not.
func iterSourceText(text string) string {
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' || text[i] == '\r' {
			return "the value"
		}
	}
	if len(text) > 40 {
		return "the value"
	}
	return text
}
