package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers statements: variable declarations, assignments and updates,
// returns, and the control flow (if, for, for..of, while). Each lowering keeps
// the honest boundary of the package: a statement outside the subset hands back
// a NotYetLowerable so the unit routes to the engine.

// lowerBlock lowers a block node's statements to a Go block. It is used for the
// function body and for the arms of the control-flow statements, so a nested
// block lowers the same way the top-level one does.
func (r *Renderer) lowerBlock(block frontend.Node) (*ast.BlockStmt, error) {
	stmts, err := r.lowerStatements(r.prog.Children(block))
	if err != nil {
		return nil, err
	}
	return &ast.BlockStmt{List: stmts}, nil
}

// lowerStatements lowers a sequence of statement nodes, in order.
func (r *Renderer) lowerStatements(nodes []frontend.Node) ([]ast.Stmt, error) {
	out := make([]ast.Stmt, 0, len(nodes))
	for _, n := range nodes {
		stmts, err := r.lowerStatementMulti(n)
		if err != nil {
			return nil, err
		}
		out = append(out, stmts...)
	}
	return out, nil
}

// lowerStatementMulti lowers one statement, letting it expand into more than one Go
// statement. A statement-position ternary flattens here to an if plus the arms that
// return or assign each branch; every other statement lowers to the single Go
// statement lowerStatement builds and comes back as a one-element slice.
func (r *Renderer) lowerStatementMulti(n frontend.Node) ([]ast.Stmt, error) {
	if stmts, ok, err := r.flattenConditionalStatement(n); err != nil {
		return nil, err
	} else if ok {
		return stmts, nil
	}
	if stmts, ok, err := r.flattenCommaStatement(n); err != nil {
		return nil, err
	} else if ok {
		return stmts, nil
	}
	if stmts, ok, err := r.flattenChainedAssignStatement(n); err != nil {
		return nil, err
	} else if ok {
		return stmts, nil
	}
	s, err := r.lowerStatement(n)
	if err != nil {
		return nil, err
	}
	return []ast.Stmt{s}, nil
}

// lowerStatement lowers one statement. The covered set is the straight-line and
// control-flow forms a numeric body is built from: a return, a local variable
// declaration, an assignment, an if, and a while. The rest land in later slices,
// each handing back until then.
func (r *Renderer) lowerStatement(n frontend.Node) (ast.Stmt, error) {
	switch n.Kind() {
	case frontend.NodeReturnStatement:
		return r.lowerReturn(n)
	case frontend.NodeVariableStatement:
		return r.lowerVarStatement(n)
	case frontend.NodeExpressionStatement:
		return r.lowerExprStatement(n)
	case frontend.NodeIfStatement:
		return r.lowerIf(n)
	case frontend.NodeWhileStatement:
		return r.lowerWhile(n)
	case frontend.NodeForStatement:
		return r.lowerFor(n)
	case frontend.NodeForOfStatement:
		return r.lowerForOf(n)
	case frontend.NodeThrowStatement:
		return r.lowerThrow(n)
	case frontend.NodeTryStatement:
		return r.lowerTry(n)
	case frontend.NodeSwitchStatement:
		return r.lowerSwitch(n)
	case frontend.NodeBlock:
		// A bare block is a lexical scope, `{ let x = 1; ... }`, which Go spells the
		// same way. It appears as a braced case body and as a standalone scope in a
		// function body, and lowers to a Go block either way.
		return r.lowerBlock(n)
	case frontend.NodeUnknown:
		// The frontend leaves several control-flow statements unnamed, so each surfaces
		// here as an unclassified node the lowering recognizes by its shape: a break or
		// continue by its keyword text, a do...while by a body block and condition, a
		// labeled statement by a label identifier and the statement it labels. Anything
		// else keeps the default hand-back below.
		if s, ok := r.lowerBranch(n); ok {
			return s, nil
		}
		if s, ok, err := r.lowerDoWhile(n); ok || err != nil {
			return s, err
		}
		if s, ok, err := r.lowerLabeled(n); ok || err != nil {
			return s, err
		}
		return nil, &NotYetLowerable{Reason: "statement kind " + kindName(n.Kind()) + " is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "statement kind " + kindName(n.Kind()) + " is a later slice"}
	}
}

// lowerForOf lowers for (const x of arr) over an array to a Go range loop,
// for _, x := range arr.Elems(). Ranging the backing slice visits the elements
// in order, which is exactly what the array iterator does for a dense array, so
// no index arithmetic or bounds check is emitted. Only the array case is
// covered: iterating a string, a Map, a Set, or a general iterable is a later
// slice, and a destructured or already-declared loop variable hands back. The
// loop variable takes the element type from the range, so no explicit type is
// written for it.
func (r *Renderer) lowerForOf(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 3 {
		return nil, &NotYetLowerable{Reason: "only for...of with a declaration and a body is lowered yet"}
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, kids[0], &decls)
	if len(decls) != 1 {
		return nil, &NotYetLowerable{Reason: "for...of with other than a single loop binding is a later slice"}
	}
	dkids := r.prog.Children(decls[0])
	if len(dkids) != 1 || dkids[0].Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "for...of with a destructuring or annotated loop variable is a later slice"}
	}
	name, ok := localName(r.prog.Text(dkids[0]))
	if !ok {
		return nil, &NotYetLowerable{Reason: "for...of loop variable is not a Go identifier"}
	}
	// The iterable is an array (ranged over its Elems) or a string (ranged over its
	// code points). A string yields one substring per Unicode code point, so it
	// lowers to a range over CodePoints() the same way an array ranges over Elems();
	// the loop variable is the string of that code point, which is how the checker
	// types it. Any other iterable (a Set, a Map, a generator, a user iterator) is a
	// later slice and hands back.
	var elemsMethod string
	switch {
	case isArrayElem(r, kids[1]):
		elemsMethod = "Elems"
	case r.isString(kids[1]):
		elemsMethod = "CodePoints"
	default:
		return nil, &NotYetLowerable{Reason: "for...of over a non-array, non-string iterable is a later slice"}
	}
	iter, err := r.lowerExpr(kids[1])
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(kids[2])
	if err != nil {
		return nil, err
	}
	rng := &ast.RangeStmt{
		X:    &ast.CallExpr{Fun: &ast.SelectorExpr{X: iter, Sel: ident(elemsMethod)}},
		Body: body,
	}
	// A JavaScript for...of may bind a loop variable it never reads, the common
	// counting idiom `for (const c of s) n++`, but Go rejects an unused range value
	// and a `for _, _ := range` with no new variable. When the body reads the
	// binding it is ranged into the loop variable; when it does not, the loop drops
	// the binding entirely and ranges only to drive the iteration (`for range xs`).
	if r.bodyUsesName(kids[2], r.prog.Text(dkids[0])) {
		rng.Key = ident("_")
		rng.Value = ident(name)
		rng.Tok = token.DEFINE
	}
	return rng, nil
}

