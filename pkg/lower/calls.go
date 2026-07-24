package lower

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers calls: the callExpr dispatch, method calls on primitives and
// receivers, the built-in globals (console, Math, JSON, Number, String,
// process, performance), the primitive coercions called as functions, and the
// method tables that guard argument types.

// callExpr lowers a call whose callee is a bare identifier. The identifier
// resolves either to a top-level function symbol, lowered to the same exported
// Go name RenderFunc gives the declaration so a call and its target agree, or to
// a value binding of function type (an arrow stored in a const, a callback
// parameter), lowered to a direct Go call on that func value. A method call
// routes through methodCall before here. Arguments lower positionally; a spread
// or a defaulted or omitted argument hands back.
func (r *Renderer) callExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		return nil, &NotYetLowerable{Reason: "call expression exposed no callee"}
	}
	// A dynamic import() is a call whose callee is the import keyword, not a value.
	// It routes to the dynamic-import classifier before the callee paths below,
	// which would otherwise treat the keyword as a function value and hand back
	// with a misleading reason rather than the honest static/computed split.
	if expr, handled, err := r.dynamicImportCall(n, kids); handled || err != nil {
		return expr, err
	}
	// A callee that is a user-defined callable object, assert(x) where assert holds
	// an Assert struct, calls through the struct's reserved Call field: assert.Call(x).
	// It routes before the method and value-callee paths, which would otherwise call
	// the struct itself as if it were a bare Go func. A member callee is excluded
	// here because assert.sameValue(...) is a method on the object, handled by
	// methodCall below, not a call of the object itself. An ambient global (String,
	// Number, Array) is excluded too: those are callable objects in the type system,
	// but each has its own dedicated lowering below, so the reserved-Call path must
	// not shadow them. An import binding is excluded for the same reason.
	if kids[0].Kind() != frontend.NodePropertyAccessExpression && !r.isBuiltinCallee(kids[0]) && r.isCallableObject(r.prog.TypeAt(kids[0])) {
		recv, err := r.lowerExpr(kids[0])
		if err != nil {
			return nil, err
		}
		return r.finishCall(n, &ast.SelectorExpr{X: recv, Sel: ident("Call")}, kids[1:], nil, false, false, false)
	}
	// A member callee (s.charCodeAt(...)) is a method call, not a plain function
	// call; the string methods are the only ones covered so far. A call on a
	// namespace go: import (zstd.NewReader(...)) is also a member callee, but it is a
	// direct call into the Go package the namespace names, so it routes to the interop
	// lowering before the method-call path.
	if kids[0].Kind() == frontend.NodePropertyAccessExpression {
		if b, ok := r.namespaceGoCall(kids[0]); ok {
			return r.goImportCall(b, n, kids[1:])
		}
		// A call on a namespace import of a composed sibling (m.inc(1) where m is
		// import * as m) is a direct call to the export's package-level Go func, so it
		// routes here before the method-call path, which expects a value receiver the
		// namespace binding has none of.
		if expr, handled, err := r.internalNamespaceCall(n, kids[0], kids[1:]); handled || err != nil {
			return expr, err
		}
		// A static call on the global Array constructor (Array.of, Array.isArray)
		// is not a method on a value receiver, so it routes here before methodCall,
		// which expects a string, array, map, or class receiver. It needs the whole
		// call node n for Array.of's element type, which methodCall does not receive.
		if expr, handled, err := r.arrayStaticCall(n, kids[0], kids[1:]); handled || err != nil {
			return expr, err
		}
		// A static call on the global Promise constructor (Promise.resolve,
		// Promise.reject) is a constructor-level factory, not a method on a promise
		// value, so it routes here before methodCall, which expects a settled promise
		// receiver. It needs the whole call node n to read the resolved element type.
		if expr, handled, err := r.promiseStaticCall(n, kids[0], kids[1:]); handled || err != nil {
			return expr, err
		}
		// A static call on a concrete typed-array constructor (Int32Array.of,
		// Int32Array.from) is a constructor-level factory, not a method on a view, so
		// it routes here before methodCall, which expects a value receiver. It needs
		// the whole call node n to read the constructed array's element type.
		if expr, handled, err := r.typedArrayStaticCall(n, kids[0], kids[1:]); handled || err != nil {
			return expr, err
		}
		// Object.entries(o) on a fixed-shape object folds to a [key, value][] built
		// from the struct's fields, which needs the whole call node n to read the
		// result tuple's element type, a type methodCall does not receive. The dynamic
		// receiver stays on the objectCall path below.
		if expr, handled, err := r.objectEntriesShapeCall(n, kids[0], kids[1:]); handled || err != nil {
			return expr, err
		}
		return r.methodCall(kids[0], kids[1:])
	}
	// A method call spelled with a bracket key, C["m"](...) or c["m"](...), is the
	// call form a non-identifier or computed method name takes. When the key is a
	// constant string naming a static or instance method, it routes to the same
	// static or instance method dispatch the dotted call uses, before the
	// function-value path below, which would lower the callee as a bare method read
	// and hand back since a method read as a value is a later slice.
	if kids[0].Kind() == frontend.NodeElementAccessExpression {
		if expr, handled, err := r.bracketMethodCall(kids[0], kids[1:]); handled || err != nil {
			return expr, err
		}
	}
	// A callee that is neither a bare identifier nor a member expression is a
	// larger expression that evaluates to a function value: an array element
	// fs[0](x), the result of another call mk(5)(), a parenthesized arrow. It has
	// no name to resolve to a symbol, so it skips the builtin and user-function
	// routing below and lowers as a function value applied to its arguments.
	if kids[0].Kind() != frontend.NodeIdentifier {
		// A callee whose own type is dynamic is a boxed function value, so it dispatches
		// through the runtime Call rather than a static Go call, the same way a dynamic
		// member read resolves at runtime.
		if r.isDynamic(kids[0]) {
			return r.dynamicCall(kids[0], kids[1:])
		}
		callee, err := r.functionValueCallee(kids[0])
		if err != nil {
			return nil, err
		}
		return r.finishCall(n, callee, kids[1:], nil, false, false, false)
	}
	// A call to a Promise executor's resolve or reject parameter settles the promise
	// rather than calling a plain function value: the callback's lib.d.ts signature
	// carries unions and optionals that do not render, so its argument is bridged the
	// settle way here instead of through the normal value-callee path below.
	if s, ok := r.promiseSettleParams[r.prog.Text(kids[0])]; ok {
		return r.settleCall(s, r.prog.Text(kids[0]), kids[1:])
	}
	// A call to a name bound by a node: import is a call to a host builtin, not a
	// user function, so it routes to the value helper the builtin maps to before the
	// user-function path, which would reject the alias symbol the binding carries.
	if b, ok := r.nodeImports[r.prog.Text(kids[0])]; ok {
		return r.nodeBuiltinCall(b, kids[1:])
	}
	// A call to a name bound by a go: import is a direct call into a real Go package,
	// so it routes to the interop lowering before the user-function path, which would
	// reject the alias symbol the binding carries.
	if b, ok := r.goImports[r.prog.Text(kids[0])]; ok {
		return r.goImportCall(b, n, kids[1:])
	}
	// A bare call to an ambient global function (isNaN, isFinite) is not a call to
	// a user binding, so it routes to the global-function lowering before the
	// user-function path, which would otherwise reject it.
	if goName, ok := globalFn(r.prog.Text(kids[0])); ok && r.isAmbientGlobal(kids[0]) {
		return r.globalFnCall(goName, kids[0], kids[1:])
	}
	// String(x) called as a function is a primitive-to-string coercion, an ambient
	// global constructor call rather than a user function, so it routes before the
	// user-function path.
	if r.prog.Text(kids[0]) == "String" && r.isAmbientGlobal(kids[0]) {
		return r.stringCoercion(kids[1:])
	}
	// Number(x) called as a function is a primitive-to-number coercion, the
	// companion to String(x), and routes the same way before the user path.
	if r.prog.Text(kids[0]) == "Number" && r.isAmbientGlobal(kids[0]) {
		return r.numberCoercion(kids[1:])
	}
	// Boolean(x) called as a function is the third primitive coercion, and routes
	// the same way as String and Number before the user path.
	if r.prog.Text(kids[0]) == "Boolean" && r.isAmbientGlobal(kids[0]) {
		return r.booleanCoercion(kids[1:])
	}
	// BigInt(x) called as a function converts a number, string, or boolean to a
	// bigint, and routes the same way as the other three coercions before the user
	// path.
	if r.prog.Text(kids[0]) == "BigInt" && r.isAmbientGlobal(kids[0]) {
		return r.bigIntCoercion(kids[1:])
	}
	// parseFloat is a bare ambient global that reads a number from the front of a
	// string, so it routes like the coercions before the user path.
	if r.prog.Text(kids[0]) == "parseFloat" && r.isAmbientGlobal(kids[0]) {
		return r.parseFloatCall(kids[1:])
	}
	// parseInt takes an optional radix, so it has its own lowering rather than the
	// single-argument coercion shape, but routes the same way before the user path.
	if r.prog.Text(kids[0]) == "parseInt" && r.isAmbientGlobal(kids[0]) {
		return r.parseIntCall(kids[1:])
	}
	// The URI and base64 codec globals are bare ambient globals that take a single
	// string and return a string, so they route like the coercions before the user
	// path.
	if callee := r.prog.Text(kids[0]); r.isAmbientGlobal(kids[0]) {
		switch callee {
		case "encodeURIComponent":
			return r.unaryStringGlobal("EncodeURIComponent", callee, kids[1:])
		case "decodeURIComponent":
			return r.unaryStringGlobal("DecodeURIComponent", callee, kids[1:])
		case "encodeURI":
			return r.unaryStringGlobal("EncodeURI", callee, kids[1:])
		case "decodeURI":
			return r.unaryStringGlobal("DecodeURI", callee, kids[1:])
		case "btoa":
			return r.unaryStringGlobal("Btoa", callee, kids[1:])
		case "atob":
			return r.unaryStringGlobal("Atob", callee, kids[1:])
		}
	}
	// Symbol() and Symbol(desc) construct a fresh unique symbol, the boxed value a
	// symbol-keyed property carries, and route before the user path the way the
	// coercions do since Symbol is an ambient global, not a user binding.
	if r.prog.Text(kids[0]) == "Symbol" && r.isAmbientGlobal(kids[0]) {
		return r.symbolConstructor(kids[1:])
	}
	// Function("a", "return a") called as a function builds a function from source
	// text at run time, the same construction as new Function and a member of the
	// eval family: the argument strings are parsed as a parameter list and a body. A
	// program that parses source it was handed is phase 11 (eval) work, so bento hands
	// it back with the reason that names where it belongs rather than the generic
	// ambient-global reason below.
	if r.prog.Text(kids[0]) == "Function" && r.isAmbientGlobal(kids[0]) {
		return nil, &NotYetLowerable{Reason: "a Function built from a source string is eval, deferred to phase 11"}
	}
	// require(specifier) calls the CommonJS loader. A specifier that resolves to a
	// module the build composed lowers to a direct call on that module's loader
	// function, which runs the module body once, caches its exports, and returns them
	// as the require expression's value. A specifier require cannot resolve statically,
	// or one whose target this slice does not compose, routes through the dynamic call
	// path instead: require is an ambient global backed by a package-level function
	// value (requireRef), so the callee lowers to bentoRequire and each argument boxes,
	// giving bentoRequire.Call(specifier). That path is used rather than the declared-
	// parameter one because require is typed any and has no signature to bind against,
	// so a bound call would drop the argument; the dynamic path passes it by value. The
	// runtime require throws "Cannot find module", so an unresolved require fails
	// honestly rather than resolving to a wrong value.
	if r.isGlobalRef(kids[0], "require") {
		if expr, handled, err := r.requireModuleCall(kids[0], kids[1:]); handled || err != nil {
			return expr, err
		}
		if expr, handled := r.builtinRequireCall(kids[1:]); handled {
			return expr, nil
		}
		return r.dynamicCall(kids[0], kids[1:])
	}
	// A bare call to any other ambient global (eval, and the globals whose lowering
	// is a later slice) is not a user binding and has no generated Go function to
	// stand behind it. The user-function path below would emit a call to the name's
	// capitalized form, which was never declared, so the whole unit hands back to the
	// interpreter here instead of building Go that names an undefined symbol. Every
	// ambient global bento does lower returned above before this point.
	if r.isAmbientGlobal(kids[0]) {
		return nil, &NotYetLowerable{Reason: "call to the ambient global " + r.prog.Text(kids[0]) + " is a later slice"}
	}
	sym, ok := r.prog.SymbolAt(kids[0])
	if !ok {
		return nil, &NotYetLowerable{Reason: "call to an unresolved callee is a later slice"}
	}
	// A call to a name imported from a sibling module resolves to an alias symbol,
	// which carries neither the function flag nor the exported name its declaration
	// took. Resolving the alias to the symbol it names recovers the callee the
	// sibling emitted, so the call spells the same Go function name, the way a call
	// within the declaring module does.
	sym = r.derefAlias(sym)
	// The callee resolves either to a top-level function symbol, called by the
	// exported Go name its declaration takes, or to a value binding of function
	// type (an arrow stored in a const, a callback parameter), called directly on
	// the Go func value the binding already lowered to. The two share the argument
	// lowering below; only the callee spelling differs.
	var callee ast.Expr
	var defaults []frontend.Node
	var variadicTail bool
	// A callee whose every reference is a direct call threads the real call-site
	// arguments through a hidden trailing parameter (argumentsthread.go), so this call
	// builds and passes that array; the snapshot arity guard is off for it. A callee
	// that keeps the parameter snapshot (argumentsPlan) instead needs the call to pass
	// exactly one argument per parameter, so buildCall hands a mismatched arity back
	// rather than drop or fill against the snapshot. Both are resolved once here from the
	// callee symbol and forwarded through finishCall; a function is at most one of the
	// two, so the snapshot guard never fires on a threaded callee.
	calleeThreadsArgs := r.funcSymThreadsArgs(sym)
	calleeReadsArgs := !calleeThreadsArgs && r.funcSymReadsArguments(sym)
	if goName, ok := r.funcExprSelf[sym]; ok {
		// The callee is a named function expression calling itself. Its two-step
		// lowering bound the closure to a Go local, so the recursive call is a plain
		// call on that func value rather than on a top-level function name.
		callee = ident(goName)
	} else if r.isDynamic(kids[0]) || r.localStorageDynamic(kids[0]) {
		// A callee identifier bound to a boxed value.Value slot dispatches through the
		// runtime Call even when its symbol carries the function flag, the shape a
		// require of a function-exporting module takes: const f = require('./fn') binds
		// f to the module's exported function as a box, not a Go func a static name could
		// call, and the checker aliases f's symbol to that exported function so the flag
		// is set. This must precede the SymbolFunction branch, which would otherwise
		// spell a capitalized Go name the boxed binding never emitted. The storage check
		// also catches an implicit-any binding whose type the checker evolved to a
		// concrete function while the slot itself stays a value.Value box.
		return r.dynamicCall(kids[0], kids[1:])
	} else if sym.Flags&frontend.SymbolFunction != 0 {
		name, ok := exportedField(sym.Name)
		if !ok {
			return nil, &NotYetLowerable{Reason: "called function name is not a Go identifier"}
		}
		// A call to an overloaded function runs its all-dynamic implementation, which
		// lowered to a Go func over value.Value parameters. Its argument bridging is not
		// the matched overload's parameter types SignatureAt reports here, so it routes to
		// overloadedCall, which boxes each argument instead. This is checked ahead of the
		// monomorphization and default handling below, which read a single signature the
		// overloaded callee does not have.
		if _, ok := r.overloadedFuncImpl(sym); ok {
			return r.overloadedCall(n, name, kids[1:])
		}
		// A call to a generic function resolves to the monomorphization its type
		// arguments fix, so the callee is the specialized Go name (Identity_num) the
		// declaration emitted, not the bare exported name. A generic bento could not
		// monomorphize hands back here rather than name a Go function that was never
		// emitted.
		if monoName, ok, err := r.monoCalleeName(n); err != nil {
			return nil, err
		} else if ok {
			name = monoName
		}
		callee = ident(name)
		// A direct call to a top-level function may omit a defaulted trailing
		// argument. A callee that fills its optional tail in its own body (a default
		// that reads an earlier parameter) takes the arguments the call supplies into a
		// Go variadic and defaults the rest itself, so no fill rides in; every other
		// defaulting callee fills the omitted slot at the call site from its defaults.
		if variadic, err := r.calleeFillsInBody(sym); err != nil {
			return nil, err
		} else if variadic {
			variadicTail = true
		} else {
			defaults = r.calleeDefaults(sym)
		}
	} else {
		// A bare identifier used as a callee that the checker accepted has a call
		// signature, so the binding is a function value: an arrow or function
		// expression stored in a local, or a parameter typed as a function. It lowers
		// as a function value the same way a non-identifier callee does, so the two
		// share functionValueCallee.
		lowered, err := r.functionValueCallee(kids[0])
		if err != nil {
			return nil, err
		}
		callee = lowered
		// A const-bound arrow collectArrowDefaults proved escape-safe lowered its
		// defaulted parameters as plain Go fields, so a call that omits one fills it here
		// with the arrow's default, exactly as a top-level function's default is filled.
		// The arrow is only ever a direct call (the escape analysis guarantees it), so the
		// call site always sees the binding and can reconstruct the default.
		if defs, ok := r.arrowCallDefaults[sym]; ok {
			defaults = defs
		}
	}
	return r.finishCall(n, callee, kids[1:], defaults, variadicTail, calleeReadsArgs, calleeThreadsArgs)
}

// finishCall lowers the argument list of call n against its signature and builds
// the Go call on the already-lowered callee. It is shared by every user-call path
// (a top-level function, a function-valued binding, and a non-identifier callee
// expression) so the argument bridging is written once. Arguments lower
// positionally, each bridged against its declared parameter, so a derived instance
// passed for a base parameter upcasts to the embedded base the same way an
// assignment would.
func (r *Renderer) finishCall(n frontend.Node, callee ast.Expr, argNodes []frontend.Node, defaults []frontend.Node, variadicTail bool, calleeReadsArgs bool, calleeThreadsArgs bool) (ast.Expr, error) {
	var params []frontend.Param
	var rest *frontend.Param
	if sig, ok := r.prog.SignatureAt(n); ok {
		params = sig.Params
		rest = sig.RestParam
	}
	return r.buildCall(callee, argNodes, params, rest, defaults, variadicTail, calleeReadsArgs, calleeThreadsArgs)
}

// buildFixedTupleSpreadCall lowers a call whose argument list contains a spread of a
// fixed-length tuple, f(...pair) or g(x, ...rest3), when the callee has only fixed
// parameters and the spread expands onto them exactly. Each spread of a tuple typed
// [A, B, C] becomes the field reads operand.E0, operand.E1, operand.E2, the same
// positional struct fields a tuple element access reads, so the spread splices its
// members in position with no runtime array. It reports handled false, leaving the
// caller's honest handback in place, for any shape it does not cover: a call-site
// default or variadic tail, a spread of a non-tuple or a tuple with an optional or
// rest element, a side-effecting spread operand whose repeated field reads would run
// its effect more than once, an argument count that does not match the parameters, or
// a spread element whose Go type differs from its parameter and would need the source
// node the bridge reads to coerce.
func (r *Renderer) buildFixedTupleSpreadCall(callee ast.Expr, argNodes []frontend.Node, params []frontend.Param, defaults []frontend.Node, variadicTail bool) (ast.Expr, bool, error) {
	if variadicTail || len(defaults) != 0 {
		return nil, false, nil
	}
	// A plan entry is either a source node to lower against its parameter, or a tuple
	// element read already at its element type. The two passes keep the structural
	// checks ahead of any lowering, so an unsupported shape returns handled false
	// without interning a struct or requiring an import on a path that then bails.
	type planEntry struct {
		node    frontend.Node // a plain argument to lower and bridge; nil for a spread element
		operand frontend.Node // the spread operand a tuple element reads from
		index   int           // the element position within the tuple
		typ     frontend.Type // the tuple element type
	}
	var plan []planEntry
	sawSpread := false
	for _, a := range argNodes {
		if a.Kind() != frontend.NodeSpreadElement {
			plan = append(plan, planEntry{node: a})
			continue
		}
		sawSpread = true
		operands := r.prog.Children(a)
		if len(operands) != 1 {
			return nil, false, nil
		}
		operand := operands[0]
		elems, ok := r.prog.TupleElements(r.prog.TypeAt(operand))
		if !ok {
			return nil, false, nil
		}
		if !r.repeatableOperand(operand) {
			return nil, false, nil
		}
		for i, el := range elems {
			if el.Optional || el.Rest {
				return nil, false, nil
			}
			plan = append(plan, planEntry{operand: operand, index: i, typ: el.Type})
		}
	}
	if !sawSpread || len(plan) != len(params) {
		return nil, false, nil
	}
	// Every spread element must already share its parameter's Go type: a spliced field
	// read carries no source node, so the argument bridge cannot coerce an int into a
	// float or box a static value into a dynamic slot the way a lowered node would.
	for j, e := range plan {
		if e.node != nil {
			continue
		}
		same, err := r.sameParamGoType(e.typ, params[j].Type)
		if err != nil {
			return nil, false, err
		}
		if !same {
			return nil, false, nil
		}
	}
	// The shape checks all passed, so lowering the operands and plain arguments now
	// commits to this call. A spread operand is lowered once and its field reads share
	// that receiver, which is why the operand must be repeatable.
	lowered := make(map[frontend.Node]ast.Expr)
	args := make([]ast.Expr, 0, len(plan))
	for j, e := range plan {
		if e.node != nil {
			a, err := r.lowerArgAt(e.node, params[j].Type)
			if err != nil {
				return nil, false, err
			}
			args = append(args, a)
			continue
		}
		recv, ok := lowered[e.operand]
		if !ok {
			rv, err := r.lowerExpr(e.operand)
			if err != nil {
				return nil, false, err
			}
			recv = rv
			lowered[e.operand] = rv
		}
		args = append(args, &ast.SelectorExpr{X: recv, Sel: ident("E" + itoa(e.index))})
	}
	return &ast.CallExpr{Fun: callee, Args: args}, true, nil
}

// sameParamGoType reports whether two checker types render to the same Go type, the
// test a spread element must pass to ride into a parameter with no bridging. It renders
// each type and compares the two Go type expressions structurally.
func (r *Renderer) sameParamGoType(a, b frontend.Type) (bool, error) {
	ago, err := r.typeExpr(a)
	if err != nil {
		return false, err
	}
	bgo, err := r.typeExpr(b)
	if err != nil {
		return false, err
	}
	return sameGoType(ago, bgo)
}

// calleeFillsInBody reports whether the top-level function a symbol names fills its
// optional tail in its own body through a Go variadic, the shape a default that
// reads an earlier parameter takes. A call to such a function passes only the
// arguments it supplies and lets the callee default the rest, so no call-site fill
// rides in. A shape that reads an earlier parameter but does not fit the variadic
// returns false here; the function itself hands back when it is lowered, so the
// whole unit routes to the engine and the mismatched call is never emitted.
func (r *Renderer) calleeFillsInBody(sym frontend.Symbol) (bool, error) {
	for _, d := range r.prog.Declarations(sym) {
		sig, ok := r.prog.SignatureAt(d)
		if !ok {
			continue
		}
		plan, err := r.variadicDefaultPlan(d, sig)
		if err != nil {
			return false, nil
		}
		return plan != nil, nil
	}
	return false, nil
}

