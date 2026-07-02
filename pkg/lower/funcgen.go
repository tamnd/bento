package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers a checked function to a runnable Go function (05_type_lowering
// sections 13 to 16). It is the first slice that emits statements and expressions
// rather than only types: a typed function with a straight-line body of returns,
// arithmetic, identifiers, and numeric literals becomes an *ast.FuncDecl the Go
// toolchain compiles. Everything outside that subset hands back a NotYetLowerable
// so the partitioner routes the unit to the engine, the same honest boundary the
// type renderer keeps (section 30).

// RenderFunc lowers a function declaration to its Go declaration: the signature
// from the checker plus a lowered body. It returns a NotYetLowerable for any
// construct the statement and expression subset does not cover yet, so a caller
// emits Go only for what lowers soundly.
func (r *Renderer) RenderFunc(fn frontend.Node) (Decl, error) {
	decl, err := r.funcDecl(fn)
	if err != nil {
		return Decl{}, err
	}
	src, err := printDecl(decl)
	if err != nil {
		return Decl{}, err
	}
	return Decl{Name: decl.Name.Name, Source: src}, nil
}

// funcDecl builds the Go declaration node for a function without printing it, so
// both RenderFunc (which prints one declaration) and the program assembler (which
// prints a whole file at once) share the one place a signature and body become a
// FuncDecl. It returns the same NotYetLowerable for an unlowerable construct.
func (r *Renderer) funcDecl(fn frontend.Node) (*ast.FuncDecl, error) {
	sym, ok := r.prog.SymbolAt(fn)
	if !ok {
		return nil, &NotYetLowerable{Reason: "function declaration has no symbol (anonymous functions are a later slice)"}
	}
	name, ok := exportedField(sym.Name)
	if !ok {
		return nil, &NotYetLowerable{Reason: "function name is not a Go identifier"}
	}

	sig, ok := r.prog.SignatureAt(fn)
	if !ok {
		return nil, &NotYetLowerable{Reason: "function has no call signature"}
	}
	if len(sig.TypeParams) != 0 {
		return nil, &NotYetLowerable{Reason: "generic function needs monomorphization, a later slice"}
	}
	if sig.RestParam != nil {
		return nil, &NotYetLowerable{Reason: "rest parameter needs the array boxing slice"}
	}

	params, err := r.paramFields(sig)
	if err != nil {
		return nil, err
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return nil, err
	}

	body, err := r.blockOf(fn)
	if err != nil {
		return nil, err
	}

	return &ast.FuncDecl{
		Name: ident(name),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}, nil
}

// paramFields lowers each parameter to a Go field with its lowered type. An
// optional parameter (one a caller may omit, so its index is at or past the
// signature's MinArgs) still hands back: its type is the optional value.Opt[T]
// now, but a call that omits the argument must synthesize the undefined optional,
// the call-site defaulting of a later slice, so lowering the parameter without it
// would emit a Go function no omitting caller could call. Its type carrying an
// explicit undefined member is not what marks it optional here, since the checker
// reports the same T | undefined type for a required parameter annotated that
// way; the caller-omittable distinction is MinArgs alone.
func (r *Renderer) paramFields(sig frontend.Signature) (*ast.FieldList, error) {
	fields := &ast.FieldList{}
	for i, p := range sig.Params {
		if i >= sig.MinArgs {
			return nil, &NotYetLowerable{Flags: p.Type.Flags, Reason: "optional parameter needs call-site defaulting, a later slice"}
		}
		pname, ok := localName(p.Name)
		if !ok {
			return nil, &NotYetLowerable{Reason: "parameter name is not a Go identifier"}
		}
		pt, err := r.typeExpr(p.Type)
		if err != nil {
			return nil, err
		}
		fields.List = append(fields.List, &ast.Field{Names: []*ast.Ident{ident(pname)}, Type: pt})
	}
	return fields, nil
}

// resultFields lowers the return type to the function's result list. A void or
// undefined return (the zero type carries no flags) is no result at all.
func (r *Renderer) resultFields(ret frontend.Type) (*ast.FieldList, error) {
	if ret.Flags == 0 || ret.Flags&frontend.TypeVoid != 0 || ret.Flags&frontend.TypeUndefined != 0 {
		return nil, nil
	}
	rt, err := r.typeExpr(ret)
	if err != nil {
		return nil, err
	}
	return &ast.FieldList{List: []*ast.Field{{Type: rt}}}, nil
}

// blockOf finds the function's body block and lowers it. A function with no body
// (an overload signature or a declare) is not a lowerable unit.
func (r *Renderer) blockOf(fn frontend.Node) (*ast.BlockStmt, error) {
	var block frontend.Node
	for _, c := range r.prog.Children(fn) {
		if c.Kind() == frontend.NodeBlock {
			block = c
		}
	}
	if block == nil {
		return nil, &NotYetLowerable{Reason: "function has no body block (declare or overload)"}
	}
	return r.lowerBlock(block)
}

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
		s, err := r.lowerStatement(n)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
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
	return r.lowerExpr(initNode)
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
		return r.lowerAssign(n)
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
	} else {
		rhs, err = r.lowerExpr(parts[2])
	}
	if err != nil {
		return nil, err
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{ident(name)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{rhs},
	}, nil
}

// compoundBaseOp maps a compound assignment operator to the binary operator it
// fuses, so combineBinary can build the x <op> rhs half of x <op>= rhs. Every
// arithmetic and bitwise compound is here; the plain "=" is not a compound and
// returns false.
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

	loop := &ast.ForStmt{Cond: cond, Post: post, Body: body}
	return &ast.BlockStmt{List: []ast.Stmt{initDecl, loop}}, nil
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
	stmt, err := r.lowerStatement(n)
	if err != nil {
		return nil, err
	}
	return &ast.BlockStmt{List: []ast.Stmt{stmt}}, nil
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
	s, err := r.lowerStatement(n)
	if err != nil {
		return nil, err
	}
	return &ast.BlockStmt{List: []ast.Stmt{s}}, nil
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

// lowerExpr lowers one expression node to a Go expression. It covers the leaves
// and operators a numeric-typed body is built from: identifiers, numeric
// literals, parentheses, and binary arithmetic on numbers.
func (r *Renderer) lowerExpr(n frontend.Node) (ast.Expr, error) {
	switch n.Kind() {
	case frontend.NodeIdentifier:
		name, ok := localName(r.prog.Text(n))
		if !ok {
			return nil, &NotYetLowerable{Reason: "identifier is not a Go identifier"}
		}
		return ident(name), nil

	case frontend.NodeParenthesizedExpression:
		kids := r.prog.Children(n)
		if len(kids) != 1 {
			return nil, &NotYetLowerable{Reason: "parenthesized expression did not wrap a single expression"}
		}
		inner, err := r.lowerExpr(kids[0])
		if err != nil {
			return nil, err
		}
		return &ast.ParenExpr{X: inner}, nil

	case frontend.NodeNumericLiteral:
		return r.numericLiteral(n)

	case frontend.NodeStringLiteral:
		return r.stringLiteral(n)

	case frontend.NodeNoSubstitutionTemplateLiteral:
		return r.noSubTemplate(n)

	case frontend.NodeTemplateExpression:
		return r.templateExpression(n)

	case frontend.NodePropertyAccessExpression:
		return r.propertyAccess(n)

	case frontend.NodeTrueKeyword:
		return ident("true"), nil

	case frontend.NodeFalseKeyword:
		return ident("false"), nil

	case frontend.NodeBinaryExpression:
		return r.binaryExpr(n)

	case frontend.NodeCallExpression:
		return r.callExpr(n)

	case frontend.NodePrefixUnaryExpression:
		return r.prefixUnary(n)

	case frontend.NodeConditionalExpression:
		return r.conditionalExpr(n)

	case frontend.NodeArrayLiteralExpression:
		return r.arrayLiteral(n)

	case frontend.NodeElementAccessExpression:
		return r.elementAccess(n)

	case frontend.NodeObjectLiteralExpression:
		return r.objectLiteral(n)

	case frontend.NodeArrowFunction:
		return r.arrowFunc(n)

	default:
		return nil, &NotYetLowerable{Reason: "expression kind " + kindName(n.Kind()) + " is a later slice"}
	}
}

// arrowFunc lowers an arrow function to a Go function literal. Only a concise
// expression body is covered, the shape a map or filter callback almost always
// takes; a block body, which needs the statement lowering to run inside a
// literal, is a later slice. Each parameter takes its type from the checker,
// which has already applied the contextual type from the call site, so a bare
// x in xs.map(x => ...) is typed number without an annotation. The result type
// comes from the body expression. This makes an arrow usable anywhere an
// expression is, but its first consumers are the higher-order array methods.
func (r *Renderer) arrowFunc(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) < 2 {
		return nil, &NotYetLowerable{Reason: "arrow function did not expose parameters and a body"}
	}
	body := kids[len(kids)-1]
	if body.Kind() == frontend.NodeBlock {
		return nil, &NotYetLowerable{Reason: "arrow function with a block body is a later slice"}
	}
	fields := make([]*ast.Field, 0, len(kids))
	for _, k := range kids[:len(kids)-1] {
		if k.Kind() != frontend.NodeParameter {
			continue
		}
		pkids := r.prog.Children(k)
		if len(pkids) != 1 || pkids[0].Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: "arrow parameter that is not a plain identifier is a later slice"}
		}
		name, ok := localName(r.prog.Text(pkids[0]))
		if !ok {
			return nil, &NotYetLowerable{Reason: "arrow parameter is not a Go identifier"}
		}
		ptype, err := r.typeExpr(r.prog.TypeAt(pkids[0]))
		if err != nil {
			return nil, err
		}
		fields = append(fields, &ast.Field{Names: []*ast.Ident{ident(name)}, Type: ptype})
	}
	retType, err := r.typeExpr(r.prog.TypeAt(body))
	if err != nil {
		return nil, err
	}
	loweredBody, err := r.lowerExpr(body)
	if err != nil {
		return nil, err
	}
	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: fields},
			Results: &ast.FieldList{List: []*ast.Field{{Type: retType}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{loweredBody}}}},
	}, nil
}

// conditionalExpr lowers a ternary cond ? whenTrue : whenFalse. Go has no
// conditional operator, so it lowers to an immediately-invoked function that
// returns one branch or the other. The function form, rather than a helper
// taking both values, is what preserves JavaScript's laziness: only the taken
// branch's expression runs, so a side effect or a call in the untaken branch
// does not fire. The condition reuses lowerCondition, so it must be a boolean
// (a truthy number or object condition hands back until truthiness lands), and
// the result type comes from the checker's type for the whole expression, which
// is the common supertype of the two branches.
func (r *Renderer) conditionalExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 5 || r.prog.Text(kids[1]) != "?" || r.prog.Text(kids[3]) != ":" {
		return nil, &NotYetLowerable{Reason: "conditional expression did not expose condition, true, and false branches"}
	}
	cond, err := r.lowerCondition(kids[0])
	if err != nil {
		return nil, err
	}
	whenTrue, err := r.lowerExpr(kids[2])
	if err != nil {
		return nil, err
	}
	whenFalse, err := r.lowerExpr(kids[4])
	if err != nil {
		return nil, err
	}
	// The IIFE's return type is the branches' widened primitive, not the checker's
	// type for the whole expression: TypeScript types a ternary as the union of
	// its branch types, so two string literals give a "a" | "b" literal union and
	// a chained numeric ternary gives a numeric-literal union, neither of which is
	// the value.BStr or float64 the branch expressions actually produce. Both
	// branches must widen to the same primitive; a ternary that mixes types (or
	// whose branches are objects) needs the tagged union and hands back.
	retType, kind, ok := r.condBranchType(kids[2])
	_, otherKind, otherOK := r.condBranchType(kids[4])
	if !ok || !otherOK || kind != otherKind {
		return nil, &NotYetLowerable{Reason: "conditional whose branches are not both the same primitive type needs a union, a later slice"}
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: retType}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.IfStmt{
				Cond: cond,
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{whenTrue}}}},
			},
			&ast.ReturnStmt{Results: []ast.Expr{whenFalse}},
		}},
	}
	return &ast.CallExpr{Fun: lit}, nil
}

