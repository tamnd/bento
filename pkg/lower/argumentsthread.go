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
	reads, supported := false, true
	for _, stmt := range r.prog.Children(block) {
		r.scanArguments(stmt, &reads, &supported)
	}
	if !reads || !supported {
		return false
	}
	if r.bodyWritesParam(block, sig.Params) {
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