// funcSymReadsArguments reports whether the function a symbol names reads its
// arguments object through the parameter snapshot argumentsPlan materializes. A
// call that reaches such a function must pass exactly one argument per parameter
// for the snapshot to stand in for the passed arguments, so the arity guard in
// buildCall consults this. Only a declaration whose signature is the all-required,
// rest-free shape the snapshot backs is considered: argumentsPlan hands a rest or
// omittable-arity function back on its own, so a mismatched call to one never
// reaches emission and needs no guard here. The body is scanned with the same
// scanArguments machinery the plan uses so the two agree on what a read is.
func (r *Renderer) funcSymReadsArguments(sym frontend.Symbol) bool {
	for _, fn := range r.symFuncNodes(sym) {
		if r.funcNodeReadsArguments(fn) {
			return true
		}
	}
	return false
}

// funcNodeReadsArguments reports whether one function node reads its arguments
// object through the parameter snapshot argumentsPlan materializes. Only a
// signature the snapshot backs is at issue: argumentsPlan hands a rest or
// omittable-arity function back on its own, so one is never emitted to be called
// wrong. The body is scanned with the same scanArguments machinery the plan uses so
// the two agree on what a read is.
func (r *Renderer) funcNodeReadsArguments(fn frontend.Node) bool {
	sig, ok := r.prog.SignatureAt(fn)
	if !ok {
		return false
	}
	if sig.RestParam != nil || sig.MinArgs != len(sig.Params) {
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
	return reads
}

// boxedFuncReadsArguments reports whether the function value flowing into a dynamic
// slot reads its arguments object through the parameter snapshot. It covers a
// function expression boxed directly (myAny = function(){...}) and a named function
// or binding boxed by reference (myAny = f), so either boxing hands back rather than
// emit a snapshot the dynamic call convention cannot keep in step with the real
// argument count. An arrow carries no arguments of its own, so it is not a case here.
func (r *Renderer) boxedFuncReadsArguments(src frontend.Node) bool {
	if src.Kind() == frontend.NodeFunctionExpression {
		return r.funcNodeReadsArguments(src)
	}
	if src.Kind() == frontend.NodeIdentifier {
		if sym, ok := r.prog.SymbolAt(src); ok {
			return r.funcSymReadsArguments(r.derefAlias(sym))
		}
	}
	return false
}

// symFuncNodes returns the function declaration or expression nodes that stand
// behind a called symbol, whose body the arity guard scans for a read of
// arguments. A top-level function is its own declaration; a value binding holds its
// function as an initializer (const f = function(){...}) or takes it from a later
// assignment (var ref; ref = function(){...}). An arrow is not returned: it has no
// arguments object of its own and reads the enclosing function's, so a call to an
// arrow-bound value carries no snapshot to guard. The later-assignment scan runs
// only for a binding whose declaration holds no function-like value at all, so a
// plain call to a function or an arrow pays no scan.
func (r *Renderer) symFuncNodes(sym frontend.Symbol) []frontend.Node {
	var out []frontend.Node
	sawFuncLike := false
	for _, d := range r.prog.Declarations(sym) {
		fn, ok := r.declValueFunc(d)
		if !ok {
			continue
		}
		sawFuncLike = true
		if k := fn.Kind(); k == frontend.NodeFunctionDeclaration || k == frontend.NodeFunctionExpression {
			out = append(out, fn)
		}
	}
	if !sawFuncLike {
		if fn, ok := r.assignedFuncExpr(sym); ok {
			out = append(out, fn)
		}
	}
	return out
}

// declValueFunc returns the function-like node a declaration binds as its value:
// the declaration itself for a function declaration, or the function or arrow
// expression a value binding holds as its initializer. A pre-order walk finds the
// binding's own function before any it nests. The bool reports whether any
// function-like initializer was found, so the caller can tell a binding whose value
// is a known arrow apart from a bare declaration whose function arrives in a later
// assignment.
func (r *Renderer) declValueFunc(d frontend.Node) (frontend.Node, bool) {
	var found frontend.Node
	var ok bool
	var walk func(frontend.Node)
	walk = func(node frontend.Node) {
		if ok {
			return
		}
		if isFunctionLike(node.Kind()) {
			found, ok = node, true
			return
		}
		for _, c := range r.prog.Children(node) {
			walk(c)
		}
	}
	walk(d)
	return found, ok
}

// assignedFuncExpr finds the function expression a bare-declared binding takes from
// a later assignment, ref = function(){...} for a var ref with no initializer. It
// scans the source files for the first assignment whose target resolves to the
// symbol and whose value is a function expression, the shape a function-valued
// binding written in two steps takes. It reports ok=false when no such assignment
// exists, leaving the binding uncovered rather than guessing.
func (r *Renderer) assignedFuncExpr(sym frontend.Symbol) (frontend.Node, bool) {
	var found frontend.Node
	var ok bool
	var walk func(frontend.Node)
	walk = func(node frontend.Node) {
		if ok {
			return
		}
		if node.Kind() == frontend.NodeBinaryExpression {
			kids := r.prog.Children(node)
			if len(kids) == 3 && r.prog.Text(kids[1]) == "=" &&
				kids[0].Kind() == frontend.NodeIdentifier &&
				kids[2].Kind() == frontend.NodeFunctionExpression {
				if s, has := r.prog.SymbolAt(kids[0]); has && s == sym {
					found, ok = kids[2], true
					return
				}
			}
		}
		for _, c := range r.prog.Children(node) {
			walk(c)
		}
	}
	for _, f := range r.prog.SourceFiles() {
		walk(f)
		if ok {
			break
		}
	}
	return found, ok
}

// buildCall lowers the argument list against a known parameter list and builds
// the Go call on the already-lowered callee. It is the body finishCall reaches
// through the call node's signature, split out so a member call whose signature
// comes from the receiver's property type (an assert.sameValue(...) on a callable
// object) shares the exact same argument bridging without a call node to look the
// signature up from.
func (r *Renderer) buildCall(callee ast.Expr, argNodes []frontend.Node, params []frontend.Param, rest *frontend.Param, defaults []frontend.Node, variadicTail bool, calleeReadsArgs bool, calleeThreadsArgs bool) (ast.Expr, error) {
	// A threaded callee takes the real call-site arguments through a hidden trailing
	// parameter, so the declared parameters still bind by position (extras dropped,
	// omissions filled) exactly as below while the actual argument list rides the hidden
	// array appended at the end. The snapshot arity guard is skipped, and an extra
	// argument past the parameters is kept for the array rather than dropped: it is a
	// real argument the arguments object must see. funcSymCallShape proved every such
	// call passes repeatable, non-spread arguments, so building the array below re-reads
	// no side effect and enumerates every argument.
	// A callee whose body reads its arguments object models that object from a
	// snapshot of its parameters (see argumentsPlan), which stands in for the passed
	// arguments only when the call passes exactly one argument per parameter. The
	// tolerant argument handling below drops an argument past the parameter count and
	// fills a missing one from a default or an absent value, so a call whose fixed
	// argument count differs from the parameter count would leave the snapshot reading
	// the wrong length and the wrong slots: the dropped extras never reach it and the
	// filled slots box a value the call never passed. Neither the too-many nor the
	// too-few direction is faithful without the real call-site count, so such a call
	// hands back rather than emit a snapshot the arity does not fix. This reads the raw
	// argument node count before any drop or fill. A rest parameter or a variadic-tail
	// callee gathers a call-varying count that is not a snapshot function, so the check
	// is confined to the fixed-arity callees the snapshot backs.
	if calleeReadsArgs && rest == nil && !variadicTail && len(argNodes) != len(params) {
		return nil, &NotYetLowerable{Reason: "arguments in a function called with an arity the parameters do not fix needs the call-site count, a later slice"}
	}
	// A spread of a fixed-length tuple into a fixed parameter list expands to the
	// tuple's element reads, f(...pair) becoming f(pair.E0, pair.E1), so the spread
	// lowers rather than hand back on the spread element kind the ordinary argument
	// loop cannot lower. It routes only when there is no rest parameter and the
	// expansion lands exactly on the fixed parameters; every other spread shape leaves
	// handled false and falls through to the honest handback below.
	if rest == nil {
		if expr, handled, err := r.buildFixedTupleSpreadCall(callee, argNodes, params, defaults, variadicTail); handled || err != nil {
			return expr, err
		}
	}
	args := make([]ast.Expr, 0, len(params)+1)
	// The arguments that land on a fixed parameter lower and bridge in position; any
	// beyond the fixed count belong to a rest parameter and are gathered below.
	nFixed := len(argNodes)
	if nFixed > len(params) {
		nFixed = len(params)
	}
	for i := 0; i < nFixed; i++ {
		a := argNodes[i]
		// An explicit undefined argument in a defaulted slot counts as a missing
		// argument: the language fills the parameter's default for undefined exactly as
		// it does for an omission. The default stands in for the undefined so the Go
		// call carries the default's value, not an undefined the static slot cannot
		// hold. A variadicTail callee fills its own tail by arity in its body, so an
		// undefined there rides the variadic untouched and is left alone.
		if !variadicTail && i < len(defaults) && defaults[i] != nil && r.isUndefinedLiteral(a) {
			a = defaults[i]
		}
		// A bare reference to a pure rest-parameter function passed to a rest-parameter
		// func-typed slot fits the slot's Go func type directly: funcTypeOf gives the
		// slot the same trailing *value.Array[T] field the function's own rest parameter
		// lowers to, so the exported name fits with no bridging. It routes here before
		// the value path, which hands a rest function back as needing a defaulting
		// wrapper because its arity is omittable in the source.
		if name, ok := r.restFuncValueArg(a, params[i].Type); ok {
			args = append(args, name)
			continue
		}
		// An untyped destructured parameter takes one boxed value.Value slot rather than
		// the Go struct or slice its checker type would map to, so the argument boxes to a
		// dynamic value here the same way a static value crossing into any does. Bridging
		// against the parameter's object type instead would build the wrong struct, the one
		// with dynamic fields the argument's own concrete struct does not match. An array or
		// object literal boxes member by member straight from its node: its contextual type
		// is the pattern's shapeless tuple, which the typed literal path cannot spell, so it
		// skips that lowering rather than hand the whole call back on it.
		if r.dynamicParamSlot(params[i]) {
			if boxed, ok, err := r.boxLiteralToDynamic(a); err != nil {
				return nil, err
			} else if ok {
				args = append(args, boxed)
				continue
			}
			lowered, err := r.lowerExpr(a)
			if err != nil {
				return nil, err
			}
			lowered, err = r.bridgeArg(lowered, a, frontend.Type{Flags: frontend.TypeAny})
			if err != nil {
				return nil, err
			}
			args = append(args, lowered)
			continue
		}
		lowered, err := r.lowerArgAt(a, params[i].Type)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	// A callee that fills its optional tail in its own body takes the arguments the
	// call supplies into a Go variadic and defaults the rest itself, so an omitted
	// trailing argument needs no call-site fill: the provided ones already landed in
	// the loop above and the variadic absorbs the gap. Every other defaulting callee
	// fills an omitted trailing argument here with the parameter's default, so the Go
	// call passes every argument the lowered function expects. A defaultless omission
	// on a dynamic parameter fills with value.Undefined, the absent value the language
	// binds there; a defaultless static omission hands back rather than emit a short
	// call.
	if !variadicTail {
		for i := len(argNodes); i < len(params); i++ {
			var def frontend.Node
			if i < len(defaults) {
				def = defaults[i]
			}
			if def == nil {
				if params[i].Type.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
					r.requireImport(valuePkg)
					args = append(args, sel("value", "Undefined"))
					continue
				}
				// A bare optional parameter of the T | undefined shape takes a
				// value.Opt[T] field, so an omitted trailing argument fills the empty
				// option value.None[T](), the absent value its body reads, matching the
				// Some a supplied argument wraps in through bridgeArg.
				if inner, ok := r.optionalInner(r.prog.UnionMembers(params[i].Type)); ok {
					none, err := r.noneOf(inner)
					if err != nil {
						return nil, err
					}
					args = append(args, none)
					continue
				}
				return nil, &NotYetLowerable{Reason: "a call that omits an argument the callee does not default is a later slice"}
			}
			lowered, err := r.lowerArgAt(def, params[i].Type)
			if err != nil {
				return nil, err
			}
			args = append(args, lowered)
		}
	}
	if rest != nil {
		restArg, err := r.gatherRest(*rest, argNodes[nFixed:])
		if err != nil {
			return nil, err
		}
		args = append(args, restArg)
	} else if !calleeThreadsArgs && len(argNodes) > len(params) {
		// JavaScript evaluates every argument left to right and then ignores the ones
		// past the callee's parameter count, so a call with extra arguments runs the
		// fixed ones and drops the rest. Go has no way to pass an argument a function
		// does not declare, so the extras cannot ride into the call; dropping them is
		// faithful only when evaluating them changes nothing observable. An extra that
		// is a literal or a plain variable read has no side effect and cannot throw, so
		// it drops cleanly; an extra that could mutate, call, or throw would need its
		// effect preserved before the call and hands back to a later slice rather than
		// silently vanishing. A threaded callee is excluded: its extras are real
		// arguments the hidden array carries, not values to drop.
		for _, extra := range argNodes[len(params):] {
			if !r.isDroppableExtraArg(extra) {
				return nil, &NotYetLowerable{Reason: "a call with a side-effecting extra argument the callee ignores is a later slice"}
			}
		}
	}
	// The hidden arguments array rides last, after the fixed parameters and any rest, so
	// its position matches the trailing field hiddenArgsField added to the declaration.
	// It carries every actual argument, so arguments.length and arguments[i] in the body
	// see the real call, whatever the parameter count.
	if calleeThreadsArgs {
		hidden, err := r.hiddenArgsArray(argNodes)
		if err != nil {
			return nil, err
		}
		args = append(args, hidden)
	}
	return &ast.CallExpr{Fun: callee, Args: args}, nil
}

// restFuncValueArg lowers a bare reference to a top-level function passed as an
// argument to a rest-parameter func-typed parameter. A pure rest-parameter function
// (a rest, with no defaulted or optional parameter) lowers to a Go func whose single
// trailing *value.Array[T] field is the same shape funcTypeOf gives the slot, so the
// exported name fits the slot directly with no defaulting wrapper. It reports
// ok=false for any other shape, leaving the general argument path to lower or hand
// back: a slot that is not rest-typed takes a Go func of a different arity the name
// does not fit, and the value path keeps its handback there.
func (r *Renderer) restFuncValueArg(argNode frontend.Node, pt frontend.Type) (ast.Expr, bool) {
	if argNode.Kind() != frontend.NodeIdentifier {
		return nil, false
	}
	sym, ok := r.prog.SymbolAt(argNode)
	if !ok || sym.Flags&frontend.SymbolFunction == 0 {
		return nil, false
	}
	if !r.pureRestFunc(sym) {
		return nil, false
	}
	if calls, _ := r.prog.Signatures(pt); len(calls) != 1 || calls[0].RestParam == nil {
		return nil, false
	}
	name, ok := exportedField(sym.Name)
	if !ok {
		return nil, false
	}
	return ident(name), true
}

// functionMethodCall lowers f.call(...), f.apply(...), and f.bind(...) on a plain
// function value. call and apply invoke the function with the this argument the source
// passes and a list of positional arguments, call spelling them inline and apply
// gathering them in an array; bind fixes this and any leading arguments and yields a
// new function value. bento's plain functions take no this, since a body that reads
// this hands back when the function is lowered, so the this argument only sets a
// receiver the function never reads and drops once its evaluation is known to be pure.
// call and apply then lower exactly as the direct call F(args) would, so an invocation
// through them and a bare call share the same argument bridging. A method other than
// these three reports ok=false and falls to the non-string handback, a later slice.
func (r *Renderer) functionMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, bool, error) {
	if method == "bind" {
		return r.functionBindCall(recvNode)
	}
	if method != "call" && method != "apply" {
		return nil, false, nil
	}
	name, sig, ok, err := r.functionMethodTarget(recvNode, method)
	if !ok || err != nil {
		return nil, false, err
	}
	if len(argNodes) > 0 && !r.droppableThisArg(argNodes[0]) {
		return nil, false, &NotYetLowerable{Reason: method + " with a this argument that is not a plain value is a later slice"}
	}
	callArgs, err := r.functionInvokeArgs(method, argNodes)
	if err != nil {
		return nil, false, err
	}
	e, err := r.buildCall(ident(name), callArgs, sig.Params, nil, nil, false, false, false)
	if err != nil {
		return nil, false, err
	}
	return e, true, nil
}

// functionMethodTarget resolves the receiver of f.call, f.apply, or f.bind to the
// top-level function it names and that function's call signature, applying the guards
// the three share. The receiver must be a plain named function: bento models no this,
// so a method or a callable value routes elsewhere and reports ok=false, falling to
// the non-string handback. A generic or omittable-arity callee, whose direct call
// carries its own reconstruction, hands back rather than emit a call of the wrong
// arity through these methods.
func (r *Renderer) functionMethodTarget(recvNode frontend.Node, method string) (string, frontend.Signature, bool, error) {
	if recvNode.Kind() != frontend.NodeIdentifier {
		return "", frontend.Signature{}, false, nil
	}
	sym, ok := r.prog.SymbolAt(recvNode)
	if !ok || sym.Flags&frontend.SymbolFunction == 0 {
		return "", frontend.Signature{}, false, nil
	}
	name, ok := exportedField(sym.Name)
	if !ok {
		return "", frontend.Signature{}, false, nil
	}
	var sig frontend.Signature
	for _, d := range r.prog.Declarations(sym) {
		if s, ok := r.prog.SignatureAt(d); ok {
			sig = s
			break
		}
	}
	if len(sig.TypeParams) != 0 {
		return "", frontend.Signature{}, false, &NotYetLowerable{Reason: method + " on a generic function is a later slice"}
	}
	if r.funcOmittable(sym) {
		return "", frontend.Signature{}, false, &NotYetLowerable{Reason: method + " on a function with a defaulted or rest parameter is a later slice"}
	}
	return name, sig, true, nil
}

// functionBindCall recognizes f.bind(...) on a plain function value and hands it back
// with a precise reason. bind produces a new function with this and any leading
// arguments fixed, but the checker types that new function as (...args: [tuple]) => R:
// a rest parameter whose element type is the tuple of the remaining parameters. bento
// renders a rest parameter through its element type (funcTypeOf), and a tuple element
// type is not yet a lowerable Go type, so every use of the bound value, calling it or
// binding it to a name, would render that rest-over-tuple function type. The bound
// value is therefore unrenderable today no matter how bind itself lowers, so bind
// stays a clean handback rather than emit a closure the rest of the unit cannot
// consume. It routes here, ahead of the non-string receiver handback, only to name the
// real blocker: the tuple-typed rest parameter the bound value's type carries. It
// reports ok=false for a receiver that is not a plain function, leaving the general
// path to handle a method or callable-value receiver.
func (r *Renderer) functionBindCall(recvNode frontend.Node) (ast.Expr, bool, error) {
	_, _, ok, err := r.functionMethodTarget(recvNode, "bind")
	if !ok || err != nil {
		return nil, false, err
	}
	return nil, false, &NotYetLowerable{Reason: "bind produces a rest-over-tuple function type whose bound value is a later slice"}
}

// functionInvokeArgs returns the positional argument nodes an invocation through call
// or apply passes to the function, past the leading this argument. call spells the
// arguments inline, so they are the arguments after this. apply gathers them in an
// array, so they are the elements of that array; only a plain array literal is read
// here, since a runtime array's length is not known at lowering, and a spread inside
// the literal waits on the spread slice.
func (r *Renderer) functionInvokeArgs(method string, argNodes []frontend.Node) ([]frontend.Node, error) {
	if method == "call" {
		if len(argNodes) == 0 {
			return nil, nil
		}
		return argNodes[1:], nil
	}
	if len(argNodes) < 2 {
		return nil, nil
	}
	arr := argNodes[1]
	if arr.Kind() != frontend.NodeArrayLiteralExpression {
		return nil, &NotYetLowerable{Reason: "apply whose arguments are not a plain array literal is a later slice"}
	}
	elems := r.prog.Children(arr)
	for _, el := range elems {
		if el.Kind() == frontend.NodeSpreadElement {
			return nil, &NotYetLowerable{Reason: "apply over an array literal with a spread element is a later slice"}
		}
	}
	return elems, nil
}

// droppableThisArg reports whether the this argument of a .call can be dropped
// without changing what the program observes. bento's plain function ignores this,
// so the argument only sets a receiver that is never read; dropping it is faithful
// only when evaluating it has no side effect and cannot throw. A literal (including
// null) and the undefined global evaluate to a constant with no reference to a
// binding, so removing the argument changes nothing; any other form could be
// observable and keeps the handback.
func (r *Renderer) droppableThisArg(n frontend.Node) bool {
	return r.isDroppableExtraArg(n) || r.isUndefinedLiteral(n)
}

// pureRestFunc reports whether a function symbol is omittable only through a rest
// parameter, with no defaulted or optional parameter. Such a function lowers to a
// single fixed Go func shape, its fixed parameters followed by one trailing array
// field, so it fits a rest-parameter func-typed slot directly, unlike a function
// whose arity varies through a default the call site must fill.
func (r *Renderer) pureRestFunc(sym frontend.Symbol) bool {
	rest := false
	for _, d := range r.prog.Declarations(sym) {
		sig, ok := r.prog.SignatureAt(d)
		if !ok {
			continue
		}
		if sig.MinArgs < len(sig.Params) {
			return false
		}
		if sig.RestParam != nil {
			rest = true
		}
	}
	return rest
}

// isDroppableExtraArg reports whether an argument past the callee's parameter
// count can be dropped without changing what the program observes. JavaScript
// still evaluates the argument before ignoring it, so dropping it is only sound
// when the evaluation has no side effect, cannot throw, and reads nothing. A
// literal (a number, string, bigint, boolean, null, or a template with no
// substitutions) evaluates to a constant with no reference to a binding, so
// removing it changes neither state nor which locals the program uses. A plain
// identifier is pure to evaluate but is deliberately not droppable: dropping it
// removes the only read of a local, which Go rejects as a declared-and-unused
// binding, so an identifier extra hands back rather than emit Go that fails to
// compile. Any other form, a call, a property or element access that could fault
// on a nullish base, a new, an assignment, or an increment, could be observable
// and is not droppable either.
func (r *Renderer) isDroppableExtraArg(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeNumericLiteral,
		frontend.NodeStringLiteral,
		frontend.NodeBigIntLiteral,
		frontend.NodeTrueKeyword,
		frontend.NodeFalseKeyword,
		frontend.NodeNullKeyword,
		frontend.NodeNoSubstitutionTemplateLiteral:
		return true
	default:
		return false
	}
}

