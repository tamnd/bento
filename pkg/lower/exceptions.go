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

// errorCtorValueNames is the set of built-in error constructors that lower to a
// value when named without new, the argument-and-comparison surface assert.throws
// drives. It is broader than errorCtors because boxing a constructor and reading
// its name work off the name alone, with no construction path, so it lists every
// standard error constructor plus URIError, matching the value package's own set.
var errorCtorValueNames = map[string]bool{
	"Error":          true,
	"TypeError":      true,
	"RangeError":     true,
	"SyntaxError":    true,
	"ReferenceError": true,
	"EvalError":      true,
	"URIError":       true,
	"AggregateError": true,
}

// errorConstructorRef reports the built-in error constructor an identifier names as
// a value, and ok=false for anything else. It gates on the ambient-global test so a
// user binding or class that shadows one of the names (class TypeError of one's own)
// is never mistaken for the built-in and keeps its own lowering. This is what the
// dynamic-boxing path checks to lower TypeError, passed as an argument or compared
// for identity, to the interned constructor value.
func (r *Renderer) errorConstructorRef(n frontend.Node) (string, bool) {
	if n.Kind() != frontend.NodeIdentifier {
		return "", false
	}
	name := r.prog.Text(n)
	if !errorCtorValueNames[name] {
		return "", false
	}
	if !r.isAmbientGlobal(n) {
		return "", false
	}
	return name, true
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
	// Re-throwing a caught error is the rethrow half of the exception model:
	// catch (err) { ...; throw err } passes the recovered value straight back up.
	// The binding is already a *value.Error, which carries the runtime's Thrown
	// surface, so the throw re-raises that exact value with its identity intact.
	// Reading the binding any other way still hands back at expr.go, so this is
	// spelled out here rather than routed through lowerExpr, which would decline.
	if kids[0].Kind() == frontend.NodeIdentifier {
		if name, ok := localName(r.prog.Text(kids[0])); ok && r.errorLocals[name] {
			r.usesThrow = true
			return &ast.ExprStmt{X: &ast.CallExpr{Fun: sel("value", "Throw"), Args: []ast.Expr{ident(name)}}}, nil
		}
	}
	if !r.isThrowable(kids[0]) {
		// A new expression of a registered class whose instances carry a string
		// message throws too: the instance itself is the panic payload, and the
		// class gains the ErrorName and ErrorMessage methods (renderClasses) that
		// satisfy the runtime's Thrown surface, so a catch that recovers it binds
		// an error named after the class carrying the instance's message. A
		// thrown string wraps in the runtime's ThrownString, which carries the
		// same surface with the string as the name.
		if info, ok := r.thrownClassOf(kids[0]); ok {
			info.thrownAsError = true
		} else if r.isString(kids[0]) {
			operand, err := r.lowerExpr(kids[0])
			if err != nil {
				return nil, err
			}
			r.usesThrow = true
			wrapped := &ast.CallExpr{Fun: sel("value", "ThrownString"), Args: []ast.Expr{operand}}
			return &ast.ExprStmt{X: &ast.CallExpr{Fun: sel("value", "Throw"), Args: []ast.Expr{wrapped}}}, nil
		} else {
			return nil, &NotYetLowerable{Reason: "throwing a value that is not a built-in error is a later slice"}
		}
	}
	operand, err := r.lowerExpr(kids[0])
	if err != nil {
		return nil, err
	}
	r.usesThrow = true
	return &ast.ExprStmt{X: &ast.CallExpr{Fun: sel("value", "Throw"), Args: []ast.Expr{operand}}}, nil
}

// isThrowable reports whether a throw operand lowers to a value.Error the runtime
// can raise and recover. Only a new expression for a built-in error qualifies here;
// re-throwing a caught error is recognized earlier in lowerThrow, where the binding
// is known to be a *value.Error. A locally constructed error variable still hands
// back until its type flows through, and any other operand hands back until
// arbitrary thrown values are boxed.
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

// thrownClassOf reports the registered class a throw's new expression
// constructs, when its instances can ride the runtime's throw path: the class
// declares its own string message field, so the Thrown surface the panic
// payload needs (a name and a message) reads straight off the instance. A
// class without one hands back rather than throw a value a catch could not
// read.
func (r *Renderer) thrownClassOf(n frontend.Node) (*classInfo, bool) {
	if n.Kind() != frontend.NodeNewExpression {
		return nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return nil, false
	}
	info, ok := r.classNameRef(kids[0])
	if !ok {
		return nil, false
	}
	for _, f := range info.fields {
		if f.prop == "message" && r.prog.TypeAt(f.ident).Flags&frontend.TypeString != 0 {
			return info, true
		}
	}
	return nil, false
}

// tryRetMode is how a return statement leaves the try construct it sits in;
// the modes are documented on the Renderer's tryRet field. tryRetDeferPlain is
// the deferred-handler mode of the always-returning form, whose closure carries
// only the value: the assignment fills ret alone, with no done to raise.
type tryRetMode int

