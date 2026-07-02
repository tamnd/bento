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
	if !r.isString(recvNode) {
		return nil, &NotYetLowerable{Reason: "method call on a non-string receiver is a later slice"}
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
// functions cannot. Left out on purpose: the transcendental functions (sin, log,
// exp), whose last-bit results are not guaranteed identical across two libm
// implementations. Each is a later slice.
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
	case "startsWith":
		return "StartsWith", []argKind{argString, argNumber}, 1, false, true
	case "endsWith":
		return "EndsWith", []argKind{argString, argNumber}, 1, false, true
	case "slice":
		return "Slice", []argKind{argNumber, argNumber}, 0, false, true
	case "substring":
		return "Substring", []argKind{argNumber, argNumber}, 0, false, true
	case "padStart":
		return "PadStart", []argKind{argNumber, argString}, 1, false, true
	case "padEnd":
		return "PadEnd", []argKind{argNumber, argString}, 1, false, true
	case "concat":
		// concat takes any number of string arguments, so it is variadic over a
		// single repeating string kind and has no upper bound.
		return "ConcatN", []argKind{argString}, 0, true, true
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