// isArrayElem reports whether n is an array-typed for...of iterable, the receiver
// whose element type arrayElem resolves. It wraps the arrayElem probe so the
// for...of dispatch reads as a set of iterable kinds rather than a bare ok check.
func isArrayElem(r *Renderer, n frontend.Node) bool {
	_, ok := r.arrayElem(n)
	return ok
}

// bodyUsesName reports whether the subtree rooted at n contains an identifier
// whose source text is name. lowerForOf uses it to decide whether a for...of loop
// variable is read in the body: a binding the body never mentions ranges into the
// blank identifier so the Go loop compiles. The scan is by source text, the same
// spelling the binding carries, so it sees a read however deeply nested. A body
// that shadows the binding with its own declaration of the same name is a
// pathological case this over-approximates as a use, which only keeps the binding.
func (r *Renderer) bodyUsesName(n frontend.Node, name string) bool {
	if n.Kind() == frontend.NodeIdentifier {
		return r.prog.Text(n) == name
	}
	for _, c := range r.prog.Children(n) {
		if r.bodyUsesName(c, name) {
			return true
		}
	}
	return false
}

// lowerReturn lowers a return, with or without a value.
func (r *Renderer) lowerReturn(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		return &ast.ReturnStmt{}, nil
	}
	expr, err := r.lowerExpr(kids[0])
	if err != nil {
		return nil, err
	}
	expr, err = r.coerceReturn(expr, kids[0])
	if err != nil {
		return nil, err
	}
	return &ast.ReturnStmt{Results: []ast.Expr{expr}}, nil
}

// lowerVarStatement lowers a const or let statement to Go var declarations, one
// per binding. Both const and let become a Go var, because a Go const cannot
// hold a runtime float64 initializer and TypeScript already forbids reassigning
// a const, so the mutability distinction is enforced upstream, not here. The Go
// type is always written explicitly from the checker's inferred type rather than
// left to :=, because a bare integer literal would infer Go int where the source
// means float64. A binding with no initializer, or one carrying a type
// annotation node this slice does not read yet, hands back.
func (r *Renderer) lowerVarStatement(n frontend.Node) (ast.Stmt, error) {
	var decls []frontend.Node
	collectVarDecls(r.prog, n, &decls)
	return r.varDeclStmt(decls)
}

// varDeclStmt builds a Go var declaration statement from a set of variable
// declaration nodes. It is shared by a const/let statement and a for-loop
// initializer, so both spell a binding the same way.
func (r *Renderer) varDeclStmt(decls []frontend.Node) (ast.Stmt, error) {
	if len(decls) == 0 {
		return nil, &NotYetLowerable{Reason: "variable declaration has no binding"}
	}
	// A lone float64 binding off a decimal literal reads better as name := 0.0 than as
	// var name float64 = 0, and the two declare the same block-scoped variable. Every
	// shape the fold declines (an int32 counter, a multi-binding statement, a non-decimal
	// initializer) keeps the typed var form below.
	if short, ok := r.foldFloatDecl(decls); ok {
		return short, nil
	}
	// A lone binding whose initializer already carries the binding's own type reads
	// better as name := value than as var name T = value, and infers the same type.
	// This covers a string, an array, or any local built from a constructor or a
	// bridged call, everything past the numeric literals foldFloatDecl keeps.
	if short, ok := r.foldShortDecl(decls); ok {
		return short, nil
	}
	specs := make([]ast.Spec, 0, len(decls))
	for _, d := range decls {
		kids := r.prog.Children(d)
		// A binding is [name, initializer] without an annotation, or [name, type,
		// initializer] with one. The Go type is always read from the checker's type
		// for the name, so the annotation node itself is not lowered, only skipped;
		// what it changes is the child count, and the initializer is the last child
		// in both shapes. A binding with no initializer (a bare declaration or an
		// ambient one) has no last-child value to lower and hands back, since the
		// zero-value strategy it would need is a later slice.
		if len(kids) != 2 && len(kids) != 3 {
			return nil, &NotYetLowerable{Reason: "variable binding with no initializer is a later slice"}
		}
		name, ok := localName(r.prog.Text(kids[0]))
		if !ok {
			return nil, &NotYetLowerable{Reason: "variable name is not a Go identifier"}
		}
		// A local the analysis proved holds only 32-bit integers is declared as a Go
		// int32 and its initializer is lowered in the int32 domain, so the counter or
		// accumulator lives in a register with no float64 coercion on any of its
		// operations. Every other local keeps its float64 (or richer) type and the
		// ordinary boundary-coercing initializer.
		if r.int32Locals[name] {
			init, err := r.int32Of(kids[len(kids)-1])
			if err != nil {
				return nil, err
			}
			specs = append(specs, &ast.ValueSpec{
				Names:  []*ast.Ident{ident(name)},
				Type:   ident("int32"),
				Values: []ast.Expr{init},
			})
			continue
		}
		typ, err := r.typeExpr(r.prog.TypeAt(kids[0]))
		if err != nil {
			return nil, err
		}
		init, err := r.bindingInit(kids[0], kids[len(kids)-1])
		if err != nil {
			return nil, err
		}
		specs = append(specs, &ast.ValueSpec{
			Names:  []*ast.Ident{ident(name)},
			Type:   typ,
			Values: []ast.Expr{init},
		})
	}
	return &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: specs}}, nil
}

// bindingInit lowers a binding's initializer, with one context-sensitive case an
// initializer read on its own cannot get right: an empty array literal. The
// checker types a bare [] as never[], since it has no element to infer from, so
// lowering it in isolation would instantiate value.NewArray at never. The
// annotated binding does carry the element type, so when the initializer is an
// empty array literal and the binding is an array, the element type is taken from
// the binding rather than from the literal, which is exactly the contextual type
// TypeScript itself applies here. Every other initializer lowers on its own.
func (r *Renderer) bindingInit(nameNode, initNode frontend.Node) (ast.Expr, error) {
	if initNode.Kind() == frontend.NodeArrayLiteralExpression && len(r.prog.Children(initNode)) == 0 {
		if elem, ok := r.prog.ElementType(r.prog.TypeAt(nameNode)); ok {
			elemType, err := r.typeExpr(elem)
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: index(sel("value", "NewArray"), elemType)}, nil
		}
	}
	init, err := r.lowerExpr(initNode)
	if err != nil {
		return nil, err
	}
	// A binding whose declared type crosses the dynamic boundary coerces the
	// initializer to match: a dynamic value into a static binding runs the ToNumber
	// family, and a static value into an any binding boxes. A binding whose two
	// sides agree passes the initializer through unchanged, which is every static
	// binding lowered so far.
	return r.coerceToTarget(init, initNode, nameNode)
}

