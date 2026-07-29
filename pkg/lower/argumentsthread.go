package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file threads the real call-site arguments into a function that reads its
// arguments object. The snapshot arguments.go materializes from the parameters
// (section on argumentsPlan) only equals the passed arguments when every call
// passes exactly one argument per parameter, so a loose-arity call, an optional or
// rest parameter, all hand back there. Node and V8 give arguments the arguments
// actually passed at the call site regardless of the declared parameter count, so
// this reproduces that faithfully: a function whose body reads arguments takes a
// hidden trailing *value.Array[value.Value] parameter carrying the real arguments,
// and every call site builds that array from the actual argument expressions.
//
// The mechanism is sound only when the declaration and every call site agree on the
// hidden parameter, so it is confined to a plain top-level function declaration all
// of whose references are direct calls this pass rewrites: a value use, a .call or
// .apply, a boxing into a dynamic slot, a namespace member access, any of these
// leaves a reference the call-site pass would not append the array to, so the
// function keeps the snapshot model (or hands back) rather than take the hidden
// parameter. The decision is a pure function of the symbol so the declaration and
// each call site reach the same answer; funcSymThreadsArgs memoizes it.

// funcSymThreadsArgs reports whether the top-level function a symbol names threads
// the real call-site arguments through a hidden trailing parameter. It is the single
// predicate the declaration consults to add the parameter and every call site
// consults to pass the array, so both always agree. The answer is memoized because
// it walks the whole program to prove every reference is a direct call.
func (r *Renderer) funcSymThreadsArgs(sym frontend.Symbol) bool {
	if v, ok := r.argsThreads[sym]; ok {
		return v
	}
	result := r.computeThreadsArgs(sym)
	r.argsThreads[sym] = result
	return result
}

func (r *Renderer) computeThreadsArgs(sym frontend.Symbol) bool {
	if sym.Flags&frontend.SymbolFunction == 0 {
		return false
	}
	// An overloaded function lowers its all-dynamic implementation over value.Value
	// parameters, a convention the positional hidden parameter does not fit, so it is
	// left to its own boxed call path.
	if _, ok := r.overloadedFuncImpl(sym); ok {
		return false
	}
	// A function-valued binding (const f = function(){...}) or a two-step assignment
	// resolves to more than the single top-level declaration this pass rewrites, so
	// only a symbol with exactly one function-declaration node behind it is threaded;
	// every other shape keeps the snapshot model.
	nodes := r.symFuncNodes(sym)
	if len(nodes) != 1 {
		return false
	}
	fn := nodes[0]
	sig, ok := r.prog.SignatureAt(fn)
	if !ok {
		return false
	}
	if !r.funcNodeThreadsArgs(fn, sig) {
		return false
	}
	clean, anyLoose := r.funcSymCallShape(sym, len(sig.Params))
	if !clean {
		return false
	}
	// Threading only earns its keep when the snapshot could not stand in: a call whose
	// argument count differs from the parameter count, or a signature with an optional,
	// defaulted, or rest parameter that lets a call vary the arity. An all-required,
	// rest-free function every call hits exactly keeps the simpler snapshot, so its
	// existing lowering and the tests that pin it are untouched.
	return anyLoose || sig.MinArgs != len(sig.Params) || sig.RestParam != nil
}

// funcNodeThreadsArgs reports whether one function-declaration node is the shape the
// hidden-parameter threading backs: a plain, synchronous, non-generic top-level
// function whose body reads arguments in a supported way and does not write a named
// parameter. An async or generator body lowers through its own coroutine path, a
// generic one monomorphizes to several Go names, and a callable object collides with
// its interned struct, so none reach the funcDeclNamed path that adds the parameter.
// A body that writes a named parameter keeps the mapped-arguments handback: in sloppy
// mode arguments[i] would track that parameter, an aliasing the unmapped hidden array
// does not mirror, so it stays a later slice rather than emit a store that diverges.
func (r *Renderer) funcNodeThreadsArgs(fn frontend.Node, sig frontend.Signature) bool {
	if fn.Kind() != frontend.NodeFunctionDeclaration {
		return false
	}
	if len(sig.TypeParams) != 0 {
		return false
	}
	if r.isAsyncFunc(fn) || r.isGeneratorFunc(fn) {
		return false
	}
	if r.isCallableObject(r.prog.TypeAt(fn)) {
		return false
	}
	block, ok := r.funcBodyBlock(fn)
	if !ok {
		return false
	}
	reads, supported, indexed := false, true, false
	for _, stmt := range r.prog.Children(block) {
		r.scanArguments(stmt, &reads, &supported, &indexed)
	}
	if !reads || !supported {
		return false
	}
	// A write to a named parameter alongside an element read stays a later slice, the
	// same mapped-arguments handback the snapshot plan keeps; a length-only read does
	// not observe the aliasing, so the write does not bar threading.
	if indexed && r.bodyWritesParam(block, sig.Params) {
		return false
	}
	// A default that reads an earlier parameter collapses the optional tail into one Go
	// variadic filled in the body, a convention the fixed hidden parameter does not sit
	// beside, so such a function keeps its own path rather than thread.
	if plan, err := r.variadicDefaultPlan(fn, sig); err != nil || plan != nil {
		return false
	}
	return true
}