// condBranchType reports the Go type a ternary branch lowers to and a name for
// its primitive kind, seeing through parentheses and a nested ternary so a
// chained a ? x : b ? y : z types by its leaf primitive rather than by the
// checker's literal union. It returns ok == false for a branch that is not a
// number, string, or boolean, so a ternary over objects or mixed types hands
// back rather than guessing a return type.
func (r *Renderer) condBranchType(n frontend.Node) (ast.Expr, string, bool) {
	switch {
	case r.isNumber(n):
		return ident("float64"), "number", true
	case r.isBool(n):
		return ident("bool"), "bool", true
	case r.isString(n):
		r.requireImport(valuePkg)
		return sel("value", "BStr"), "string", true
	}
	switch n.Kind() {
	case frontend.NodeParenthesizedExpression:
		if kids := r.prog.Children(n); len(kids) == 1 {
			return r.condBranchType(kids[0])
		}
	case frontend.NodeConditionalExpression:
		if kids := r.prog.Children(n); len(kids) == 5 {
			return r.condBranchType(kids[2])
		}
	}
	return nil, "", false
}

// callExpr lowers a call to a top-level function. The callee must be an
// identifier that resolves to a function symbol, lowered to the same exported Go
// name RenderFunc gives the declaration, so a call and its target agree. Calling
// a local closure, a method, or a value is a later slice. Arguments lower
// positionally; a spread or a defaulted or omitted argument hands back.
func (r *Renderer) callExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		return nil, &NotYetLowerable{Reason: "call expression exposed no callee"}
	}
	// A member callee (s.charCodeAt(...)) is a method call, not a plain function
	// call; the string methods are the only ones covered so far.
	if kids[0].Kind() == frontend.NodePropertyAccessExpression {
		return r.methodCall(kids[0], kids[1:])
	}
	if kids[0].Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "call to a non-identifier callee is a later slice"}
	}
	// A call to a name bound by a node: import is a call to a host builtin, not a
	// user function, so it routes to the value helper the builtin maps to before the
	// user-function path, which would reject the alias symbol the binding carries.
	if b, ok := r.nodeImports[r.prog.Text(kids[0])]; ok {
		return r.nodeBuiltinCall(b, kids[1:])
	}
	// A bare call to an ambient global function (isNaN, isFinite) is not a call to
	// a user binding, so it routes to the global-function lowering before the
	// user-function path, which would otherwise reject it.
	if goName, ok := globalFn(r.prog.Text(kids[0])); ok && r.isAmbientGlobal(kids[0]) {
		return r.globalFnCall(goName, kids[0], kids[1:])
	}
	// String(x) called as a function is a primitive-to-string coercion, an ambient
	// global constructor call rather than a user function, so it routes before the
	// user-function path.
	if r.prog.Text(kids[0]) == "String" && r.isAmbientGlobal(kids[0]) {
		return r.stringCoercion(kids[1:])
	}
	// Number(x) called as a function is a primitive-to-number coercion, the
	// companion to String(x), and routes the same way before the user path.
	if r.prog.Text(kids[0]) == "Number" && r.isAmbientGlobal(kids[0]) {
		return r.numberCoercion(kids[1:])
	}
	// Boolean(x) called as a function is the third primitive coercion, and routes
	// the same way as String and Number before the user path.
	if r.prog.Text(kids[0]) == "Boolean" && r.isAmbientGlobal(kids[0]) {
		return r.booleanCoercion(kids[1:])
	}
	// parseFloat is a bare ambient global that reads a number from the front of a
	// string, so it routes like the coercions before the user path.
	if r.prog.Text(kids[0]) == "parseFloat" && r.isAmbientGlobal(kids[0]) {
		return r.parseFloatCall(kids[1:])
	}
	// parseInt takes an optional radix, so it has its own lowering rather than the
	// single-argument coercion shape, but routes the same way before the user path.
	if r.prog.Text(kids[0]) == "parseInt" && r.isAmbientGlobal(kids[0]) {
		return r.parseIntCall(kids[1:])
	}
	sym, ok := r.prog.SymbolAt(kids[0])
	if !ok || sym.Flags&frontend.SymbolFunction == 0 {
		return nil, &NotYetLowerable{Reason: "call to a callee that is not a top-level function is a later slice"}
	}
	name, ok := exportedField(sym.Name)
	if !ok {
		return nil, &NotYetLowerable{Reason: "called function name is not a Go identifier"}
	}
	args := make([]ast.Expr, 0, len(kids)-1)
	for _, a := range kids[1:] {
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	return &ast.CallExpr{Fun: ident(name), Args: args}, nil
}

// methodCall lowers a call whose callee is a member expression. The only
// receivers covered so far are strings, whose methods map to value.BStr methods,
// so the receiver must type as string and the method must be one bento maps.
// Every string method covered here takes number arguments, so a non-number
// argument hands back rather than mistyping the Go call. A method on any other
// receiver, or an unmapped string method, is its own later slice.
func (r *Renderer) methodCall(callee frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(callee)
	if len(kids) != 2 {
		return nil, &NotYetLowerable{Reason: "method callee did not expose a receiver and a method name"}
	}
	recvNode, method := kids[0], r.prog.Text(kids[1])
	// process.stdout.write(s) and process.stderr.write(s) are the process output
	// streams. The receiver is not a value, it is the ambient stream, so the call
	// lowers to a value write helper rather than a method on a runtime object.
	if stream, ok := r.processStream(recvNode); ok {
		return r.processStreamCall(stream, method, argNodes)
	}
	// console.log(...) and friends are calls on the global console, not a value
	// receiver, so they lower to the value console helpers rather than a method on a
	// runtime object. This is the print path a developer reaches for by default.
	if r.isGlobalRef(recvNode, "console") {
		return r.consoleCall(method, argNodes)
	}
	// Math.floor(x) and friends are calls on the global Math namespace, not a
	// value receiver, so they lower to the Go math package rather than a method.
	if r.isGlobalRef(recvNode, "Math") {
		return r.mathCall(method, argNodes)
	}
	// Number.isInteger(x) and friends are static calls on the global Number, which
	// lower to value package predicates.
	if r.isGlobalRef(recvNode, "Number") {
		return r.numberCall(method, argNodes)
	}
	// String.fromCharCode(...) is a static call on the global String constructor,
	// not a method on a string value, so it lowers to a value constructor before
	// the string-method path below, which expects a string receiver.
	if r.isGlobalRef(recvNode, "String") {
		return r.stringStaticCall(method, argNodes)
	}
	// JSON.stringify(x) is a static call on the global JSON namespace, not a method
	// on a value, so it lowers to the value JSON serializer before the
	// receiver-value paths below. JSON.parse waits on the dynamic value box.
	if r.isGlobalRef(recvNode, "JSON") {
		return r.jsonCall(method, argNodes)
	}
	// A method on an array receiver lowers to a value.Array method. This routes
	// before the primitive and string paths, which expect a number, boolean, or
	// string receiver an array is not.
	if _, ok := r.arrayElem(recvNode); ok {
		return r.arrayMethodCall(recvNode, method, argNodes)
	}
	// toString and valueOf on a number or a boolean value are the first methods on
	// a non-string receiver: they lower to the same coercion a String() call or a
	// bare use would take, so they route here before the string-method path.
	if r.isNumber(recvNode) || r.isBool(recvNode) {
		return r.primitiveValueCall(recvNode, method, argNodes)
	}
	if !r.isString(recvNode) {
		return nil, &NotYetLowerable{Reason: "method call on a non-string receiver is a later slice"}
	}
	// replace and replaceAll with a regexp literal first argument are their own
	// path: a plain-literal pattern (no metacharacters) is exactly the string
	// search the value replace methods do, so it lowers when the pattern is plain
	// and the flags are a subset bento models, and hands back otherwise so a real
	// pattern routes to the engine rather than compiling a wrong search.
	if method == "replace" || method == "replaceAll" {
		if len(argNodes) >= 1 {
			if pattern, flags, isRe := r.regexLiteralArg(argNodes[0]); isRe {
				return r.regexReplaceCall(recvNode, method, pattern, flags, argNodes)
			}
		}
	}
	goName, params, minArgs, variadic, ok := stringMethod(method)
	if !ok {
		return nil, &NotYetLowerable{Reason: "string method ." + method + " is a later slice"}
	}
	if len(argNodes) < minArgs || (!variadic && len(argNodes) > len(params)) {
		return nil, &NotYetLowerable{Reason: "string method ." + method + " with this argument count is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for i, a := range argNodes {
		// A variadic method repeats its last argument kind for every argument past
		// the declared list, so concat's trailing string arguments all check as
		// strings.
		idx := i
		if idx >= len(params) {
			idx = len(params) - 1
		}
		kind := params[idx]
		if !r.argHasKind(a, kind) {
			return nil, &NotYetLowerable{Reason: "string method ." + method + " with an argument of the wrong type is a later slice"}
		}
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}, Args: args}, nil
}

// mathCall lowers a call on the global Math namespace to the matching function in
// the Go math package. Every Math method covered here takes numbers and returns a
// number, so the argument count must match exactly and each argument must type as
// number; anything else hands back rather than emitting a mistyped call. The
// receiver is not lowered, since Math is a namespace, not a value: it becomes the
// math package qualifier.
func (r *Renderer) mathCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	pkg, goName, minArity, maxArity, ok := mathMethod(method)
	if !ok {
		return nil, &NotYetLowerable{Reason: "Math." + method + " is a later slice"}
	}
	if len(argNodes) < minArity || (maxArity >= 0 && len(argNodes) > maxArity) {
		return nil, &NotYetLowerable{Reason: "Math." + method + " with this argument count is a later slice"}
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		if !r.isNumber(a) {
			return nil, &NotYetLowerable{Reason: "Math." + method + " with a non-number argument is a later slice"}
		}
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	if pkg == valuePkg {
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", goName), Args: args}, nil
	}
	r.requireImport("math")
	return &ast.CallExpr{Fun: sel("math", goName), Args: args}, nil
}

// numberCall lowers a static call on the global Number namespace to the matching
// predicate in the value package. Each covered method takes one number and
// returns a boolean, so the argument count must be one and the argument must type
// as number; anything else hands back. Like Math, the Number receiver is not
// lowered to a value, since it is a namespace, not a runtime object.
func (r *Renderer) numberCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	// Number.parseInt and Number.parseFloat are the same function objects as the
	// global parseInt and parseFloat by specification, so they lower through the
	// exact same paths rather than a separate mapping. They take a string and
	// return a number, unlike the predicates below that take a number.
	switch method {
	case "parseInt":
		return r.parseIntCall(argNodes)
	case "parseFloat":
		return r.parseFloatCall(argNodes)
	}
	goName, ok := numberMethod(method)
	if !ok {
		return nil, &NotYetLowerable{Reason: "Number." + method + " is a later slice"}
	}
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Number." + method + " with this argument count is a later slice"}
	}
	if !r.isNumber(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Number." + method + " on a non-number argument is a later slice"}
	}
	arg, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", goName), Args: []ast.Expr{arg}}, nil
}

