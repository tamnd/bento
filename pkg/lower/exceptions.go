package lower

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the exception surface: constructing a built-in error and
// throwing it. A throw becomes a Go panic carrying a value.Error, and the
// assembled main defers value.ReportUncaught so a throw that escapes every catch
// prints an uncaught-error line and exits non-zero rather than crashing with a Go
// stack (10_value_model.md, and 16_go_interop.md section 7.7 for the boundary
// errors that share the same reporting). Recovering a throw into a catch binding
// is a later slice; this one covers raising and reporting, the half a program
// needs before try/catch can catch anything.

// errorCtors maps a built-in error constructor name to the value constructor its
// new expression lowers to. Only the errors bento's own lowerings raise (a plain
// Error, a TypeError from a failed guard, a RangeError from a range check, a
// SyntaxError from BigInt string parsing) are covered; the DOM and Node error
// subclasses are a later slice, and an unlisted name hands back so a user class
// named Error-something is never mistaken for a built-in.
var errorCtors = map[string]string{
	"Error":       "NewError",
	"TypeError":   "NewTypeError",
	"RangeError":  "NewRangeError",
	"SyntaxError": "NewSyntaxError",
}

// newExpr lowers a new expression. Only the built-in error constructors are
// covered: new Error(message) and its TypeError and RangeError siblings lower to
// the matching value constructor, taking a single string message or none. A
// constructor bento does not recognize, or an error constructor called with a
// non-string or with more than one argument, hands back to the engine.
func (r *Renderer) newExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		return nil, &NotYetLowerable{Reason: "new expression did not expose a constructor"}
	}
	// A user class constructs through its generated NewX function. The name is
	// resolved through the constructor identifier's symbol, not its spelling, so a
	// local class named Map or Uint8Array still constructs as the class and never
	// falls through to a built-in with the same name.
	if kids[0].Kind() == frontend.NodeIdentifier {
		if info, ok := r.classNameRef(kids[0]); ok {
			return r.newClass(info, kids[1:])
		}
	}
	if r.prog.Text(kids[0]) == "Uint8Array" {
		return r.newTypedArray("Uint8Array", kids[1:])
	}
	if _, ok := typedArrayElemGo(r.prog.Text(kids[0])); ok {
		return r.newTypedArray(r.prog.Text(kids[0]), kids[1:])
	}
	if r.prog.Text(kids[0]) == "Map" {
		return r.newMap(n, kids[1:])
	}
	if r.prog.Text(kids[0]) == "Set" {
		return r.newSet(n, kids[1:])
	}
	ctor, ok := errorCtors[r.prog.Text(kids[0])]
	if !ok {
		return nil, &NotYetLowerable{Reason: "new of a constructor other than a built-in error is a later slice"}
	}
	args := kids[1:]
	if len(args) > 1 {
		return nil, &NotYetLowerable{Reason: "an error constructed with more than a message is a later slice"}
	}
	var message ast.Expr
	if len(args) == 0 {
		// new Error() with no argument carries an empty message, the same shape as
		// new Error(""), so it lowers to an empty bento string.
		message = r.bstrLit(nil)
	} else {
		if !r.isString(args[0]) {
			return nil, &NotYetLowerable{Reason: "an error constructed from a non-string message is a later slice"}
		}
		lowered, err := r.lowerExpr(args[0])
		if err != nil {
			return nil, err
		}
		message = lowered
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", ctor), Args: []ast.Expr{message}}, nil
}