// gatherRest packs the trailing arguments a rest parameter collects into one
// value.NewArray of the parameter's element type, the same array a plain array
// argument would arrive as, so the callee reads its rest parameter with no special
// casing. Each argument bridges to the element type the way a positional argument
// bridges to its parameter. A spread into the rest position waits on the spread
// slice, and a rest whose element type does not lower hands back.
func (r *Renderer) gatherRest(rest frontend.Param, restNodes []frontend.Node) (ast.Expr, error) {
	elemT, ok := r.prog.ElementType(rest.Type)
	if !ok {
		return nil, &NotYetLowerable{Reason: "rest parameter whose element type does not lower yet"}
	}
	elemGo, err := r.typeExpr(elemT)
	if err != nil {
		return nil, err
	}
	hasSpread := false
	for _, a := range restNodes {
		if a.Kind() == frontend.NodeSpreadElement {
			hasSpread = true
			break
		}
	}
	if !hasSpread {
		restArgs := make([]ast.Expr, 0, len(restNodes))
		for _, a := range restNodes {
			lowered, err := r.lowerExpr(a)
			if err != nil {
				return nil, err
			}
			lowered, err = r.bridgeArg(lowered, a, elemT)
			if err != nil {
				return nil, err
			}
			restArgs = append(restArgs, lowered)
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: index(sel("value", "NewArray"), elemGo), Args: restArgs}, nil
	}
	// A spread of a user iterable into the rest position walks the iterator
	// protocol: the rest array is built by appending, splicing each iterable's
	// drained slice between the positional arguments, then wrapped as the value.Array
	// the callee reads. It is the arraySpread build over the rest element type, so a
	// spread rest and a spread array literal collect the same way.
	seedType := &ast.ArrayType{Elt: elemGo}
	var acc ast.Expr
	var pending []ast.Expr
	flush := func() {
		if len(pending) == 0 {
			return
		}
		if acc == nil {
			acc = &ast.CompositeLit{Type: seedType, Elts: pending}
		} else {
			acc = &ast.CallExpr{Fun: ident("append"), Args: append([]ast.Expr{acc}, pending...)}
		}
		pending = nil
	}
	for _, a := range restNodes {
		if a.Kind() != frontend.NodeSpreadElement {
			lowered, err := r.lowerExpr(a)
			if err != nil {
				return nil, err
			}
			lowered, err = r.bridgeArg(lowered, a, elemT)
			if err != nil {
				return nil, err
			}
			pending = append(pending, lowered)
			continue
		}
		operands := r.prog.Children(a)
		if len(operands) != 1 {
			return nil, &NotYetLowerable{Reason: "a spread argument with an unexpected shape is a later slice"}
		}
		operand := operands[0]
		// A spread of a generator into a rest parameter drains its *value.Gen coroutine
		// into a slice of its yielded element type, then splices that slice, the same
		// drain the array-literal spread takes. A generator lowers to a coroutine the
		// protocol path (symbolIteratorShape) deliberately skips, so it is drained on its
		// own path here; an iterator-helper result has a different Next and stays out,
		// matching how for...of routes generators and helpers apart.
		if r.isGeneratorIterable(operand) && !r.isIterHelperType(r.prog.TypeAt(operand)) {
			if yieldT, ok := r.generatorElemType(r.prog.TypeAt(operand)); ok {
				yieldGo, err := r.typeExpr(yieldT)
				if err != nil {
					return nil, err
				}
				same, err := sameGoType(elemGo, yieldGo)
				if err != nil {
					return nil, err
				}
				if !same {
					return nil, &NotYetLowerable{Reason: "a spread of a generator with a different element type into a rest parameter is a later slice"}
				}
				src, err := r.lowerExpr(operand)
				if err != nil {
					return nil, err
				}
				flush()
				if acc == nil {
					acc = &ast.CompositeLit{Type: seedType}
				}
				drained := r.generatorToSliceExpr(src, elemGo)
				acc = &ast.CallExpr{Fun: ident("append"), Args: []ast.Expr{acc, drained}, Ellipsis: token.Pos(1)}
				continue
			}
		}
		// A spread of a Set into a rest parameter splices its typed Members() snapshot in
		// insertion order, the same splice the array-literal spread takes. The members are
		// already the element type, so no drain or conversion is needed and the Set's
		// de-duplication rides through; a member type that does not lower to the rest
		// element's Go type hands back rather than mix kinds.
		if r.isSet(operand) {
			if setElemT, ok := r.setElem(r.prog.TypeAt(operand)); ok {
				memberGo, err := r.typeExpr(setElemT)
				if err != nil {
					return nil, err
				}
				same, err := sameGoType(elemGo, memberGo)
				if err != nil {
					return nil, err
				}
				if !same {
					return nil, &NotYetLowerable{Reason: "a spread of a set with a different element type into a rest parameter is a later slice"}
				}
				src, err := r.lowerExpr(operand)
				if err != nil {
					return nil, err
				}
				flush()
				if acc == nil {
					acc = &ast.CompositeLit{Type: seedType}
				}
				members := &ast.CallExpr{Fun: &ast.SelectorExpr{X: src, Sel: ident("Members")}}
				acc = &ast.CallExpr{Fun: ident("append"), Args: []ast.Expr{acc, members}, Ellipsis: token.Pos(1)}
				continue
			}
		}
		// A spread of a Map used directly or of any entries() call into a rest parameter
		// collects its [key, value] pairs into a fresh []Tuple the append splices, the same
		// slice the array-literal spread builds. The rest element type must be the matching
		// two-element tuple, so this is checked before the keys()/values() accessor path,
		// which an entries() call also matches on receiver.
		if members, ok, err := r.spreadCollEntries(operand, elemT, true); ok {
			if err != nil {
				return nil, err
			}
			flush()
			if acc == nil {
				acc = &ast.CompositeLit{Type: seedType}
			}
			acc = &ast.CallExpr{Fun: ident("append"), Args: []ast.Expr{acc, members}, Ellipsis: token.Pos(1)}
			continue
		}
		// A spread of a Map or Set keys()/values() call into a rest parameter splices the
		// same insertion-ordered snapshot slice the array-literal spread does: a Map's
		// keys() and values() splice Keys() and Values(), a Set's keys() and values() both
		// splice Members(). entries(), which yields a [key, value] tuple, and an unreadable
		// member type hand back.
		if recv, accessor, memberT, ok := r.collIterAccessor(operand); ok {
			memberGo, err := r.typeExpr(memberT)
			if err != nil {
				return nil, err
			}
			same, err := sameGoType(elemGo, memberGo)
			if err != nil {
				return nil, err
			}
			if !same {
				return nil, &NotYetLowerable{Reason: "a spread of a map or set iterator with a different element type into a rest parameter is a later slice"}
			}
			src, err := r.lowerExpr(recv)
			if err != nil {
				return nil, err
			}
			flush()
			if acc == nil {
				acc = &ast.CompositeLit{Type: seedType}
			}
			members := collCall(src, accessor)
			acc = &ast.CallExpr{Fun: ident("append"), Args: []ast.Expr{acc, members}, Ellipsis: token.Pos(1)}
			continue
		}
		shape, ok := r.symbolIteratorShape(r.prog.TypeAt(operand))
		if !ok {
			return nil, &NotYetLowerable{Reason: "a spread of a non-iterable into a rest parameter is a later slice"}
		}
		iterElemGo, err := r.typeExpr(shape.elem)
		if err != nil {
			return nil, err
		}
		same, err := sameGoType(elemGo, iterElemGo)
		if err != nil {
			return nil, err
		}
		if !same {
			return nil, &NotYetLowerable{Reason: "a spread of an iterable with a different element type into a rest parameter is a later slice"}
		}
		src, err := r.lowerExpr(operand)
		if err != nil {
			return nil, err
		}
		flush()
		if acc == nil {
			acc = &ast.CompositeLit{Type: seedType}
		}
		drained := r.iterableToSliceExpr(src, elemGo, shape)
		acc = &ast.CallExpr{Fun: ident("append"), Args: []ast.Expr{acc, drained}, Ellipsis: token.Pos(1)}
	}
	flush()
	if acc == nil {
		acc = &ast.CompositeLit{Type: seedType}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "ArrayFrom"), Args: []ast.Expr{acc}}, nil
}

// bridgeArg fits a lowered argument to its declared parameter type the way an
// assignment would: it boxes into an optional, wraps into a tagged union, or upcasts
// a derived instance to the embedded base, whichever the parameter asks for. It is
// shared by a provided argument and a default filled into an omitted slot, so both
// cross the parameter boundary by the same rule.
func (r *Renderer) bridgeArg(lowered ast.Expr, node frontend.Node, pt frontend.Type) (ast.Expr, error) {
	if tup, ok, err := r.arrayAssertedToTuple(lowered, node, pt); err != nil {
		return nil, err
	} else if ok {
		return tup, nil
	}
	if empty, ok, err := r.emptyArrayContextual(node, pt); err != nil {
		return nil, err
	} else if ok {
		return empty, nil
	}
	if adapted, ok, err := r.arrayToLengthShape(lowered, node, pt); err != nil {
		return nil, err
	} else if ok {
		return adapted, nil
	}
	if dyn, ok, err := r.dynArrayLiteralContextual(node, pt); err != nil {
		return nil, err
	} else if ok {
		return dyn, nil
	}
	if opt, ok, err := r.optArrayLiteralContextual(node, pt); err != nil {
		return nil, err
	} else if ok {
		return opt, nil
	}
	if fn, ok, err := r.coerceFuncValue(lowered, node, pt); err != nil {
		return nil, err
	} else if ok {
		return fn, nil
	}
	if boxed, ok, err := r.boxToOptional(lowered, node, pt); err != nil {
		return nil, err
	} else if ok {
		return boxed, nil
	}
	if wrapped, ok, err := r.wrapToUnion(lowered, node, pt); err != nil {
		return nil, err
	} else if ok {
		return wrapped, nil
	}
	// An argument bound into a parameter of a different fixed shape where either
	// shape carries an optional property cannot compile as one Go struct passed for
	// another, so it hands back the way an assignment to such a slot does. An object
	// literal argument builds at the parameter's shape before it reaches here (see
	// lowerArgAt), so this guard fires only for a non-literal source, a variable of a
	// fresh required shape passed where the optional shape is declared.
	if err := r.guardOptionalShapeCross(node, pt); err != nil {
		return nil, err
	}
	// The tuple twin of the object guard: a tuple value of one fixed signature passed
	// where the parameter declares a tuple whose optional element gives it a different
	// signature cannot compile as one Go struct passed for another. A tuple literal
	// argument builds at the parameter's shape before it reaches here (see lowerArgAt),
	// so this fires only for a non-literal source of a fresh required shape.
	if r.tupleShapeMismatch(r.prog.TypeAt(node), pt) {
		return nil, &NotYetLowerable{Reason: "a tuple with an optional element passed where a different tuple shape is declared is a later slice"}
	}
	// An argument crosses the dynamic boundary the way an assignment does: a
	// static value into a dynamic parameter boxes, and a dynamic value into a
	// static parameter coerces, so a string passed for a message?: any lands as
	// the boxed string the body reads.
	srcDyn := r.isDynamic(node)
	tgtDyn := pt.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 || r.isNarrowableBoxType(pt)
	switch {
	case srcDyn && !tgtDyn:
		return r.coerceDynamicToStaticFlags(lowered, pt.Flags)
	case !srcDyn && tgtDyn:
		return r.boxStaticToDynamic(lowered, node)
	}
	// Both sides are static here. A not-assignable value the front door tolerated
	// under code 2345 can reach the bridge, so an argument whose Go type differs from
	// the parameter hands back rather than drop a Go value into a slot of another type
	// that does not compile (see staticReprMismatch).
	if r.staticReprMismatch(node, r.prog.TypeAt(node), pt) {
		return nil, &NotYetLowerable{Reason: "an argument the checker calls not assignable whose Go type differs from the parameter is a later slice"}
	}
	return r.bridgeClassBinding(lowered, node, pt)
}

// lowerArgAt lowers one argument node against its declared parameter type. An
// object literal passed where the parameter declares a fixed shape with an optional
// property must build at that shape, not its own all-required fresh type, the same
// contextual typing a declaration and an assignment apply: the literal's own type
// interns a different Go struct than the parameter's, so passing it straight would
// emit Go that does not compile. Every other argument lowers at its own type and
// bridges to the parameter the ordinary way.
func (r *Renderer) lowerArgAt(a frontend.Node, pt frontend.Type) (ast.Expr, error) {
	if a.Kind() == frontend.NodeObjectLiteralExpression && !r.isNarrowableBoxType(pt) {
		// An object literal argument to a string-index dictionary parameter boxes into a
		// value.Value bag rather than building at a fixed shape, so it skips the
		// contextual-shape path and falls to the boxing bridge below the way a literal
		// into an any slot does.
		if shape, wrap, ok := r.contextualObjectShape(pt); ok {
			if wrap {
				return nil, &NotYetLowerable{Reason: "an object literal argument in a T | undefined optional slot is a later slice"}
			}
			return r.objectLiteralContextual(a, shape)
		}
	}
	// A function literal is assignable to a callback slot that declares more
	// parameters than the literal takes, since JavaScript lets a function ignore the
	// trailing arguments, but the Go func value must carry the slot's exact arity. When
	// the literal flowing into a func-typed slot declares fewer parameters than the
	// slot, record the slot's trailing parameters so closureParamFields pads them onto
	// the emitted func type as blank-named fields. The clear is deferred so a nested
	// literal in the body lowers against its own slot, not this one.
	if a.Kind() == frontend.NodeFunctionExpression || a.Kind() == frontend.NodeArrowFunction {
		if pad, ok := r.closurePadForSlot(a, pt); ok {
			r.closurePadParams[a] = pad
			defer delete(r.closurePadParams, a)
		}
	}
	// A tuple literal passed where the parameter declares a tuple with an optional
	// element builds at that declared shape, not its own all-required twin, the same
	// contextual build a declaration and an object-literal argument apply: the two
	// intern different Go structs, so passing the literal's own would not compile.
	if a.Kind() == frontend.NodeArrayLiteralExpression {
		if t, elems, ok := r.contextualTupleElems(pt); ok {
			return r.tupleLiteralAt(a, t, elems)
		}
	}
	lowered, err := r.lowerExpr(a)
	if err != nil {
		return nil, err
	}
	return r.bridgeArg(lowered, a, pt)
}

// closurePadForSlot reports the trailing parameters a lower-arity function literal
// must grow to match its callback slot's Go func type, and whether such padding is
// needed. The slot type must carry exactly one call signature (a plain func type or a
// callback interface); the literal keeps every parameter it declares and gains the
// slot's remaining ones as unused fields. It declines when the slot has no single call
// signature, when the literal already declares at least as many parameters as the
// slot, or when either side takes a rest parameter, whose gathering the plain arity
// pad does not model. An unused pad parameter never reads its value, so binding one is
// faithful: the literal ignores the argument exactly as the source function does.
func (r *Renderer) closurePadForSlot(a frontend.Node, pt frontend.Type) ([]frontend.Param, bool) {
	call, _ := r.prog.Signatures(pt)
	if len(call) != 1 {
		return nil, false
	}
	sig := call[0]
	if sig.RestParam != nil {
		return nil, false
	}
	own := len(r.funcParamNodes(a))
	if own >= len(sig.Params) {
		return nil, false
	}
	return sig.Params[own:], true
}

// objectMethodCall lowers a method call whose receiver is a fixed-shape object
// and whose named member is a function-valued property, so a member closure is
// called through the Go struct field it interned to: recv.SameValue(args). It
// reports ok=false when the receiver is not such an object or the member is not a
// function property, so methodCall falls through to its string path; the member's
// own call signature drives the argument bridging, the same rule a named call
// applies. A data property (a non-function field) is not callable and stays for
// the read path.
func (r *Renderer) objectMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, bool, error) {
	// A boxed receiver, the exports object a require binds as a value.Value, carries
	// the module's inferred object shape yet has no Go struct to select a func field
	// off. Declining here keeps such a call off the struct-field path and lets it reach
	// the dynamic dispatch below, which reads the method with a runtime Get and Call.
	if r.isDynamic(recvNode) {
		return nil, false, nil
	}
	t := r.prog.TypeAt(recvNode)
	if t.Flags&frontend.TypeObject == 0 {
		return nil, false, nil
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return nil, false, nil
	}
	sp, ok := r.shapeProp(t, method)
	if !ok {
		return nil, false, nil
	}
	call, _ := r.prog.Signatures(sp.Type)
	if len(call) != 1 {
		return nil, false, nil
	}
	field, ok := exportedField(method)
	if !ok {
		return nil, false, &NotYetLowerable{Reason: "method name ." + method + " is not a Go identifier"}
	}
	// Interning the receiver's shape declares the func field this call selects, so
	// the call and the struct agree on the field name and type.
	if _, err := r.decls.internStruct(r, t); err != nil {
		return nil, false, err
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, false, err
	}
	callee := &ast.SelectorExpr{X: recv, Sel: ident(field)}
	e, err := r.buildCall(callee, argNodes, call[0].Params, call[0].RestParam, nil, false, false, false)
	if err != nil {
		return nil, false, err
	}
	return e, true, nil
}

// dynamicCall lowers fn(args) when the callee's own type is dynamic, so its shape
// is known only at runtime: the callee lowers to a value.Value and the call goes
// through the box's Call method with every argument boxed, the runtime dispatch
// that mirrors a dynamic member read. The result is itself a value.Value, the any
// type the checker gives a call whose callee is dynamic, so it flows on as a boxed
// value with no further coercion here. A spread argument waits on the spread slice.
func (r *Renderer) dynamicCall(calleeNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	callee, err := r.lowerExpr(calleeNode)
	if err != nil {
		return nil, err
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		if a.Kind() == frontend.NodeSpreadElement {
			return nil, &NotYetLowerable{Reason: "a spread argument in a dynamic call is a later slice"}
		}
		boxed, err := r.boxOperand(a)
		if err != nil {
			return nil, err
		}
		args = append(args, boxed)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: callee, Sel: ident("Call")}, Args: args}, nil
}

// functionValueCallee lowers a callee expression that is not a named function to
// the Go func value it evaluates to: a binding of function type, or a larger
// expression whose type is a function (an array element fs[0], the result of
// another call mk(5), a parenthesized arrow). The call proceeds only when the
// callee's type lowers to a clean Go func type, the same shape a function
// declaration takes, so a callable object, an overload set, or a generic function
// value hands back here instead of emitting a call on a wrong Go type.
func (r *Renderer) functionValueCallee(calleeNode frontend.Node) (ast.Expr, error) {
	if ft, ok, err := r.renderFuncType(r.prog.TypeAt(calleeNode)); err != nil {
		return nil, err
	} else if !ok || ft == nil {
		return nil, &NotYetLowerable{Reason: "call to a callee that is not a top-level function or a plain function value is a later slice"}
	}
	return r.lowerExpr(calleeNode)
}