// stringStaticCall lowers a static call on the global String constructor. Only
// fromCharCode is covered here: it takes any number of number arguments, coerces
// each to a UTF-16 code unit, and returns a string, so it maps to the variadic
// value.FromCharCode. fromCodePoint waits for the exception machinery, since it
// throws a RangeError on a code point outside the Unicode range. Like Math and
// Number, String is a namespace on this path, not a value, so the receiver is
// not lowered.
func (r *Renderer) stringStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "fromCharCode" {
		return nil, &NotYetLowerable{Reason: "String." + method + " is a later slice"}
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		if !r.isNumber(a) {
			return nil, &NotYetLowerable{Reason: "String.fromCharCode with a non-number argument is a later slice"}
		}
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "FromCharCode"), Args: args}, nil
}

// jsonCall lowers a static call on the global JSON namespace. Only stringify is
// covered here: it takes a single value and returns the exact text V8 produces,
// which lowers to value.JSONStringify with the argument boxed as any so the
// serializer's reflection walk can dispatch on its concrete type. A replacer or
// a space argument (the second and third parameters) changes the output, so a
// call that passes one hands back rather than ignoring it. JSON.parse produces a
// dynamic any value, which waits on the value box, so it hands back here.
func (r *Renderer) jsonCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "stringify" {
		return nil, &NotYetLowerable{Reason: "JSON." + method + " is a later slice"}
	}
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "JSON.stringify with a replacer or space argument is a later slice"}
	}
	arg, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "JSONStringify"), Args: []ast.Expr{arg}}, nil
}

// primitiveValueCall lowers toString and valueOf on a number or boolean value.
// Both take no arguments here: number.toString with a radix throws a RangeError
// on a radix outside 2..36, which waits on the exception machinery, so a call
// with any argument hands back. toString is the same coercion String(x) already
// uses, a number through value.NumberToString and a boolean through
// value.BoolToString, so it shares stringify to stay in step with it. valueOf
// returns the primitive itself, so it lowers to the receiver expression
// unchanged. Any other method on a primitive receiver is a later slice.
func (r *Renderer) primitiveValueCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	// number.toString(radix) is the one primitive method with an argument this
	// slice covers. The radix must be a literal in 2..36 so no RangeError can fire
	// (a bad radix throws, which waits on the exception machinery, and a dynamic
	// radix cannot be range-checked at compile time). A radix of 10 is the same
	// coercion String(x) runs, so it routes through stringify; any other radix
	// lowers to value.NumberToStringRadix with the literal folded in.
	if method == "toString" && len(argNodes) == 1 && r.isNumber(recvNode) {
		radix, ok := r.literalIntArg(argNodes[0], 2, 36)
		if !ok {
			return nil, &NotYetLowerable{Reason: "number toString with a non-literal or out-of-range radix is a later slice"}
		}
		if radix == 10 {
			return r.stringify(recvNode)
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{
			Fun:  sel("value", "NumberToStringRadix"),
			Args: []ast.Expr{recv, &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(radix)}},
		}, nil
	}
	// number.toFixed(digits) formats with a fixed number of fraction digits. The
	// digit count must be a literal in 0..100 for the same reason the radix must:
	// a count outside that range throws a RangeError, and a dynamic count cannot be
	// range-checked at compile time. An omitted count means zero. It lowers to
	// value.NumberToFixed, which rounds the exact double the way the specification
	// does.
	if method == "toFixed" && len(argNodes) <= 1 && r.isNumber(recvNode) {
		digits := 0
		if len(argNodes) == 1 {
			d, ok := r.literalIntArg(argNodes[0], 0, 100)
			if !ok {
				return nil, &NotYetLowerable{Reason: "number toFixed with a non-literal or out-of-range digit count is a later slice"}
			}
			digits = d
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{
			Fun:  sel("value", "NumberToFixed"),
			Args: []ast.Expr{recv, &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(digits)}},
		}, nil
	}
	if len(argNodes) != 0 {
		return nil, &NotYetLowerable{Reason: "primitive method ." + method + " with arguments is a later slice"}
	}
	switch method {
	case "toString":
		return r.stringify(recvNode)
	case "valueOf":
		return r.lowerExpr(recvNode)
	default:
		return nil, &NotYetLowerable{Reason: "primitive method ." + method + " is a later slice"}
	}
}

// literalIntArg reads a numeric-literal argument whose ToInteger value lands in
// [lo, hi], returning that integer. It is the shared compile-time guard for the
// number formatting methods whose argument must be in a fixed range or throw: a
// toString radix in 2..36 and a toFixed digit count in 0..100. A non-literal
// argument (which cannot be checked at compile time) or a literal that truncates
// outside the range returns false, so the caller hands back rather than emit a
// call that could throw a RangeError with no handler in place.
func (r *Renderer) literalIntArg(n frontend.Node, lo, hi int) (int, bool) {
	if n.Kind() != frontend.NodeNumericLiteral {
		return 0, false
	}
	v, ok := numericLiteralValue(r.prog.Text(n))
	if !ok {
		return 0, false
	}
	i := int(v) // ToInteger truncates toward zero.
	if i < lo || i > hi {
		return 0, false
	}
	return i, true
}

// numberMethod maps a JavaScript Number static predicate to the value function
// that implements it. Only the non-coercing predicates are here: isNaN, isFinite,
// isInteger, and isSafeInteger, whose meaning on a number argument is exact.
// Number.parseFloat and parseInt are handled before this map in numberCall, since
// they route to the same lowering as the global parseInt/parseFloat. The Number(x)
// coercion call is a call on Number itself rather than a static method, so it is
// handled on the coercion path.
func numberMethod(name string) (goName string, ok bool) {
	switch name {
	case "isNaN":
		return "NumberIsNaN", true
	case "isFinite":
		return "NumberIsFinite", true
	case "isInteger":
		return "NumberIsInteger", true
	case "isSafeInteger":
		return "NumberIsSafeInteger", true
	default:
		return "", false
	}
}

// mathMethod maps a JavaScript Math method to the Go function that computes the
// same value, with the package it lives in and the accepted argument count as a
// [minArity, maxArity] range (maxArity of -1 means unbounded). Most map straight
// onto the Go math package: floor, ceil, trunc, abs, and sqrt are IEEE operations
// that agree bit for bit, and pow folds two numbers with the same NaN and
// signed-zero rules. min and max map to the value package because JavaScript lets
// them take any number of arguments where math.Min and math.Max take exactly two,
// so value.MinN and value.MaxN fold a whole argument list with the same identity,
// NaN, and signed-zero rules. round and sign also map to value: Math.round breaks
// a tie toward +Infinity where math.Round rounds away from zero, and Go has no
// math.Sign at all. fround, clz32, and imul map to value too, but for a different
// reason than round: they are integer or single-precision operations, so they are
// bit-exact and agree with the engine to the last bit the way the transcendental
// functions cannot. The transcendental functions (cbrt, exp, expm1, the logs, the
// trig and inverse-trig and hyperbolic families, atan2, and the two-argument
// hypot) map straight onto the Go math package too, but their last-bit results are
// not guaranteed identical across two libm implementations, so they are proven by
// the equivalence harness's numeric-tolerance mode rather than by an exact match.
// hypot stays two-argument because math.Hypot takes exactly two; the variadic
// Math.hypot(a, b, c) hands back until a value.HypotN folds a list.
func mathMethod(name string) (pkg, goName string, minArity, maxArity int, ok bool) {
	switch name {
	case "floor":
		return "math", "Floor", 1, 1, true
	case "ceil":
		return "math", "Ceil", 1, 1, true
	case "trunc":
		return "math", "Trunc", 1, 1, true
	case "abs":
		return "math", "Abs", 1, 1, true
	case "sqrt":
		return "math", "Sqrt", 1, 1, true
	case "pow":
		return "math", "Pow", 2, 2, true
	case "min":
		return valuePkg, "MinN", 0, -1, true
	case "max":
		return valuePkg, "MaxN", 0, -1, true
	case "round":
		return valuePkg, "Round", 1, 1, true
	case "sign":
		return valuePkg, "Sign", 1, 1, true
	case "fround":
		return valuePkg, "Fround", 1, 1, true
	case "clz32":
		return valuePkg, "Clz32", 1, 1, true
	case "imul":
		return valuePkg, "Imul", 2, 2, true
	case "cbrt":
		return "math", "Cbrt", 1, 1, true
	case "exp":
		return "math", "Exp", 1, 1, true
	case "expm1":
		return "math", "Expm1", 1, 1, true
	case "log":
		return "math", "Log", 1, 1, true
	case "log2":
		return "math", "Log2", 1, 1, true
	case "log10":
		return "math", "Log10", 1, 1, true
	case "log1p":
		return "math", "Log1p", 1, 1, true
	case "sin":
		return "math", "Sin", 1, 1, true
	case "cos":
		return "math", "Cos", 1, 1, true
	case "tan":
		return "math", "Tan", 1, 1, true
	case "asin":
		return "math", "Asin", 1, 1, true
	case "acos":
		return "math", "Acos", 1, 1, true
	case "atan":
		return "math", "Atan", 1, 1, true
	case "atan2":
		return "math", "Atan2", 2, 2, true
	case "sinh":
		return "math", "Sinh", 1, 1, true
	case "cosh":
		return "math", "Cosh", 1, 1, true
	case "tanh":
		return "math", "Tanh", 1, 1, true
	case "asinh":
		return "math", "Asinh", 1, 1, true
	case "acosh":
		return "math", "Acosh", 1, 1, true
	case "atanh":
		return "math", "Atanh", 1, 1, true
	case "hypot":
		return "math", "Hypot", 2, 2, true
	default:
		return "", "", 0, 0, false
	}
}

// isGlobalRef reports whether n is a reference to the ambient global named name,
// like Math, rather than a user binding that happens to share the name. It checks
// the identifier text and then requires every declaration of the resolved symbol
// to live in a .d.ts library file, so a local `const Math = ...` that adds a
// source-file declaration is correctly excluded and its methods do not lower to
// the Go math package.
func (r *Renderer) isGlobalRef(n frontend.Node, name string) bool {
	if r.prog.Text(n) != name {
		return false
	}
	return r.isAmbientGlobal(n)
}

// isAmbientGlobal reports whether n resolves to a symbol declared only in .d.ts
// library files, the test that separates an ambient global from a user binding
// that shadows the same name. isGlobalRef adds a name check on top; the bare
// global-function path uses this directly, having already matched the name.
func (r *Renderer) isAmbientGlobal(n frontend.Node) bool {
	if n.Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(n)
	if !ok {
		return false
	}
	decls := r.prog.Declarations(sym)
	if len(decls) == 0 {
		return false
	}
	for _, d := range decls {
		if d.File().Kind != frontend.FileDTS {
			return false
		}
	}
	return true
}

// processStream reports whether n refers to process.stdout or process.stderr,
// the ambient output streams, and returns which one. It matches a property access
// whose object is the ambient global process and whose property is stdout or
// stderr, so a user object that happens to carry a stdout field does not match and
// its methods do not lower to the process write helpers.
func (r *Renderer) processStream(n frontend.Node) (string, bool) {
	if n.Kind() != frontend.NodePropertyAccessExpression {
		return "", false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return "", false
	}
	if !r.isGlobalRef(kids[0], "process") {
		return "", false
	}
	switch prop := r.prog.Text(kids[1]); prop {
	case "stdout", "stderr":
		return prop, true
	default:
		return "", false
	}
}