// newTypedArray lowers a typed-array construction, the fixed-width numeric buffers
// of section 6.3, for the whole family by name. Two forms are covered. new
// Int32Array(n) allocates a zeroed buffer of length n, so a single Number argument
// lowers to value.NewInt32Array(n). new Int32Array([a, b, c]) fills a buffer from a
// list of values, so a single array-literal argument lowers to value.Int32ArrayOf(a,
// b, c) with each element coerced by the element kind's store rule. The two are told
// apart by the argument's syntax, an array literal versus anything else, so a length
// that happens to be a variable still takes the length form. The constructor names
// follow the New<Name> and <Name>Of scheme every family member shares, so the name
// alone selects them. The other overloads (a copy from another typed array, a view
// over an ArrayBuffer with an offset and length) are later slices and hand back, as
// does a call with no argument or more than one.
func (r *Renderer) newTypedArray(name string, args []frontend.Node) (ast.Expr, error) {
	if len(args) != 1 {
		return nil, &NotYetLowerable{Reason: "only new " + name + "(length) and new " + name + "([...]) are lowered yet"}
	}
	r.requireImport(valuePkg)
	if args[0].Kind() == frontend.NodeArrayLiteralExpression {
		elems := r.prog.Children(args[0])
		lowered := make([]ast.Expr, 0, len(elems))
		for _, e := range elems {
			if e.Kind() == frontend.NodeSpreadElement {
				return nil, &NotYetLowerable{Reason: "spread element in a " + name + " initializer is a later slice"}
			}
			if !r.isNumber(e) {
				return nil, &NotYetLowerable{Reason: "a " + name + " initialized from a non-number element is a later slice"}
			}
			v, err := r.lowerExpr(e)
			if err != nil {
				return nil, err
			}
			lowered = append(lowered, v)
		}
		return &ast.CallExpr{Fun: sel("value", name+"Of"), Args: lowered}, nil
	}
	if !r.isNumber(args[0]) {
		return nil, &NotYetLowerable{Reason: "a " + name + " length that is not a number is a later slice"}
	}
	length, err := r.lowerExpr(args[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: sel("value", "New"+name), Args: []ast.Expr{length}}, nil
}

// newMap lowers a Map construction, the keyed collection of section 6.5. Only the
// empty new Map<K, V>() is covered: it picks the value constructor for the key kind
// (a number, string, or boolean key each compares by its own SameValueZero) and
// instantiates it at the value type, so new Map<string, number>() lowers to
// value.NewStringMap[float64](). The key and value come from the map's own type at
// this node, read off its set signature the same way renderMap reads them, so the
// instantiation matches the type a binding of the map is declared with. The
// entries-argument form (new Map([[k, v], ...])) and a non-primitive key are later
// slices and hand back.
func (r *Renderer) newMap(n frontend.Node, args []frontend.Node) (ast.Expr, error) {
	// The children of a new expression carry the written type arguments (new
	// Map<string, number>()) ahead of the value arguments, and the frontend leaves a
	// type node unnamed, so it reads as NodeUnknown. Only a real value argument (the
	// entries list) has a named kind, so counting the named children is what tells the
	// empty constructor from the entries form.
	valueArgs := 0
	for _, a := range args {
		if a.Kind() != frontend.NodeUnknown {
			valueArgs++
		}
	}
	if valueArgs != 0 {
		return nil, &NotYetLowerable{Reason: "only the empty new Map() is lowered yet, not the entries-argument form"}
	}
	k, v, ok := r.mapKeyVal(r.prog.TypeAt(n))
	if !ok {
		return nil, &NotYetLowerable{Reason: "new Map did not expose its key and value types"}
	}
	var ctor string
	switch {
	case k.Flags&frontend.TypeNumber != 0:
		ctor = "NewNumberMap"
	case k.Flags&frontend.TypeString != 0:
		ctor = "NewStringMap"
	case k.Flags&frontend.TypeBoolean != 0:
		ctor = "NewBoolMap"
	default:
		return nil, &NotYetLowerable{Reason: "a Map with a key that is not a number, string, or boolean is a later slice"}
	}
	vExpr, err := r.typeExpr(v)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: &ast.IndexExpr{X: sel("value", ctor), Index: vExpr}}, nil
}