// collectVarDecls gathers the variable declarations inside a variable statement.
// They sit one level down under the declaration list, which bento does not name,
// so the walk descends through it.
func collectVarDecls(prog *frontend.Program, n frontend.Node, out *[]frontend.Node) {
	for _, c := range prog.Children(n) {
		if c.Kind() == frontend.NodeVariableDeclaration {
			*out = append(*out, c)
			continue
		}
		collectVarDecls(prog, c, out)
	}
}

// lowerExprStatement lowers an expression used as a statement. The covered
// forms all update a local: a plain or compound assignment, which the checker
// exposes as a binary expression, and a prefix or postfix ++/--; a call or
// other expression statement is a later slice.
func (r *Renderer) lowerExprStatement(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "expression statement did not expose a single expression"}
	}
	return r.lowerUpdate(kids[0])
}

// flattenCommaStatement lowers a statement-position comma expression, a = 1, b = 2
// or f(), g(), to the Go statements its operands spell, evaluated left to right
// with each value discarded. That is exactly the comma operator's meaning when its
// result is thrown away, which is every use of it in statement position. It
// returns ok=false when the statement is not a comma expression, so a plain
// expression statement falls through to the single-statement path below. An
// operand the update lowering does not cover (a bare value with no effect, say)
// makes the whole statement hand back rather than emit a partial flatten.
func (r *Renderer) flattenCommaStatement(n frontend.Node) ([]ast.Stmt, bool, error) {
	if n.Kind() != frontend.NodeExpressionStatement {
		return nil, false, nil
	}
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, false, nil
	}
	ops, ok := commaOperands(r.prog, kids[0])
	if !ok {
		return nil, false, nil
	}
	stmts := make([]ast.Stmt, 0, len(ops))
	for _, op := range ops {
		s, err := r.lowerUpdate(op)
		if err != nil {
			return nil, false, err
		}
		stmts = append(stmts, s)
	}
	return stmts, true, nil
}

// flattenChainedAssignStatement lowers a statement-position chained assignment,
// a = b = 5, to the Go statements it means: the innermost assignment runs, then
// each outer target takes the value the one inside it just held. So a = b = 5
// becomes b = 5; a = b, which evaluates the right side once and settles every
// target, the same as the chain does. It returns ok=false for a plain single
// assignment, which the normal statement path already lowers, and hands back when
// a link in the chain targets something other than a local identifier.
func (r *Renderer) flattenChainedAssignStatement(n frontend.Node) ([]ast.Stmt, bool, error) {
	if n.Kind() != frontend.NodeExpressionStatement {
		return nil, false, nil
	}
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, false, nil
	}
	outer, final, ok := r.chainedAssign(kids[0])
	if !ok || len(outer) == 0 {
		return nil, false, nil
	}
	// The innermost assignment lowers on its own, b = 5. Its target then feeds the
	// next target out, and so on, so the value walks from the inside to the outside
	// exactly as the chain assigns it.
	first, err := r.lowerUpdate(final)
	if err != nil {
		return nil, false, err
	}
	stmts := []ast.Stmt{first}
	source := r.prog.Children(final)[0]
	for i := len(outer) - 1; i >= 0; i-- {
		copyStmt, err := r.assignCopy(outer[i], source)
		if err != nil {
			return nil, false, err
		}
		stmts = append(stmts, copyStmt)
		source = outer[i]
	}
	return stmts, true, nil
}

// chainedAssign walks a chain of plain assignments a = b = ... = value, returning
// the outer targets in source order and the innermost assignment node that carries
// the value. Assignment is right associative, so a = b = 5 nests as a = (b = 5);
// the walk descends the right side while it stays a plain "=" assignment. A single
// assignment has no outer targets and reports ok=false so the caller leaves it to
// the ordinary path. A compound link like a = (b += 5) stops the walk, since its
// value is not a plain copy, and that link lowers (and hands back) on its own.
func (r *Renderer) chainedAssign(n frontend.Node) ([]frontend.Node, frontend.Node, bool) {
	if n.Kind() != frontend.NodeBinaryExpression {
		return nil, n, false
	}
	kids := r.prog.Children(n)
	if len(kids) != 3 || r.prog.Text(kids[1]) != "=" {
		return nil, n, false
	}
	if inner, final, ok := r.chainedAssign(kids[2]); ok {
		return append([]frontend.Node{kids[0]}, inner...), final, true
	}
	return nil, n, true
}

// assignCopy builds target = source for one link of a chained assignment, where
// source is the target one step inside it. The source lowers as a read and coerces
// to the target's type, so a chain that crosses the dynamic boundary boxes or
// unboxes at each step the same way a written-out assignment would.
func (r *Renderer) assignCopy(target, source frontend.Node) (ast.Stmt, error) {
	if target.Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "chained assignment to a non-identifier target is a later slice"}
	}
	name, ok := localName(r.prog.Text(target))
	if !ok {
		return nil, &NotYetLowerable{Reason: "chained assignment target is not a Go identifier"}
	}
	src, err := r.lowerExpr(source)
	if err != nil {
		return nil, err
	}
	src, err = r.coerceToTarget(src, source, target)
	if err != nil {
		return nil, err
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.ASSIGN, Rhs: []ast.Expr{src}}, nil
}

// commaOperands returns the operands of a comma expression in source order, or
// ok=false when n is not a comma expression. The comma operator is left
// associative, so a, b, c parses as (a, b), c; the walk flattens that left spine
// so the operands come back in the order they were written.
func commaOperands(prog *frontend.Program, n frontend.Node) ([]frontend.Node, bool) {
	if n.Kind() != frontend.NodeBinaryExpression {
		return nil, false
	}
	kids := prog.Children(n)
	if len(kids) != 3 || prog.Text(kids[1]) != "," {
		return nil, false
	}
	var out []frontend.Node
	if left, ok := commaOperands(prog, kids[0]); ok {
		out = append(out, left...)
	} else {
		out = append(out, kids[0])
	}
	out = append(out, kids[2])
	return out, true
}

