package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers a checked function to a runnable Go function (05_type_lowering
// sections 13 to 16): the signature from the checker, the body-scoped analyses
// set up around a body, and the arrow function forms. The statement and
// expression lowerings the body uses live in stmt.go, expr.go, and their
// siblings. Everything outside the covered subset hands back a NotYetLowerable
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

	params, err := r.funcParamFields(fn, sig)
	if err != nil {
		return nil, err
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return nil, err
	}

	// The declared return type is stashed for the duration of this body so a return
	// statement can coerce its value across the dynamic boundary, the way an
	// assignment does: a dynamic value returned from a function typed to return a
	// number runs ToNumber, and a static value returned as any boxes. It is saved
	// and restored so a later slice's nested function does not inherit the outer
	// return type.
	prevRet := r.retType
	r.retType = sig.Return
	defer func() { r.retType = prevRet }()

	// The union-locals set is scoped to this body the same way retType is, built
	// from both the signature parameters and the body declarations so a narrowed
	// read of either unwraps to the arm's field wherever it sits, and one function's
	// union bindings do not leak into another's reads.
	prevUnion := r.unionLocals
	var bodyStmts []frontend.Node
	if block, ok := r.funcBodyBlock(fn); ok {
		bodyStmts = r.prog.Children(block)
	}
	r.unionLocals = r.unionLocalsOf(sig.Params, bodyStmts)
	defer func() { r.unionLocals = prevUnion }()

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

// funcParamFields lowers a top-level function's parameters, and unlike the shared
// paramFields it accepts a default-valued parameter: an omittable parameter (index
// at or past MinArgs) becomes a plain Go field of its type when it carries a default
// the call site can fill, so the Go function reads it as an ordinary argument and
// every call supplies the default in the omitted slot. A default that reads a
// variable or makes a call needs the callee's parameter scope at the call site,
// which is not modeled yet, so it hands back; an omittable parameter with no default
// (a bare `x?: T`) still hands back on the undefined-optional synthesis. Methods and
// constructors keep the stricter paramFields, so a default there is a later slice.
func (r *Renderer) funcParamFields(fn frontend.Node, sig frontend.Signature) (*ast.FieldList, error) {
	paramNodes := r.funcParamNodes(fn)
	fields := &ast.FieldList{}
	for i, p := range sig.Params {
		pname, ok := localName(p.Name)
		if !ok {
			return nil, &NotYetLowerable{Reason: "parameter name is not a Go identifier"}
		}
		if i >= sig.MinArgs {
			def, ok := r.paramDefaultNode(paramNodes, i)
			if !ok {
				return nil, &NotYetLowerable{Flags: p.Type.Flags, Reason: "optional parameter needs call-site defaulting, a later slice"}
			}
			if !packageSafeInit(r.prog, def) {
				return nil, &NotYetLowerable{Reason: "a default parameter value that reads a variable or makes a call needs the callee's scope at the call site, a later slice"}
			}
		}
		pt, err := r.typeExpr(p.Type)
		if err != nil {
			return nil, err
		}
		fields.List = append(fields.List, &ast.Field{Names: []*ast.Ident{ident(pname)}, Type: pt})
	}
	return fields, nil
}

// funcParamNodes returns the parameter nodes of a function or arrow declaration in
// declaration order, so a caller can read a parameter's default off the AST, which
// the checker signature does not carry.
func (r *Renderer) funcParamNodes(fn frontend.Node) []frontend.Node {
	var out []frontend.Node
	for _, k := range r.prog.Children(fn) {
		if k.Kind() == frontend.NodeParameter {
			out = append(out, k)
		}
	}
	return out
}

// paramDefaultNode returns the default-value expression of the parameter at index i,
// if it has one. A parameter node's children are the name, an optional type
// annotation the shim leaves as an opaque unknown node, and an optional default
// value, which is a real expression node. The default is the first child past the
// name that is not the unknown type node; a default the shim itself leaves unknown
// (a rarer operator form) reads as absent, so the parameter hands back rather than
// lower a default the call site could not reconstruct.
func (r *Renderer) paramDefaultNode(paramNodes []frontend.Node, i int) (frontend.Node, bool) {
	if i < 0 || i >= len(paramNodes) {
		return nil, false
	}
	pkids := r.prog.Children(paramNodes[i])
	if len(pkids) == 0 {
		return nil, false
	}
	for _, c := range pkids[1:] {
		if c.Kind() != frontend.NodeUnknown {
			return c, true
		}
	}
	return nil, false
}

// calleeDefaults returns the default-value nodes of the function a call resolves to,
// aligned to the parameter list with a nil where a parameter has no default, or nil
// when the callee has no defaults at all. finishCall reads it to fill an omitted
// trailing argument with the parameter's default.
func (r *Renderer) calleeDefaults(sym frontend.Symbol) []frontend.Node {
	for _, d := range r.prog.Declarations(sym) {
		paramNodes := r.funcParamNodes(d)
		if len(paramNodes) == 0 {
			continue
		}
		out := make([]frontend.Node, len(paramNodes))
		found := false
		for i := range paramNodes {
			if def, ok := r.paramDefaultNode(paramNodes, i); ok {
				out[i] = def
				found = true
			}
		}
		if found {
			return out
		}
	}
	return nil
}

// funcOmittable reports whether the function a symbol names has a parameter a caller
// may omit, whether by a default value, a trailing `?`, or a rest. A function like
// that lowers to a Go func whose arity exceeds its minimal call, so using it as a
// value (a callback, a binding) where the slot expects the minimal arity would not
// type; such a use hands back until a defaulting wrapper is modeled.
func (r *Renderer) funcOmittable(sym frontend.Symbol) bool {
	for _, d := range r.prog.Declarations(sym) {
		if sig, ok := r.prog.SignatureAt(d); ok {
			if sig.MinArgs < len(sig.Params) || sig.RestParam != nil {
				return true
			}
		}
	}
	return false
}

// resultFields lowers the return type to the function's result list. A void or
// undefined return (the zero type carries no flags) is no result at all.
func (r *Renderer) resultFields(ret frontend.Type) (*ast.FieldList, error) {
	if isVoidReturn(ret) {
		return nil, nil
	}
	rt, err := r.typeExpr(ret)
	if err != nil {
		return nil, err
	}
	return &ast.FieldList{List: []*ast.Field{{Type: rt}}}, nil
}

// isVoidReturn reports whether a return type carries no value: a bare void, an
// undefined, or the zero type a function with no annotated return and no value
// carries. These are the shapes that give a func literal no result, whether the
// return sits on a function declaration or a concise-body arrow.
func isVoidReturn(ret frontend.Type) bool {
	return ret.Flags == 0 || ret.Flags&frontend.TypeVoid != 0 || ret.Flags&frontend.TypeUndefined != 0
}

// blockOf finds the function's body block and lowers it. A function with no body
// (an overload signature or a declare) is not a lowerable unit.
func (r *Renderer) blockOf(fn frontend.Node) (*ast.BlockStmt, error) {
	block, ok := r.funcBodyBlock(fn)
	if !ok {
		return nil, &NotYetLowerable{Reason: "function has no body block (declare or overload)"}
	}
	return r.scopedBlock(block, 0)
}

// funcBodyBlock returns a function's body block node, and ok=false when the
// function has none (an overload signature or a declare). It is the one place the
// body block is found, shared by blockOf and the union-locals pre-pass so both read
// the same block.
func (r *Renderer) funcBodyBlock(fn frontend.Node) (frontend.Node, bool) {
	var block frontend.Node
	for _, c := range r.prog.Children(fn) {
		if c.Kind() == frontend.NodeBlock {
			block = c
		}
	}
	return block, block != nil
}

// scopedBlock lowers a body block with the per-body analysis sets scoped to
// it, skipping the first skip statements. The only caller that skips is the
// derived constructor, whose validated super() call is emitted as the base
// assignment before the field initializers rather than lowered in place.
func (r *Renderer) scopedBlock(block frontend.Node, skip int) (*ast.BlockStmt, error) {
	// The int32 specialization set is computed once over the whole body and held for
	// the duration of this function, so a counter declared in a nested loop is seen
	// and the nested block inherits the same set. It is saved and restored like
	// retType so one function's specialized locals do not leak into another.
	// The const-integer map is set first, since the int32 specialization and the
	// counter and fixed-array proofs all read it to resolve a const N used as a bound
	// or a length to its literal value.
	prevCN := r.constInt
	r.constInt = r.constIntsOf(r.prog.Children(block))
	defer func() { r.constInt = prevCN }()

	prev := r.int32Locals
	r.int32Locals = r.int32LocalsOf(r.prog.Children(block))
	defer func() { r.int32Locals = prev }()

	// The proven-index sets ride the same body scope: a counter's range and a
	// fixed-length integer array are both facts about this body, so an access at a
	// proven-in-range index anywhere in it lowers to the native slice, and one
	// function's proofs do not leak into another's accesses.
	prevCI := r.counterIvl
	r.counterIvl = r.counterIvlOf(r.prog.Children(block))
	defer func() { r.counterIvl = prevCI }()
	prevFA := r.fixedTArr
	r.fixedTArr = r.fixedTypedArraysOf(r.prog.Children(block))
	defer func() { r.fixedTArr = prevFA }()

	// The optional-locals set is scoped to this body the same way, so a narrowed read
	// of an option unwraps with .Get() wherever in the body it sits and one function's
	// options do not leak into another's reads.
	prevOpt := r.optLocals
	r.optLocals = r.optLocalsOf(r.prog.Children(block))
	defer func() { r.optLocals = prevOpt }()

	// The builder set is scoped to this body the same way: a template site anywhere
	// in the body, however deeply nested, records its builder here, and blockOf
	// hoists a var for each above the whole body so a builder inside a loop is reused
	// across iterations. It is saved and restored so one function's builders do not
	// leak into another's hoist.
	prevSB := r.strBuilders
	r.strBuilders = nil
	defer func() { r.strBuilders = prevSB }()

	// The bigint ownership set is scoped to this body the same way, so a
	// self-referential bigint update anywhere in the body mutates in place exactly
	// when this body proves the local unshared, and one function's owned locals do
	// not leak into another.
	prevBig := r.bigOwned
	r.bigOwned = r.bigOwnedLocalsOf(r.prog.Children(block))
	defer func() { r.bigOwned = prevBig }()
	stmts, err := r.lowerStatements(r.prog.Children(block)[skip:])
	if err != nil {
		return nil, err
	}
	return &ast.BlockStmt{List: r.hoistStrBuilders(stmts)}, nil
}

// arrowFunc lowers an arrow function to a Go function literal. Both a concise
// expression body, the shape a map or filter callback almost always takes, and a
// block body, which runs the statement lowering inside the literal, are covered.
// Each parameter takes its type from the checker, which has already applied the
// contextual type from the call site, so a bare x in xs.map(x => ...) is typed
// number without an annotation. A concise body's result type comes from the body
// expression; a block body's comes from the arrow's own call signature, the same
// return the enclosed return statements coerce to. This makes an arrow usable
// anywhere an expression is, but its first consumers are the higher-order array
// methods and go: callbacks.
func (r *Renderer) arrowFunc(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) < 2 {
		return nil, &NotYetLowerable{Reason: "arrow function did not expose parameters and a body"}
	}
	body := kids[len(kids)-1]
	fields := make([]*ast.Field, 0, len(kids))
	for _, k := range kids[:len(kids)-1] {
		if k.Kind() != frontend.NodeParameter {
			continue
		}
		pkids := r.prog.Children(k)
		// A bare parameter is a lone identifier; an annotated one, (n: number),
		// carries the type node as a second child. The type is already folded into
		// the checker's answer for the identifier, so we read the name off the first
		// child and let the annotation ride along. Anything whose first child is not
		// the identifier is a rest element or a binding pattern, and any extra child
		// past the annotation is a default value that would need call-site
		// defaulting; both stay a later slice.
		if len(pkids) == 0 || pkids[0].Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: "arrow parameter that is not a plain identifier is a later slice"}
		}
		for _, extra := range pkids[1:] {
			if extra.Kind() != frontend.NodeUnknown {
				return nil, &NotYetLowerable{Reason: "arrow parameter with a default value is a later slice"}
			}
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
	if body.Kind() == frontend.NodeBlock {
		return r.blockBodyArrow(n, fields)
	}
	bodyType := r.prog.TypeAt(body)
	loweredBody, err := r.lowerExpr(body)
	if err != nil {
		return nil, err
	}
	// A void body, the shape a callback that runs for its effect takes ((i) =>
	// console.log(i) against a Go func(int)), gives the func literal no result and
	// stands the body in the statement position, the same way resultFields drops a
	// void return. Only a call expression is a legal Go statement, so a void body
	// that lowered to anything else hands back rather than emit invalid Go.
	if isVoidReturn(bodyType) {
		call, ok := loweredBody.(*ast.CallExpr)
		if !ok {
			return nil, &NotYetLowerable{Reason: "arrow with a void body that is not a call is a later slice"}
		}
		return &ast.FuncLit{
			Type: &ast.FuncType{Params: &ast.FieldList{List: fields}},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: call}}},
		}, nil
	}
	retType, err := r.typeExpr(bodyType)
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