// newSet lowers a Set construction, the collection of unique members of section
// 6.5. Only the empty new Set<T>() is covered: it picks the value constructor for
// the member kind (a number, string, or boolean member each compares by its own
// SameValueZero), so new Set<string>() lowers to value.NewStringSet(). Unlike a Map,
// whose value type is free and so needs a type argument, the member type fully
// determines the Set constructor, so the call carries no instantiation. The member
// type comes from the set's own type at this node, read off its add signature the
// same way renderSet reads it. The iterable-argument form (new Set([a, b, c])) and a
// non-primitive member are later slices and hand back.
func (r *Renderer) newSet(n frontend.Node, args []frontend.Node) (ast.Expr, error) {
	// As with new Map, the written type argument (new Set<string>()) reads as a
	// NodeUnknown child ahead of any value argument, so counting the named children is
	// what tells the empty constructor from the iterable form.
	valueArgs := 0
	for _, a := range args {
		if a.Kind() != frontend.NodeUnknown {
			valueArgs++
		}
	}
	if valueArgs != 0 {
		return nil, &NotYetLowerable{Reason: "only the empty new Set() is lowered yet, not the iterable-argument form"}
	}
	elem, ok := r.setElem(r.prog.TypeAt(n))
	if !ok {
		return nil, &NotYetLowerable{Reason: "new Set did not expose its member type"}
	}
	var ctor string
	switch {
	case elem.Flags&frontend.TypeNumber != 0:
		ctor = "NewNumberSet"
	case elem.Flags&frontend.TypeString != 0:
		ctor = "NewStringSet"
	case elem.Flags&frontend.TypeBoolean != 0:
		ctor = "NewBoolSet"
	default:
		return nil, &NotYetLowerable{Reason: "a Set with a member that is not a number, string, or boolean is a later slice"}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", ctor)}, nil
}

// errorInstanceof lowers `e instanceof Error` on a caught error, the guard a
// catch narrows its binding with before reading the error. The checker types a
// catch binding unknown, so the only way a catch reads .message or .name is to
// narrow first, and instanceof is that narrowing (04_frontend_typescript_go.md,
// the narrowing table). The left operand must be a catch binding in scope and the
// right a built-in error constructor; the test lowers to the IsA name check, since
// the runtime models the error family as one type with a name rather than
// distinct Go types. It reports handled=false when the left is not a caught error,
// so a general instanceof still hands back, and an error against a non-error
// constructor hands back explicitly.
func (r *Renderer) errorInstanceof(left, right frontend.Node) (ast.Expr, bool, error) {
	if left.Kind() != frontend.NodeIdentifier {
		return nil, false, nil
	}
	name, ok := localName(r.prog.Text(left))
	if !ok || !r.errorLocals[name] {
		return nil, false, nil
	}
	ctor := r.prog.Text(right)
	// e instanceof GoError narrows a catch binding to the Go-error surface of section
	// 7.7, the guard an author writes before reading err.is. It lowers to the
	// *value.Error IsGoError test, which is true when the caught error carries a Go
	// error behind it: a caught go: failure narrows through, a program-thrown error
	// does not. GoError is the bento:go vocabulary class, not a built-in error, so it
	// routes before the errorCtors check below.
	if ctor == "GoError" {
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("IsGoError")}}, true, nil
	}
	if _, isErr := errorCtors[ctor]; !isErr {
		return nil, false, &NotYetLowerable{Reason: "instanceof a class other than a built-in error on a caught error is a later slice"}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ident(name), Sel: ident("IsA")},
		Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(ctor)}},
	}, true, nil
}

// errorMethodCall lowers a method call on a caught error, the error-identity
// surface of section 7.7. Only is() is covered here: err.is(sentinel) lowers to
// the *value.Error Is method against the Go error the caught error carries, so the
// comparison is errors.Is against the original error and a caught go: failure
// branches on identity the way Go code does. The argument must be a reference to a
// go: sentinel error variable (io.EOF and its kind); anything else hands back,
// since a comparison against a value that is not a Go error would be unsound. as()
// waits on the concrete-struct projection it unwraps into, and any other method on
// a caught error is its own later slice.
func (r *Renderer) errorMethodCall(name, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "is":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "err.is takes exactly one sentinel argument"}
		}
		sentinel, ok := r.goErrorSentinelRef(argNodes[0])
		if !ok {
			return nil, &NotYetLowerable{Reason: "err.is against a value that is not a go: sentinel error is a later slice"}
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("Is")}, Args: []ast.Expr{sentinel}}, nil
	case "as":
		return nil, &NotYetLowerable{Reason: "err.as waits on concrete Go error type projection"}
	default:
		return nil, &NotYetLowerable{Reason: "a caught error's ." + method + "() is a later slice"}
	}
}

