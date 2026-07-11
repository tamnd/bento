package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file answers what the checker typed a node as (number, string, bigint,
// dynamic) and converts between the static and dynamic worlds: boxing a static
// value to a value.Value and coercing a dynamic value back to the static type a
// site expects.

// primitiveFlags returns the type flags of n with a branded alias folded down to
// its underlying primitive. A go: defined type over a basic (time.Duration over
// int64) projects as a branded alias (number & { __brand }), an intersection whose
// runtime value is the underlying primitive, so its primitive-member flag is folded
// in and a consumer coerces and operates on it as the number, string, or boolean it
// is (section 6.11). A plain type is returned unchanged.
func (r *Renderer) primitiveFlags(n frontend.Node) frontend.TypeFlags {
	// A call whose callee is a boxed value.Value binding returns a box, not a
	// primitive, even where control-flow analysis evolved the callee to a concrete
	// return type at the call site. Reporting no primitive facet routes every
	// predicate over the call result (isNumber, isString, isBool, isBigInt) to the
	// dynamic path the box needs, rather than a static coercion that would mistype it.
	if r.callOfDynamicStorage(n) {
		return frontend.TypeAny
	}
	return r.primitiveFlagsOfType(r.prog.TypeAt(n))
}

// primitiveFlagsOfType is primitiveFlags over a Type rather than a node, so a
// return type, a union member, or any other type handle folds down to its
// primitive facet the same way. typeExpr uses it to pick the Go type of a folded
// union, and primitiveFlags forwards a node's type to it.
func (r *Renderer) primitiveFlagsOfType(t frontend.Type) frontend.TypeFlags {
	f := t.Flags
	// A registered enum is backed by a primitive (float64 for a numeric enum,
	// value.BStr for a string enum), so a value of the enum type is that primitive
	// wherever the predicates ask: a member read already carries the primitive flag,
	// but the whole enum type an annotation names (a parameter, a return, a typed
	// binding) is an enum-flagged union with no primitive bit, so the flag is folded
	// in here the way a branded alias folds to its primitive.
	if f&frontend.TypeEnum != 0 {
		if info, ok := r.enumOfType(t); ok {
			if info.isString {
				f |= frontend.TypeString
			} else {
				f |= frontend.TypeNumber
			}
		}
	}
	const prim = frontend.TypeNumber | frontend.TypeString | frontend.TypeBoolean
	if f&frontend.TypeIntersection != 0 {
		for _, m := range r.prog.IntersectionMembers(t) {
			f |= m.Flags & prim
		}
	}
	// A union folds in a primitive facet only when every member carries it, so a
	// union of numeric literals (1 | 2 | 3) is a number and true | false (how the
	// checker often spells boolean) is a boolean, the same widening TypeScript
	// applies. A member outside that primitive, including null or undefined, clears
	// the facet, so a mixed union like string | number or an optional string |
	// undefined folds nothing and stays on its own path. String is deliberately not
	// in this mask: a closed string-literal union ("on" | "off") lowers to a compact
	// integer tag enum (union.go, section 10), not a bstr, so its value is a tag and
	// must not be treated as a string for coercion or type mapping.
	const unionPrim = frontend.TypeNumber | frontend.TypeBoolean
	if f&frontend.TypeUnion != 0 {
		if members := r.prog.UnionMembers(t); len(members) > 0 {
			common := frontend.TypeFlags(unionPrim)
			for _, m := range members {
				common &= m.Flags
			}
			f |= common
		}
	}
	return f
}

// isNumber reports whether the checker types n as number, the guard that keeps
// the arithmetic path sound while string and mixed operands wait for their slice.
// It sees through a branded alias to the underlying number (section 6.11).
func (r *Renderer) isNumber(n frontend.Node) bool {
	return r.primitiveFlags(n)&frontend.TypeNumber != 0
}

// isBool reports whether the checker types n as boolean, the guard that keeps a
// control-flow condition a real Go bool rather than a coerced value. A read of an
// IteratorResult's .done counts too: the checker types it undefined | false | true
// (the optional done? on the yield branch widens in undefined), so it carries no
// folded boolean facet, but member.go lowers it to the IterResult.Done field, a real
// Go bool, so every consumer of the predicate sees the bool the lowered read is.
func (r *Renderer) isBool(n frontend.Node) bool {
	return r.primitiveFlags(n)&frontend.TypeBoolean != 0 || r.iterResultDoneRead(n)
}

// iterResultDoneRead reports whether n reads .done off an IteratorResult, the read
// member.go lowers to the value.IterResult Done field, a Go bool. The checker types
// .done as undefined | false | true rather than boolean, so it needs this hook to
// take the boolean path in truthiness and String() coercion the way a caught error's
// string read needs caughtErrorStringRead.
func (r *Renderer) iterResultDoneRead(n frontend.Node) bool {
	if n.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 || r.prog.Text(kids[1]) != "done" {
		return false
	}
	return r.isIterResultReceiver(kids[0])
}

// isString reports whether the checker types n as string, the guard that routes
// + to value.Concat and .length to value.BStr.Length rather than to a number or
// object path. A read of a caught error's .message or .name counts too: the
// checker types the binding any or unknown, so the read carries no string flag,
// but it lowers to the *value.Error Message or Name method (member.go), which
// returns the bento string, so every consumer of the predicate sees the string
// the lowered expression is.
func (r *Renderer) isString(n frontend.Node) bool {
	return r.primitiveFlags(n)&frontend.TypeString != 0 || r.caughtErrorStringRead(n)
}

// isBoxedValue reports whether n lowers to a boxed value.Value at this use, the
// receiver shape the dynamic method path needs. A node the checker types any or
// unknown is boxed, and so is a dynamic local the checker narrowed to a kind the
// accessors do not unbox: a typeof guard narrows an any binding to symbol inside
// the guarded block, but the binding still holds the bare box, so a method call
// on it (message.toString() in assert.compareArray) reads through the box rather
// than a static receiver that does not exist. A narrow to string, number, or
// boolean unboxes through its accessor, so those take the static method path.
func (r *Renderer) isBoxedValue(n frontend.Node) bool {
	if r.isDynamic(n) {
		return true
	}
	if n.Kind() != frontend.NodeIdentifier {
		return false
	}
	name, ok := localName(r.prog.Text(n))
	if !ok || !r.dynLocals[name] {
		return false
	}
	_, unboxes := dynAccessor(r.primitiveFlags(n))
	return !unboxes
}

// isCaughtErrorRef reports whether n is a bare reference to a catch binding in
// scope, the *value.Error a catch bound. It is the guard the caught-error paths
// use to route typeof and a null compare over the binding, which the checker
// types any or unknown but which the runtime always holds as an error object.
func (r *Renderer) isCaughtErrorRef(n frontend.Node) bool {
	if n.Kind() != frontend.NodeIdentifier {
		return false
	}
	name, ok := localName(r.prog.Text(n))
	return ok && r.errorLocals[name]
}

// caughtErrorNullCompare folds an equality between a caught error and the null or
// undefined literal to a Go boolean constant. A caught value is a non-nil
// *value.Error, so it is never null or undefined, which makes === and == false and
// !== and != true regardless of which side holds the literal. It reports
// handled=false when neither shape matches, so a compare that is not this one falls
// through to the normal path.
func (r *Renderer) caughtErrorNullCompare(opText string, left, right frontend.Node) (ast.Expr, bool) {
	switch opText {
	case "===", "!==", "==", "!=":
	default:
		return nil, false
	}
	isNullish := func(n frontend.Node) bool {
		return n.Kind() == frontend.NodeNullKeyword || r.isUndefinedLiteral(n)
	}
	caughtVsNullish := (r.isCaughtErrorRef(left) && isNullish(right)) ||
		(r.isCaughtErrorRef(right) && isNullish(left))
	if !caughtVsNullish {
		return nil, false
	}
	result := "false"
	if opText == "!==" || opText == "!=" {
		result = "true"
	}
	return ident(result), true
}

// caughtErrorStringRead reports whether n reads .message or .name off a catch
// binding in scope, the two reads member.go lowers to the *value.Error methods.
func (r *Renderer) caughtErrorStringRead(n frontend.Node) bool {
	if n.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
		return false
	}
	name, ok := localName(r.prog.Text(kids[0]))
	if !ok || !r.errorLocals[name] {
		return false
	}
	prop := r.prog.Text(kids[1])
	return prop == "message" || prop == "name"
}

