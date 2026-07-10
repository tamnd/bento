package lower

import (
	"go/ast"
	"go/token"
	"maps"
	"strconv"
	"strings"

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

	// A function declaration whose name later carries own data properties (foo.x = 1)
	// is a callable object, not a bare func. The callable-object model interns a
	// `type Foo struct { Call func(); ... }` for that shape, which collides with the
	// `func Foo` this declaration emits: two Foo declarations in one block, which does
	// not compile. Modeling a named function that is also an object is a later slice,
	// so hand back rather than emit the colliding pair.
	if r.isCallableObject(r.prog.TypeAt(fn)) {
		return nil, &NotYetLowerable{Reason: "a function declaration with own properties is a callable object, a later slice"}
	}

	sig, ok := r.prog.SignatureAt(fn)
	if !ok {
		return nil, &NotYetLowerable{Reason: "function has no call signature"}
	}
	if len(sig.TypeParams) != 0 {
		return nil, &NotYetLowerable{Reason: "generic function needs monomorphization, a later slice"}
	}
	params, err := r.funcParamFields(fn, sig)
	if err != nil {
		return nil, err
	}
	// A rest parameter gathers the trailing arguments into an array, so it lowers to
	// a final Go field of the array type its `T[]` annotation carries; every call
	// packs its extra arguments into that array at the call site.
	if sig.RestParam != nil {
		restField, err := r.restParamField(*sig.RestParam)
		if err != nil {
			return nil, err
		}
		params.List = append(params.List, restField)
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

	// The dynamic-locals set rides the same scope: a parameter or local typed any
	// binds a boxed value.Value, and a read of it the checker narrowed to one
	// primitive unwraps through the matching accessor wherever in the body it
	// sits.
	prevDyn := r.dynLocals
	r.dynLocals = r.dynLocalsOf(sig.Params, bodyStmts)
	defer func() { r.dynLocals = prevDyn }()

	body, err := r.blockOf(fn)
	if err != nil {
		return nil, err
	}
	// A destructured parameter lowered to a synthesized Go field holding the whole
	// object or array; the names the pattern bound are read from it at the top of the
	// body, so the body sees the same names the source destructured.
	binds, err := r.paramDestructureBindings(r.funcParamNodes(fn), sig)
	if err != nil {
		return nil, err
	}
	if len(binds) != 0 {
		body.List = append(binds, body.List...)
	}
	r.endWithImplicitUndefinedReturn(body, bodyStmts, sig.Return)

	return &ast.FuncDecl{
		Name: ident(name),
		Type: &ast.FuncType{Params: params, Results: results},
		Body: body,
	}, nil
}

// endWithImplicitUndefinedReturn appends the return undefined a JavaScript
// function runs off its end when the body can complete without a return and the
// declared return type is dynamic. TypeScript only lets a body fall through when
// the return type admits undefined, and any and unknown are the ones that lower to
// a value.Value slot, so a static return never reaches here. Go then requires the
// trailing return the switch or if the body ends on cannot provide, and undefined
// is the value the fall-through yields, so this closes the gap without changing
// what a returning path produces.
func (r *Renderer) endWithImplicitUndefinedReturn(body *ast.BlockStmt, bodyStmts []frontend.Node, ret frontend.Type) {
	if body == nil || r.bodyTerminates(bodyStmts) {
		return
	}
	if ret.Flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 {
		return
	}
	r.requireImport(valuePkg)
	body.List = append(body.List, &ast.ReturnStmt{Results: []ast.Expr{sel("value", "Undefined")}})
}

// paramFields lowers each parameter to a Go field with its lowered type. An
// optional parameter (one a caller may omit, so its index is at or past the
// signature's MinArgs) lowers only when its type is dynamic: a dynamic slot
// holds undefined natively, so an omitting call site fills it with
// value.Undefined and the body reads the same absent value JavaScript binds. A
// static optional still hands back, because its Go type has no room for the
// undefined an omission means; the value.Opt[T] synthesis is a later slice. Its
// type carrying an explicit undefined member is not what marks it optional
// here, since the checker reports the same T | undefined type for a required
// parameter annotated that way; the caller-omittable distinction is MinArgs
// alone.
func (r *Renderer) paramFields(sig frontend.Signature) (*ast.FieldList, error) {
	fields := &ast.FieldList{}
	for i, p := range sig.Params {
		if i >= sig.MinArgs && p.Type.Flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 {
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
			switch {
			case ok:
				// A default that reads a module binding or calls a top-level function
				// lowers at the omitting call site: the binding is hoisted to a package
				// var (its read inside this default keeps it cross-boundary) and a
				// top-level function is always package-visible, so the call site sees the
				// same value the callee scope would. A default that reads an earlier
				// parameter is the one form the call site cannot reconstruct, since that
				// parameter is in scope only inside the callee, so it hands back.
				if r.defaultReadsOwnParam(sig, def) {
					return nil, &NotYetLowerable{Reason: "a default parameter value that reads an earlier parameter needs the callee's scope, a later slice"}
				}
			case p.Type.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0:
				// A bare optional of dynamic type needs no default: the omitted slot
				// fills with value.Undefined at the call site, the same absent value
				// the language binds.
			default:
				return nil, &NotYetLowerable{Flags: p.Type.Flags, Reason: "optional parameter needs call-site defaulting, a later slice"}
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

// restParamField lowers a rest parameter to its Go field. The parameter's type is
// the `T[]` the checker gives the gathered arguments, so it lowers to the same
// *value.Array[T] a plain array parameter takes, and the body reads it as an array
// with no special casing; only the call site differs, packing the trailing
// arguments into the array rather than passing one already built.
func (r *Renderer) restParamField(rest frontend.Param) (*ast.Field, error) {
	name, ok := localName(rest.Name)
	if !ok {
		return nil, &NotYetLowerable{Reason: "rest parameter name is not a Go identifier"}
	}
	pt, err := r.typeExpr(rest.Type)
	if err != nil {
		return nil, err
	}
	return &ast.Field{Names: []*ast.Ident{ident(name)}, Type: pt}, nil
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

// defaultReadsOwnParam reports whether a parameter default reads one of the
// function's own parameters. Such a default is evaluated in the callee's scope,
// where the earlier parameters are bound, so the omitting call site cannot
// reconstruct it and the parameter hands back. Any other identifier the default
// reads resolves to a module binding or a top-level function, both of which the
// call site can see, so only a self-parameter read blocks the call-site fill. A
// property access reads a binding only on its object side, so the member name is
// not treated as a parameter read.
func (r *Renderer) defaultReadsOwnParam(sig frontend.Signature, def frontend.Node) bool {
	names := make(map[string]bool, len(sig.Params))
	for _, p := range sig.Params {
		names[p.Name] = true
	}
	var reads func(n frontend.Node) bool
	reads = func(n frontend.Node) bool {
		kids := r.prog.Children(n)
		if n.Kind() == frontend.NodeIdentifier {
			return names[r.prog.Text(n)]
		}
		if n.Kind() == frontend.NodePropertyAccessExpression && len(kids) == 2 {
			return reads(kids[0])
		}
		for _, c := range kids {
			if reads(c) {
				return true
			}
		}
		return false
	}
	return reads(def)
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
// undefined, the zero type a function with no annotated return and no value
// carries, or never. A never function always throws or loops, so no call site
// ever reads a result from it; giving it no Go result is the whole lowering.
// These are the shapes that give a func literal no result, whether the return
// sits on a function declaration or a concise-body arrow.
func isVoidReturn(ret frontend.Type) bool {
	return ret.Flags == 0 || ret.Flags&(frontend.TypeVoid|frontend.TypeUndefined|frontend.TypeNever) != 0
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
	return r.scopedBlockRange(block, skip, len(r.prog.Children(block)))
}

// scopedBlockRange lowers a contiguous slice children[lo:hi] of a block while
// computing every body-scoped analysis (the int32 and int64 tiers, counter
// ranges, var hoisting, string builders) over the whole block, so a fact proven
// anywhere in the body applies to the lowered slice and a range that omits a
// statement (a derived constructor's super call, which the caller emits as the
// base assignment) still sees the body it belongs to. The common case lowers the
// whole block through scopedBlock; a range is used only where a caller emits part
// of the body itself.
func (r *Renderer) scopedBlockRange(block frontend.Node, lo, hi int) (*ast.BlockStmt, error) {
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

	// A nested function's returns are its own, not the enclosing try's, so the
	// try return mode resets around every function body; every function body
	// lowering runs through here, so this is the one reset site.
	prevTryRet := r.tryRet
	r.tryRet = tryRetNone
	defer func() { r.tryRet = prevTryRet }()

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

	// The int64 tier runs after the int32 set and the counter ranges are in place,
	// since its interval proof reads both: an int32 local or a bounded counter is a
	// known-range leaf inside an int64 candidate's writes.
	prevI64 := r.int64Locals
	r.int64Locals = r.int64LocalsOf(r.prog.Children(block))
	defer func() { r.int64Locals = prevI64 }()

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

	// A var written in a nested block of this body and read outside it hoists to a
	// declaration at the top of the function, the same function-scoping the module
	// body gets, so the var is one binding the whole body shares. A hoisted binding
	// reads at one Go type, so it is kept off the int32 and int64 tiers.
	bodyStmts := r.prog.Children(block)[lo:hi]
	hoistDecls, restoreHoist, err := r.enterVarHoistScope(bodyStmts)
	if err != nil {
		return nil, err
	}
	defer restoreHoist()
	for name := range r.hoistedVars {
		delete(r.int32Locals, name)
		delete(r.int64Locals, name)
	}
	// A callable-object binding captured by an earlier statement of this body has its
	// pointer declared at the top of the function, the same forward hoist the module
	// body gets, so the closure closes over a variable already in scope.
	fwdDecls, restoreFwd, err := r.enterFwdHoistScope(bodyStmts)
	if err != nil {
		return nil, err
	}
	defer restoreFwd()
	stmts, err := r.lowerStatements(bodyStmts)
	if err != nil {
		return nil, err
	}
	stmts = append(hoistDecls, stmts...)
	stmts = append(fwdDecls, stmts...)
	return &ast.BlockStmt{List: r.hoistStrBuilders(stmts)}, nil
}

// closureParamFields lowers the parameters of an arrow or function expression to
// Go fields. Both forms share the shape: one plain identifier per parameter, its
// type folded into the checker's answer for that identifier, so a bare x in
// xs.map(x => ...) is typed number without an annotation. A rest element or a
// binding pattern (whose first child is not a lone identifier), and a default
// value (an extra child past the annotation), each stay a later slice, named by
// noun so the reason reads for the form the caller lowered. A named function
// expression's own name is a NodeIdentifier child, not a NodeParameter, so it is
// skipped here and ruled out separately by functionExpr.
func (r *Renderer) closureParamFields(n frontend.Node, noun string) ([]*ast.Field, error) {
	kids := r.prog.Children(n)
	fields := make([]*ast.Field, 0, len(kids))
	for _, k := range kids {
		if k.Kind() != frontend.NodeParameter {
			continue
		}
		pkids := r.prog.Children(k)
		if len(pkids) == 0 || pkids[0].Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: noun + " parameter that is not a plain identifier is a later slice"}
		}
		for _, extra := range pkids[1:] {
			if extra.Kind() != frontend.NodeUnknown {
				return nil, &NotYetLowerable{Reason: noun + " parameter with a default value is a later slice"}
			}
		}
		name, ok := localName(r.prog.Text(pkids[0]))
		if !ok {
			return nil, &NotYetLowerable{Reason: noun + " parameter is not a Go identifier"}
		}
		ptype, err := r.typeExpr(r.prog.TypeAt(pkids[0]))
		if err != nil {
			return nil, err
		}
		fields = append(fields, &ast.Field{Names: []*ast.Ident{ident(name)}, Type: ptype})
	}
	return fields, nil
}

// paramDestructureBindings returns the statements that bind the names an object or
// array pattern parameter destructured, read from the synthesized Go field the
// pattern lowered to. Go has no destructuring parameter, so the whole object or
// array arrives in one field (named __0, __1, and so on) and the body reads the
// bound names out of it at entry, the same selector and indexed reads a `const {a}
// = o` or `const [x] = xs` statement lowers to. A plain-identifier parameter
// contributes nothing. Only the shorthand shapes the statement destructuring
// already covers are lowered; a rename, default, rest, or nested pattern hands back.
func (r *Renderer) paramDestructureBindings(paramNodes []frontend.Node, sig frontend.Signature) ([]ast.Stmt, error) {
	var out []ast.Stmt
	for i, pn := range paramNodes {
		if i >= len(sig.Params) {
			break
		}
		pkids := r.prog.Children(pn)
		if len(pkids) == 0 || pkids[0].Kind() == frontend.NodeIdentifier {
			continue
		}
		pat := pkids[0]
		goName, ok := localName(sig.Params[i].Name)
		if !ok {
			return nil, &NotYetLowerable{Reason: "a destructured parameter has no Go name to read from, a later slice"}
		}
		text := strings.TrimSpace(r.prog.Text(pat))
		switch {
		case strings.HasPrefix(text, "{"):
			stmts, err := r.objectPatternBindings(pat, goName, sig.Params[i].Type)
			if err != nil {
				return nil, err
			}
			out = append(out, stmts...)
		case strings.HasPrefix(text, "["):
			stmts, err := r.arrayPatternBindings(pat, goName, sig.Params[i].Type)
			if err != nil {
				return nil, err
			}
			out = append(out, stmts...)
		default:
			return nil, &NotYetLowerable{Reason: "a parameter that is neither an identifier nor an object or array pattern is a later slice"}
		}
	}
	return out, nil
}

// objectPatternBindings binds each shorthand name an object pattern parameter
// destructured from the field of the same name on the held object, name := __0.Name,
// the same struct-field selector a written-out property access lowers to. It mirrors
// flattenObjectDestructure's element loop over the pattern parameter's held value, so
// a rename, default, rest, or nested member hands back the same way the statement form
// does.
func (r *Renderer) objectPatternBindings(pat frontend.Node, goName string, objType frontend.Type) ([]ast.Stmt, error) {
	if objType.Flags&frontend.TypeObject == 0 {
		return nil, &NotYetLowerable{Reason: "an object-pattern parameter on a non-object type is a later slice"}
	}
	if _, err := r.decls.internStruct(r, objType); err != nil {
		return nil, err
	}
	elems := r.prog.Children(pat)
	if len(elems) == 0 {
		return nil, &NotYetLowerable{Reason: "an empty object-pattern parameter binds nothing"}
	}
	var out []ast.Stmt
	for _, el := range elems {
		ec := r.prog.Children(el)
		if len(ec) != 1 || ec[0].Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: "an object-pattern parameter rename, default, rest, or nested pattern is a later slice"}
		}
		name, ok := localName(r.prog.Text(ec[0]))
		if !ok {
			return nil, &NotYetLowerable{Reason: "a destructured parameter name is not a Go identifier"}
		}
		field, ok := exportedField(name)
		if !ok {
			return nil, &NotYetLowerable{Reason: "a destructured parameter property is not a Go field name"}
		}
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(name)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.SelectorExpr{X: ident(goName), Sel: ident(field)}},
		})
	}
	return out, nil
}