// lowerThrow lowers a throw statement to a panic carrying the thrown error, the
// raise half of the exception model. Only throwing a built-in error is covered
// (throw new Error("..."), or throwing a variable bound to one), because the
// thrown value must be a value.Error for the panic to round-trip through the
// runtime's recover; throwing a bare string or number is a later slice that boxes
// the operand. Emitting the throw records that the program can raise, so the
// assembled main defers the uncaught-error reporter.
func (r *Renderer) lowerThrow(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "throw did not expose a single operand"}
	}
	if !r.isThrowable(kids[0]) {
		return nil, &NotYetLowerable{Reason: "throwing a value that is not a built-in error is a later slice"}
	}
	operand, err := r.lowerExpr(kids[0])
	if err != nil {
		return nil, err
	}
	r.usesThrow = true
	return &ast.ExprStmt{X: &ast.CallExpr{Fun: sel("value", "Throw"), Args: []ast.Expr{operand}}}, nil
}

// isThrowable reports whether a throw operand lowers to a value.Error the runtime
// can raise and recover. A new expression for a built-in error qualifies directly;
// an identifier qualifies when the checker types it as an Error, which is how a
// caught-and-rethrown or a locally-constructed error variable throws. Anything
// else hands back until arbitrary thrown values are boxed.
func (r *Renderer) isThrowable(n frontend.Node) bool {
	if n.Kind() == frontend.NodeNewExpression {
		kids := r.prog.Children(n)
		if len(kids) == 0 {
			return false
		}
		_, ok := errorCtors[r.prog.Text(kids[0])]
		return ok
	}
	return false
}

// lowerTry lowers a try/catch/finally to a Go closure over panic and recover. A
// throw inside the try body is a panic, so the try body runs inside an immediately
// invoked function whose deferred functions handle it: a catch is a deferred
// recover that binds the caught error and runs the catch body, and a finally is a
// deferred call that runs whether or not the body threw. The two are deferred in
// order so the catch runs first and the finally last, matching the language:
// finally runs after the catch, and after a normal completion too.
//
// The closure cannot carry an abrupt completion out of the construct, so a try,
// catch, or finally body that returns hands the whole statement back; a break or
// continue inside a body already hands back at the statement lowering, so only a
// return needs guarding here. Widening to abrupt completions that escape the
// construct is a later slice.
func (r *Renderer) lowerTry(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeBlock {
		return nil, &NotYetLowerable{Reason: "try did not expose a block body"}
	}
	tryBlock := kids[0]

	var catchClause, finallyBlock frontend.Node
	var hasCatch, hasFinally bool
	for _, k := range kids[1:] {
		if k.Kind() == frontend.NodeBlock {
			finallyBlock, hasFinally = k, true
		} else {
			catchClause, hasCatch = k, true
		}
	}
	if !hasCatch && !hasFinally {
		return nil, &NotYetLowerable{Reason: "a try with neither catch nor finally is a later slice"}
	}
	if r.blockReturns(tryBlock) || (hasFinally && r.blockReturns(finallyBlock)) {
		return nil, &NotYetLowerable{Reason: "a return that escapes a try, catch, or finally is a later slice"}
	}

	var closureBody []ast.Stmt

	// The finally defer is added first so it runs last, after the catch and after a
	// normal completion, the language's ordering.
	if hasFinally {
		finStmts, err := r.lowerBlock(finallyBlock)
		if err != nil {
			return nil, err
		}
		closureBody = append(closureBody, &ast.DeferStmt{Call: callClosure(finStmts.List)})
	}

	if hasCatch {
		catchDefer, err := r.catchDefer(catchClause)
		if err != nil {
			return nil, err
		}
		closureBody = append(closureBody, catchDefer)
	}

	tryStmts, err := r.lowerBlock(tryBlock)
	if err != nil {
		return nil, err
	}
	closureBody = append(closureBody, tryStmts.List...)

	return &ast.ExprStmt{X: callClosure(closureBody)}, nil
}