// isSymbol reports whether the checker types n as a symbol, the guard that routes a
// computed key `o[s]` through the runtime GetElem and SetElem, where the boxed
// symbol looks up the property bag by identity. Only an explicitly annotated symbol
// carries the flag: a `const s = Symbol()` binding is typed unique symbol, which
// the frontend leaves flagless, so that form is recognized instead by the dynamic
// mark its initializer set.
func (r *Renderer) isSymbol(n frontend.Node) bool {
	return r.prog.TypeAt(n).Flags&frontend.TypeSymbol != 0
}

// isBigInt reports whether the checker types n as bigint, the guard that routes the
// operators and coercions to the *big.Int method forms rather than the float64
// operator forms. It sees through a branded alias the same way isNumber does, so a
// go: defined type over bigint still lands on the bigint path.
func (r *Renderer) isBigInt(n frontend.Node) bool {
	return r.primitiveFlags(n)&frontend.TypeBigInt != 0
}

// isDynamic reports whether the checker types n as any or unknown, the types that
// have no static Go shape and so live as a boxed value.Value. It is the guard the
// dynamic paths use to route a property read, a +, or an assignment through the
// value box rather than a static field, operator, or slot.
func (r *Renderer) isDynamic(n frontend.Node) bool {
	// A read of a caught error's .message or .name lowers to a bento string
	// (member.go), so it is not a boxed value even though the checker types the
	// catch binding any or unknown; keeping it off the dynamic path routes a +
	// over it to the plain string concat the lowered expression supports.
	if r.caughtErrorStringRead(n) {
		return false
	}
	// A read of a property a fixed shape does not declare lowers to the boxed
	// undefined singleton (member.go), so it is a dynamic value even though the
	// checker gives it the error type rather than any. Routing it here keeps the
	// enclosing call, +, or coercion treating the read as the box it is.
	if r.missingPropertyRead(n) {
		return true
	}
	// A call whose callee is a binding stored as a boxed value.Value returns a box:
	// the runtime Call always yields a value.Value. Control-flow analysis may have
	// evolved an implicit-any callee to a concrete return type at the call site,
	// which would drive a static coercion over the box and mistype it, so the call
	// reads as dynamic off the callee's storage rather than the narrowed type.
	if r.callOfDynamicStorage(n) {
		return true
	}
	// Object.fromEntries builds a runtime object and Object.entries a runtime array
	// of pair arrays, each a boxed value.Value, even though the checker types the
	// results as an index-signature object and a tuple array. A binding holds the box
	// and a member or element read off it must dispatch through the dynamic Get, so
	// the call reads as dynamic off its callee rather than its non-any result type.
	if r.objectBoxedResultCall(n) {
		return true
	}
	// A borrowed Array.prototype.<m>.call/apply on a generic receiver runs the
	// generic-receiver runtime, whose result is a boxed value.Value whatever the
	// method returns. The checker types the borrowed call off the method's static
	// signature, not any, so isDynamic recognizes it by shape here to keep the box on
	// the dynamic path where a member or element read dispatches through Get.
	if r.arrayProtoBorrowedResultCall(n) {
		return true
	}
	// Array.from over a dynamic source, or with a map callback, lowers to
	// value.ArrayFromArrayLike, whose result is a boxed value.Value. The checker
	// types Array.from as a concrete array, so isDynamic recognizes the boxed form
	// by shape to keep a member or element read off the result on the dynamic path.
	if r.arrayFromBoxedResultCall(n) {
		return true
	}
	// A .value read off an IteratorResult whose type is not a clean primitive, the
	// array iterator's `number | undefined` value being the first, stays the boxed
	// value.Value the IterResult carries: there is no single Go type to coerce it to,
	// so a member or element read off it and a flow into an any slot take the dynamic
	// path. A generator whose value is a clean number keeps the static coercion.
	if r.iterResultBoxedValueRead(n) {
		return true
	}
	// An object rest binding an untyped pattern gathered holds the plain object
	// ObjectRest built, a boxed value.Value, even though the checker gave it the fixed
	// shape of the properties the pattern did not name. A property read off it must
	// dispatch through the dynamic Get, so the read routes here off the binding's
	// storage rather than its non-any type, which would fold to a fixed-shape miss.
	if n.Kind() == frontend.NodeIdentifier {
		if name, ok := localName(r.prog.Text(n)); ok && r.dynBoundLocals[name] {
			return true
		}
	}
	return r.prog.TypeAt(n).Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0
}

