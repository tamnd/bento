package lower

// This file lowers URL and URLSearchParams, the WHATWG pair Node and the web platform
// expose as globals and as the node:url module. A compiled program that wrote
// `new URL(...)` handed back until now, which was a shame twice over: URL is one of
// the first things a real .js entry reaches for, and the parse it needs already lives
// in Go, in pkg/nodehost, where the interpreter reaches it through a JSON bridge.
//
// Both lower to a Go type rather than to a boxed property bag, the same shape
// TextEncoder and Date take. searchParams returns a URLSearchParams and getAll returns
// an array, both typed-world values, so neither could be handed back out of a
// value.Value.
//
// params.get(name) is the exception. The specification's answer for an absent name is
// null, and there is no string that means "no such parameter", so its result is the
// boxed value.Value the runtime returns and the call is recognized by shape on the
// dynamic path, exactly the way re.exec's array-or-null is.

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// isURLType reports whether an object type is a URL. searchParams is unique to it
// among the standard library's types, and pairing it with href keeps a plain options
// bag that happens to carry a searchParams field from matching, the same two-property
// fingerprint the TextDecoder check takes.
func (r *Renderer) isURLType(t frontend.Type) bool {
	var hasHref, hasSearchParams bool
	for _, p := range r.prog.Properties(t) {
		switch p.Name {
		case "href":
			hasHref = true
		case "searchParams":
			hasSearchParams = true
		}
	}
	return hasHref && hasSearchParams
}

// isURL reports whether the checker types a node as a URL, the receiver test the URL
// lowerings share.
func (r *Renderer) isURL(n frontend.Node) bool {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	return r.isURLType(t)
}

// isURLSearchParamsType reports whether an object type is a URLSearchParams. getAll is
// unique to it, and append alongside separates it from a bag that merely names a
// getAll callback.
func (r *Renderer) isURLSearchParamsType(t frontend.Type) bool {
	var hasGetAll, hasAppend bool
	for _, p := range r.prog.Properties(t) {
		switch p.Name {
		case "getAll":
			hasGetAll = true
		case "append":
			hasAppend = true
		}
	}
	return hasGetAll && hasAppend
}

// isURLSearchParams reports whether the checker types a node as a URLSearchParams.
func (r *Renderer) isURLSearchParams(n frontend.Node) bool {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	return r.isURLSearchParamsType(t)
}

// newURL lowers new URL(input) and new URL(input, base) to value.NewURL, whose base is
// variadic. Both arguments must be strings: the standard library also admits a URL as
// the base, and the specification coerces anything else through String(), so a
// non-string argument hands back rather than pass a value the constructor has no slot
// for.
func (r *Renderer) newURL(args []frontend.Node) (ast.Expr, error) {
	valueArgs := r.namedArgs(args)
	if len(valueArgs) == 0 || len(valueArgs) > 2 {
		return nil, &NotYetLowerable{Reason: "new URL takes an input and an optional base"}
	}
	lowered, err := r.urlStringArgs("new URL", valueArgs)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "NewURL"), Args: lowered}, nil
}

