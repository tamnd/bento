package lower

import (
	"go/ast"
	"maps"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the Promise constructor, new Promise(executor). The executor is
// an inline function the constructor runs now, handed a resolve and a reject callback
// it calls to settle the promise. The lib.d.ts types of those two callbacks carry
// unions and optionals (resolve takes T or a PromiseLike<T>, reject takes an optional
// any) that the normal callback-parameter path cannot render, so the executor is
// lowered with synthetic Go parameter types, resolve func(T) and reject
// func(value.Value), and a call to either is intercepted in callExpr and bridged the
// settle way rather than through its declared signature.

// promiseSettle records how one executor parameter settles the promise. resolve
// carries a value of the element type, so it holds that element type to bridge the
// argument against; reject carries an arbitrary boxed value and needs no element
// type. isVoid marks a resolve on a Promise<void>, whose element type is the unit
// placeholder rather than a real value.
type promiseSettle struct {
	isReject bool
	isVoid   bool
	elem     frontend.Type
}

// newPromise lowers new Promise(executor) to value.NewPromise[T](func(resolve
// func(T), reject func(value.Value)) { <executor body> }). The executor must be an
// inline function so its body lowers in place with the two settle callbacks in scope;
// a promise built from a stored executor value hands back. The element type T comes
// off the promise the new expression produces, the same read the async paths make.
// The executor's own parameter names bind the two callbacks, so a body that calls
// resolve or reject routes through the settle interception in callExpr.
func (r *Renderer) newPromise(n frontend.Node, args []frontend.Node) (ast.Expr, error) {
	// An explicit type argument, new Promise<number>(...), rides in as an unknown
	// node ahead of the executor, so the value arguments are the ones that are not
	// type nodes; only the executor may remain.
	var valueArgs []frontend.Node
	for _, a := range args {
		if a.Kind() != frontend.NodeUnknown {
			valueArgs = append(valueArgs, a)
		}
	}
	if len(valueArgs) != 1 {
		return nil, &NotYetLowerable{Reason: "new Promise takes a single executor argument"}
	}
	executor := valueArgs[0]
	if executor.Kind() != frontend.NodeArrowFunction && executor.Kind() != frontend.NodeFunctionExpression {
		return nil, &NotYetLowerable{Reason: "a Promise executor that is not an inline function is a later slice"}
	}
	ekids := r.prog.Children(executor)
	if len(ekids) == 0 || ekids[len(ekids)-1].Kind() != frontend.NodeBlock {
		return nil, &NotYetLowerable{Reason: "a Promise executor with a concise body is a later slice"}
	}
	elem, ok := r.promiseElem(r.prog.TypeAt(n))
	if !ok {
		return nil, &NotYetLowerable{Reason: "new Promise whose type is not a Promise is a later slice"}
	}
	names, err := r.promiseExecutorParams(executor)
	if err != nil {
		return nil, err
	}

	void := isVoidReturn(elem)
	elemType := ast.Expr(sel("value", "Unit"))
	if !void {
		et, err := r.typeExpr(elem)
		if err != nil {
			return nil, err
		}
		elemType = et
	}

	// The two Go parameters are always emitted, even when the executor names only
	// resolve, because value.NewPromise passes both callbacks and the func value must
	// match its signature. An unnamed slot takes the blank identifier so it neither
	// declares a Go local nor registers as a settle callee.
	resolveName, rejectName := "_", "_"
	if len(names) >= 1 && names[0] != "" {
		resolveName = names[0]
	}
	if len(names) >= 2 && names[1] != "" {
		rejectName = names[1]
	}
	fields := []*ast.Field{
		{Names: []*ast.Ident{ident(resolveName)}, Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Type: elemType}}}}},
		{Names: []*ast.Ident{ident(rejectName)}, Type: &ast.FuncType{Params: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "Value")}}}}},
	}

	prev := r.promiseSettleParams
	r.promiseSettleParams = mergeSettle(prev, names, void, elem)
	defer func() { r.promiseSettleParams = prev }()

	lit, err := r.blockBodyArrow(executor, fields)
	if err != nil {
		return nil, err
	}
	r.usesPromise = true
	r.requireImport(valuePkg)
	call := &ast.CallExpr{Fun: index(sel("value", "NewPromise"), elemType), Args: []ast.Expr{lit}}
	return call, nil
}

// promiseExecutorParams reads the executor's parameter names in order, the source
// spellings a resolve or reject call in the body refers to. Each parameter must be a
// plain identifier; a destructured or defaulted executor parameter is a later slice.
// An empty slot (fewer than two parameters) yields an empty name, which newPromise
// renders as the blank identifier.
func (r *Renderer) promiseExecutorParams(executor frontend.Node) ([]string, error) {
	var names []string
	for _, k := range r.prog.Children(executor) {
		if k.Kind() != frontend.NodeParameter {
			continue
		}
		pkids := r.prog.Children(k)
		if len(pkids) == 0 || pkids[0].Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: "a Promise executor parameter that is not a plain identifier is a later slice"}
		}
		names = append(names, r.prog.Text(pkids[0]))
	}
	if len(names) > 2 {
		return nil, &NotYetLowerable{Reason: "a Promise executor with more than a resolve and reject parameter is a later slice"}
	}
	return names, nil
}