// methodCall lowers a call whose callee is a member expression. The only
// receivers covered so far are strings, whose methods map to value.BStr methods,
// so the receiver must type as string and the method must be one bento maps.
// Every string method covered here takes number arguments, so a non-number
// argument hands back rather than mistyping the Go call. A method on any other
// receiver, or an unmapped string method, is its own later slice.
func (r *Renderer) methodCall(callee frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(callee)
	if len(kids) != 2 {
		return nil, &NotYetLowerable{Reason: "method callee did not expose a receiver and a method name"}
	}
	recvNode, method := kids[0], r.prog.Text(kids[1])
	// A method on a caught error is one of the error-identity calls of section 7.7:
	// err.is(sentinel) lowers to errors.Is against the Go error the caught error
	// carries. It routes first because the checker types a catch binding unknown, so
	// the receiver-value paths below would not recognize it, and because the argument
	// is a go: sentinel reference rather than a bento value. Only a receiver the
	// lowerer bound as a caught error takes this path.
	if recvNode.Kind() == frontend.NodeIdentifier {
		if name, ok := localName(r.prog.Text(recvNode)); ok && r.errorLocals[name] {
			return r.errorMethodCall(name, method, argNodes)
		}
	}
	// process.stdout.write(s) and process.stderr.write(s) are the process output
	// streams. The receiver is not a value, it is the ambient stream, so the call
	// lowers to a value write helper rather than a method on a runtime object.
	if stream, ok := r.processStream(recvNode); ok {
		return r.processStreamCall(stream, method, argNodes)
	}
	// process.on('exit', fn) registers a run-at-exit callback, a call on the global
	// process object rather than a value receiver, so it lowers to the value exit
	// registry rather than a method on a runtime object. It routes here after the
	// stream check, whose receiver is process.stdout, before the string gate below.
	if r.isGlobalRef(recvNode, "process") {
		return r.processCall(method, argNodes)
	}
	// console.log(...) and friends are calls on the global console, not a value
	// receiver, so they lower to the value console helpers rather than a method on a
	// runtime object. This is the print path a developer reaches for by default.
	if r.isGlobalRef(recvNode, "console") {
		return r.consoleCall(method, argNodes)
	}
	// performance.now() is a call on the global performance object, not a value
	// receiver, so it lowers to the value clock helper rather than a method on a
	// runtime object. It is the timer a workload brackets its hot loop with to
	// report an in-process compute number apart from process startup.
	if r.isGlobalRef(recvNode, "performance") {
		return r.performanceCall(method, argNodes)
	}
	// Math.floor(x) and friends are calls on the global Math namespace, not a
	// value receiver, so they lower to the Go math package rather than a method.
	if r.isGlobalRef(recvNode, "Math") {
		return r.mathCall(method, argNodes)
	}
	// Atomics.load(ta, i) and friends are calls on the global Atomics namespace, not a
	// value receiver, so they lower to the value atomic helpers over the typed array
	// they take rather than a method on Atomics.
	if r.isGlobalRef(recvNode, "Atomics") {
		return r.atomicsCall(method, argNodes)
	}
	// Number.isInteger(x) and friends are static calls on the global Number, which
	// lower to value package predicates.
	if r.isGlobalRef(recvNode, "Number") {
		return r.numberCall(method, argNodes)
	}
	// String.fromCharCode(...) is a static call on the global String constructor,
	// not a method on a string value, so it lowers to a value constructor before
	// the string-method path below, which expects a string receiver.
	if r.isGlobalRef(recvNode, "String") {
		return r.stringStaticCall(method, argNodes)
	}
	// JSON.stringify(x) is a static call on the global JSON namespace, not a method
	// on a value, so it lowers to the value JSON serializer before the
	// receiver-value paths below. JSON.parse waits on the dynamic value box.
	if r.isGlobalRef(recvNode, "JSON") {
		return r.jsonCall(method, argNodes)
	}
	// Object.keys(o) and friends are static calls on the global Object namespace,
	// not a method on a value, so they lower to a value constructor before the
	// receiver-value paths below.
	if r.isGlobalRef(recvNode, "Object") {
		return r.objectCall(method, argNodes)
	}
	// BigInt.asIntN(bits, x) and BigInt.asUintN(bits, x) are static calls on the
	// global BigInt, not a method on a value, so they lower to the value wrap
	// helpers. BigInt as a namespace routes here; BigInt(x) as a conversion function
	// is handled at the bare-call path above.
	if r.isGlobalRef(recvNode, "BigInt") {
		return r.bigIntStaticCall(method, argNodes)
	}
	// Map.groupBy(items, cb) is a static call on the global Map constructor, not a
	// method on a map value, so it lowers to a map builder before the receiver-value
	// paths below. new Map(...) as a construction is handled at the new-expression
	// path; the namespace form routes here.
	if r.isGlobalRef(recvNode, "Map") {
		return r.mapStaticCall(method, argNodes)
	}
	// Symbol.for(key) and Symbol.keyFor(sym) are static calls on the ambient Symbol
	// global that read and write the global symbol registry, not methods on a symbol
	// value, so they lower to the value registry helpers before the receiver-value
	// paths below. Symbol(desc) as a construction is handled at the bare-call path.
	if r.isGlobalRef(recvNode, "Symbol") {
		return r.symbolStaticCall(method, argNodes)
	}
	// Reflect.get(o, k) and its siblings are static calls on the ambient Reflect
	// global, the reflective layer over the object model, not methods on a value, so
	// they lower to the value Reflect helpers before the receiver-value paths below.
	if r.isGlobalRef(recvNode, "Reflect") {
		return r.reflectCall(method, argNodes)
	}
	// Proxy.revocable(target, handler) is a static call on the ambient Proxy global
	// that pairs a proxy with a revoke function, not a method on a value, so it lowers
	// to the value ProxyRevocable helper before the receiver-value paths below.
	if r.isGlobalRef(recvNode, "Proxy") {
		return r.proxyStaticCall(method, argNodes)
	}
	// Iterator.from(x) is a static call on the ambient Iterator global that wraps an
	// iterable as an iterator helper, not a method on a value, so it lowers to the value
	// IterFrom helper before the receiver-value paths below.
	if r.isGlobalRef(recvNode, "Iterator") {
		return r.iteratorStaticCall(method, argNodes)
	}
	// A static call on a Temporal namespace member, Temporal.PlainDate.compare(a, b)
	// or Temporal.PlainDate.from(x), reads as a two-level access whose object is
	// itself Temporal.<Type>: the receiver here is the property access Temporal.PlainDate,
	// not a plain identifier, so it routes before the identifier-only class-name path
	// below. temporalStaticCall dispatches on the type name and hands back any Temporal
	// type this slice does not host.
	if recvNode.Kind() == frontend.NodePropertyAccessExpression {
		if parts := r.prog.Children(recvNode); len(parts) == 2 && r.isGlobalRef(parts[0], "Temporal") {
			return r.temporalStaticCall(r.prog.Text(parts[1]), method, argNodes)
		}
	}
	// A static call A.m(...) lowers to the package function the static method
	// became. The class name's type shares the class symbol an instance walks
	// to, so this routes before the instance path below.
	if recvNode.Kind() == frontend.NodeIdentifier {
		if info, ok := r.classNameRef(recvNode); ok {
			return r.staticMethodCall(info, method, argNodes)
		}
	}
	// String.fromCharCode and String.fromCodePoint invoked through .call or .apply are
	// static builtins reached indirectly, not a method on a value, so they lower to the
	// same variadic value constructor the direct static call takes, an .apply over a
	// runtime array spreading it into the call. It routes before the receiver-value
	// paths below, which would reject the String.<static> receiver as a non-string.
	if e, ok, err := r.stringStaticMethodCall(recvNode, method, argNodes); ok || err != nil {
		return e, err
	}
	// call on a plain function value invokes it with an explicit this and the
	// remaining positional arguments. bento's plain functions take no this (a body
	// that reads this hands back at its declaration), so f.call(thisArg, a, b) is the
	// direct call F(a, b) with the this argument dropped once its evaluation is known
	// to be pure. It routes before the receiver-value paths below, which would reject
	// a function receiver as a non-string later slice.
	if e, ok, err := r.functionMethodCall(recvNode, method, argNodes); ok || err != nil {
		return e, err
	}
	// super.m(...) inside a static method calls the base class's static method,
	// which lowered to a package function, so it routes to the same static call
	// A.m(...) takes rather than to the instance path: a static body has no
	// receiver, and the base's static method is a function, not a method on a
	// value.
	if recvNode.Kind() == frontend.NodeSuperKeyword && r.staticClass != nil && r.staticClass.base != nil {
		return r.staticMethodCall(r.staticClass.base, method, argNodes)
	}
	// A method on a class instance, this.m(...) inside a class body or p.m(...)
	// on an instance, lowers to the Go method the class declared. It routes
	// before the array, map, and string paths so a class receiver is dispatched
	// as the class it is rather than re-derived structurally.
	if info, ok := r.classReceiver(recvNode); ok {
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return r.classMethodCall(info, recv, method, argNodes, recvNode.Kind() == frontend.NodeSuperKeyword)
	}
	// A method on a tuple receiver: a tuple is an array subtype, so an array method
	// like map borrowed on it materializes the tuple as a value.Array over its element
	// union and dispatches the array method on that. It routes before the array path,
	// whose arrayElem check answers false for a tuple (the checker reports a tuple's
	// positions through TupleElements, not one array element type). A method the tuple
	// path does not host reports handled=false and falls through unchanged.
	if elems, ok := r.prog.TupleElements(r.prog.TypeAt(recvNode)); ok {
		if expr, handled, err := r.tupleArrayMethodCall(recvNode, method, argNodes, elems); handled || err != nil {
			return expr, err
		}
	}
	// A method on an array receiver lowers to a value.Array method. This routes
	// before the primitive and string paths, which expect a number, boolean, or
	// string receiver an array is not.
	if _, ok := r.arrayElem(recvNode); ok {
		return r.arrayMethodCall(recvNode, method, argNodes)
	}
	// A method on a numeric typed-array receiver lowers to a value.TypedArray
	// method. It routes right after the array path, since a typed array shares the
	// copy and search shapes but runs over a view that clamps to its length, and
	// before the Map, Set, and primitive paths, which expect a receiver a typed
	// array is not. Only the generic *value.TypedArray[T] family routes here:
	// Uint8Array has its own []byte representation without these view methods, and
	// the bigint typed arrays are a separate value type whose methods are not wired,
	// so both fall through to the hand-back below rather than emit a call to a method
	// they do not have.
	if r.genericTypedArray(recvNode) {
		return r.typedArrayMethodCall(recvNode, method, argNodes)
	}
	// A method on an ArrayBuffer receiver lowers to a value.ArrayBuffer method: the
	// transfer pair that moves the bytes to a fresh buffer and detaches this one, and
	// the resize surface. It routes here after the view paths, since a buffer is the
	// backing store a view aliases rather than a view itself, and before the Map, Set,
	// and primitive paths, which expect a receiver a buffer is not.
	if r.isArrayBuffer(recvNode) {
		return r.arrayBufferMethodCall(recvNode, method, argNodes)
	}
	// A method on a SharedArrayBuffer receiver lowers to a value.SharedArrayBuffer
	// method: the grow that only enlarges the shared run and the slice that copies a
	// span into a fresh shared buffer. It routes alongside the ArrayBuffer path, after
	// the view paths and before the Map, Set, and primitive paths, which expect a
	// receiver a shared buffer is not.
	if r.isSharedArrayBuffer(recvNode) {
		return r.sharedArrayBufferMethodCall(recvNode, method, argNodes)
	}
	// A method on a DataView receiver lowers to a value.DataView getter or setter (25
	// §25.3). It routes here alongside the other view paths, after the typed-array and
	// buffer checks and before the Map, Set, and primitive paths, which expect a
	// receiver a DataView is not.
	if r.isDataView(recvNode) {
		return r.dataViewMethodCall(recvNode, method, argNodes)
	}
	// A method on a RegExp receiver, re.exec(s) or re.test(s), lowers to a value.RegExp
	// method (22 §22.2.7). It routes here alongside the other exotic-type paths, before
	// the Map, Set, and primitive paths, which expect a receiver a RegExp is not.
	if r.isRegExp(recvNode) {
		return r.regExpMethodCall(recvNode, method, argNodes)
	}
	// A method on a Temporal.PlainDate receiver lowers to a value.PlainDate method
	// (Temporal §3): equals, toString, and toJSON over the ISO calendar. It routes
	// here alongside the other exotic-type paths, before the Map, Set, and primitive
	// paths, which expect a receiver a PlainDate is not. The arithmetic and conversion
	// methods hand back with a named reason.
	if r.isPlainDate(recvNode) {
		return r.plainDateMethodCall(recvNode, method, argNodes)
	}
	if r.isPlainTime(recvNode) {
		return r.plainTimeMethodCall(recvNode, method, argNodes)
	}
	if r.isPlainDateTime(recvNode) {
		return r.plainDateTimeMethodCall(recvNode, method, argNodes)
	}
	if r.isDuration(recvNode) {
		return r.durationMethodCall(recvNode, method, argNodes)
	}
	if r.isPlainYearMonth(recvNode) {
		return r.plainYearMonthMethodCall(recvNode, method, argNodes)
	}
	if r.isPlainMonthDay(recvNode) {
		return r.plainMonthDayMethodCall(recvNode, method, argNodes)
	}
	if r.isInstant(recvNode) {
		return r.instantMethodCall(recvNode, method, argNodes)
	}
	if r.isZonedDateTime(recvNode) {
		return r.zonedDateTimeMethodCall(recvNode, method, argNodes)
	}
	// A register or unregister call on a FinalizationRegistry receiver lowers to the
	// value.FinalizationRegistry surface (25 §26.2). It routes alongside the other weak
	// paths, before the primitive and string paths a registry receiver is not.
	if r.isFinalizationRegistry(recvNode) {
		return r.finalizationRegistryMethodCall(recvNode, method, argNodes)
	}
	// A weakRef.deref() call lowers to a value.WeakRef method (25 §26.1). It routes
	// alongside the other weak-collection paths, before the primitive and string paths,
	// which expect a number, boolean, or string receiver a WeakRef is not.
	if r.isWeakRef(recvNode) {
		return r.weakRefMethodCall(recvNode, method, argNodes)
	}
	// A method on a WeakMap receiver lowers to a value.WeakMap method (25 §24.3). It
	// routes before the Map path: a WeakMap's fingerprint has no size, so isMap never
	// matches it, but routing it first keeps the two collection dispatches adjacent and
	// makes the intent plain.
	if r.isWeakMap(recvNode) {
		return r.weakMapMethodCall(recvNode, method, argNodes)
	}
	// A method on a Map receiver lowers to a value.Map method (section 6.5). This
	// routes before the primitive and string paths, which expect a number, boolean,
	// or string receiver a map is not.
	if r.isMap(recvNode) {
		return r.mapMethodCall(recvNode, method, argNodes)
	}
	// A method on a WeakSet receiver lowers to a value.WeakSet method (25 §24.4). Like
	// the WeakMap path it routes before the Set path, whose fingerprint requires a size
	// a WeakSet has not, keeping the two collection dispatches adjacent.
	if r.isWeakSet(recvNode) {
		return r.weakSetMethodCall(recvNode, method, argNodes)
	}
	// A method on a Set receiver lowers to a value.Set method (section 6.5). Like the
	// Map path it routes before the primitive and string paths, which expect a number,
	// boolean, or string receiver a set is not.
	if r.isSet(recvNode) {
		return r.setMethodCall(recvNode, method, argNodes)
	}
	// A method on a Promise receiver, p.then(cb) or p.catch(cb), lowers to a
	// value.Promise method. Like Map and Set it routes before the primitive and
	// string paths, which expect a number, boolean, or string receiver a promise is
	// not.
	if r.isPromise(recvNode) {
		return r.promiseMethodCall(recvNode, method, argNodes)
	}
	// An iterator helper, arr.values().map(f) or Iterator.from(x).filter(g), lowers to
	// the value.Iter* free function over the receiver's Next. It routes before the
	// generator drive because a helper result is typed IteratorObject, one of the wider
	// iterator names generatorMethodCall also claims, and before the array iterator drive
	// so a helper method on an ArrayIterator receiver is caught here rather than handed
	// back as an unknown array-iterator method. A receiver that is neither an
	// IteratorObject nor an ArrayIterator returns ok false and falls through, so a real
	// generator (typed Generator) still reaches generatorMethodCall below.
	if e, ok, err := r.iterHelperMethodCall(recvNode, method, argNodes); ok || err != nil {
		return e, err
	}
	// A manual drive of a generator, it.next(v), lowers to the runtime helper that packs
	// the { value, done } result. Like Map, Set, and Promise it routes before the
	// primitive and string paths, which expect a number, boolean, or string receiver a
	// generator is not. A non-generator receiver returns ok false and falls through.
	if e, ok, err := r.generatorMethodCall(recvNode, method, argNodes); ok || err != nil {
		return e, err
	}
	// A manual drive of an async generator, it.next(v), lowers to the pull that returns
	// the promise the consumer awaits. It routes next to the plain generator drive, since
	// an async generator receiver carries the AsyncGenerator element type rather than the
	// Generator one; a non-async-generator receiver returns ok false and falls through.
	if e, ok, err := r.asyncGeneratorMethodCall(recvNode, method, argNodes); ok || err != nil {
		return e, err
	}
	// A manual drive of an array iterator, it.next(), lowers to the runtime's Next,
	// which packs the { value, done } result. Like the generator drives it routes
	// before the primitive and string paths, which expect a number, boolean, or
	// string receiver an array iterator is not; a non-iterator receiver falls through.
	if e, ok, err := r.arrayIterMethodCall(recvNode, method, argNodes); ok || err != nil {
		return e, err
	}
	// A manual drive of a stored Map or Set iterator, it.next(), lowers to the runtime's
	// Next over the *value.ArrayIter its construction minted, the same { value, done }
	// pack the array iterator drive returns. Like the other iterator drives it routes
	// before the primitive and string paths, which expect a number, boolean, or string
	// receiver a collection iterator is not; a non-iterator receiver falls through.
	if e, ok, err := r.collIterMethodCall(recvNode, method, argNodes); ok || err != nil {
		return e, err
	}
	// toString and valueOf on a number or a boolean value are the first methods on
	// a non-string receiver: they lower to the same coercion a String() call or a
	// bare use would take, so they route here before the string-method path.
	if r.isNumber(recvNode) || r.isBool(recvNode) {
		return r.primitiveValueCall(recvNode, method, argNodes)
	}
	// toString and valueOf on a bigint value: valueOf is identity, so it lowers to the
	// receiver with no call, and toString renders the digits, in base 10 by default or
	// the base a radix argument names. They route here before the string-method path,
	// which expects a string receiver a bigint is not.
	if r.isBigInt(recvNode) {
		return r.bigIntValueCall(recvNode, method, argNodes)
	}
	// Object.prototype.toString.call(x) is the class-tag idiom: it borrows the
	// base object toString and applies it to any value to read its internal class
	// as "[object Type]". The receiver is the function Object.prototype.toString,
	// not a value, so it routes here before the non-string gate below, which would
	// otherwise reject a function receiver as a later slice.
	if r.isObjectProtoToString(recvNode) {
		return r.objectProtoToStringCall(method, argNodes)
	}
	// Array.prototype.map.call(arrayLike, String) is the array-format idiom: it
	// borrows the base array map and applies it to any array-like to stringify each
	// element. The receiver is the function Array.prototype.map, not a value, so it
	// routes here before the non-string gate below, which would otherwise reject a
	// function receiver as a later slice. The assert prelude's compareArray.format
	// is the reach that makes this worth lowering.
	if r.isArrayProtoMap(recvNode) {
		if e, err := r.arrayProtoMapCall(method, argNodes); err == nil {
			return e, nil
		}
	}
	// Array.prototype.<m>.call(arrayLike, ...) borrows an array method onto a generic
	// receiver, a plain object with a length property and integer keys standing in for
	// an array. The receiver is the function Array.prototype.<m>, not a value, so it
	// routes here before the non-string gate below, which would reject a function
	// receiver as a later slice. The map+String prelude form above is handled first;
	// every other borrowed method runs the generic-receiver runtime.
	if name, ok := r.arrayProtoMethodName(recvNode); ok {
		return r.arrayProtoBorrowedCall(name, method, argNodes)
	}
	// String.prototype.<m>.call(recv, ...) borrows a string method onto a generic
	// receiver, coercing the receiver to a string first the way the spec's ToString
	// does. The receiver is the function String.prototype.<m>, not a value, so it
	// routes here before the non-string gate below, which would reject a function
	// receiver as a later slice.
	if name, ok := r.stringProtoMethodName(recvNode); ok {
		return r.stringProtoBorrowedCall(name, method, argNodes)
	}
	// A method on a fixed-shape object receiver whose property is itself a function
	// (assert.sameValue(...) on a callable object, or a plain object that holds a
	// closure in a field) calls through the Go struct field: recv.SameValue(args).
	// It routes here, after the primitive and collection receivers, so a member
	// closure is called as the func field it interned to rather than falling to the
	// string-method gate below.
	if e, ok, err := r.objectMethodCall(recvNode, method, argNodes); ok || err != nil {
		return e, err
	}
	// toString on a boxed receiver dispatches at runtime: the value carries its
	// kind, so recv.ToStringMethod() runs the toString the receiver's prototype
	// installs, throwing on undefined and null the way the language does. It routes
	// here before the string gate below, which would otherwise reject a boxed
	// receiver as a later slice. compareArray in the test262 prelude calls
	// message.toString() where a typeof guard narrowed the any-typed message to
	// symbol, so the binding is still a box the checker no longer types dynamic.
	if r.isBoxedValue(recvNode) && method == "toString" {
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "dynamic .toString with an argument is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		call := &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ToStringMethod")}}
		// A receiver typed any keeps the boxed result: any.toString() is itself any,
		// so the call node stays a value the consumer takes. A receiver whose kind the
		// checker knows types the call string, so the boxed result unboxes to its BStr
		// to match the string the checker promised the consumer. The gate is the
		// receiver's declared type, not its storage: a symbol binding is a boxed value
		// yet the checker types it symbol, so its toString() unboxes like any other
		// known kind, only a genuinely any or unknown receiver stays boxed.
		if r.prog.TypeAt(recvNode).Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
			return call, nil
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: call, Sel: ident("AsString")}}, nil
	}
	// valueOf on a boxed receiver dispatches at runtime the way toString does:
	// Object.prototype.valueOf returns the receiver itself, and the primitive wrappers
	// return the primitive they box, so recv.ValueOfMethod() answers the receiver value
	// unchanged and throws on undefined and null the way the language does. It routes
	// here before the string gate below, which would otherwise reject a boxed receiver.
	// The result stays boxed: a boxed receiver is typed any (valueOf is any) or symbol
	// (valueOf is symbol), both of which the consumer takes as a value.Value, so unlike
	// the toString case there is no known primitive kind to unbox to.
	if r.isBoxedValue(recvNode) && method == "valueOf" {
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "dynamic .valueOf with an argument is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ValueOfMethod")}}, nil
	}
	// A method the special cases above did not claim, called on a boxed receiver,
	// dispatches through the runtime: the member m.fn is read with a dynamic Get and
	// the result invoked with Call, the shape m.fn(x) takes where m = require('./mod')
	// binds a module's exports object as a box. It routes here after toString and
	// valueOf, which the value model answers with their own dedicated helpers, and
	// before the string gate below, which would reject a boxed receiver.
	if r.isDynamic(recvNode) {
		return r.dynamicCall(callee, argNodes)
	}
	if !r.isString(recvNode) {
		return nil, &NotYetLowerable{Reason: "method call on a non-string receiver is a later slice"}
	}
	// toString and valueOf on a string are identity: both return the string itself,
	// so they lower to the receiver expression with no call at all, which is both
	// exact and the most readable Go. Neither takes an argument, so a call with any
	// argument hands back rather than emitting a call the BStr type does not have.
	if method == "toString" || method == "valueOf" {
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "string ." + method + " with an argument is a later slice"}
		}
		return r.lowerExpr(recvNode)
	}
	// match, search, replace, replaceAll, and split with a regexp pattern or
	// separator route to the value.RegExp engine. replace and replaceAll keep the
	// plain-literal fast path first: a metacharacter-free pattern is exactly the byte
	// search the value replace methods do, so it stays that direct call, and only a
	// pattern that is not plain, or a flag the fast path does not model, falls through
	// to the engine. A string argument is not a regexp here, so it drops through to
	// the ordinary string-method dispatch below.
	switch method {
	case "match", "search", "replace", "replaceAll", "split":
		if len(argNodes) >= 1 && r.isRegExpArg(argNodes[0]) {
			if method == "replace" || method == "replaceAll" {
				if pattern, flags, ok := r.regexLiteralArg(argNodes[0]); ok && isPlainRegexPattern(pattern) {
					if expr, err := r.regexReplaceCall(recvNode, method, pattern, flags, argNodes); err == nil {
						return expr, nil
					}
				}
			}
			return r.stringRegExpMethodCall(recvNode, method, argNodes)
		}
	}
	goName, params, minArgs, variadic, ok := stringMethod(method)
	if !ok {
		return nil, &NotYetLowerable{Reason: "string method ." + method + " is a later slice"}
	}
	if len(argNodes) < minArgs || (!variadic && len(argNodes) > len(params)) {
		return nil, &NotYetLowerable{Reason: "string method ." + method + " with this argument count is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for i, a := range argNodes {
		// A variadic method repeats its last argument kind for every argument past
		// the declared list, so concat's trailing string arguments all check as
		// strings.
		idx := i
		if idx >= len(params) {
			idx = len(params) - 1
		}
		kind := params[idx]
		if !r.argHasKind(a, kind) {
			return nil, &NotYetLowerable{Reason: "string method ." + method + " with an argument of the wrong type is a later slice"}
		}
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}, Args: args}, nil
}

// performanceCall lowers a call on the global performance object. Only now() is
// covered: it takes no arguments and returns a number of milliseconds, so it maps
// to value.PerformanceNow with an exact zero-argument check; anything else hands
// back. Like Math and the other namespace globals, the performance receiver is not
// lowered to a value, since it is the ambient timer, not a runtime object.
func (r *Renderer) performanceCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "now" {
		return nil, &NotYetLowerable{Reason: "performance." + method + " is a later slice"}
	}
	if len(argNodes) != 0 {
		return nil, &NotYetLowerable{Reason: "performance.now with arguments is a later slice"}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "PerformanceNow")}, nil
}

// mathCall lowers a call on the global Math namespace to the matching function in
// the Go math package. Every Math method covered here takes numbers and returns a
// number, so the argument count must match exactly and each argument must type as
// number; anything else hands back rather than emitting a mistyped call. The
// receiver is not lowered, since Math is a namespace, not a value: it becomes the
// math package qualifier.
func (r *Renderer) mathCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	pkg, goName, minArity, maxArity, ok := mathMethod(method)
	if !ok {
		return nil, &NotYetLowerable{Reason: "Math." + method + " is a later slice"}
	}
	if len(argNodes) < minArity || (maxArity >= 0 && len(argNodes) > maxArity) {
		return nil, &NotYetLowerable{Reason: "Math." + method + " with this argument count is a later slice"}
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		if !r.isNumber(a) {
			return nil, &NotYetLowerable{Reason: "Math." + method + " with a non-number argument is a later slice"}
		}
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	if pkg == valuePkg {
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", goName), Args: args}, nil
	}
	r.requireImport("math")
	return &ast.CallExpr{Fun: sel("math", goName), Args: args}, nil
}

// numberCall lowers a static call on the global Number namespace to the matching
// predicate in the value package. Each covered method takes one number and
// returns a boolean, so the argument count must be one and the argument must type
// as number; anything else hands back. Like Math, the Number receiver is not
// lowered to a value, since it is a namespace, not a runtime object.
func (r *Renderer) numberCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	// Number.parseInt and Number.parseFloat are the same function objects as the
	// global parseInt and parseFloat by specification, so they lower through the
	// exact same paths rather than a separate mapping. They take a string and
	// return a number, unlike the predicates below that take a number.
	switch method {
	case "parseInt":
		return r.parseIntCall(argNodes)
	case "parseFloat":
		return r.parseFloatCall(argNodes)
	}
	goName, ok := numberMethod(method)
	if !ok {
		return nil, &NotYetLowerable{Reason: "Number." + method + " is a later slice"}
	}
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Number." + method + " with this argument count is a later slice"}
	}
	if !r.isNumber(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Number." + method + " on a non-number argument is a later slice"}
	}
	arg, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", goName), Args: []ast.Expr{arg}}, nil
}

