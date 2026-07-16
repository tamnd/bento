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
	return r.primitiveFlags(n)&frontend.TypeString != 0 || r.caughtErrorStringRead(n) || r.isTypeofExpr(n) || r.conditionalStringValued(n)
}

// conditionalStringValued reports whether n is a ternary whose branches both
// lower to a value.BStr, the shape `cond ? "a" : "b"`. The checker types the
// whole ternary as the union of its branch types ("a" | "b"), and a closed
// string-literal union folds no String facet (primitiveFlagsOfType keeps String
// out of the union mask because such a union is otherwise a tag enum), so the
// ternary node carries no string flag even though conditionalExpr lowers it to a
// value.BStr IIFE. isString consults this so a ternary of strings coerces as the
// string it is rather than handing back, the same rescue caughtErrorStringRead
// gives a read the checker leaves untyped. It fires only for a conditional
// expression node, never for a tag-enum-typed binding, so a real tag value still
// takes its own path.
func (r *Renderer) conditionalStringValued(n frontend.Node) bool {
	if n.Kind() != frontend.NodeConditionalExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) != 5 {
		return false
	}
	return r.branchStringValued(kids[2]) && r.branchStringValued(kids[4])
}

// branchStringValued reports whether a ternary branch lowers to a value.BStr,
// seeing through parentheses so `cond ? ("a") : "b"` reads the same as the bare
// literal. A nested string ternary is caught by the isString delegation, which
// re-enters conditionalStringValued on the strictly smaller inner node.
func (r *Renderer) branchStringValued(n frontend.Node) bool {
	if n.Kind() == frontend.NodeParenthesizedExpression {
		if kids := r.prog.Children(n); len(kids) == 1 {
			return r.branchStringValued(kids[0])
		}
	}
	return r.isString(n)
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

// isSymbolKey reports whether a computed key lowers to a symbol value, the guard
// the dynamic element store and load use to route a key through SetElem and GetElem
// where the boxed symbol keys the bag by identity. A key annotated symbol carries
// the flag isSymbol reads; a well-known symbol read off the ambient Symbol global,
// whose lib type is the flagless unique symbol, carries no flag, so it is
// recognized structurally instead. That is what lets o[Symbol.toStringTag] land in
// the symbol bag rather than hand back as a non-number, non-string index.
func (r *Renderer) isSymbolKey(n frontend.Node) bool {
	return r.isSymbol(n) || r.isWellKnownSymbolRef(n)
}

// isWellKnownSymbolRef reports whether n reads a well-known symbol off the ambient
// Symbol global, the Symbol.toStringTag and Symbol.match forms member.go lowers to
// the interned identity in the value model. The checker types those reads the
// flagless unique symbol, so isSymbol misses them; this recovers them by shape so a
// computed key or an identity compare over a well-known symbol still routes as a
// symbol.
func (r *Renderer) isWellKnownSymbolRef(n frontend.Node) bool {
	if n.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return false
	}
	if !r.isGlobalRef(kids[0], "Symbol") {
		return false
	}
	_, ok := wellKnownSymbolAccessor(r.prog.Text(kids[1]))
	return ok
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
	// A call to a user-defined overloaded function runs its all-dynamic implementation,
	// whose Go func returns a value.Value. The checker narrows the call to the matched
	// overload's return, a concrete type the box does not carry as that Go type, so the
	// call reads as dynamic by shape here to keep the box on the dynamic path where the
	// surrounding coercion unwraps it.
	if r.callOfOverloadedFunc(n) {
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
	// A re.exec(s), str.match(re), or str.split(re) call returns the boxed value.Value
	// the match yields, an array or null. The checker types each with a concrete Go
	// shape the box does not have (RegExpExecArray | null, RegExpMatchArray | null, and
	// string[]), so isDynamic recognizes the call by shape to keep the box on the
	// dynamic path, where the null compare and the element and property reads off the
	// result dispatch through the value model.
	if r.regExpBoxedResultCall(n) {
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
	// A property or element read off a dynamic receiver lowers to a Get on the box,
	// which yields a box unless the read's own type is a clean primitive that
	// unboxDynamicRead coerces down. So the read is itself dynamic when its type is not
	// one of those primitives, which keeps a further member or element read off it, the
	// m.groups.year chain off a boxed exec result being the motivating case, on the
	// dynamic Get path rather than folding to a fixed-shape miss on an index-signature
	// type the box does not actually carry as Go fields.
	if n.Kind() == frontend.NodePropertyAccessExpression || n.Kind() == frontend.NodeElementAccessExpression {
		if kids := r.prog.Children(n); len(kids) >= 1 && r.isDynamic(kids[0]) {
			if r.prog.TypeAt(n).Flags&(frontend.TypeNumber|frontend.TypeString|frontend.TypeBoolean) == 0 {
				return true
			}
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
	// A typed-array element read flowing into a dynamic slot boxes through GetIndex,
	// the read that answers the undefined an out-of-range or non-canonical index
	// gives, rather than value.Number over the numeric At, which would box a stand-in
	// 0 where the spec reads undefined. It routes before the primitive number case,
	// which would otherwise wrap the numeric read; the lowered expr from that path is
	// dropped for the GetIndex form.
	if boxed, ok, err := r.typedArrayBoxedRead(src); err != nil {
		return nil, err
	} else if ok {
		return boxed, nil
	}
	// A function value flowing into a dynamic slot boxes into a callable value.Value,
	// so a dynamic call site can invoke it without knowing its static signature. It
	// routes before the primitive switch, whose kind tests a function type would
	// otherwise fall past to the handback.
	if calls, _ := r.prog.Signatures(r.prog.TypeAt(src)); len(calls) == 1 {
		return r.boxFuncToDynamic(expr, calls[0])
	}
	// An optional (T | undefined) flowing into a dynamic slot boxes through
	// value.OptToValue, which yields the element's box when present and the undefined
	// singleton when not, the box an array's at or pop and a member the checker types
	// number | undefined take when they reach console.log or another dynamic sink. It
	// routes before the primitive switch, whose kind tests read the union member and
	// would box the present case while dropping the undefined one.
	if boxed, ok, err := r.boxOptionalToDynamic(expr, src); err != nil {
		return nil, err
	} else if ok {
		return boxed, nil
	}
	r.requireImport(valuePkg)
	switch {
	case r.isNumber(src):
		return &ast.CallExpr{Fun: sel("value", "Number"), Args: []ast.Expr{expr}}, nil
	case r.isString(src):
		return &ast.CallExpr{Fun: sel("value", "StringValue"), Args: []ast.Expr{expr}}, nil
	case r.isBool(src):
		return &ast.CallExpr{Fun: sel("value", "Bool"), Args: []ast.Expr{expr}}, nil
	case r.isSymbolKey(src):
		// A symbol expression already lowers to a value.Value: Symbol(x) builds one,
		// a symbol binding stores it, a symbol read off the bag hands one back, and a
		// well-known symbol read (Symbol.toPrimitive) lowers to its interned identity.
		// So boxing a symbol into a dynamic slot is the identity, the way null and
		// undefined are boxes already. isSymbolKey covers the well-known form too,
		// whose flagless unique-symbol type isSymbol alone would miss.
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

// boxOptionalToDynamic boxes a T | undefined result into a dynamic value.Value. The
// optional lowers to a value.Opt[T], so the box threads it through value.OptToValue
// with the element's own box constructor as the present-case wrapper: value.Number
// for a numeric optional, value.StringValue for a string one, value.Bool for a
// boolean. Each constructor already has the func(T) value.Value shape OptToValue
// wants, so it passes as the wrapper directly with no closure. It reports ok=false
// when the source is not an optional, so a non-union type falls through to the
// primitive path, and hands back for an optional of a shape with no dynamic box yet
// rather than emit a call that would not compile.
func (r *Renderer) boxOptionalToDynamic(expr ast.Expr, src frontend.Node) (ast.Expr, bool, error) {
	t := r.prog.TypeAt(src)
	if !r.isOptionalType(t) {
		return nil, false, nil
	}
	inner, ok := r.optionalInner(r.prog.UnionMembers(t))
	if !ok {
		return nil, false, nil
	}
	var box ast.Expr
	switch {
	case inner.Flags&frontend.TypeNumber != 0:
		box = sel("value", "Number")
	case inner.Flags&frontend.TypeString != 0:
		box = sel("value", "StringValue")
	case inner.Flags&frontend.TypeBoolean != 0:
		box = sel("value", "Bool")
	default:
		return nil, false, &NotYetLowerable{Reason: "boxing an optional of this type into a dynamic value is a later slice"}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "OptToValue"), Args: []ast.Expr{expr, box}}, true, nil
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
	// A call to a user-defined overloaded function runs its all-dynamic implementation,
	// whose Go func returns a value.Value box. The checker narrows the call to the matched
	// overload's return, a concrete type the primitive box path would try to construct, so
	// recognizing the call here lets a slot, a stringify, or a console.log take the box
	// straight through the same as any other boxed result.
	return r.isDynamicDescriptorRead(src) || r.isProxyRevocableCall(src) || r.isIterTerminalBoxedCall(src) || r.callOfOverloadedFunc(src)
}

// isIterTerminalBoxedCall reports whether src is a terminal iterator helper whose
// result lowers to a value.Value box: reduce folds the source to the accumulator and
// toArray collects it into an array, both returned as boxes (see value.IterReduce and
// value.IterToArray). The checker types reduce as the accumulator type and toArray as
// an array, neither of which the primitive box path has a constructor for, so
// recognizing the calls here is what lets const x: any = it.reduce(...) and a
// console.log(it.toArray()) store or print the box straight through. find joins them:
// it returns the first passing value or undefined as a box. some and every return a Go
// bool, not a box, and forEach returns undefined for its side effect, so none of those
// are claimed here.
func (r *Renderer) isIterTerminalBoxedCall(src frontend.Node) bool {
	if src.Kind() != frontend.NodeCallExpression {
		return false
	}
	kids := r.prog.Children(src)
	if len(kids) < 1 {
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
	method := r.prog.Text(ck[1])
	if method != "reduce" && method != "toArray" && method != "find" {
		return false
	}
	return r.isIterHelperReceiver(ck[0])
}

// isProxyRevocableCall reports whether src is a Proxy.revocable(target, handler)
// call, which lowers to value.ProxyRevocable and returns a value.Value: the
// { proxy, revoke } object as a live box. The checker types the result a static
// object shape, which the primitive box path has no constructor for, so recognizing
// the call here is what lets const r: any = Proxy.revocable(t, h) store the box
// straight through rather than hand back trying to box the static shape.
func (r *Renderer) isProxyRevocableCall(src frontend.Node) bool {
	if src.Kind() != frontend.NodeCallExpression {
		return false
	}
	kids := r.prog.Children(src)
	if len(kids) < 1 {
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
	return r.isGlobalRef(ck[0], "Proxy") && r.prog.Text(ck[1]) == "revocable"
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
		if p.Kind() == frontend.NodeMethodDeclaration {
			member, err := r.boxObjectMethodMember(obj, p)
			if err != nil {
				return nil, err
			}
			obj = member
			continue
		}
		if p.Kind() != frontend.NodeUnknown {
			return nil, &NotYetLowerable{Reason: "boxing an object literal with an accessor member is a later slice"}
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

// boxObjectMethodMember boxes one method member of an object literal onto obj, the
// live object boxObjectLiteral is building, and returns the extended object
// expression. A method lowers here only in the narrow shape the coercion items
// need: a plain method (no get, set, async, generator, or private marker) with no
// declared parameters, whose body is a single `return <expr>`, and whose returned
// expression neither reads `this` nor names a parameter. The method becomes a
// value.NewFunc closure that ignores its arguments and returns the boxed
// expression, so a coercion protocol lookup finds a callable that yields the value.
// A named method writes its slot through Set; a well-known computed name like
// [Symbol.toPrimitive] boxes the key and writes through SetKeyed, the same slot the
// runtime's Symbol.toPrimitive probe reads. Any richer method (a parameter, a body
// that is more than one return, a this reference) hands back to a later slice.
func (r *Renderer) boxObjectMethodMember(obj ast.Expr, m frontend.Node) (ast.Expr, error) {
	// Strip and reject modifiers. A childless leading unnamed node is a keyword
	// marker (async, the generator star) or a private name (#m); each is a shape
	// this slice does not box. A computed name [expr] wraps an expression, so it
	// carries a child and is not stripped: it is the method's name, read below.
	kids := r.prog.Children(m)
	if len(kids) > 0 && kids[0].Kind() == frontend.NodeUnknown && len(r.prog.Children(kids[0])) == 0 {
		w := strings.TrimSpace(r.prog.Text(kids[0]))
		if strings.HasPrefix(w, "#") {
			return nil, &NotYetLowerable{Reason: "boxing a private object method is a later slice"}
		}
		return nil, &NotYetLowerable{Reason: "boxing a " + w + " object method is a later slice"}
	}
	if len(kids) == 0 {
		return nil, &NotYetLowerable{Reason: "boxing an object method without a name is a later slice"}
	}
	nameNode := kids[0]
	// A declared parameter would need the receiver-bound argument binding this
	// closure does not build, so a parameterless method is the only shape boxed.
	for _, k := range kids {
		if k.Kind() == frontend.NodeParameter {
			return nil, &NotYetLowerable{Reason: "boxing an object method with a parameter is a later slice"}
		}
	}
	fn, err := r.boxMethodClosure(m)
	if err != nil {
		return nil, err
	}
	if inner, ok := r.computedKey(m); ok {
		boxedKey, err := r.boxOperand(inner)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: obj, Sel: ident("SetKeyed")}, Args: []ast.Expr{boxedKey, fn}}, nil
	}
	name, ok := r.memberName(nameNode)
	if !ok {
		return nil, r.memberNameReason(nameNode, "method")
	}
	return &ast.CallExpr{
		Fun: &ast.SelectorExpr{X: obj, Sel: ident("Set")},
		Args: []ast.Expr{
			&ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{
				&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(name)},
			}},
			fn,
		},
	}, nil
}

// boxMethodClosure lowers a parameterless object method whose body is a single
// return into a value.NewFunc closure that ignores its arguments and returns the
// boxed return expression. The receiver scope is cleared before the return
// expression lowers, so a `this` in the body declines through boxOperand rather
// than binding to an enclosing class receiver this free closure does not carry;
// that decline is what keeps a this-reading method out of this slice.
func (r *Renderer) boxMethodClosure(m frontend.Node) (ast.Expr, error) {
	block, ok := r.funcBodyBlock(m)
	if !ok {
		return nil, &NotYetLowerable{Reason: "boxing an object method with no body is a later slice"}
	}
	stmts := r.prog.Children(block)
	if len(stmts) != 1 || stmts[0].Kind() != frontend.NodeReturnStatement {
		return nil, &NotYetLowerable{Reason: "boxing an object method whose body is not a single return is a later slice"}
	}
	retKids := r.prog.Children(stmts[0])
	if len(retKids) != 1 {
		return nil, &NotYetLowerable{Reason: "boxing an object method with a bare or multi-value return is a later slice"}
	}
	prevClass, prevThis := r.curClass, r.thisName
	r.curClass, r.thisName = nil, ""
	defer func() { r.curClass, r.thisName = prevClass, prevThis }()
	boxed, err := r.boxOperand(retKids[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	const argsName = "__a"
	thunk := &ast.FuncLit{
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{{
				Names: []*ast.Ident{ident(argsName)},
				Type:  &ast.ArrayType{Elt: sel("value", "Value")},
			}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "Value")}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{boxed}}}},
	}
	return &ast.CallExpr{Fun: sel("value", "NewFunc"), Args: []ast.Expr{thunk}}, nil
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

// unboxDynamicRead adapts a read off a boxed receiver, a value.Value the runtime
// Get or GetIndex yields, to the type the checker gave the read. A receiver typed
// any yields an any-typed read, so the box passes through untouched, the common
// case. A receiver the compiler boxed while the checker kept a concrete type, a
// RegExp exec result's string element or number .index being the first, gives the
// read a primitive type its consumer expects unboxed, so the box coerces down
// through the ToNumber family the same way an IteratorResult .value does. A
// non-primitive read type, an object or array, keeps the box, since there is no
// single Go value to coerce it to here.
func (r *Renderer) unboxDynamicRead(read ast.Expr, n frontend.Node) (ast.Expr, error) {
	flags := r.prog.TypeAt(n).Flags
	// A read whose type is a clean primitive (number, string, or boolean with no any
	// or unknown facet) has one Go value to coerce the box down to, so it coerces even
	// when a shape query flagged the read dynamic. An index-signature read like
	// m.groups.year resolves to string through the signature, but the fixed-shape query
	// that backs missingPropertyRead sees no declared "year" field and reports the read
	// dynamic, which would otherwise leave the box uncoerced where its string consumer
	// expects a bstr. Keying off the precise type first coerces it correctly.
	if flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 &&
		flags&(frontend.TypeNumber|frontend.TypeString|frontend.TypeBoolean) != 0 {
		return r.coerceDynamicToStaticFlags(read, flags)
	}
	// A read the checker left any or unknown, or gave a non-primitive shape, keeps the
	// box: there is no single Go value to coerce it to here.
	return read, nil
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
		// A not-assignable value the front door tolerated under code 2345 reaches this
		// bridge once the optional and union widenings above have declined it, so a value
		// whose Go type differs from the slot hands back rather than emit an assignment
		// across two Go types that does not compile (see staticReprMismatch).
		if r.staticReprMismatch(src, r.prog.TypeAt(src), r.prog.TypeAt(target)) {
			return nil, &NotYetLowerable{Reason: "a value the checker calls not assignable whose Go type differs from the slot is a later slice"}
		}
		// An assignment or initializer the front door tolerated under code 2322 reaches
		// this bridge too, with the diagnostic anchored on the target name (or on the
		// value for an array element). A mismatch whose two sides lower to different Go
		// types hands back; a same-representation 2322 (a literal type receiving another
		// literal of the same primitive) falls through and lowers (see assignmentReprMismatch).
		if r.assignmentReprMismatch(src, target) {
			return nil, &NotYetLowerable{Reason: "an assignment the checker calls not assignable whose Go type differs from the slot is a later slice"}
		}
		return r.bridgeClassBinding(expr, src, r.prog.TypeAt(target))
	}
}

// staticReprMismatch reports whether a static source bound into a static target would
// drop a Go value into a slot of another Go type at a site the checker itself calls not
// assignable. It fires only where the checker reported code 2345 against the source
// node, so a type-correct binding that reaches the same bridge through a legitimate
// structural coercion (a shaped object literal, a boxed computed key) is left alone;
// only when neither side is a class, since a class binding keeps its own upcast bridge
// whose change of Go type is deliberate; and only when the two types provably lower to
// different Go types, the direct proof the emitted Go would not compile. The argument
// bridge and the constructor bridge share it so a not-assignable value the front door
// tolerated under code 2345 hands back on either path rather than emit Go the toolchain
// rejects.
func (r *Renderer) staticReprMismatch(srcNode frontend.Node, srcType, tgtType frontend.Type) bool {
	if !r.notAssignableAt(srcNode) {
		return false
	}
	if _, srcClass := r.classOfNode(srcNode); srcClass {
		return false
	}
	if _, tgtClass := r.classOfType(tgtType); tgtClass {
		return false
	}
	return r.mismatchedLoweredType(srcType, tgtType)
}

// notAssignableAt reports whether the checker put a code 2345 (argument not assignable)
// diagnostic on the given node, the front-door-tolerated error the representation guard
// keys off. The checker anchors the argument-not-assignable error on the argument
// expression itself, so the match is by shared token extent (see spanCoversNode). That
// extent keeps the guard and the end-of-render reconciliation sound. A user call wrapping
// a builtin call, f([1,2,3].indexOf("x")), carries the 2345 on the inner "x", not on the
// outer argument f is passed, so the outer bridge does not mistake the inner mismatch for
// its own and mark it handled. A match records the span as seen, so unguardedNotAssign can
// tell which 2345 sites a guarded bridge reached.
func (r *Renderer) notAssignableAt(node frontend.Node) bool {
	if node == nil {
		return false
	}
	r.ensureNotAssignSpans()
	for _, s := range r.notAssignSpans {
		if spanCoversNode(s, node) {
			r.seenAssign[s] = true
			return true
		}
	}
	return false
}

// spanCoversNode reports whether a diagnostic span anchors on node, allowing for the
// leading trivia a node's full start carries but the diagnostic's start does not. The
// frontend reports a node's Pos() from its full start, which includes any leading
// whitespace or comment before the first token, while the checker anchors a diagnostic on
// the token's real start past that trivia (const n: 0 = 1 gives the name node a Pos of 5,
// the space, but the 2322 a start of 6, the n). So the two share an end and the
// diagnostic's start falls at or after the node's full start. Matching on the shared end
// keeps the guard sound the way an exact match did: a nested inner node, the "x" inside
// f([1,2,3].indexOf("x")), ends before its enclosing argument, so it cannot be mistaken
// for it even though both starts sit past their own leading trivia.
func spanCoversNode(s frontend.Span, node frontend.Node) bool {
	return s.End == node.End() && node.Pos() <= s.Start && s.Start <= s.End
}

// assignmentReprMismatch reports whether a static value bound into a static slot would
// drop a Go value into a slot of another Go type at a site the checker calls not
// assignable under code 2322, the initializer/assignment analog of staticReprMismatch's
// 2345. A 2322 anchors on the target name for an initializer, an assignment, or a
// property declaration, and on the value for an array element, so it matches both the
// source and the target node against the tolerated 2322 spans and records either as seen
// so the end-of-render reconciliation knows the binding bridge reached this site. When a
// site matches it hands back only if neither side is a class and the two types provably
// lower to different Go types; a match whose two sides share a Go type (a literal type
// receiving another literal of the same primitive, both float64) is left to lower.
func (r *Renderer) assignmentReprMismatch(srcNode, tgtNode frontend.Node) bool {
	srcHit := r.assign2322At(srcNode)
	tgtHit := r.assign2322At(tgtNode)
	if !srcHit && !tgtHit {
		return false
	}
	if _, srcClass := r.classOfNode(srcNode); srcClass {
		return false
	}
	tgtType := r.prog.TypeAt(tgtNode)
	if _, tgtClass := r.classOfType(tgtType); tgtClass {
		return false
	}
	return r.mismatchedLoweredType(r.prog.TypeAt(srcNode), tgtType)
}

// assign2322At reports whether the checker put a code 2322 (assignment not assignable)
// diagnostic on the given node and records a match as seen. The binding bridge calls it
// for both the source and the target node because a 2322 does not anchor on one fixed
// role: an initializer and an assignment carry it on the target name, an array element
// carries it on the value. The shared-extent match (spanCoversNode) keeps it sound the
// same way it does for 2345, and folds in the leading trivia a declaration name node
// carries: const n: 0 = 1 gives the name node a Pos at the space before n while the 2322
// starts at n itself.
func (r *Renderer) assign2322At(node frontend.Node) bool {
	if node == nil {
		return false
	}
	r.ensureNotAssignSpans()
	for _, s := range r.assign2322Spans {
		if spanCoversNode(s, node) {
			r.seenAssign[s] = true
			return true
		}
	}
	return false
}

// ensureNotAssignSpans collects the program's code 2345 and 2322 spans once and caches
// them, so both the per-site guards and the end-of-render reconciliation read the same
// sets without re-querying the checker.
func (r *Renderer) ensureNotAssignSpans() {
	if r.notAssignReady {
		return
	}
	for _, d := range r.prog.Diagnostics() {
		switch d.Code {
		case 2345:
			r.notAssignSpans = append(r.notAssignSpans, d.Span)
		case 2322:
			r.assign2322Spans = append(r.assign2322Spans, d.Span)
		case 2769:
			r.overload2769Spans = append(r.overload2769Spans, d.Span)
		}
	}
	r.notAssignReady = true
}

// markOverloadCallSeen records the 2769 span the overloaded-call path lowered, so the
// end-of-render reconciliation does not hand the unit back for it. A valid overloaded
// call carries no 2769 and marks nothing; only a checker-rejected call the boxed
// dispatch handled matches a span here. The checker anchors a 2769 on the offending
// argument, which sits inside the call expression, so the span is matched by containment
// in the call node's range rather than by the shared-end rule a target-anchored code uses.
func (r *Renderer) markOverloadCallSeen(call frontend.Node) {
	r.ensureNotAssignSpans()
	for _, s := range r.overload2769Spans {
		if call.Pos() <= s.Start && s.End <= call.End() {
			r.seenAssign[s] = true
		}
	}
}

// unguardedNotAssign reports the first tolerated-code site (2345, 2322, or 2769) no
// guarded path inspected, or nil if every one the front door tolerated flowed through the
// argument, constructor, or binding bridge that either lowered it safely or handed it
// back. A site left unseen was lowered by a path with no representation guard, a builtin
// higher-order method callback, a builtin element-slot argument, or an assignment
// construct no bridge reaches, whose emitted Go drops a value into a slot of another Go
// type and does not compile. Rather than ship that, the whole unit hands back to the
// interpreter, which keeps the front-door tolerance zero-fail no matter how many such
// paths exist: as more of them grow the guard, more of these programs lower, and until
// then they route to the engine exactly as they did before the front door admitted the
// code at all.
func (r *Renderer) unguardedNotAssign() error {
	r.ensureNotAssignSpans()
	for _, s := range r.notAssignSpans {
		if !r.seenAssign[s] {
			return &NotYetLowerable{Reason: "an argument the checker calls not assignable reaches a builtin lowering with no representation guard, so the unit routes to the interpreter until that path grows one"}
		}
	}
	for _, s := range r.assign2322Spans {
		if !r.seenAssign[s] {
			return &NotYetLowerable{Reason: "a value the checker calls not assignable reaches an assignment construct with no representation guard, so the unit routes to the interpreter until that construct grows one"}
		}
	}
	for _, s := range r.overload2769Spans {
		if !r.seenAssign[s] {
			return &NotYetLowerable{Reason: "a call with no matching overload reaches a path with no overloaded-call guard, so the unit routes to the interpreter until that path grows one"}
		}
	}
	return nil
}

// mismatchedLoweredType reports whether two checker types provably lower to different
// Go types. It lowers each through typeExpr and compares the printed Go, so a number
// and a numeric-literal union that both fold to float64 read as the same
// representation while a number and a string do not. It answers only what it can
// prove: a type that does not lower yet, a type parameter awaiting monomorphization
// being the first, leaves typeExpr with a handback, and the check declines to false so
// a bridge keeps its prior behavior rather than convert an unrelated lowering gap into
// a spurious representation mismatch.
func (r *Renderer) mismatchedLoweredType(a, b frontend.Type) bool {
	ae, err := r.typeExpr(a)
	if err != nil {
		return false
	}
	be, err := r.typeExpr(b)
	if err != nil {
		return false
	}
	same, err := sameGoType(ae, be)
	if err != nil {
		return false
	}
	return !same
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