// funcExprBoxThreadsArgs reports whether a function expression boxed directly into a
// dynamic value threads its real call-site arguments through a hidden trailing
// parameter rather than the parameter snapshot. It is the expression analogue of
// funcNodeThreadsArgs, gated to the shape the boxed wrapper already fills: an
// all-required, rest-free, non-generic, synchronous function whose body reads
// arguments in a supported way and does not write a named parameter alongside an
// element read. Unlike the declaration form it needs no whole-program reference
// walk: a directly boxed function expression has exactly one reference, the boxing
// site, so the wrapper is the sole caller and always passes the hidden array. A rest
// or optional parameter, an async or generator body, a generic signature, or a
// parameter write beside an element read keeps the existing handback.
func (r *Renderer) funcExprBoxThreadsArgs(n frontend.Node) bool {
	if n.Kind() != frontend.NodeFunctionExpression {
		return false
	}
	sig, ok := r.prog.SignatureAt(n)
	if !ok {
		return false
	}
	// The boxed wrapper coerces one argument per declared parameter, so only an
	// all-required, rest-free signature fits it. A rest or optional parameter stays on
	// the wrapper's own rest and optional handbacks.
	if sig.RestParam != nil || sig.MinArgs != len(sig.Params) {
		return false
	}
	if len(sig.TypeParams) != 0 {
		return false
	}
	if r.isAsyncFunc(n) || r.isGeneratorFunc(n) {
		return false
	}
	if r.isCallableObject(r.prog.TypeAt(n)) {
		return false
	}
	block, ok := r.funcBodyBlock(n)
	if !ok {
		return false
	}
	reads, supported, indexed := false, true, false
	for _, stmt := range r.prog.Children(block) {
		r.scanArguments(stmt, &reads, &supported, &indexed)
	}
	if !reads || !supported {
		return false
	}
	// A write to a named parameter alongside an element read is the mapped-arguments
	// aliasing the unmapped hidden array does not mirror, the same handback the
	// snapshot plan keeps.
	if indexed && r.bodyWritesParam(block, sig.Params) {
		return false
	}
	return true
}

// boxNamedFuncReadingArgs boxes a named top-level function that reads its arguments
// object and flows into a dynamic slot by reference (const g: any = f) into a shared
// module-level value.Value. It routes only when src is an identifier resolving to a
// single top-level function declaration whose shape the hidden-parameter threading
// backs (funcNodeThreadsArgs) and whose signature the boxed wrapper can bridge
// (all-required, rest-free). ok is false for every other shape, so a binding-held
// function, a rest or optional parameter, or an unsupported body falls through to the
// existing handback. Once it commits to the boxed shape, a wrapper build that hands
// back is propagated as the error, not swallowed. It uses the declaration's own
// signature, not the reference site's, so the wrapper coerces one argument per declared
// parameter.
func (r *Renderer) boxNamedFuncReadingArgs(src frontend.Node) (ast.Expr, bool, error) {
	if src.Kind() != frontend.NodeIdentifier {
		return nil, false, nil
	}
	sym, ok := r.prog.SymbolAt(src)
	if !ok {
		return nil, false, nil
	}
	sym = r.derefAlias(sym)
	nodes := r.symFuncNodes(sym)
	if len(nodes) != 1 {
		return nil, false, nil
	}
	decl := nodes[0]
	if decl.Kind() != frontend.NodeFunctionDeclaration {
		return nil, false, nil
	}
	sig, ok := r.prog.SignatureAt(decl)
	if !ok {
		return nil, false, nil
	}
	if !r.funcNodeThreadsArgs(decl, sig) {
		return nil, false, nil
	}
	// The boxed wrapper coerces exactly one argument per declared parameter, so it fits
	// only an all-required, rest-free signature, the same constraint boxFuncToDynamic
	// enforces. A rest or optional parameter keeps the existing handback.
	if sig.RestParam != nil || sig.MinArgs != len(sig.Params) {
		return nil, false, nil
	}
	boxed, err := r.boxedNamedArgFunc(sym, decl, sig)
	if err != nil {
		return nil, false, err
	}
	return boxed, true, nil
}