// arrowResultType is the Go type an arrow returns, wherever a caller needs it
// spelled out (the type-changing map's result parameter, for one). A concise body
// carries the result on the body expression itself, which the checker has already
// inferred; a block body has no single body expression, so the result comes from
// the arrow's own call signature, the same return the enclosed return statements
// coerce to. Both routes end at the same typeExpr, so the two arrow forms give the
// map the same U.
func (r *Renderer) arrowResultType(arrow frontend.Node) (ast.Expr, error) {
	rt, ok := r.arrowResultFrontendType(arrow)
	if !ok {
		return nil, &NotYetLowerable{Reason: "arrow function with a block body has no call signature"}
	}
	return r.typeExpr(rt)
}

// arrowResultFrontendType is the checker's type for what an arrow returns, the
// frontend type behind arrowResultType. A caller that needs to inspect the result
// type rather than just spell it (flatMap, which asks whether the result is an
// array and takes its element type) reads it here. A concise body carries the
// result on its body expression; a block body's result comes from the arrow's own
// call signature. The bool is false only when a block-bodied arrow has no
// signature to read.
func (r *Renderer) arrowResultFrontendType(arrow frontend.Node) (frontend.Type, bool) {
	kids := r.prog.Children(arrow)
	body := kids[len(kids)-1]
	if body.Kind() == frontend.NodeBlock {
		sig, ok := r.prog.SignatureAt(arrow)
		if !ok {
			return frontend.Type{}, false
		}
		return sig.Return, true
	}
	return r.prog.TypeAt(body), true
}

// blockBodyArrow lowers an arrow whose body is a statement block, the shape a
// callback that needs a conditional or a local takes ((i) => { if (i === 2) {
// throw new Error(...); } }). It mirrors funcDecl: the return type comes from the
// arrow's own call signature, stashed on retType so an enclosed return coerces
// across the dynamic boundary the way a named function's does, and the body lowers
// through blockOf so the int32, optional-local, and builder scoping that runs for
// a named function runs inside the literal too. The parameters were already
// lowered by arrowFunc from the checker's contextual types, so this only adds the
// result and the lowered block.
func (r *Renderer) blockBodyArrow(n frontend.Node, fields []*ast.Field) (ast.Expr, error) {
	sig, ok := r.prog.SignatureAt(n)
	if !ok {
		return nil, &NotYetLowerable{Reason: "arrow function with a block body has no call signature"}
	}
	results, err := r.resultFields(sig.Return)
	if err != nil {
		return nil, err
	}
	prevRet := r.retType
	r.retType = sig.Return
	defer func() { r.retType = prevRet }()
	body, err := r.blockOf(n)
	if err != nil {
		return nil, err
	}
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{List: fields}, Results: results},
		Body: body,
	}, nil
}