// mergeSettle overlays the executor's settle parameters on the inherited set, so a
// resolve or reject inside a nested Promise executor refers to its own promise while
// an outer executor's callbacks stay reachable from code that did not shadow them. A
// name the inner executor reuses shadows the outer entry, matching lexical scope.
func mergeSettle(prev map[string]promiseSettle, names []string, void bool, elem frontend.Type) map[string]promiseSettle {
	out := make(map[string]promiseSettle, len(prev)+len(names))
	maps.Copy(out, prev)
	if len(names) >= 1 && names[0] != "" {
		out[names[0]] = promiseSettle{isVoid: void, elem: elem}
	}
	if len(names) >= 2 && names[1] != "" {
		out[names[1]] = promiseSettle{isReject: true}
	}
	return out
}

// promiseStaticCall lowers a static call on the global Promise constructor. It
// reports handled=false when the callee is not Promise.<method> on the ambient
// global, so the caller falls through to the ordinary dispatch; a call that is on
// Promise but names a method this slice does not cover reports handled=true with a
// hand-back so it does not fall through to a misleading receiver-typed error.
// Promise.resolve and Promise.reject are covered; Promise.all and the rest wait on
// their own slices.
func (r *Renderer) promiseStaticCall(call, callee frontend.Node, argNodes []frontend.Node) (ast.Expr, bool, error) {
	kids := r.prog.Children(callee)
	if len(kids) != 2 {
		return nil, false, nil
	}
	recvNode, method := kids[0], r.prog.Text(kids[1])
	if !r.isGlobalRef(recvNode, "Promise") {
		return nil, false, nil
	}
	switch method {
	case "resolve":
		expr, err := r.promiseResolve(call, argNodes)
		return expr, true, err
	case "reject":
		expr, err := r.promiseReject(call, argNodes)
		return expr, true, err
	case "all":
		expr, err := r.promiseAll(call, argNodes)
		return expr, true, err
	case "allSettled":
		// allSettled fulfills with a record per input carrying its status and value or
		// reason, a discriminated union of objects the runtime would have to construct
		// from the fulfilled values. Building that union from a generated object value
		// is the union-construction slice, so allSettled hands back until it lands.
		return nil, true, &NotYetLowerable{Reason: "Promise.allSettled needs discriminated-union construction, a later slice"}
	default:
		return nil, true, &NotYetLowerable{Reason: "Promise." + method + " is a later slice"}
	}
}

// promiseResolve lowers Promise.resolve(value). A value that is already a promise is
// handed back untouched, the same-promise rule Promise.resolve applies to a genuine
// promise (the common thenable); a plain, non-thenable value mints a settled promise
// carrying it, value.Resolved[T](value). A foreign thenable, an object with a then
// method that is not a runtime promise, needs the adoption machinery and is a later
// slice, as is a resolve of a dynamic value whose shape is hidden. The element type
// comes off the promise the call produces, the resolved Awaited<T>.
func (r *Renderer) promiseResolve(call frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	elem, ok := r.promiseElem(r.prog.TypeAt(call))
	if !ok {
		return nil, &NotYetLowerable{Reason: "Promise.resolve whose type is not a Promise is a later slice"}
	}
	elemType := ast.Expr(sel("value", "Unit"))
	if !isVoidReturn(elem) {
		et, err := r.typeExpr(elem)
		if err != nil {
			return nil, err
		}
		elemType = et
	}
	r.usesPromise = true
	r.requireImport(valuePkg)
	if len(argNodes) == 0 {
		return &ast.CallExpr{Fun: index(sel("value", "Resolved"), elemType), Args: []ast.Expr{&ast.CompositeLit{Type: sel("value", "Unit")}}}, nil
	}
	arg := argNodes[0]
	argType := r.prog.TypeAt(arg)
	if _, isThenable := r.promiseElem(argType); isThenable {
		argGo, err := r.typeExpr(argType)
		if err != nil {
			return nil, err
		}
		if !isPromiseGoType(argGo) {
			return nil, &NotYetLowerable{Reason: "Promise.resolve of a foreign thenable is a later slice"}
		}
		return r.lowerExpr(arg)
	}
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	bridged, err := r.bridgeArg(lowered, arg, elem)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: index(sel("value", "Resolved"), elemType), Args: []ast.Expr{bridged}}, nil
}

