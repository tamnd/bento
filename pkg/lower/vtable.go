package lower

import (
	"go/ast"
	"go/token"
	"strconv"
)

// This file lowers virtual dispatch (05_type_lowering sections 13 and 14). A
// method some subclass overrides cannot resolve at the call site, so the
// hierarchy root grows one vtable: a struct of function pointers, one slot per
// overridden method, taking the root pointer as their receiver argument. The
// root struct carries an unexported vtable pointer, each constructor pins the
// class's vtable var before any initializer runs, and the original method name
// becomes a one-line entry that calls through the slot, so every call site
// keeps the direct spelling and only overridden methods pay for indirection.
// The method bodies move to lowercase Impl names; a derived class's slot wraps
// its Impl behind a pointer cast that is sound because the embedded base is
// the derived struct's first field, so both pointers address the same word.
//
// Construction splits in a virtual hierarchy: NewX allocates and sets the
// vtable, then an initX function runs the base chain's initializers on the
// embedded base in place. The split keeps two JavaScript behaviors the plain
// *NewBase copy would lose: a virtual call inside a base constructor already
// dispatches to the derived override (a JS instance is its final class from
// the first line of the base constructor), and the derived vtable pointer,
// set before init, is never clobbered by a whole-struct base copy.

// root walks to the top of the class's base chain, the class that owns the
// vtable when the hierarchy has one.
func (c *classInfo) root() *classInfo {
	top := c
	for top.base != nil {
		top = top.base
	}
	return top
}

// hasVTable reports whether the class belongs to a hierarchy with virtual
// dispatch, which is what switches its constructor to the split form.
func (c *classInfo) hasVTable() bool {
	return len(c.root().vprops) > 0
}

// isVirtual reports whether the named method dispatches through the vtable.
func (c *classInfo) isVirtual(prop string) bool {
	return c.root().vprops[prop]
}

// chainHasAbstract reports whether the class or any ancestor is abstract,
// which routes its construction to the split form: an abstract base has no
// NewX, only an init a derived constructor runs on the embedded base.
func (c *classInfo) chainHasAbstract() bool {
	for ci := c; ci != nil; ci = ci.base {
		if ci.abstract {
			return true
		}
	}
	return false
}

// descendsFrom reports whether t is a proper ancestor of c, the test an upcast
// bridge runs before addressing the embedded base.
func (c *classInfo) descendsFrom(t *classInfo) bool {
	for b := c.base; b != nil; b = b.base {
		if b == t {
			return true
		}
	}
	return false
}

// vtableTypeName is the vtable struct's type name, spelled off the root the
// way a package-private companion type is (animalVTable for Animal).
func vtableTypeName(root *classInfo) string {
	return lowerFirst(root.goName) + "VTable"
}

// vtableVarName is the name of the vtable var instances of exactly this class
// dispatch through: the root's is its Base table, a subclass's carries the
// class's own name.
func vtableVarName(c *classInfo) string {
	if c.base == nil {
		return lowerFirst(c.goName) + "BaseVTable"
	}
	return lowerFirst(c.goName) + "VTable"
}

// implName is the emitted name of a virtual method's body, the original name
// made unexported with an Impl suffix so the exported name stays free for the
// dispatching entry.
func implName(m classMethod) string {
	return lowerFirst(m.goName) + "Impl"
}

// initName is the name of the split initializer a derived constructor calls
// on its embedded base.
func initName(c *classInfo) string {
	return "init" + c.goName
}

// vtableVarRef names the vtable var this class's constructor pins: the class's
// own var when it overrides anything or is the root, otherwise the nearest
// ancestor's, since a class adding no overrides dispatches exactly as its base
// does.
func vtableVarRef(c *classInfo) string {
	for ci := c; ; ci = ci.base {
		if ci.base == nil || len(ci.overrides) > 0 {
			return vtableVarName(ci)
		}
	}
}

// mintName reserves a render-time name (the vtable type and vars, the init
// functions) against the module's spoken identifiers, the same check
// registration runs for a constructor's name. what names the owner in the
// hand-back.
func (r *Renderer) mintName(name, what string) error {
	if r.classTaken[name] || goKeywords[name] {
		return &NotYetLowerable{Reason: "the module already speaks " + name + ", the name " + what + " needs"}
	}
	r.classTaken[name] = true
	return nil
}