// arrayPatternBindings binds each name an array pattern parameter destructured from
// the matching position of the held array, name := __0.AtI(i), the same indexed read
// a written-out element access lowers to. It mirrors flattenArrayDestructure's element
// loop over the pattern parameter's held value: the type must be a homogeneous array,
// so a tuple whose positions differ hands back, and a hole, default, rest, or nested
// element is a later slice.
func (r *Renderer) arrayPatternBindings(pat frontend.Node, goName string, arrType frontend.Type) ([]ast.Stmt, error) {
	elemT, ok := r.prog.ElementType(arrType)
	if !ok {
		return nil, &NotYetLowerable{Reason: "an array-pattern parameter on a non-array or tuple type is a later slice"}
	}
	elemGo, err := r.typeExpr(elemT)
	if err != nil {
		return nil, err
	}
	elems := r.prog.Children(pat)
	if len(elems) == 0 {
		return nil, &NotYetLowerable{Reason: "an empty array-pattern parameter binds nothing"}
	}
	var out []ast.Stmt
	for i, el := range elems {
		ec := r.prog.Children(el)
		if len(ec) != 1 || ec[0].Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: "an array-pattern parameter hole, default, rest, or nested pattern is a later slice"}
		}
		name, ok := localName(r.prog.Text(ec[0]))
		if !ok {
			return nil, &NotYetLowerable{Reason: "a destructured parameter name is not a Go identifier"}
		}
		nameGo, err := r.typeExpr(r.prog.TypeAt(ec[0]))
		if err != nil {
			return nil, err
		}
		if same, err := sameGoType(nameGo, elemGo); err != nil {
			return nil, err
		} else if !same {
			return nil, &NotYetLowerable{Reason: "an array-pattern parameter whose element type differs from the array element type is a later slice"}
		}
		read := &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ident(goName), Sel: ident("AtI")},
			Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}},
		}
		out = append(out, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(name)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{read},
		})
	}
	return out, nil
}