// objectBoxedResultCall reports whether n is a call to Object.fromEntries or
// Object.entries, whose runtime result is a boxed value.Value: fromEntries a runtime
// object and entries a runtime array of pair arrays. The checker types the results
// as an index-signature object and a tuple array, not any, so isDynamic recognizes
// the calls by shape here to keep the box on the dynamic path, where a member or
// element read dispatches through Get and a flow into an any slot is the identity.
func (r *Renderer) objectBoxedResultCall(n frontend.Node) bool {
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
	if !r.isGlobalRef(parts[0], "Object") {
		return false
	}
	method := r.prog.Text(parts[1])
	return method == "fromEntries" || method == "entries"
}

// arrayProtoBorrowedResultCall reports whether n is a call whose callee is
// Array.prototype.<m>.call or Array.prototype.<m>.apply, the borrowed form the
// generic-receiver runtime lowers to a boxed value.Value. isDynamic recognizes it
// by shape so the box stays on the dynamic path whatever static type the checker
// gave the borrowed method's result.
func (r *Renderer) arrayProtoBorrowedResultCall(n frontend.Node) bool {
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
	switch r.prog.Text(parts[1]) {
	case "call", "apply":
	default:
		return false
	}
	_, ok := r.arrayProtoMethodName(parts[0])
	return ok
}

// arrayFromBoxedResultCall reports whether n is an Array.from call the lowerer
// routes to value.ArrayFromArrayLike, whose result is a boxed value.Value: the
// form over a dynamic source, with or without a map callback, as opposed to the
// copy of a typed array, string, or user iterable. A dynamic source means the
// surrounding context is dynamic too, so the boxed array flows without a
// representation mismatch; a map callback over a typed source, whose result the
// checker types a concrete array, is a later slice and does not take this path.
// isDynamic recognizes the boxed form by shape so a read off the result stays on
// the dynamic path whatever array type the checker gave Array.from. The routing
// in arrayFrom shares this same rule.
func (r *Renderer) arrayFromBoxedResultCall(n frontend.Node) bool {
	if n.Kind() != frontend.NodeCallExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	parts := r.prog.Children(kids[0])
	if len(parts) != 2 || !r.isGlobalRef(parts[0], "Array") || r.prog.Text(parts[1]) != "from" {
		return false
	}
	args := kids[1:]
	if len(args) < 1 || len(args) > 2 {
		return false
	}
	return r.isDynamic(args[0])
}