// slotFuncType builds the Go function type of one vtable slot: the root
// pointer first, then the method's own parameters, so a slot call passes the
// receiver explicitly the way a method expression does. It returns the
// parameter names alongside for the callers that forward them.
func (r *Renderer) slotFuncType(root *classInfo, m classMethod) (*ast.FuncType, []string, error) {
	sig, ok := r.prog.SignatureAt(m.node)
	if !ok {
		return nil, nil, &NotYetLowerable{Reason: "method has no call signature"}
	}
	params, err := r.paramFields(sig)
	if err != nil {
		return nil, nil, err
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return nil, nil, err
	}
	names := []string{root.recv}
	for _, f := range params.List {
		names = append(names, f.Names[0].Name)
	}
	all := &ast.FieldList{List: append([]*ast.Field{{
		Names: []*ast.Ident{ident(root.recv)},
		Type:  star(ident(root.goName)),
	}}, params.List...)}
	return &ast.FuncType{Params: all, Results: results}, names, nil
}

// vtableTypeDecl emits the root's vtable struct, one slot per overridden
// method in the root's declaration order, each named by the method's
// unexported spelling.
func (r *Renderer) vtableTypeDecl(root *classInfo) (ast.Decl, error) {
	if err := r.mintName(vtableTypeName(root), "class "+root.name+"'s vtable type"); err != nil {
		return nil, err
	}
	fields := &ast.FieldList{}
	for _, m := range root.methods {
		if !root.vprops[m.prop] {
			continue
		}
		ft, _, err := r.slotFuncType(root, m)
		if err != nil {
			return nil, err
		}
		fields.List = append(fields.List, &ast.Field{
			Names: []*ast.Ident{ident(lowerFirst(m.goName))},
			Type:  ft,
		})
	}
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{&ast.TypeSpec{
			Name: ident(vtableTypeName(root)),
			Type: &ast.StructType{Fields: fields},
		}},
	}, nil
}

// vtableVarDecl emits the vtable var for one class, filling every slot with
// the Impl an instance of exactly this class dispatches to: the root's own
// Impl as a method expression when no ancestor between here and the root
// overrides the slot, otherwise a wrapper that casts the root pointer down to
// the overriding class and calls its Impl. The cast through unsafe.Pointer is
// sound because the embedded base chain puts the root at offset zero of every
// derived struct, and the wrapper is only ever reached through a vtable pinned
// by that exact class's constructor.
func (r *Renderer) vtableVarDecl(c *classInfo) (ast.Decl, error) {
	root := c.root()
	if err := r.mintName(vtableVarName(c), "class "+c.name+"'s vtable"); err != nil {
		return nil, err
	}
	lit := &ast.CompositeLit{Type: ident(vtableTypeName(root))}
	for _, m := range root.methods {
		if !root.vprops[m.prop] {
			continue
		}
		var owner *classInfo
		var impl classMethod
		for ci := c; ci != nil; ci = ci.base {
			if om, ok := ci.methodByName(m.prop); ok {
				owner, impl = ci, om
				break
			}
		}
		var slot ast.Expr
		if owner == root && impl.abstract {
			// An abstract slot has no Impl to point at. It panics instead, and
			// the panic is unreachable in well-typed code: the checker makes
			// every concrete subclass override the method, so every vtable a
			// constructor can pin fills this slot with a real body.
			var err error
			slot, err = r.abstractSlot(root, m)
			if err != nil {
				return nil, err
			}
		} else if owner == root {
			slot = &ast.SelectorExpr{
				X:   &ast.ParenExpr{X: star(ident(root.goName))},
				Sel: ident(implName(m)),
			}
		} else {
			same, err := r.sameMethodSignature(m, impl)
			if err != nil {
				return nil, err
			}
			if !same {
				return nil, &NotYetLowerable{Reason: "class " + owner.name + " overrides ." + m.prop + " with a signature that differs from " + root.name + "'s; a changed override signature is a later slice"}
			}
			slot, err = r.overrideWrapper(root, owner, m)
			if err != nil {
				return nil, err
			}
		}
		lit.Elts = append(lit.Elts, &ast.KeyValueExpr{Key: ident(lowerFirst(m.goName)), Value: slot})
	}
	return &ast.GenDecl{
		Tok: token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{
			Names:  []*ast.Ident{ident(vtableVarName(c))},
			Values: []ast.Expr{lit},
		}},
	}, nil
}

// abstractSlot builds the vtable slot of an abstract method: a function of
// the slot's type whose body is one panic naming the method.
func (r *Renderer) abstractSlot(root *classInfo, m classMethod) (ast.Expr, error) {
	ft, _, err := r.slotFuncType(root, m)
	if err != nil {
		return nil, err
	}
	call := &ast.CallExpr{
		Fun:  ident("panic"),
		Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(root.name + "." + m.prop + " is abstract")}},
	}
	return &ast.FuncLit{
		Type: ft,
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: call}}},
	}, nil
}