// lowerUpdate lowers a statement-position expression that mutates a local: a
// plain assignment (=), a compound assignment (+=, -=, and the rest), or a
// prefix/postfix increment or decrement. It is shared by an expression
// statement and a for-loop's post clause, both of which discard the value, so
// the prefix and postfix forms of ++/-- lower the same way.
func (r *Renderer) lowerUpdate(n frontend.Node) (ast.Stmt, error) {
	switch n.Kind() {
	case frontend.NodeBinaryExpression:
		if stmt, ok, err := r.bytesElementAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.arrayElementAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.bigIntInPlaceAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.classFieldAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.objectFieldAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.logicalAssign(n); ok || err != nil {
			return stmt, err
		}
		assign, err := r.lowerAssign(n)
		if err != nil {
			return nil, err
		}
		// A compound step of exactly one, x += 1 or x -= 1, is the increment a person
		// writes x++ or x--. Both discard the value here (a statement or a for-loop's
		// post clause), so the IncDecStmt is the same operation spelled the shorter way.
		if inc, ok := incDecFromStep(assign); ok {
			return inc, nil
		}
		return assign, nil
	case frontend.NodePrefixUnaryExpression, frontend.NodePostfixUnaryExpression:
		return r.lowerIncDec(n)
	case frontend.NodeCallExpression:
		// A call used as a statement discards its result, which Go allows for any
		// function call, so it lowers to the call wrapped in an expression
		// statement. This is how a mutating method like an array push is invoked
		// for its effect rather than its value.
		call, err := r.lowerExpr(n)
		if err != nil {
			return nil, err
		}
		return &ast.ExprStmt{X: call}, nil
	default:
		return nil, &NotYetLowerable{Reason: "expression statement that is not an assignment, update, or call is a later slice"}
	}
}

// bytesElementAssign lowers a typed-array element write a[i] = v to the buffer's
// SetAt, the store half of the typed-array indexing lowered as a read by
// elementAccess (section 6.3). It reports ok=false when the statement is not an
// element write into a typed array, so lowerUpdate falls through to lowerAssign,
// which handles the local-identifier assignments; only when the target is an
// element access whose receiver the checker types a numeric typed array does this
// claim the statement. The value coerces to the element inside SetAt with the
// element kind's store rule, so the lowering passes it as the Number the checker
// typed it. Only a plain "=" is covered: a compound write a[i] += v reads and
// writes the element and is a later slice, so it hands back for the engine rather
// than dropping the read.
func (r *Renderer) bytesElementAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" {
		return nil, false, nil
	}
	target := parts[0]
	if target.Kind() != frontend.NodeElementAccessExpression {
		return nil, false, nil
	}
	idxParts := r.prog.Children(target)
	if len(idxParts) != 2 {
		return nil, false, nil
	}
	recvNode, idxNode := idxParts[0], idxParts[1]
	if !r.numericTypedArray(recvNode) {
		return nil, false, nil
	}
	if !r.isNumber(idxNode) {
		return nil, false, &NotYetLowerable{Reason: "a typed-array write with a non-number index is a later slice"}
	}
	if !r.isNumber(parts[2]) {
		return nil, false, &NotYetLowerable{Reason: "a typed-array write of a non-number value is a later slice"}
	}
	// A write into a fixed-length integer typed array at a proven-in-range index whose
	// value is itself an integer stores straight into the backing slice,
	// recv.Data()[idx] = int8(v). The Go conversion to the element width wraps exactly
	// as the store coercion does for an integer value, so it drops the coerce function
	// pointer, the bounds branch, and the float round trip the read-modify-write loop
	// otherwise pays every iteration. A non-integer value keeps SetAt, whose coercion
	// this native store cannot stand in for.
	if info, idxNode2, ok := r.provenTypedRead(target); ok && r.int32Producing(parts[2]) {
		stmt, err := r.typedSliceStore(recvNode, idxNode2, parts[2], info)
		if err != nil {
			return nil, false, err
		}
		return stmt, true, nil
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, false, err
	}
	val, err := r.lowerExpr(parts[2])
	if err != nil {
		return nil, false, err
	}
	// A proven-integer loop index writes through SetAtI, the integer-index store that
	// takes the index already narrowed and coerces and bounds-checks exactly as SetAt
	// does. A dynamic or fractional index keeps SetAt, which truncates the Number.
	if r.intLoopIndex(idxNode) {
		idx, err := r.intIndexExpr(idxNode)
		if err != nil {
			return nil, false, err
		}
		return &ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ident("SetAtI")},
			Args: []ast.Expr{idx, val},
		}}, true, nil
	}
	idx, err := r.lowerExpr(idxNode)
	if err != nil {
		return nil, false, err
	}
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: recv, Sel: ident("SetAt")},
		Args: []ast.Expr{idx, val},
	}}, true, nil
}

// arrayElementAssign lowers a general array element write a[i] = v to the array's
// Set, the store half of the At read elementAccess lowers (section 4.0). It reports
// ok=false when the statement is not an element write into an array, so lowerUpdate
// falls through to the byte-buffer, class-field, and local-identifier paths that
// own the other targets; only when the target is an element access whose receiver
// the checker types an array does this claim the statement. A byte buffer is
// handled by bytesElementAssign before this and is not an array in the checker's
// vocabulary, so it never reaches here. The value coerces to the element type the
// same way a local assignment coerces to its target, so a widening or a boxing the
// element type needs is applied rather than dropped. Only a plain "=" is covered: a
// compound write a[i] += v reads and writes the element and is a later slice, so it
// hands back for the engine rather than dropping the read.
func (r *Renderer) arrayElementAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" {
		return nil, false, nil
	}
	target := parts[0]
	if target.Kind() != frontend.NodeElementAccessExpression {
		return nil, false, nil
	}
	idxParts := r.prog.Children(target)
	if len(idxParts) != 2 {
		return nil, false, nil
	}
	recvNode, idxNode := idxParts[0], idxParts[1]
	if _, ok := r.arrayElem(recvNode); !ok {
		return nil, false, nil
	}
	if !r.isNumber(idxNode) {
		return nil, false, &NotYetLowerable{Reason: "an array write with a non-number index is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, false, err
	}
	idx, err := r.lowerExpr(idxNode)
	if err != nil {
		return nil, false, err
	}
	val, err := r.lowerExpr(parts[2])
	if err != nil {
		return nil, false, err
	}
	val, err = r.coerceToTarget(val, parts[2], target)
	if err != nil {
		return nil, false, err
	}
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: recv, Sel: ident("Set")},
		Args: []ast.Expr{idx, val},
	}}, true, nil
}