// promiseReject lowers Promise.reject(reason) to a promise already rejected carrying
// the reason, value.Rejected[T](value.NewRejection(reason)). The reason boxes to a
// dynamic value.Value, or undefined when omitted, since a promise may reject with any
// JavaScript value. The element type comes off the promise the call produces; a reject
// whose element type is never or otherwise does not lower carries the unit type, since
// a rejected promise holds no fulfilled value the surrounding code reads.
func (r *Renderer) promiseReject(call frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	elemType := ast.Expr(sel("value", "Unit"))
	if elem, ok := r.promiseElem(r.prog.TypeAt(call)); ok && !isVoidReturn(elem) {
		if et, err := r.typeExpr(elem); err == nil {
			elemType = et
		}
	}
	reason := ast.Expr(sel("value", "Undefined"))
	if len(argNodes) > 0 {
		boxed, err := r.boxOperand(argNodes[0])
		if err != nil {
			return nil, err
		}
		reason = boxed
	}
	r.usesPromise = true
	r.requireImport(valuePkg)
	rejection := &ast.CallExpr{Fun: sel("value", "NewRejection"), Args: []ast.Expr{reason}}
	return &ast.CallExpr{Fun: index(sel("value", "Rejected"), elemType), Args: []ast.Expr{rejection}}, nil
}

// promiseAll lowers Promise.all(iterable) to value.All[T](iterable), the promise that
// fulfills with the slice of the inputs' values once every input fulfills and rejects
// with the first rejection. The element type T is the array element of the fulfilled
// value the call produces, a Promise<T[]>, and the argument is an array of promises of
// that same T, which lowers directly to the []*value.Promise[T] the runtime takes. An
// argument that is not an array of runtime promises, a bare iterable or a mixed slice,
// hands back rather than pass the runtime a slice it cannot combine.
func (r *Renderer) promiseAll(call frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Promise.all takes a single iterable argument"}
	}
	resultElem, ok := r.promiseElem(r.prog.TypeAt(call))
	if !ok {
		return nil, &NotYetLowerable{Reason: "Promise.all whose type is not a Promise is a later slice"}
	}
	elem, ok := r.prog.ElementType(resultElem)
	if !ok {
		return nil, &NotYetLowerable{Reason: "Promise.all whose fulfilled value is not an array is a later slice"}
	}
	elemType, err := r.typeExpr(elem)
	if err != nil {
		return nil, err
	}
	arg := argNodes[0]
	argElem, ok := r.arrayElem(arg)
	if !ok || !isPromiseGoType(argElem) {
		return nil, &NotYetLowerable{Reason: "Promise.all over a non-array or non-promise iterable is a later slice"}
	}
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	r.usesPromise = true
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: index(sel("value", "All"), elemType), Args: []ast.Expr{lowered}}, nil
}

// isPromiseGoType reports whether a lowered Go type is *value.Promise[...], the shape
// a genuine runtime promise takes, so Promise.resolve can tell a real promise (handed
// back untouched) from a foreign thenable object (whose lowered type is a struct).
func isPromiseGoType(e ast.Expr) bool {
	star, ok := e.(*ast.StarExpr)
	if !ok {
		return false
	}
	idx, ok := star.X.(*ast.IndexExpr)
	if !ok {
		return false
	}
	s, ok := idx.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := s.X.(*ast.Ident)
	return ok && pkg.Name == "value" && s.Sel.Name == "Promise"
}

// settleCall lowers a call to a Promise executor's resolve or reject callback. A
// resolve carries a value of the promise's element type, bridged the way an argument
// crosses into a typed parameter, or the unit placeholder for a Promise<void>; a
// reject carries an arbitrary value boxed into a dynamic value.Value, or undefined
// when called with no argument, since a promise may reject with any JavaScript value.
func (r *Renderer) settleCall(s promiseSettle, name string, args []frontend.Node) (ast.Expr, error) {
	goName, ok := localName(name)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a Promise settle callback name is not a Go identifier"}
	}
	if s.isReject {
		arg := ast.Expr(sel("value", "Undefined"))
		if len(args) > 0 {
			boxed, err := r.boxOperand(args[0])
			if err != nil {
				return nil, err
			}
			arg = boxed
		}
		return &ast.CallExpr{Fun: ident(goName), Args: []ast.Expr{arg}}, nil
	}
	if s.isVoid {
		return &ast.CallExpr{Fun: ident(goName), Args: []ast.Expr{&ast.CompositeLit{Type: sel("value", "Unit")}}}, nil
	}
	if len(args) == 0 {
		return nil, &NotYetLowerable{Reason: "resolve() with no value on a valued promise is a later slice"}
	}
	lowered, err := r.lowerExpr(args[0])
	if err != nil {
		return nil, err
	}
	bridged, err := r.bridgeArg(lowered, args[0], s.elem)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: ident(goName), Args: []ast.Expr{bridged}}, nil
}
