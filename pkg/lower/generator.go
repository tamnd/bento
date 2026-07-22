package lower

import (
	"go/ast"
	"go/token"

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

// generatorToSliceExpr drains a generator into a Go slice of its yielded element
// type, the form a spread needs in expression position. It is the pull-until-done
// walk forOfGenerator emits, wrapped in a func literal that returns the collected
// slice so it stands where a value is expected: a spread splices the slice. The
// generator is obtained inside the literal, so each spread of the same source gets
// its own fresh coroutine, the protocol's rule. Next takes value.Undefined as the
// sent value, since a spread never feeds a value back in the way a hand-written
// next(v) can. The element type is the caller's, already checked to match the
// generator's yield type, so the appended values need no conversion.
func (r *Renderer) generatorToSliceExpr(src, elemType ast.Expr) ast.Expr {
	r.requireImport(valuePkg)
	sliceName := r.freshTemp()
	genName := r.freshTemp()
	valName := r.freshTemp()
	doneName := r.freshTemp()
	decl := &ast.DeclStmt{Decl: &ast.GenDecl{
		Tok:   token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ident(sliceName)}, Type: &ast.ArrayType{Elt: elemType}}},
	}}
	getGen := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(genName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{src},
	}
	pull := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(valName), ident(doneName)},
		Tok: token.DEFINE,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ident(genName), Sel: ident("Next")},
			Args: []ast.Expr{sel("value", "Undefined")},
		}},
	}
	brk := &ast.IfStmt{Cond: ident(doneName), Body: &ast.BlockStmt{List: []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}}}
	grow := &ast.AssignStmt{
		Lhs: []ast.Expr{ident(sliceName)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{&ast.CallExpr{Fun: ident("append"), Args: []ast.Expr{ident(sliceName), ident(valName)}}},
	}
	loop := &ast.ForStmt{Body: &ast.BlockStmt{List: []ast.Stmt{pull, brk, grow}}}
	ret := &ast.ReturnStmt{Results: []ast.Expr{ident(sliceName)}}
	fn := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: &ast.ArrayType{Elt: elemType}}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{decl, getGen, loop, ret}},
	}
	return &ast.CallExpr{Fun: fn}
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
func (r *Renderer) generatorYieldType(block frontend.Node, declElem frontend.Type, declOK bool) (frontend.Type, ast.Expr, error) {
	var yields []frontend.Node
	r.collectYields(block, &yields)
	var elemGo ast.Expr
	var elemType frontend.Type
	for _, y := range yields {
		kids := r.prog.Children(y)
		var t frontend.Type
		switch {
		case len(kids) == 0:
			continue
		case len(kids) > 1:
			// yield* delegates to a sub-iterable, whose element type joins the channel's
			// Y since every value the delegate yields flows out through this generator. A
			// generator delegate reports its Y the same way a Generator-typed slot does;
			// inside an async generator the delegate is an async generator, whose element
			// type reads off its own AsyncGenerator symbol. A delegate that is neither has
			// no such element type here and is a later slice.
			delegateT := r.prog.TypeAt(kids[len(kids)-1])
			elem, ok := r.generatorElemType(delegateT)
			if !ok && r.inAsyncGen {
				elem, ok = r.asyncGeneratorElemType(delegateT)
			}
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
		// A generator whose body yields no typed value, either empty (function* g() {})
		// or with only valueless yields (yield;), takes its element type from its declared
		// return annotation Generator<T>, the same type the consumer reads it through: a
		// for...of binding or a .next() mapper is typed off that declared T, so decl-side
		// Y and consumption-side T must agree or the emitted Go will not compile. An empty
		// Generator<number> is a Gen[float64] the consumer maps with func(float64); a
		// valueless Generator<undefined> is a Gen[value.Value] the consumer maps with
		// func(value.Value), and value.Undefined the valueless yield sends is a value.Value.
		if declOK {
			g, err := r.typeExpr(declElem)
			if err != nil {
				return frontend.Type{}, nil, err
			}
			return declElem, g, nil
		}
		// With no readable declared element (an untyped generator), the element type is the
		// dynamic value.Value box: value.Undefined the valueless yield sends is a value.Value,
		// and an empty generator yields nothing at all. yieldExpr reads this element type to
		// admit the valueless yield. A generator that mixes a valueless and a typed yield
		// keeps the concrete elemGo the typed yield set above, so its Y stays that Go type
		// and its valueless yield stays on the handback rather than send an undefined into a
		// typed channel.
		dyn := frontend.Type{Flags: frontend.TypeAny}
		g, err := r.typeExpr(dyn)
		if err != nil {
			return frontend.Type{}, nil, err
		}
		return dyn, g, nil
	}
	return elemType, elemGo, nil
}

// isIteratorResult reports whether t is the IteratorResult union a generator's next,
// return, and throw hand back: a union whose every member is an object carrying both a
// `done` and a `value` property, the { value, done } shape the language reads .done and
// .value off. The checker gives the union no symbol, so it is recognized by shape
// rather than by name, which also means a hand-written type of the same shape reads the
// same way. This is the union the general tagged-sum path declines, because its
// discriminant `done` is a boolean literal rather than a string one; recognizing it
// here routes it to the value.IterResult struct instead.
func (r *Renderer) isIteratorResult(t frontend.Type) bool {
	if t.Flags&frontend.TypeUnion == 0 {
		return false
	}
	members := r.prog.UnionMembers(t)
	if len(members) < 2 {
		return false
	}
	for _, m := range members {
		if m.Flags&frontend.TypeObject == 0 {
			return false
		}
		hasValue, hasDone := false, false
		for _, p := range r.prog.Properties(m) {
			switch p.Name {
			case "value":
				hasValue = true
			case "done":
				hasDone = true
			}
		}
		if !hasValue || !hasDone {
			return false
		}
	}
	return true
}

// isIterResultReceiver reports whether obj holds a value.IterResult, so a .value or
// .done read off it routes to the struct fields. It matches the IteratorResult union
// directly, and also the declared type of a binding whose narrowed type collapsed to a
// single member: inside a `while (!r.done)` the checker narrows r to the yield branch,
// a lone object, but the Go variable is still the value.IterResult its declared union
// mapped to, so the read must reach the same fields regardless of the narrowing.
func (r *Renderer) isIterResultReceiver(obj frontend.Node) bool {
	if r.isIteratorResult(r.prog.TypeAt(obj)) {
		return true
	}
	if obj.Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(obj)
	if !ok {
		return false
	}
	return r.isIteratorResult(r.prog.TypeOfSymbol(sym))
}

// generatorMethodCall lowers a manual drive of a generator, it.next(v), it.return(v),
// or it.throw(e), to the runtime helper that packs the { value, done } result the
// language hands a hand-rolled caller. The receiver is the *value.Gen[Y] the generator
// produced; the helper pulls one step and returns a value.IterResult the caller reads
// .value and .done off. next(v) and return(v) box their argument into the generator's
// dynamic in-channel, throw(e) converts its argument to the runtime's Thrown surface the
// same way a throw statement does, and a boxer closure lifts the yielded Y back into a
// value.Value, since the helpers are generic over Y and cannot know the element type's
// constructor themselves. A receiver that is not a generator returns ok false so the
// caller keeps looking; a method other than next, return, or throw is a later slice.
func (r *Renderer) generatorMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, bool, error) {
	elem, ok := r.generatorElemType(r.prog.TypeAt(recvNode))
	if !ok {
		return nil, false, nil
	}
	switch method {
	case "next", "return", "throw":
	default:
		return nil, false, &NotYetLowerable{Reason: "a generator's ." + method + "() is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, false, err
	}
	boxer, err := r.genElemBoxer(elem)
	if err != nil {
		return nil, false, err
	}
	r.requireImport(valuePkg)
	switch method {
	case "next":
		// next(v) resumes the suspended yield with v; next() with no argument resumes
		// with undefined, the value the language passes when the caller sends nothing.
		sent, err := r.genSentArg(argNodes)
		if err != nil {
			return nil, false, err
		}
		return &ast.CallExpr{Fun: sel("value", "GenNext"), Args: []ast.Expr{recv, sent, boxer}}, true, nil
	case "return":
		// return(v) closes the generator early, completing it with v so the suspended
		// body unwinds through its finally blocks; return() completes with undefined.
		ret, err := r.genSentArg(argNodes)
		if err != nil {
			return nil, false, err
		}
		return &ast.CallExpr{Fun: sel("value", "GenReturn"), Args: []ast.Expr{recv, ret, boxer}}, true, nil
	default: // "throw"
		// throw(e) raises e at the suspended yield: a try/catch in the body catches it
		// and the generator yields again or completes, an uncaught throw escapes. There
		// is nothing to raise without an argument, so throw() hands back.
		if len(argNodes) == 0 {
			return nil, false, &NotYetLowerable{Reason: "a generator's throw() with no argument is a later slice"}
		}
		thrown, err := r.thrownOperand(argNodes[0])
		if err != nil {
			return nil, false, err
		}
		return &ast.CallExpr{Fun: sel("value", "GenThrow"), Args: []ast.Expr{recv, thrown, boxer}}, true, nil
	}
}

// genSentArg boxes the argument a manual next(v) or return(v) passes into the
// generator's dynamic in-channel, defaulting to undefined when the caller passes
// nothing, the value the language sends for a bare next() or return().
func (r *Renderer) genSentArg(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) == 0 {
		return sel("value", "Undefined"), nil
	}
	return r.boxOperand(argNodes[0])
}

