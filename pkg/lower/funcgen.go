package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

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
	sym, ok := r.prog.SymbolAt(fn)
	if !ok {
		return Decl{}, &NotYetLowerable{Reason: "function declaration has no symbol (anonymous functions are a later slice)"}
	}
	name, ok := exportedField(sym.Name)
	if !ok {
		return Decl{}, &NotYetLowerable{Reason: "function name is not a Go identifier"}
	}

	sig, ok := r.prog.SignatureAt(fn)
	if !ok {
		return Decl{}, &NotYetLowerable{Reason: "function has no call signature"}
	}
	if len(sig.TypeParams) != 0 {
		return Decl{}, &NotYetLowerable{Reason: "generic function needs monomorphization, a later slice"}
	}
	if sig.RestParam != nil {
		return Decl{}, &NotYetLowerable{Reason: "rest parameter needs the array boxing slice"}
	}

	params, err := r.paramFields(sig)
	if err != nil {
		return Decl{}, err
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return Decl{}, err
	}

	body, err := r.blockOf(fn)
	if err != nil {
		return Decl{}, err
	}

	decl := &ast.FuncDecl{
		Name: ident(name),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}
	src, err := printDecl(decl)
	if err != nil {
		return Decl{}, err
	}
	return Decl{Name: name, Source: src}, nil
}