// objectFieldAssign lowers a property write o.k = v on a plain fixed-shape object
// to the Go struct field assignment o.K = v, the store half of the o.k read
// propertyAccess lowers (section 4.1). A plain object lowers to a pointer to its
// interned struct, so the field write shows through every reference to the object,
// which is what a mutation on a JavaScript object must be. It reports ok=false when
// the statement is not a property write into a plain object, so lowerUpdate falls
// through to the accessor and local-identifier paths that own the other targets. A
// class instance is claimed by classFieldAssign before this and is excluded again
// here for good measure, and an array, a byte buffer, a Map, and a Set are not
// plain objects, so none of them reach the field store. The value coerces to the
// field type the same way a local assignment coerces to its target. Only a plain
// "=" is covered: a compound write o.k += v is a later slice, so it hands back
// rather than emitting a store that skips the read.
func (r *Renderer) objectFieldAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" {
		return nil, false, nil
	}
	target := parts[0]
	if target.Kind() != frontend.NodePropertyAccessExpression {
		return nil, false, nil
	}
	tParts := r.prog.Children(target)
	if len(tParts) != 2 {
		return nil, false, nil
	}
	obj := tParts[0]
	objType := r.prog.TypeAt(obj)
	if objType.Flags&frontend.TypeObject == 0 {
		return nil, false, nil
	}
	if _, isArray := r.prog.ElementType(objType); isArray {
		return nil, false, nil
	}
	if r.isTypedArray(obj) || r.isMap(obj) || r.isSet(obj) {
		return nil, false, nil
	}
	if _, ok := r.classReceiver(obj); ok {
		return nil, false, nil
	}
	lhs, err := r.lowerExpr(target)
	if err != nil {
		return nil, false, err
	}
	rhs, err := r.lowerExpr(parts[2])
	if err != nil {
		return nil, false, err
	}
	rhs, err = r.coerceToTarget(rhs, parts[2], target)
	if err != nil {
		return nil, false, err
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: token.ASSIGN, Rhs: []ast.Expr{rhs}}, true, nil
}

// lowerIncDec lowers a ++ or -- applied to a local number. In statement
// position the prefix and postfix forms have the same effect since the value
// is discarded, so both map to a Go IncDecStmt. Go's ++ and -- accept a
// float64, so the loop counter keeps its number type. The operand must be a
// local identifier of number type; ++/-- on anything else hands back.
func (r *Renderer) lowerIncDec(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "increment expression did not expose a single operand"}
	}
	operand := kids[0]
	op := strings.TrimSpace(strings.Trim(strings.ReplaceAll(r.prog.Text(n), r.prog.Text(operand), ""), " "))
	var tok token.Token
	switch op {
	case "++":
		tok = token.INC
	case "--":
		tok = token.DEC
	default:
		return nil, &NotYetLowerable{Reason: "update operator " + op + " is a later slice"}
	}
	// this.count++ and p.count++ on a class field are the same statement on a
	// struct field, so the field selector the class lowering builds takes the
	// place of the local identifier.
	if operand.Kind() == frontend.NodePropertyAccessExpression {
		if lhs, ok, err := r.classFieldTarget(operand); err != nil {
			return nil, err
		} else if ok {
			if !r.isNumber(operand) {
				return nil, &NotYetLowerable{Reason: "increment of a non-number needs coercion, a later slice"}
			}
			return &ast.IncDecStmt{X: lhs, Tok: tok}, nil
		}
	}
	if operand.Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "increment of a non-identifier target is a later slice"}
	}
	if !r.isNumber(operand) {
		return nil, &NotYetLowerable{Reason: "increment of a non-number needs coercion, a later slice"}
	}
	name, ok := localName(r.prog.Text(operand))
	if !ok {
		return nil, &NotYetLowerable{Reason: "increment target is not a Go identifier"}
	}
	return &ast.IncDecStmt{X: ident(name), Tok: tok}, nil
}

// lowerAssign lowers a binary assignment to a Go assignment statement. It
// covers both a plain "=" and a compound operator like "+=": a compound
// assignment desugars to x = x <op> rhs and reuses combineBinary, so a "+="
// on strings concatenates and a "%=" becomes math.Mod, exactly as the binary
// operator would. It is shared by an assignment used as a statement and a
// for-loop's post clause. The target must be a local identifier; assigning to
// a property or an element is a later slice.
func (r *Renderer) lowerAssign(bin frontend.Node) (*ast.AssignStmt, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 {
		return nil, &NotYetLowerable{Reason: "binary expression did not expose left, operator, right"}
	}
	opText := r.prog.Text(parts[1])
	baseOp, compound := compoundBaseOp(opText)
	if opText != "=" && !compound {
		return nil, &NotYetLowerable{Reason: "non-assignment expression used as a statement is a later slice"}
	}
	if parts[0].Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "assignment to a non-identifier target is a later slice"}
	}
	name, ok := localName(r.prog.Text(parts[0]))
	if !ok {
		return nil, &NotYetLowerable{Reason: "assignment target is not a Go identifier"}
	}
	var rhs ast.Expr
	var err error
	if compound {
		rhs, err = r.combineBinary(baseOp, parts[0], parts[2])
		if err != nil {
			return nil, err
		}
		// A compound + on a dynamic operand produces a boxed value.Value, so when the
		// target is a static primitive the result must coerce back to it (total += x
		// keeps total a number even though x is any). The target being dynamic needs
		// no coercion, since the boxed result already fits, and a compound whose result
		// is static reaches a static target directly.
		if r.combineIsDynamic(baseOp, parts[0], parts[2]) && !r.isDynamic(parts[0]) {
			rhs, err = r.coerceDynamicToStatic(rhs, parts[0])
			if err != nil {
				return nil, err
			}
		}
	} else if r.int32Locals[name] {
		// The target is an int32-specialized local, so the right-hand side lowers in the
		// int32 domain and the assignment to the int32 variable is the ToInt32 the value
		// would otherwise get from a | 0 or a Math.imul. The analysis only specialized
		// this local because every one of its writes is inherently int32, so int32Of has
		// a native lowering for this right-hand side rather than the coercing fallback.
		rhs, err = r.int32Of(parts[2])
		if err != nil {
			return nil, err
		}
	} else {
		rhs, err = r.lowerExpr(parts[2])
		if err != nil {
			return nil, err
		}
		rhs, err = r.coerceToTarget(rhs, parts[2], parts[0])
		if err != nil {
			return nil, err
		}
	}
	// A right-hand side that is exactly "target op value" over a native Go operator
	// collapses to Go's compound assignment, so total = total + i is written total +=
	// i the way a developer would. This fires whether the source wrote += or spelled
	// the self-reference out, and only on a bare binary whose left operand is the
	// target identifier, so a string concat (a .Concat call, not a Go +) or a
	// coercion-wrapped result is left as the plain assignment it needs to be.
	tok := token.ASSIGN
	if bin, ok := rhs.(*ast.BinaryExpr); ok {
		if ctok, ok := compoundAssignToken(bin.Op); ok {
			if x, ok := bin.X.(*ast.Ident); ok && x.Name == name {
				tok = ctok
				rhs = bin.Y
			}
		}
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{ident(name)},
		Tok: tok,
		Rhs: []ast.Expr{rhs},
	}, nil
}

