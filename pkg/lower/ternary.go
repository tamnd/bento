package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file flattens a ternary that stands in statement position into the if a
// person writes, in place of the immediately-invoked function conditionalExpr
// emits when a ternary is embedded in a larger expression. A ternary whose value
// is returned becomes an if that returns each branch; a ternary bound to a new
// local or assigned to an existing one becomes an if that assigns the branch to
// that slot, the shape doc 05 asks for ("lower ternaries to an if that assigns a
// temporary"). The expression-position IIFE stays for a ternary nested inside a
// call, an operator, or another expression, where Go still wants a value, so this
// only claims the three positions where the ternary is the whole statement.
//
// Flattening keeps the meaning exact: only the taken branch's expression lowers
// into its arm, so a side effect in the untaken branch stays unevaluated the way
// the ternary and the IIFE both leave it, and the condition lowers once. A chained
// a ? x : b ? y : z flattens by recursion, into a straight run of ifs in return
// position and an else-if ladder in assignment position, the readable spelling of
// the same decision tree. A branch or a condition the subset cannot lower hands the
// unit back with its reason rather than fall back to the IIFE, since the flattened
// form coerces each branch to the target itself and so lowers a superset of what the
// IIFE would.

// leafLower lowers a non-conditional branch of a flattened ternary to the Go
// expression that lands in its statement, coercing it to what the position expects:
// a binding coerces to the binding's type, an assignment to the target's type. The
// return position does its own coercion, so it does not go through a leafLower.
type leafLower func(node frontend.Node) (ast.Expr, error)

// flattenConditionalStatement lowers a statement whose value is a top-level ternary
// to the if form, reporting ok=false for every other statement so the caller keeps
// the ordinary single-statement path. It claims three positions: return c ? a : b, a
// single binding const x = c ? a : b, and a plain assignment x = c ? a : b. A branch
// or the condition that does not lower returns the error, handing the unit back.
func (r *Renderer) flattenConditionalStatement(n frontend.Node) ([]ast.Stmt, bool, error) {
	switch n.Kind() {
	case frontend.NodeReturnStatement:
		return r.flattenConditionalReturn(n)
	case frontend.NodeVariableStatement:
		return r.flattenConditionalDecl(n)
	case frontend.NodeExpressionStatement:
		return r.flattenConditionalAssign(n)
	default:
		return nil, false, nil
	}
}

// flattenConditionalReturn lowers return c ? a : b to an if that returns one branch
// or the other. It engages only when the returned value, seen through any
// parentheses, is a ternary; a plain return keeps the ordinary lowering.
func (r *Renderer) flattenConditionalReturn(n frontend.Node) ([]ast.Stmt, bool, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 || r.unwrapParens(kids[0]).Kind() != frontend.NodeConditionalExpression {
		return nil, false, nil
	}
	stmts, err := r.conditionalReturnStmts(kids[0])
	if err != nil {
		return nil, false, err
	}
	return stmts, true, nil
}

// conditionalReturnStmts lowers a returned expression to the statements that return
// its value, flattening a ternary into an if and recursing so a chained ternary
// becomes a run of ifs: `if c { return a }; if d { return b }; return e`. A branch
// that is not a ternary is the base case, a single return coerced to the function's
// return type the same way lowerReturn coerces a plain one.
func (r *Renderer) conditionalReturnStmts(node frontend.Node) ([]ast.Stmt, error) {
	node = r.unwrapParens(node)
	cond, whenTrue, whenFalse, ok := r.conditionalParts(node)
	if !ok {
		expr, err := r.lowerExpr(node)
		if err != nil {
			return nil, err
		}
		expr, err = r.coerceReturn(expr, node)
		if err != nil {
			return nil, err
		}
		return []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{expr}}}, nil
	}
	// A condition the checker proved always truthy or always falsy collapses to the
	// branch that runs, the same drop conditionalExpr takes for an embedded ternary:
	// return o ? a : b over a non-null object flattens to just return a, with no test
	// emitted, since only the taken branch ever runs. Taken only for a repeatable
	// condition, so dropping the test loses no side effect.
	if val, known := r.staticTruthy(cond); known && r.repeatableOperand(cond) {
		if val {
			return r.conditionalReturnStmts(whenTrue)
		}
		return r.conditionalReturnStmts(whenFalse)
	}
	condExpr, err := r.lowerCondition(cond)
	if err != nil {
		return nil, err
	}
	thenStmts, err := r.conditionalReturnStmts(whenTrue)
	if err != nil {
		return nil, err
	}
	elseStmts, err := r.conditionalReturnStmts(whenFalse)
	if err != nil {
		return nil, err
	}
	ifStmt := &ast.IfStmt{Cond: condExpr, Body: &ast.BlockStmt{List: thenStmts}}
	return append([]ast.Stmt{ifStmt}, elseStmts...), nil
}