// paramFields lowers each parameter to a Go field with its lowered type. An
// optional parameter is T | undefined plus a presence bit, the optional tagged
// type of a later slice, so a signature with one hands back.
func (r *Renderer) paramFields(sig frontend.Signature) (*ast.FieldList, error) {
	fields := &ast.FieldList{}
	for _, p := range sig.Params {
		if p.Optional {
			return nil, &NotYetLowerable{Flags: p.Type.Flags, Reason: "optional parameter needs the optional tagged type, a later slice"}
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
	default:
		return nil, &NotYetLowerable{Reason: "statement kind " + kindName(n.Kind()) + " is a later slice"}
	}
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
		if len(kids) != 2 {
			return nil, &NotYetLowerable{Reason: "variable binding with a type annotation or no initializer is a later slice"}
		}
		name, ok := localName(r.prog.Text(kids[0]))
		if !ok {
			return nil, &NotYetLowerable{Reason: "variable name is not a Go identifier"}
		}
		typ, err := r.typeExpr(r.prog.TypeAt(kids[0]))
		if err != nil {
			return nil, err
		}
		init, err := r.lowerExpr(kids[1])
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

// lowerExprStatement lowers an expression used as a statement. The only form
// covered is an assignment to a local, which the checker exposes as a binary
// expression with an "=" operator; a call or other expression statement is a
// later slice.
func (r *Renderer) lowerExprStatement(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 || kids[0].Kind() != frontend.NodeBinaryExpression {
		return nil, &NotYetLowerable{Reason: "expression statement that is not an assignment is a later slice"}
	}
	return r.lowerAssign(kids[0])
}

// lowerAssign lowers a binary "=" expression to a Go assignment statement. It is
// shared by an assignment used as a statement and a for-loop's post clause. The
// target must be a local identifier; assigning to a property or an element is a
// later slice.
func (r *Renderer) lowerAssign(bin frontend.Node) (*ast.AssignStmt, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" {
		return nil, &NotYetLowerable{Reason: "compound or non-assignment expression is a later slice"}
	}
	if parts[0].Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "assignment to a non-identifier target is a later slice"}
	}
	name, ok := localName(r.prog.Text(parts[0]))
	if !ok {
		return nil, &NotYetLowerable{Reason: "assignment target is not a Go identifier"}
	}
	rhs, err := r.lowerExpr(parts[2])
	if err != nil {
		return nil, err
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{ident(name)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{rhs},
	}, nil
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
	if len(kids) != 4 || kids[3].Kind() != frontend.NodeBlock {
		return nil, &NotYetLowerable{Reason: "only a for with a declaration, condition, increment, and block body is lowered yet"}
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
	post, err := r.lowerAssign(kids[2])
	if err != nil {
		return nil, err
	}
	body, err := r.lowerBlock(kids[3])
	if err != nil {
		return nil, err
	}

	loop := &ast.ForStmt{Cond: cond, Post: post, Body: body}
	return &ast.BlockStmt{List: []ast.Stmt{initDecl, loop}}, nil
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
	if kids[1].Kind() != frontend.NodeBlock {
		return nil, &NotYetLowerable{Reason: "if body that is not a block is a later slice"}
	}
	body, err := r.lowerBlock(kids[1])
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

// lowerArm lowers one arm of an if: a block, or a chained if for an else-if.
func (r *Renderer) lowerArm(n frontend.Node) (ast.Stmt, error) {
	switch n.Kind() {
	case frontend.NodeBlock:
		return r.lowerBlock(n)
	case frontend.NodeIfStatement:
		return r.lowerIf(n)
	default:
		return nil, &NotYetLowerable{Reason: "if arm that is not a block or else-if is a later slice"}
	}
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

	default:
		return nil, &NotYetLowerable{Reason: "expression kind " + kindName(n.Kind()) + " is a later slice"}
	}
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
	if !r.isString(recvNode) {
		return nil, &NotYetLowerable{Reason: "method call on a non-string receiver is a later slice"}
	}
	goName, params, minArgs, ok := stringMethod(method)
	if !ok {
		return nil, &NotYetLowerable{Reason: "string method ." + method + " is a later slice"}
	}
	if len(argNodes) < minArgs || len(argNodes) > len(params) {
		return nil, &NotYetLowerable{Reason: "string method ." + method + " with this argument count is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for i, a := range argNodes {
		if !r.argHasKind(a, params[i]) {
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
// one signature covers every arity, the count selecting the defaults. A call
// still passes exactly the arguments the source wrote, so the emitted call form
// is the same whether the method is variadic or not.
func stringMethod(name string) (goName string, params []argKind, minArgs int, ok bool) {
	switch name {
	case "charCodeAt":
		return "CharCodeAt", []argKind{argNumber}, 1, true
	case "charAt":
		return "CharAt", []argKind{argNumber}, 1, true
	case "indexOf":
		return "IndexOf", []argKind{argString}, 1, true
	case "includes":
		return "Includes", []argKind{argString}, 1, true
	case "startsWith":
		return "StartsWith", []argKind{argString}, 1, true
	case "slice":
		return "Slice", []argKind{argNumber, argNumber}, 0, true
	case "substring":
		return "Substring", []argKind{argNumber, argNumber}, 0, true
	case "trim":
		return "Trim", nil, 0, true
	case "trimStart":
		return "TrimStart", nil, 0, true
	case "trimEnd":
		return "TrimEnd", nil, 0, true
	default:
		return "", nil, 0, false
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
	default:
		return nil, &NotYetLowerable{Reason: "prefix operator " + op + " is a later slice"}
	}
}

// stringLiteral lowers a string literal to a value.BStr built from its Go-string
// form. The literal's code-unit content is its source text with the quotes
// stripped, which for a literal with no backslash escapes is exactly the runtime
// string, so it is re-quoted as a Go string literal and wrapped in
// value.FromGoString. A literal that carries an escape sequence hands back: the
// escape grammars of the two languages differ in the corners (a JavaScript
// \uXXXX surrogate escape has no Go spelling), so decoding them soundly is its
// own slice rather than a guess here.
func (r *Renderer) stringLiteral(n frontend.Node) (ast.Expr, error) {
	text := r.prog.Text(n)
	if len(text) < 2 {
		return nil, &NotYetLowerable{Reason: "string literal source too short to lower"}
	}
	quote := text[0]
	if (quote != '"' && quote != '\'') || text[len(text)-1] != quote {
		return nil, &NotYetLowerable{Reason: "unusual string literal quoting is a later slice"}
	}
	inner := text[1 : len(text)-1]
	if strings.ContainsRune(inner, '\\') {
		return nil, &NotYetLowerable{Reason: "string literal with escape sequences needs the escape-decoding slice"}
	}
	r.requireImport(valuePkg)
	lit := &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(inner)}
	return &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{lit}}, nil
}

// propertyAccess lowers a member expression. The only member this slice covers
// is .length on a string, which is the code-unit count and lowers to the
// value.BStr Length method, a float64 that matches the number type the checker
// gives .length. Every other property (a field of a lowered object, a method
// call, .length on an array) is its own later slice and hands back.
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
	return nil, &NotYetLowerable{Reason: "property access ." + prop + " on this type is a later slice"}
}

// numericLiteral lowers a number literal. number is float64, so the literal is
// emitted as a Go floating constant; a plain decimal integer or fraction maps
// directly, while an exotic form (hex, binary, separators, exponent edge cases)
// hands back until the numeric-parsing slice normalizes it.
func (r *Renderer) numericLiteral(n frontend.Node) (ast.Expr, error) {
	text := r.prog.Text(n)
	if !isPlainDecimal(text) {
		return nil, &NotYetLowerable{Reason: "numeric literal " + text + " needs the number-parsing slice"}
	}
	kind := token.INT
	for i := 0; i < len(text); i++ {
		if text[i] == '.' {
			kind = token.FLOAT
		}
	}
	return &ast.BasicLit{Kind: kind, Value: text}, nil
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

	// + on two strings is concatenation of a UTF-16 string, not a Go string +,
	// which would be UTF-8, and not a Go operator at all since bstr is a struct.
	// It lowers to value.Concat, which picks the wider backing form and copies
	// once (section 5). It is handled before the operator table so the string path
	// emits a call rather than reaching the number/bool dispatch.
	if opText == "+" && r.isString(left) && r.isString(right) {
		l, err := r.lowerExpr(left)
		if err != nil {
			return nil, err
		}
		rr, err := r.lowerExpr(right)
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

// isPlainDecimal reports whether text is a plain base-ten integer or fraction
// with no sign, separators, exponent, or radix prefix, the numeric forms that
// map straight to a Go floating constant.
func isPlainDecimal(text string) bool {
	if text == "" {
		return false
	}
	dots := 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c >= '0' && c <= '9':
		case c == '.':
			dots++
		default:
			return false
		}
	}
	if dots > 1 {
		return false
	}
	return text != "."
}