// genElemBoxer builds the func(Y) value.Value closure a generator drive passes to
// value.GenNext, which lifts the generator's typed yield into the dynamic value the
// IteratorResult carries. The closure boxes its argument with the same primitive
// constructor a static-to-dynamic crossing uses, so a Generator<number> yields
// value.Number(y). A yield type with no dynamic boxing (an object or array element) is
// a later slice, the same boundary boxStaticToDynamicFlags draws.
func (r *Renderer) genElemBoxer(elem frontend.Type) (ast.Expr, error) {
	elemGo, err := r.typeExpr(elem)
	if err != nil {
		return nil, err
	}
	param := r.freshTemp()
	body, err := r.boxStaticToDynamicFlags(ident(param), elem.Flags)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident(param)}, Type: elemGo}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "Value")}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{body}}}},
	}, nil
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
	if r.tupleShapeMismatch(r.prog.TypeAt(src), target) {
		return nil, &NotYetLowerable{Reason: "a tuple with an optional element built at a different arity or shape than the slot declares is a later slice"}
	}
	srcDyn := r.isDynamic(src)
	tgtDyn := target.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 || r.isNarrowableBoxType(target)
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
		// A valueless yield sends value.Undefined, valid only when the channel's Y is the
		// boxed value.Value: the dynamic box of an untyped generator, or a declared element
		// type that itself lowers to value.Value (undefined, null, void, never, any, unknown),
		// as in a Generator<undefined>. A generator whose Y is a concrete Go type no undefined
		// fits (a typed yield fixed it, or the declared element is a real type) stays a later
		// slice rather than send an undefined into a typed channel.
		const boxFlags = frontend.TypeAny | frontend.TypeUnknown | frontend.TypeUndefined |
			frontend.TypeNull | frontend.TypeVoid | frontend.TypeNever
		if r.genYieldType.Flags&boxFlags == 0 {
			return nil, &NotYetLowerable{Reason: "a valueless yield in a generator that also yields a typed value is a later slice"}
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(r.genCo), Sel: ident("Yield")}, Args: []ast.Expr{sel("value", "Undefined")}}, nil
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
	delegateT := r.prog.TypeAt(delegate)
	// Inside an async generator the delegate is another async generator: pulling it awaits
	// each of its pulls, so YieldFrom takes the boxer that lifts the delegate's yield into
	// the value.Value the awaited pull carries, the async mirror of the plain delegate drive.
	if r.inAsyncGen {
		elem, ok := r.asyncGeneratorElemType(delegateT)
		if !ok {
			return nil, &NotYetLowerable{Reason: "a yield* over a non-async-generator iterable is a later slice"}
		}
		sub, err := r.lowerExpr(delegate)
		if err != nil {
			return nil, err
		}
		boxer, err := r.genElemBoxer(elem)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(r.genCo), Sel: ident("YieldFrom")}, Args: []ast.Expr{sub, boxer}}, nil
	}
	if _, ok := r.generatorElemType(delegateT); !ok {
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
	// A body that yields no typed value takes its element type from the declared
	// Generator<T> return annotation so the coroutine's Y matches the T the consumer
	// reads it through. Read that declared element off the signature return here, where
	// the function node is in hand, and pass it as the fallback generatorYieldType uses
	// only when the body itself fixes no element type.
	var declElem frontend.Type
	var declOK bool
	if sig, ok := r.prog.SignatureAt(fn); ok {
		declElem, declOK = r.generatorElemType(sig.Return)
	}
	elemType, elemGo, err := r.generatorYieldType(block, declElem, declOK)
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
	// A destructured parameter reads its bound names out of the synthesized parameter
	// field at body entry, the same entry bindings the plain function path injects. The
	// coroutine func closes over the enclosing params (the Go function's for a
	// declaration, the closure's for an expression), so the bindings sit at the top of
	// the coroutine body where the yielded statements read the names. nil when no
	// parameter destructures, so a plain generator body is untouched.
	sig, _ := r.prog.SignatureAt(fn)
	binds, err := r.paramDestructureBindings(r.funcParamNodes(fn), sig)
	if err != nil {
		return nil, nil, err
	}
	if len(binds) != 0 {
		body.List = append(binds, body.List...)
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
	// A generator is called through the shared finishCall path, which fills value.None
	// for an omitted bare optional the same way a plain function's call does, so the
	// body tracks the full optParamsOf, both the bare x?: T and a required
	// x: T | undefined. The push runs before funcParamFields builds the fields, so a
	// bare optional renders its value.Opt[T] field instead of handing back, and a read
	// the checker narrowed to T unwraps it with .Get().
	defer r.pushOptParams(r.optParamsOf(fn, sig))()
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