// callOfDynamicStorage reports whether n is a call whose callee is a bare
// identifier bound to a value.Value slot: a var declared with no initializer, or an
// implicit-any binding later assigned a function. The runtime Call on such a slot
// yields a boxed value, so the call result is dynamic even when control-flow
// analysis narrowed the callee to a concrete function type at the call site. A
// top-level function symbol is excluded: it lowers to a static Go func, not a box.
func (r *Renderer) callOfDynamicStorage(n frontend.Node) bool {
	if n.Kind() != frontend.NodeCallExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return false
	}
	if sym, ok := r.prog.SymbolAt(kids[0]); !ok || sym.Flags&frontend.SymbolFunction != 0 {
		return false
	}
	return r.localStorageDynamic(kids[0])
}

// localStorageDynamic reports whether a local identifier's Go slot is a boxed
// value.Value, which is how a binding whose declared type is any or unknown is
// stored (stmt.go declares `var y value.Value` for `var y;` with no initializer).
// It reads the declared type off the symbol, not the identifier node, because
// control-flow analysis narrows a later read to a primitive while the slot itself
// stays a box. A compound assignment reads its narrowed target and so needs the
// slot's real storage to decide whether the static result must be boxed back.
func (r *Renderer) localStorageDynamic(target frontend.Node) bool {
	if target.Kind() == frontend.NodeIdentifier {
		if name, ok := localName(r.prog.Text(target)); ok && r.dynBoundLocals[name] {
			return true
		}
	}
	sym, ok := r.prog.SymbolAt(target)
	if !ok {
		return false
	}
	return r.prog.TypeOfSymbol(sym).Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0
}

// isUndefinedLiteral reports whether n is the ambient undefined global, the one
// identifier whose type is exactly undefined. It tells the literal apart from a
// user binding that could be typed undefined only loosely, but the pair with the
// null keyword is what the dynamic-boxing path needs: a value whose whole meaning
// is the absent singleton, so boxing it is the identity.
func (r *Renderer) isUndefinedLiteral(n frontend.Node) bool {
	return n.Kind() == frontend.NodeIdentifier &&
		r.prog.Text(n) == "undefined" &&
		r.prog.TypeAt(n).Flags == frontend.TypeUndefined
}

// combineIsDynamic reports whether a binary operator on these operands produces a
// boxed dynamic result, which is the case only for + with a dynamic operand: the
// result kind is not known until runtime, so it goes through value.Add. When the
// other operand is a known string the result kind is known after all, since + with
// a string operand always concatenates, so the checker types it string and the
// concat path produces the bstr that type promises; the dynamic operand runs
// through ToString there, the same coercion Add would apply. Every other operator
// on a dynamic operand is not lowered here and hands back through the operator
// table, so this stays narrow to the one case combineBinary boxes.
func (r *Renderer) combineIsDynamic(opText string, left, right frontend.Node) bool {
	if opText != "+" {
		return false
	}
	if r.isString(left) || r.isString(right) {
		return false
	}
	return r.isDynamic(left) || r.isDynamic(right)
}

// boxOperand lowers an operand to a value.Value so a dynamic operator can take it.
// A dynamic operand already lowers to a value.Value and passes through; a static
// primitive is lifted through its box constructor. A non-primitive static operand
// has no box constructor on this path yet and hands back.
func (r *Renderer) boxOperand(n frontend.Node) (ast.Expr, error) {
	// An array or object literal boxes member by member straight from its node, since its
	// own type can be a shapeless tuple the typed literal path cannot spell; routing it
	// here before lowerExpr keeps a nested literal, whose element type does not lower on the
	// typed path, from handing the whole box back on a lowering it never needs.
	if boxed, ok, err := r.boxLiteralToDynamic(n); err != nil {
		return nil, err
	} else if ok {
		return boxed, nil
	}
	e, err := r.lowerExpr(n)
	if err != nil {
		return nil, err
	}
	if r.isDynamic(n) {
		return e, nil
	}
	return r.boxStaticToDynamic(e, n)
}