// flattenConditionalDecl lowers a single binding const x = c ? a : b to a bare var
// declaration followed by an if that assigns the taken branch to it, the temporary
// doc 05 names. It engages only for a lone binding whose initializer, through any
// parentheses, is a ternary; a multi-binding statement, an int32-specialized
// counter (whose initializer lowers in the int32 domain), or a non-ternary
// initializer keeps the ordinary lowering.
func (r *Renderer) flattenConditionalDecl(n frontend.Node) ([]ast.Stmt, bool, error) {
	var decls []frontend.Node
	collectVarDecls(r.prog, n, &decls)
	if len(decls) != 1 {
		return nil, false, nil
	}
	kids := r.prog.Children(decls[0])
	if len(kids) != 2 && len(kids) != 3 {
		return nil, false, nil
	}
	initNode := kids[len(kids)-1]
	if r.unwrapParens(initNode).Kind() != frontend.NodeConditionalExpression {
		return nil, false, nil
	}
	nameNode := kids[0]
	name, ok := localName(r.prog.Text(nameNode))
	if !ok || r.int32Locals[name] {
		return nil, false, nil
	}
	// The temporary takes the branches' widened primitive type, not the checker's
	// type for the binding: an un-annotated const s = c ? "pos" : "neg" is inferred
	// as the literal union "pos" | "neg", but the branches lower to value.BStr, so
	// the slot the assignments write is value.BStr the same way conditionalExpr's
	// IIFE widens the literal union to the primitive it returns. A ternary whose
	// branches are not one uniform primitive has no such widened slot and keeps the
	// ordinary path, which routes it through the IIFE or hands it back.
	typ, _, ok := r.condResultType(initNode)
	if !ok {
		return nil, false, nil
	}
	leaf := func(branch frontend.Node) (ast.Expr, error) {
		return r.lowerExpr(branch)
	}
	assign, err := r.conditionalAssignStmt(name, initNode, leaf)
	if err != nil {
		return nil, false, err
	}
	decl := &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{
		&ast.ValueSpec{Names: []*ast.Ident{ident(name)}, Type: typ},
	}}}
	return []ast.Stmt{decl, assign}, true, nil
}

// flattenConditionalAssign lowers a plain assignment x = c ? a : b to an if that
// assigns the taken branch to x. It engages only when the target is a local
// identifier the flatten does not specialize and the right-hand side, through any
// parentheses, is a ternary; a compound assignment, a property or element target, or
// an int32-specialized local keeps the ordinary lowering.
func (r *Renderer) flattenConditionalAssign(n frontend.Node) ([]ast.Stmt, bool, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 || kids[0].Kind() != frontend.NodeBinaryExpression {
		return nil, false, nil
	}
	parts := r.prog.Children(kids[0])
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" || parts[0].Kind() != frontend.NodeIdentifier {
		return nil, false, nil
	}
	if r.unwrapParens(parts[2]).Kind() != frontend.NodeConditionalExpression {
		return nil, false, nil
	}
	name, ok := localName(r.prog.Text(parts[0]))
	if !ok || r.int32Locals[name] {
		return nil, false, nil
	}
	targetNode := parts[0]
	leaf := func(branch frontend.Node) (ast.Expr, error) {
		val, err := r.lowerExpr(branch)
		if err != nil {
			return nil, err
		}
		return r.coerceToTarget(val, branch, targetNode)
	}
	assign, err := r.conditionalAssignStmt(name, parts[2], leaf)
	if err != nil {
		return nil, false, err
	}
	return []ast.Stmt{assign}, true, nil
}

