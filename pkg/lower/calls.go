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

// callExpr lowers a call to a top-level function. The callee must be an
// identifier that resolves to a function symbol, lowered to the same exported Go
// name RenderFunc gives the declaration, so a call and its target agree. Calling
// a local closure, a method, or a value is a later slice. Arguments lower
// positionally; a spread or a defaulted or omitted argument hands back.
func (r *Renderer) callExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		return nil, &NotYetLowerable{Reason: "call expression exposed no callee"}
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
		return r.methodCall(kids[0], kids[1:])
	}
	if kids[0].Kind() != frontend.NodeIdentifier {
		return nil, &NotYetLowerable{Reason: "call to a non-identifier callee is a later slice"}
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
	sym, ok := r.prog.SymbolAt(kids[0])
	if !ok || sym.Flags&frontend.SymbolFunction == 0 {
		return nil, &NotYetLowerable{Reason: "call to a callee that is not a top-level function is a later slice"}
	}
	name, ok := exportedField(sym.Name)
	if !ok {
		return nil, &NotYetLowerable{Reason: "called function name is not a Go identifier"}
	}
	// Arguments lower positionally, each bridged against its declared
	// parameter, so a derived instance passed for a base parameter upcasts to
	// the embedded base the same way an assignment would.
	var params []frontend.Param
	if sig, ok := r.prog.SignatureAt(n); ok {
		params = sig.Params
	}
	args := make([]ast.Expr, 0, len(kids)-1)
	for i, a := range kids[1:] {
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		if i < len(params) {
			if boxed, ok, berr := r.boxToOptional(lowered, a, params[i].Type); berr != nil {
				return nil, berr
			} else if ok {
				lowered = boxed
			} else {
				lowered, err = r.bridgeClassBinding(lowered, a, params[i].Type)
				if err != nil {
					return nil, err
				}
			}
		}
		args = append(args, lowered)
	}
	return &ast.CallExpr{Fun: ident(name), Args: args}, nil
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
	// A static call A.m(...) lowers to the package function the static method
	// became. The class name's type shares the class symbol an instance walks
	// to, so this routes before the instance path below.
	if recvNode.Kind() == frontend.NodeIdentifier {
		if info, ok := r.classNameRef(recvNode); ok {
			return r.staticMethodCall(info, method, argNodes)
		}
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
	// A method on an array receiver lowers to a value.Array method. This routes
	// before the primitive and string paths, which expect a number, boolean, or
	// string receiver an array is not.
	if _, ok := r.arrayElem(recvNode); ok {
		return r.arrayMethodCall(recvNode, method, argNodes)
	}
	// A method on a Map receiver lowers to a value.Map method (section 6.5). This
	// routes before the primitive and string paths, which expect a number, boolean,
	// or string receiver a map is not.
	if r.isMap(recvNode) {
		return r.mapMethodCall(recvNode, method, argNodes)
	}
	// toString and valueOf on a number or a boolean value are the first methods on
	// a non-string receiver: they lower to the same coercion a String() call or a
	// bare use would take, so they route here before the string-method path.
	if r.isNumber(recvNode) || r.isBool(recvNode) {
		return r.primitiveValueCall(recvNode, method, argNodes)
	}
	if !r.isString(recvNode) {
		return nil, &NotYetLowerable{Reason: "method call on a non-string receiver is a later slice"}
	}
	// replace and replaceAll with a regexp literal first argument are their own
	// path: a plain-literal pattern (no metacharacters) is exactly the string
	// search the value replace methods do, so it lowers when the pattern is plain
	// and the flags are a subset bento models, and hands back otherwise so a real
	// pattern routes to the engine rather than compiling a wrong search.
	if method == "replace" || method == "replaceAll" {
		if len(argNodes) >= 1 {
			if pattern, flags, isRe := r.regexLiteralArg(argNodes[0]); isRe {
				return r.regexReplaceCall(recvNode, method, pattern, flags, argNodes)
			}
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

// stringStaticCall lowers a static call on the global String constructor. Only
// fromCharCode is covered here: it takes any number of number arguments, coerces
// each to a UTF-16 code unit, and returns a string, so it maps to the variadic
// value.FromCharCode. fromCodePoint waits for the exception machinery, since it
// throws a RangeError on a code point outside the Unicode range. Like Math and
// Number, String is a namespace on this path, not a value, so the receiver is
// not lowered.
func (r *Renderer) stringStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "fromCharCode" {
		return nil, &NotYetLowerable{Reason: "String." + method + " is a later slice"}
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		if !r.isNumber(a) {
			return nil, &NotYetLowerable{Reason: "String.fromCharCode with a non-number argument is a later slice"}
		}
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "FromCharCode"), Args: args}, nil
}

// jsonCall lowers a static call on the global JSON namespace. stringify takes a
// single value and returns the exact text V8 produces, which lowers to
// value.JSONStringify with the argument boxed as any so the serializer's
// reflection walk can dispatch on its concrete type. parse takes a single string
// and returns a dynamic any value, which lowers to value.JSONParse and lands in
// the boxed value world the checker already typed the result as. A replacer, a
// space, or a reviver argument (the extra parameters) changes the behavior, so a
// call that passes one hands back rather than ignoring it.
func (r *Renderer) jsonCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "stringify":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "JSON.stringify with a replacer or space argument is a later slice"}
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
		return &ast.CallExpr{Fun: sel("value", "JSONStringify"), Args: []ast.Expr{arg}}, nil
	case "parse":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "JSON.parse with a reviver argument is a later slice"}
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "JSONParse"), Args: []ast.Expr{arg}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "JSON." + method + " is a later slice"}
	}
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
	// slice covers. The radix must be a literal in 2..36 so no RangeError can fire
	// (a bad radix throws, which waits on the exception machinery, and a dynamic
	// radix cannot be range-checked at compile time). A radix of 10 is the same
	// coercion String(x) runs, so it routes through stringify; any other radix
	// lowers to value.NumberToStringRadix with the literal folded in.
	if method == "toString" && len(argNodes) == 1 && r.isNumber(recvNode) {
		radix, ok := r.literalIntArg(argNodes[0], 2, 36)
		if !ok {
			return nil, &NotYetLowerable{Reason: "number toString with a non-literal or out-of-range radix is a later slice"}
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
	// number.toFixed(digits) formats with a fixed number of fraction digits. The
	// digit count must be a literal in 0..100 for the same reason the radix must:
	// a count outside that range throws a RangeError, and a dynamic count cannot be
	// range-checked at compile time. An omitted count means zero. It lowers to
	// value.NumberToFixed, which rounds the exact double the way the specification
	// does.
	if method == "toFixed" && len(argNodes) <= 1 && r.isNumber(recvNode) {
		digits := 0
		if len(argNodes) == 1 {
			d, ok := r.literalIntArg(argNodes[0], 0, 100)
			if !ok {
				return nil, &NotYetLowerable{Reason: "number toFixed with a non-literal or out-of-range digit count is a later slice"}
			}
			digits = d
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
	// digits fraction digits. The count must be a literal in 0..100 for the reason
	// the toFixed count must: a value outside that range throws a RangeError, and a
	// dynamic count cannot be range-checked at compile time. The omitted-count form
	// uses as many digits as the value needs, a different rule, so it hands back
	// rather than defaulting to zero the way toFixed's omitted count does. It lowers
	// to value.NumberToExponential, which rounds the exact double the way the
	// specification does.
	if method == "toExponential" && len(argNodes) == 1 && r.isNumber(recvNode) {
		digits, ok := r.literalIntArg(argNodes[0], 0, 100)
		if !ok {
			return nil, &NotYetLowerable{Reason: "number toExponential with a non-literal or out-of-range digit count is a later slice"}
		}
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
	// The precision must be a literal in 1..100 (not 0..100 like the other two: zero
	// significant digits is not a valid precision and throws), for the same reason
	// the others take a literal in range: a value outside throws a RangeError and a
	// dynamic count cannot be range-checked at compile time. The omitted form is
	// Number::toString, a different rule, so it hands back rather than defaulting. It
	// lowers to value.NumberToPrecision, which shares toExponential's exact rounding.
	if method == "toPrecision" && len(argNodes) == 1 && r.isNumber(recvNode) {
		precision, ok := r.literalIntArg(argNodes[0], 1, 100)
		if !ok {
			return nil, &NotYetLowerable{Reason: "number toPrecision with a non-literal or out-of-range precision is a later slice"}
		}
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
// functions cannot. The transcendental functions (cbrt, exp, expm1, the logs, the
// trig and inverse-trig and hyperbolic families, atan2, and the two-argument
// hypot) map straight onto the Go math package too, but their last-bit results are
// not guaranteed identical across two libm implementations, so they are proven by
// the equivalence harness's numeric-tolerance mode rather than by an exact match.
// hypot stays two-argument because math.Hypot takes exactly two; the variadic
// Math.hypot(a, b, c) hands back until a value.HypotN folds a list.
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
		return "math", "Pow", 2, 2, true
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
		return "math", "Hypot", 2, 2, true
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
	return r.stringify(arg)
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
	lowered, err := r.lowerExpr(arg)
	if err != nil {
		return nil, err
	}
	switch {
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
	default:
		return nil, &NotYetLowerable{Reason: "coercing this type to a string is a later slice"}
	}
}

// numberCoercion lowers Number(x) called as a function over a primitive argument.
// A string goes through value.StringToNumber (the exact ECMAScript ToNumber over
// the StrNumericLiteral grammar, not strconv), a boolean through value.BoolToNumber
// (true is 1, false is 0), and a number is already a float64 so it passes through
// unchanged. It takes exactly one argument; a different arity, or an argument this
// slice does not coerce (an object, whose valueOf runs user code), hands back.
func (r *Renderer) numberCoercion(argNodes []frontend.Node) (ast.Expr, error) {
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
	case "charCodeAt":
		return "CharCodeAt", []argKind{argNumber}, 1, false, true
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
	default:
		return "", nil, 0, false, false
	}
}