// boxStaticToDynamic wraps a statically typed primitive expression in the value
// constructor that boxes it, so a number, string, or boolean can flow into a
// dynamic slot or a dynamic operator. The source node carries the primitive kind,
// which picks the constructor. A non-primitive source has no constructor here yet
// and hands back.
func (r *Renderer) boxStaticToDynamic(expr ast.Expr, src frontend.Node) (ast.Expr, error) {
	// An object or array literal flowing into a dynamic slot builds a live
	// value.Object straight from its members rather than the static struct or
	// slice the fixed-shape path would build, since the any binding stores a box,
	// not a Go shape. This routes before the primitive cases so { x: 1 } and
	// [1, 2] take the object path even though their own type is a fixed shape.
	if boxed, ok, err := r.boxLiteralToDynamic(src); err != nil {
		return nil, err
	} else if ok {
		return boxed, nil
	}
	// A source that already lowers to a boxed value.Value, new Object() being the
	// first, needs no wrapping: its lowered expr is the box, so it enters a dynamic
	// slot as itself. This routes before the primitive switch, whose type tests a
	// non-primitive box would otherwise fall past to the handback.
	if r.producesBoxedValue(src) {
		return expr, nil
	}
	// A function value flowing into a dynamic slot boxes into a callable value.Value,
	// so a dynamic call site can invoke it without knowing its static signature. It
	// routes before the primitive switch, whose kind tests a function type would
	// otherwise fall past to the handback.
	if calls, _ := r.prog.Signatures(r.prog.TypeAt(src)); len(calls) == 1 {
		return r.boxFuncToDynamic(expr, calls[0])
	}
	r.requireImport(valuePkg)
	switch {
	case r.isNumber(src):
		return &ast.CallExpr{Fun: sel("value", "Number"), Args: []ast.Expr{expr}}, nil
	case r.isString(src):
		return &ast.CallExpr{Fun: sel("value", "StringValue"), Args: []ast.Expr{expr}}, nil
	case r.isBool(src):
		return &ast.CallExpr{Fun: sel("value", "Bool"), Args: []ast.Expr{expr}}, nil
	case r.isSymbol(src):
		// A symbol expression already lowers to a value.Value: Symbol(x) builds one,
		// a symbol binding stores it, and a symbol read off the bag hands one back. So
		// boxing a symbol into a dynamic slot is the identity, the way null and
		// undefined are boxes already.
		return expr, nil
	case src.Kind() == frontend.NodeNullKeyword, r.isUndefinedLiteral(src):
		// The null and undefined literals already lower to the value.Null and
		// value.Undefined singletons, which are boxes, so boxing them into a dynamic
		// slot is the identity. Gating on the literal node keeps a typed null or
		// undefined inside a union, whose representation is not a bare box, out.
		return expr, nil
	}
	// A built-in error constructor named as a value (TypeError passed as an argument,
	// RangeError compared for identity) boxes to the interned constructor value, which
	// carries the name and compares equal to itself. The lowered expr from the general
	// path above is dropped: the identifier has no value form of its own, so the
	// constructor value stands in for it. This routes after the primitive cases so a
	// binding named like a constructor but typed a primitive still takes its box.
	if name, ok := r.errorConstructorRef(src); ok {
		return &ast.CallExpr{
			Fun:  sel("value", "ErrorConstructor"),
			Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(name)}},
		}, nil
	}
	return nil, &NotYetLowerable{Reason: "boxing this static type into a dynamic value is a later slice"}
}

// boxFuncToDynamic wraps a lowered function value in a callable value.Value, the
// box a static function takes when it flows into a dynamic slot so a dynamic call
// site can invoke it through value.Call. The wrapper is a value.NewFunc closure
// that takes its arguments already boxed: it coerces each into the static
// parameter type the lowered func expects, calls the func, and boxes the result
// back into a value.Value. Coercion and boxing reuse the same dynamic-boundary
// rules an argument and a return crossing that boundary already take, so the
// boxed call behaves as the direct call would. A shape the wrapper cannot bridge
// (a rest parameter, an optional parameter, a parameter or result type with no
// coercion yet) hands back rather than emit a wrapper that would not compile.
func (r *Renderer) boxFuncToDynamic(expr ast.Expr, sig frontend.Signature) (ast.Expr, error) {
	if sig.RestParam != nil {
		return nil, &NotYetLowerable{Reason: "boxing a function with a rest parameter into a dynamic value is a later slice"}
	}
	if sig.MinArgs != len(sig.Params) {
		return nil, &NotYetLowerable{Reason: "boxing a function with an optional parameter into a dynamic value is a later slice"}
	}
	r.requireImport(valuePkg)
	const argsName = "__a"
	callArgs := make([]ast.Expr, 0, len(sig.Params))
	for i, p := range sig.Params {
		at := &ast.CallExpr{Fun: sel("value", "Arg"), Args: []ast.Expr{ident(argsName), &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}}}
		if p.Type.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
			// A dynamic parameter takes the boxed argument as-is; the body already reads
			// a value.Value there, so no coercion is needed.
			callArgs = append(callArgs, at)
			continue
		}
		coerced, err := r.coerceDynamicToStaticFlags(at, p.Type.Flags)
		if err != nil {
			return nil, err
		}
		callArgs = append(callArgs, coerced)
	}
	// The lowered func is called inline; a bare func literal needs parentheses to sit
	// in call position, and wrapping a plain identifier callee too is harmless.
	inner := &ast.CallExpr{Fun: &ast.ParenExpr{X: expr}, Args: callArgs}
	var body []ast.Stmt
	if sig.Return.Flags&(frontend.TypeVoid|frontend.TypeUndefined|frontend.TypeNever) != 0 {
		// A void or undefined return runs the call for its effect and yields the
		// undefined the language binds to the result of such a call. A never return,
		// a function whose body always throws, joins them: the call never completes
		// normally, so the trailing undefined is unreachable at run time yet keeps the
		// wrapper well-typed, the shape a throwing assert.throws callback takes.
		body = []ast.Stmt{
			&ast.ExprStmt{X: inner},
			&ast.ReturnStmt{Results: []ast.Expr{sel("value", "Undefined")}},
		}
	} else {
		boxed, err := r.boxStaticToDynamicFlags(inner, sig.Return.Flags)
		if err != nil {
			return nil, err
		}
		body = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{boxed}}}
	}
	thunk := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident(argsName)}, Type: &ast.ArrayType{Elt: sel("value", "Value")}}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "Value")}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
	return &ast.CallExpr{Fun: sel("value", "NewFunc"), Args: []ast.Expr{thunk}}, nil
}

