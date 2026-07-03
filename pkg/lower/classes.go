package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers classes (05_type_lowering sections on classes and method
// sets). A class whose methods are only ever called on statically typed
// instances needs no dynamic dispatch, so the core slice is the direct mapping a
// Go programmer writes by hand: the class becomes a struct, the constructor
// becomes a NewX function returning *X, each method becomes a pointer-receiver
// method, and this becomes the receiver. Everything that needs more machinery
// (inheritance and super, statics, accessors, a method used as a value, virtual
// dispatch through a base-typed variable) hands back to the engine, the section
// 30 boundary every other lowering keeps.
//
// The registry of top-level classes is collected in a pre-pass before any body
// lowers, so a function body can construct an instance of a class declared
// below it, the same hoisting the checker applies. An instance is recognized at
// a use site through the checker, not by shape: the instance type's declaring
// symbol (frontend.TypeSymbol) is walked back to the registered declaration, so
// a structural twin of a class shape is never mistaken for the class and a
// class whose fields happen to spell a Map or Uint8Array fingerprint is never
// hijacked by those paths.

// classInfo is one registered top-level class.
type classInfo struct {
	name   string // source class name, the registry key
	goName string // Go type name, exportedField of name
	recv   string // receiver identifier shared by the constructor and every method
	decl   frontend.Node
	fields []classField
	ctor   frontend.Node // nil when the class declares no constructor
	// ctorParams are the constructor's parameter nodes, kept so a new expression
	// coerces each argument against the declared parameter, and empty for the
	// default constructor.
	ctorParams []frontend.Node
	methods    []classMethod
}

// classField is one instance field, in declaration order.
type classField struct {
	prop   string        // source property name
	goName string        // exported Go field name
	ident  frontend.Node // the name node, whose checker type is the declared field type
	init   frontend.Node // the initializer expression, nil when none
}

// classMethod is one instance method.
type classMethod struct {
	prop   string
	goName string
	node   frontend.Node
}

func (c *classInfo) fieldByName(prop string) (classField, bool) {
	for _, f := range c.fields {
		if f.prop == prop {
			return f, true
		}
	}
	return classField{}, false
}

func (c *classInfo) methodByName(prop string) (classMethod, bool) {
	for _, m := range c.methods {
		if m.prop == prop {
			return m, true
		}
	}
	return classMethod{}, false
}

// collectClasses registers every top-level class of the entry module before any
// body lowers, the same pre-pass collectNodeImports runs for imports, so a use
// anywhere in the module resolves against the full registry. A class the slice
// does not cover hands the whole unit back here, before any Go is emitted.
func (r *Renderer) collectClasses(entry frontend.Node) error {
	for _, stmt := range r.prog.Children(entry) {
		if stmt.Kind() != frontend.NodeClassDeclaration {
			continue
		}
		if err := r.registerClass(stmt); err != nil {
			return err
		}
	}
	return nil
}

// registerClass validates one class declaration against the covered subset and
// records it. The checks are spelling-level where the frontend does not expose
// a construct as its own node kind (a static or async modifier rides on the
// member's source text), which errs toward handing back: a modifier bento does
// not model routes the unit to the engine rather than lowering a member with
// the wrong semantics.
func (r *Renderer) registerClass(decl frontend.Node) error {
	kids := r.prog.Children(decl)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return &NotYetLowerable{Reason: "a class without a name is a later slice"}
	}
	name := r.prog.Text(kids[0])
	goName, ok := exportedField(name)
	if !ok {
		return &NotYetLowerable{Reason: "class name is not a Go identifier"}
	}
	if _, dup := r.classes[name]; dup {
		return &NotYetLowerable{Reason: "two classes named " + name + " in one module is a later slice"}
	}
	head, _, _ := strings.Cut(r.prog.Text(decl), "{")
	if strings.Contains(head, "abstract") {
		return &NotYetLowerable{Reason: "an abstract class is a later slice"}
	}

	info := &classInfo{name: name, goName: goName, decl: decl}
	for _, m := range kids[1:] {
		switch m.Kind() {
		case frontend.NodeIdentifier:
			// The name, already read.
		case frontend.NodePropertyDeclaration:
			f, err := r.classFieldOf(m)
			if err != nil {
				return err
			}
			info.fields = append(info.fields, f)
		case frontend.NodeConstructor:
			if info.ctor != nil {
				return &NotYetLowerable{Reason: "constructor overloads are a later slice"}
			}
			params, err := r.ctorParamsOf(m)
			if err != nil {
				return err
			}
			info.ctor = m
			info.ctorParams = params
		case frontend.NodeMethodDeclaration:
			meth, err := r.classMethodOf(m)
			if err != nil {
				return err
			}
			if _, clash := info.fieldByName(meth.prop); clash {
				return &NotYetLowerable{Reason: "a method named like a field is a later slice"}
			}
			info.methods = append(info.methods, meth)
		case frontend.NodeGetAccessor, frontend.NodeSetAccessor:
			return &NotYetLowerable{Reason: "class accessors are a later slice"}
		case frontend.NodeUnknown:
			// A heritage clause (extends, implements) surfaces as an unnamed node;
			// inheritance is a later slice. An empty leftover token is skipped.
			text := strings.TrimSpace(r.prog.Text(m))
			if text != "" {
				return &NotYetLowerable{Reason: "class heritage (" + firstWord(text) + ") is a later slice"}
			}
		default:
			return &NotYetLowerable{Reason: "this class member kind is a later slice"}
		}
	}

	info.recv = r.receiverName(info)
	r.classes[name] = info
	r.classOrder = append(r.classOrder, name)
	return nil
}