// functionExpr lowers a function expression used as a value, the function(){}
// form the test262 assert prelude assigns to a const and to its members. A
// function expression always has a block body, which is the same closure a
// block-body arrow lowers to, so it routes through the same generator once the
// forms an arrow does not share are ruled out. An async or generator function is
// a coroutine, a body that reads this or arguments needs a receiver or the arity
// object neither a Go closure carries, and a named function expression needs the
// two-step a self-reference takes; each hands back.
func (r *Renderer) functionExpr(n frontend.Node) (ast.Expr, error) {
	text := strings.TrimSpace(r.prog.Text(n))
	if firstWord(text) == "async" {
		return nil, &NotYetLowerable{Reason: "an async function expression is a coroutine, a later slice"}
	}
	if strings.HasPrefix(strings.TrimSpace(strings.TrimPrefix(text, "function")), "*") {
		return nil, &NotYetLowerable{Reason: "a generator function expression is a coroutine, a later slice"}
	}
	if subtreeHasKind(r.prog, n, frontend.NodeThisKeyword) {
		return nil, &NotYetLowerable{Reason: "a function expression that reads this needs a receiver, a later slice"}
	}
	if subtreeHasIdent(r.prog, n, "arguments") {
		return nil, &NotYetLowerable{Reason: "a function expression that reads arguments needs the arity object, a later slice"}
	}
	fields, err := r.closureParamFields(n, "function")
	if err != nil {
		return nil, err
	}
	// A named function expression carries its own name as a NodeIdentifier child. The
	// name is in scope only inside the body, where a recursive call reads it, so it
	// takes the self-reference two-step: bind the closure to a Go local of its own
	// function type, then let the body's recursive calls resolve to that local. A name
	// the body never reads needs no two-step, so it lowers as a plain closure.
	if nameNode, ok := r.funcExprNameNode(n); ok {
		return r.namedFunctionExpr(n, nameNode, fields)
	}
	return r.blockBodyArrow(n, fields)
}