// processStreamCall lowers a call on a process output stream. Only write is
// covered: it takes a single string and lowers to value.WriteStdout or
// value.WriteStderr, which write the string's UTF-8 view to the file descriptor
// and return the boolean write reports. A different method, a different arity, or
// a non-string argument hands back rather than emitting a mistyped call.
func (r *Renderer) processStreamCall(stream, method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "write" {
		return nil, &NotYetLowerable{Reason: "process." + stream + "." + method + " is a later slice"}
	}
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "process." + stream + ".write with this argument count is a later slice"}
	}
	if !r.isString(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "process." + stream + ".write of a non-string is a later slice"}
	}
	arg, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	goName := "WriteStdout"
	if stream == "stderr" {
		goName = "WriteStderr"
	}
	return &ast.CallExpr{Fun: sel("value", goName), Args: []ast.Expr{arg}}, nil
}

// consoleCall lowers a call on the global console. The methods that write to
// standard output (log, info, debug) lower to value.ConsoleLog, and the ones that
// write to standard error (error, warn) to value.ConsoleError. Each argument is
// stringified with the same ECMAScript ToString a String() call uses, so a
// number, boolean, or string prints exactly as Node's console does for that
// primitive, and the parts join with a space and a trailing newline inside the
// helper. An argument this slice cannot stringify (an object, whose inspect runs
// richer formatting) hands back rather than printing the wrong text.
func (r *Renderer) consoleCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	var goName string
	switch method {
	case "log", "info", "debug":
		goName = "ConsoleLog"
	case "error", "warn":
		goName = "ConsoleError"
	default:
		return nil, &NotYetLowerable{Reason: "console." + method + " is a later slice"}
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		part, err := r.stringify(a)
		if err != nil {
			return nil, err
		}
		args = append(args, part)
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", goName), Args: args}, nil
}

// globalFn maps a bare global function name to the value function that implements
// it. Only the two number predicates are here: the global isNaN and isFinite
// coerce their argument to a number and then test it, so on an argument that
// already types as number they are exactly value.NumberIsNaN and
// value.NumberIsFinite, the same functions Number.isNaN and Number.isFinite use.
// The global parseInt and parseFloat, which parse a string, are a later slice.
func globalFn(name string) (goName string, ok bool) {
	switch name {
	case "isNaN":
		return "NumberIsNaN", true
	case "isFinite":
		return "NumberIsFinite", true
	default:
		return "", false
	}
}

// globalFnCall lowers a bare call to an ambient global function. The covered
// functions take one number and return a boolean, so the argument count must be
// one and the argument must type as number; anything else hands back rather than
// emitting a call whose coercion this slice does not model. calleeNode is the
// callee identifier, used only for its position in the error.
func (r *Renderer) globalFnCall(goName string, calleeNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	name := r.prog.Text(calleeNode)
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: name + " with this argument count is a later slice"}
	}
	if !r.isNumber(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: name + " on a non-number argument is a later slice"}
	}
	arg, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", goName), Args: []ast.Expr{arg}}, nil
}

// stringCoercion lowers String(x) called as a function over a primitive argument.
// A number goes through value.NumberToString (the exact ECMAScript
// Number::toString, not strconv), a boolean through value.BoolToString, and a
// string is already a value.BStr so it passes through unchanged, the identity
// String(s) is. It takes exactly one argument; a different arity, or an argument
// this slice does not coerce (an object, whose ToString runs user code), hands
// back.
func (r *Renderer) stringCoercion(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "String() with this argument count is a later slice"}
	}
	return r.stringify(argNodes[0])
}

// stringify lowers one expression to its string form under the ECMAScript ToString
// used by String(x) and by a template literal substitution: a number goes through
// value.NumberToString (the exact Number::toString, not strconv), a boolean
// through value.BoolToString, and a string is already a value.BStr so it passes
// through unchanged. An argument this slice does not coerce (an object, whose
// ToString runs user code) hands back. String(x) and `${x}` share this so the two
// paths always agree on how a value becomes a string.
func (r *Renderer) stringify(arg frontend.Node) (ast.Expr, error) {
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	switch {
	case r.isString(arg):
		return lowered, nil // already a string, the identity
	case r.isNumber(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "NumberToString"), Args: []ast.Expr{lowered}}, nil
	case r.isBool(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "BoolToString"), Args: []ast.Expr{lowered}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "coercing this type to a string is a later slice"}
	}
}

// numberCoercion lowers Number(x) called as a function over a primitive argument.
// A string goes through value.StringToNumber (the exact ECMAScript ToNumber over
// the StrNumericLiteral grammar, not strconv), a boolean through value.BoolToNumber
// (true is 1, false is 0), and a number is already a float64 so it passes through
// unchanged. It takes exactly one argument; a different arity, or an argument this
// slice does not coerce (an object, whose valueOf runs user code), hands back.
func (r *Renderer) numberCoercion(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Number() with this argument count is a later slice"}
	}
	arg := argNodes[0]
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	switch {
	case r.isNumber(arg):
		return lowered, nil // Number(n) on a number is the identity
	case r.isString(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "StringToNumber"), Args: []ast.Expr{lowered}}, nil
	case r.isBool(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "BoolToNumber"), Args: []ast.Expr{lowered}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Number() on this argument type is a later slice"}
	}
}

// booleanCoercion lowers Boolean(x) called as a function over a primitive argument,
// the third primitive coercion. A number goes through value.NumberToBool (false
// only at zero or NaN), a string through value.StringToBool (false only when
// empty), and a boolean passes through unchanged since Boolean(b) on a boolean is
// the identity. It takes exactly one argument; a different arity, or an argument
// this slice does not coerce (an object, which is always truthy but whose
// evaluation this slice does not model), hands back.
func (r *Renderer) booleanCoercion(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Boolean() with this argument count is a later slice"}
	}
	arg := argNodes[0]
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	switch {
	case r.isBool(arg):
		return lowered, nil // Boolean(b) on a boolean is the identity
	case r.isNumber(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "NumberToBool"), Args: []ast.Expr{lowered}}, nil
	case r.isString(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "StringToBool"), Args: []ast.Expr{lowered}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Boolean() on this argument type is a later slice"}
	}
}

// parseFloatCall lowers parseFloat(s) over a string argument to value.ParseFloat,
// the lenient prefix parse. It takes exactly one string argument; a different
// arity, or a non-string argument (which parseFloat would coerce to a string
// first, running that conversion), hands back.
func (r *Renderer) parseFloatCall(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "parseFloat with this argument count is a later slice"}
	}
	arg := argNodes[0]
	if !r.isString(arg) {
		return nil, &NotYetLowerable{Reason: "parseFloat on a non-string argument is a later slice"}
	}
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "ParseFloat"), Args: []ast.Expr{lowered}}, nil
}

// parseIntCall lowers parseInt(s) and parseInt(s, radix) to value.ParseInt. The
// first argument must be a string; the optional second must be a number and
// becomes the radix, while an omitted radix lowers to the literal 0, which
// value.ParseInt treats (as the specification does) the same as an omitted
// argument. A different arity or an argument of the wrong type hands back.
func (r *Renderer) parseIntCall(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) < 1 || len(argNodes) > 2 {
		return nil, &NotYetLowerable{Reason: "parseInt with this argument count is a later slice"}
	}
	if !r.isString(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "parseInt on a non-string argument is a later slice"}
	}
	str, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	// The radix argument, or the literal 0 when it is omitted.
	var radix ast.Expr = &ast.BasicLit{Kind: token.FLOAT, Value: "0"}
	if len(argNodes) == 2 {
		if !r.isNumber(argNodes[1]) {
			return nil, &NotYetLowerable{Reason: "parseInt with a non-number radix is a later slice"}
		}
		radix, err = r.lowerExpr(argNodes[1])
		if err != nil {
			return nil, err
		}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "ParseInt"), Args: []ast.Expr{str, radix}}, nil
}

// argKind names the primitive type a string method expects for one argument, so
// methodCall guards each argument against the checker's type before lowering it.
type argKind int

const (
	argNumber argKind = iota
	argString
)

// argHasKind reports whether the checker types n as the primitive the method
// expects at that position, the guard that keeps a mistyped argument out of a
// method call rather than emitting Go that would not compile or would coerce.
func (r *Renderer) argHasKind(n frontend.Node, k argKind) bool {
	switch k {
	case argString:
		return r.isString(n)
	default:
		return r.isNumber(n)
	}
}

// stringMethod maps a JavaScript string method to the value.BStr method that
// implements it, the primitive kind of each argument, and the minimum number of
// arguments a call must supply. The argument kinds let methodCall guard a
// string-taking method (indexOf) apart from a number-taking one (charCodeAt).
// minArgs below len(params) marks the trailing arguments optional: slice and
// substring take zero, one, or two numbers, and their Go methods are variadic so
// one signature covers every arity, the count selecting the defaults. The kinds
// need not all match: padStart takes a required number then an optional string,
// so the guard admits one or two arguments and still checks each against its
// declared kind. A call always passes exactly the arguments the source wrote, so
// the emitted call form is the same whether the method is variadic or not.
func stringMethod(name string) (goName string, params []argKind, minArgs int, variadic bool, ok bool) {
	switch name {
	case "charCodeAt":
		return "CharCodeAt", []argKind{argNumber}, 1, false, true
	case "charAt":
		return "CharAt", []argKind{argNumber}, 1, false, true
	case "indexOf":
		return "IndexOf", []argKind{argString, argNumber}, 1, false, true
	case "lastIndexOf":
		return "LastIndexOf", []argKind{argString, argNumber}, 1, false, true
	case "includes":
		return "Includes", []argKind{argString, argNumber}, 1, false, true
	case "replace":
		// Only the string-pattern, string-replacement form lowers; a regexp pattern
		// or a replacer function argument does not type as a string, so methodCall
		// hands it back. Both arguments are required, so it is not variadic.
		return "Replace", []argKind{argString, argString}, 2, false, true
	case "replaceAll":
		return "ReplaceAll", []argKind{argString, argString}, 2, false, true
	case "split":
		// Only the string-separator form lowers, to value.BStr.Split returning a
		// string array; a regexp separator does not type as a string, so methodCall
		// hands it back, and the optional limit argument is a later slice, so exactly
		// one argument is admitted.
		return "Split", []argKind{argString}, 1, false, true
	case "startsWith":
		return "StartsWith", []argKind{argString, argNumber}, 1, false, true
	case "endsWith":
		return "EndsWith", []argKind{argString, argNumber}, 1, false, true
	case "slice":
		return "Slice", []argKind{argNumber, argNumber}, 0, false, true
	case "substring":
		return "Substring", []argKind{argNumber, argNumber}, 0, false, true
	case "substr":
		// The legacy start-and-length form: a required start and an optional length,
		// both numbers, so it admits one or two arguments like slice and substring.
		return "Substr", []argKind{argNumber, argNumber}, 1, false, true
	case "padStart":
		return "PadStart", []argKind{argNumber, argString}, 1, false, true
	case "padEnd":
		return "PadEnd", []argKind{argNumber, argString}, 1, false, true
	case "concat":
		// concat takes any number of string arguments, so it is variadic over a
		// single repeating string kind and has no upper bound.
		return "ConcatN", []argKind{argString}, 0, true, true
	case "repeat":
		// repeat takes exactly one number, the count. value.Repeat coerces it the
		// way String.prototype.repeat does and treats a negative or non-finite count
		// as the RangeError it is, so a bad count is caught at runtime rather than
		// miscompiled.
		return "Repeat", []argKind{argNumber}, 1, false, true
	case "toUpperCase":
		return "ToUpperCase", nil, 0, false, true
	case "toLowerCase":
		return "ToLowerCase", nil, 0, false, true
	case "trim":
		return "Trim", nil, 0, false, true
	case "trimStart":
		return "TrimStart", nil, 0, false, true
	case "trimEnd":
		return "TrimEnd", nil, 0, false, true
	default:
		return "", nil, 0, false, false
	}
}