// classFieldOf reads one property declaration into a classField. The member's
// children are the name, an optional type annotation (an unnamed node), and an
// optional initializer expression. An initializer bento's node vocabulary does
// not name would be indistinguishable from an annotation, so a member whose
// source carries an = that did not surface as an initializer hands back rather
// than silently dropping the value; a definite-assignment assertion (x!: T)
// waives the checker's initialization proof this lowering relies on, so it
// hands back too.
func (r *Renderer) classFieldOf(m frontend.Node) (classField, error) {
	text := strings.TrimSpace(r.prog.Text(m))
	if w := firstWord(text); w == "static" || w == "declare" || w == "accessor" {
		return classField{}, &NotYetLowerable{Reason: "a " + w + " class field is a later slice"}
	}
	kids := r.prog.Children(m)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return classField{}, &NotYetLowerable{Reason: "a class field without a plain identifier name is a later slice"}
	}
	prop := r.prog.Text(kids[0])
	goName, ok := exportedField(prop)
	if !ok {
		return classField{}, &NotYetLowerable{Reason: "class field name is not a Go identifier"}
	}
	var init frontend.Node
	if last := kids[len(kids)-1]; len(kids) > 1 && last.Kind() != frontend.NodeUnknown {
		init = last
	}
	if init == nil && strings.Contains(text, "=") {
		return classField{}, &NotYetLowerable{Reason: "class field initializer bento could not read is a later slice"}
	}
	if strings.Contains(strings.SplitN(text, "=", 2)[0], "!") {
		return classField{}, &NotYetLowerable{Reason: "a definite-assignment field (x!: T) is a later slice"}
	}
	return classField{prop: prop, goName: goName, ident: kids[0], init: init}, nil
}

// ctorParamsOf validates the constructor's shape and returns its parameter
// nodes. Only plain typed parameters are covered: a default value needs
// call-site defaulting, and a parameter property (constructor(private x: ...))
// declares a field as a side effect, both later slices.
func (r *Renderer) ctorParamsOf(ctor frontend.Node) ([]frontend.Node, error) {
	var params []frontend.Node
	hasBody := false
	for _, k := range r.prog.Children(ctor) {
		switch k.Kind() {
		case frontend.NodeParameter:
			if err := r.plainParam(k); err != nil {
				return nil, err
			}
			params = append(params, k)
		case frontend.NodeBlock:
			hasBody = true
		}
	}
	if !hasBody {
		return nil, &NotYetLowerable{Reason: "a constructor overload signature is a later slice"}
	}
	return params, nil
}

// plainParam checks one parameter node is a bare typed identifier: the name,
// then at most an unnamed type annotation. A modifier (a parameter property), a
// default value, a rest element, or a binding pattern all hand back.
func (r *Renderer) plainParam(p frontend.Node) error {
	if w := firstWord(strings.TrimSpace(r.prog.Text(p))); w == "public" || w == "private" || w == "protected" || w == "readonly" {
		return &NotYetLowerable{Reason: "a parameter property (constructor(" + w + " x)) is a later slice"}
	}
	kids := r.prog.Children(p)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return &NotYetLowerable{Reason: "a parameter that is not a plain identifier is a later slice"}
	}
	for _, extra := range kids[1:] {
		if extra.Kind() != frontend.NodeUnknown {
			return &NotYetLowerable{Reason: "a parameter with a default value is a later slice"}
		}
	}
	return nil
}