// boxStaticToDynamicFlags boxes a static primitive result into a value.Value by its
// type flags, the type-driven companion to boxStaticToDynamic used where only a
// type is in hand and not a source node, as the function-boxing wrapper is when it
// boxes a call result. A dynamic result is already a box and passes through; a
// primitive rides its box constructor; any other result hands back.
func (r *Renderer) boxStaticToDynamicFlags(expr ast.Expr, flags frontend.TypeFlags) (ast.Expr, error) {
	r.requireImport(valuePkg)
	switch {
	case flags&(frontend.TypeAny|frontend.TypeUnknown) != 0:
		return expr, nil
	case flags&frontend.TypeNumber != 0:
		return &ast.CallExpr{Fun: sel("value", "Number"), Args: []ast.Expr{expr}}, nil
	case flags&frontend.TypeString != 0:
		return &ast.CallExpr{Fun: sel("value", "StringValue"), Args: []ast.Expr{expr}}, nil
	case flags&frontend.TypeBoolean != 0:
		return &ast.CallExpr{Fun: sel("value", "Bool"), Args: []ast.Expr{expr}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "boxing this static result type into a dynamic value is a later slice"}
	}
}

// producesBoxedValue reports whether a source expression already lowers to a
// value.Value box, so boxing it into a dynamic slot is the identity. new Object()
// is the first such form: it lowers to value.NewObject(), a live property bag that
// is already a box. This lets the coercion pass the lowered expr through rather
// than searching for a primitive constructor it has none of.
func (r *Renderer) producesBoxedValue(src frontend.Node) bool {
	if src.Kind() == frontend.NodeNewExpression {
		kids := r.prog.Children(src)
		return len(kids) >= 1 && r.prog.Text(kids[0]) == "Object" && len(kids) == 1
	}
	// Object.getOwnPropertyDescriptor(o, key) and Object.getOwnPropertyDescriptors(o)
	// lower to runtime calls that return a value.Value, the descriptor object or
	// undefined and the map of descriptors, so a slot that takes the result as a
	// dynamic value needs no further boxing. The checker types them
	// PropertyDescriptor | undefined and a Record of descriptors, neither of which
	// the primitive box path has a constructor for, so recognizing the calls here is
	// what lets const d: any = Object.getOwnPropertyDescriptor(o, k) store the box
	// straight through.
	return r.isDynamicDescriptorRead(src)
}

// isDynamicDescriptorRead reports whether src is an Object.getOwnPropertyDescriptor
// or Object.getOwnPropertyDescriptors call on a dynamic receiver, the descriptor
// reads that lower to a boxed value.Value rather than a static shape. The receiver
// must be dynamic for the call to route to the runtime read; a fixed-shape receiver
// takes a later slice and does not produce a box, so it is not claimed here.
func (r *Renderer) isDynamicDescriptorRead(src frontend.Node) bool {
	if src.Kind() != frontend.NodeCallExpression {
		return false
	}
	kids := r.prog.Children(src)
	if len(kids) < 2 {
		return false
	}
	callee := kids[0]
	if callee.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	ck := r.prog.Children(callee)
	if len(ck) != 2 {
		return false
	}
	if !r.isGlobalRef(ck[0], "Object") || !r.isDynamic(kids[1]) {
		return false
	}
	method := r.prog.Text(ck[1])
	return method == "getOwnPropertyDescriptor" || method == "getOwnPropertyDescriptors"
}

// boxLiteralToDynamic builds the boxed value form of an object or array literal
// whose slot is dynamic, so { x: 1 } and [1, 2] enter an any binding as a live
// value.Object rather than the static struct or slice the fixed-shape path would
// build. The second result reports whether src is a literal this path claims, so a
// non-literal returns (nil, false, nil) and the caller falls through to the
// primitive boxes. A member the plain data build cannot express (a spread, a
// method, a computed key) hands back to a later slice.
func (r *Renderer) boxLiteralToDynamic(src frontend.Node) (ast.Expr, bool, error) {
	switch src.Kind() {
	case frontend.NodeArrayLiteralExpression:
		e, err := r.boxArrayLiteral(src)
		return e, true, err
	case frontend.NodeObjectLiteralExpression:
		e, err := r.boxObjectLiteral(src)
		return e, true, err
	}
	return nil, false, nil
}

// boxArrayLiteral lowers [e0, e1, ...] into value.NewArrayValue over a []value.Value
// of the boxed elements, the dense array a boxed value carries. Each element boxes
// through boxOperand, so a primitive rides its box constructor and a nested literal
// recurses here. A spread element hands back, keeping this to the plain element run.
func (r *Renderer) boxArrayLiteral(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	elems := make([]ast.Expr, 0, len(kids))
	for _, k := range kids {
		if k.Kind() == frontend.NodeSpreadElement {
			return nil, &NotYetLowerable{Reason: "boxing an array literal with a spread element is a later slice"}
		}
		boxed, err := r.boxOperand(k)
		if err != nil {
			return nil, err
		}
		elems = append(elems, boxed)
	}
	r.requireImport(valuePkg)
	lit := &ast.CompositeLit{Type: &ast.ArrayType{Elt: sel("value", "Value")}, Elts: elems}
	return &ast.CallExpr{Fun: sel("value", "NewArrayValue"), Args: []ast.Expr{lit}}, nil
}