// prefixUnary lowers a prefix unary expression. The operator is not a child
// node, so it is read as the part of the node's text before the operand.
// Negation on a number and logical not on a boolean map to their Go unary
// operators, and a unary plus on a number is the no-op it is in both languages,
// so the operand passes through. The increment and decrement operators mutate
// their operand and are a later slice.
func (r *Renderer) prefixUnary(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "prefix expression did not expose a single operand"}
	}
	operand := kids[0]
	op := strings.TrimSpace(strings.TrimSuffix(r.prog.Text(n), r.prog.Text(operand)))
	switch op {
	case "-":
		if !r.isNumber(operand) {
			return nil, &NotYetLowerable{Reason: "unary minus on a non-number is a later slice"}
		}
		x, err := r.lowerExpr(operand)
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: token.SUB, X: x}, nil
	case "+":
		if !r.isNumber(operand) {
			return nil, &NotYetLowerable{Reason: "unary plus on a non-number is a later slice"}
		}
		return r.lowerExpr(operand)
	case "!":
		if !r.isBool(operand) {
			return nil, &NotYetLowerable{Reason: "logical not on a non-boolean needs truthiness, a later slice"}
		}
		x, err := r.lowerExpr(operand)
		if err != nil {
			return nil, err
		}
		return &ast.UnaryExpr{Op: token.NOT, X: x}, nil
	case "~":
		// Bitwise NOT is the unary member of the bitwise family: it coerces its
		// operand to a 32-bit integer, complements it, and returns the result as a
		// number, so it lowers to float64(^value.ToInt32(x)), the same coercion the
		// binary bitwise operators use, not a Go ^ on the float64.
		if !r.isNumber(operand) {
			return nil, &NotYetLowerable{Reason: "bitwise not on a non-number is a later slice"}
		}
		x, err := r.lowerExpr(operand)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		conv := &ast.CallExpr{Fun: sel("value", "ToInt32"), Args: []ast.Expr{x}}
		return &ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{&ast.UnaryExpr{Op: token.XOR, X: conv}}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "prefix operator " + op + " is a later slice"}
	}
}

// stringLiteral lowers a string literal to a value.BStr. The literal's runtime
// content is not its source text: the source carries backslash escapes, so it is
// decoded into UTF-16 code units first (decodeJSString). A content that decodes to
// valid UTF-16 becomes a Go string literal wrapped in value.FromGoString, the
// common case. A content that decodes to a lone surrogate, which a \u escape can
// name and which no Go string can hold, is emitted as a raw []uint16 wrapped in
// value.FromUTF16 so the surrogate survives. A content that does not decode (a
// malformed escape) hands back.
func (r *Renderer) stringLiteral(n frontend.Node) (ast.Expr, error) {
	text := r.prog.Text(n)
	if len(text) < 2 {
		return nil, &NotYetLowerable{Reason: "string literal source too short to lower"}
	}
	quote := text[0]
	if (quote != '"' && quote != '\'') || text[len(text)-1] != quote {
		return nil, &NotYetLowerable{Reason: "unusual string literal quoting is a later slice"}
	}
	units, ok := decodeJSString(text[1 : len(text)-1])
	if !ok {
		return nil, &NotYetLowerable{Reason: "string literal has a malformed escape sequence"}
	}
	return r.bstrLit(units), nil
}

// bstrLit builds the AST for a value.BStr holding the given UTF-16 code units. A
// content that is valid UTF-16 becomes a Go string literal wrapped in
// value.FromGoString, the common case; a content that carries a lone surrogate,
// which no Go string can hold, is emitted as a raw []uint16 wrapped in
// value.FromUTF16 so the surrogate survives. The string literal and template
// paths share this so both spell a compile-time string the same way.
func (r *Renderer) bstrLit(units []uint16) ast.Expr {
	r.requireImport(valuePkg)
	if hasLoneSurrogate(units) {
		return &ast.CallExpr{Fun: sel("value", "FromUTF16"), Args: []ast.Expr{uint16SliceLit(units)}}
	}
	lit := &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(string(utf16.Decode(units)))}
	return &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{lit}}
}

// noSubTemplate lowers a template literal with no substitutions, `like this`,
// which denotes exactly one string. Its cooked content is the source between the
// backticks with escapes resolved, so it lowers to the same value.BStr a string
// literal of that content would, only the delimiters differ.
func (r *Renderer) noSubTemplate(n frontend.Node) (ast.Expr, error) {
	units, ok := templateCooked(r.prog.Text(n))
	if !ok {
		return nil, &NotYetLowerable{Reason: "template literal has a malformed escape sequence"}
	}
	return r.bstrLit(units), nil
}

// templateExpression lowers a template literal with substitutions, `a${x}b`, to a
// single string. The frontend exposes it as a head literal followed by one span
// per substitution, each span holding the interpolated expression and the literal
// text that follows it (a middle, or the tail at the end). The result is the head
// concatenated with, for each span, the expression coerced to a string and then
// the following literal, so `a${x}b${y}c` becomes head "a", String(x), "b",
// String(y), "c" joined in order. The expressions coerce through stringify, the
// same ToString String(x) uses, so a template and an explicit String() call agree.
// The join is one ConcatN on the head, which materializes the result once rather
// than building an intermediate string per piece. An expression whose type does
// not coerce (an object) hands the whole template back.
func (r *Renderer) templateExpression(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) < 2 {
		return nil, &NotYetLowerable{Reason: "template expression did not expose a head and at least one span"}
	}
	headUnits, ok := templateCooked(r.prog.Text(kids[0]))
	if !ok {
		return nil, &NotYetLowerable{Reason: "template head has a malformed escape sequence"}
	}
	// pieces is the flat ordered list of string values to join: the head, then for
	// each span the coerced expression and the literal that follows it.
	pieces := []ast.Expr{r.bstrLit(headUnits)}
	for _, span := range kids[1:] {
		parts := r.prog.Children(span)
		if len(parts) != 2 {
			return nil, &NotYetLowerable{Reason: "template span did not expose an expression and a literal"}
		}
		strExpr, err := r.stringify(parts[0])
		if err != nil {
			return nil, err
		}
		litUnits, ok := templateCooked(r.prog.Text(parts[1]))
		if !ok {
			return nil, &NotYetLowerable{Reason: "template literal part has a malformed escape sequence"}
		}
		pieces = append(pieces, strExpr, r.bstrLit(litUnits))
	}
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: pieces[0], Sel: ident("ConcatN")},
		Args: pieces[1:],
	}, nil
}

// templateCooked decodes the cooked value of one template literal token: the head,
// a middle, the tail, or a whole no-substitution literal. The raw source carries
// the delimiters the parser matched, so they are stripped first, a leading
// backtick (a head or whole literal) or close brace (a middle or the tail), and a
// trailing "${" before a substitution or a backtick at the end. What remains is
// the same escaped content a string literal holds between its quotes, so
// decodeJSString resolves it, including \` and \$ which stand for themselves. It
// returns false when the delimiters are not the expected shape or an escape is
// malformed, so the caller hands the template back rather than guessing.
func templateCooked(text string) ([]uint16, bool) {
	if len(text) < 2 {
		return nil, false
	}
	if text[0] != '`' && text[0] != '}' {
		return nil, false
	}
	inner := text[1:]
	switch {
	case strings.HasSuffix(inner, "${"):
		inner = inner[:len(inner)-2]
	case strings.HasSuffix(inner, "`"):
		inner = inner[:len(inner)-1]
	default:
		return nil, false
	}
	return decodeJSString(inner)
}

// uint16SliceLit builds the AST for a []uint16{...} composite literal of the given
// code units, each written as a hex constant so a reader sees the code units the
// way the string tables do.
func uint16SliceLit(units []uint16) ast.Expr {
	elts := make([]ast.Expr, len(units))
	for i, u := range units {
		elts[i] = &ast.BasicLit{Kind: token.INT, Value: "0x" + strconv.FormatUint(uint64(u), 16)}
	}
	return &ast.CompositeLit{
		Type: &ast.ArrayType{Elt: ident("uint16")},
		Elts: elts,
	}
}

// propertyAccess lowers a member expression. Two members are covered: .length on
// a string, which is the code-unit count and lowers to the value.BStr Length
// method, a float64 that matches the number type the checker gives .length; and a
// numeric constant on the Math or Number namespace (Math.PI, Number.EPSILON, and
// their siblings), which is a property read on a global rather than a method call,
// so it lowers to the matching value-package constant. Every other property (a
// field of a lowered object, a method call, .length on an array) is its own later
// slice and hands back.
func (r *Renderer) propertyAccess(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return nil, &NotYetLowerable{Reason: "property access did not expose an object and a property name"}
	}
	obj, nameNode := kids[0], kids[1]
	prop := r.prog.Text(nameNode)
	if r.isString(obj) && prop == "length" {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Length")}}, nil
	}
	if prop == "length" {
		if _, ok := r.arrayElem(obj); ok {
			recv, err := r.lowerExpr(obj)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Len")}}, nil
		}
	}
	if r.isGlobalRef(obj, "Math") {
		if e, ok := mathConstant(prop); ok {
			r.requireImport(valuePkg)
			return e, nil
		}
		return nil, &NotYetLowerable{Reason: "Math." + prop + " as a value is a later slice"}
	}
	if r.isGlobalRef(obj, "Number") {
		if e, ok := numberConstant(prop); ok {
			r.requireImport(valuePkg)
			return e, nil
		}
		return nil, &NotYetLowerable{Reason: "Number." + prop + " as a value is a later slice"}
	}
	// A plain read o.k on a fixed-shape object lowers to the Go struct field the
	// shape's property interns to. The field name comes from the same exportedField
	// mapping and the same internStruct registration the object literal and the
	// type renderer use, so a read and the value it reads agree on the field. A
	// shape that does not lower (an optional property, say) hands back through
	// internStruct rather than reading a field that was never declared.
	objType := r.prog.TypeAt(obj)
	if objType.Flags&frontend.TypeObject != 0 {
		if _, isArray := r.prog.ElementType(objType); !isArray {
			field, ok := exportedField(prop)
			if !ok {
				return nil, &NotYetLowerable{Reason: "property name ." + prop + " is not a Go identifier"}
			}
			if _, err := r.decls.internStruct(r, objType); err != nil {
				return nil, err
			}
			recv, err := r.lowerExpr(obj)
			if err != nil {
				return nil, err
			}
			return &ast.SelectorExpr{X: recv, Sel: ident(field)}, nil
		}
	}
	return nil, &NotYetLowerable{Reason: "property access ." + prop + " on this type is a later slice"}
}

// mathConstant maps a Math namespace property name to the value-package constant
// that holds the exact double the specification names. Only the eight numeric
// constants are covered; a method name (Math.floor and the like) is a function
// value, not a number, and hands back.
func mathConstant(prop string) (ast.Expr, bool) {
	name, ok := map[string]string{
		"E":       "MathE",
		"LN10":    "MathLN10",
		"LN2":     "MathLN2",
		"LOG10E":  "MathLOG10E",
		"LOG2E":   "MathLOG2E",
		"PI":      "MathPI",
		"SQRT1_2": "MathSQRT12",
		"SQRT2":   "MathSQRT2",
	}[prop]
	if !ok {
		return nil, false
	}
	return sel("value", name), true
}