// boxedNamedArgFunc returns a reference to the one module-level value.Value a named
// function declaration is boxed into, generating that var the first time the function
// is boxed and memoizing it by symbol so every later box of the same function reads the
// same var. Sharing the var is what preserves JavaScript reference identity: g === h
// when both bind f, because both boxes lower to the same package-level name rather than
// to two fresh wrapper closures.
//
// The wrapper inlines the declaration's body (blockBodyArrow with boxThreadArgs set) and
// does not call the named Go function, so the declaration's own lowering and its
// direct-call and value-use paths are untouched; a recursive call inside the inlined
// body still lowers to the package function. The name is reserved and the memo entry
// set before the body lowers, so a self-box inside the body resolves to the same var
// rather than recursing. A wrapper build that hands back rolls the reservation and the
// memo entry back, so the failure stays a clean handback that a later lowerable use can
// retry, and never memoizes a half-built var.
func (r *Renderer) boxedNamedArgFunc(sym frontend.Symbol, decl frontend.Node, sig frontend.Signature) (ast.Expr, error) {
	if name, ok := r.boxedArgFuncs[sym]; ok {
		return ident(name), nil
	}
	base, ok := exportedField(sym.Name)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a named function boxed into a dynamic value whose name is not a Go identifier is a later slice"}
	}
	name := r.decls.reserveName(base + "__argbox")
	r.boxedArgFuncs[sym] = name
	wrapper, err := r.namedArgFuncWrapper(decl, sig)
	if err != nil {
		delete(r.boxedArgFuncs, sym)
		r.decls.releaseName(name)
		return nil, err
	}
	if err := r.decls.addVar(name, wrapper); err != nil {
		delete(r.boxedArgFuncs, sym)
		r.decls.releaseName(name)
		return nil, err
	}
	return ident(name), nil
}

// namedArgFuncWrapper builds the value.NewFunc closure a boxed named function is
// materialized as: it lowers the declaration's parameter fields, marks the declaration
// so its body threads the real call-site arguments through a hidden trailing parameter
// (blockBodyArrow reads the mark), inlines the body into a Go closure, and wraps that in
// the boxing thunk boxFuncToDynamic builds, which fills the hidden parameter with the
// arguments the dynamic call passed. The mark is cleared on every exit: boxFuncToDynamic
// clears it on the success path, and this deletes it defensively so a handback anywhere
// in the build leaves no stale mark on the declaration node.
func (r *Renderer) namedArgFuncWrapper(decl frontend.Node, sig frontend.Signature) (ast.Expr, error) {
	fields, err := r.closureParamFields(decl, sig, "function")
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	r.boxThreadArgs[decl] = true
	inner, err := r.blockBodyArrow(decl, fields)
	if err != nil {
		delete(r.boxThreadArgs, decl)
		return nil, err
	}
	wrapper, err := r.boxFuncToDynamic(inner, sig, decl)
	if err != nil {
		delete(r.boxThreadArgs, decl)
		return nil, err
	}
	return wrapper, nil
}