const (
	tryRetNone tryRetMode = iota
	tryRetBody
	tryRetDefer
	tryRetDeferPlain
)

// lowerTry lowers a try/catch/finally to a Go closure over panic and recover. A
// throw inside the try body is a panic, so the try body runs inside an immediately
// invoked function whose deferred functions handle it: a catch is a deferred
// recover that binds the caught error and runs the catch body, and a finally is a
// deferred call that runs whether or not the body threw. The two are deferred in
// order so the catch runs first and the finally last, matching the language:
// finally runs after the catch, and after a normal completion too.
//
// A try none of whose bodies return stays that plain closure. A body that
// returns compiles to the escape form instead: the closure takes named results
// (ret T, done bool), a return in the try body fills them with `return x,
// true`, a return in a catch or finally assigns them from inside the deferred
// function, and the call site turns done back into the enclosing function's
// return. A break or continue inside a body still hands back at the statement
// lowering.
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
	escapes := r.blockReturns(tryBlock) ||
		(hasCatch && r.blockReturns(catchClause)) ||
		(hasFinally && r.blockReturns(finallyBlock))
	if escapes {
		return r.lowerTryEscape(n, tryBlock, catchClause, finallyBlock, hasCatch, hasFinally)
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
		catchDefer, err := r.catchDefer(catchClause, false)
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

// lowerTryEscape lowers a try one of whose bodies returns. Two forms cover it,
// both closures whose named results a deferred catch or finally can fill.
//
// When every path through the try body and the catch ends in a return or a
// throw, the construct as a whole always returns, so it lowers to returning
// the closure directly, and the enclosing function needs nothing after it:
//
//	return func() (ret T) { ... }()
//
// A return in that try body stays a plain Go return; a return in the catch or
// finally assigns ret and leaves the handler with a bare return.
//
// Otherwise control can run past the construct, so the closure carries a
// second result saying whether a body returned, and the call site turns it
// back into a real return:
//
//	if ret, done := func() (ret T, done bool) { ... }(); done {
//		return ret
//	}
//
// There a return in the try body is `return x, true`, and a return in a catch
// or finally assigns both named results. A function returning nothing always
// takes the done form with ret dropped, since it has no missing-return
// obligation the always form would be needed for.
func (r *Renderer) lowerTryEscape(n, tryBlock, catchClause, finallyBlock frontend.Node, hasCatch, hasFinally bool) (ast.Stmt, error) {
	if r.tryRet == tryRetDefer || r.tryRet == tryRetDeferPlain {
		return nil, &NotYetLowerable{Reason: "a try with an escaping return nested in a catch or finally is a later slice"}
	}
	// The closure's named results live in the same scope as the bodies, so a
	// source binding or reference named ret or done inside the construct would
	// resolve to them rather than to the source's own variable; that shadowing
	// hazard hands back rather than miscompile.
	if r.mentionsName(n, "ret") || r.mentionsName(n, "done") {
		return nil, &NotYetLowerable{Reason: "a name colliding with the try escape results (ret, done) is a later slice"}
	}
	valued := !isVoidReturn(r.retType)
	always := valued && r.alwaysAbrupt(r.prog.Children(tryBlock)) &&
		(!hasCatch || r.alwaysAbrupt(r.prog.Children(r.catchBlockOf(catchClause))))

	bodyMode, deferMode := tryRetBody, tryRetDefer
	if always {
		// The always form's closure returns the plain value, so returns in the
		// try body need no rewriting at all, and the handlers fill only ret.
		bodyMode, deferMode = tryRetNone, tryRetDeferPlain
	}

	var closureBody []ast.Stmt
	if hasFinally {
		prev := r.tryRet
		r.tryRet = deferMode
		finStmts, err := r.lowerBlock(finallyBlock)
		r.tryRet = prev
		if err != nil {
			return nil, err
		}
		closureBody = append(closureBody, &ast.DeferStmt{Call: callClosure(finStmts.List)})
	}
	if hasCatch {
		prev := r.tryRet
		r.tryRet = deferMode
		catchDefer, err := r.catchDefer(catchClause, true)
		r.tryRet = prev
		if err != nil {
			return nil, err
		}
		closureBody = append(closureBody, catchDefer)
	}
	prev := r.tryRet
	r.tryRet = bodyMode
	tryStmts, err := r.lowerBlock(tryBlock)
	r.tryRet = prev
	if err != nil {
		return nil, err
	}
	closureBody = append(closureBody, tryStmts.List...)
	// The closure's tail return is the fall-off path; a body already ending in
	// a Go return needs none, and in the always form a throw ends the body
	// instead, whose panic the tail return sits after only syntactically.
	if len(closureBody) == 0 || !isGoReturn(closureBody[len(closureBody)-1]) {
		closureBody = append(closureBody, &ast.ReturnStmt{})
	}

	results := &ast.FieldList{}
	if valued {
		rt, err := r.typeExpr(r.retType)
		if err != nil {
			return nil, err
		}
		results.List = append(results.List, &ast.Field{Names: []*ast.Ident{ident("ret")}, Type: rt})
	}
	if !always {
		results.List = append(results.List, &ast.Field{Names: []*ast.Ident{ident("done")}, Type: ident("bool")})
	}
	closure := &ast.CallExpr{Fun: &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: results},
		Body: &ast.BlockStmt{List: closureBody},
	}}

	if always {
		// Inside an enclosing escape closure this return itself raises that
		// closure's done.
		if r.tryRet == tryRetBody {
			return &ast.ReturnStmt{Results: []ast.Expr{closure, ident("true")}}, nil
		}
		return &ast.ReturnStmt{Results: []ast.Expr{closure}}, nil
	}

	// The done form's call site: bind the results and return when a body did.
	// Inside an enclosing escape closure the emitted return fills that closure's
	// named results; the if-init's ret shadows the outer one only for the
	// return's operand, which is exactly the value being handed up.
	var lhs []ast.Expr
	var thenRet *ast.ReturnStmt
	if valued {
		lhs = []ast.Expr{ident("ret"), ident("done")}
		if r.tryRet == tryRetBody {
			thenRet = &ast.ReturnStmt{Results: []ast.Expr{ident("ret"), ident("true")}}
		} else {
			thenRet = &ast.ReturnStmt{Results: []ast.Expr{ident("ret")}}
		}
	} else {
		lhs = []ast.Expr{ident("done")}
		if r.tryRet == tryRetBody {
			thenRet = &ast.ReturnStmt{Results: []ast.Expr{ident("true")}}
		} else {
			thenRet = &ast.ReturnStmt{}
		}
	}
	return &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: lhs, Tok: token.DEFINE, Rhs: []ast.Expr{closure}},
		Cond: ident("done"),
		Body: &ast.BlockStmt{List: []ast.Stmt{thenRet}},
	}, nil
}