// newURLSearchParams lowers new URLSearchParams(init). Three inits are covered: none,
// a query string, and another URLSearchParams, which copies its pairs. The record and
// the array-of-pairs forms the standard library also declares need a walk of an object
// or a nested tuple to reach the constructor's parameters, so they hand back naming
// themselves.
func (r *Renderer) newURLSearchParams(args []frontend.Node) (ast.Expr, error) {
	valueArgs := r.namedArgs(args)
	if len(valueArgs) > 1 {
		return nil, &NotYetLowerable{Reason: "new URLSearchParams takes at most one argument"}
	}
	r.requireImport(valuePkg)
	if len(valueArgs) == 0 {
		return &ast.CallExpr{Fun: sel("value", "NewURLSearchParams")}, nil
	}
	if r.isURLSearchParams(valueArgs[0]) {
		other, err := r.lowerExpr(valueArgs[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: sel("value", "NewURLSearchParamsFrom"), Args: []ast.Expr{other}}, nil
	}
	if !r.isString(valueArgs[0]) {
		return nil, &NotYetLowerable{Reason: "new URLSearchParams over a record or an array of pairs is a later slice"}
	}
	query, err := r.lowerExpr(valueArgs[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: sel("value", "NewURLSearchParams"), Args: []ast.Expr{query}}, nil
}

// urlStaticCall lowers a static call on the global URL. canParse asks whether
// construction would succeed without paying for the exception, which is why the
// specification added it. The file-path helpers Node hangs off the same constructor
// are the node:url module's surface, a later slice.
func (r *Renderer) urlStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "canParse" {
		return nil, &NotYetLowerable{Reason: "the static URL." + method + " is a later slice"}
	}
	args := r.namedArgs(argNodes)
	if len(args) == 0 || len(args) > 2 {
		return nil, &NotYetLowerable{Reason: "URL.canParse takes an input and an optional base"}
	}
	lowered, err := r.urlStringArgs("URL.canParse", args)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "URLCanParse"), Args: lowered}, nil
}

// urlStringArgs lowers the input and optional base a URL construction or canParse
// takes, holding both to string. The runtime parses text, so a non-string argument
// would need the String() coercion the specification applies first, which is a later
// slice; naming the caller keeps the handback readable.
func (r *Renderer) urlStringArgs(what string, args []frontend.Node) ([]ast.Expr, error) {
	out := make([]ast.Expr, 0, len(args))
	for _, a := range args {
		if !r.isString(a) {
			return nil, &NotYetLowerable{Reason: what + " with an argument that is not a string needs coercion, a later slice"}
		}
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		out = append(out, lowered)
	}
	return out, nil
}

// urlMethodCall lowers a method call on a URL receiver. toString and toJSON are the
// whole of the method surface: everything else a URL exposes is an accessor, which
// urlGetter handles.
func (r *Renderer) urlMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	var goName string
	switch method {
	case "toString":
		goName = "ToString"
	case "toJSON":
		goName = "ToJSON"
	default:
		return nil, &NotYetLowerable{Reason: "URL method ." + method + " is a later slice"}
	}
	if len(r.namedArgs(argNodes)) != 0 {
		return nil, &NotYetLowerable{Reason: "url." + method + " takes no argument"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}}, nil
}

// urlSearchParamsMethods maps each covered URLSearchParams method to its runtime name
// and the argument count it takes. get, getAll, has and delete read or remove by name;
// append and set write a pair; sort orders by name; toString serializes.
var urlSearchParamsMethods = map[string]struct {
	goName string
	args   int
}{
	"append":   {"Append", 2},
	"delete":   {"Delete", 1},
	"get":      {"Get", 1},
	"getAll":   {"GetAll", 1},
	"has":      {"Has", 1},
	"set":      {"Set", 2},
	"sort":     {"Sort", 0},
	"toString": {"ToString", 0},
}

// urlSearchParamsMethodCall lowers a method call on a URLSearchParams receiver. Every
// argument is a name or a value, so each is held to string: the specification coerces
// through String() first, which is a later slice. forEach takes a callback and routes
// to its own lowering. keys, values and entries mint an iterator over pairs this slice
// does not materialize, so they hand back naming themselves.
//
// delete(name, value) and has(name, value), the two-argument forms a recent revision
// added, filter on the value as well; the runtime takes the name alone, so those hand
// back rather than silently ignore the second argument and delete too much.
func (r *Renderer) urlSearchParamsMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method == "forEach" {
		return r.urlSearchParamsForEach(recvNode, argNodes)
	}
	if method == "keys" || method == "values" || method == "entries" {
		return nil, &NotYetLowerable{Reason: "a URLSearchParams ." + method + "() iterator is a later slice"}
	}
	m, ok := urlSearchParamsMethods[method]
	if !ok {
		return nil, &NotYetLowerable{Reason: "URLSearchParams method ." + method + " is a later slice"}
	}
	args := r.namedArgs(argNodes)
	if len(args) != m.args {
		if (method == "delete" || method == "has") && len(args) == 2 {
			return nil, &NotYetLowerable{Reason: "a URLSearchParams ." + method + " that also filters on the value is a later slice"}
		}
		return nil, &NotYetLowerable{Reason: "params." + method + " with this argument count is a later slice"}
	}
	lowered := make([]ast.Expr, 0, len(args))
	for _, a := range args {
		if !r.isString(a) {
			return nil, &NotYetLowerable{Reason: "params." + method + " with an argument that is not a string needs coercion, a later slice"}
		}
		e, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		lowered = append(lowered, e)
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(m.goName)}, Args: lowered}, nil
}