// funcSymCallShape walks the whole program and reports whether every reference to a
// function symbol is a direct call this pass can rewrite (clean), and whether any of
// those calls passes an argument count other than the parameter count (anyLoose).
// A reference in any other position, a value use, a member access such as f.call, a
// boxing, leaves clean false, so the function is not threaded and its arguments read
// keeps the snapshot model. A direct call whose arguments are not all repeatable
// (a literal or a plain identifier) or that spreads also leaves clean false: the
// hidden array re-evaluates each argument that also binds a parameter, so an argument
// with a side effect or an unenumerable spread cannot be threaded soundly and the
// whole function stays on its existing path.
func (r *Renderer) funcSymCallShape(sym frontend.Symbol, paramCount int) (clean bool, anyLoose bool) {
	clean = true
	decls := make(map[frontend.Node]bool)
	for _, d := range r.prog.Declarations(sym) {
		decls[d] = true
	}
	refersToSym := func(n frontend.Node) bool {
		if n.Kind() != frontend.NodeIdentifier {
			return false
		}
		s, ok := r.prog.SymbolAt(n)
		return ok && r.derefAlias(s) == sym
	}
	var walk func(n frontend.Node, isCallee bool)
	walk = func(n frontend.Node, isCallee bool) {
		if !clean {
			return
		}
		if n.Kind() == frontend.NodeIdentifier {
			if refersToSym(n) && !isCallee {
				clean = false
			}
			return
		}
		kids := r.prog.Children(n)
		if n.Kind() == frontend.NodeCallExpression && len(kids) >= 1 && refersToSym(kids[0]) {
			// A direct call of the symbol: the callee identifier is allowed, and the
			// arguments are inspected for the count and for a shape the hidden array can
			// carry. Each argument is still walked as a value so a function passed as its
			// own argument is caught as a value use.
			argNodes := kids[1:]
			for _, a := range argNodes {
				if a.Kind() == frontend.NodeSpreadElement || !r.isRepeatableArg(a) {
					clean = false
					return
				}
			}
			if len(argNodes) != paramCount {
				anyLoose = true
			}
			for _, a := range argNodes {
				walk(a, false)
			}
			return
		}
		isDecl := decls[n]
		for _, c := range kids {
			// The name a declaration binds is the symbol's definition, not a use, so it is
			// skipped: it would otherwise read as a non-call reference and block threading.
			if isDecl && refersToSym(c) {
				continue
			}
			walk(c, false)
		}
	}
	for _, f := range r.prog.SourceFiles() {
		walk(f, false)
	}
	return clean, anyLoose
}

// isRepeatableArg reports whether an argument expression may be evaluated more than
// once with no observable difference, the property the hidden arguments array needs
// for any argument that also binds a parameter: the parameter slot and the array
// both read it. A literal and a plain identifier read run no side effect and cannot
// throw, so either repeats cleanly; every other expression (a call, a property read
// that could trip a getter, an assignment) is not repeated and leaves the function on
// its existing path rather than risk running an effect twice.
func (r *Renderer) isRepeatableArg(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeNumericLiteral,
		frontend.NodeStringLiteral,
		frontend.NodeBigIntLiteral,
		frontend.NodeTrueKeyword,
		frontend.NodeFalseKeyword,
		frontend.NodeNullKeyword,
		frontend.NodeNoSubstitutionTemplateLiteral,
		frontend.NodeIdentifier:
		return true
	default:
		return false
	}
}

// hiddenArgsArray builds the *value.Array[value.Value] a threaded call passes as its
// hidden trailing argument, holding every actual argument in source order each boxed
// to value.Value. Missing arguments simply are not in the array, so arguments.length
// reads the real call count; an argument past the parameter count is here even though
// no parameter binds it, which is exactly what arguments[i] must see.
func (r *Renderer) hiddenArgsArray(argNodes []frontend.Node) (ast.Expr, error) {
	r.requireImport(valuePkg)
	elems := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		boxed, err := r.boxArgToValue(a)
		if err != nil {
			return nil, err
		}
		elems = append(elems, boxed)
	}
	return &ast.CallExpr{
		Fun:  index(sel("value", "NewArray"), sel("value", "Value")),
		Args: elems,
	}, nil
}

// boxArgToValue lowers one argument expression to a value.Value for the hidden
// arguments array, the same boxing a static value crossing into any takes: a literal
// boxes straight from its node, and every other expression lowers and bridges to the
// dynamic any slot.
func (r *Renderer) boxArgToValue(a frontend.Node) (ast.Expr, error) {
	if boxed, ok, err := r.boxLiteralToDynamic(a); err != nil {
		return nil, err
	} else if ok {
		return boxed, nil
	}
	// A value the dynamic paths already hold as a value.Value box (an any binding, a
	// primitive wrapper object, a boxed member read) needs no further boxing: its lowered
	// form is the box. Routing it through bridgeArg against an any target would land in the
	// bridge's both-sides-dynamic gap and hand back on a spurious representation mismatch,
	// so it lowers straight through here instead.
	if r.isDynamic(a) {
		return r.lowerExpr(a)
	}
	lowered, err := r.lowerExpr(a)
	if err != nil {
		return nil, err
	}
	return r.bridgeArg(lowered, a, frontend.Type{Flags: frontend.TypeAny})
}

// hiddenArgsField is the Go field a threaded function declaration takes as its hidden
// trailing parameter, __args__ *value.Array[value.Value], the store the body reads
// arguments through in place of the entry snapshot.
func hiddenArgsField(name string) *ast.Field {
	return &ast.Field{
		Names: []*ast.Ident{ident(name)},
		Type:  &ast.StarExpr{X: index(sel("value", "Array"), sel("value", "Value"))},
	}
}
