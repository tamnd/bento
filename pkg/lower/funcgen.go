package lower

import (
	"go/ast"
	"go/token"
	"strconv"

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

// blockOf finds the function's body block and lowers its statements. A function
// with no body (an overload signature or a declare) is not a lowerable unit.
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

// lowerStatement lowers one statement. The covered set is small on purpose: a
// return, with or without a value. The rest of the statement forms land in
// later slices, each handing back until then.
func (r *Renderer) lowerStatement(n frontend.Node) (ast.Stmt, error) {
	switch n.Kind() {
	case frontend.NodeReturnStatement:
		kids := r.prog.Children(n)
		if len(kids) == 0 {
			return &ast.ReturnStmt{}, nil
		}
		expr, err := r.lowerExpr(kids[0])
		if err != nil {
			return nil, err
		}
		return &ast.ReturnStmt{Results: []ast.Expr{expr}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "statement kind " + kindName(n.Kind()) + " is a later slice"}
	}
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

	case frontend.NodeBinaryExpression:
		return r.binaryExpr(n)

	default:
		return nil, &NotYetLowerable{Reason: "expression kind " + kindName(n.Kind()) + " is a later slice"}
	}
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

// binaryExpr lowers a binary expression. The operands must both be numbers for
// now: the arithmetic operators map directly on float64, while + on strings is
// concatenation of a different type and the relational and equality operators
// yield booleans, each its own later slice. The children are left, operator,
// right, the shape the frontend exposes for a binary node.
func (r *Renderer) binaryExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 3 {
		return nil, &NotYetLowerable{Reason: "binary expression did not expose left, operator, right"}
	}
	left, op, right := kids[0], kids[1], kids[2]

	if !r.isNumber(left) || !r.isNumber(right) {
		return nil, &NotYetLowerable{Reason: "binary operator on non-number operands is a later slice"}
	}
	goOp, ok := numericBinaryOp(r.prog.Text(op))
	if !ok {
		return nil, &NotYetLowerable{Reason: "binary operator " + r.prog.Text(op) + " on numbers is a later slice"}
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

// isNumber reports whether the checker types n as number, the guard that keeps
// the arithmetic path sound while string and mixed operands wait for their slice.
func (r *Renderer) isNumber(n frontend.Node) bool {
	return r.prog.TypeAt(n).Flags&frontend.TypeNumber != 0
}

// numericBinaryOp maps a TypeScript numeric operator token to its Go token. Only
// the operators whose float64 semantics match JavaScript's number semantics are
// here: %, which is fmod on JS numbers and does not apply to Go float64, and the
// bitwise operators, which coerce to int32 first, are later slices.
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