// bigIntValueCall lowers toString and valueOf on a bigint value. valueOf is the
// identity, so it lowers to the receiver with no call. toString with no argument
// renders the decimal digits through value.BigIntToString, the same digits String(b)
// produces; toString(radix) renders the digits in the named base through
// value.BigIntToStringRadix, which validates the radix and throws the RangeError a
// base outside [2, 36] raises. Anything else on a bigint hands back.
func (r *Renderer) bigIntValueCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	switch method {
	case "valueOf":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "bigint valueOf with an argument is a later slice"}
		}
		return recv, nil
	case "toString":
		r.requireImport(valuePkg)
		if len(argNodes) == 0 {
			return &ast.CallExpr{Fun: sel("value", "BigIntToString"), Args: []ast.Expr{recv}}, nil
		}
		if len(argNodes) != 1 || !r.isNumber(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "bigint toString with this argument is a later slice"}
		}
		radix, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: sel("value", "BigIntToStringRadix"), Args: []ast.Expr{recv, radix}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "bigint method ." + method + " is a later slice"}
	}
}

// bigIntStaticCall lowers a static call on the global BigInt namespace. asIntN and
// asUintN take a bit width and a bigint and wrap the bigint into a fixed-width
// signed or unsigned integer, so each maps to its value wrap helper. The width is a
// number and the value a bigint, so the first argument lowers as a number and the
// second as a bigint; a call whose arguments do not have those types hands back.
func (r *Renderer) bigIntStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	var goName string
	switch method {
	case "asIntN":
		goName = "BigIntAsIntN"
	case "asUintN":
		goName = "BigIntAsUintN"
	default:
		return nil, &NotYetLowerable{Reason: "BigInt." + method + " is a later slice"}
	}
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "BigInt." + method + " with this argument count is a later slice"}
	}
	if !r.isNumber(argNodes[0]) || !r.isBigInt(argNodes[1]) {
		return nil, &NotYetLowerable{Reason: "BigInt." + method + " expects a number width and a bigint value"}
	}
	bits, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	x, err := r.lowerExpr(argNodes[1])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", goName), Args: []ast.Expr{bits, x}}, nil
}

// stringStaticCall lowers a static call on the global String constructor.
// fromCharCode takes any number of number arguments, coerces each to a UTF-16
// code unit, and returns a string, so it maps to the variadic value.FromCharCode.
// fromCodePoint is its code-point sibling: each argument is a full Unicode code
// point rather than a code unit, an astral point becomes a surrogate pair, and a
// code point outside [0, 0x10FFFF] throws a RangeError, so it maps to the
// variadic value.FromCodePoint. Like Math and Number, String is a namespace on
// this path, not a value, so the receiver is not lowered.
func (r *Renderer) stringStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	var goName string
	switch method {
	case "fromCharCode":
		goName = "FromCharCode"
	case "fromCodePoint":
		goName = "FromCodePoint"
	default:
		return nil, &NotYetLowerable{Reason: "String." + method + " is a later slice"}
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		if !r.isNumber(a) {
			return nil, &NotYetLowerable{Reason: "String." + method + " with a non-number argument is a later slice"}
		}
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", goName), Args: args}, nil
}

// stringStaticMethodCall lowers String.fromCharCode and String.fromCodePoint invoked
// through .call or .apply, reporting ok=false when the receiver is not one of those two
// statics so the caller falls through to the general method paths. Both statics are
// variadic, so .call spells the arguments after the dropped this inline exactly as a
// direct String.<static>(...) call would, and .apply gathers them: a plain array literal
// reuses the direct-call path over its elements, and a runtime number array spreads into
// the Go variadic call, the form a spread literal cannot express. This is the shape
// regExpUtils.js's buildString reaches for, String.fromCodePoint.apply(null, codePoints),
// to turn a code-point array of unknown length into a string. The this argument only sets
// a receiver these statics never read, so it drops when evaluating it is pure.
func (r *Renderer) stringStaticMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, bool, error) {
	if method != "call" && method != "apply" {
		return nil, false, nil
	}
	if recvNode.Kind() != frontend.NodePropertyAccessExpression {
		return nil, false, nil
	}
	kids := r.prog.Children(recvNode)
	if len(kids) != 2 || !r.isGlobalRef(kids[0], "String") {
		return nil, false, nil
	}
	static := r.prog.Text(kids[1])
	if static != "fromCharCode" && static != "fromCodePoint" {
		return nil, false, nil
	}
	if len(argNodes) > 0 && !r.droppableThisArg(argNodes[0]) {
		return nil, false, &NotYetLowerable{Reason: "String." + static + " through " + method + " with a this argument that is not a plain value is a later slice"}
	}
	if method == "call" {
		var rest []frontend.Node
		if len(argNodes) > 1 {
			rest = argNodes[1:]
		}
		e, err := r.stringStaticCall(static, rest)
		return e, err == nil, err
	}
	// apply gathers the arguments in a single array. A missing array behaves as no
	// arguments; a literal reuses the direct-call path over its elements.
	if len(argNodes) < 2 {
		e, err := r.stringStaticCall(static, nil)
		return e, err == nil, err
	}
	arr := argNodes[1]
	if arr.Kind() == frontend.NodeArrayLiteralExpression {
		elems := r.prog.Children(arr)
		for _, el := range elems {
			if el.Kind() == frontend.NodeSpreadElement {
				return nil, false, &NotYetLowerable{Reason: "String." + static + " through apply over an array literal with a spread element is a later slice"}
			}
		}
		e, err := r.stringStaticCall(static, elems)
		return e, err == nil, err
	}
	// A runtime array spreads its Elems slice into the variadic value constructor. A
	// number array's Go form is a *value.Array[float64] whose float64 elements the plain
	// constructor takes directly. A dynamic array, element type any or unknown, is a
	// *value.Array[value.Value] whose boxed elements route through the coercing Values
	// constructor, which runs the ToNumber apply applies to each element before it reads
	// the code unit; this is the shape regExpUtils.js's buildString reaches when its
	// code-point array is untyped. A concrete non-number element type hands back rather
	// than emit a spread of the wrong Go type.
	elemT, ok := r.prog.ElementType(r.prog.TypeAt(arr))
	if !ok {
		return nil, false, &NotYetLowerable{Reason: "String." + static + " through apply over a non-array value is a later slice"}
	}
	dynamicElem := elemT.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0
	if !dynamicElem && elemT.Flags&frontend.TypeNumber == 0 {
		return nil, false, &NotYetLowerable{Reason: "String." + static + " through apply over a non-number array is a later slice"}
	}
	lowered, err := r.lowerExpr(arr)
	if err != nil {
		return nil, false, err
	}
	goName := "FromCharCode"
	if static == "fromCodePoint" {
		goName = "FromCodePoint"
	}
	if dynamicElem {
		goName += "Values"
	}
	r.requireImport(valuePkg)
	elems := &ast.CallExpr{Fun: &ast.SelectorExpr{X: lowered, Sel: ident("Elems")}}
	return &ast.CallExpr{Fun: sel("value", goName), Args: []ast.Expr{elems}, Ellipsis: token.Pos(1)}, true, nil
}

// jsonCall lowers a static call on the global JSON namespace. stringify takes a
// value and returns the exact text V8 produces, which lowers to value.JSONStringify
// with the argument boxed as any so the serializer's reflection walk can dispatch
// on its concrete type. A space argument switches to the indented form
// value.JSONStringifyIndentNum or IndentStr. parse takes a single string and
// returns a dynamic any value, which lowers to value.JSONParse and lands in the
// boxed value world the checker already typed the result as. A replacer or a
// reviver function still changes the behavior, so a call that passes one hands
// back rather than ignoring it.
func (r *Renderer) jsonCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "stringify":
		if len(argNodes) == 0 || len(argNodes) > 3 {
			return nil, &NotYetLowerable{Reason: "JSON.stringify takes a value and an optional replacer and space"}
		}
		// A value statically typed as an extended class may hold a subclass at
		// runtime, and JavaScript would serialize the subclass's own fields
		// while the reflection walk over the base struct cannot see them, so
		// the call hands back rather than printing the wrong object.
		if info, ok := r.classOfNode(argNodes[0]); ok && info.extended {
			return nil, &NotYetLowerable{Reason: "JSON.stringify of a value typed as class " + info.name + ", which another class extends, is a later slice"}
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		// A replacer function or array whitelist changes which values are
		// serialized and how, so a present replacer routes to the replacer walk;
		// a null or undefined replacer the specification ignores falls through to
		// the plain serializer.
		if len(argNodes) >= 2 && !r.isJSONNullish(argNodes[1]) {
			return r.jsonStringifyReplacer(arg, argNodes)
		}
		if len(argNodes) == 3 {
			return r.jsonStringifySpace(arg, argNodes[2])
		}
		return &ast.CallExpr{Fun: sel("value", "JSONStringify"), Args: []ast.Expr{arg}}, nil
	case "parse":
		if len(argNodes) == 0 || len(argNodes) > 2 {
			return nil, &NotYetLowerable{Reason: "JSON.parse takes a text and an optional reviver"}
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		// A reviver function rewrites the parsed tree bottom-up, so a present
		// reviver routes to the reviving parse; a null or undefined reviver the
		// specification ignores falls through to the plain parse.
		if len(argNodes) == 2 && !r.isJSONNullish(argNodes[1]) {
			return r.jsonParseReviver(arg, argNodes[1])
		}
		return &ast.CallExpr{Fun: sel("value", "JSONParse"), Args: []ast.Expr{arg}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "JSON." + method + " is a later slice"}
	}
}

// jsonStringifySpace lowers the space argument of a three-argument JSON.stringify
// over an already-lowered value. A numeric space routes to
// value.JSONStringifyIndentNum and a string space to value.JSONStringifyIndentStr,
// each of which computes the indentation gap the specification prescribes (a
// number of spaces clamped to ten, or the first ten characters of the string) and
// falls back to the compact form when the gap is empty. A null or undefined space
// is the compact form outright. Any other space type is a later slice.
func (r *Renderer) jsonStringifySpace(arg ast.Expr, spaceNode frontend.Node) (ast.Expr, error) {
	switch {
	case r.isNumber(spaceNode):
		space, err := r.lowerExpr(spaceNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: sel("value", "JSONStringifyIndentNum"), Args: []ast.Expr{arg, space}}, nil
	case r.isString(spaceNode):
		space, err := r.lowerExpr(spaceNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: sel("value", "JSONStringifyIndentStr"), Args: []ast.Expr{arg, space}}, nil
	case r.isJSONNullish(spaceNode):
		return &ast.CallExpr{Fun: sel("value", "JSONStringify"), Args: []ast.Expr{arg}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "JSON.stringify with a space that is not a number or string is a later slice"}
	}
}

// isJSONNullish reports whether a node is the null keyword or the undefined
// literal, the two values JSON.stringify treats as an absent replacer or space.
func (r *Renderer) isJSONNullish(n frontend.Node) bool {
	return n.Kind() == frontend.NodeNullKeyword || r.isUndefinedLiteral(n)
}

// jsonStringifyReplacer lowers a three-argument JSON.stringify whose replacer is
// present. An inline arrow replacer lowers to value.JSONStringifyReplacerFunc over
// the lowered function, and an array-literal whitelist lowers to
// value.JSONStringifyReplacerArray over the listed keys; each also takes the
// indentation gap the optional space argument computes. A replacer that is neither
// an inline arrow nor an array literal hands back, since only those two forms have
// a static shape the walk can honor.
func (r *Renderer) jsonStringifyReplacer(arg ast.Expr, argNodes []frontend.Node) (ast.Expr, error) {
	gap, err := r.jsonGapExpr(argNodes)
	if err != nil {
		return nil, err
	}
	replacer := argNodes[1]
	switch replacer.Kind() {
	case frontend.NodeArrowFunction:
		if r.arrowParamCount(replacer) != 2 {
			return nil, &NotYetLowerable{Reason: "JSON.stringify with a replacer arrow that does not take the key and value parameters is a later slice"}
		}
		fn, err := r.lowerExpr(replacer)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: sel("value", "JSONStringifyReplacerFunc"), Args: []ast.Expr{arg, fn, gap}}, nil
	case frontend.NodeArrayLiteralExpression:
		keys, err := r.jsonReplacerKeys(replacer)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: sel("value", "JSONStringifyReplacerArray"), Args: []ast.Expr{arg, keys, gap}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "JSON.stringify with a replacer that is not an inline arrow function or array literal is a later slice"}
	}
}

// jsonParseReviver lowers a two-argument JSON.parse whose reviver is present. An
// inline arrow reviver taking the key and value lowers to value.JSONParseReviver
// over the lowered function, which walks the parsed tree bottom-up. A reviver that
// is not an inline arrow, or one that does not take the two parameters, hands
// back, since only an inline arrow has a static shape the walk can call.
func (r *Renderer) jsonParseReviver(arg ast.Expr, reviver frontend.Node) (ast.Expr, error) {
	if reviver.Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "JSON.parse with a reviver that is not an inline arrow function is a later slice"}
	}
	if r.arrowParamCount(reviver) != 2 {
		return nil, &NotYetLowerable{Reason: "JSON.parse with a reviver arrow that does not take the key and value parameters is a later slice"}
	}
	fn, err := r.lowerExpr(reviver)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: sel("value", "JSONParseReviver"), Args: []ast.Expr{arg, fn}}, nil
}

// jsonGapExpr builds the Go expression for the indentation gap a replacer walk
// takes: the empty string when there is no space argument or it is null or
// undefined, value.JSONGapNum over a numeric space, and value.JSONGapStr over a
// string space. Any other space type hands back, matching the plain space path.
func (r *Renderer) jsonGapExpr(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) < 3 || r.isJSONNullish(argNodes[2]) {
		return &ast.BasicLit{Kind: token.STRING, Value: `""`}, nil
	}
	space := argNodes[2]
	switch {
	case r.isNumber(space):
		lowered, err := r.lowerExpr(space)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: sel("value", "JSONGapNum"), Args: []ast.Expr{lowered}}, nil
	case r.isString(space):
		lowered, err := r.lowerExpr(space)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: sel("value", "JSONGapStr"), Args: []ast.Expr{lowered}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "JSON.stringify with a space that is not a number or string is a later slice"}
	}
}

// jsonReplacerKeys builds the []value.BStr the array-whitelist form takes from an
// array literal of string literals, lowering each element to the property name it
// names through the same decoding a string-literal expression uses. An element
// that is not a string literal hands back, since a computed or numeric entry has
// no static string key this slice lists.
func (r *Renderer) jsonReplacerKeys(arr frontend.Node) (ast.Expr, error) {
	elems := r.prog.Children(arr)
	items := make([]ast.Expr, 0, len(elems))
	for _, el := range elems {
		if el.Kind() != frontend.NodeStringLiteral {
			return nil, &NotYetLowerable{Reason: "JSON.stringify with a replacer array of anything but string literals is a later slice"}
		}
		key, err := r.stringLiteral(el)
		if err != nil {
			return nil, err
		}
		items = append(items, key)
	}
	return &ast.CompositeLit{Type: &ast.ArrayType{Elt: sel("value", "BStr")}, Elts: items}, nil
}

// objectCall lowers a static call on the global Object namespace. Only keys is
// covered: Object.keys(o) on a fixed-shape object returns its own enumerable
// property names, in declaration order, which the checker knows in full at
// compile time, so it lowers to a value.NewArray[value.BStr] of the field-name
// literals rather than a runtime property walk. The argument is required to be a
// plain identifier, whose type carries the shape and which has no side effect to
// drop, since only the type is read here. An interned shape with an optional
// property would give a key list the runtime object might not match (a missing
// optional field is not a key), so the call gates on internStruct and hands back
// when the shape does not lower. values and entries mix element types and wait on
// their own slice; any other Object method is a later slice.
func (r *Renderer) objectCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "keys":
		return r.objectOwnNameArray("keys", argNodes)
	case "getOwnPropertyNames":
		return r.objectOwnNameArray("getOwnPropertyNames", argNodes)
	case "values":
		return r.objectValues(argNodes)
	case "hasOwn":
		return r.objectHasOwn(argNodes)
	case "defineProperty":
		return r.objectDefineProperty(argNodes)
	case "defineProperties":
		return r.objectDefineProperties(argNodes)
	case "getOwnPropertyDescriptor":
		return r.objectGetOwnPropertyDescriptor(argNodes)
	case "getOwnPropertyDescriptors":
		return r.objectGetOwnPropertyDescriptors(argNodes)
	case "is":
		return r.objectIs(argNodes)
	case "create":
		return r.objectCreate(argNodes)
	case "getPrototypeOf":
		return r.objectGetPrototypeOf(argNodes)
	case "setPrototypeOf":
		return r.objectSetPrototypeOf(argNodes)
	case "preventExtensions":
		return r.objectIntegrityUnary("preventExtensions", "PreventExtensions", argNodes)
	case "seal":
		return r.objectIntegrityUnary("seal", "Seal", argNodes)
	case "freeze":
		return r.objectIntegrityUnary("freeze", "Freeze", argNodes)
	case "isExtensible":
		return r.objectIntegrityUnary("isExtensible", "IsExtensible", argNodes)
	case "isSealed":
		return r.objectIntegrityUnary("isSealed", "IsSealed", argNodes)
	case "isFrozen":
		return r.objectIntegrityUnary("isFrozen", "IsFrozen", argNodes)
	case "assign":
		return r.objectAssign(argNodes)
	case "fromEntries":
		return r.objectFromEntries(argNodes)
	case "entries":
		return r.objectEntries(argNodes)
	case "getOwnPropertySymbols":
		return r.objectOwnSymbols(argNodes)
	default:
		return nil, &NotYetLowerable{Reason: "Object." + method + " is a later slice"}
	}
}

// reflectCall lowers a static call on the ambient Reflect global to the value
// Reflect helper of the same job. Each helper is a free function taking the boxed
// target and the boxed remaining operands, so a dynamic reflective operation lowers
// without knowing the target's static shape. A method not yet lowered, and the
// receiver-carrying overloads of get and set, hand back with a named reason.
func (r *Renderer) reflectCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "get":
		return r.reflectFree("get", "ReflectGet", 2, argNodes)
	case "set":
		return r.reflectFree("set", "ReflectSet", 3, argNodes)
	case "has":
		return r.reflectFree("has", "ReflectHas", 2, argNodes)
	case "deleteProperty":
		return r.reflectFree("deleteProperty", "ReflectDeleteProperty", 2, argNodes)
	case "ownKeys":
		return r.reflectFree("ownKeys", "ReflectOwnKeys", 1, argNodes)
	case "defineProperty":
		return r.reflectFree("defineProperty", "ReflectDefineProperty", 3, argNodes)
	case "getOwnPropertyDescriptor":
		return r.reflectFree("getOwnPropertyDescriptor", "ReflectGetOwnPropertyDescriptor", 2, argNodes)
	case "getPrototypeOf":
		return r.reflectFree("getPrototypeOf", "ReflectGetPrototypeOf", 1, argNodes)
	case "setPrototypeOf":
		return r.reflectFree("setPrototypeOf", "ReflectSetPrototypeOf", 2, argNodes)
	case "isExtensible":
		return r.reflectFree("isExtensible", "ReflectIsExtensible", 1, argNodes)
	case "preventExtensions":
		return r.reflectFree("preventExtensions", "ReflectPreventExtensions", 1, argNodes)
	case "apply":
		return r.reflectFree("apply", "ReflectApply", 3, argNodes)
	case "construct":
		// Reflect.construct reaches into [[Construct]] over a runtime constructor and
		// threads a newTarget that redirects which prototype the new object receives.
		// bento has no runtime construct over a dynamic value and no newTarget slot on
		// the class path, so there is nothing faithful to emit and it hands back.
		return nil, &NotYetLowerable{Reason: "Reflect.construct needs a runtime [[Construct]] over a dynamic constructor with a newTarget, which is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "Reflect." + method + " is a later slice"}
	}
}

// reflectFree lowers a Reflect method to a call of the value free function goName,
// boxing the target and every remaining operand into a dynamic value. The target
// must be a dynamic value, since a fixed-shape Go struct has no runtime property bag
// a reflective operation can read or write, so a non-dynamic target hands back. The
// call arity must match exactly; an extra receiver argument on get or set routes to
// the handback the switch names, since the receiver overload is a later slice. Every
// operand is evaluated, so no read is dropped.
func (r *Renderer) reflectFree(apiName, goName string, wantArgs int, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != wantArgs {
		return nil, &NotYetLowerable{Reason: "Reflect." + apiName + " with other than " + strconv.Itoa(wantArgs) + " arguments is a later slice"}
	}
	if !r.isDynamic(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Reflect." + apiName + " on a fixed-shape target, which has no runtime property bag, is a later slice"}
	}
	args := make([]ast.Expr, len(argNodes))
	for i, n := range argNodes {
		a, err := r.boxOperand(n)
		if err != nil {
			return nil, err
		}
		args[i] = a
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", goName), Args: args}, nil
}

// objectIntegrityUnary lowers a one-argument integrity static, the family that
// takes a single object and either changes its integrity level or reads it back:
// preventExtensions, seal, freeze, isExtensible, isSealed, and isFrozen. Each maps
// to a runtime method of the same job on the dynamic receiver, so the mutators
// return the receiver value and the predicates return a Go bool that lands in the
// boolean slot the checker gives them. The receiver must be a dynamic value, since
// a fixed-shape Go struct has no runtime integrity state to change or read, so a
// non-dynamic receiver hands back. The receiver is evaluated, so no read is dropped.
func (r *Renderer) objectIntegrityUnary(apiName, runtimeMethod string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Object." + apiName + " with other than one argument is a later slice"}
	}
	if !r.isDynamic(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Object." + apiName + " on a fixed-shape receiver, which has no runtime integrity state, is a later slice"}
	}
	recv, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(runtimeMethod)}}, nil
}

// objectAssign lowers Object.assign(target, ...sources) to a runtime Assign on the
// dynamic target, which copies each source's own enumerable string and symbol
// properties onto the target through the ordinary get and set path and returns the
// target. The target must be a dynamic value, since a fixed-shape Go struct has no
// runtime bag to copy onto, so a non-dynamic target hands back. Each source is boxed
// the way any dynamic operand is, so the runtime reads a null or undefined source as
// a no-op the way the spec skips it. A call with no source is the identity on the
// target. Every operand is evaluated, so no read is dropped.
func (r *Renderer) objectAssign(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) == 0 {
		return nil, &NotYetLowerable{Reason: "Object.assign with no target is a later slice"}
	}
	if !r.isDynamic(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Object.assign onto a fixed-shape target, which has no runtime bag to copy onto, is a later slice"}
	}
	recv, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	sources := make([]ast.Expr, 0, len(argNodes)-1)
	for _, node := range argNodes[1:] {
		src, err := r.boxOperand(node)
		if err != nil {
			return nil, err
		}
		sources = append(sources, src)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Assign")}, Args: sources}, nil
}

// objectFromEntries lowers Object.fromEntries(iterable) to a runtime
// value.FromEntries, which walks the iterable's key-value pairs and sets each onto a
// fresh object, so a later pair with the same key overwrites an earlier one. The
// result is always a runtime object, so the iterable is boxed the way any dynamic
// operand is and no receiver shape is involved. The single operand is evaluated, so
// no read is dropped.
func (r *Renderer) objectFromEntries(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Object.fromEntries with other than one argument is a later slice"}
	}
	iterable, err := r.boxOperand(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "FromEntries"), Args: []ast.Expr{iterable}}, nil
}