// classMethodOf reads one method declaration into a classMethod.
func (r *Renderer) classMethodOf(m frontend.Node) (classMethod, error) {
	if w := firstWord(strings.TrimSpace(r.prog.Text(m))); w == "static" || w == "async" {
		return classMethod{}, &NotYetLowerable{Reason: "a " + w + " method is a later slice"}
	}
	kids := r.prog.Children(m)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return classMethod{}, &NotYetLowerable{Reason: "a method without a plain identifier name is a later slice"}
	}
	prop := r.prog.Text(kids[0])
	goName, ok := exportedField(prop)
	if !ok {
		return classMethod{}, &NotYetLowerable{Reason: "method name is not a Go identifier"}
	}
	return classMethod{prop: prop, goName: goName, node: m}, nil
}

// receiverName picks the receiver identifier the constructor and every method
// share, the way a Go programmer names one: the first letter of the type,
// falling back to the lowercased type name and then to self when the short name
// is already spoken for by any identifier inside the class body (a parameter, a
// local, a global the body references), so the receiver never shadows a name a
// body reads.
func (r *Renderer) receiverName(info *classInfo) string {
	taken := map[string]bool{}
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		if n.Kind() == frontend.NodeIdentifier {
			taken[r.prog.Text(n)] = true
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	walk(info.decl)
	for _, cand := range []string{strings.ToLower(info.goName[:1]), strings.ToLower(info.goName), "self"} {
		if !taken[cand] && !goKeywords[cand] {
			return cand
		}
	}
	base := strings.ToLower(info.goName[:1])
	for n := 2; ; n++ {
		cand := base + itoa(n)
		if !taken[cand] {
			return cand
		}
	}
}

// classOfType resolves a type to the registered class it instantiates, by
// walking from the type to its declaring symbol and from the symbol to the
// declaration the registry recorded. The declaration must match, not just the
// name, so a class declared in a nested scope that shadows a top-level name is
// never confused with the registered one.
func (r *Renderer) classOfType(t frontend.Type) (*classInfo, bool) {
	if len(r.classes) == 0 || t.Flags&frontend.TypeObject == 0 {
		return nil, false
	}
	sym, ok := r.prog.TypeSymbol(t)
	if !ok || sym.Flags&frontend.SymbolClass == 0 {
		return nil, false
	}
	info, ok := r.classes[sym.Name]
	if !ok {
		return nil, false
	}
	for _, d := range r.prog.Declarations(sym) {
		if d == info.decl {
			return info, true
		}
	}
	return nil, false
}

// classOfNode resolves an expression node to the registered class its type
// instantiates.
func (r *Renderer) classOfNode(n frontend.Node) (*classInfo, bool) {
	return r.classOfType(r.prog.TypeAt(n))
}

// classReceiver resolves the receiver of a member access to a registered
// class: this inside a lowered class body resolves to that class, and any other
// expression resolves through its checker type.
func (r *Renderer) classReceiver(obj frontend.Node) (*classInfo, bool) {
	if obj.Kind() == frontend.NodeThisKeyword {
		return r.curClass, r.curClass != nil
	}
	return r.classOfNode(obj)
}

// classCtorRef resolves the constructor identifier of a new expression to the
// registered class it names. The identifier's symbol must declare the exact
// registered declaration, so a nested class that shadows a top-level name, or a
// class spelled like a built-in, still resolves to what the name means at the
// use site.
func (r *Renderer) classCtorRef(nameNode frontend.Node) (*classInfo, bool) {
	info, ok := r.classes[r.prog.Text(nameNode)]
	if !ok {
		return nil, false
	}
	sym, ok := r.prog.SymbolAt(nameNode)
	if !ok || sym.Flags&frontend.SymbolClass == 0 {
		return nil, false
	}
	for _, d := range r.prog.Declarations(sym) {
		if d == info.decl {
			return info, true
		}
	}
	return nil, false
}

// renderClasses emits the declarations of every registered class in source
// order: the struct, the constructor, then the methods, the order a hand-written
// Go file keeps a type and its method set in.
func (r *Renderer) renderClasses() ([]ast.Decl, error) {
	var out []ast.Decl
	for _, name := range r.classOrder {
		info := r.classes[name]
		decls, err := r.renderClass(info)
		if err != nil {
			return nil, err
		}
		out = append(out, decls...)
	}
	return out, nil
}

func (r *Renderer) renderClass(info *classInfo) ([]ast.Decl, error) {
	structDecl, err := r.classStruct(info)
	if err != nil {
		return nil, err
	}
	ctorDecl, err := r.classCtor(info)
	if err != nil {
		return nil, err
	}
	out := []ast.Decl{structDecl, ctorDecl}
	for _, m := range info.methods {
		md, err := r.classMethodDecl(info, m)
		if err != nil {
			return nil, err
		}
		out = append(out, md)
	}
	return out, nil
}

// classStruct emits the struct: one field per instance field, in declaration
// order, each carrying the source property name in a json tag the same way an
// object shape's struct does, so a reflection walk recovers the exact key.
func (r *Renderer) classStruct(info *classInfo) (ast.Decl, error) {
	fields := &ast.FieldList{}
	for _, f := range info.fields {
		goType, err := r.typeExpr(r.prog.TypeAt(f.ident))
		if err != nil {
			return nil, err
		}
		fields.List = append(fields.List, &ast.Field{
			Names: []*ast.Ident{ident(f.goName)},
			Type:  goType,
			Tag:   &ast.BasicLit{Kind: token.STRING, Value: "`json:\"" + f.prop + "\"`"},
		})
	}
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{&ast.TypeSpec{
			Name: ident(info.goName),
			Type: &ast.StructType{Fields: fields},
		}},
	}, nil
}

