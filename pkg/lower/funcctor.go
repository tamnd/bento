package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the ES5 data constructor: a plain function whose body does
// nothing but assign to this.<name>, invoked through new to build a fresh object.
// The checker gives new F() no object type of its own, an untyped constructor's
// instance is opaque, so the instance cannot ride a Go struct the way a class
// instance does; it lives as a value.Object bag from creation. F lowers to a NewF
// that builds the bag, sets each field, and returns it, and new F() calls NewF. The
// binding new F() fills is marked dynamic so a later read of it routes through the
// value model, the same bag machinery a const o = {} read takes.
//
// F must be used only through new, never called plainly and never read as a bare
// value, so replacing its func declaration with NewF drops no other reference. This
// sub-slice claims the zero-parameter body only; a body that reads a parameter, a
// this, control flow, or a return is left for a later slice and keeps the handback.

// dataCtorAssign is one this.<name> = <rhs> a data constructor body performs.
type dataCtorAssign struct {
	name string
	rhs  frontend.Node
}

// pureDataCtorBody returns the this-field assignments a function body performs when
// the body is nothing but them, the straight-line shape a data constructor lowers.
// It reports ok=false for any other statement, a return, an async or generator
// function, a non-zero parameter list, or an rhs that reads this, so only a body a
// bag builder can reproduce verbatim is claimed.
func (r *Renderer) pureDataCtorBody(fn frontend.Node) ([]dataCtorAssign, bool) {
	if fn.Kind() != frontend.NodeFunctionDeclaration {
		return nil, false
	}
	if r.isAsyncFunc(fn) || r.isGeneratorFunc(fn) {
		return nil, false
	}
	sig, ok := r.prog.SignatureAt(fn)
	if !ok || len(sig.Params) != 0 || sig.RestParam != nil {
		return nil, false
	}
	block, ok := r.funcBodyBlock(fn)
	if !ok {
		return nil, false
	}
	var out []dataCtorAssign
	for _, s := range r.prog.Children(block) {
		if s.Kind() != frontend.NodeExpressionStatement {
			return nil, false
		}
		kids := r.prog.Children(s)
		if len(kids) != 1 {
			return nil, false
		}
		asg, ok := r.thisFieldInit(kids[0])
		if !ok {
			return nil, false
		}
		out = append(out, asg)
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

// thisFieldInit matches this.<name> = <rhs> whose rhs never reads this, the one
// statement a data constructor body may hold. A compound assignment, a computed
// member, or an rhs that reads this, which the bag has no receiver for, reports
// ok=false so the whole body falls out of the data-constructor shape.
func (r *Renderer) thisFieldInit(n frontend.Node) (dataCtorAssign, bool) {
	var zero dataCtorAssign
	if n.Kind() != frontend.NodeBinaryExpression {
		return zero, false
	}
	parts := r.prog.Children(n)
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" {
		return zero, false
	}
	target := parts[0]
	if target.Kind() != frontend.NodePropertyAccessExpression {
		return zero, false
	}
	tk := r.prog.Children(target)
	if len(tk) != 2 || tk[0].Kind() != frontend.NodeThisKeyword {
		return zero, false
	}
	name := r.prog.Text(tk[1])
	if name == "" {
		return zero, false
	}
	rhs := parts[2]
	if subtreeHasKind(r.prog, rhs, frontend.NodeThisKeyword) {
		return zero, false
	}
	return dataCtorAssign{name: name, rhs: rhs}, true
}

// isDataCtor reports whether a function symbol is a data constructor used only
// through new, computing the program-wide set once on the first query so newExpr
// routing and the top-level decl emitter agree on which functions become a NewF bag
// builder rather than a plain func.
func (r *Renderer) isDataCtor(sym frontend.Symbol) bool {
	if !r.dataCtorScanned {
		r.scanDataCtors()
	}
	return r.dataCtorSyms[sym]
}

// scanDataCtors walks every source file for function declarations whose body is a
// pure data constructor, then keeps only those whose every reference is the
// constructor position of a new expression. A function read anywhere else, called
// plainly, passed as a value, or given own properties like a prototype assignment,
// is dropped: replacing its declaration with NewF would leave that other reference
// dangling. The result is a closed set the routing predicates read without walking
// again.
func (r *Renderer) scanDataCtors() {
	r.dataCtorScanned = true
	candidate := map[frontend.Symbol]bool{}
	var findCandidates func(n frontend.Node)
	findCandidates = func(n frontend.Node) {
		if n.Kind() == frontend.NodeFunctionDeclaration {
			if _, ok := r.pureDataCtorBody(n); ok {
				if sym, ok := r.prog.SymbolAt(n); ok {
					if _, named := exportedField(sym.Name); named {
						candidate[sym] = true
					}
				}
			}
		}
		for _, c := range r.prog.Children(n) {
			findCandidates(c)
		}
	}
	for _, sf := range r.prog.SourceFiles() {
		findCandidates(sf)
	}
	if len(candidate) == 0 {
		r.dataCtorSyms = candidate
		return
	}
	// A reference disqualifies a candidate unless it is the constructor of a new
	// expression or the declaration's own name. The declaration name resolves to the
	// same symbol as a use, so it is skipped by node identity against the recorded
	// declaration node.
	declName := map[frontend.Symbol]frontend.Node{}
	var recordDecl func(n frontend.Node)
	recordDecl = func(n frontend.Node) {
		if n.Kind() == frontend.NodeFunctionDeclaration {
			if sym, ok := r.prog.SymbolAt(n); ok && candidate[sym] {
				if nm, ok := r.funcDeclNameNode(n); ok {
					declName[sym] = nm
				}
			}
		}
		for _, c := range r.prog.Children(n) {
			recordDecl(c)
		}
	}
	for _, sf := range r.prog.SourceFiles() {
		recordDecl(sf)
	}
	var walk func(n, parent frontend.Node)
	walk = func(n, parent frontend.Node) {
		if n.Kind() == frontend.NodeIdentifier {
			if sym, ok := r.prog.SymbolAt(n); ok && candidate[sym] {
				decl := declName[sym]
				if decl != nil && sameNode(decl, n) {
					// the declaration name, not a use
				} else if !r.isNewConstructorPosition(n, parent) {
					candidate[sym] = false
				}
			}
		}
		for _, c := range r.prog.Children(n) {
			walk(c, n)
		}
	}
	for _, sf := range r.prog.SourceFiles() {
		walk(sf, nil)
	}
	r.dataCtorSyms = candidate
}

// sameNode reports whether two node handles name the same syntax node, comparing the
// kind and the source span, which together pin one node in one file. It lets the
// reference walk tell a data constructor's declaration name from a use of it.
func sameNode(a, b frontend.Node) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Kind() == b.Kind() && a.Pos() == b.Pos() && a.End() == b.End() &&
		a.File().Path == b.File().Path
}

