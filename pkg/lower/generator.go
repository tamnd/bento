package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// A generator function (function* g()) lowers to a Go function that returns a
// running coroutine, the *value.Gen the runtime drives (pkg/value/generator.go).
// The body becomes the goroutine func value.NewGen wraps: each yield in the body
// lowers to a call on the coroutine handle, which suspends the goroutine until the
// consumer pulls again, and the body's fall-off completes the generator with
// undefined. A for...of over the result, or a manual next(), pulls it one value at
// a time through that same Gen. This file builds the function form and the shared
// coroutine body; the class method form reuses the body builder from classes.go.

// isGeneratorFunc reports whether a function declaration carries the generator
// star, the function* marker the parser surfaces as a childless unnamed "*" token
// before the name. It is the same token the class-method scan reads to route a
// method to the coroutine, read here off the declaration's own children.
func (r *Renderer) isGeneratorFunc(fn frontend.Node) bool {
	for _, k := range r.prog.Children(fn) {
		if k.Kind() == frontend.NodeUnknown && len(r.prog.Children(k)) == 0 && r.prog.Text(k) == "*" {
			return true
		}
		// The star sits before the name; once a named child (the function name or a
		// parameter) is reached the marker cannot appear, so stop scanning.
		if k.Kind() == frontend.NodeIdentifier {
			return false
		}
	}
	return false
}

// generatorElemType reports the yielded element type of a generator or iterator
// type, the Y in the *value.Gen[Y] the runtime drives, so a Generator-typed slot
// (the return of a generator function value, a `const it = g()` binding) renders to
// the coroutine pointer rather than expanding structurally into the IteratorResult
// union. The judgment is the type's symbol name, the same built-in family
// isGeneratorIterable keys on, and the element type is the generic's first type
// argument (Generator<T, TReturn, TNext> puts T first). A generator with no readable
// type argument reports false so the caller keeps its existing handling.
func (r *Renderer) generatorElemType(t frontend.Type) (frontend.Type, bool) {
	sym, ok := r.prog.TypeSymbol(t)
	if !ok {
		return frontend.Type{}, false
	}
	switch sym.Name {
	case "Generator", "IterableIterator", "Iterator", "IteratorObject":
	default:
		return frontend.Type{}, false
	}
	args := r.prog.TypeArguments(t)
	if len(args) == 0 || args[0].Flags == 0 {
		return frontend.Type{}, false
	}
	return args[0], true
}

// collectYields gathers every yield expression under n, the nodes whose operands fix
// the generator's element type. It descends the body but not into a nested function,
// whose own yields belong to its own generator.
func (r *Renderer) collectYields(n frontend.Node, out *[]frontend.Node) {
	switch n.Kind() {
	case frontend.NodeFunctionDeclaration, frontend.NodeFunctionExpression, frontend.NodeArrowFunction,
		frontend.NodeMethodDeclaration, frontend.NodeGetAccessor, frontend.NodeSetAccessor, frontend.NodeConstructor:
		return
	case frontend.NodeYieldExpression:
		*out = append(*out, n)
	}
	for _, k := range r.prog.Children(n) {
		r.collectYields(k, out)
	}
}

// generatorYieldType reads the element type a generator body yields, the Y in
// *value.Gen[Y], off its yielded expressions: the common type of every yield, required
// to lower to one Go type so the single channel of Y carries them all. A plain yield
// carries its operand as its one child and contributes that operand's type; a yield*
// delegation carries an extra star child and contributes the delegate's element type,
// since every value the delegate yields flows out through this generator. A valueless
// yield has no operand to read a type off, so a body whose only yields are those keeps
// its own precise reason rather than the generic no-element-type one. A body whose
// yields disagree in Go type hands back rather than pick one.
func (r *Renderer) generatorYieldType(block frontend.Node) (frontend.Type, ast.Expr, error) {
	var yields []frontend.Node
	r.collectYields(block, &yields)
	var elemGo ast.Expr
	var elemType frontend.Type
	sawValueless := false
	for _, y := range yields {
		kids := r.prog.Children(y)
		var t frontend.Type
		switch {
		case len(kids) == 0:
			sawValueless = true
			continue
		case len(kids) > 1:
			// yield* delegates to a sub-iterable, whose element type joins the channel's
			// Y since every value the delegate yields flows out through this generator. A
			// generator delegate reports its Y the same way a Generator-typed slot does;
			// a non-generator iterable has no such element type here and is a later slice.
			elem, ok := r.generatorElemType(r.prog.TypeAt(kids[len(kids)-1]))
			if !ok {
				return frontend.Type{}, nil, &NotYetLowerable{Reason: "a yield* over a non-generator iterable is a later slice"}
			}
			t = elem
		default:
			t = r.prog.TypeAt(kids[0])
		}
		g, err := r.typeExpr(t)
		if err != nil {
			return frontend.Type{}, nil, err
		}
		if elemGo == nil {
			elemGo, elemType = g, t
			continue
		}
		if same, err := sameGoType(elemGo, g); err != nil || !same {
			return frontend.Type{}, nil, &NotYetLowerable{Reason: "a generator whose yields differ in type is a later slice"}
		}
	}
	if elemGo == nil {
		if sawValueless {
			return frontend.Type{}, nil, &NotYetLowerable{Reason: "a valueless yield is a later slice"}
		}
		return frontend.Type{}, nil, &NotYetLowerable{Reason: "a generator with no yielded value has no element type here, a later slice"}
	}
	return elemType, elemGo, nil
}