// logicalAssign lowers a logical assignment used as a statement: x ??= y, x ||= y,
// or x &&= y. Unlike an arithmetic compound, these short-circuit: y is evaluated
// and stored only when x has the operator's trigger value, so the whole thing is
// an if that guards a plain assignment rather than x = x <op> y. Putting the
// assignment inside the guard keeps the short-circuit exactly, so y may have a
// side effect the eager forms could not admit.
//
// ??= assigns when x is undefined, so the target must be an optional (the
// T | undefined the Opt models), and the guard is x.IsUndefined(). ||= assigns
// when x is falsy and &&= when x is truthy, which needs JavaScript truthiness on
// x; until that lands, both are taken only for a boolean target, where falsy is
// exactly !x and truthy is x. The target must be a plain local identifier so it
// can be named in both the guard and the assignment with no repeated side effect.
// A non-logical operator reports ok=false so the caller falls through to the plain
// and arithmetic-compound path.
func (r *Renderer) logicalAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 {
		return nil, false, nil
	}
	op := r.prog.Text(parts[1])
	if op != "??=" && op != "||=" && op != "&&=" {
		return nil, false, nil
	}
	target := parts[0]
	if target.Kind() != frontend.NodeIdentifier {
		return nil, true, &NotYetLowerable{Reason: "logical assignment to a non-identifier target is a later slice"}
	}
	name, ok := localName(r.prog.Text(target))
	if !ok {
		return nil, true, &NotYetLowerable{Reason: "logical assignment target is not a Go identifier"}
	}
	// Build the guard the operator triggers on.
	var cond ast.Expr
	switch op {
	case "??=":
		if !r.isOptional(target) {
			return nil, true, &NotYetLowerable{Reason: "??= on a target that is not the optional T | undefined is a later slice"}
		}
		// A definite right-hand side leaves the target definitely present, which the
		// checker narrows to the bare T; reading it there would need a .Get() the
		// narrowing at an assignment does not yet insert, so the emitted slot (still
		// Opt[T]) and the narrowed use would disagree. When the right-hand side is
		// itself optional the target stays T | undefined, no narrowing happens, and a
		// later read flows through the existing presence test, so only that form
		// lowers here.
		if !r.isOptional(parts[2]) {
			return nil, true, &NotYetLowerable{Reason: "??= with a definite right-hand side narrows the target, which needs narrowing at an assignment, a later slice"}
		}
		cond = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("IsUndefined")}}
	case "||=":
		if !r.isBool(target) {
			return nil, true, &NotYetLowerable{Reason: "||= on a non-boolean target needs JavaScript truthiness, a later slice"}
		}
		cond = &ast.UnaryExpr{Op: token.NOT, X: ident(name)}
	case "&&=":
		if !r.isBool(target) {
			return nil, true, &NotYetLowerable{Reason: "&&= on a non-boolean target needs JavaScript truthiness, a later slice"}
		}
		cond = ident(name)
	}
	rhs, err := r.lowerExpr(parts[2])
	if err != nil {
		return nil, true, err
	}
	rhs, err = r.coerceToTarget(rhs, parts[2], target)
	if err != nil {
		return nil, true, err
	}
	assign := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(name)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{rhs},
	}
	return &ast.IfStmt{
		Cond: cond,
		Body: &ast.BlockStmt{List: []ast.Stmt{assign}},
	}, true, nil
}

// incDecFromStep rewrites a compound step of one, x += 1 or x -= 1, into Go's ++
// or --. It fires only on a bare += or -= whose right-hand side is the literal 1,
// so a step of any other size or a wider expression keeps the compound assignment.
// The value is discarded in every position lowerUpdate serves, so ++ and += 1 are
// the same operation, and ++ is how it reads.
func incDecFromStep(a *ast.AssignStmt) (*ast.IncDecStmt, bool) {
	var tok token.Token
	switch a.Tok {
	case token.ADD_ASSIGN:
		tok = token.INC
	case token.SUB_ASSIGN:
		tok = token.DEC
	default:
		return nil, false
	}
	if len(a.Lhs) != 1 || len(a.Rhs) != 1 {
		return nil, false
	}
	lit, ok := a.Rhs[0].(*ast.BasicLit)
	if !ok || (lit.Value != "1" && lit.Value != "1.0") {
		return nil, false
	}
	return &ast.IncDecStmt{X: a.Lhs[0], Tok: tok}, true
}

// compoundBaseOp maps a compound assignment operator to the binary operator it
// fuses, so combineBinary can build the x <op> rhs half of x <op>= rhs. Every
// arithmetic and bitwise compound is here, including **= which fuses to the **
// combineBinary lowers to math.Pow; the plain "=" is not a compound and returns
// false.
// compoundAssignToken maps a native Go binary operator to its compound-assignment
// form (ADD to +=, SHL to <<=, and so on), reporting whether one exists. It is the
// peephole that lets total = total + i print as total += i: every arithmetic and
// bitwise Go operator has a compound form, so the rewrite is always available when the
// right-hand side is a bare binary over the target. Comparison and logical operators
// have no compound form and return false, so a boolean assignment is left alone.
func compoundAssignToken(op token.Token) (token.Token, bool) {
	switch op {
	case token.ADD:
		return token.ADD_ASSIGN, true
	case token.SUB:
		return token.SUB_ASSIGN, true
	case token.MUL:
		return token.MUL_ASSIGN, true
	case token.QUO:
		return token.QUO_ASSIGN, true
	case token.REM:
		return token.REM_ASSIGN, true
	case token.AND:
		return token.AND_ASSIGN, true
	case token.OR:
		return token.OR_ASSIGN, true
	case token.XOR:
		return token.XOR_ASSIGN, true
	case token.SHL:
		return token.SHL_ASSIGN, true
	case token.SHR:
		return token.SHR_ASSIGN, true
	case token.AND_NOT:
		return token.AND_NOT_ASSIGN, true
	default:
		return token.ILLEGAL, false
	}
}

func compoundBaseOp(op string) (string, bool) {
	switch op {
	case "+=":
		return "+", true
	case "-=":
		return "-", true
	case "*=":
		return "*", true
	case "**=":
		return "**", true
	case "/=":
		return "/", true
	case "%=":
		return "%", true
	case "&=":
		return "&", true
	case "|=":
		return "|", true
	case "^=":
		return "^", true
	case "<<=":
		return "<<", true
	case ">>=":
		return ">>", true
	case ">>>=":
		return ">>>", true
	default:
		return "", false
	}
}