// isNewConstructorPosition reports whether identifier n sits as the constructor of
// its parent new expression, the one reference a data constructor may have. A new
// expression holds the constructor first and its arguments after, so the identifier
// qualifies only when the parent is a new expression whose first child is n.
func (r *Renderer) isNewConstructorPosition(n, parent frontend.Node) bool {
	if parent == nil || parent.Kind() != frontend.NodeNewExpression {
		return false
	}
	kids := r.prog.Children(parent)
	return len(kids) > 0 && sameNode(kids[0], n)
}

// funcDeclNameNode returns the identifier a function declaration is named by, the
// node the reference walk skips so a declaration name is not mistaken for a use.
func (r *Renderer) funcDeclNameNode(fn frontend.Node) (frontend.Node, bool) {
	for _, c := range r.prog.Children(fn) {
		if c.Kind() == frontend.NodeIdentifier {
			return c, true
		}
	}
	return nil, false
}

// newExprIsDataCtor reports whether a new expression constructs a data constructor,
// the case newExpr lowers to a NewF call and the binding site marks dynamic.
func (r *Renderer) newExprIsDataCtor(n frontend.Node) bool {
	if n.Kind() != frontend.NodeNewExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(kids[0])
	return ok && r.isDataCtor(sym)
}

// dataCtorCall lowers new F() to a call of F's generated NewF bag builder. Only the
// zero-argument form is claimed, matching the zero-parameter body; a new with
// arguments hands back until the parameter-carrying slice lands.
func (r *Renderer) dataCtorCall(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) > 1 {
		return nil, &NotYetLowerable{Reason: "a data constructor invoked with arguments is a later slice"}
	}
	sym, _ := r.prog.SymbolAt(kids[0])
	name, ok := exportedField(sym.Name)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a data constructor whose name is not a Go identifier is a later slice"}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: ident("New" + name)}, nil
}

// dataCtorDecl builds the Go declaration a data constructor contributes: a NewF that
// builds a value.Object, sets each field to its boxed initializer, and returns the
// bag. The set discards its chained receiver return, mutating the bag in place, the
// same live object the binding then reads through.
func (r *Renderer) dataCtorDecl(fn frontend.Node) (ast.Decl, error) {
	sym, ok := r.prog.SymbolAt(fn)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a data constructor with no symbol is a later slice"}
	}
	name, ok := exportedField(sym.Name)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a data constructor whose name is not a Go identifier is a later slice"}
	}
	assigns, ok := r.pureDataCtorBody(fn)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a data constructor body changed shape after detection, a later slice"}
	}
	r.requireImport(valuePkg)
	thisName := r.freshTemp()
	stmts := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ident(thisName)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: sel("value", "NewObject")}},
		},
	}
	for _, a := range assigns {
		boxed, err := r.boxOperand(a.rhs)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ident(thisName), Sel: ident("Set")},
			Args: []ast.Expr{r.goStringValue(a.name), boxed},
		}})
	}
	stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{ident(thisName)}})
	return &ast.FuncDecl{
		Name: ident("New" + name),
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "Value")}}},
		},
		Body: &ast.BlockStmt{List: stmts},
	}, nil
}