// boxObjectLiteral lowers { k: v, ... } into value.NewObject().Set(...) per member,
// the ordered property map a boxed object keeps, so the keys enumerate in source
// order the way JavaScript's own property order does. Each value boxes through
// boxOperand; a plain or shorthand key is the property name string set through Set,
// and a computed key `[expr]: v` boxes the key expression and writes it through
// SetElem, which resolves it to a string or symbol property at runtime the way the
// dynamic bracket write does, so `{ [k]: 1 }` and `{ [s]: 1 }` land the same slot a
// later `o[k]` or `o[s]` reads. A member that is not a plain, shorthand, or computed
// key-value (a method or a spread) hands back to a later slice.
func (r *Renderer) boxObjectLiteral(n frontend.Node) (ast.Expr, error) {
	r.requireImport(valuePkg)
	var obj ast.Expr = &ast.CallExpr{Fun: sel("value", "NewObject")}
	for _, p := range r.prog.Children(n) {
		if p.Kind() != frontend.NodeUnknown {
			return nil, &NotYetLowerable{Reason: "boxing an object literal with a method or accessor member is a later slice"}
		}
		if inner, ok := r.computedKey(p); ok {
			kids := r.prog.Children(p)
			if len(kids) != 2 {
				return nil, &NotYetLowerable{Reason: "boxing an object literal computed member with an unexpected shape is a later slice"}
			}
			boxedKey, err := r.boxOperand(inner)
			if err != nil {
				return nil, err
			}
			boxedVal, err := r.boxOperand(kids[1])
			if err != nil {
				return nil, err
			}
			obj = &ast.CallExpr{Fun: &ast.SelectorExpr{X: obj, Sel: ident("SetKeyed")}, Args: []ast.Expr{boxedKey, boxedVal}}
			continue
		}
		kids := r.prog.Children(p)
		var keyNode, valNode frontend.Node
		colon := false
		switch len(kids) {
		case 1:
			if strings.HasPrefix(strings.TrimSpace(r.prog.Text(p)), "...") {
				return nil, &NotYetLowerable{Reason: "boxing an object literal with a spread member is a later slice"}
			}
			keyNode, valNode = kids[0], kids[0]
		case 2:
			keyNode, valNode, colon = kids[0], kids[1], true
		default:
			return nil, &NotYetLowerable{Reason: "boxing an object literal member with an unexpected shape is a later slice"}
		}
		if keyNode.Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: "boxing an object literal with a non-identifier key is a later slice"}
		}
		boxedVal, err := r.boxOperand(valNode)
		if err != nil {
			return nil, err
		}
		// The __proto__: v member is a directive on the object's prototype, not an own
		// property of that name, so it writes the slot rather than a slot Set would land.
		// Only the colon form is the prototype directive; the { __proto__ } shorthand is
		// an ordinary own property.
		if colon && r.prog.Text(keyNode) == "__proto__" {
			obj = &ast.CallExpr{Fun: &ast.SelectorExpr{X: obj, Sel: ident("SetProtoAssign")}, Args: []ast.Expr{boxedVal}}
			continue
		}
		obj = &ast.CallExpr{
			Fun: &ast.SelectorExpr{X: obj, Sel: ident("Set")},
			Args: []ast.Expr{
				&ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{
					&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(r.prog.Text(keyNode))},
				}},
				boxedVal,
			},
		}
	}
	return obj, nil
}

// coerceDynamicToStatic wraps a boxed dynamic value in the coercion that lands it
// in a static primitive slot, the ToNumber, ToString, or ToBoolean the language
// runs when a value typed any flows into a number, string, or boolean binding. A
// target that is not one of those three primitives has no coercion here and hands
// back.
func (r *Renderer) coerceDynamicToStatic(expr ast.Expr, target frontend.Node) (ast.Expr, error) {
	return r.coerceDynamicToStaticFlags(expr, r.prog.TypeAt(target).Flags)
}

// coerceDynamicToStaticFlags is the flag-keyed core of coerceDynamicToStatic, so a
// caller that holds a target type rather than a node (a return statement, whose
// target is the function's declared return type) can pick the same coercion. It
// maps a number, string, or boolean target to the matching ToNumber, ToString, or
// ToBoolean; any other target hands back.
func (r *Renderer) coerceDynamicToStaticFlags(expr ast.Expr, flags frontend.TypeFlags) (ast.Expr, error) {
	r.requireImport(valuePkg)
	switch {
	case flags&frontend.TypeNumber != 0:
		return &ast.CallExpr{Fun: sel("value", "ToNumber"), Args: []ast.Expr{expr}}, nil
	case flags&frontend.TypeString != 0:
		return &ast.CallExpr{Fun: sel("value", "ToString"), Args: []ast.Expr{expr}}, nil
	case flags&frontend.TypeBoolean != 0:
		return &ast.CallExpr{Fun: sel("value", "ToBoolean"), Args: []ast.Expr{expr}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "coercing a dynamic value into this static type is a later slice"}
	}
}