// coerceToType bridges a value from a source node's type to a target type across the
// dynamic boundary, the same static/dynamic coercion coerceToTarget applies but
// against a type the caller holds rather than a target node. A generator yields
// through it: the yielded value is coerced to the channel's element type Y, which is
// the value's own type in the common case and so passes through unchanged.
func (r *Renderer) coerceToType(expr ast.Expr, src frontend.Node, target frontend.Type) (ast.Expr, error) {
	if boxed, ok, err := r.boxToOptional(expr, src, target); err != nil {
		return nil, err
	} else if ok {
		return boxed, nil
	}
	if wrapped, ok, err := r.wrapToUnion(expr, src, target); err != nil {
		return nil, err
	} else if ok {
		return wrapped, nil
	}
	if err := r.guardOptionalShapeCrossTypes(r.prog.TypeAt(src), target); err != nil {
		return nil, err
	}
	srcDyn := r.isDynamic(src)
	tgtDyn := target.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0
	switch {
	case srcDyn && !tgtDyn:
		return r.coerceDynamicToStaticFlags(expr, target.Flags)
	case !srcDyn && tgtDyn:
		return r.boxStaticToDynamic(expr, src)
	default:
		return r.bridgeClassBinding(expr, src, target)
	}
}

// yieldExpr lowers a yield expression to a call on the current generator's coroutine
// handle. A plain yield lowers to _co.Yield(v), which sends v to the consumer and
// blocks until the next pull; a yield* delegation lowers to _co.YieldFrom(sub), which
// drives the delegate generator and forwards its values. Either way the call evaluates
// to the value the yield expression takes on: the value the consumer passed back
// through next(v) for a plain yield, the delegate's return value for a yield*. A yield
// outside a lowered generator body has no handle to send on and hands back; a valueless
// yield is a later slice.
func (r *Renderer) yieldExpr(n frontend.Node) (ast.Expr, error) {
	if r.genCo == "" {
		return nil, &NotYetLowerable{Reason: "a yield outside a lowered generator body is a later slice"}
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		return nil, &NotYetLowerable{Reason: "a valueless yield is a later slice"}
	}
	var call ast.Expr
	if len(kids) > 1 {
		c, err := r.yieldStar(kids[len(kids)-1])
		if err != nil {
			return nil, err
		}
		call = c
	} else {
		val, err := r.lowerExpr(kids[0])
		if err != nil {
			return nil, err
		}
		val, err = r.coerceToType(val, kids[0], r.genYieldType)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		call = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(r.genCo), Sel: ident("Yield")}, Args: []ast.Expr{val}}
	}
	// The call hands its result back as a dynamic value.Value: the argument the consumer
	// passed to next(v) for a plain yield, or the delegate's completion value for a
	// yield*. The yield expression reads as the generator's TNext type (a plain yield) or
	// the delegate's TReturn type (a yield*), so when that type is a concrete primitive
	// the dynamic result coerces to it through the ToNumber family, the same crossing an
	// assignment applies. A type that is itself dynamic, the unknown a plain Generator<Y>
	// carries, needs no coercion and the value.Value passes through as the yield's value.
	result := r.prog.TypeAt(n)
	if result.Flags != 0 && result.Flags&(frontend.TypeAny|frontend.TypeUnknown|frontend.TypeVoid) == 0 {
		return r.coerceDynamicToStaticFlags(call, result.Flags)
	}
	return call, nil
}

// yieldStar lowers a yield* delegation to a drive of the delegate generator on the
// current coroutine: _co.YieldFrom(sub) pulls every value the delegate yields, re-yields
// it to this generator's consumer, threads the value the consumer sends back into the
// delegate's own next, and evaluates to the value the delegate completed with. The
// element types already agree by generatorYieldType, so the forwarded values need no
// coercion. Only a generator delegate is lowerable; a non-generator iterable is a later
// slice, the same reason generatorYieldType reports for the element type.
func (r *Renderer) yieldStar(delegate frontend.Node) (ast.Expr, error) {
	if _, ok := r.generatorElemType(r.prog.TypeAt(delegate)); !ok {
		return nil, &NotYetLowerable{Reason: "a yield* over a non-generator iterable is a later slice"}
	}
	sub, err := r.lowerExpr(delegate)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(r.genCo), Sel: ident("YieldFrom")}, Args: []ast.Expr{sub}}, nil
}