// catchBlockOf returns the block body of a catch clause, or the clause itself
// when it has none, which only makes the abruptness analysis answer false.
func (r *Renderer) catchBlockOf(catchClause frontend.Node) frontend.Node {
	for _, k := range r.prog.Children(catchClause) {
		if k.Kind() == frontend.NodeBlock {
			return k
		}
	}
	return catchClause
}

// alwaysAbrupt reports whether a statement list cannot complete normally: its
// last statement returns, throws, or is an if-else (or block) whose every arm
// does. It is deliberately conservative; a shape it cannot see answers false
// and only costs the try escape its done result.
func (r *Renderer) alwaysAbrupt(stmts []frontend.Node) bool {
	if len(stmts) == 0 {
		return false
	}
	return r.stmtAbrupt(stmts[len(stmts)-1])
}

// stmtAbrupt reports whether one statement cannot complete normally, the
// per-statement half of alwaysAbrupt.
func (r *Renderer) stmtAbrupt(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeReturnStatement, frontend.NodeThrowStatement:
		return true
	case frontend.NodeBlock:
		return r.alwaysAbrupt(r.prog.Children(n))
	case frontend.NodeIfStatement:
		kids := r.prog.Children(n)
		return len(kids) >= 3 && r.stmtAbrupt(kids[1]) && r.stmtAbrupt(kids[2])
	}
	return false
}

// isGoReturn reports whether a lowered statement is a Go return, the check the
// try escape uses to skip its redundant tail return.
func isGoReturn(s ast.Stmt) bool {
	_, ok := s.(*ast.ReturnStmt)
	return ok
}

// catchDefer builds the deferred recover that runs a catch clause. It recovers the
// panic, and when something was thrown it converts the payload to the *value.Error
// the binding names (value.Caught re-panics a Go runtime bug so a genuine crash is
// not swallowed) and runs the catch body. A catch with no binding still recovers
// and converts, so a Go runtime panic is re-raised rather than silently caught, but
// discards the error. allowReturns is set by the escape form, whose named results
// a catch return can fill; the plain form keeps its hand-back on a returning
// catch, since its closure has nowhere to carry the value.
func (r *Renderer) catchDefer(catchClause frontend.Node, allowReturns bool) (ast.Stmt, error) {
	var binding string
	var catchBlock frontend.Node
	for _, k := range r.prog.Children(catchClause) {
		switch k.Kind() {
		case frontend.NodeBlock:
			catchBlock = k
		case frontend.NodeVariableDeclaration:
			// The binding is the declaration's leading identifier; a type
			// annotation (catch (err: any), the strict-checker spelling) rides
			// along as a trailing child and changes nothing, since the binding
			// is the *value.Error the recover converts regardless of what the
			// author annotated. Only a binding pattern declines.
			vk := r.prog.Children(k)
			if len(vk) == 0 || vk[0].Kind() != frontend.NodeIdentifier {
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
	if !allowReturns && r.blockReturns(catchBlock) {
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