// classCtor emits the NewX constructor. JavaScript runs the field initializers
// in declaration order and then the constructor body, so the general form is an
// allocation, the initializer assignments, the lowered body with this bound to
// the receiver, and a return. When every initializer and every body statement
// is a pure this.field = value store, the whole sequence folds to the composite
// literal a Go programmer writes: return &Point{X: x, Y: y}. An unassigned
// non-optional field would read as undefined in JavaScript but as the Go zero
// value here; the strict checker the frontend always runs proves every field
// definitely assigned before a program reaches lowering, so the two never
// diverge on a checked program.
func (r *Renderer) classCtor(info *classInfo) (ast.Decl, error) {
	params := &ast.FieldList{}
	for _, p := range info.ctorParams {
		nameNode := r.prog.Children(p)[0]
		pname, ok := localName(r.prog.Text(nameNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "constructor parameter name is not a Go identifier"}
		}
		pt, err := r.typeExpr(r.prog.TypeAt(nameNode))
		if err != nil {
			return nil, err
		}
		params.List = append(params.List, &ast.Field{Names: []*ast.Ident{ident(pname)}, Type: pt})
	}

	prevClass, prevThis := r.curClass, r.thisName
	r.curClass, r.thisName = info, info.recv
	defer func() { r.curClass, r.thisName = prevClass, prevThis }()

	body, err := r.ctorBody(info)
	if err != nil {
		return nil, err
	}
	return &ast.FuncDecl{
		Name: ident("New" + info.goName),
		Type: &ast.FuncType{
			Params:  params,
			Results: &ast.FieldList{List: []*ast.Field{{Type: star(ident(info.goName))}}},
		},
		Body: body,
	}, nil
}

// ctorBody builds the constructor's statements, folding to a composite literal
// when every store is pure. A return statement inside a constructor body needs
// its own lowering (a bare return must still yield the receiver), so it hands
// back for now.
func (r *Renderer) ctorBody(info *classInfo) (*ast.BlockStmt, error) {
	if info.ctor != nil && subtreeHasKind(r.prog, info.ctor, frontend.NodeReturnStatement) {
		return nil, &NotYetLowerable{Reason: "a return inside a constructor is a later slice"}
	}
	for _, f := range info.fields {
		if f.init != nil && subtreeHasKind(r.prog, f.init, frontend.NodeThisKeyword) {
			return nil, &NotYetLowerable{Reason: "a field initializer that reads this is a later slice"}
		}
	}

	if lit, ok, err := r.ctorCompositeFold(info); err != nil {
		return nil, err
	} else if ok {
		return &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{lit}}}}, nil
	}

	// The general form: allocate, run the field initializers in order, run the
	// body with this bound to the receiver, return the receiver.
	stmts := []ast.Stmt{&ast.AssignStmt{
		Lhs: []ast.Expr{ident(info.recv)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(info.goName)}}},
	}}
	for _, f := range info.fields {
		if f.init == nil {
			continue
		}
		rhs, err := r.lowerExpr(f.init)
		if err != nil {
			return nil, err
		}
		rhs, err = r.coerceToTarget(rhs, f.init, f.ident)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{&ast.SelectorExpr{X: ident(info.recv), Sel: ident(f.goName)}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{rhs},
		})
	}
	if info.ctor != nil {
		block, err := r.blockOf(info.ctor)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, block.List...)
	}
	stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{ident(info.recv)}})
	return &ast.BlockStmt{List: stmts}, nil
}