// overrideWrapper builds the slot function for an override below the root: a
// literal that casts the root pointer to the overriding class and calls its
// Impl, forwarding the parameters unchanged.
func (r *Renderer) overrideWrapper(root, owner *classInfo, m classMethod) (ast.Expr, error) {
	ft, names, err := r.slotFuncType(root, m)
	if err != nil {
		return nil, err
	}
	r.requireImport("unsafe")
	cast := &ast.CallExpr{
		Fun: &ast.ParenExpr{X: star(ident(owner.goName))},
		Args: []ast.Expr{&ast.CallExpr{
			Fun:  sel("unsafe", "Pointer"),
			Args: []ast.Expr{ident(names[0])},
		}},
	}
	args := make([]ast.Expr, 0, len(names)-1)
	for _, n := range names[1:] {
		args = append(args, ident(n))
	}
	call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: cast, Sel: ident(implName(m))}, Args: args}
	var stmt ast.Stmt = &ast.ExprStmt{X: call}
	if ft.Results != nil {
		stmt = &ast.ReturnStmt{Results: []ast.Expr{call}}
	}
	return &ast.FuncLit{Type: ft, Body: &ast.BlockStmt{List: []ast.Stmt{stmt}}}, nil
}

// sameMethodSignature reports whether an override's parameter and return types
// print to the same Go as the root's declaration, the condition under which
// the override's Impl fits the root-typed slot behind the pointer cast. The
// checker permits looser overrides (a covariant return, an optional extra
// parameter); those change the Go type and hand back at the caller.
func (r *Renderer) sameMethodSignature(a, b classMethod) (bool, error) {
	at, _, err := r.methodTypeText(a)
	if err != nil {
		return false, err
	}
	bt, _, err := r.methodTypeText(b)
	if err != nil {
		return false, err
	}
	return at == bt, nil
}

// methodTypeText prints a method's lowered parameter and result types, names
// stripped, for the signature comparison above.
func (r *Renderer) methodTypeText(m classMethod) (string, bool, error) {
	sig, ok := r.prog.SignatureAt(m.node)
	if !ok {
		return "", false, &NotYetLowerable{Reason: "method has no call signature"}
	}
	params, err := r.paramFields(sig)
	if err != nil {
		return "", false, err
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return "", false, err
	}
	text := "("
	for i, f := range params.List {
		if i > 0 {
			text += ", "
		}
		s, err := printExpr(f.Type)
		if err != nil {
			return "", false, err
		}
		text += s
	}
	text += ")"
	if results != nil {
		s, err := printExpr(results.List[0].Type)
		if err != nil {
			return "", false, err
		}
		text += " " + s
	}
	return text, results != nil, nil
}

// virtualEntryDecl emits the root's dispatching method under the original
// exported name: one line that calls the pinned vtable's slot with the
// receiver and the parameters. Every existing call site, including a call on a
// derived receiver served by promotion, reaches dispatch through this entry
// without changing its spelling.
func (r *Renderer) virtualEntryDecl(root *classInfo, m classMethod) (ast.Decl, error) {
	sig, ok := r.prog.SignatureAt(m.node)
	if !ok {
		return nil, &NotYetLowerable{Reason: "method has no call signature"}
	}
	params, err := r.paramFields(sig)
	if err != nil {
		return nil, err
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return nil, err
	}
	args := []ast.Expr{ident(root.recv)}
	for _, f := range params.List {
		args = append(args, ident(f.Names[0].Name))
	}
	call := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.SelectorExpr{X: ident(root.recv), Sel: ident("vtable")},
			Sel: ident(lowerFirst(m.goName)),
		},
		Args: args,
	}
	var stmt ast.Stmt = &ast.ExprStmt{X: call}
	if results != nil {
		stmt = &ast.ReturnStmt{Results: []ast.Expr{call}}
	}
	return &ast.FuncDecl{
		Recv: &ast.FieldList{List: []*ast.Field{{
			Names: []*ast.Ident{ident(root.recv)},
			Type:  star(ident(root.goName)),
		}}},
		Name: ident(m.goName),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: &ast.BlockStmt{List: []ast.Stmt{stmt}},
	}, nil
}

// classNeedsInit reports whether constructing the class runs any code beyond
// allocation: a declared constructor, a field initializer, or a base that
// needs one. A class that needs none gets no init function and its subclasses
// skip the base init call; that skip is sound because a false answer implies
// the whole chain up declares no constructor, so there are no super arguments
// to evaluate either.
func classNeedsInit(c *classInfo) bool {
	if c.ctor != nil {
		return true
	}
	for _, f := range c.fields {
		if f.init != nil {
			return true
		}
	}
	return c.base != nil && classNeedsInit(c.base)
}