// objectEntries lowers Object.entries(o) to a runtime Entries on the dynamic
// receiver, which walks the bag's own enumerable keys and returns an array of
// [key, value] pairs in enumeration order. The receiver must be a dynamic value,
// since a fixed-shape Go struct's field values need not share one element type the
// way a pair array's second slot does, so a non-dynamic receiver hands back. The
// receiver is evaluated, so no read is dropped.
func (r *Renderer) objectEntries(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Object.entries with other than one argument is a later slice"}
	}
	if !r.isDynamic(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Object.entries on a fixed-shape receiver, whose field values need not share one element type, is a later slice"}
	}
	recv, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Entries")}}, nil
}

// objectEntriesShapeCall lowers Object.entries(o) on a fixed-shape object to the
// array of [name, value] pairs the checker types the call as, built at compile
// time from the struct's fields the way Object.values already builds its value
// array. The result is a value.NewArray of the interned two-element tuple struct
// Tuple_str_T, one element per field, each pairing the field-name literal with the
// field read. It owns only the fixed-shape case: a dynamic receiver, a call with
// other than one argument, or a receiver whose global is not Object returns
// handled false, so the runtime Entries walk in objectEntries still covers those.
//
// The value slot of the pair is one Go type, the tuple's second element, so the
// field values must share it: the checker types Object.entries of a heterogeneous
// shape as [string, A | B][], whose tuple value element is a union that hands back
// at typeExpr, and a shape whose fields agree gives a concrete slot every field
// read fills. fixedShapeProps already rejects an optional field, whose absence
// would leave a pair with no value, so only required fields reach the pairing.
func (r *Renderer) objectEntriesShapeCall(call, callee frontend.Node, argNodes []frontend.Node) (ast.Expr, bool, error) {
	kids := r.prog.Children(callee)
	if len(kids) != 2 {
		return nil, false, nil
	}
	recvNode, method := kids[0], r.prog.Text(kids[1])
	if !r.isGlobalRef(recvNode, "Object") || method != "entries" {
		return nil, false, nil
	}
	if len(argNodes) != 1 || r.isDynamic(argNodes[0]) {
		return nil, false, nil
	}
	props, err := r.objectShapeArg("entries", argNodes)
	if err != nil {
		return nil, true, err
	}
	// The result the checker gives the whole call is [string, T][]; its array
	// element is the pair tuple, and its two element types are the Go types the
	// field-name literal and the field read must produce. A result that is not a
	// two-element tuple array, or whose value element does not lower, hands back.
	tupleT, ok := r.prog.ElementType(r.prog.TypeAt(call))
	if !ok {
		return nil, true, &NotYetLowerable{Reason: "Object.entries whose result is not an array is a later slice"}
	}
	elems, ok := r.prog.TupleElements(tupleT)
	if !ok || len(elems) != 2 {
		return nil, true, &NotYetLowerable{Reason: "Object.entries whose result is not a two-element tuple array is a later slice"}
	}
	keyType, err := r.typeExpr(elems[0].Type)
	if err != nil {
		return nil, true, err
	}
	if goTypeString(keyType) != "value.BStr" {
		return nil, true, &NotYetLowerable{Reason: "Object.entries whose key element is not a string is a later slice"}
	}
	valType, err := r.typeExpr(elems[1].Type)
	if err != nil {
		return nil, true, err
	}
	valGo := goTypeString(valType)
	// Each field read fills the tuple's value slot, so a field whose Go type differs
	// from that slot would not compile into the pair, and hands back. A heterogeneous
	// shape already fails above at the union value element, so this guards the
	// residual mismatch a widened field type could leave.
	for _, p := range props {
		t, err := r.typeExpr(p.Type)
		if err != nil {
			return nil, true, err
		}
		if goTypeString(t) != valGo {
			return nil, true, &NotYetLowerable{Reason: "Object.entries of a shape whose field types differ is a later slice"}
		}
	}
	tname, err := r.decls.internTuple(r, tupleT, elems)
	if err != nil {
		return nil, true, err
	}
	recv, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, true, err
	}
	r.requireImport(valuePkg)
	pairs := make([]ast.Expr, len(props))
	for i, p := range props {
		field, ok := exportedField(p.Name)
		if !ok {
			return nil, true, &NotYetLowerable{Reason: "Object.entries field name is not a Go identifier"}
		}
		pairs[i] = &ast.CompositeLit{Type: ident(tname), Elts: []ast.Expr{
			&ast.KeyValueExpr{Key: ident("E0"), Value: &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(p.Name)}}}},
			&ast.KeyValueExpr{Key: ident("E1"), Value: &ast.SelectorExpr{X: recv, Sel: ident(field)}},
		}}
	}
	return &ast.CallExpr{Fun: index(sel("value", "NewArray"), ident(tname)), Args: pairs}, true, nil
}

// objectOwnSymbols lowers Object.getOwnPropertySymbols(o) to a runtime OwnSymbols on
// the dynamic receiver, which returns the receiver's own symbol keys as a value
// array in insertion order. The receiver must be a dynamic value, since a fixed-shape
// Go struct carries no symbol keys to list, so a non-dynamic receiver hands back. The
// receiver is evaluated, so no read is dropped.
func (r *Renderer) objectOwnSymbols(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Object.getOwnPropertySymbols with other than one argument is a later slice"}
	}
	if !r.isDynamic(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Object.getOwnPropertySymbols on a fixed-shape receiver, which carries no symbol keys, is a later slice"}
	}
	recv, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("OwnSymbols")}}, nil
}

// objectSetPrototypeOf lowers Object.setPrototypeOf(o, proto) to a runtime
// SetPrototype on the dynamic receiver, which writes the prototype slot and returns
// the receiver. An object or null becomes the new prototype; a non-extensible
// object rejects a change to a different prototype with a TypeError. The receiver
// must be a dynamic value, since a fixed-shape Go struct has no runtime prototype
// slot to write, so a non-dynamic receiver hands back. The prototype is boxed the
// way any dynamic operand is; both operands are evaluated, so no read is dropped.
func (r *Renderer) objectSetPrototypeOf(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "Object.setPrototypeOf with other than two arguments is a later slice"}
	}
	if !r.isDynamic(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Object.setPrototypeOf on a fixed-shape receiver, which has no runtime prototype slot, is a later slice"}
	}
	recv, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	proto, err := r.boxOperand(argNodes[1])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("SetPrototype")}, Args: []ast.Expr{proto}}, nil
}

// objectGetPrototypeOf lowers Object.getPrototypeOf(o) to a runtime GetPrototype on
// the dynamic receiver, which returns the object's prototype slot as a value or
// null when it has none. The receiver must be a dynamic value, since a fixed-shape
// Go struct has no runtime prototype slot to read, so a non-dynamic receiver hands
// back. The receiver is evaluated, so no read is dropped.
func (r *Renderer) objectGetPrototypeOf(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Object.getPrototypeOf with other than one argument is a later slice"}
	}
	if !r.isDynamic(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Object.getPrototypeOf on a fixed-shape receiver, which has no runtime prototype slot, is a later slice"}
	}
	recv, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("GetPrototype")}}, nil
}

// objectCreate lowers Object.create(proto) and Object.create(proto, descs) to a
// runtime ObjectCreate on the boxed prototype, which returns a new object whose
// [[Prototype]] is that value. The two-argument form applies the descriptor map
// through the same DefineProperties path Object.defineProperties takes, so the
// created object gets its properties in one expression. The prototype and the
// descriptor map are boxed the way any dynamic operand is; both are evaluated, so
// no read is dropped.
func (r *Renderer) objectCreate(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 && len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "Object.create with other than one or two arguments is a later slice"}
	}
	proto, err := r.boxOperand(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	created := &ast.CallExpr{Fun: sel("value", "ObjectCreate"), Args: []ast.Expr{proto}}
	if len(argNodes) == 1 {
		return created, nil
	}
	descs, err := r.boxOperand(argNodes[1])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: created, Sel: ident("DefineProperties")}, Args: []ast.Expr{descs}}, nil
}

// objectIs lowers Object.is(a, b), the SameValue equality, over two operands the
// checker types as the same primitive. Both operands are always evaluated, so no
// read is dropped. For two numbers it lowers to value.NumberSameValue, which
// treats two NaNs as equal and the signed zeros as distinct, the two points
// SameValue parts from Go ==. For two strings it is value.BStr.Equal and for two
// booleans a Go ==, since SameValue agrees with strict equality away from
// numbers. Operands of different types compare false at compile time, which would
// drop both reads, so a mixed pair hands back, and so does any non-primitive
// operand whose SameValue is reference identity.
func (r *Renderer) objectIs(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "Object.is with other than two arguments is a later slice"}
	}
	a, b := argNodes[0], argNodes[1]
	lowerBoth := func() (ast.Expr, ast.Expr, error) {
		la, err := r.lowerExpr(a)
		if err != nil {
			return nil, nil, err
		}
		lb, err := r.lowerExpr(b)
		if err != nil {
			return nil, nil, err
		}
		return la, lb, nil
	}
	switch {
	case r.isNumber(a) && r.isNumber(b):
		la, lb, err := lowerBoth()
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "NumberSameValue"), Args: []ast.Expr{la, lb}}, nil
	case r.isString(a) && r.isString(b):
		la, lb, err := lowerBoth()
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: la, Sel: ident("Equal")}, Args: []ast.Expr{lb}}, nil
	case r.isBool(a) && r.isBool(b):
		la, lb, err := lowerBoth()
		if err != nil {
			return nil, err
		}
		return &ast.BinaryExpr{X: la, Op: token.EQL, Y: lb}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Object.is on operands that are not both the same primitive is a later slice"}
	}
}

// objectShapeArg reads the single fixed-shape argument shared by the Object
// statics that fold at compile time. The argument must be a plain identifier,
// whose type carries the shape and which has no side effect to drop since only
// the type is read, and its type must be a non-array object with known
// properties whose interned struct lowers. It returns the properties in
// declaration order, which is the own enumerable key order these statics walk.
func (r *Renderer) objectShapeArg(method string, argNodes []frontend.Node) ([]frontend.Property, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Object." + method + " with other than one argument is a later slice"}
	}
	return r.fixedShapeProps(method, argNodes[0])
}

// fixedShapeProps reads the properties of a single fixed-shape object argument,
// the shape check the Object statics share. The argument must be a plain
// identifier, whose type carries the shape and which has no side effect to drop
// since only the type is read, and its type must be a non-array object with
// known properties whose interned struct lowers. It returns the properties in
// declaration order, the own enumerable key order these statics walk.
func (r *Renderer) fixedShapeProps(method string, arg frontend.Node) ([]frontend.Property, error) {
	if arg.Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "Object." + method + " of an expression that is not a plain identifier is a later slice"}
	}
	objType := r.prog.TypeAt(arg)
	if objType.Flags&frontend.TypeObject == 0 {
		return nil, &NotYetLowerable{Reason: "Object." + method + " of a non-object is a later slice"}
	}
	if _, isArray := r.prog.ElementType(objType); isArray {
		return nil, &NotYetLowerable{Reason: "Object." + method + " of an array is a later slice"}
	}
	// An optional field may be absent at runtime, so it is not always an own key,
	// which the compile-time key and value lists these statics fold cannot express.
	// The interned struct now lowers such a shape, so the handback that used to ride
	// on internStruct is spelled out here instead.
	if r.shapeHasOptional(objType) {
		return nil, &NotYetLowerable{Reason: "Object." + method + " of a shape with an optional property is a later slice"}
	}
	if _, err := r.decls.internStruct(r, objType); err != nil {
		return nil, err
	}
	props := r.prog.Properties(objType)
	if len(props) == 0 {
		return nil, &NotYetLowerable{Reason: "Object." + method + " of a shape with no known properties is a later slice"}
	}
	return props, nil
}

// objectHasOwn lowers Object.hasOwn(o, key) on a fixed-shape object with a
// string-literal key to the compile-time answer, since the shape names every own
// key. A key that matches a required field is always present, so it folds to
// true, and a key the shape does not have folds to false. Both operands are read
// only for their static value, the identifier's shape and the literal's text, so
// dropping the reads loses nothing. An optional field might be absent at runtime,
// which the shape cannot settle, so a key that names one hands back, and so does
// a dynamic key or a receiver that is not a fixed-shape identifier.
func (r *Renderer) objectHasOwn(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "Object.hasOwn with other than two arguments is a later slice"}
	}
	if r.isDynamic(argNodes[0]) {
		recv, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		key, err := r.boxOperand(argNodes[1])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("HasOwnElem")}, Args: []ast.Expr{key}}, nil
	}
	props, err := r.fixedShapeProps("hasOwn", argNodes[0])
	if err != nil {
		return nil, err
	}
	key, ok := r.stringLiteralKey(argNodes[1])
	if !ok {
		return nil, &NotYetLowerable{Reason: "Object.hasOwn with a key that is not a string literal is a later slice"}
	}
	for _, p := range props {
		if p.Name == key {
			if p.Optional {
				return nil, &NotYetLowerable{Reason: "Object.hasOwn on an optional property, whose presence is not known at compile time, is a later slice"}
			}
			return ident("true"), nil
		}
	}
	return ident("false"), nil
}

// objectDefineProperty lowers Object.defineProperty(o, key, desc) to a runtime
// DefineProperty on the dynamic receiver, which reads the descriptor object's
// value, writable, get, set, enumerable, and configurable fields and applies them
// to the key on the bag. The receiver must be a dynamic value, since a fixed-shape
// Go struct has no runtime descriptor bag to define onto, so a non-dynamic
// receiver hands back. The key and the descriptor are boxed to values so the
// runtime reads them the way it reads any dynamic operand. All three operands are
// evaluated, so no read is dropped.
func (r *Renderer) objectDefineProperty(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 3 {
		return nil, &NotYetLowerable{Reason: "Object.defineProperty with other than three arguments is a later slice"}
	}
	if !r.isDynamic(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Object.defineProperty on a fixed-shape receiver, which has no runtime descriptor bag, is a later slice"}
	}
	recv, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	key, err := r.boxOperand(argNodes[1])
	if err != nil {
		return nil, err
	}
	desc, err := r.boxOperand(argNodes[2])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("DefineProperty")}, Args: []ast.Expr{key, desc}}, nil
}

// objectDefineProperties lowers Object.defineProperties(o, props) to a runtime
// DefineProperties on the dynamic receiver, which walks the descriptor map's own
// enumerable properties and defines each onto the receiver through the same path
// the single-property form takes. The receiver must be a dynamic value for the
// same reason defineProperty requires it, so a fixed-shape receiver hands back.
// Both operands are evaluated, so no read is dropped.
func (r *Renderer) objectDefineProperties(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "Object.defineProperties with other than two arguments is a later slice"}
	}
	if !r.isDynamic(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Object.defineProperties on a fixed-shape receiver, which has no runtime descriptor bag, is a later slice"}
	}
	recv, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	props, err := r.boxOperand(argNodes[1])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("DefineProperties")}, Args: []ast.Expr{props}}, nil
}

// objectGetOwnPropertyDescriptor lowers Object.getOwnPropertyDescriptor(o, key)
// to a runtime GetOwnPropertyDescriptor on the dynamic receiver, which returns the
// descriptor object for the key, value, writable, get, set, enumerable, and
// configurable as verifyProperty reads them, or undefined when the property is
// absent. The receiver must be a dynamic value, since a fixed-shape Go struct has
// no runtime descriptor bag to read from, so a non-dynamic receiver hands back.
// The key is boxed the way any dynamic operand is. Both operands are evaluated, so
// no read is dropped.
func (r *Renderer) objectGetOwnPropertyDescriptor(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "Object.getOwnPropertyDescriptor with other than two arguments is a later slice"}
	}
	if !r.isDynamic(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Object.getOwnPropertyDescriptor on a fixed-shape receiver, which has no runtime descriptor bag, is a later slice"}
	}
	recv, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	key, err := r.boxOperand(argNodes[1])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("GetOwnPropertyDescriptor")}, Args: []ast.Expr{key}}, nil
}

// objectGetOwnPropertyDescriptors lowers Object.getOwnPropertyDescriptors(o) to a
// runtime GetOwnPropertyDescriptors on the dynamic receiver, which builds an object
// mapping every own key to its descriptor object over the same read the single-key
// form takes. The receiver must be a dynamic value, since a fixed-shape Go struct
// has no runtime descriptor bag to read from, so a non-dynamic receiver hands back.
// The receiver is evaluated, so no read is dropped.
func (r *Renderer) objectGetOwnPropertyDescriptors(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Object.getOwnPropertyDescriptors with other than one argument is a later slice"}
	}
	if !r.isDynamic(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Object.getOwnPropertyDescriptors on a fixed-shape receiver, which has no runtime descriptor bag, is a later slice"}
	}
	recv, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("GetOwnPropertyDescriptors")}}, nil
}

// objectOwnNameArray lowers Object.keys(o) and Object.getOwnPropertyNames(o) on
// a fixed-shape object to its own property names, in declaration order, which
// the checker knows in full at compile time, so it lowers to a
// value.NewArray[value.BStr] of the field-name literals rather than a runtime
// property walk. The two statics differ only for the reference-object features
// keys omits, non-enumerable properties and symbol keys, and a struct shape has
// neither, so on a fixed shape both return the same name list. An interned shape
// with an optional property would give a key list the runtime object might not
// match (a missing optional field is not a key), which objectShapeArg already
// gates through internStruct.
func (r *Renderer) objectOwnNameArray(method string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) == 1 && r.isDynamic(argNodes[0]) {
		recv, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		// keys drops the non-enumerable properties defineProperty can create;
		// getOwnPropertyNames keeps them, so each routes to its own bag walk.
		walk := "OwnKeys"
		if method == "keys" {
			walk = "OwnEnumerableKeys"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(walk)}}, nil
	}
	props, err := r.objectShapeArg(method, argNodes)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	keys := make([]ast.Expr, len(props))
	for i, p := range props {
		keys[i] = &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(p.Name)}}}
	}
	return &ast.CallExpr{Fun: index(sel("value", "NewArray"), sel("value", "BStr")), Args: keys}, nil
}

// objectValues lowers Object.values(o) on a fixed-shape object to the field
// reads in declaration order, gathered into one array. The values become the
// elements of a single array, so they must share one Go element type: a shape
// whose field types differ would need a mixed-element array, its own slice, so a
// heterogeneous shape hands back. An optional field might be absent, in which
// case it is not a value, so a shape with an optional field hands back too. The
// field types are compared through their rendered Go source so a number-literal
// field type and a widened number field type read as the same element type.
func (r *Renderer) objectValues(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) == 1 && r.isDynamic(argNodes[0]) {
		recv, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("OwnValues")}}, nil
	}
	props, err := r.objectShapeArg("values", argNodes)
	if err != nil {
		return nil, err
	}
	elemType, err := r.typeExpr(props[0].Type)
	if err != nil {
		return nil, err
	}
	elemGo := goTypeString(elemType)
	for _, p := range props {
		if p.Optional {
			return nil, &NotYetLowerable{Reason: "Object.values of a shape with an optional field is a later slice"}
		}
		t, err := r.typeExpr(p.Type)
		if err != nil {
			return nil, err
		}
		if goTypeString(t) != elemGo {
			return nil, &NotYetLowerable{Reason: "Object.values of a shape whose field types differ is a later slice"}
		}
	}
	recv, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	vals := make([]ast.Expr, len(props))
	for i, p := range props {
		field, ok := exportedField(p.Name)
		if !ok {
			return nil, &NotYetLowerable{Reason: "Object.values field name is not a Go identifier"}
		}
		vals[i] = &ast.SelectorExpr{X: recv, Sel: ident(field)}
	}
	return &ast.CallExpr{Fun: index(sel("value", "NewArray"), elemType), Args: vals}, nil
}

// primitiveValueCall lowers toString and valueOf on a number or boolean value.
// Both take no arguments here: number.toString with a radix throws a RangeError
// on a radix outside 2..36, which waits on the exception machinery, so a call
// with any argument hands back. toString is the same coercion String(x) already
// uses, a number through value.NumberToString and a boolean through
// value.BoolToString, so it shares stringify to stay in step with it. valueOf
// returns the primitive itself, so it lowers to the receiver expression
// unchanged. Any other method on a primitive receiver is a later slice.
func (r *Renderer) primitiveValueCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	// number.toString(radix) is the one primitive method with an argument this
	// slice covers. A literal radix in 2..36 lowers straight to
	// value.NumberToStringRadix with the literal folded in, and radix 10 is the same
	// coercion String(x) runs so it routes through stringify. A radix the compiler
	// cannot prove is in range, a non-literal or a literal outside 2..36, lowers to
	// value.NumberToStringRadixDynamic, which applies ToInteger, throws the
	// RangeError a bad radix raises, and renders the same way.
	if method == "toString" && len(argNodes) == 1 && r.isNumber(recvNode) {
		radix, ok := r.literalIntArg(argNodes[0], 2, 36)
		if !ok {
			return r.numberFormatDynamic("NumberToStringRadixDynamic", recvNode, argNodes[0])
		}
		if radix == 10 {
			return r.stringify(recvNode)
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{
			Fun:  sel("value", "NumberToStringRadix"),
			Args: []ast.Expr{recv, &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(radix)}},
		}, nil
	}
	// number.toFixed(digits) formats with a fixed number of fraction digits. A
	// literal count in 0..100 lowers straight to value.NumberToFixed, which rounds
	// the exact double the way the specification does. An omitted count means zero.
	// A count the compiler cannot prove is in range, a non-literal or a literal
	// outside 0..100, lowers to value.NumberToFixedDynamic, which applies ToInteger,
	// throws the RangeError on an out-of-range result, and formats the same way.
	if method == "toFixed" && len(argNodes) <= 1 && r.isNumber(recvNode) {
		if len(argNodes) == 1 {
			if _, ok := r.literalIntArg(argNodes[0], 0, 100); !ok {
				return r.numberFormatDynamic("NumberToFixedDynamic", recvNode, argNodes[0])
			}
		}
		digits := 0
		if len(argNodes) == 1 {
			digits, _ = r.literalIntArg(argNodes[0], 0, 100)
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{
			Fun:  sel("value", "NumberToFixed"),
			Args: []ast.Expr{recv, &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(digits)}},
		}, nil
	}
	// number.toExponential(digits) formats in exponential notation with exactly
	// digits fraction digits. A literal count in 0..100 lowers straight to
	// value.NumberToExponential. A count the compiler cannot prove is in range, a
	// non-literal or a literal outside 0..100, lowers to
	// value.NumberToExponentialDynamic, which range-checks at runtime and throws the
	// RangeError. The omitted-count form uses as many digits as the value needs, a
	// different rule, so it still hands back rather than defaulting to zero.
	if method == "toExponential" && len(argNodes) == 1 && r.isNumber(recvNode) {
		if _, ok := r.literalIntArg(argNodes[0], 0, 100); !ok {
			return r.numberFormatDynamic("NumberToExponentialDynamic", recvNode, argNodes[0])
		}
		digits, _ := r.literalIntArg(argNodes[0], 0, 100)
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{
			Fun:  sel("value", "NumberToExponential"),
			Args: []ast.Expr{recv, &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(digits)}},
		}, nil
	}
	// number.toPrecision(precision) formats with a fixed number of significant
	// digits, choosing fixed or exponential notation the way the specification does.
	// A literal precision in 1..100 (not 0..100 like the other two: zero significant
	// digits is not a valid precision and throws) lowers straight to
	// value.NumberToPrecision. A precision the compiler cannot prove is in range, a
	// non-literal or a literal outside 1..100, lowers to
	// value.NumberToPrecisionDynamic, which range-checks at runtime and throws the
	// RangeError. The omitted form is Number::toString, a different rule, so it still
	// hands back rather than defaulting.
	if method == "toPrecision" && len(argNodes) == 1 && r.isNumber(recvNode) {
		if _, ok := r.literalIntArg(argNodes[0], 1, 100); !ok {
			return r.numberFormatDynamic("NumberToPrecisionDynamic", recvNode, argNodes[0])
		}
		precision, _ := r.literalIntArg(argNodes[0], 1, 100)
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{
			Fun:  sel("value", "NumberToPrecision"),
			Args: []ast.Expr{recv, &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(precision)}},
		}, nil
	}
	if len(argNodes) != 0 {
		return nil, &NotYetLowerable{Reason: "primitive method ." + method + " with arguments is a later slice"}
	}
	switch method {
	case "toString":
		return r.stringify(recvNode)
	case "valueOf":
		return r.lowerExpr(recvNode)
	default:
		return nil, &NotYetLowerable{Reason: "primitive method ." + method + " is a later slice"}
	}
}