// numberConstant maps a Number namespace property name to its value-package
// counterpart. The finite constants are named constants; the three non-finite
// ones (the infinities and NaN) cannot be Go constants, so they lower to a call
// that builds the value.
func numberConstant(prop string) (ast.Expr, bool) {
	switch prop {
	case "EPSILON":
		return sel("value", "NumberEpsilon"), true
	case "MAX_SAFE_INTEGER":
		return sel("value", "NumberMaxSafeInteger"), true
	case "MIN_SAFE_INTEGER":
		return sel("value", "NumberMinSafeInteger"), true
	case "MAX_VALUE":
		return sel("value", "NumberMaxValue"), true
	case "MIN_VALUE":
		return sel("value", "NumberMinValue"), true
	case "POSITIVE_INFINITY":
		return &ast.CallExpr{Fun: sel("value", "NumberPositiveInfinity")}, true
	case "NEGATIVE_INFINITY":
		return &ast.CallExpr{Fun: sel("value", "NumberNegativeInfinity")}, true
	case "NaN":
		return &ast.CallExpr{Fun: sel("value", "NumberNaN")}, true
	}
	return nil, false
}

// numericLiteral lowers a number literal. number is float64, and a well-formed
// JavaScript literal denotes the same float64 whether it is written in decimal,
// hex, binary, or octal, with digit separators, or with an exponent, so
// decodeNumericLiteral validates the value and returns the cleaned Go literal for
// it. A BigInt or a value that overflows to Infinity is not a float64 this slice
// lowers and hands back.
func (r *Renderer) numericLiteral(n frontend.Node) (ast.Expr, error) {
	text := r.prog.Text(n)
	value, kind, ok := decodeNumericLiteral(text)
	if !ok {
		return nil, &NotYetLowerable{Reason: "numeric literal " + text + " is not a finite number this slice lowers"}
	}
	return &ast.BasicLit{Kind: kind, Value: value}, nil
}

// binaryExpr lowers a binary expression on two operands of the same primitive
// type. On two numbers the arithmetic operators map directly on float64 and the
// relational and equality operators map to Go comparisons that yield bool. On
// two booleans the short-circuit && and || map to Go's &&/||, which evaluate in
// the same order and short-circuit the same way, and === / !== map to Go
// ==/!=. The operand types are guarded because + on strings is a different-typed
// concatenation and === on objects is reference identity, each its own later
// slice. An assignment (the "=" operator) is a statement form and is handled
// there, so as a value it hands back. The children are left, operator, right,
// the shape the frontend exposes.
func (r *Renderer) binaryExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 3 {
		return nil, &NotYetLowerable{Reason: "binary expression did not expose left, operator, right"}
	}
	left, op, right := kids[0], kids[1], kids[2]
	opText := r.prog.Text(op)
	if opText == "=" {
		return nil, &NotYetLowerable{Reason: "assignment used as a value is a later slice"}
	}
	return r.combineBinary(opText, left, right)
}

// combineBinary lowers a JavaScript binary operator applied to two operand
// nodes to the Go expression with the same meaning. It is the shared core of
// binaryExpr and of a compound assignment (x += y desugars to x = x + y), so
// the string, remainder, and bitwise special cases apply the same way whether
// the operator was written on its own or fused to an assignment.
func (r *Renderer) combineBinary(opText string, left, right frontend.Node) (ast.Expr, error) {
	// + where either operand is a string is concatenation of a UTF-16 string, not
	// a Go string +, which would be UTF-8, and not a Go operator at all since bstr
	// is a struct. JavaScript + is string concatenation as soon as one operand is a
	// string: the other operand is coerced to a string with the same ToString the
	// value model runs, so a number becomes value.NumberToString and a boolean
	// value.BoolToString before both are joined with value.Concat, which picks the
	// wider backing form and copies once (section 5). It is handled before the
	// operator table so the string path emits a call rather than reaching the
	// number/bool dispatch, and a string operand against a non-primitive (an object
	// or array, whose ToString is a later slice) hands back through stringifyOperand.
	if opText == "+" && (r.isString(left) || r.isString(right)) {
		l, err := r.stringifyOperand(left)
		if err != nil {
			return nil, err
		}
		rr, err := r.stringifyOperand(right)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "Concat"), Args: []ast.Expr{l, rr}}, nil
	}

	// === and !== on two strings compare by UTF-16 code unit, which is what
	// JavaScript string equality does and what value.BStr.Equal implements. A Go
	// == on the struct would compare backing fields instead and call two strings
	// unequal when one is UTF-8 backed and the other UTF-16 backed but they hold
	// the same code units. Handled before the operator table so the string path
	// emits the method call, negated for !==.
	if (opText == "===" || opText == "!==") && r.isString(left) && r.isString(right) {
		l, err := r.lowerExpr(left)
		if err != nil {
			return nil, err
		}
		rr, err := r.lowerExpr(right)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		eq := &ast.CallExpr{Fun: &ast.SelectorExpr{X: l, Sel: ident("Equal")}, Args: []ast.Expr{rr}}
		if opText == "!==" {
			return &ast.UnaryExpr{Op: token.NOT, X: eq}, nil
		}
		return eq, nil
	}

	// The relational operators on two strings order them by UTF-16 code unit, the
	// Abstract Relational Comparison, which value.BStr.Compare implements as a
	// three-way result. A Go relational operator on the struct would not compile,
	// and comparing the UTF-8 views with Go's < would misorder any string past the
	// BMP. So each operator lowers to a comparison of Compare against zero: a < b is
	// a.Compare(b) < 0, and so on. Handled before the operator table so the string
	// path emits the method call.
	if relOp, ok := relationalToken(opText); ok && r.isString(left) && r.isString(right) {
		l, err := r.lowerExpr(left)
		if err != nil {
			return nil, err
		}
		rr, err := r.lowerExpr(right)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		cmp := &ast.CallExpr{Fun: &ast.SelectorExpr{X: l, Sel: ident("Compare")}, Args: []ast.Expr{rr}}
		return &ast.BinaryExpr{X: cmp, Op: relOp, Y: &ast.BasicLit{Kind: token.INT, Value: "0"}}, nil
	}

	// === and !== against undefined, where the other operand is an optional
	// (T | undefined, lowered to value.Opt[T]), is a presence test, not a value
	// compare. undefined has no Go value to put on the right of a ==, and the
	// optional carries its presence in a flag, so the comparison lowers to the
	// optional's IsUndefined: x === undefined is x.IsUndefined() and x !== undefined
	// is its negation. Only the undefined operand is skipped; the optional operand
	// is lowered normally to receive the method. A === or !== between two optionals,
	// or an optional against a defined value, is a value compare and not this case,
	// so both operands being checked keeps this to the undefined-literal shape.
	if opText == "===" || opText == "!==" {
		if optNode, ok := r.optionalUndefinedCompare(left, right); ok {
			opt, err := r.lowerExpr(optNode)
			if err != nil {
				return nil, err
			}
			check := &ast.CallExpr{Fun: &ast.SelectorExpr{X: opt, Sel: ident("IsUndefined")}}
			if opText == "!==" {
				return &ast.UnaryExpr{Op: token.NOT, X: check}, nil
			}
			return check, nil
		}
	}

	// Remainder on numbers is the one arithmetic operator that is not a Go binary
	// operator: JavaScript % is fmod (a floating remainder that keeps the sign of
	// the dividend), which Go spells math.Mod, not the integer-only % token. It is
	// handled here, before the operator table, so the number path can emit a call.
	if opText == "%" && r.isNumber(left) && r.isNumber(right) {
		l, err := r.lowerExpr(left)
		if err != nil {
			return nil, err
		}
		rr, err := r.lowerExpr(right)
		if err != nil {
			return nil, err
		}
		r.requireImport("math")
		return &ast.CallExpr{Fun: sel("math", "Mod"), Args: []ast.Expr{l, rr}}, nil
	}

	// The bitwise operators on numbers do not work on float64: JavaScript coerces
	// each operand to a 32-bit integer, operates, and turns the result back into a
	// number. So they cannot be a plain Go operator on the float64 values; they
	// wrap the operands in value.ToInt32/ToUint32 and the result in a float64 cast.
	// Handled here, before the operator table, so the number path can emit that
	// form rather than a bare Go bitwise operator that would reject a float.
	if goOp, shift, unsignedLeft, ok := bitwiseOp(opText); ok && r.isNumber(left) && r.isNumber(right) {
		return r.bitwiseExpr(goOp, shift, unsignedLeft, left, right)
	}

	goOp, err := r.binaryOp(opText, left, right)
	if err != nil {
		return nil, err
	}

	l, err := r.lowerExpr(left)
	if err != nil {
		return nil, err
	}
	rr, err := r.lowerExpr(right)
	if err != nil {
		return nil, err
	}
	return &ast.BinaryExpr{X: l, Op: goOp, Y: rr}, nil
}

// optionalUndefinedCompare recognizes an equality between an optional and the
// bare undefined literal and returns the optional operand. One operand must type
// as exactly undefined (the undefined keyword, flags TypeUndefined) and the other
// must be an optional (a union whose members are the T | undefined shape). It
// returns false when neither operand is the undefined literal, when both are, or
// when the non-undefined operand is not an optional, so the caller only rewrites
// the genuine presence test and leaves every other equality to the value compare.
func (r *Renderer) optionalUndefinedCompare(left, right frontend.Node) (frontend.Node, bool) {
	lUndef := r.prog.TypeAt(left).Flags == frontend.TypeUndefined
	rUndef := r.prog.TypeAt(right).Flags == frontend.TypeUndefined
	switch {
	case rUndef && !lUndef && r.isOptional(left):
		return left, true
	case lUndef && !rUndef && r.isOptional(right):
		return right, true
	default:
		return nil, false
	}
}

// isOptional reports whether a node's type is an optional, the T | undefined
// shape that lowers to value.Opt[T]. It reads the type as a union and checks the
// optional shape the same way renderUnion does, so the presence-test rewrite
// fires exactly when the operand is a value.Opt and not on a wider union.
func (r *Renderer) isOptional(n frontend.Node) bool {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeUnion == 0 {
		return false
	}
	_, ok := r.optionalInner(r.prog.UnionMembers(t))
	return ok
}

// relationalToken maps a TypeScript relational operator to the Go comparison
// token that has the same meaning against a three-way compare result: a < b is
// Compare(a, b) < 0, and the other three follow the same shape. Only the four
// ordering operators are here; equality is not relational and lowers separately.
func relationalToken(op string) (token.Token, bool) {
	switch op {
	case "<":
		return token.LSS, true
	case "<=":
		return token.LEQ, true
	case ">":
		return token.GTR, true
	case ">=":
		return token.GEQ, true
	default:
		return token.ILLEGAL, false
	}
}

// bitwiseOp maps a TypeScript bitwise operator to the Go token that computes it
// on the coerced integers, and reports how the operands are coerced. shift is
// true for the three shift operators, whose right operand is a shift count masked
// to five bits rather than a second full operand. unsignedLeft is true only for
// the unsigned right shift >>>, whose left operand is coerced with ToUint32 so Go
// does a logical shift and the result is non-negative; every other operator
// coerces the left operand with ToInt32. Arithmetic-versus-logical right shift is
// carried entirely by the operand's signedness, since Go's >> is arithmetic on a
// signed type and logical on an unsigned one, exactly matching >> and >>>.
func bitwiseOp(op string) (goOp token.Token, shift, unsignedLeft, ok bool) {
	switch op {
	case "&":
		return token.AND, false, false, true
	case "|":
		return token.OR, false, false, true
	case "^":
		return token.XOR, false, false, true
	case "<<":
		return token.SHL, true, false, true
	case ">>":
		return token.SHR, true, false, true
	case ">>>":
		return token.SHR, true, true, true
	default:
		return token.ILLEGAL, false, false, false
	}
}