// ctorCompositeFold recognizes the constructor whose whole effect is storing
// pure values into fields, the overwhelmingly common shape, and folds it to the
// one-line composite literal a person writes. Every field initializer and every
// body statement must be a this.field = expr store of a pure, this-free
// expression; the fold keeps the last store per field, which is sound exactly
// because a pure expression's evaluation cannot be observed out of order or
// skipped. Anything else reports ok=false and the general form runs.
func (r *Renderer) ctorCompositeFold(info *classInfo) (ast.Expr, bool, error) {
	values := map[string]frontend.Node{}
	for _, f := range info.fields {
		if f.init == nil {
			continue
		}
		if !r.pureCtorValue(f.init) {
			return nil, false, nil
		}
		values[f.prop] = f.init
	}
	if info.ctor != nil {
		var block frontend.Node
		for _, k := range r.prog.Children(info.ctor) {
			if k.Kind() == frontend.NodeBlock {
				block = k
			}
		}
		for _, stmt := range r.prog.Children(block) {
			prop, rhs, ok := r.thisFieldStore(stmt)
			if !ok || !r.pureCtorValue(rhs) {
				return nil, false, nil
			}
			if _, isField := info.fieldByName(prop); !isField {
				return nil, false, nil
			}
			values[prop] = rhs
		}
	}

	lit := &ast.CompositeLit{Type: ident(info.goName)}
	for _, f := range info.fields {
		v, ok := values[f.prop]
		if !ok {
			continue
		}
		rhs, err := r.lowerExpr(v)
		if err != nil {
			return nil, false, err
		}
		rhs, err = r.coerceToTarget(rhs, v, f.ident)
		if err != nil {
			return nil, false, err
		}
		lit.Elts = append(lit.Elts, &ast.KeyValueExpr{Key: ident(f.goName), Value: rhs})
	}
	return &ast.UnaryExpr{Op: token.AND, X: lit}, true, nil
}

// thisFieldStore recognizes a statement of the exact shape this.f = expr and
// returns the field name and the value expression.
func (r *Renderer) thisFieldStore(stmt frontend.Node) (string, frontend.Node, bool) {
	if stmt.Kind() != frontend.NodeExpressionStatement {
		return "", nil, false
	}
	kids := r.prog.Children(stmt)
	if len(kids) != 1 || kids[0].Kind() != frontend.NodeBinaryExpression {
		return "", nil, false
	}
	parts := r.prog.Children(kids[0])
	if len(parts) != 3 || r.prog.Text(parts[1]) != "=" {
		return "", nil, false
	}
	if parts[0].Kind() != frontend.NodePropertyAccessExpression {
		return "", nil, false
	}
	tkids := r.prog.Children(parts[0])
	if len(tkids) != 2 || tkids[0].Kind() != frontend.NodeThisKeyword {
		return "", nil, false
	}
	return r.prog.Text(tkids[1]), parts[2], true
}

// pureCtorValue reports whether an expression may be reordered into the
// composite-literal fold: built only of names, literals, parentheses, and
// operators, with no call, no allocation, no assignment, and no this. A false
// answer is never wrong, it just keeps the general constructor form.
func (r *Renderer) pureCtorValue(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeIdentifier, frontend.NodeNumericLiteral, frontend.NodeStringLiteral,
		frontend.NodeBigIntLiteral, frontend.NodeNoSubstitutionTemplateLiteral,
		frontend.NodeTrueKeyword, frontend.NodeFalseKeyword:
		return true
	case frontend.NodeParenthesizedExpression, frontend.NodePrefixUnaryExpression:
		kids := r.prog.Children(n)
		return len(kids) == 1 && r.pureCtorValue(kids[0])
	case frontend.NodeBinaryExpression:
		parts := r.prog.Children(n)
		if len(parts) != 3 {
			return false
		}
		if op := r.prog.Text(parts[1]); op == "=" {
			return false
		} else if _, compound := compoundBaseOp(op); compound {
			return false
		}
		return r.pureCtorValue(parts[0]) && r.pureCtorValue(parts[2])
	default:
		return false
	}
}

