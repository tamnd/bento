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
	if len(kids) != 3 || kids[2].Kind() != frontend.NodeBlock {
		return nil, &NotYetLowerable{Reason: "only for...of with a declaration and a block body is lowered yet"}
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
	if _, ok := r.arrayElem(kids[1]); !ok {
		return nil, &NotYetLowerable{Reason: "for...of over a non-array iterable is a later slice"}
	}
	iter, err := r.lowerExpr(kids[1])
	if err != nil {
		return nil, err
	}
	body, err := r.lowerBlock(kids[2])
	if err != nil {
		return nil, err
	}
	return &ast.RangeStmt{
		Key:   ident("_"),
		Value: ident(name),
		Tok:   token.DEFINE,
		X:     &ast.CallExpr{Fun: &ast.SelectorExpr{X: iter, Sel: ident("Elems")}},
		Body:  body,
	}, nil
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
		if stmt, ok, err := r.bigIntInPlaceAssign(n); ok || err != nil {
			return stmt, err
		}
		if stmt, ok, err := r.classFieldAssign(n); ok || err != nil {
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

// bytesElementAssign lowers a Uint8Array element write a[i] = v to the buffer's
// SetAt, the store half of the byte-buffer indexing lowered as a read by
// elementAccess (section 6.3). It reports ok=false when the statement is not an
// element write into a byte buffer, so lowerUpdate falls through to lowerAssign,
// which handles the local-identifier assignments; only when the target is an
// element access whose receiver the checker types a Uint8Array does this claim the
// statement. The value coerces to a byte inside SetAt with the runtime's ToUint8,
// so the lowering passes it as the Number the checker typed it. Only a plain "="
// is covered: a compound write a[i] += v reads and writes the element and is a
// later slice, so it hands back for the engine rather than dropping the read.
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
	if !r.isBytes(recvNode) {
		return nil, false, nil
	}
	if !r.isNumber(idxNode) {
		return nil, false, &NotYetLowerable{Reason: "a Uint8Array write with a non-number index is a later slice"}
	}
	if !r.isNumber(parts[2]) {
		return nil, false, &NotYetLowerable{Reason: "a Uint8Array write of a non-number value is a later slice"}
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
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: recv, Sel: ident("SetAt")},
		Args: []ast.Expr{idx, val},
	}}, true, nil
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
// arithmetic and bitwise compound is here; the plain "=" is not a compound and
// returns false.
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
	post, err := r.lowerUpdate(kids[2])
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
	if len(kids) != 2 || kids[1].Kind() != frontend.NodeBlock {
		return nil, &NotYetLowerable{Reason: "while statement did not expose a condition and block body"}
	}
	cond, err := r.lowerCondition(kids[0])
	if err != nil {
		return nil, err
	}
	body, err := r.lowerBlock(kids[1])
	if err != nil {
		return nil, err
	}
	return &ast.ForStmt{Cond: cond, Body: body}, nil
}

// lowerCondition lowers a control-flow condition, requiring it to be typed
// boolean so the emitted Go is a real bool and not a coerced number.
func (r *Renderer) lowerCondition(n frontend.Node) (ast.Expr, error) {
	if !r.isBool(n) {
		return nil, &NotYetLowerable{Reason: "non-boolean condition needs JavaScript truthiness, a later slice"}
	}
	return r.lowerExpr(n)
}