// generatorCoroutine builds the value.NewGen[Y](func(_co *value.GenCo[Y]) value.Value
// { ... }) expression a generator body lowers to, and returns the Go element type Y
// alongside it so the caller can spell the *value.Gen[Y] the enclosing function or
// method returns. The body lowers with the coroutine handle name and the element type
// in scope, so a yield inside it routes to yieldExpr and a return boxes to
// value.Value; the body's fall-off appends `return value.Undefined`, the completion a
// generator that runs off its end reports.
func (r *Renderer) generatorCoroutine(fn frontend.Node) (yieldGo ast.Expr, newGen ast.Expr, err error) {
	block, ok := r.funcBodyBlock(fn)
	if !ok {
		return nil, nil, &NotYetLowerable{Reason: "a generator without a body is a later slice"}
	}
	elemType, elemGo, err := r.generatorYieldType(block)
	if err != nil {
		return nil, nil, err
	}

	// The body-scoped analyses are set up the way funcDeclNamed sets them, off both the
	// signature parameters and the body declarations, so a union-typed or any-typed
	// parameter of the generator is tracked inside the body the same as in a plain
	// function. They are saved and restored so the coroutine's scope does not leak out.
	var params []frontend.Param
	if sig, ok := r.prog.SignatureAt(fn); ok {
		params = sig.Params
	}
	bodyStmts := r.prog.Children(block)
	prevUnion := r.unionLocals
	r.unionLocals = r.unionLocalsOf(params, bodyStmts)
	defer func() { r.unionLocals = prevUnion }()
	prevDyn := r.dynLocals
	r.dynLocals = r.dynLocalsOf(params, bodyStmts)
	defer func() { r.dynLocals = prevDyn }()

	coName := r.freshTemp()
	prevCo, prevYT := r.genCo, r.genYieldType
	r.genCo, r.genYieldType = coName, elemType
	defer func() { r.genCo, r.genYieldType = prevCo, prevYT }()

	body, err := r.blockOf(fn)
	if err != nil {
		return nil, nil, err
	}
	r.requireImport(valuePkg)
	// A generator that runs off its end completes with undefined, so the coroutine func
	// returns value.Undefined after the body, the value a { value, done: true } result
	// carries for a return-less generator. A body that already ends in a return
	// completes with that value, so the fall-off return would be dead code and is left off.
	if n := len(body.List); n == 0 || !isGoReturn(body.List[n-1]) {
		body.List = append(body.List, &ast.ReturnStmt{Results: []ast.Expr{sel("value", "Undefined")}})
	}

	coParam := &ast.Field{
		Names: []*ast.Ident{ident(coName)},
		Type:  star(index(sel("value", "GenCo"), elemGo)),
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{coParam}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "Value")}}},
		},
		Body: body,
	}
	newGen = &ast.CallExpr{Fun: index(sel("value", "NewGen"), elemGo), Args: []ast.Expr{lit}}
	// The element type is spelled fresh for the caller so the returned *value.Gen[Y]
	// does not share the node the NewGen call already holds.
	yieldGo, err = r.typeExpr(elemType)
	if err != nil {
		return nil, nil, err
	}
	return yieldGo, newGen, nil
}

// generatorFuncDecl lowers a top-level generator function to a Go function that
// returns a running coroutine: g(params) *value.Gen[Y] { return value.NewGen[Y](...) }.
// The body is the coroutine func generatorCoroutine builds, so the function's only
// statement hands the caller the *value.Gen the for...of or the manual next() drives.
// A generic generator or one with a rest parameter is a later slice.
func (r *Renderer) generatorFuncDecl(fn frontend.Node, sig frontend.Signature, name string) (*ast.FuncDecl, error) {
	if len(sig.TypeParams) != 0 {
		return nil, &NotYetLowerable{Reason: "a generic generator needs monomorphization, a later slice"}
	}
	if sig.RestParam != nil {
		return nil, &NotYetLowerable{Reason: "a generator with a rest parameter is a later slice"}
	}
	params, err := r.funcParamFields(fn, sig)
	if err != nil {
		return nil, err
	}
	yieldGo, newGen, err := r.generatorCoroutine(fn)
	if err != nil {
		return nil, err
	}
	result := &ast.Field{Type: star(index(sel("value", "Gen"), yieldGo))}
	return &ast.FuncDecl{
		Name: ident(name),
		Type: &ast.FuncType{Params: params, Results: &ast.FieldList{List: []*ast.Field{result}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{newGen}}}},
	}, nil
}

// generatorFuncExpr lowers a generator function expression used as a value to a
// closure that returns the running coroutine: func(params) *value.Gen[Y] { return
// value.NewGen[Y](...) }. It is the expression form of generatorFuncDecl, the shape a
// const bound to a function* takes, and shares the same coroutine body builder.
func (r *Renderer) generatorFuncExpr(n frontend.Node, fields []*ast.Field) (ast.Expr, error) {
	yieldGo, newGen, err := r.generatorCoroutine(n)
	if err != nil {
		return nil, err
	}
	result := &ast.Field{Type: star(index(sel("value", "Gen"), yieldGo))}
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{List: fields}, Results: &ast.FieldList{List: []*ast.Field{result}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{newGen}}}},
	}, nil
}