// bitwiseExpr lowers a bitwise expression on two numbers. The operands are
// coerced with value.ToInt32 (or ToUint32 for the left operand of >>>), the Go
// bitwise operator runs on the integers, and the result is cast back to float64
// because a JavaScript bitwise result is a number. For a shift, the right operand
// is a count masked to the low five bits (value.ToUint32(r) & 31), the ECMAScript
// rule that a shift by 32 is a shift by 0.
func (r *Renderer) bitwiseExpr(goOp token.Token, shift, unsignedLeft bool, left, right frontend.Node) (ast.Expr, error) {
	l, err := r.lowerExpr(left)
	if err != nil {
		return nil, err
	}
	rr, err := r.lowerExpr(right)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	leftConv := "ToInt32"
	if unsignedLeft {
		leftConv = "ToUint32"
	}
	lx := &ast.CallExpr{Fun: sel("value", leftConv), Args: []ast.Expr{l}}
	var inner ast.Expr
	if shift {
		count := &ast.ParenExpr{X: &ast.BinaryExpr{
			X:  &ast.CallExpr{Fun: sel("value", "ToUint32"), Args: []ast.Expr{rr}},
			Op: token.AND,
			Y:  &ast.BasicLit{Kind: token.INT, Value: "31"},
		}}
		inner = &ast.BinaryExpr{X: lx, Op: goOp, Y: count}
	} else {
		rx := &ast.CallExpr{Fun: sel("value", "ToInt32"), Args: []ast.Expr{rr}}
		inner = &ast.BinaryExpr{X: lx, Op: goOp, Y: rx}
	}
	return &ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{inner}}, nil
}

// stringifyOperand lowers an operand of a string concatenation to a value.BStr,
// coercing it the way JavaScript + does once the other operand is a string. A
// string operand is already a BStr and passes through; a number becomes
// value.NumberToString and a boolean value.BoolToString, the exact ToString the
// value model runs so a concatenated number reads the same bytes String(x) would
// and matches the engine. A non-primitive operand, whose ToString needs the
// object machinery, hands back for a later slice rather than emitting a coercion
// that does not exist yet.
func (r *Renderer) stringifyOperand(n frontend.Node) (ast.Expr, error) {
	e, err := r.lowerExpr(n)
	if err != nil {
		return nil, err
	}
	switch {
	case r.isString(n):
		return e, nil
	case r.isNumber(n):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "NumberToString"), Args: []ast.Expr{e}}, nil
	case r.isBool(n):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "BoolToString"), Args: []ast.Expr{e}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "string concatenation with a non-primitive operand is a later slice"}
	}
}

// binaryOp picks the Go operator for a TypeScript binary operator given its
// operand types. It dispatches on the operand types so a number path and a
// boolean path stay separate and each guards its own sound operator set; mixed
// or non-primitive operands hand back for a later slice.
func (r *Renderer) binaryOp(opText string, left, right frontend.Node) (token.Token, error) {
	switch {
	case r.isNumber(left) && r.isNumber(right):
		goOp, ok := numericBinaryOp(opText)
		if !ok {
			return token.ILLEGAL, &NotYetLowerable{Reason: "binary operator " + opText + " on numbers is a later slice"}
		}
		return goOp, nil
	case r.isBool(left) && r.isBool(right):
		goOp, ok := booleanBinaryOp(opText)
		if !ok {
			return token.ILLEGAL, &NotYetLowerable{Reason: "binary operator " + opText + " on booleans is a later slice"}
		}
		return goOp, nil
	default:
		return token.ILLEGAL, &NotYetLowerable{Reason: "binary operator on mixed or non-primitive operands is a later slice"}
	}
}

// isNumber reports whether the checker types n as number, the guard that keeps
// the arithmetic path sound while string and mixed operands wait for their slice.
func (r *Renderer) isNumber(n frontend.Node) bool {
	return r.prog.TypeAt(n).Flags&frontend.TypeNumber != 0
}

// isBool reports whether the checker types n as boolean, the guard that keeps a
// control-flow condition a real Go bool rather than a coerced value.
func (r *Renderer) isBool(n frontend.Node) bool {
	return r.prog.TypeAt(n).Flags&frontend.TypeBoolean != 0
}

// isString reports whether the checker types n as string, the guard that routes
// + to value.Concat and .length to value.BStr.Length rather than to a number or
// object path.
func (r *Renderer) isString(n frontend.Node) bool {
	return r.prog.TypeAt(n).Flags&frontend.TypeString != 0
}

// arrayElem reports whether the checker types n as an array, returning the
// lowered Go element type when so. TypeObject covers both arrays and fixed-shape
// objects in the frontend vocabulary, so an element type is what distinguishes
// the two, the same test typeExpr uses to route an array type to renderArray. A
// hand-back on the element type (an element that does not lower yet) reads here
// as "not a lowerable array", so the caller hands the whole construct back.
func (r *Renderer) arrayElem(n frontend.Node) (ast.Expr, bool) {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return nil, false
	}
	elem, ok := r.prog.ElementType(t)
	if !ok {
		return nil, false
	}
	e, err := r.typeExpr(elem)
	if err != nil {
		return nil, false
	}
	return e, true
}

// arrayLiteral lowers an array literal [e0, e1, ...] to a value.NewArray call
// instantiated at the element type. The element type is taken from the checker's
// type for the whole literal, not guessed from the elements, so a widened or
// empty literal is spelled the way the checker sees it and NewArray's type
// argument is explicit rather than inferred from a possibly empty argument list.
// Only a literal of plain element expressions whose element type lowers is
// covered; a spread element or an elided hole hands back to a later slice.
func (r *Renderer) arrayLiteral(n frontend.Node) (ast.Expr, error) {
	elemType, ok := r.arrayElem(n)
	if !ok {
		return nil, &NotYetLowerable{Reason: "array literal whose element type does not lower yet"}
	}
	kids := r.prog.Children(n)
	args := make([]ast.Expr, 0, len(kids))
	for _, k := range kids {
		if k.Kind() == frontend.NodeSpreadElement {
			return nil, &NotYetLowerable{Reason: "spread element in an array literal is a later slice"}
		}
		e, err := r.lowerExpr(k)
		if err != nil {
			return nil, err
		}
		args = append(args, e)
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: index(sel("value", "NewArray"), elemType), Args: args}, nil
}

// elementAccess lowers an index expression a[i] to the array's At method. Only an
// array receiver is covered: arrayElem confirms the checker types the receiver as
// an array whose element type lowers, and the index must be a Number, the JS array
// index. An object property read spelled o["k"] and a string character read s[i]
// have different runtime meanings and hand back to their own later slices. The
// element type is carried by the receiver, so At needs no type argument here; it
// returns the element the checker already typed the whole access as.
func (r *Renderer) elementAccess(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return nil, &NotYetLowerable{Reason: "element access did not expose an object and an index"}
	}
	obj, idxNode := kids[0], kids[1]
	if _, ok := r.arrayElem(obj); !ok {
		return nil, &NotYetLowerable{Reason: "element access on a non-array receiver is a later slice"}
	}
	if !r.isNumber(idxNode) {
		return nil, &NotYetLowerable{Reason: "array element access with a non-number index is a later slice"}
	}
	recv, err := r.lowerExpr(obj)
	if err != nil {
		return nil, err
	}
	idx, err := r.lowerExpr(idxNode)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("At")}, Args: []ast.Expr{idx}}, nil
}

// objectLiteral lowers an object literal { k: v, ... } to a composite literal
// that builds a pointer to the generated struct the object's shape interns to.
// The struct name comes from the same internStruct path a variable annotated
// with this shape takes, so a literal and a binding of the same shape produce
// the same Go type and structural assignability becomes Go assignability
// (05_type_lowering section 12). Each property lowers to a keyed field, so the
// literal's declaration order need not match the struct's field order. Only the
// plain identifier-keyed forms are covered: a computed or string key belongs in
// the object side table, a spread copies another object's own fields, and a
// method or accessor is a function member, each its own later slice, so any of
// them hands back rather than emit a wrong or partial struct.
func (r *Renderer) objectLiteral(n frontend.Node) (ast.Expr, error) {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "object literal whose type is not an object shape is a later slice"}
	}
	if _, ok := r.prog.ElementType(t); ok {
		return nil, &NotYetLowerable{Reason: "object literal typed as an array is a later slice"}
	}
	// internStruct registers the struct and reports the name; a shape that does
	// not lower (an optional property, a non-identifier field) hands back here, so
	// the literal never builds a struct the type side would refuse to declare.
	name, err := r.decls.internStruct(r, t)
	if err != nil {
		return nil, err
	}
	props := r.prog.Children(n)
	elts := make([]ast.Expr, 0, len(props))
	for _, p := range props {
		if p.Kind() != frontend.NodeUnknown {
			// A method, getter, or setter member is a function property, which the
			// frontend names its own kind rather than a property assignment.
			return nil, &NotYetLowerable{Reason: "object literal with a method or accessor member is a later slice"}
		}
		kids := r.prog.Children(p)
		var keyNode, valNode frontend.Node
		switch len(kids) {
		case 1:
			// A shorthand { x } is { x: x }: the single child is both the key and the
			// value reference. A spread { ...o } is also a single-child member, but
			// its text opens with the spread token, so it routes to the handback.
			if strings.HasPrefix(strings.TrimSpace(r.prog.Text(p)), "...") {
				return nil, &NotYetLowerable{Reason: "object spread in a literal is a later slice"}
			}
			keyNode, valNode = kids[0], kids[0]
		case 2:
			keyNode, valNode = kids[0], kids[1]
		default:
			return nil, &NotYetLowerable{Reason: "object literal member with an unexpected shape is a later slice"}
		}
		if keyNode.Kind() != frontend.NodeIdentifier {
			// A computed [k] key or a string/numeric key does not become a struct
			// field; it belongs in the object side table, a later slice.
			return nil, &NotYetLowerable{Reason: "object literal with a non-identifier key is a later slice"}
		}
		field, ok := exportedField(r.prog.Text(keyNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "object literal property name is not a Go identifier"}
		}
		val, err := r.lowerExpr(valNode)
		if err != nil {
			return nil, err
		}
		elts = append(elts, &ast.KeyValueExpr{Key: ident(field), Value: val})
	}
	return &ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(name), Elts: elts}}, nil
}