// splitCtorDecls emits a split hierarchy's construction: NewX allocates, pins
// the class's vtable when the hierarchy has one, runs the initializers, and
// returns. When the class is itself extended and has anything to initialize,
// the initializers split into an initX function taking the receiver, so a
// subclass constructor can run them on its embedded base in place, under the
// subclass's already-pinned vtable. An abstract class emits the init function
// alone: the checker rejects new on it, so there is no NewX to allocate one.
func (r *Renderer) splitCtorDecls(info *classInfo, params *ast.FieldList) ([]ast.Decl, error) {
	if info.abstract {
		if !info.extended || !classNeedsInit(info) {
			return nil, nil
		}
		if err := r.mintName(initName(info), "class "+info.name+"'s init function"); err != nil {
			return nil, err
		}
		initDecl, err := r.initFuncDecl(info)
		if err != nil {
			return nil, err
		}
		return []ast.Decl{initDecl}, nil
	}
	stmts := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ident(info.recv)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(info.goName)}}},
		},
	}
	if info.hasVTable() {
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{&ast.SelectorExpr{X: ident(info.recv), Sel: ident("vtable")}},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{&ast.UnaryExpr{Op: token.AND, X: ident(vtableVarRef(info))}},
		})
	}
	if info.extended && classNeedsInit(info) {
		if err := r.mintName(initName(info), "class "+info.name+"'s init function"); err != nil {
			return nil, err
		}
		args := []ast.Expr{ident(info.recv)}
		for _, f := range params.List {
			args = append(args, ident(f.Names[0].Name))
		}
		stmts = append(stmts,
			&ast.ExprStmt{X: &ast.CallExpr{Fun: ident(initName(info)), Args: args}},
			&ast.ReturnStmt{Results: []ast.Expr{ident(info.recv)}},
		)
		newDecl := &ast.FuncDecl{
			Name: ident("New" + info.goName),
			Type: &ast.FuncType{
				Params:  params,
				Results: &ast.FieldList{List: []*ast.Field{{Type: star(ident(info.goName))}}},
			},
			Body: &ast.BlockStmt{List: stmts},
		}
		initDecl, err := r.initFuncDecl(info)
		if err != nil {
			return nil, err
		}
		return []ast.Decl{newDecl, initDecl}, nil
	}
	initStmts, err := r.ctorInitStmts(info)
	if err != nil {
		return nil, err
	}
	stmts = append(stmts, initStmts...)
	stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{ident(info.recv)}})
	return []ast.Decl{&ast.FuncDecl{
		Name: ident("New" + info.goName),
		Type: &ast.FuncType{
			Params:  params,
			Results: &ast.FieldList{List: []*ast.Field{{Type: star(ident(info.goName))}}},
		},
		Body: &ast.BlockStmt{List: stmts},
	}}, nil
}

// initFuncDecl builds the initX function a derived constructor calls on the
// embedded base: the receiver first, then the constructor's own parameters,
// running the base chain's init, the field initializers, and the body.
func (r *Renderer) initFuncDecl(info *classInfo) (*ast.FuncDecl, error) {
	initParams, err := r.ctorParamFields(info)
	if err != nil {
		return nil, err
	}
	initParams.List = append([]*ast.Field{{
		Names: []*ast.Ident{ident(info.recv)},
		Type:  star(ident(info.goName)),
	}}, initParams.List...)
	initStmts, err := r.ctorInitStmts(info)
	if err != nil {
		return nil, err
	}
	return &ast.FuncDecl{
		Name: ident(initName(info)),
		Type: &ast.FuncType{Params: initParams},
		Body: &ast.BlockStmt{List: initStmts},
	}, nil
}

// ctorInitStmts builds the initializer statements a virtual-hierarchy
// constructor runs after the vtable is pinned: the base chain's init on the
// embedded base, the field initializers in declaration order, then the
// constructor body, the order JavaScript runs super, the initializers, and the
// body.
func (r *Renderer) ctorInitStmts(info *classInfo) ([]ast.Stmt, error) {
	var stmts []ast.Stmt
	if info.base != nil && classNeedsInit(info.base) {
		superArgs, err := r.superCtorArgs(info)
		if err != nil {
			return nil, err
		}
		args := append([]ast.Expr{&ast.UnaryExpr{
			Op: token.AND,
			X:  &ast.SelectorExpr{X: ident(info.recv), Sel: ident(info.base.goName)},
		}}, superArgs...)
		stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{Fun: ident(initName(info.base)), Args: args}})
	}
	inits, err := r.fieldInitStmts(info)
	if err != nil {
		return nil, err
	}
	stmts = append(stmts, inits...)
	body, err := r.ctorBodyStmts(info)
	if err != nil {
		return nil, err
	}
	return append(stmts, body...), nil
}