// catchDefer builds the deferred recover that runs a catch clause. It recovers the
// panic, and when something was thrown it converts the payload to the *value.Error
// the binding names (value.Caught re-panics a Go runtime bug so a genuine crash is
// not swallowed) and runs the catch body. A catch with no binding still recovers
// and converts, so a Go runtime panic is re-raised rather than silently caught, but
// discards the error.
func (r *Renderer) catchDefer(catchClause frontend.Node) (ast.Stmt, error) {
	var binding string
	var catchBlock frontend.Node
	for _, k := range r.prog.Children(catchClause) {
		switch k.Kind() {
		case frontend.NodeBlock:
			catchBlock = k
		case frontend.NodeVariableDeclaration:
			vk := r.prog.Children(k)
			if len(vk) != 1 || vk[0].Kind() != frontend.NodeIdentifier {
				return nil, &NotYetLowerable{Reason: "a destructured catch binding is a later slice"}
			}
			name, ok := localName(r.prog.Text(vk[0]))
			if !ok {
				return nil, &NotYetLowerable{Reason: "catch binding is not a Go identifier"}
			}
			binding = name
		default:
			return nil, &NotYetLowerable{Reason: "an unusual catch clause is a later slice"}
		}
	}
	if catchBlock == nil {
		return nil, &NotYetLowerable{Reason: "catch clause did not expose a block body"}
	}
	if r.blockReturns(catchBlock) {
		return nil, &NotYetLowerable{Reason: "a return that escapes a try, catch, or finally is a later slice"}
	}

	// The binding is in scope only while the catch block is lowered, so a read of its
	// .message or .name resolves to the error and a use elsewhere hands back.
	if binding != "" {
		prev := r.errorLocals[binding]
		r.errorLocals[binding] = true
		defer func() { r.errorLocals[binding] = prev }()
	}
	catchStmts, err := r.lowerBlock(catchBlock)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)

	// bind is `e := value.Caught(rec)` when the clause names the error, or a bare
	// `value.Caught(rec)` call when it does not, so a Go runtime panic re-raises
	// either way. A named binding is also assigned to blank so a catch that never
	// reads the error still compiles.
	caught := &ast.CallExpr{Fun: sel("value", "Caught"), Args: []ast.Expr{ident("rec")}}
	var body []ast.Stmt
	if binding == "" {
		body = append(body, &ast.ExprStmt{X: caught})
	} else {
		body = append(body,
			&ast.AssignStmt{Lhs: []ast.Expr{ident(binding)}, Tok: token.DEFINE, Rhs: []ast.Expr{caught}},
			&ast.AssignStmt{Lhs: []ast.Expr{ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ident(binding)}},
		)
	}
	body = append(body, catchStmts.List...)

	// if rec := recover(); rec != nil { <body> }
	guard := &ast.IfStmt{
		Init: &ast.AssignStmt{
			Lhs: []ast.Expr{ident("rec")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: ident("recover")}},
		},
		Cond: &ast.BinaryExpr{X: ident("rec"), Op: token.NEQ, Y: ident("nil")},
		Body: &ast.BlockStmt{List: body},
	}
	return &ast.DeferStmt{Call: callClosure([]ast.Stmt{guard})}, nil
}

// blockReturns reports whether a statement subtree contains a return that would
// complete out of it, descending through nested statements but not into a nested
// function, whose own return is its own. It is the guard the try lowering uses to
// keep an escaping return out of the closure it emits, where a Go return would
// leave the closure rather than the enclosing function.
func (r *Renderer) blockReturns(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeFunctionDeclaration, frontend.NodeFunctionExpression, frontend.NodeArrowFunction,
		frontend.NodeMethodDeclaration, frontend.NodeGetAccessor, frontend.NodeSetAccessor, frontend.NodeConstructor:
		return false
	case frontend.NodeReturnStatement:
		return true
	}
	for _, k := range r.prog.Children(n) {
		if r.blockReturns(k) {
			return true
		}
	}
	return false
}

// callClosure wraps statements in an immediately invoked function with no
// parameters or results, the form a try body and its deferred handlers take so a
// recover can run inside a deferred function.
func callClosure(stmts []ast.Stmt) *ast.CallExpr {
	return &ast.CallExpr{Fun: &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: stmts},
	}}
}