// isAnonymousFunctionDef reports whether a node is an anonymous function definition,
// the right-hand side named evaluation gives a name: an arrow function, which never
// carries a name, or a function expression with no name node. A named function
// expression already has its own name and is left alone.
func (r *Renderer) isAnonymousFunctionDef(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeArrowFunction:
		return true
	case frontend.NodeFunctionExpression:
		_, named := r.funcExprNameNode(n)
		return !named
	}
	return false
}

// funcExprNameNode returns a function expression's own name node, the NodeIdentifier
// child that sits before its parameters, if it has one. An anonymous function
// expression has no such child and reads as not-named here.
func (r *Renderer) funcExprNameNode(n frontend.Node) (frontend.Node, bool) {
	for _, k := range r.prog.Children(n) {
		if k.Kind() == frontend.NodeIdentifier {
			return k, true
		}
	}
	return nil, false
}

// namedFunctionExpr lowers a named function expression through the self-reference
// two-step. Go has no self-referential function literal, so the closure binds to a
// declared local first and the literal is assigned second, which lets the body call
// the local by name:
//
//	func() func(float64) float64 {
//		var fac func(float64) float64
//		fac = func(n float64) float64 { ... fac(n-1) ... }
//		return fac
//	}()
//
// The self name is registered so a recursive call inside the body resolves to the
// local rather than to a top-level function name. When the body never reads the
// name, the two-step is unnecessary and the closure lowers plainly.
func (r *Renderer) namedFunctionExpr(n, nameNode frontend.Node, fields []*ast.Field) (ast.Expr, error) {
	sym, ok := r.prog.SymbolAt(nameNode)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a named function expression whose name has no symbol is a later slice"}
	}
	goName, ok := localName(r.prog.Text(nameNode))
	if !ok {
		return nil, &NotYetLowerable{Reason: "a named function expression whose name is not a Go identifier is a later slice"}
	}
	if !r.subtreeReferencesSymbol(r.funcExprBody(n), sym) {
		return r.blockBodyArrow(n, fields)
	}
	prev, had := r.funcExprSelf[sym]
	r.funcExprSelf[sym] = goName
	lit, err := r.blockBodyArrow(n, fields)
	if had {
		r.funcExprSelf[sym] = prev
	} else {
		delete(r.funcExprSelf, sym)
	}
	if err != nil {
		return nil, err
	}
	funcLit, ok := lit.(*ast.FuncLit)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a named function expression body did not lower to a closure"}
	}
	funcType := funcLit.Type
	body := []ast.Stmt{
		&ast.DeclStmt{Decl: &ast.GenDecl{Tok: token.VAR, Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ident(goName)}, Type: funcType}}}},
		&ast.AssignStmt{Lhs: []ast.Expr{ident(goName)}, Tok: token.ASSIGN, Rhs: []ast.Expr{funcLit}},
		&ast.ReturnStmt{Results: []ast.Expr{ident(goName)}},
	}
	return &ast.CallExpr{Fun: &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: &ast.FieldList{List: []*ast.Field{{Type: funcType}}}},
		Body: &ast.BlockStmt{List: body},
	}}, nil
}