// numberFormatDynamic lowers a number formatter, toFixed, toExponential, or
// toPrecision, whose digit count the compiler cannot prove is in range: a
// non-literal count or a literal outside the method's valid range. It emits the
// named value.*Dynamic runtime, which applies ToInteger to the count, throws the
// RangeError JavaScript raises for an out-of-range result, and formats the same
// way as the exact formatter. The count must itself be a number; a count of any
// other type hands back, since the ToInteger of an arbitrary value is a later
// slice.
func (r *Renderer) numberFormatDynamic(fn string, recvNode, countNode frontend.Node) (ast.Expr, error) {
	if !r.isNumber(countNode) {
		return nil, &NotYetLowerable{Reason: "number formatter with a non-number digit count is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	count, err := r.lowerExpr(countNode)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun:  sel("value", fn),
		Args: []ast.Expr{recv, count},
	}, nil
}

// literalIntArg reads a numeric-literal argument whose ToInteger value lands in
// [lo, hi], returning that integer. It is the shared compile-time guard for the
// number formatting methods whose argument must be in a fixed range or throw: a
// toString radix in 2..36 and a toFixed digit count in 0..100. A non-literal
// argument (which cannot be checked at compile time) or a literal that truncates
// outside the range returns false, so the caller hands back rather than emit a
// call that could throw a RangeError with no handler in place.
func (r *Renderer) literalIntArg(n frontend.Node, lo, hi int) (int, bool) {
	if n.Kind() != frontend.NodeNumericLiteral {
		return 0, false
	}
	v, ok := numericLiteralValue(r.prog.Text(n))
	if !ok {
		return 0, false
	}
	i := int(v) // ToInteger truncates toward zero.
	if i < lo || i > hi {
		return 0, false
	}
	return i, true
}

// numberMethod maps a JavaScript Number static predicate to the value function
// that implements it. Only the non-coercing predicates are here: isNaN, isFinite,
// isInteger, and isSafeInteger, whose meaning on a number argument is exact.
// Number.parseFloat and parseInt are handled before this map in numberCall, since
// they route to the same lowering as the global parseInt/parseFloat. The Number(x)
// coercion call is a call on Number itself rather than a static method, so it is
// handled on the coercion path.
func numberMethod(name string) (goName string, ok bool) {
	switch name {
	case "isNaN":
		return "NumberIsNaN", true
	case "isFinite":
		return "NumberIsFinite", true
	case "isInteger":
		return "NumberIsInteger", true
	case "isSafeInteger":
		return "NumberIsSafeInteger", true
	default:
		return "", false
	}
}

// mathMethod maps a JavaScript Math method to the Go function that computes the
// same value, with the package it lives in and the accepted argument count as a
// [minArity, maxArity] range (maxArity of -1 means unbounded). Most map straight
// onto the Go math package: floor, ceil, trunc, abs, and sqrt are IEEE operations
// that agree bit for bit, and pow folds two numbers with the same NaN and
// signed-zero rules. min and max map to the value package because JavaScript lets
// them take any number of arguments where math.Min and math.Max take exactly two,
// so value.MinN and value.MaxN fold a whole argument list with the same identity,
// NaN, and signed-zero rules. round and sign also map to value: Math.round breaks
// a tie toward +Infinity where math.Round rounds away from zero, and Go has no
// math.Sign at all. fround, clz32, and imul map to value too, but for a different
// reason than round: they are integer or single-precision operations, so they are
// bit-exact and agree with the engine to the last bit the way the transcendental
// functions cannot. hypot also maps to value: Math.hypot takes any number of
// arguments where math.Hypot takes exactly two, so value.HypotN folds a whole list
// with the same +0 identity and infinity-over-NaN order, and the pairwise fold
// avoids the overflow a naive sum of squares would hit. The transcendental
// functions (cbrt, exp, expm1, the logs, the trig and inverse-trig and hyperbolic
// families, and atan2) map straight onto the Go math package too, but their
// last-bit results are not guaranteed identical across two libm implementations,
// so they are proven by the equivalence harness's numeric-tolerance mode rather
// than by an exact match, and value.HypotN inherits that tolerance since it folds
// math.Hypot. random maps to value.MathRandom, the one Math method that is not a
// function of its arguments: it draws a fresh number on every call, so the two
// runtimes cannot agree on its output and the differential oracle checks its shape
// (range and non-constancy) rather than an exact value.
func mathMethod(name string) (pkg, goName string, minArity, maxArity int, ok bool) {
	switch name {
	case "floor":
		return "math", "Floor", 1, 1, true
	case "ceil":
		return "math", "Ceil", 1, 1, true
	case "trunc":
		return "math", "Trunc", 1, 1, true
	case "abs":
		return "math", "Abs", 1, 1, true
	case "sqrt":
		return "math", "Sqrt", 1, 1, true
	case "pow":
		return valuePkg, "Pow", 2, 2, true
	case "min":
		return valuePkg, "MinN", 0, -1, true
	case "max":
		return valuePkg, "MaxN", 0, -1, true
	case "round":
		return valuePkg, "Round", 1, 1, true
	case "sign":
		return valuePkg, "Sign", 1, 1, true
	case "fround":
		return valuePkg, "Fround", 1, 1, true
	case "clz32":
		return valuePkg, "Clz32", 1, 1, true
	case "imul":
		return valuePkg, "Imul", 2, 2, true
	case "cbrt":
		return "math", "Cbrt", 1, 1, true
	case "exp":
		return "math", "Exp", 1, 1, true
	case "expm1":
		return "math", "Expm1", 1, 1, true
	case "log":
		return "math", "Log", 1, 1, true
	case "log2":
		return "math", "Log2", 1, 1, true
	case "log10":
		return "math", "Log10", 1, 1, true
	case "log1p":
		return "math", "Log1p", 1, 1, true
	case "sin":
		return "math", "Sin", 1, 1, true
	case "cos":
		return "math", "Cos", 1, 1, true
	case "tan":
		return "math", "Tan", 1, 1, true
	case "asin":
		return "math", "Asin", 1, 1, true
	case "acos":
		return "math", "Acos", 1, 1, true
	case "atan":
		return "math", "Atan", 1, 1, true
	case "atan2":
		return "math", "Atan2", 2, 2, true
	case "sinh":
		return "math", "Sinh", 1, 1, true
	case "cosh":
		return "math", "Cosh", 1, 1, true
	case "tanh":
		return "math", "Tanh", 1, 1, true
	case "asinh":
		return "math", "Asinh", 1, 1, true
	case "acosh":
		return "math", "Acosh", 1, 1, true
	case "atanh":
		return "math", "Atanh", 1, 1, true
	case "hypot":
		return valuePkg, "HypotN", 0, -1, true
	case "random":
		return valuePkg, "MathRandom", 0, 0, true
	default:
		return "", "", 0, 0, false
	}
}

// isGlobalRef reports whether n is a reference to the ambient global named name,
// like Math, rather than a user binding that happens to share the name. It checks
// the identifier text and then requires every declaration of the resolved symbol
// to live in a .d.ts library file, so a local `const Math = ...` that adds a
// source-file declaration is correctly excluded and its methods do not lower to
// the Go math package.
func (r *Renderer) isGlobalRef(n frontend.Node, name string) bool {
	if r.prog.Text(n) != name {
		return false
	}
	return r.isAmbientGlobal(n)
}

// isAmbientGlobal reports whether n resolves to a symbol declared only in .d.ts
// library files, the test that separates an ambient global from a user binding
// that shadows the same name. isGlobalRef adds a name check on top; the bare
// global-function path uses this directly, having already matched the name.
func (r *Renderer) isAmbientGlobal(n frontend.Node) bool {
	if n.Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(n)
	if !ok {
		return false
	}
	decls := r.prog.Declarations(sym)
	if len(decls) == 0 {
		return false
	}
	for _, d := range decls {
		if d.File().Kind != frontend.FileDTS {
			return false
		}
	}
	return true
}

// isBuiltinCallee reports whether a call-position identifier resolves to a host
// builtin rather than a user binding: an ambient global (String, Number, Array)
// or a name bound by a node: or go: import. The callable-object call path checks
// this so it does not shadow a builtin that is a callable object in the type
// system but has its own dedicated lowering further down callExpr.
func (r *Renderer) isBuiltinCallee(n frontend.Node) bool {
	if r.isAmbientGlobal(n) {
		return true
	}
	if n.Kind() != frontend.NodeIdentifier {
		return false
	}
	name := r.prog.Text(n)
	if _, ok := r.nodeImports[name]; ok {
		return true
	}
	if _, ok := r.goImports[name]; ok {
		return true
	}
	return false
}

// isObjectProtoToString reports whether n is the member chain
// Object.prototype.toString, the function the class-tag idiom borrows with .call.
// It matches a property access .toString whose object is a property access
// .prototype whose object is the ambient global Object, so a user value carrying
// its own prototype.toString chain does not match and its methods stay on their
// own paths.
func (r *Renderer) isObjectProtoToString(n frontend.Node) bool {
	if n.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 || r.prog.Text(kids[1]) != "toString" {
		return false
	}
	proto := kids[0]
	if proto.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	pkids := r.prog.Children(proto)
	if len(pkids) != 2 || r.prog.Text(pkids[1]) != "prototype" {
		return false
	}
	return r.isGlobalRef(pkids[0], "Object")
}

// objectProtoToStringCall lowers Object.prototype.toString.call(x) to the value
// class-tag helper. Only .call with exactly one argument is the idiom; .apply, a
// bind, or a different arity hands back so a real reflective borrow is not
// mislowered to the tag helper. The argument boxes to a value.Value so the helper
// can read any kind's tag, the same box a dynamic operand takes.
func (r *Renderer) objectProtoToStringCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "call" || len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Object.prototype.toString borrowed with anything other than .call(x) is a later slice"}
	}
	// A statically known typed array carries the Symbol.toStringTag its constructor
	// installs, so Object.prototype.toString.call(ta) reads "[object Int32Array]" off
	// the concrete constructor name rather than the generic object tag. A typed array
	// does not box into a value.Value, so the runtime ClassTag cannot recover the name;
	// TypedArrayClassTag takes the view and the compiler-known name, evaluating the view
	// for its side effects and reading it as a use while the tag comes from the name.
	// The whole family is covered, since typedArrayName names Uint8Array and the bigint
	// arrays too, each of which has the same class-tag rule.
	if name, ok := r.typedArrayName(r.prog.TypeAt(argNodes[0])); ok {
		view, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{
			Fun:  sel("value", "TypedArrayClassTag"),
			Args: []ast.Expr{view, &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(name)}},
		}, nil
	}
	// A Map or Set carries the Symbol.toStringTag its prototype installs, "Map" and
	// "Set", so Object.prototype.toString.call reads "[object Map]" and "[object
	// Set]". Like a typed array, neither boxes into a value.Value the runtime ClassTag
	// could read, so NamedClassTag takes the collection for its side effects and reads
	// the tag from the compiler-known name.
	tag := ""
	switch {
	case r.isMap(argNodes[0]):
		tag = "Map"
	case r.isSet(argNodes[0]):
		tag = "Set"
	case r.isRegExp(argNodes[0]):
		// A RegExp carries a Symbol.toStringTag of "RegExp", so
		// Object.prototype.toString.call(re) reads "[object RegExp]". Like a Map, it
		// does not box into a value.Value the runtime ClassTag could read, so the tag
		// comes from the compiler-known name.
		tag = "RegExp"
	}
	if tag != "" {
		recv, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{
			Fun:  sel("value", "NamedClassTag"),
			Args: []ast.Expr{recv, &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(tag)}},
		}, nil
	}
	arg, err := r.boxOperand(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "ClassTag"), Args: []ast.Expr{arg}}, nil
}

// isArrayProtoMap reports whether n is the member chain Array.prototype.map, the
// function the assert prelude borrows with .call to format an array-like for a
// failed comparison. It matches a property access .map whose object is a property
// access .prototype whose object is the ambient global Array, so a user value that
// carries its own map does not match and stays on the array-method path.
func (r *Renderer) isArrayProtoMap(n frontend.Node) bool {
	if n.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 || r.prog.Text(kids[1]) != "map" {
		return false
	}
	proto := kids[0]
	if proto.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	pkids := r.prog.Children(proto)
	if len(pkids) != 2 || r.prog.Text(pkids[1]) != "prototype" {
		return false
	}
	return r.isGlobalRef(pkids[0], "Array")
}

// arrayProtoMethodName reports whether n is the member chain Array.prototype.<name>
// and returns the borrowed method's name. It matches a property access whose object
// is a property access .prototype whose object is the ambient global Array, so a
// user value that carries a like-named method does not match and stays on the
// array-method path.
func (r *Renderer) arrayProtoMethodName(n frontend.Node) (string, bool) {
	if n.Kind() != frontend.NodePropertyAccessExpression {
		return "", false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return "", false
	}
	proto := kids[0]
	if proto.Kind() != frontend.NodePropertyAccessExpression {
		return "", false
	}
	pkids := r.prog.Children(proto)
	if len(pkids) != 2 || r.prog.Text(pkids[1]) != "prototype" {
		return "", false
	}
	if !r.isGlobalRef(pkids[0], "Array") {
		return "", false
	}
	return r.prog.Text(kids[1]), true
}

// arrayBorrowedArgs splits the arguments of a borrowed call Array.prototype.<m>.call
// or .apply into the array-like receiver and the arguments the method itself sees.
// call spells the method arguments inline after the receiver; apply gathers them in
// a plain array literal, the only apply form whose length is known at lowering. A
// borrow with no receiver, or an apply whose arguments are not a plain array literal
// or carry a spread, hands back rather than emit a call of the wrong shape.
func (r *Renderer) arrayBorrowedArgs(name, method string, argNodes []frontend.Node) (frontend.Node, []frontend.Node, error) {
	if len(argNodes) == 0 {
		return nil, nil, &NotYetLowerable{Reason: "Array.prototype." + name + " borrowed with no receiver is a later slice"}
	}
	switch method {
	case "call":
		return argNodes[0], argNodes[1:], nil
	case "apply":
		if len(argNodes) < 2 {
			return argNodes[0], nil, nil
		}
		arr := argNodes[1]
		if arr.Kind() != frontend.NodeArrayLiteralExpression {
			return nil, nil, &NotYetLowerable{Reason: "Array.prototype." + name + " applied over a non-literal argument array is a later slice"}
		}
		elems := r.prog.Children(arr)
		for _, el := range elems {
			if el.Kind() == frontend.NodeSpreadElement {
				return nil, nil, &NotYetLowerable{Reason: "Array.prototype." + name + " applied over an array literal with a spread element is a later slice"}
			}
		}
		return argNodes[0], elems, nil
	default:
		return nil, nil, &NotYetLowerable{Reason: "Array.prototype." + name + " borrowed with ." + method + " is a later slice"}
	}
}

// arrayProtoBorrowedCall lowers Array.prototype.<name>.call/apply(arrayLike, ...) to
// the matching generic-receiver runtime, which reads length and integer keys off the
// receiver and runs the method's algorithm whatever the receiver's kind. The
// receiver and each method argument box to a value.Value, the same box a dynamic
// operand takes, so the runtime reads them uniformly. A method the generic runtime
// does not cover yet hands back with its name, so the borrow is never mislowered.
func (r *Renderer) arrayProtoBorrowedCall(name, method string, argNodes []frontend.Node) (ast.Expr, error) {
	recvNode, methodArgs, err := r.arrayBorrowedArgs(name, method, argNodes)
	if err != nil {
		return nil, err
	}
	recv, err := r.boxOperand(recvNode)
	if err != nil {
		return nil, err
	}
	args := make([]ast.Expr, 0, len(methodArgs)+1)
	args = append(args, recv)
	for _, a := range methodArgs {
		boxed, err := r.boxOperand(a)
		if err != nil {
			return nil, err
		}
		args = append(args, boxed)
	}
	r.requireImport(valuePkg)
	switch name {
	case "indexOf":
		return r.arrayGenericCall("GenericIndexOf", args, methodArgs, 1)
	case "lastIndexOf":
		return r.arrayGenericCall("GenericLastIndexOf", args, methodArgs, 1)
	case "includes":
		return r.arrayGenericCall("GenericIncludes", args, methodArgs, 1)
	case "fill":
		return r.arrayGenericCall("GenericFill", args, methodArgs, 1)
	case "join":
		return r.arrayGenericCall("GenericJoin", args, methodArgs, 0)
	case "copyWithin":
		return r.arrayGenericCall("GenericCopyWithin", args, methodArgs, 0)
	case "reverse":
		return r.arrayGenericCall("GenericReverse", args, methodArgs, 0)
	case "slice":
		return r.arrayGenericCall("GenericSlice", args, methodArgs, 0)
	case "concat":
		return r.arrayGenericCall("GenericConcat", args, methodArgs, 0)
	case "forEach":
		return r.arrayGenericCall("GenericForEach", args, methodArgs, 1)
	case "map":
		return r.arrayGenericCall("GenericMap", args, methodArgs, 1)
	case "filter":
		return r.arrayGenericCall("GenericFilter", args, methodArgs, 1)
	case "some":
		return r.arrayGenericCall("GenericSome", args, methodArgs, 1)
	case "every":
		return r.arrayGenericCall("GenericEvery", args, methodArgs, 1)
	case "find":
		return r.arrayGenericCall("GenericFind", args, methodArgs, 1)
	case "findIndex":
		return r.arrayGenericCall("GenericFindIndex", args, methodArgs, 1)
	case "reduce":
		return r.arrayGenericCall("GenericReduce", args, methodArgs, 1)
	case "reduceRight":
		return r.arrayGenericCall("GenericReduceRight", args, methodArgs, 1)
	default:
		return nil, &NotYetLowerable{Reason: "Array.prototype." + name + " on a generic receiver is a later slice"}
	}
}

// arrayGenericCall emits the call to a generic-receiver runtime helper, checking the
// method saw at least minArgs positional arguments so a borrow that omits a required
// one hands back rather than emit a call that reads a missing argument.
func (r *Renderer) arrayGenericCall(fn string, args []ast.Expr, methodArgs []frontend.Node, minArgs int) (ast.Expr, error) {
	if len(methodArgs) < minArgs {
		return nil, &NotYetLowerable{Reason: "borrowed " + fn + " with too few arguments is a later slice"}
	}
	return &ast.CallExpr{Fun: sel("value", fn), Args: args}, nil
}

// arrayProtoMapCall lowers Array.prototype.map.call(arrayLike, String) to the
// value map-and-stringify helper. Only .call with exactly the array-like receiver
// and the global String as the callback is the idiom the assert prelude uses; a
// different arity, .apply, or any other callback hands back so a general borrow is
// not mislowered to the String-specific helper. The receiver boxes to a value.Value
// so the helper can read its length and elements whatever kind it is, the same box
// a dynamic operand takes.
func (r *Renderer) arrayProtoMapCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "call" || len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "Array.prototype.map borrowed with anything other than .call(arrayLike, String) is a later slice"}
	}
	if !r.isGlobalRef(argNodes[1], "String") {
		return nil, &NotYetLowerable{Reason: "Array.prototype.map.call with a callback other than the String built-in is a later slice"}
	}
	recv, err := r.boxOperand(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "MapCallString"), Args: []ast.Expr{recv}}, nil
}

// processStream reports whether n refers to process.stdout or process.stderr,
// the ambient output streams, and returns which one. It matches a property access
// whose object is the ambient global process and whose property is stdout or
// stderr, so a user object that happens to carry a stdout field does not match and
// its methods do not lower to the process write helpers.
func (r *Renderer) processStream(n frontend.Node) (string, bool) {
	if n.Kind() != frontend.NodePropertyAccessExpression {
		return "", false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return "", false
	}
	if !r.isGlobalRef(kids[0], "process") {
		return "", false
	}
	switch prop := r.prog.Text(kids[1]); prop {
	case "stdout", "stderr":
		return prop, true
	default:
		return "", false
	}
}

// processStreamCall lowers a call on a process output stream. Only write is
// covered: it takes a single string and lowers to value.WriteStdout or
// value.WriteStderr, which write the string's UTF-8 view to the file descriptor
// and return the boolean write reports. A different method, a different arity, or
// a non-string argument hands back rather than emitting a mistyped call.
func (r *Renderer) processStreamCall(stream, method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "write" {
		return nil, &NotYetLowerable{Reason: "process." + stream + "." + method + " is a later slice"}
	}
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "process." + stream + ".write with this argument count is a later slice"}
	}
	if !r.isString(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "process." + stream + ".write of a non-string is a later slice"}
	}
	arg, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	goName := "WriteStdout"
	if stream == "stderr" {
		goName = "WriteStderr"
	}
	return &ast.CallExpr{Fun: sel("value", goName), Args: []ast.Expr{arg}}, nil
}

// processCall lowers a call on the global process object. Only on is covered, and
// only for the "exit" event: process.on('exit', fn) registers fn as a run-at-exit
// callback, lowering to value.OnExit with the callback boxed into a value.Value so
// the end-of-main drain can invoke it without its static signature. It sets the
// program's exit-callback flag so the assembled main appends value.RunExitCallbacks
// as its final statement. A different method, a different event, a different arity,
// or a non-function listener hands back rather than emitting a call that would drop
// the registration.
func (r *Renderer) processCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "on" {
		return nil, &NotYetLowerable{Reason: "process." + method + " is a later slice"}
	}
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "process.on with this argument count is a later slice"}
	}
	if argNodes[0].Kind() != frontend.NodeStringLiteral || unquote(r.prog.Text(argNodes[0])) != "exit" {
		return nil, &NotYetLowerable{Reason: "process.on for an event other than exit is a later slice"}
	}
	if calls, _ := r.prog.Signatures(r.prog.TypeAt(argNodes[1])); len(calls) != 1 {
		return nil, &NotYetLowerable{Reason: "process.on('exit') with a non-function listener is a later slice"}
	}
	fn, err := r.boxOperand(argNodes[1])
	if err != nil {
		return nil, err
	}
	r.usesExitCallbacks = true
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "OnExit"), Args: []ast.Expr{fn}}, nil
}

// consoleCall lowers a call on the global console. The methods that write to
// standard output (log, info, debug) lower to value.ConsoleLog, and the ones that
// write to standard error (error, warn) to value.ConsoleError. Each argument is
// stringified with the same ECMAScript ToString a String() call uses, so a
// number, boolean, or string prints exactly as Node's console does for that
// primitive, and the parts join with a space and a trailing newline inside the
// helper. An argument this slice cannot stringify (an object, whose inspect runs
// richer formatting) hands back rather than printing the wrong text.
func (r *Renderer) consoleCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	var goName string
	switch method {
	case "log", "info", "debug":
		goName = "ConsoleLog"
	case "error", "warn":
		goName = "ConsoleError"
	default:
		return nil, &NotYetLowerable{Reason: "console." + method + " is a later slice"}
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		part, err := r.consoleStringify(a)
		if err != nil {
			return nil, err
		}
		args = append(args, part)
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", goName), Args: args}, nil
}