// urlSearchParamsForEach lowers params.forEach(cb), the insertion-order walk. Only an
// inline arrow is covered, the same restriction map.forEach takes: a one-parameter
// arrow reads the value and lowers to ForEachValue, and a two-parameter arrow reads the
// value then the name, the order the specification passes them, and lowers to ForEach.
// A callback passed by name, or one that also reads the params parameter, is a later
// slice, and so is a thisArg, which is inert for an arrow's lexical this but would have
// its evaluation dropped.
func (r *Renderer) urlSearchParamsForEach(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 || argNodes[0].Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "params.forEach with a callback that is not a single inline arrow function is a later slice"}
	}
	var goName string
	switch r.arrowParamCount(argNodes[0]) {
	case 1:
		goName = "ForEachValue"
	case 2:
		goName = "ForEach"
	default:
		return nil, &NotYetLowerable{Reason: "params.forEach with a callback that also reads the params parameter is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	fn, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	// forEach discards the callback's result, so the runtime parameter is a func with no
	// result. An arrow with an expression body lowers to a func that returns that body's
	// value, so it is wrapped to run for effect, the same adapter array forEach takes.
	if lit, ok := fn.(*ast.FuncLit); ok && lit.Type.Results != nil {
		fn = r.dropFuncResult(lit)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}, Args: []ast.Expr{fn}}, nil
}

// urlGetters maps each URL accessor to its runtime method. Every component the
// specification exposes is here, so a program can read the whole parse.
var urlGetters = map[string]string{
	"href":         "Href",
	"protocol":     "Protocol",
	"username":     "Username",
	"password":     "Password",
	"host":         "Host",
	"hostname":     "Hostname",
	"port":         "Port",
	"pathname":     "Pathname",
	"search":       "Search",
	"hash":         "Hash",
	"origin":       "Origin",
	"searchParams": "SearchParams",
}

// urlGetter lowers an accessor read on a URL or URLSearchParams receiver. Each is an
// accessor in the source and a method on the runtime type, the same shape map.size
// takes, so it routes before the struct-field path would try to intern the name as a
// field of a shape neither type has. It reports false for any other read so the caller
// falls through.
func (r *Renderer) urlGetter(obj frontend.Node, prop string) (ast.Expr, bool, error) {
	var goName string
	switch {
	case r.isURL(obj):
		goName = urlGetters[prop]
	case r.isURLSearchParams(obj) && prop == "size":
		goName = "Size"
	}
	if goName == "" {
		return nil, false, nil
	}
	recv, err := r.lowerExpr(obj)
	if err != nil {
		return nil, false, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}}, true, nil
}

// urlSearchParamsGetCall reports whether n is a params.get(name) call, whose runtime
// result is the boxed value.Value the lookup returns, the string or null. The checker
// types get as string | null, a union bento renders as a tagged sum the runtime has no
// way to name, so the call is recognized by shape here to keep the box on the dynamic
// path, where the null compare and any read off the result dispatch through the value
// model. It is the same treatment re.exec's array-or-null takes.
func (r *Renderer) urlSearchParamsGetCall(n frontend.Node) bool {
	if n.Kind() != frontend.NodeCallExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	parts := r.prog.Children(kids[0])
	if len(parts) != 2 {
		return false
	}
	return r.prog.Text(parts[1]) == "get" && r.isURLSearchParams(parts[0])
}