// funcExprBody returns the block body of a function expression, or the node itself
// when it has no block, so a caller can scan the body for a self-reference.
func (r *Renderer) funcExprBody(n frontend.Node) frontend.Node {
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		return n
	}
	if last := kids[len(kids)-1]; last.Kind() == frontend.NodeBlock {
		return last
	}
	return n
}

// subtreeReferencesSymbol reports whether any identifier in n's subtree resolves to
// sym, so a named function expression can tell whether its body reads its own name.
// Resolving through the symbol, rather than matching the text, keeps a shadowing
// local of the same name from counting as a self-reference.
func (r *Renderer) subtreeReferencesSymbol(n frontend.Node, sym frontend.Symbol) bool {
	if n.Kind() == frontend.NodeIdentifier {
		if s, ok := r.prog.SymbolAt(n); ok && s == sym {
			return true
		}
	}
	for _, c := range r.prog.Children(n) {
		if r.subtreeReferencesSymbol(c, sym) {
			return true
		}
	}
	return false
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
	fields, err := r.closureParamFields(n, "arrow")
	if err != nil {
		return nil, err
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

	// The dynamic-locals set rescopes to this nested body the way the named path
	// scopes it, so an any-typed parameter of a function expression or an arrow
	// binds a tracked box: a read the checker narrowed past a typeof guard unwraps
	// through its accessor, and a method call on the still-boxed binding routes to
	// the runtime dispatch. Without this a nested function saw only the enclosing
	// function's set, so its own parameters went untracked. The nested set merges
	// over the inherited one rather than replacing it, so a captured outer dynamic
	// stays tracked inside the closure.
	var bodyStmts []frontend.Node
	kids := r.prog.Children(n)
	if last := kids[len(kids)-1]; last.Kind() == frontend.NodeBlock {
		bodyStmts = r.prog.Children(last)
	}
	prevDyn := r.dynLocals
	r.dynLocals = mergeNameSets(prevDyn, r.dynLocalsOf(sig.Params, bodyStmts), r.scopeDeclaredNames(sig.Params, bodyStmts))
	defer func() { r.dynLocals = prevDyn }()

	body, err := r.blockOf(n)
	if err != nil {
		return nil, err
	}
	r.endWithImplicitUndefinedReturn(body, bodyStmts, sig.Return)
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{List: fields}, Results: results},
		Body: body,
	}, nil
}

// mergeNameSets overlays a nested body's dynamic set on the inherited one, so a
// closure keeps tracking a captured outer binding while also tracking its own. A
// name the closure redeclares as its own local shadows the outer binding, so the
// shadowed set is subtracted from the inherited names before the inner set is laid
// over: a static local that shares a name with an outer dynamic (a helper's
// `var result` under a top-level `var result`) drops the inherited dynamic bit and
// keeps its own Go type. A name the closure redeclares and itself classifies dynamic
// comes back through inner. A nil inner with no shadows returns the outer unchanged,
// and a nil outer returns the inner, so the common body allocates nothing.
func mergeNameSets(outer, inner, shadowed map[string]bool) map[string]bool {
	if len(inner) == 0 && len(shadowed) == 0 {
		return outer
	}
	if len(outer) == 0 {
		return inner
	}
	merged := make(map[string]bool, len(outer)+len(inner))
	maps.Copy(merged, outer)
	for name := range shadowed {
		delete(merged, name)
	}
	maps.Copy(merged, inner)
	if len(merged) == 0 {
		return nil
	}
	return merged
}