// lowerFor lowers a C-style for to Go. The classic three-clause form maps almost
// directly, but Go forbids a var declaration in a for's init clause, so a
// let-initialized loop is emitted as a Go block holding the declaration followed
// by a for with an empty init: the loop variable keeps its block scope and its
// float64 type, which a := init would lose to int inference. Only the full
// declare-condition-increment-block shape is covered; an omitted clause or an
// expression initializer hands back.
// lowerForPost lowers a for loop's post clause. A single update lowers the way it
// does anywhere else. A comma of updates, the i++, j-- a two-pointer loop walks
// with, cannot be two Go statements because a Go post clause holds exactly one, so
// its operands fuse into one parallel assignment, i, j = i + 1, j - 1, which runs
// them together the way the comma sequence does. The fuse needs every operand to
// assign a distinct local; an operand that targets a property, repeats a target,
// or is a call (which cannot sit on the left of an assignment) hands back, since
// those cannot join one parallel assignment.
func (r *Renderer) lowerForPost(n frontend.Node) (ast.Stmt, error) {
	ops, ok := commaOperands(r.prog, n)
	if !ok {
		return r.lowerUpdate(n)
	}
	var lhs, rhs []ast.Expr
	seen := make(map[string]bool, len(ops))
	for _, op := range ops {
		s, err := r.lowerUpdate(op)
		if err != nil {
			return nil, err
		}
		l, r2, ok := postAssignPair(s)
		if !ok {
			return nil, &NotYetLowerable{Reason: "a for post clause comma with an operand that is not a local assignment is a later slice"}
		}
		name, ok := l.(*ast.Ident)
		if !ok || seen[name.Name] {
			return nil, &NotYetLowerable{Reason: "a for post clause comma that repeats or computes its target is a later slice"}
		}
		seen[name.Name] = true
		lhs = append(lhs, l)
		rhs = append(rhs, r2)
	}
	return &ast.AssignStmt{Lhs: lhs, Tok: token.ASSIGN, Rhs: rhs}, nil
}

// postAssignPair rewrites one lowered update statement into the (target, value)
// pair a parallel assignment needs, so an increment, a plain assignment, and a
// compound step all reduce to target = value. An increment becomes target +/- 1;
// a compound step x <op>= v becomes x = x <op> v, the same fuse combineBinary
// undoes going the other way. A statement that is not one of these (a call) has no
// such pair and reports ok=false.
func postAssignPair(s ast.Stmt) (ast.Expr, ast.Expr, bool) {
	switch st := s.(type) {
	case *ast.IncDecStmt:
		op := token.ADD
		if st.Tok == token.DEC {
			op = token.SUB
		}
		return st.X, &ast.BinaryExpr{X: st.X, Op: op, Y: &ast.BasicLit{Kind: token.INT, Value: "1"}}, true
	case *ast.AssignStmt:
		if len(st.Lhs) != 1 || len(st.Rhs) != 1 {
			return nil, nil, false
		}
		if st.Tok == token.ASSIGN {
			return st.Lhs[0], st.Rhs[0], true
		}
		base, ok := compoundAssignBase(st.Tok)
		if !ok {
			return nil, nil, false
		}
		return st.Lhs[0], &ast.BinaryExpr{X: st.Lhs[0], Op: base, Y: st.Rhs[0]}, true
	default:
		return nil, nil, false
	}
}

// compoundAssignBase maps a Go compound-assignment token to the binary operator it
// fuses, the inverse of compoundAssignToken, so x += v can be rewritten as x + v
// for a parallel assignment. A token that is not a compound assignment returns
// false.
func compoundAssignBase(tok token.Token) (token.Token, bool) {
	switch tok {
	case token.ADD_ASSIGN:
		return token.ADD, true
	case token.SUB_ASSIGN:
		return token.SUB, true
	case token.MUL_ASSIGN:
		return token.MUL, true
	case token.QUO_ASSIGN:
		return token.QUO, true
	case token.REM_ASSIGN:
		return token.REM, true
	case token.AND_ASSIGN:
		return token.AND, true
	case token.OR_ASSIGN:
		return token.OR, true
	case token.XOR_ASSIGN:
		return token.XOR, true
	case token.SHL_ASSIGN:
		return token.SHL, true
	case token.SHR_ASSIGN:
		return token.SHR, true
	case token.AND_NOT_ASSIGN:
		return token.AND_NOT, true
	default:
		return token.ILLEGAL, false
	}
}

func (r *Renderer) lowerFor(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 4 {
		return nil, &NotYetLowerable{Reason: "only a for with a declaration, condition, and increment is lowered yet"}
	}

	var decls []frontend.Node
	collectVarDecls(r.prog, kids[0], &decls)
	if len(decls) == 0 {
		return nil, &NotYetLowerable{Reason: "for loop without a let/const initializer is a later slice"}
	}
	initDecl, err := r.varDeclStmt(decls)
	if err != nil {
		return nil, err
	}
	cond, err := r.lowerCondition(kids[1])
	if err != nil {
		return nil, err
	}
	post, err := r.lowerForPost(kids[2])
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(kids[3])
	if err != nil {
		return nil, err
	}

	// A single float64 loop variable folds into the for's own init clause, so the
	// loop reads for i := 0.0; i < n; i++ the way a developer writes it rather than a
	// block wrapping a var declaration. The block form stays for everything the fold
	// declines (an int32-specialized counter, a hex or non-literal initializer, more
	// than one loop variable), because Go's := would infer int for those and lose the
	// declared type the block's var keeps.
	if init, ok := r.foldFloatDecl(decls); ok {
		return &ast.ForStmt{Init: init, Cond: cond, Post: post, Body: body}, nil
	}
	loop := &ast.ForStmt{Cond: cond, Post: post, Body: body}
	return &ast.BlockStmt{List: []ast.Stmt{initDecl, loop}}, nil
}

// foldFloatDecl builds a Go short variable declaration from a single float64
// binding, so name := 0.0 stands in for var name float64 = 0. It serves both a
// for loop's init clause, where the short form folds into the for statement, and a
// plain const or let statement, where it reads the way a person would write it. It
// returns false, and the caller keeps the typed var form, unless there is exactly
// one binding, that binding is a plain float64 (not an int32-specialized counter),
// and its initializer is a decimal literal floatLiteral can retype: Go's := infers
// int from a bare 0, so the fold is sound only when the initializer already denotes,
// or can be spelled as, a float64.
func (r *Renderer) foldFloatDecl(decls []frontend.Node) (ast.Stmt, bool) {
	if len(decls) != 1 {
		return nil, false
	}
	kids := r.prog.Children(decls[0])
	if len(kids) != 2 && len(kids) != 3 {
		return nil, false
	}
	name, ok := localName(r.prog.Text(kids[0]))
	if !ok || r.int32Locals[name] {
		return nil, false
	}
	typ, err := r.typeExpr(r.prog.TypeAt(kids[0]))
	if err != nil || !isFloat64Ident(typ) {
		return nil, false
	}
	init, err := r.bindingInit(kids[0], kids[len(kids)-1])
	if err != nil {
		return nil, false
	}
	finit, ok := floatLiteral(init)
	if !ok {
		return nil, false
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.DEFINE, Rhs: []ast.Expr{finit}}, true
}