// classMethodDecl emits one method as a pointer-receiver Go method: the
// signature from the checker like a function declaration's, the body lowered
// with this bound to the receiver.
func (r *Renderer) classMethodDecl(info *classInfo, m classMethod) (ast.Decl, error) {
	sig, ok := r.prog.SignatureAt(m.node)
	if !ok {
		return nil, &NotYetLowerable{Reason: "method has no call signature"}
	}
	if len(sig.TypeParams) != 0 {
		return nil, &NotYetLowerable{Reason: "generic method needs monomorphization, a later slice"}
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

	prevRet := r.retType
	r.retType = sig.Return
	defer func() { r.retType = prevRet }()
	prevClass, prevThis := r.curClass, r.thisName
	r.curClass, r.thisName = info, info.recv
	defer func() { r.curClass, r.thisName = prevClass, prevThis }()

	body, err := r.blockOf(m.node)
	if err != nil {
		return nil, err
	}
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ident(info.recv)},
			Type:  star(ident(info.goName)),
		}}},
		Name: ident(m.goName),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}, nil
}

// newClass lowers new Point(args) to the NewPoint constructor call, coercing
// each argument against the declared parameter the way an assignment coerces.
// The argument count must match the constructor exactly; optional and default
// parameters are the same later slice they are for functions.
func (r *Renderer) newClass(info *classInfo, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != len(info.ctorParams) {
		return nil, &NotYetLowerable{Reason: "new " + info.name + " with an argument count that differs from the constructor is a later slice"}
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for i, a := range argNodes {
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		paramName := r.prog.Children(info.ctorParams[i])[0]
		lowered, err = r.coerceToTarget(lowered, a, paramName)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	return &ast.CallExpr{Fun: ident("New" + info.goName), Args: args}, nil
}

// classMethodCall lowers recv.method(args) on a class instance to the Go method
// call. Arguments lower plainly, the same way a top-level function call's do.
func (r *Renderer) classMethodCall(info *classInfo, recv ast.Expr, method string, argNodes []frontend.Node) (ast.Expr, error) {
	m, ok := info.methodByName(method)
	if !ok {
		if _, isField := info.fieldByName(method); isField {
			return nil, &NotYetLowerable{Reason: "calling a field of class " + info.name + " as a function is a later slice"}
		}
		return nil, &NotYetLowerable{Reason: "class " + info.name + " has no method ." + method + " this slice lowers"}
	}
	sig, ok := r.prog.SignatureAt(m.node)
	if !ok {
		return nil, &NotYetLowerable{Reason: "method has no call signature"}
	}
	if len(argNodes) != len(sig.Params) {
		return nil, &NotYetLowerable{Reason: "method call with an argument count that differs from the declaration is a later slice"}
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(m.goName)}, Args: args}, nil
}

// classFieldOfTarget resolves a property access to the class field it stores
// into, when its receiver is this inside a lowered class body or a plain
// identifier the checker types as a registered class instance. The receiver
// restriction is what makes a compound store and an increment sound: a name or
// this cannot carry a side effect, so lowering it twice (the read and the
// write) cannot double one. A recognized class receiver with an unrecognized
// property is an error, not a fall-through, so the store never silently routes
// to a lowering that would treat the instance as something else.
func (r *Renderer) classFieldOfTarget(target frontend.Node) (*classInfo, classField, bool, error) {
	tkids := r.prog.Children(target)
	if len(tkids) != 2 {
		return nil, classField{}, false, nil
	}
	obj := tkids[0]
	if obj.Kind() != frontend.NodeThisKeyword && obj.Kind() != frontend.NodeIdentifier {
		return nil, classField{}, false, nil
	}
	info, ok := r.classReceiver(obj)
	if !ok {
		return nil, classField{}, false, nil
	}
	prop := r.prog.Text(tkids[1])
	f, ok := info.fieldByName(prop)
	if !ok {
		return nil, classField{}, false, &NotYetLowerable{Reason: "storing into ." + prop + " of class " + info.name + " is a later slice"}
	}
	return info, f, true, nil
}

// classFieldTarget resolves a property access to the lowered field selector
// when it is a class field store target, for the increment statement that needs
// only the left-hand side.
func (r *Renderer) classFieldTarget(target frontend.Node) (ast.Expr, bool, error) {
	_, _, ok, err := r.classFieldOfTarget(target)
	if err != nil || !ok {
		return nil, ok, err
	}
	lhs, err := r.lowerExpr(target)
	if err != nil {
		return nil, false, err
	}
	return lhs, true, nil
}

// classFieldAssign lowers a store into a class field, this.x = v inside a class
// body or p.x = v on an instance local, to the Go field assignment. It reports
// ok=false when the statement is not a store into a recognized class field, so
// lowerUpdate falls through to the local-identifier assignment. The receiver
// must be this or a plain identifier, so a compound store (p.x += v), which
// reads the receiver twice, cannot double a side effect.
func (r *Renderer) classFieldAssign(bin frontend.Node) (ast.Stmt, bool, error) {
	parts := r.prog.Children(bin)
	if len(parts) != 3 {
		return nil, false, nil
	}
	opText := r.prog.Text(parts[1])
	baseOp, compound := compoundBaseOp(opText)
	if opText != "=" && !compound {
		return nil, false, nil
	}
	target := parts[0]
	if target.Kind() != frontend.NodePropertyAccessExpression {
		return nil, false, nil
	}
	_, f, ok, err := r.classFieldOfTarget(target)
	if err != nil || !ok {
		return nil, ok, err
	}

	lhs, err := r.lowerExpr(target)
	if err != nil {
		return nil, false, err
	}
	var rhs ast.Expr
	if compound {
		rhs, err = r.combineBinary(baseOp, target, parts[2])
		if err != nil {
			return nil, false, err
		}
		if r.combineIsDynamic(baseOp, target, parts[2]) && !r.isDynamic(target) {
			rhs, err = r.coerceDynamicToStatic(rhs, target)
			if err != nil {
				return nil, false, err
			}
		}
	} else {
		rhs, err = r.lowerExpr(parts[2])
		if err != nil {
			return nil, false, err
		}
		rhs, err = r.coerceToTarget(rhs, parts[2], f.ident)
		if err != nil {
			return nil, false, err
		}
	}

	// A self-referential store collapses to the compound form and a step of one to
	// ++/--, so p.count = p.count + 1 prints p.count++, the way lowerAssign
	// collapses a local's step.
	tok := token.ASSIGN
	if b, ok := rhs.(*ast.BinaryExpr); ok {
		if ctok, ok := compoundAssignToken(b.Op); ok && sameFieldSelector(b.X, lhs) {
			tok = ctok
			rhs = b.Y
		}
	}
	assign := &ast.AssignStmt{Lhs: []ast.Expr{lhs}, Tok: tok, Rhs: []ast.Expr{rhs}}
	if inc, ok := incDecFromStep(assign); ok {
		return inc, true, nil
	}
	return assign, true, nil
}

// sameFieldSelector reports whether two expressions are the same simple field
// selector, an identifier dot field, so the compound collapse only fires on the
// exact receiver-and-field pair the assignment targets.
func sameFieldSelector(a, b ast.Expr) bool {
	sa, ok1 := a.(*ast.SelectorExpr)
	sb, ok2 := b.(*ast.SelectorExpr)
	if !ok1 || !ok2 || sa.Sel.Name != sb.Sel.Name {
		return false
	}
	xa, ok1 := sa.X.(*ast.Ident)
	xb, ok2 := sb.X.(*ast.Ident)
	return ok1 && ok2 && xa.Name == xb.Name
}

// subtreeHasKind reports whether any node under n (inclusive) has the kind.
func subtreeHasKind(prog *frontend.Program, n frontend.Node, kind frontend.NodeKind) bool {
	if n.Kind() == kind {
		return true
	}
	for _, c := range prog.Children(n) {
		if subtreeHasKind(prog, c, kind) {
			return true
		}
	}
	return false
}

// firstWord returns the first whitespace-delimited word of s, for the modifier
// checks that read a member's source spelling.
func firstWord(s string) string {
	if i := strings.IndexAny(s, " \t\n"); i >= 0 {
		return s[:i]
	}
	return s
}