// consoleStringify lowers one console argument to its inspected string form, which
// matches ToString for every type but bigint: console.log(10n) prints "10n" with the
// suffix the inspector adds, while String(10n) and `${10n}` stay "10". So a bigint
// argument goes through value.BigIntToConsole and every other type defers to the
// shared stringify, keeping the two string paths in step everywhere the suffix does
// not apply.
func (r *Renderer) consoleStringify(arg frontend.Node) (ast.Expr, error) {
	if r.isBigInt(arg) {
		lowered, err := r.lowerExpr(arg)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "BigIntToConsole"), Args: []ast.Expr{lowered}}, nil
	}
	// console.log inspects a value whose kind is known only at run time rather than
	// coercing it, and a boxed bigint prints with the trailing "n" String drops, so a
	// value that lowers to a value.Value box renders through value.ConsoleValue, the
	// console sibling of value.ToString. ConsoleValue matches ToString for every other
	// kind, so only a bigint in a dynamic slot changes; without this a bigint that flowed
	// into an any slot would print its digits with no "n".
	//
	// The precedence below mirrors stringify exactly so the reroute fires on the same
	// arguments stringify would have wrapped in value.ToString, and only those. A caught
	// error is a *value.Error, not a box, so it takes stringify's Error.prototype.toString
	// path. A read that folds to a concrete Go primitive (a function's .length arity folds
	// to a float64 constant, yet reads as dynamic) must take stringify's primitive path,
	// so the primitive checks come before the dynamic one, the order stringify uses;
	// wrapping that float64 in value.ConsoleValue would not compile. producesBoxedValue
	// comes first because a call already lowered to a box (an iterator helper, a
	// descriptor read) is a value.Value whatever concrete type the checker gave it.
	if r.isCaughtErrorRef(arg) {
		return r.stringify(arg)
	}
	if r.producesBoxedValue(arg) {
		return r.consoleInspect(arg)
	}
	if r.isString(arg) || r.isNumber(arg) || r.isBool(arg) {
		return r.stringify(arg)
	}
	if r.isDynamic(arg) {
		return r.consoleInspect(arg)
	}
	return r.stringify(arg)
}

// consoleInspect lowers a boxed or dynamic console argument through value.ConsoleValue,
// the console inspector: it renders a boxed bigint with the "n" suffix and every other
// kind exactly as value.ToString. The argument lowers to a value.Value here, the
// invariant its two callers (producesBoxedValue and the terminal isDynamic case) share
// with stringify's own value.ToString wrapping.
func (r *Renderer) consoleInspect(arg frontend.Node) (ast.Expr, error) {
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "ConsoleValue"), Args: []ast.Expr{lowered}}, nil
}

// globalFn maps a bare global function name to the value function that implements
// it. Only the two number predicates are here: the global isNaN and isFinite
// coerce their argument to a number and then test it, so on an argument that
// already types as number they are exactly value.NumberIsNaN and
// value.NumberIsFinite, the same functions Number.isNaN and Number.isFinite use.
// The global parseInt and parseFloat, which parse a string, are a later slice.
func globalFn(name string) (goName string, ok bool) {
	switch name {
	case "isNaN":
		return "NumberIsNaN", true
	case "isFinite":
		return "NumberIsFinite", true
	default:
		return "", false
	}
}

// globalFnCall lowers a bare call to an ambient global function. The covered
// functions take one number and return a boolean, so the argument count must be
// one and the argument must type as number; anything else hands back rather than
// emitting a call whose coercion this slice does not model. calleeNode is the
// callee identifier, used only for its position in the error.
func (r *Renderer) globalFnCall(goName string, calleeNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	name := r.prog.Text(calleeNode)
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: name + " with this argument count is a later slice"}
	}
	if !r.isNumber(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: name + " on a non-number argument is a later slice"}
	}
	arg, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", goName), Args: []ast.Expr{arg}}, nil
}

// stringCoercion lowers String(x) called as a function over a primitive argument.
// A number goes through value.NumberToString (the exact ECMAScript
// Number::toString, not strconv), a boolean through value.BoolToString, and a
// string is already a value.BStr so it passes through unchanged, the identity
// String(s) is. It takes exactly one argument; a different arity, or an argument
// this slice does not coerce (an object, whose ToString runs user code), hands
// back.
func (r *Renderer) stringCoercion(argNodes []frontend.Node) (ast.Expr, error) {
	// String() with no argument is the empty string, the coercion-edge count the
	// spec fixes: with no value to convert, the result is "" rather than "undefined".
	if len(argNodes) == 0 {
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `""`}}}, nil
	}
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "String() with this argument count is a later slice"}
	}
	return r.stringify(argNodes[0])
}

// stringify lowers one expression to its string form under the ECMAScript ToString
// used by String(x) and by a template literal substitution: a number goes through
// value.NumberToString (the exact Number::toString, not strconv), a boolean
// through value.BoolToString, and a string is already a value.BStr so it passes
// through unchanged. An argument this slice does not coerce (an object, whose
// ToString runs user code) hands back. String(x) and `${x}` share this so the two
// paths always agree on how a value becomes a string.
func (r *Renderer) stringify(arg frontend.Node) (ast.Expr, error) {
	// A caught error coerces through Error.prototype.toString, "Name: message", the
	// bento string the *value.Error produces directly, so String(err) and `${err}`
	// read the same as the engine. It routes before the general lower below, which
	// hands a caught error back outside a .message, .name, or .constructor read.
	if r.isCaughtErrorRef(arg) {
		r.requireImport(valuePkg)
		name, _ := localName(r.prog.Text(arg))
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("ToBStr")}}, nil
	}
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	switch {
	case r.producesBoxedValue(arg):
		// A call that already lowers to a value.Value box (a terminal iterator helper,
		// a descriptor read) defers the whole ToString to the value model, which
		// dispatches on the runtime kind, the same as any dynamic argument. This routes
		// before the primitive cases because the checker types the call by its result
		// (a number for reduce, an array for toArray) while the lowered expr is a box, so
		// the primitive stringifiers would be handed a value.Value they cannot take.
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "ToString"), Args: []ast.Expr{lowered}}, nil
	case r.isString(arg):
		return lowered, nil // already a string, the identity
	case r.isNumber(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "NumberToString"), Args: []ast.Expr{lowered}}, nil
	case r.isBool(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "BoolToString"), Args: []ast.Expr{lowered}}, nil
	case r.isBigInt(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "BigIntToString"), Args: []ast.Expr{lowered}}, nil
	case r.isDynamic(arg):
		// A dynamic argument defers the whole ToString to the value model,
		// which dispatches on the runtime kind.
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "ToString"), Args: []ast.Expr{lowered}}, nil
	case r.isOptional(arg):
		// A T | undefined optional stringifies through the value model: box it into a
		// dynamic value (a present element through its own constructor, an undefined one
		// the undefined singleton) and defer to value.ToString, which renders the present
		// value the way String does and the missing one as "undefined", the same result
		// String over a possibly-undefined value gives. An optional of a shape with no
		// dynamic box yet hands back through boxStaticToDynamic.
		boxed, err := r.boxStaticToDynamic(lowered, arg)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "ToString"), Args: []ast.Expr{boxed}}, nil
	default:
		// A tagged-sum union reads its string through the ToString method the renderer
		// emits for it: the method switches the tag to the active arm's string form, so
		// String(x) over a number | string | undefined renders the number, the string, or
		// "undefined" as the arm selects. The union is evaluated once by the method call.
		if _, ok := r.unionStringValued(arg); ok {
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: lowered, Sel: ident("ToString")}}, nil
		}
		return nil, &NotYetLowerable{Reason: "coercing this type to a string is a later slice"}
	}
}

// numberCoercion lowers Number(x) called as a function over a primitive argument.
// Number() with no argument is +0, the coercion-edge count the spec fixes: with no
// value to convert the result is zero, so it lowers to a float64 zero literal, the
// mirror of String()'s empty-string default. A string goes through
// value.StringToNumber (the exact ECMAScript ToNumber over the StrNumericLiteral
// grammar, not strconv), a boolean through value.BoolToNumber (true is 1, false is
// 0), and a number is already a float64 so it passes through unchanged. Any other
// arity, or an argument this slice does not coerce (an object, whose valueOf runs
// user code), hands back.
func (r *Renderer) numberCoercion(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) == 0 {
		// Number() with no argument is +0 (21.1.1.1). The literal carries a fractional
		// part so it prints as a float64 constant: a bare 0 is an untyped constant that
		// infers to int in `n := Number()`, which then fails to type-check against the
		// float64 a Number is everywhere else.
		return &ast.BasicLit{Kind: token.FLOAT, Value: "0.0"}, nil
	}
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Number() with this argument count is a later slice"}
	}
	arg := argNodes[0]
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	switch {
	case r.isNumber(arg):
		return lowered, nil // Number(n) on a number is the identity
	case r.isString(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "StringToNumber"), Args: []ast.Expr{lowered}}, nil
	case r.isBool(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "BoolToNumber"), Args: []ast.Expr{lowered}}, nil
	case r.isBigInt(arg):
		// Number(b) rounds the bigint to the nearest float64 the way JavaScript
		// does, losing low bits past 2^53 and saturating to an infinity past the
		// float64 range; value.BigIntToNumber is that rounding.
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "BigIntToNumber"), Args: []ast.Expr{lowered}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Number() on this argument type is a later slice"}
	}
}

// booleanCoercion lowers Boolean(x) called as a function over a primitive argument,
// the third primitive coercion. A number goes through value.NumberToBool (false
// only at zero or NaN), a string through value.StringToBool (false only when
// empty), and a boolean passes through unchanged since Boolean(b) on a boolean is
// the identity. It takes exactly one argument; a different arity, or an argument
// this slice does not coerce (an object, which is always truthy but whose
// evaluation this slice does not model), hands back.
func (r *Renderer) booleanCoercion(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Boolean() with this argument count is a later slice"}
	}
	arg := argNodes[0]
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	switch {
	case r.isBool(arg):
		return lowered, nil // Boolean(b) on a boolean is the identity
	case r.isNumber(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "NumberToBool"), Args: []ast.Expr{lowered}}, nil
	case r.isString(arg):
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "StringToBool"), Args: []ast.Expr{lowered}}, nil
	case r.isBigInt(arg):
		// Boolean(b) is the bigint truthiness: only 0n is false, a sign test.
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "BigIntToBool"), Args: []ast.Expr{lowered}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Boolean() on this argument type is a later slice"}
	}
}

// parseFloatCall lowers parseFloat(s) over a string argument to value.ParseFloat,
// the lenient prefix parse. It takes exactly one string argument; a different
// arity, or a non-string argument (which parseFloat would coerce to a string
// first, running that conversion), hands back.
func (r *Renderer) parseFloatCall(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "parseFloat with this argument count is a later slice"}
	}
	arg := argNodes[0]
	if !r.isString(arg) {
		return nil, &NotYetLowerable{Reason: "parseFloat on a non-string argument is a later slice"}
	}
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "ParseFloat"), Args: []ast.Expr{lowered}}, nil
}

// unaryStringGlobal lowers a bare global that takes exactly one string and returns
// one string to its value runtime function: the URI codecs encodeURIComponent /
// decodeURIComponent / encodeURI / decodeURI and the base64 codecs btoa / atob. A
// different arity, or a non-string argument (which the global would coerce to a
// string first, running that conversion), hands back. goName is the runtime
// function to call and jsName names the global for the handback reason.
func (r *Renderer) unaryStringGlobal(goName, jsName string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: jsName + " with this argument count is a later slice"}
	}
	arg := argNodes[0]
	if !r.isString(arg) {
		return nil, &NotYetLowerable{Reason: jsName + " on a non-string argument is a later slice"}
	}
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", goName), Args: []ast.Expr{lowered}}, nil
}

// symbolConstructor lowers Symbol() and Symbol(desc) to the boxed symbol value the
// property bag keys by identity. No argument builds the descriptionless symbol
// through value.NewSymbolNoDesc, keeping Symbol() apart from Symbol(""); a string
// description builds value.NewSymbol over the lowered string. A non-string
// description would need the ToString coercion the constructor applies and is a
// later slice, and more than one argument is not the constructor's shape.
func (r *Renderer) symbolConstructor(argNodes []frontend.Node) (ast.Expr, error) {
	r.requireImport(valuePkg)
	if len(argNodes) == 0 {
		return &ast.CallExpr{Fun: sel("value", "NewSymbolNoDesc")}, nil
	}
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Symbol called with more than one argument is a later slice"}
	}
	if !r.isString(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "Symbol with a non-string description is a later slice"}
	}
	desc, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: sel("value", "NewSymbol"), Args: []ast.Expr{desc}}, nil
}

// symbolStaticCall lowers the static calls on the ambient Symbol global that read
// the global symbol registry. Symbol.for(key) interns and returns one symbol per
// string key through value.SymbolFor, so two calls with an equal key share a
// reference; Symbol.keyFor(sym) reads back the key a registered symbol was
// interned under through value.SymbolKeyFor. Symbol.for takes a string and
// Symbol.keyFor a symbol; a different method or argument shape hands back.
func (r *Renderer) symbolStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "for":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Symbol.for with this argument count is a later slice"}
		}
		if !r.isString(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Symbol.for with a non-string key is a later slice"}
		}
		key, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "SymbolFor"), Args: []ast.Expr{key}}, nil
	case "keyFor":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Symbol.keyFor with this argument count is a later slice"}
		}
		if !r.isSymbol(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Symbol.keyFor on a non-symbol argument is a later slice"}
		}
		sym, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "SymbolKeyFor"), Args: []ast.Expr{sym}}, nil
	}
	return nil, &NotYetLowerable{Reason: "this static Symbol method is a later slice"}
}

// parseIntCall lowers parseInt(s) and parseInt(s, radix) to value.ParseInt. The
// first argument must be a string; the optional second must be a number and
// becomes the radix, while an omitted radix lowers to the literal 0, which
// value.ParseInt treats (as the specification does) the same as an omitted
// argument. A different arity or an argument of the wrong type hands back.
func (r *Renderer) parseIntCall(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) < 1 || len(argNodes) > 2 {
		return nil, &NotYetLowerable{Reason: "parseInt with this argument count is a later slice"}
	}
	if !r.isString(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "parseInt on a non-string argument is a later slice"}
	}
	str, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	// The radix argument, or the literal 0 when it is omitted.
	var radix ast.Expr = &ast.BasicLit{Kind: token.FLOAT, Value: "0"}
	if len(argNodes) == 2 {
		if !r.isNumber(argNodes[1]) {
			return nil, &NotYetLowerable{Reason: "parseInt with a non-number radix is a later slice"}
		}
		radix, err = r.lowerExpr(argNodes[1])
		if err != nil {
			return nil, err
		}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "ParseInt"), Args: []ast.Expr{str, radix}}, nil
}

// argKind names the primitive type a string method expects for one argument, so
// methodCall guards each argument against the checker's type before lowering it.
type argKind int

const (
	argNumber argKind = iota
	argString
)

// argHasKind reports whether the checker types n as the primitive the method
// expects at that position, the guard that keeps a mistyped argument out of a
// method call rather than emitting Go that would not compile or would coerce.
func (r *Renderer) argHasKind(n frontend.Node, k argKind) bool {
	switch k {
	case argString:
		return r.isString(n)
	default:
		return r.isNumber(n)
	}
}

// stringProtoMethodName reports whether n is the member chain String.prototype.<name>
// and returns the borrowed method's name. It matches a property access whose object
// is a property access .prototype whose object is the ambient global String, so a
// user value that carries a like-named method does not match and stays on the
// value-method path.
func (r *Renderer) stringProtoMethodName(n frontend.Node) (string, bool) {
	if n.Kind() != frontend.NodePropertyAccessExpression {
		return "", false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return "", false
	}
	proto := kids[0]
	if proto.Kind() != frontend.NodePropertyAccessExpression {
		return "", false
	}
	pkids := r.prog.Children(proto)
	if len(pkids) != 2 || r.prog.Text(pkids[1]) != "prototype" {
		return "", false
	}
	if !r.isGlobalRef(pkids[0], "String") {
		return "", false
	}
	return r.prog.Text(kids[1]), true
}

// stringProtoBorrowedCall lowers String.prototype.<name>.call(recv, ...) to the
// matching value.BStr method run on the receiver coerced to a string, the way the
// spec's ToString coerces the this value before the method runs. The receiver boxes
// to a value.Value and value.ToString reduces it to a BStr whatever its kind, so a
// number or boolean receiver stringifies exactly as the engine does. Only .call
// with an inline argument list is the borrow form the tests use; .apply and a method
// the string table does not cover hand back so a borrow is never mislowered. Each
// method argument is checked against the method's declared kinds, the same guard the
// direct string-method path applies.
func (r *Renderer) stringProtoBorrowedCall(name, method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "call" {
		return nil, &NotYetLowerable{Reason: "String.prototype." + name + " borrowed with ." + method + " is a later slice"}
	}
	if len(argNodes) == 0 {
		return nil, &NotYetLowerable{Reason: "String.prototype." + name + " borrowed with no receiver is a later slice"}
	}
	recv, err := r.boxOperand(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	str := &ast.CallExpr{Fun: sel("value", "ToString"), Args: []ast.Expr{recv}}
	methodArgs := argNodes[1:]
	// toString and valueOf on the coerced string are identity, so they read as the
	// coerced BStr with no further call, the same shortcut the direct path takes.
	if name == "toString" || name == "valueOf" {
		if len(methodArgs) != 0 {
			return nil, &NotYetLowerable{Reason: "borrowed string ." + name + " with an argument is a later slice"}
		}
		return str, nil
	}
	goName, params, minArgs, variadic, ok := stringMethod(name)
	if !ok {
		return nil, &NotYetLowerable{Reason: "borrowed string method ." + name + " is a later slice"}
	}
	if len(methodArgs) < minArgs || (!variadic && len(methodArgs) > len(params)) {
		return nil, &NotYetLowerable{Reason: "borrowed string method ." + name + " with this argument count is a later slice"}
	}
	args := make([]ast.Expr, 0, len(methodArgs))
	for i, a := range methodArgs {
		idx := i
		if idx >= len(params) {
			idx = len(params) - 1
		}
		if !r.argHasKind(a, params[idx]) {
			return nil, &NotYetLowerable{Reason: "borrowed string method ." + name + " with an argument of the wrong type is a later slice"}
		}
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: str, Sel: ident(goName)}, Args: args}, nil
}

// stringMethod maps a JavaScript string method to the value.BStr method that
// implements it, the primitive kind of each argument, and the minimum number of
// arguments a call must supply. The argument kinds let methodCall guard a
// string-taking method (indexOf) apart from a number-taking one (charCodeAt).
// minArgs below len(params) marks the trailing arguments optional: slice and
// substring take zero, one, or two numbers, and their Go methods are variadic so
// one signature covers every arity, the count selecting the defaults. The kinds
// need not all match: padStart takes a required number then an optional string,
// so the guard admits one or two arguments and still checks each against its
// declared kind. A call always passes exactly the arguments the source wrote, so
// the emitted call form is the same whether the method is variadic or not.
func stringMethod(name string) (goName string, params []argKind, minArgs int, variadic bool, ok bool) {
	switch name {
	case "at":
		// at returns string | undefined, so the Go method returns an Opt[BStr]; a
		// negative index counts from the end and an out-of-range index reads as the
		// undefined optional, consumed through the same !== undefined narrowing the
		// array at read uses. It takes exactly one number.
		return "AtOpt", []argKind{argNumber}, 1, false, true
	case "charCodeAt":
		return "CharCodeAt", []argKind{argNumber}, 1, false, true
	case "codePointAt":
		// codePointAt returns number | undefined, so the Go method returns an
		// Opt[float64], consumed through the same !== undefined narrowing; it combines
		// a surrogate pair into the astral code point rather than reading one unit. It
		// takes exactly one number.
		return "CodePointAtOpt", []argKind{argNumber}, 1, false, true
	case "charAt":
		return "CharAt", []argKind{argNumber}, 1, false, true
	case "indexOf":
		return "IndexOf", []argKind{argString, argNumber}, 1, false, true
	case "lastIndexOf":
		return "LastIndexOf", []argKind{argString, argNumber}, 1, false, true
	case "includes":
		return "Includes", []argKind{argString, argNumber}, 1, false, true
	case "replace":
		// Only the string-pattern, string-replacement form lowers; a regexp pattern
		// or a replacer function argument does not type as a string, so methodCall
		// hands it back. Both arguments are required, so it is not variadic.
		return "Replace", []argKind{argString, argString}, 2, false, true
	case "replaceAll":
		return "ReplaceAll", []argKind{argString, argString}, 2, false, true
	case "split":
		// Only the string-separator form lowers, to value.BStr.Split returning a
		// string array; a regexp separator does not type as a string, so methodCall
		// hands it back, and the optional limit argument is a later slice, so exactly
		// one argument is admitted.
		return "Split", []argKind{argString}, 1, false, true
	case "startsWith":
		return "StartsWith", []argKind{argString, argNumber}, 1, false, true
	case "endsWith":
		return "EndsWith", []argKind{argString, argNumber}, 1, false, true
	case "slice":
		return "Slice", []argKind{argNumber, argNumber}, 0, false, true
	case "substring":
		return "Substring", []argKind{argNumber, argNumber}, 0, false, true
	case "substr":
		// The legacy start-and-length form: a required start and an optional length,
		// both numbers, so it admits one or two arguments like slice and substring.
		return "Substr", []argKind{argNumber, argNumber}, 1, false, true
	case "padStart":
		return "PadStart", []argKind{argNumber, argString}, 1, false, true
	case "padEnd":
		return "PadEnd", []argKind{argNumber, argString}, 1, false, true
	case "concat":
		// concat takes any number of string arguments, so it is variadic over a
		// single repeating string kind and has no upper bound.
		return "ConcatN", []argKind{argString}, 0, true, true
	case "repeat":
		// repeat takes exactly one number, the count. value.Repeat coerces it the
		// way String.prototype.repeat does and treats a negative or non-finite count
		// as the RangeError it is, so a bad count is caught at runtime rather than
		// miscompiled.
		return "Repeat", []argKind{argNumber}, 1, false, true
	case "toUpperCase":
		return "ToUpperCase", nil, 0, false, true
	case "toLowerCase":
		return "ToLowerCase", nil, 0, false, true
	case "trim":
		return "Trim", nil, 0, false, true
	case "trimStart":
		return "TrimStart", nil, 0, false, true
	case "trimEnd":
		return "TrimEnd", nil, 0, false, true
	case "isWellFormed":
		// A lone-surrogate predicate returning a boolean; no arguments.
		return "IsWellFormed", nil, 0, false, true
	case "toWellFormed":
		// Returns a copy with each lone surrogate replaced by U+FFFD; no arguments.
		return "ToWellFormed", nil, 0, false, true
	case "normalize":
		// normalize takes an optional form name and defaults to NFC when it is
		// omitted, so it admits zero or one string argument. value.Normalize throws
		// the RangeError for a form that is not one of the four the specification
		// allows, so a bad name is caught at runtime rather than miscompiled.
		return "Normalize", []argKind{argString}, 0, false, true
	default:
		return "", nil, 0, false, false
	}
}
