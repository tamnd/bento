package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file hoists a callable-object binding whose name an earlier statement in
// the same scope captures inside a closure. The test262 assert prelude assigns
// `assert.compareArray = function () { ... compareArray(...) ... }` before it
// declares `const compareArray = function () { ... }`, so the closure captures a
// name that, in JavaScript, is scoped to the whole module and only read when the
// closure later runs. Go has no such forward capture: a function literal can only
// close over a variable already declared above it. So the binding's pointer is
// declared once at the top of the scope, and its own site lowers to a plain
// assignment, which leaves every alias sharing the one object the way the const
// does in JavaScript.

// singleCallableBindingName returns the binding-name node of a statement that
// declares exactly one callable-object local, the shape flattenCallableBinding
// expands into a pointer and a Call assignment. Anything else reports ok=false.
func (r *Renderer) singleCallableBindingName(n frontend.Node) (frontend.Node, bool) {
	if n.Kind() != frontend.NodeVariableStatement {
		return n, false
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, n, &decls)
	if len(decls) != 1 {
		return n, false
	}
	kids := r.prog.Children(decls[0])
	if len(kids) != 2 && len(kids) != 3 {
		return n, false
	}
	nameNode := kids[0]
	if !r.isCallableObject(r.prog.TypeAt(nameNode)) {
		return n, false
	}
	return nameNode, true
}

// countIdentInClosures counts identifiers spelling name that sit inside a
// function-like descendant of the subtree, the only place a Go closure can
// capture a name declared later. A reference outside every closure is left
// uncounted: an ordinary top-level use of a not-yet-declared const is a
// JavaScript temporal-dead-zone error the source would not carry, so the hoist
// this count drives targets the closure capture alone and leaves the far more
// common already-declared callable binding on its ordinary `:=` path.
func (r *Renderer) countIdentInClosures(n frontend.Node, name string) int {
	c := 0
	for _, ch := range r.prog.Children(n) {
		if isFunctionLike(ch.Kind()) {
			c += r.countIdentAnywhere(ch, name)
		} else {
			c += r.countIdentInClosures(ch, name)
		}
	}
	return c
}

// countIdentAnywhere counts identifiers spelling name in a subtree, descending
// into every child including nested functions, since a closure nested inside the
// captured closure reads the same forward name.
func (r *Renderer) countIdentAnywhere(n frontend.Node, name string) int {
	if n.Kind() == frontend.NodeIdentifier && r.prog.Text(n) == name {
		return 1
	}
	c := 0
	for _, ch := range r.prog.Children(n) {
		c += r.countIdentAnywhere(ch, name)
	}
	return c
}

// callableFwdHoists returns the binding-name nodes of the callable-object
// declarations in a scope's top-level statement list that a statement above them
// captures in a closure. topStmts is the scope's statement list, the module body
// or a function body.
func (r *Renderer) callableFwdHoists(topStmts []frontend.Node) []frontend.Node {
	type binding struct {
		nameNode frontend.Node
		name     string
		idx      int
	}
	var bindings []binding
	for i, s := range topStmts {
		nn, ok := r.singleCallableBindingName(s)
		if !ok {
			continue
		}
		name, ok := localName(r.prog.Text(nn))
		if !ok {
			continue
		}
		bindings = append(bindings, binding{nn, name, i})
	}
	var out []frontend.Node
	for _, b := range bindings {
		for i := 0; i < b.idx; i++ {
			if r.countIdentInClosures(topStmts[i], b.name) > 0 {
				out = append(out, b.nameNode)
				break
			}
		}
	}
	return out
}

// buildFwdHoistDecls builds the `var name *Struct` declarations that go at a
// scope's top for its forward-captured callable bindings, each at the pointer
// type flattenCallableBinding assigns the binding, so the site below lowers to a
// plain assignment into a variable already in scope.
func (r *Renderer) buildFwdHoistDecls(nameNodes []frontend.Node) ([]ast.Stmt, error) {
	var out []ast.Stmt
	for _, nn := range nameNodes {
		name, ok := localName(r.prog.Text(nn))
		if !ok {
			return nil, &NotYetLowerable{Reason: "a forward-hoisted callable binding name is not a Go identifier"}
		}
		structName, err := r.decls.internStruct(r, r.prog.TypeAt(nn))
		if err != nil {
			return nil, err
		}
		out = append(out, &ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{
			&ast.ValueSpec{Names: []*ast.Ident{ident(name)}, Type: &ast.StarExpr{X: ident(structName)}},
		}}})
	}
	return out, nil
}

// enterFwdHoistScope computes the forward-captured callable bindings for a
// scope's statement list, records them so flattenCallableBinding lowers each site
// to an assignment rather than a fresh declaration, and returns the top-of-scope
// declarations to prepend along with a restore for the previous scope's set. The
// caller prepends the returned statements to the lowered body.
func (r *Renderer) enterFwdHoistScope(topStmts []frontend.Node) ([]ast.Stmt, func(), error) {
	prev := r.fwdHoisted
	restore := func() { r.fwdHoisted = prev }
	nameNodes := r.callableFwdHoists(topStmts)
	if len(nameNodes) == 0 {
		r.fwdHoisted = nil
		return nil, restore, nil
	}
	set := map[string]bool{}
	for _, nn := range nameNodes {
		if name, ok := localName(r.prog.Text(nn)); ok {
			set[name] = true
		}
	}
	r.fwdHoisted = set
	decls, err := r.buildFwdHoistDecls(nameNodes)
	if err != nil {
		restore()
		return nil, restore, err
	}
	return decls, restore, nil
}