// conditionalAssignStmt lowers a ternary bound to targetName into an if that assigns
// the taken branch, recursing so a chained ternary becomes an else-if ladder. A
// branch that is not a ternary is the base case, a single assignment whose value the
// caller's leaf coerces to the target. The false branch drives the else: a nested if
// becomes an else-if, a plain assignment an else block.
func (r *Renderer) conditionalAssignStmt(targetName string, node frontend.Node, leaf leafLower) (ast.Stmt, error) {
	node = r.unwrapParens(node)
	cond, whenTrue, whenFalse, ok := r.conditionalParts(node)
	if !ok {
		val, err := leaf(node)
		if err != nil {
			return nil, err
		}
		return &ast.AssignStmt{Lhs: []ast.Expr{ident(targetName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{val}}, nil
	}
	// The same always-truthy collapse the return path takes: a statically known,
	// repeatable condition assigns only the taken branch, with no if emitted.
	if val, known := r.staticTruthy(cond); known && r.repeatableOperand(cond) {
		if val {
			return r.conditionalAssignStmt(targetName, whenTrue, leaf)
		}
		return r.conditionalAssignStmt(targetName, whenFalse, leaf)
	}
	condExpr, err := r.lowerCondition(cond)
	if err != nil {
		return nil, err
	}
	thenStmt, err := r.conditionalAssignStmt(targetName, whenTrue, leaf)
	if err != nil {
		return nil, err
	}
	elseStmt, err := r.conditionalAssignStmt(targetName, whenFalse, leaf)
	if err != nil {
		return nil, err
	}
	ifStmt := &ast.IfStmt{Cond: condExpr, Body: &ast.BlockStmt{List: []ast.Stmt{thenStmt}}}
	if _, chain := elseStmt.(*ast.IfStmt); chain {
		ifStmt.Else = elseStmt
	} else {
		ifStmt.Else = &ast.BlockStmt{List: []ast.Stmt{elseStmt}}
	}
	return ifStmt, nil
}

// condResultType reports the Go type a ternary in statement position writes to its
// slot: the widened primitive its branches share, value.BStr for two string
// literals rather than the checker's "pos" | "neg" literal union. It recurses both
// branches so a chained ternary types only when every leaf is the same primitive,
// and reports ok=false when a leaf is not a number, string, or boolean or the
// leaves disagree, so a binding the flatten cannot type keeps the ordinary path.
func (r *Renderer) condResultType(node frontend.Node) (ast.Expr, string, bool) {
	node = r.unwrapParens(node)
	_, whenTrue, whenFalse, ok := r.conditionalParts(node)
	if !ok {
		return r.condBranchType(node)
	}
	trueType, trueKind, trueOK := r.condResultType(whenTrue)
	_, falseKind, falseOK := r.condResultType(whenFalse)
	if !trueOK || !falseOK || trueKind != falseKind {
		return nil, "", false
	}
	return trueType, trueKind, true
}

// conditionalParts splits a ternary into its condition and two branches, checking
// the node has the cond ? whenTrue : whenFalse shape. It reports ok=false for any
// other node so a caller treats a non-ternary as the base of the recursion.
func (r *Renderer) conditionalParts(n frontend.Node) (cond, whenTrue, whenFalse frontend.Node, ok bool) {
	if n.Kind() != frontend.NodeConditionalExpression {
		return nil, nil, nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) != 5 || r.prog.Text(kids[1]) != "?" || r.prog.Text(kids[3]) != ":" {
		return nil, nil, nil, false
	}
	return kids[0], kids[2], kids[4], true
}

// unwrapParens returns the expression a node holds through any layer of
// parentheses, so a parenthesized ternary or a parenthesized branch is seen as the
// ternary it wraps.
func (r *Renderer) unwrapParens(n frontend.Node) frontend.Node {
	for n.Kind() == frontend.NodeParenthesizedExpression {
		kids := r.prog.Children(n)
		if len(kids) != 1 {
			return n
		}
		n = kids[0]
	}
	return n
}