// coerceReturn bridges a return value from its expression's type to the function's
// declared return type across the dynamic boundary, the same coercion an
// assignment applies to its target. A dynamic value returned as a static primitive
// runs the ToNumber family, a static value returned as any boxes, and a return
// whose value already matches the declared type passes through unchanged.
func (r *Renderer) coerceReturn(expr ast.Expr, srcNode frontend.Node) (ast.Expr, error) {
	if boxed, ok, err := r.boxToOptional(expr, srcNode, r.retType); err != nil {
		return nil, err
	} else if ok {
		return boxed, nil
	}
	if wrapped, ok, err := r.wrapToUnion(expr, srcNode, r.retType); err != nil {
		return nil, err
	} else if ok {
		return wrapped, nil
	}
	// A value returned into a slot of a different fixed shape where either shape
	// carries an optional property cannot compile as one Go struct assigned to
	// another, so it hands back the way it did before optional shapes interned.
	if err := r.guardOptionalShapeCrossTypes(r.prog.TypeAt(srcNode), r.retType); err != nil {
		return nil, err
	}
	srcDyn := r.isDynamic(srcNode)
	tgtDyn := r.retType.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0
	switch {
	case srcDyn && !tgtDyn:
		return r.coerceDynamicToStaticFlags(expr, r.retType.Flags)
	case !srcDyn && tgtDyn:
		return r.boxStaticToDynamic(expr, srcNode)
	default:
		return r.bridgeClassBinding(expr, srcNode, r.retType)
	}
}

// coerceToTarget bridges a value from a source node's type to a target node's type
// across the dynamic boundary, the coercion an assignment or a binding applies when
// one side is dynamic and the other static. A dynamic source into a static target
// coerces through ToNumber and its siblings; a static source into a dynamic target
// boxes through the value constructors; matching sides pass through unchanged.
func (r *Renderer) coerceToTarget(expr ast.Expr, src, target frontend.Node) (ast.Expr, error) {
	if boxed, ok, err := r.boxToOptional(expr, src, r.prog.TypeAt(target)); err != nil {
		return nil, err
	} else if ok {
		return boxed, nil
	}
	if wrapped, ok, err := r.wrapToUnion(expr, src, r.prog.TypeAt(target)); err != nil {
		return nil, err
	} else if ok {
		return wrapped, nil
	}
	// A value bound into a slot of a different fixed shape where either shape
	// carries an optional property cannot compile as one Go struct assigned to
	// another, so it hands back the way it did before optional shapes interned.
	if err := r.guardOptionalShapeCross(src, r.prog.TypeAt(target)); err != nil {
		return nil, err
	}
	srcDyn := r.isDynamic(src)
	tgtDyn := r.isDynamic(target)
	switch {
	case srcDyn && !tgtDyn:
		return r.coerceDynamicToStatic(expr, target)
	case !srcDyn && tgtDyn:
		return r.boxStaticToDynamic(expr, src)
	default:
		return r.bridgeClassBinding(expr, src, r.prog.TypeAt(target))
	}
}

// bridgeClassBinding bridges a binding whose source is one lowered class and
// whose target declares another. The one bridge built here is the upcast: a
// derived instance flowing into an ancestor-typed slot becomes the address of
// its embedded base, the same object under the base's static type, which Go
// promotion reaches through any embedding depth with the single selector. The
// promoted vtable pointer rides along, so a virtual call through the upcast
// value still dispatches to the derived override, the JavaScript behavior.
// Any other cross-class binding (a downcast, structural twins) hands back;
// matching classes and non-class sides pass through untouched.
func (r *Renderer) bridgeClassBinding(expr ast.Expr, src frontend.Node, target frontend.Type) (ast.Expr, error) {
	srcInfo, ok := r.classOfNode(src)
	if !ok {
		return expr, nil
	}
	tgtInfo, ok := r.classOfType(target)
	if !ok || tgtInfo == srcInfo {
		return expr, nil
	}
	if srcInfo.descendsFrom(tgtInfo) {
		return &ast.UnaryExpr{
			Op: token.AND,
			X:  &ast.SelectorExpr{X: expr, Sel: ident(tgtInfo.goName)},
		}, nil
	}
	return nil, &NotYetLowerable{Reason: "binding a " + srcInfo.name + " instance to a " + tgtInfo.name + "-typed slot is a later slice"}
}

// sameGoType reports whether two lowered type expressions print to the same Go
// source, the test map uses to keep its callback within the same-element-type
// form the value method supports. Comparing the printed form is enough: the two
// expressions are both built by typeExpr, so identical types produce identical
// syntax, and any difference in element type shows up as a difference in text.
func sameGoType(a, b ast.Expr) (bool, error) {
	as, err := printExpr(a)
	if err != nil {
		return false, err
	}
	bs, err := printExpr(b)
	if err != nil {
		return false, err
	}
	return as == bs, nil
}