// foldShortDecl builds a Go short variable declaration for a lone binding whose
// initializer already carries the binding's own Go type, so name := value stands in
// for var name T = value and infers the same T. It reaches every local past the
// numeric literals: a string built from value.FromGoString, an array from
// value.NewArray, a directory from a bridged call. The safety rests on bindingInit,
// which coerces the initializer to the binding's type, so the lowered initializer is
// already of type T and := infers T exactly.
//
// It declines three shapes. More than one binding stays a grouped var. An
// int32-specialized counter keeps its explicit int32, which documents the
// specialization the analysis chose. A bare literal initializer hands back, because a
// Go untyped integer constant infers int under := where the binding means float64;
// foldFloatDecl runs first and floatifies the ones it can, and the rest keep the
// typed var.
func (r *Renderer) foldShortDecl(decls []frontend.Node) (ast.Stmt, bool) {
	if len(decls) != 1 {
		return nil, false
	}
	kids := r.prog.Children(decls[0])
	if len(kids) != 2 && len(kids) != 3 {
		return nil, false
	}
	name, ok := localName(r.prog.Text(kids[0]))
	if !ok || r.int32Locals[name] {
		return nil, false
	}
	init, err := r.bindingInit(kids[0], kids[len(kids)-1])
	if err != nil {
		return nil, false
	}
	if _, isLit := init.(*ast.BasicLit); isLit {
		return nil, false
	}
	return &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: token.DEFINE, Rhs: []ast.Expr{init}}, true
}

// isFloat64Ident reports whether a lowered type expression is the bare float64 type,
// the type every plain JavaScript number local takes. It is how the readability
// rewrites tell a number local apart from a string, boolean, or richer type whose :=
// inference is already exact.
func isFloat64Ident(typ ast.Expr) bool {
	id, ok := typ.(*ast.Ident)
	return ok && id.Name == "float64"
}

// floatLiteral retypes a numeric initializer so Go's := infers float64 from it. A
// literal already written as a float (it carries a point or an exponent) passes
// through, and a plain decimal integer literal gains a trailing .0 so 0 becomes 0.0
// and 5 becomes 5.0. A hex, binary, or octal integer literal, or anything that is not
// a bare literal, returns false: those have no short float spelling here, so the
// caller keeps the typed var form rather than change the value.
func floatLiteral(init ast.Expr) (ast.Expr, bool) {
	lit, ok := init.(*ast.BasicLit)
	if !ok {
		return nil, false
	}
	if lit.Kind == token.FLOAT {
		return lit, true
	}
	if lit.Kind != token.INT {
		return nil, false
	}
	for i := 0; i < len(lit.Value); i++ {
		if lit.Value[i] < '0' || lit.Value[i] > '9' {
			// A prefix (0x, 0b, 0o) or separator means this is not a plain decimal run,
			// so there is no bare float spelling for it.
			return nil, false
		}
	}
	return &ast.BasicLit{Kind: token.FLOAT, Value: lit.Value + ".0"}, true
}

// loopBody lowers the body of a loop, which JavaScript allows to be either a
// braced block or a single unbraced statement. A block lowers as its statements;
// a lone statement is lowered on its own and wrapped in a Go block, since a Go
// for always takes a block. This is what lets a one-line loop like
// `for (...) xs.push(f(i));` lower without the source having to add braces.
func (r *Renderer) loopBody(n frontend.Node) (*ast.BlockStmt, error) {
	if n.Kind() == frontend.NodeBlock {
		return r.lowerBlock(n)
	}
	stmts, err := r.lowerStatementMulti(n)
	if err != nil {
		return nil, err
	}
	return &ast.BlockStmt{List: stmts}, nil
}

// lowerIf lowers an if, with an optional else that is itself a block or a
// chained else-if. The condition must be a boolean expression, so a truthy
// number or object condition (JavaScript coercion) hands back until the truthy
// slice lands.
func (r *Renderer) lowerIf(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) < 2 {
		return nil, &NotYetLowerable{Reason: "if statement did not expose a condition and body"}
	}
	cond, err := r.lowerCondition(kids[0])
	if err != nil {
		return nil, err
	}
	body, err := r.lowerBodyBlock(kids[1])
	if err != nil {
		return nil, err
	}
	stmt := &ast.IfStmt{Cond: cond, Body: body}
	if len(kids) >= 3 {
		els, err := r.lowerArm(kids[2])
		if err != nil {
			return nil, err
		}
		stmt.Else = els
	}
	return stmt, nil
}

// lowerArm lowers one arm of an if: a block, a chained if for an else-if, or a
// single unbraced statement such as `else return x`, which becomes a one
// statement block so the Go if keeps its required block form.
func (r *Renderer) lowerArm(n frontend.Node) (ast.Stmt, error) {
	switch n.Kind() {
	case frontend.NodeBlock:
		return r.lowerBlock(n)
	case frontend.NodeIfStatement:
		return r.lowerIf(n)
	default:
		return r.lowerBodyBlock(n)
	}
}

// lowerBodyBlock lowers a statement that stands where a block is expected: an if
// or loop body written with braces stays a block, while a single unbraced
// statement (`if (c) return x;`) becomes a one statement block, since a Go if or
// for body is always a block. It is how the lowerer accepts the brace-optional
// bodies JavaScript allows without a special case at every use.
func (r *Renderer) lowerBodyBlock(n frontend.Node) (*ast.BlockStmt, error) {
	if n.Kind() == frontend.NodeBlock {
		return r.lowerBlock(n)
	}
	stmts, err := r.lowerStatementMulti(n)
	if err != nil {
		return nil, err
	}
	return &ast.BlockStmt{List: stmts}, nil
}

// lowerWhile lowers a while to a Go for with only a condition, Go's spelling of
// the same loop. The condition must be boolean, as for an if.
func (r *Renderer) lowerWhile(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return nil, &NotYetLowerable{Reason: "while statement did not expose a condition and body"}
	}
	cond, err := r.lowerCondition(kids[0])
	if err != nil {
		return nil, err
	}
	body, err := r.loopBody(kids[1])
	if err != nil {
		return nil, err
	}
	return &ast.ForStmt{Cond: cond, Body: body}, nil
}

// lowerCondition lowers a control-flow condition to a Go bool. A boolean operand
// lowers as itself; a non-boolean rides JavaScript truthiness through lowerTruthy,
// which reproduces the falsy set for its type, so if (n), while (s), and a ternary
// condition over a number or string lower the way the source reads them.
func (r *Renderer) lowerCondition(n frontend.Node) (ast.Expr, error) {
	return r.lowerTruthy(n)
}