// arrayMethodCall lowers a method call on an array receiver to a value.Array
// method. Only push is covered so far: it appends its arguments and returns the
// new length. The checker has already verified each argument against the element
// type, so the arguments lower directly with no per-argument kind guard the way
// the string methods need, since here the element type, not a fixed argument
// kind, is what the checker enforced. The reading, higher-order, and other
// pop is a later slice, waiting on the optional machinery for its undefined
// result. The higher-order map and filter are covered here, over a concise-body
// arrow callback that takes the element; slice, which returns a fresh array over
// a copied range; the search methods indexOf and includes, over a synthesized
// element-equality closure; and join, over a synthesized per-element ToString
// closure.
func (r *Renderer) arrayMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "push":
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		args := make([]ast.Expr, 0, len(argNodes))
		for _, a := range argNodes {
			lowered, err := r.lowerExpr(a)
			if err != nil {
				return nil, err
			}
			args = append(args, lowered)
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Push")}, Args: args}, nil
	case "map":
		return r.arrayMapFilter(recvNode, "Map", argNodes, true)
	case "filter":
		return r.arrayMapFilter(recvNode, "Filter", argNodes, false)
	case "indexOf":
		return r.arrayIndexOfIncludes(recvNode, "IndexOf", argNodes, false)
	case "includes":
		return r.arrayIndexOfIncludes(recvNode, "Includes", argNodes, true)
	case "join":
		return r.arrayJoin(recvNode, argNodes)
	case "pop":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "array pop takes no arguments"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Pop")}}, nil
	case "slice":
		if len(argNodes) > 2 {
			return nil, &NotYetLowerable{Reason: "array slice with more than two arguments is not valid"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		args := make([]ast.Expr, 0, len(argNodes))
		for _, a := range argNodes {
			if !r.isNumber(a) {
				return nil, &NotYetLowerable{Reason: "array slice with a non-number bound is a later slice"}
			}
			lowered, err := r.lowerExpr(a)
			if err != nil {
				return nil, err
			}
			args = append(args, lowered)
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Slice")}, Args: args}, nil
	default:
		return nil, &NotYetLowerable{Reason: "array method ." + method + " is a later slice"}
	}
}

// arrayMapFilter lowers a map or filter call to the matching value.Array method
// over a lowered callback. Only a single arrow-function argument is covered, the
// shape these almost always take; a callback passed as a named reference is a
// later slice, since it needs the reference resolved to a function value first.
// map carries the same-element-type restriction the value method has: a Go
// method cannot introduce a new type parameter, so a map whose callback returns
// a different type than the element hands back for the free-function slice. That
// restriction is read straight off the arrow's result type, which the checker
// has already inferred, compared against the array's element type. filter has no
// such restriction because its callback is always element to boolean.
func (r *Renderer) arrayMapFilter(recvNode frontend.Node, goMethod string, argNodes []frontend.Node, restrictToElem bool) (ast.Expr, error) {
	if len(argNodes) != 1 || argNodes[0].Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "array ." + goMethod + " with a callback that is not an inline arrow function is a later slice"}
	}
	if restrictToElem {
		elemType, ok := r.arrayElem(recvNode)
		if !ok {
			return nil, &NotYetLowerable{Reason: "array map on a receiver whose element type did not lower"}
		}
		arrow := argNodes[0]
		kids := r.prog.Children(arrow)
		bodyType, err := r.typeExpr(r.prog.TypeAt(kids[len(kids)-1]))
		if err != nil {
			return nil, err
		}
		same, err := sameGoType(elemType, bodyType)
		if err != nil {
			return nil, err
		}
		if !same {
			return nil, &NotYetLowerable{Reason: "array map that changes the element type is a later slice"}
		}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	fn, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goMethod)}, Args: []ast.Expr{fn}}, nil
}

// arrayIndexOfIncludes lowers an indexOf or includes call to the matching
// value.Array method, passing the target and a synthesized equality closure. The
// closure is what lets the value method stay type-agnostic: it cannot compare
// two values of its type parameter, so the lowerer, which knows the element
// type, builds the comparison. The equality differs by method and element type.
// For a number element, indexOf uses strict equality, which is Go ==, so a NaN
// target is never found, while includes uses SameValueZero, so its closure also
// treats two NaNs as equal. For a string element the comparison is
// value.BStr.Equal either way, since strict equality and SameValueZero agree on
// strings, and for a boolean it is Go ==. An element type outside those, whose
// equality would be reference identity or a deep compare, hands back. The
// optional fromIndex argument is a later slice, so more than one argument hands
// back.
func (r *Renderer) arrayIndexOfIncludes(recvNode frontend.Node, goMethod string, argNodes []frontend.Node, sameValueZero bool) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "array ." + goMethod + " with a fromIndex argument is a later slice"}
	}
	elemGo, ok := r.arrayElem(recvNode)
	if !ok {
		return nil, &NotYetLowerable{Reason: "array ." + goMethod + " on a receiver whose element type did not lower"}
	}
	elem, ok := r.prog.ElementType(r.prog.TypeAt(recvNode))
	if !ok {
		return nil, &NotYetLowerable{Reason: "array ." + goMethod + " could not read its element type"}
	}
	eq, err := r.equalityClosure(elem, elemGo, sameValueZero)
	if err != nil {
		return nil, err
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	target, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goMethod)}, Args: []ast.Expr{target, eq}}, nil
}

// equalityClosure builds the func(T, T) bool the array search methods take,
// spelling out the JavaScript equality for the element type. The parameters are
// named a and b and typed at the element's Go type. A number compares with ==,
// and under SameValueZero also matches two NaNs (a != a && b != b), the one place
// includes and indexOf diverge. A string compares with value.BStr.Equal, a
// boolean with ==. Any other element type hands back, since its equality is not
// one of these value comparisons.
func (r *Renderer) equalityClosure(elem frontend.Type, elemGo ast.Expr, sameValueZero bool) (ast.Expr, error) {
	var body ast.Expr
	switch {
	case elem.Flags&frontend.TypeNumber != 0:
		body = &ast.BinaryExpr{X: ident("a"), Op: token.EQL, Y: ident("b")}
		if sameValueZero {
			// a == b || a != a && b != b, so NaN matches NaN under SameValueZero.
			nanA := &ast.BinaryExpr{X: ident("a"), Op: token.NEQ, Y: ident("a")}
			nanB := &ast.BinaryExpr{X: ident("b"), Op: token.NEQ, Y: ident("b")}
			body = &ast.BinaryExpr{X: body, Op: token.LOR, Y: &ast.BinaryExpr{X: nanA, Op: token.LAND, Y: nanB}}
		}
	case elem.Flags&frontend.TypeString != 0:
		body = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident("a"), Sel: ident("Equal")}, Args: []ast.Expr{ident("b")}}
	case elem.Flags&frontend.TypeBoolean != 0:
		body = &ast.BinaryExpr{X: ident("a"), Op: token.EQL, Y: ident("b")}
	default:
		return nil, &NotYetLowerable{Reason: "array search on an element type without a value equality is a later slice"}
	}
	params := &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("a"), ident("b")}, Type: elemGo}}}
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: &ast.FieldList{List: []*ast.Field{{Type: ident("bool")}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{body}}}},
	}, nil
}

// arrayJoin lowers a join call to the value.Array Join method, passing the
// separator and a synthesized per-element ToString closure. The separator is the
// lowered string argument, or the JavaScript default comma when the call has
// none; an argument that is not a string, or more than one, hands back, since
// only the string-separator form is covered. The stringify closure is built the
// same way the search-method equality is, off the element type, because the
// value method cannot run the element-type-specific ToString on its type
// parameter.
func (r *Renderer) arrayJoin(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) > 1 {
		return nil, &NotYetLowerable{Reason: "array join with more than one argument is not valid"}
	}
	elemGo, ok := r.arrayElem(recvNode)
	if !ok {
		return nil, &NotYetLowerable{Reason: "array join on a receiver whose element type did not lower"}
	}
	elem, ok := r.prog.ElementType(r.prog.TypeAt(recvNode))
	if !ok {
		return nil, &NotYetLowerable{Reason: "array join could not read its element type"}
	}
	str, err := r.stringifyClosure(elem, elemGo)
	if err != nil {
		return nil, err
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	var sep ast.Expr
	if len(argNodes) == 1 {
		if !r.isString(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "array join with a non-string separator is a later slice"}
		}
		sep, err = r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
	} else {
		r.requireImport(valuePkg)
		sep = &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `","`}}}
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Join")}, Args: []ast.Expr{sep, str}}, nil
}

// stringifyClosure builds the func(T) value.BStr the join method takes, spelling
// out the element-type ToString. It mirrors stringify but over a synthesized
// parameter rather than a node: a number goes through value.NumberToString, a
// boolean through value.BoolToString, and a string is returned as is. Any other
// element type, whose ToString would run user code, hands back.
func (r *Renderer) stringifyClosure(elem frontend.Type, elemGo ast.Expr) (ast.Expr, error) {
	var body ast.Expr
	switch {
	case elem.Flags&frontend.TypeString != 0:
		body = ident("x")
	case elem.Flags&frontend.TypeNumber != 0:
		r.requireImport(valuePkg)
		body = &ast.CallExpr{Fun: sel("value", "NumberToString"), Args: []ast.Expr{ident("x")}}
	case elem.Flags&frontend.TypeBoolean != 0:
		r.requireImport(valuePkg)
		body = &ast.CallExpr{Fun: sel("value", "BoolToString"), Args: []ast.Expr{ident("x")}}
	default:
		return nil, &NotYetLowerable{Reason: "array join on an element type without a value ToString is a later slice"}
	}
	params := &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("x")}, Type: elemGo}}}
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "BStr")}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{body}}}},
	}, nil
}

// sameGoType reports whether two lowered type expressions print to the same Go
// source, the test map uses to keep its callback within the same-element-type
// form the value method supports. Comparing the printed form is enough: the two
// expressions are both built by typeExpr, so identical types produce identical
// syntax, and any difference in element type shows up as a difference in text.
func sameGoType(a, b ast.Expr) (bool, error) {
	as, err := printExpr(a)
	if err != nil {
		return false, err
	}
	bs, err := printExpr(b)
	if err != nil {
		return false, err
	}
	return as == bs, nil
}

// numericBinaryOp maps a TypeScript operator on number operands to its Go token.
// The arithmetic operators whose float64 semantics match JavaScript's number
// semantics are here, along with the relational and strict-equality operators,
// which compare two float64 the same way in both languages (=== on numbers is Go
// ==, !== is !=). Not here because they are not a Go binary token: %, which is
// fmod and lowers to a math.Mod call in binaryExpr. Left out on purpose: the
// bitwise operators, which coerce to int32 first, and loose == and !=, whose
// coercion has no direct Go spelling. Each is a later slice.
func numericBinaryOp(tsOp string) (token.Token, bool) {
	switch tsOp {
	case "+":
		return token.ADD, true
	case "-":
		return token.SUB, true
	case "*":
		return token.MUL, true
	case "/":
		return token.QUO, true
	case "<":
		return token.LSS, true
	case "<=":
		return token.LEQ, true
	case ">":
		return token.GTR, true
	case ">=":
		return token.GEQ, true
	case "===":
		return token.EQL, true
	case "!==":
		return token.NEQ, true
	default:
		return token.ILLEGAL, false
	}
}

// booleanBinaryOp maps a TypeScript operator on boolean operands to its Go
// token. The short-circuit && and || carry over directly: Go evaluates the left
// operand first and skips the right on the same condition JavaScript does, and
// with both operands typed boolean the result is boolean in both languages, so
// there is no truthiness gap to bridge. Strict === / !== on two booleans are Go
// == / !=. Left out on purpose: loose == and !=, whose coercion has no direct Go
// spelling, a later slice.
func booleanBinaryOp(tsOp string) (token.Token, bool) {
	switch tsOp {
	case "&&":
		return token.LAND, true
	case "||":
		return token.LOR, true
	case "===":
		return token.EQL, true
	case "!==":
		return token.NEQ, true
	default:
		return token.ILLEGAL, false
	}
}

// kindName gives a short name for a node kind, for the hand-back reason a caller
// reads when a construct is not lowered yet. It names the kinds this slice
// touches and falls back to the numeric value for the rest, which is enough to
// tell one unhandled form from another in a diagnostic.
func kindName(k frontend.NodeKind) string {
	switch k {
	case frontend.NodeReturnStatement:
		return "return statement"
	case frontend.NodeExpressionStatement:
		return "expression statement"
	case frontend.NodeVariableStatement:
		return "variable statement"
	case frontend.NodeIfStatement:
		return "if statement"
	case frontend.NodeIdentifier:
		return "identifier"
	case frontend.NodeCallExpression:
		return "call expression"
	case frontend.NodePropertyAccessExpression:
		return "property access"
	case frontend.NodeStringLiteral:
		return "string literal"
	case frontend.NodeBinaryExpression:
		return "binary expression"
	default:
		return "kind#" + strconv.Itoa(int(k))
	}
}
