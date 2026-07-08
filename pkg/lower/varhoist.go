package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file hoists a function-scoped `var` out of the nested block it is written
// in and up to the top of its scope. JavaScript scopes a var to the whole function
// (or the module), not the block it sits in, so `if (c) { var x = 1; } use(x);` is
// one binding written inside the block and read outside it. Emitting it as a Go
// block-local would leave x undeclared at the read and unused at the write, so the
// scope declares it once at its top and the in-block var lowers to an assignment.

// isFunctionLike reports whether a node opens its own function scope, so a hoist
// walk over one scope stops at its boundary rather than pulling a nested function's
// own vars up into the enclosing one.
func isFunctionLike(k frontend.NodeKind) bool {
	switch k {
	case frontend.NodeFunctionDeclaration, frontend.NodeFunctionExpression,
		frontend.NodeArrowFunction, frontend.NodeMethodDeclaration,
		frontend.NodeConstructor, frontend.NodeGetAccessor, frontend.NodeSetAccessor:
		return true
	}
	return false
}

// isVarStatement reports whether a variable statement is a `var` declaration, the
// only kind scoped to the whole function rather than the block. A let or const
// stays block-scoped and needs no hoist; the keyword is read from the leading token
// the same way isConstStatement reads const.
func (r *Renderer) isVarStatement(n frontend.Node) bool {
	if n.Kind() != frontend.NodeVariableStatement {
		return false
	}
	text := strings.TrimSpace(r.prog.Text(n))
	text = strings.TrimPrefix(text, "export ")
	return strings.HasPrefix(text, "var ")
}

// varNameNodes returns the binding-name nodes a single var statement declares. It
// walks the declaration list but stops at each binding's initializer, so a var whose
// value is a function expression does not pull that function's own vars in.
func (r *Renderer) varNameNodes(n frontend.Node) []frontend.Node {
	var out []frontend.Node
	var walk func(frontend.Node)
	walk = func(m frontend.Node) {
		for _, c := range r.prog.Children(m) {
			if c.Kind() == frontend.NodeVariableDeclaration {
				kids := r.prog.Children(c)
				if len(kids) > 0 {
					out = append(out, kids[0])
				}
				continue
			}
			if isFunctionLike(c.Kind()) {
				continue
			}
			walk(c)
		}
	}
	walk(n)
	return out
}

// varHoists returns the name nodes of the `var` bindings a scope must hoist to its
// top: a var declared inside a nested block whose references reach outside that
// block. A var declared directly in the top statement list is scope-level already,
// and a var used only inside its own block keeps its block-local declaration; both
// are left alone. The walk stops at a nested function, whose vars belong to its own
// scope. topStmts is the scope's statement list, the module body or a function body.
func (r *Renderer) varHoists(topStmts []frontend.Node) []frontend.Node {
	topNames := map[string]bool{}
	for _, s := range topStmts {
		if !r.isVarStatement(s) {
			continue
		}
		for _, nn := range r.varNameNodes(s) {
			if name, ok := localName(r.prog.Text(nn)); ok {
				topNames[name] = true
			}
		}
	}
	seen := map[string]bool{}
	var out []frontend.Node
	for _, s := range topStmts {
		// A var directly in the top list is already scope-level; only the nested
		// descendants of each top statement can hold a var that needs hoisting, so the
		// walk starts one level below.
		for _, c := range r.prog.Children(s) {
			r.collectVarHoists(c, nil, topStmts, topNames, seen, &out)
		}
	}
	return out
}

// collectVarHoists walks a subtree for nested var declarations that escape their
// block, tracking the nearest enclosing block so escape can be judged against it.
func (r *Renderer) collectVarHoists(n, block frontend.Node, topStmts []frontend.Node, topNames, seen map[string]bool, out *[]frontend.Node) {
	if isFunctionLike(n.Kind()) {
		return
	}
	cur := block
	if n.Kind() == frontend.NodeBlock {
		cur = n
	}
	if r.isVarStatement(n) && cur != nil {
		for _, nn := range r.varNameNodes(n) {
			name, ok := localName(r.prog.Text(nn))
			if !ok || topNames[name] || seen[name] {
				continue
			}
			if r.varEscapesBlock(topStmts, cur, name) {
				seen[name] = true
				*out = append(*out, nn)
			}
		}
	}
	for _, c := range r.prog.Children(n) {
		r.collectVarHoists(c, cur, topStmts, topNames, seen, out)
	}
}

// varEscapesBlock reports whether name is referenced anywhere in the scope outside
// the block it is declared in. It counts references in the whole scope and in the
// block: a count higher in the scope than in the block means a reference sits
// outside, so the binding must hoist. References inside a nested function are
// skipped on both sides, since a Go closure emitted for one can still read a
// block-local of the block it sits in.
func (r *Renderer) varEscapesBlock(topStmts []frontend.Node, block frontend.Node, name string) bool {
	inScope := 0
	for _, s := range topStmts {
		inScope += r.countIdentSkipFuncs(s, name)
	}
	return inScope > r.countIdentSkipFuncs(block, name)
}

// countIdentSkipFuncs counts identifiers spelling name in a subtree, not descending
// into a nested function, whose bindings are its own scope.
func (r *Renderer) countIdentSkipFuncs(n frontend.Node, name string) int {
	if isFunctionLike(n.Kind()) {
		return 0
	}
	if n.Kind() == frontend.NodeIdentifier && r.prog.Text(n) == name {
		return 1
	}
	c := 0
	for _, ch := range r.prog.Children(n) {
		c += r.countIdentSkipFuncs(ch, name)
	}
	return c
}

// buildVarHoistDecls builds the Go declarations that go at a scope's top for its
// hoisted vars, each `var name T` at the binding's declared Go type, whose zero
// value is the undefined a var reads before its first assignment for a dynamic slot
// and the type's zero otherwise. A binding the module never reads gets a trailing
// blank so the declaration does not trip Go's declared-and-not-used check while its
// assignments still run.
func (r *Renderer) buildVarHoistDecls(nameNodes []frontend.Node) ([]ast.Stmt, error) {
	var out []ast.Stmt
	for _, nn := range nameNodes {
		name, ok := localName(r.prog.Text(nn))
		if !ok {
			return nil, &NotYetLowerable{Reason: "a hoisted var name is not a Go identifier"}
		}
		typ, err := r.typeExpr(r.prog.TypeAt(nn))
		if err != nil {
			return nil, err
		}
		out = append(out, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{
			&ast.ValueSpec{Names: []*ast.Ident{ident(name)}, Type: typ},
		}}})
		if r.bindingUnused(nn) {
			out = append(out, &ast.AssignStmt{
				Lhs: []ast.Expr{ident("_")},
				Tok: token.ASSIGN,
				Rhs: []ast.Expr{ident(name)},
			})
		}
	}
	return out, nil
}

// enterVarHoistScope computes the var hoists for a scope's statement list, records
// them so an in-block `var` lowers to an assignment, and returns the top-of-scope
// declarations to prepend along with a restore for the previous scope's set. The
// caller prepends the returned statements to the lowered body.
func (r *Renderer) enterVarHoistScope(topStmts []frontend.Node) ([]ast.Stmt, func(), error) {
	prev := r.hoistedVars
	restore := func() { r.hoistedVars = prev }
	nameNodes := r.varHoists(topStmts)
	if len(nameNodes) == 0 {
		r.hoistedVars = nil
		return nil, restore, nil
	}
	set := map[string]bool{}
	for _, nn := range nameNodes {
		if name, ok := localName(r.prog.Text(nn)); ok {
			set[name] = true
		}
	}
	r.hoistedVars = set
	decls, err := r.buildVarHoistDecls(nameNodes)
	if err != nil {
		restore()
		return nil, restore, err
	}
	return decls, restore, nil
}
