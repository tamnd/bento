package lower

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers member access: property reads, element access, and the Math
// and Number constants a property read can name.

// classKeyRead lowers a read of member prop off a class receiver. A static read
// off the class name lowers to the package var a static field became or the call
// a static get accessor became; an instance read off this or a typed instance
// lowers to the struct-field selector a field became or the accessor-method call
// a get accessor became. It returns ok=false when obj is not a class receiver, so
// the caller falls through to its other receiver paths, and a hand-back for a
// class receiver whose prop only a set accessor serves, names a method read as a
// value (a bound-function value is a later slice), or names no member this slice
// reads. A dotted read A.total and a bracket read A["total"] with a constant
// string key share this, so o.k and o["k"] resolve the same class member.
func (r *Renderer) classKeyRead(obj frontend.Node, prop string) (ast.Expr, bool, error) {
	if obj.Kind() == frontend.NodeIdentifier {
		if info, ok := r.classNameRef(obj); ok {
			if f, ok := info.staticByName(prop); ok {
				return ident(f.goName), true, nil
			}
			// A read C.x through a static get accessor lowers to the call CX(), the
			// package function the accessor became, the static twin of an instance
			// getter read routing to c.X().
			if g, ok := info.staticGetterByName(prop); ok {
				return &ast.CallExpr{Fun: ident(g.goName)}, true, nil
			}
			if _, isSetter := info.staticSetterByName(prop); isSetter {
				return nil, true, &NotYetLowerable{Reason: "reading the write-only static accessor ." + prop + " of class " + info.name + " is a later slice"}
			}
			if _, isMethod := info.staticMethodByName(prop); isMethod {
				return nil, true, &NotYetLowerable{Reason: "a static method of class " + info.name + " read as a value is a later slice"}
			}
			return nil, true, &NotYetLowerable{Reason: "class " + info.name + " has no static ." + prop + " this slice lowers"}
		}
	}
	if info, ok := r.classReceiver(obj); ok {
		f, isField := info.lookupField(prop)
		g, isGetter := info.lookupGetter(prop)
		if !isField && !isGetter {
			if _, isMethod := info.lookupMethod(prop); isMethod {
				return nil, true, &NotYetLowerable{Reason: "a method of class " + info.name + " read as a value is a later slice"}
			}
			if _, isSetter := info.lookupSetter(prop); isSetter {
				return nil, true, &NotYetLowerable{Reason: "reading the write-only accessor ." + prop + " of class " + info.name + " is a later slice"}
			}
			return nil, true, &NotYetLowerable{Reason: "class " + info.name + " has no property ." + prop + " this slice lowers"}
		}
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, true, err
		}
		if isGetter {
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(g.goName)}}, true, nil
		}
		return &ast.SelectorExpr{X: recv, Sel: ident(f.goName)}, true, nil
	}
	return nil, false, nil
}

// propertyAccess lowers a member expression. Two members are covered: .length on
// a string, which is the code-unit count and lowers to the value.BStr Length
// method, a float64 that matches the number type the checker gives .length; and a
// numeric constant on the Math or Number namespace (Math.PI, Number.EPSILON, and
// their siblings), which is a property read on a global rather than a method call,
// so it lowers to the matching value-package constant. Every other property (a
// field of a lowered object, a method call, .length on an array) is its own later
// slice and hands back.
// wellKnownSymbolAccessor maps a well-known symbol name read off the ambient Symbol
// global to the value package accessor that returns its interned identity, so
// Symbol.match lowers to value.SymbolMatch(). Symbol.iterator and
// Symbol.asyncIterator are deliberately absent: the iteration protocol recognizes
// them by name through the class-member mechanism rather than as a value read. A
// name that is not a well-known symbol reports false and keeps its own lookup.
func wellKnownSymbolAccessor(prop string) (string, bool) {
	switch prop {
	case "hasInstance":
		return "SymbolHasInstance", true
	case "isConcatSpreadable":
		return "SymbolIsConcatSpreadable", true
	case "match":
		return "SymbolMatch", true
	case "matchAll":
		return "SymbolMatchAll", true
	case "replace":
		return "SymbolReplace", true
	case "search":
		return "SymbolSearch", true
	case "species":
		return "SymbolSpecies", true
	case "split":
		return "SymbolSplit", true
	case "toPrimitive":
		return "SymbolToPrimitive", true
	case "toStringTag":
		return "SymbolToStringTag", true
	case "unscopables":
		return "SymbolUnscopables", true
	}
	return "", false
}

func (r *Renderer) propertyAccess(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	// An optional property access a?.b carries a ?. token between the receiver and
	// the name, so the node exposes three children rather than two. The token is a
	// leaf bento does not name, recognized by its source text; when it is present
	// the access short-circuits on a nullish receiver and routes to its own lowering.
	if len(kids) == 3 && r.isQuestionDotToken(kids[1]) {
		return r.optionalChainAccess(kids[0], kids[2])
	}
	if len(kids) != 2 {
		return nil, &NotYetLowerable{Reason: "property access did not expose an object and a property name"}
	}
	obj, nameNode := kids[0], kids[1]
	prop := r.prog.Text(nameNode)
	// A member of a sibling namespace import read as a value, m.inc passed as a
	// callback or m.pi read for its number, has no Go value behind it: the export is a
	// package-level declaration the call path resolves by name, but a value read would
	// need the namespace materialized as a struct this slice does not build. It hands
	// back so the unit routes to the engine rather than emit a selector on a binding
	// with no Go storage. A call m.inc(1) is intercepted on the call path before it
	// reaches here, so only a value read routes through this guard.
	if obj.Kind() == frontend.NodeIdentifier && r.internalNamespaces[r.prog.Text(obj)] {
		return nil, &NotYetLowerable{Reason: "a namespace member read as a value is a later slice"}
	}
	// A read of any member off the ambient Iterator constructor (Iterator.prototype, its
	// name, its length) reaches for the constructor's reflective identity: the shared
	// %Iterator.prototype% the helper results inherit and the constructor's own metadata.
	// The static model hosts the helper results as concrete runtime iterators
	// (value.IterHelper) and Iterator.from as a call, but it does not host Iterator itself
	// as a first-class reflective object, so a bare member read hands back with the ceiling
	// named rather than the generic constructable-callable reason the receiver lowering
	// would give. Iterator.from(...) as a call is intercepted on the call path before here,
	// so only a bare read reaches this. The abstract-instantiation semantics (new Iterator()
	// throwing) are enforced by the checker, which rejects constructing an abstract class.
	if r.isGlobalRef(obj, "Iterator") {
		return nil, &NotYetLowerable{Reason: "the Iterator constructor and its prototype identity are a reflective surface the static model does not host"}
	}
	// arguments.length reads the count of arguments the call supplied. The current
	// body materialized a *value.Array[value.Value] store from its parameters, and
	// the parameter count is the call arity for the all-required signatures that reach
	// materialization, so the store's Len is that count. It routes before the static
	// paths, which would read a field off the IArguments shape the checker gives
	// arguments.
	if r.argsObjName != "" && prop == "length" && r.isArgumentsIdent(obj) {
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(r.argsObjName), Sel: ident("Len")}}, nil
	}
	// A read of a caught error's .message or .name lowers to the matching method on
	// the *value.Error the catch bound. It routes before the dynamic path because
	// the checker types a catch binding unknown, which would otherwise send the read
	// through the boxed-value Get the error value does not carry. Only these two
	// properties are read; any other member of a caught error hands back.
	if obj.Kind() == frontend.NodeIdentifier {
		if name, ok := localName(r.prog.Text(obj)); ok && r.errorLocals[name] {
			switch prop {
			case "message":
				return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("Message")}}, nil
			case "name":
				return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("Name")}}, nil
			case "constructor":
				// A caught error's .constructor is the constructor value for its name, the
				// same interned value the built-in constructor boxes to, so a comparison
				// like thrown.constructor === TypeError holds by identity and a read of
				// thrown.constructor.name answers the name. It is a boxed value.Value, the
				// any the checker gives a property of a catch binding, so the compare and
				// the name read downstream take the dynamic paths.
				r.requireImport(valuePkg)
				return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("Constructor")}}, nil
			default:
				return nil, &NotYetLowerable{Reason: "a caught error's ." + prop + " is a later slice"}
			}
		}
	}
	// A read of .prototype on an ambient built-in function that is not a constructor
	// (isFinite, isNaN, parseInt, parseFloat, decodeURI, and their siblings) is
	// undefined: these functions carry no prototype property. Bento models such a
	// function only as a call target, not a first-class value, so a bare reference to
	// it has no Go value, and the paths below would lower the receiver to a Go type
	// name and emit a selector on it that does not build. The built-in constructors
	// whose .prototype is a real object (Number, String, Array, Object) hand back
	// before reaching here, so an ambient receiver whose .prototype the checker widens
	// to any is one of the plain functions, and folding to the undefined singleton
	// answers the value the language gives.
	if prop == "prototype" && r.isAmbientGlobal(obj) && r.prog.TypeAt(n).Flags == frontend.TypeAny {
		r.requireImport(valuePkg)
		return sel("value", "Undefined"), nil
	}
	// A member read E.M where E is a registered numeric enum lowers to the
	// member's Go constant, or to the member's inlined value for a const enum. It
	// routes before the dynamic and instance paths because the enum name is a value
	// binding whose read would otherwise fall through to a property lookup.
	if expr, ok, err := r.enumMemberRead(obj, prop); err != nil {
		return nil, err
	} else if ok {
		return expr, nil
	}
	// A read of .description on a statically-typed symbol lowers to the symbol's
	// description, a string or undefined. A dynamic symbol binding takes the Get path
	// below, which answers the same read off the boxed KindSymbol; this branch covers
	// the receiver the checker pinned down to symbol, which is not on the dynamic path.
	if prop == "description" && r.isSymbol(obj) {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("SymbolDescription")}}, nil
	}
	// Symbol.toStringTag and the other well-known symbols read as the one interned
	// identity the value model holds for each, so Symbol.match === Symbol.match holds
	// and a well-known symbol used as a property key lands in the same slot on every
	// read. Only a read off the ambient Symbol global routes here; a like-named
	// property on a user value keeps its own lookup. Symbol.iterator and
	// Symbol.asyncIterator are left to the class-member iteration mechanism that
	// already recognizes them by name, so they are not among the accessors here.
	if r.isGlobalRef(obj, "Symbol") {
		if fn, ok := wellKnownSymbolAccessor(prop); ok {
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", fn)}, nil
		}
	}
	// A read o.k on a dynamic receiver (one typed any or unknown) has no static
	// shape to intern to a Go field, so it dispatches at runtime through the boxed
	// value's Get, which reports a string's length, an array's length and indices,
	// and an object's own properties, and undefined for a miss, the JavaScript
	// result. The property name is a plain string literal here, the source key, so
	// Get looks it up by the same name the value carries. This routes before the
	// static-shape paths below, which expect a receiver whose type the checker
	// pinned down.
	if r.isDynamic(obj) {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		// A read of the legacy __proto__ accessor is the object's prototype, not an own
		// property of that name, so it lowers to GetPrototype rather than a Get on the
		// bag. The write side mirrors this to SetPrototype.
		if prop == "__proto__" {
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("GetPrototype")}}, nil
		}
		key := &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(prop)}}}
		return r.unboxDynamicRead(&ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Get")}, Args: []ast.Expr{key}}, n)
	}
	// A read of .value or .done on an IteratorResult, the { value, done } object a
	// generator's next/return/throw hand back, lowers to the field on the
	// value.IterResult the drive produced: .value carries the yielded or completion
	// value as a dynamic value.Value, .done the Go bool a manual loop stops on. It routes
	// before the interned-shape paths, which would try to derive a struct for the union
	// and hand back on its boolean-literal discriminant.
	if r.isIterResultReceiver(obj) {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		switch prop {
		case "value":
			// .value is a dynamic value.Value in the struct, but the checker types the
			// read as the yielded element (number for Generator<number>), so a static
			// consumer expects that Go type. A dynamic read stays the box; a static one
			// coerces the box down to the number, string, or boolean the checker named.
			field := &ast.SelectorExpr{X: recv, Sel: ident("Value")}
			if r.isDynamic(n) {
				return field, nil
			}
			return r.coerceDynamicToStaticFlags(field, r.prog.TypeAt(n).Flags)
		case "done":
			return &ast.SelectorExpr{X: recv, Sel: ident("Done")}, nil
		default:
			return nil, &NotYetLowerable{Reason: "an IteratorResult's ." + prop + " is a later slice"}
		}
	}
	// A read off a class receiver, a static member off the class name or an
	// instance member off this or a typed instance, routes to the class member
	// dispatch. It routes before the length, size, and interned-shape paths so a
	// class whose fields happen to spell one of those fingerprints is still read
	// as the class it is.
	if expr, ok, err := r.classKeyRead(obj, prop); err != nil || ok {
		return expr, err
	}
	if r.isString(obj) && prop == "length" {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Length")}}, nil
	}
	if prop == "length" {
		_, isArray := r.arrayElem(obj)
		if isArray || r.numericTypedArray(obj) || r.bigintTypedArray(obj) {
			recv, err := r.lowerExpr(obj)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Len")}}, nil
		}
	}
	// map.size reads the entry count of a Map (section 6.5). It is an accessor in the
	// source but a method on value.Map, so it lowers to a Size() call, the same float64
	// the checker gives the property. This routes before the struct-field path, which
	// would otherwise try to intern size as a field of a shape a map is not.
	if prop == "size" && r.isMap(obj) {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Size")}}, nil
	}
	// set.size reads the member count of a Set (section 6.5), the same accessor-to-
	// Size()-method lowering map.size takes, routed before the struct-field path for
	// the same reason.
	if prop == "size" && r.isSet(obj) {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Size")}}, nil
	}
	// BYTES_PER_ELEMENT is the element width in bytes, a constant of the element kind
	// read the same off the constructor (Int32Array.BYTES_PER_ELEMENT) and off an
	// instance (b.BYTES_PER_ELEMENT), typed a Number in both. The static form fires on
	// the constructor global by name and folds to the width literal, since it has no
	// receiver value. The instance form lowers to the view's BytesPerElement method
	// rather than a literal: folding it to a constant would drop the receiver, and a
	// binding whose only use is this read would then be left declared and unused, which
	// Go rejects, so the method call keeps the receiver referenced and stays correct.
	if prop == "BYTES_PER_ELEMENT" {
		if width, ok := bytesPerElement(r.prog.Text(obj)); ok && r.isAmbientGlobal(obj) {
			return &ast.BasicLit{Kind: token.FLOAT, Value: strconv.Itoa(width)}, nil
		}
		if r.numericTypedArray(obj) || r.bigintTypedArray(obj) {
			recv, err := r.lowerExpr(obj)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("BytesPerElement")}}, nil
		}
	}
	// A typed array's geometry getters read off the view: .buffer is the ArrayBuffer
	// it aliases, .byteOffset the byte it starts at within that buffer, and
	// .byteLength its own span in bytes, each a method on the value.TypedArray or
	// value.Uint8Array. They route before the struct-field and ArrayBuffer paths,
	// which would try to intern the name as a field of a shape a typed array is not,
	// or read .byteLength off a buffer the receiver is not. The .length getter shares
	// the Len path above with a dense array, so only these three land here.
	if prop == "buffer" || prop == "byteOffset" || prop == "byteLength" {
		if r.numericTypedArray(obj) || r.bigintTypedArray(obj) || r.isDataView(obj) {
			recv, err := r.lowerExpr(obj)
			if err != nil {
				return nil, err
			}
			method := map[string]string{"buffer": "Buffer", "byteOffset": "ByteOffset", "byteLength": "ByteLength"}[prop]
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(method)}}, nil
		}
	}
	// buffer.byteLength reads the byte size of an ArrayBuffer (section 6.2). It is an
	// accessor in the source but a method on value.ArrayBuffer, so it lowers to a
	// ByteLength() call, the same float64 the checker gives the property. This routes
	// before the struct-field path, which would otherwise try to intern byteLength as
	// a field of a shape a buffer is not.
	if prop == "byteLength" && r.isArrayBuffer(obj) {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ByteLength")}}, nil
	}
	// buffer.detached reports whether the buffer has been detached by a transfer or
	// an explicit detach (25 §25.1.6.3). Like byteLength it is an accessor in the
	// source but a method on value.ArrayBuffer, so it lowers to a Detached() call, the
	// boolean the checker gives the property, and routes before the struct-field path.
	if prop == "detached" && r.isArrayBuffer(obj) {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Detached")}}, nil
	}
	// buffer.maxByteLength and buffer.resizable report the resizable geometry a
	// buffer built with a maxByteLength option carries (25 §25.1.6). Like byteLength
	// and detached they are accessors in the source but methods on value.ArrayBuffer,
	// so they lower to MaxByteLength and Resizable calls, and route before the
	// struct-field path. A fixed-length buffer reads resizable false and its current
	// length as the maximum, the fallback the runtime getters apply.
	if (prop == "maxByteLength" || prop == "resizable") && r.isArrayBuffer(obj) {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		method := map[string]string{"maxByteLength": "MaxByteLength", "resizable": "Resizable"}[prop]
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(method)}}, nil
	}
	// A SharedArrayBuffer's geometry getters mirror the ArrayBuffer ones on the
	// value.SharedArrayBuffer wrapper: .byteLength its size, .maxByteLength the largest
	// it may grow to, and .growable whether it may grow (the shared-buffer spelling of
	// resizable, 25 §25.2.4). They are accessors in the source but methods on the
	// wrapper, so they lower to calls and route before the struct-field path, which
	// would otherwise intern the name as a field of a shape a shared buffer is not.
	if (prop == "byteLength" || prop == "maxByteLength" || prop == "growable") && r.isSharedArrayBuffer(obj) {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		method := map[string]string{"byteLength": "ByteLength", "maxByteLength": "MaxByteLength", "growable": "Growable"}[prop]
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(method)}}, nil
	}
	// A RegExp's flag getters read off the compiled value: .source is the pattern
	// text, .flags the flag run in canonical order, and .global, .ignoreCase,
	// .multiline, .dotAll, .unicode, .unicodeSets, .sticky, and .hasIndices the
	// single-flag booleans. Each is an accessor in the source but a method on
	// value.RegExp, so it lowers to a call and routes before the struct-field path,
	// which would otherwise intern the name as a field of a shape a RegExp is not.
	if regMethod, ok := regExpAccessor(prop); ok && r.isRegExp(obj) {
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(regMethod)}}, nil
	}
	// A Temporal.PlainDate's field getters read off the ISO calendar date: .year,
	// .month, .day, and the derived .monthCode, .dayOfWeek, .dayOfYear, .daysInWeek,
	// .daysInMonth, .daysInYear, .monthsInYear, .inLeapYear, and .calendarId. Each is
	// an accessor in the source but a method on value.PlainDate, so it lowers to a
	// call and routes before the struct-field path, which would otherwise intern the
	// name as a field of a shape a PlainDate is not. The calendar-dependent getters
	// the checker types number | undefined (era, eraYear, weekOfYear, yearOfWeek) are
	// not in this map, so they hand back through plainDateAccessor rather than answer.
	if r.isPlainDate(obj) {
		pdMethod, ok := plainDateAccessor(prop)
		if !ok {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate." + prop + " is a later slice"}
		}
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(pdMethod)}}, nil
	}
	if r.isPlainTime(obj) {
		ptMethod, ok := plainTimeAccessor(prop)
		if !ok {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainTime." + prop + " is a later slice"}
		}
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(ptMethod)}}, nil
	}
	if r.isPlainDateTime(obj) {
		pdtMethod, ok := plainDateTimeAccessor(prop)
		if !ok {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime." + prop + " is a later slice"}
		}
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(pdtMethod)}}, nil
	}
	if r.isDuration(obj) {
		durMethod, ok := durationAccessor(prop)
		if !ok {
			return nil, &NotYetLowerable{Reason: "Temporal.Duration." + prop + " is a later slice"}
		}
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(durMethod)}}, nil
	}
	if r.isPlainYearMonth(obj) {
		ymMethod, ok := plainYearMonthAccessor(prop)
		if !ok {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth." + prop + " is a later slice"}
		}
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(ymMethod)}}, nil
	}
	if r.isPlainMonthDay(obj) {
		mdMethod, ok := plainMonthDayAccessor(prop)
		if !ok {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainMonthDay." + prop + " is a later slice"}
		}
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(mdMethod)}}, nil
	}
	if r.isInstant(obj) {
		instMethod, ok := instantAccessor(prop)
		if !ok {
			return nil, &NotYetLowerable{Reason: "Temporal.Instant." + prop + " is a later slice"}
		}
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(instMethod)}}, nil
	}
	if r.isZonedDateTime(obj) {
		zdtMethod, ok := zonedDateTimeAccessor(prop)
		if !ok {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime." + prop + " is a later slice"}
		}
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(zdtMethod)}}, nil
	}
	if r.isGlobalRef(obj, "Math") {
		if e, ok := mathConstant(prop); ok {
			r.requireImport(valuePkg)
			return e, nil
		}
		return nil, &NotYetLowerable{Reason: "Math." + prop + " as a value is a later slice"}
	}
	if r.isGlobalRef(obj, "Number") {
		if e, ok := numberConstant(prop); ok {
			r.requireImport(valuePkg)
			return e, nil
		}
		return nil, &NotYetLowerable{Reason: "Number." + prop + " as a value is a later slice"}
	}
	// A read of a reflective member off a plain function value, .length, .name, or the
	// .call, .apply, and .bind methods taken as a value, is not a field of a shape.
	// bento models a function as a bare Go func with no struct, so the fixed-shape path
	// below would take these for a provable miss and answer undefined, the wrong value
	// for the members the function really carries. length and name lower to their
	// compile-time constants for a named declaration; the rest hand back rather than
	// answer a wrong constant or fold to undefined.
	if e, ok, err := r.functionPropertyRead(obj, prop); ok || err != nil {
		return e, err
	}
	// A plain read o.k on a fixed-shape object lowers to the Go struct field the
	// shape's property interns to. The field name comes from the same exportedField
	// mapping and the same internStruct registration the object literal and the
	// type renderer use, so a read and the value it reads agree on the field. A
	// shape that does not lower (an optional property, say) hands back through
	// internStruct rather than reading a field that was never declared.
	objType := r.prog.TypeAt(obj)
	if objType.Flags&frontend.TypeObject != 0 {
		if _, isArray := r.prog.ElementType(objType); !isArray {
			// A fixed shape interns to a Go struct that carries exactly its declared
			// fields, so a read of a property the shape does not declare is a provable
			// runtime miss: the struct has no field that could hold it, and the language
			// answers undefined. It lowers to value.MissingProperty over the lowered
			// receiver, which evaluates the receiver for its effect and yields the
			// undefined singleton, so a read like getObj().foo still runs getObj and the
			// receiver stays referenced rather than becoming an unused Go local. The
			// checker flags this read, "Property 'X' does not exist on type 'Y'", a
			// diagnostic the front door tolerates so the read reaches here rather than
			// gating the build. It routes before the field paths below, which expect a
			// property the shape declares and would otherwise emit a selector on a Go
			// field that was never declared.
			if _, present := r.shapeProp(objType, prop); !present {
				recv, err := r.lowerExpr(obj)
				if err != nil {
					return nil, err
				}
				r.requireImport(valuePkg)
				return &ast.CallExpr{Fun: sel("value", "MissingProperty"), Args: []ast.Expr{recv}}, nil
			}
			fieldName, ok := exportedField(prop)
			if !ok {
				return nil, &NotYetLowerable{Reason: "property name ." + prop + " is not a Go identifier"}
			}
			// An optional field is a value.Opt, so a read the checker has narrowed to
			// the bare element type (inside an x !== undefined guard) unwraps with the
			// field's .Get() to match its float64, string, or struct slot, the same
			// unwrap an optional parameter's narrowed read takes. The checker proved the
			// value present at this read, so the Get is sound; an unnarrowed read keeps
			// the Opt the field holds and lowers straight to the selector below.
			if _, err := r.decls.internStruct(r, objType); err != nil {
				return nil, err
			}
			recv, err := r.lowerExpr(obj)
			if err != nil {
				return nil, err
			}
			field := &ast.SelectorExpr{X: recv, Sel: ident(fieldName)}
			if sp, ok := r.shapeProp(objType, prop); ok && sp.Optional && !r.isOptionalType(r.prog.TypeAt(n)) {
				return &ast.CallExpr{Fun: &ast.SelectorExpr{X: field, Sel: ident("Get")}}, nil
			}
			return field, nil
		}
	}
	return nil, &NotYetLowerable{Reason: "property access ." + prop + " on this type is a later slice"}
}

// functionPropertyRead lowers a read of .length or .name off a plain function value,
// and guards a read of .call, .apply, or .bind taken as a value rather than called. A
// function carries all of these as reflective members, but bento models a function as
// a bare Go func with no backing struct, so the fixed-shape path would take them for a
// provable miss and answer undefined. It fires only when the receiver's type is a bare
// function, one call signature and no construct signature or own properties, so a
// callable object, whose members are real struct fields, keeps the shape path.
//
// length and name are compile-time constants for a named function declaration: length
// is the count of parameters before the first defaulted or rest one, which is exactly
// the signature's MinArgs, and name is the declared source name. A function value that
// is not a named declaration, held in a variable or a parameter, cannot be named or
// counted at compile time and hands back rather than answer a wrong constant.
//
// call, apply, and bind read as a value, not immediately invoked, denote a bound
// method value the callable-value shape carries. bento produces no such value today,
// a bound function's own type is a rest-over-tuple that does not render, so this hands
// back rather than fold to undefined. An immediate f.call(...) never reaches here; the
// call lowering recognizes the method ahead of the member read.
//
// It reports ok=false for any other property or a non-function receiver, leaving the
// read to the general paths, where an unset expando property still folds to undefined
// through the missing-property path.
func (r *Renderer) functionPropertyRead(obj frontend.Node, prop string) (ast.Expr, bool, error) {
	if prop != "length" && prop != "name" && prop != "call" && prop != "apply" && prop != "bind" {
		return nil, false, nil
	}
	objType := r.prog.TypeAt(obj)
	call, construct := r.prog.Signatures(objType)
	if len(call) == 0 || len(construct) != 0 {
		return nil, false, nil
	}
	if len(r.prog.Properties(objType)) != 0 {
		return nil, false, nil
	}
	if prop == "call" || prop == "apply" || prop == "bind" {
		return nil, false, &NotYetLowerable{Reason: "reading ." + prop + " off a function value as a bound method value is a later slice"}
	}
	if obj.Kind() != frontend.NodeIdentifier {
		return nil, false, &NotYetLowerable{Reason: "reflective ." + prop + " off a function value that is not a named declaration is a later slice"}
	}
	sym, ok := r.prog.SymbolAt(obj)
	if !ok || sym.Flags&frontend.SymbolFunction == 0 {
		return nil, false, &NotYetLowerable{Reason: "reflective ." + prop + " off a function value that is not a named declaration is a later slice"}
	}
	if prop == "name" {
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(sym.Name)}}}, true, nil
	}
	var sig frontend.Signature
	for _, d := range r.prog.Declarations(sym) {
		if s, ok := r.prog.SignatureAt(d); ok {
			sig = s
			break
		}
	}
	return &ast.BasicLit{Kind: token.FLOAT, Value: strconv.Itoa(sig.MinArgs)}, true, nil
}

// missingPropertyRead reports whether n is a property read that propertyAccess
// lowers to value.MissingProperty: a plain read on a fixed-shape object of a
// property the shape does not declare. The checker types such a read as its error
// type (no flags), not any, so isDynamic consults this to route the read through
// the boxed-value path all the same, keeping its lowered undefined a dynamic
// value the enclosing call or coercion treats as a box rather than a static miss.
// It mirrors the condition in propertyAccess exactly, so the two never disagree
// on which reads are boxes.
func (r *Renderer) missingPropertyRead(n frontend.Node) bool {
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return false
	}
	obj := kids[0]
	// The read names a property either dotted (o.k) or bracketed with a string
	// literal (o["k"]); both lower to value.MissingProperty when the key is absent,
	// so both are recognized here. A bracket read with a computed key is not this
	// case: the shape cannot prove that key absent at compile time, so it hands back
	// rather than fold.
	var name string
	switch n.Kind() {
	case frontend.NodePropertyAccessExpression:
		name = r.prog.Text(kids[1])
	case frontend.NodeElementAccessExpression:
		key, ok := r.stringLiteralKey(kids[1])
		if !ok {
			return false
		}
		name = key
	default:
		return false
	}
	objType := r.prog.TypeAt(obj)
	if objType.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(objType); isArray {
		return false
	}
	if r.isTypedArray(obj) {
		return false
	}
	_, present := r.shapeProp(objType, name)
	return !present
}

// isQuestionDotToken reports whether a node is the ?. token that marks an
// optional chain. The token is a leaf the adapter does not give its own kind, so
// it surfaces as the unnamed fallback kind carrying the source text ?., which is
// what this checks; a plain member access a.b never exposes it.
func (r *Renderer) isQuestionDotToken(n frontend.Node) bool {
	return n.Kind() == frontend.NodeUnknown && r.prog.Text(n) == "?."
}

// optionalChainAccess lowers one link of an optional property chain, a?.b, where
// the receiver is a T | undefined optional of a lowered class instance or a
// fixed-shape object. The whole chain is nullish-poisoned: when the receiver is
// undefined the result is undefined and the member is never read, so the link
// lowers to value.OptMap over the receiver optional with the mapping function
// reading the one field. Longer chains compose because the receiver of an outer
// link is itself an optional-access node whose lowering is another Opt, so
// a?.b?.c nests one OptMap inside the next.
//
// The tractable slice is a receiver that is exactly T | undefined over a class or
// object shape whose member is a plain, non-optional field. A member that is
// itself optional (which would double-wrap under OptMap), a getter or method, an
// optional call a?.(), an optional element read a?.[i], and a receiver outside the
// class or object shapes all hand back to their own later slices.
func (r *Renderer) optionalChainAccess(recvNode, nameNode frontend.Node) (ast.Expr, error) {
	prop := r.prog.Text(nameNode)
	inner, ok := r.optionalInner(r.prog.UnionMembers(r.prog.TypeAt(recvNode)))
	if !ok {
		return nil, &NotYetLowerable{Reason: "optional chain ?." + prop + " on a receiver that is not a T | undefined optional is a later slice"}
	}
	fieldGo, memberType, ok, err := r.optionalMember(inner, prop)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, &NotYetLowerable{Reason: "optional chain member ?." + prop + " on this receiver type is a later slice"}
	}
	// A member that is itself optional would make the field read an Opt, so mapping
	// it under OptMap would nest one optional inside another. Flattening that is a
	// later slice, so it hands back and only the plain-field link lowers here.
	if _, memberOptional := r.optionalInner(r.prog.UnionMembers(memberType)); memberOptional {
		return nil, &NotYetLowerable{Reason: "optional chain onto an optional member ?." + prop + " needs the flattening OptFlatMap, a later slice"}
	}
	recvExpr, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	paramType, err := r.typeExpr(inner)
	if err != nil {
		return nil, err
	}
	retType, err := r.typeExpr(memberType)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	// value.OptMap(recv, func(v A) B { return v.Field }): the mapping function reads
	// the one field off the unwrapped receiver, and OptMap runs it only when the
	// receiver is present, propagating undefined otherwise.
	mapFn := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("v")}, Type: paramType}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: retType}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{&ast.SelectorExpr{X: ident("v"), Sel: ident(fieldGo)}}}}},
	}
	return &ast.CallExpr{Fun: sel("value", "OptMap"), Args: []ast.Expr{recvExpr, mapFn}}, nil
}

// optionalMember resolves the field read by an optional-chain link on the
// receiver's non-undefined type: it returns the Go field name and the member's
// declared type for a plain field of a lowered class instance or fixed-shape
// object, and reports not-ok for a getter, a method, an array or other receiver
// shape, or a name that is not a Go field, so those hand back rather than read a
// field that was never declared.
func (r *Renderer) optionalMember(inner frontend.Type, prop string) (string, frontend.Type, bool, error) {
	if info, ok := r.classOfType(inner); ok {
		f, isField := info.lookupField(prop)
		if !isField {
			return "", frontend.Type{}, false, nil
		}
		return f.goName, r.prog.TypeAt(f.ident), true, nil
	}
	if inner.Flags&frontend.TypeObject != 0 {
		if _, isArray := r.prog.ElementType(inner); isArray {
			return "", frontend.Type{}, false, nil
		}
		field, ok := exportedField(prop)
		if !ok {
			return "", frontend.Type{}, false, nil
		}
		for _, p := range r.prog.Properties(inner) {
			if p.Name == prop {
				if _, err := r.decls.internStruct(r, inner); err != nil {
					return "", frontend.Type{}, false, err
				}
				return field, p.Type, true, nil
			}
		}
	}
	return "", frontend.Type{}, false, nil
}

// mathConstant maps a Math namespace property name to the value-package constant
// that holds the exact double the specification names. Only the eight numeric
// constants are covered; a method name (Math.floor and the like) is a function
// value, not a number, and hands back.
func mathConstant(prop string) (ast.Expr, bool) {
	name, ok := map[string]string{
		"E":       "MathE",
		"LN10":    "MathLN10",
		"LN2":     "MathLN2",
		"LOG10E":  "MathLOG10E",
		"LOG2E":   "MathLOG2E",
		"PI":      "MathPI",
		"SQRT1_2": "MathSQRT12",
		"SQRT2":   "MathSQRT2",
	}[prop]
	if !ok {
		return nil, false
	}
	return sel("value", name), true
}

// numberConstant maps a Number namespace property name to its value-package
// counterpart. The finite constants are named constants; the three non-finite
// ones (the infinities and NaN) cannot be Go constants, so they lower to a call
// that builds the value.
func numberConstant(prop string) (ast.Expr, bool) {
	switch prop {
	case "EPSILON":
		return sel("value", "NumberEpsilon"), true
	case "MAX_SAFE_INTEGER":
		return sel("value", "NumberMaxSafeInteger"), true
	case "MIN_SAFE_INTEGER":
		return sel("value", "NumberMinSafeInteger"), true
	case "MAX_VALUE":
		return sel("value", "NumberMaxValue"), true
	case "MIN_VALUE":
		return sel("value", "NumberMinValue"), true
	case "POSITIVE_INFINITY":
		return &ast.CallExpr{Fun: sel("value", "NumberPositiveInfinity")}, true
	case "NEGATIVE_INFINITY":
		return &ast.CallExpr{Fun: sel("value", "NumberNegativeInfinity")}, true
	case "NaN":
		return &ast.CallExpr{Fun: sel("value", "NumberNaN")}, true
	}
	return nil, false
}

// elementAccess lowers an index expression a[i] to the receiver's index read: the
// array's At method, the typed array's At, or the string's CharAt code-unit read.
// arrayElem confirms an array receiver's element type lowers, and the index must
// be a Number, the JS index. An object property read spelled o["k"] lowers to the
// struct-field selector when the key is a string literal; a dynamic object key is
// its own later slice. The element type is carried by the receiver, so At needs no
// type argument here; it returns the element the checker already typed the whole
// access as.
func (r *Renderer) elementAccess(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return nil, &NotYetLowerable{Reason: "element access did not expose an object and an index"}
	}
	obj, idxNode := kids[0], kids[1]
	// arguments[i] reads the i-th argument the call supplied. The current body backs
	// arguments with a value.Array[value.Value] store, so the read is the store's At,
	// which bounds-checks and yields the boxed argument (undefined out of range, the
	// any the checker gives an arguments element). It routes before the shape and
	// string paths, which expect a receiver whose type the checker pinned down.
	if r.argsObjName != "" && r.isArgumentsIdent(obj) {
		idx, err := r.lowerExpr(idxNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(r.argsObjName), Sel: ident("At")}, Args: []ast.Expr{idx}}, nil
	}
	// C["m"] or c["m"] with a constant string key on a class receiver is the bracket
	// spelling of the dotted member read, the shape a non-identifier or computed
	// member name (a string method name, a ["m"] accessor, a [k] name whose k is a
	// const of a literal string type) is read through. It routes to the same class
	// member dispatch the dotted read uses, before the object-shape path below whose
	// internStruct would try to derive a struct for the class-constructor type and
	// hand back.
	if key, ok := r.constStringKey(idxNode); ok {
		if expr, ok, err := r.classKeyRead(obj, key); err != nil || ok {
			return expr, err
		}
	}
	// o["k"] with a compile-time-constant string key on a fixed-shape object is the
	// struct-field read o.k spelled with brackets, so it lowers to the same selector
	// through the same exportedField and internStruct the dotted read uses, and a read
	// and its value agree on the field. pureConstStringKey folds both a plain string
	// literal and a const binding the checker gave a string-literal type, so
	// `const k = "a"; o[k]` reads o.a the same way o["a"] does. It folds only a key whose
	// read runs no side effect, an identifier or a string literal, so an impure key such
	// as `o[(n++, "a")]` keeps its effect and takes the run-time path rather than fold to
	// a field. A key with no constant string type (a wide string, a let binding, a number,
	// a symbol) has no static field to select and stays a later slice. An array or typed
	// array, which is also a TypeObject, is excluded so its numeric index still routes to
	// the At read below.
	if key, ok := r.pureConstStringKey(idxNode); ok {
		objType := r.prog.TypeAt(obj)
		if objType.Flags&frontend.TypeObject != 0 && !r.isTypedArray(obj) {
			if _, isArray := r.prog.ElementType(objType); !isArray {
				// o["k"] with a key the fixed shape does not declare is the bracket
				// spelling of the absent dotted read o.k, so it folds the same way: to
				// value.MissingProperty over the lowered receiver, which evaluates the
				// receiver for its effect and yields undefined (member.go, the dotted
				// case). Guarding presence here is what keeps the selector path from
				// emitting o.K for a field the struct does not carry once the front door
				// tolerates the checker's index diagnostic for such a read.
				if _, present := r.shapeProp(objType, key); !present {
					recv, err := r.lowerExpr(obj)
					if err != nil {
						return nil, err
					}
					r.requireImport(valuePkg)
					return &ast.CallExpr{Fun: sel("value", "MissingProperty"), Args: []ast.Expr{recv}}, nil
				}
				field, ok := exportedField(key)
				if !ok {
					return nil, &NotYetLowerable{Reason: "element access key is not a Go identifier"}
				}
				if _, err := r.decls.internStruct(r, objType); err != nil {
					return nil, err
				}
				recv, err := r.lowerExpr(obj)
				if err != nil {
					return nil, err
				}
				return &ast.SelectorExpr{X: recv, Sel: ident(field)}, nil
			}
		}
	}
	// A tuple read t[i] with a literal index is the field read t.E<i>: a tuple's
	// positions are fixed and typed, so the read selects the interned struct's field
	// directly rather than going through the array At the dynamic-length receivers
	// take. It routes before the array and typed-array paths, whose arrayElem read
	// fails on a tuple (it answers TupleElements, not ElementType) and would hand the
	// read back. A non-literal or out-of-range index hands back inside.
	if elems, ok := r.prog.TupleElements(r.prog.TypeAt(obj)); ok {
		return r.tupleElementRead(obj, idxNode, elems)
	}
	// A string read s[i] is the code-unit index read: the one-code-unit string at
	// index i through BStr.CharAt, the bracket spelling of charAt. The divergence
	// from JS is the one the array read already accepts: JS answers undefined for an
	// out-of-range or fractional index where the typed read answers the zero value,
	// here the empty string, with charAt's integer coercion on the index. A
	// proven-integer loop index reads through CharAtI, the same speed-only choice
	// the array AtI makes; both forms bounds-check and read the same code unit.
	if r.isString(obj) {
		if !r.isNumber(idxNode) {
			return nil, &NotYetLowerable{Reason: "string element access with a non-number index is a later slice"}
		}
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		if r.intLoopIndex(idxNode) {
			idx, err := r.intIndexExpr(idxNode)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("CharAtI")}, Args: []ast.Expr{idx}}, nil
		}
		idx, err := r.lowerExpr(idxNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("CharAt")}, Args: []ast.Expr{idx}}, nil
	}
	// A read a[i] on a dynamic receiver dispatches at runtime: the receiver is a
	// value.Value that carries its own kind, so the read routes through GetIndex for a
	// number index and GetElem for a dynamic one, the runtime dispatch that indexes an
	// array, a string, or an object property by the same rule a static read would. A
	// string-literal key was already handled above as a property read, so what reaches
	// here is a computed index. Only a number or another dynamic value is a key this
	// slice forms; a statically typed non-number index (a bigint, a boolean) is its own
	// later slice.
	if r.isDynamic(obj) {
		r.requireImport(valuePkg)
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		switch {
		case r.isNumber(idxNode):
			idx, err := r.lowerExpr(idxNode)
			if err != nil {
				return nil, err
			}
			return r.unboxDynamicRead(&ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("GetIndex")}, Args: []ast.Expr{idx}}, n)
		case r.isDynamic(idxNode):
			idx, err := r.lowerExpr(idxNode)
			if err != nil {
				return nil, err
			}
			return r.unboxDynamicRead(&ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("GetElem")}, Args: []ast.Expr{idx}}, n)
		case r.isString(idxNode):
			idx, err := r.lowerExpr(idxNode)
			if err != nil {
				return nil, err
			}
			return r.unboxDynamicRead(&ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Get")}, Args: []ast.Expr{idx}}, n)
		case r.isSymbol(idxNode):
			// A symbol key reads through GetElem, which looks the boxed symbol up in the
			// property bag by identity rather than coercing it to a string the way a
			// number or string key resolves.
			idx, err := r.lowerExpr(idxNode)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("GetElem")}, Args: []ast.Expr{idx}}, nil
		default:
			return nil, &NotYetLowerable{Reason: "a dynamic element access with a non-number, non-string index is a later slice"}
		}
	}
	// A manual obj[Symbol.iterator] reads the iterator factory a user iterable
	// defines, the Go SymbolIterator method, so a test can drive the protocol by hand:
	// obj[Symbol.iterator]() obtains the iterator, then its .next() pulls each result.
	// It is read the same way the for...of loop obtains the iterator, and only when the
	// receiver is a user iterable this slice lowers; a built-in iterable is a later
	// slice.
	if r.isSymbolIteratorExpr(idxNode) {
		if _, ok := r.symbolIteratorShape(r.prog.TypeAt(obj)); ok {
			recv, err := r.lowerExpr(obj)
			if err != nil {
				return nil, err
			}
			return &ast.SelectorExpr{X: recv, Sel: ident(symbolIteratorGoName)}, nil
		}
		return nil, &NotYetLowerable{Reason: "a Symbol.iterator reference on a non-user-iterable receiver is a later slice"}
	}
	// A manual obj[Symbol.asyncIterator] reads the async iterator factory a user async
	// iterable defines, the Go SymbolAsyncIterator method, the async mirror of the
	// Symbol.iterator reference: obj[Symbol.asyncIterator]() obtains the async iterator,
	// then awaiting its .next() pulls each result.
	if r.isSymbolAsyncIteratorExpr(idxNode) {
		if _, ok := r.asyncIteratorShape(r.prog.TypeAt(obj)); ok {
			recv, err := r.lowerExpr(obj)
			if err != nil {
				return nil, err
			}
			return &ast.SelectorExpr{X: recv, Sel: ident(symbolAsyncIteratorGoName)}, nil
		}
		return nil, &NotYetLowerable{Reason: "a Symbol.asyncIterator reference on a non-user-async-iterable receiver is a later slice"}
	}
	// A bigint typed-array read a[i] returns its element as a *big.Int through the
	// view's At, the bigint counterpart of the numeric read. A bigint element is not a
	// Number, so it takes neither the range-proof native-slice path nor the integer-
	// index AtI the numeric family rides; the plain At handles every index, truncating
	// a Number index and reading 0n out of range, and a read flowing into a dynamic
	// slot boxes through GetIndex in boxStaticToDynamic.
	if r.bigintTypedArray(obj) {
		if !r.isNumber(idxNode) {
			return nil, &NotYetLowerable{Reason: "a bigint typed-array read with a non-number index is a later slice"}
		}
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		idx, err := r.lowerExpr(idxNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("At")}, Args: []ast.Expr{idx}}, nil
	}
	// A typed-array read a[i] returns its element as a Number through the buffer's own
	// At, the same method name a typed Array indexes through, so the receivers share
	// this shape and differ only in which value type carries At. A typed array is not
	// an array in the checker's vocabulary (it has no element type), so it is tested
	// here explicitly rather than falling through arrayElem.
	if !r.numericTypedArray(obj) {
		if _, ok := r.arrayElem(obj); !ok {
			return nil, &NotYetLowerable{Reason: "element access on a non-array receiver is a later slice"}
		}
	}
	if !r.isNumber(idxNode) {
		return nil, &NotYetLowerable{Reason: "array element access with a non-number index is a later slice"}
	}
	// A read of a fixed-length integer typed array at an index proven inside it reads
	// the backing slice directly, recv.Data()[idx] widened to a Number, dropping the
	// bounds branch and the index truncation entirely. The proof is what makes the
	// bare slice index sound: the out-of-range read-zero At gives cannot happen here.
	if info, idxNode2, ok := r.provenTypedRead(n); ok {
		return r.typedSliceRead(obj, idxNode2, "float64", info)
	}
	recv, err := r.lowerExpr(obj)
	if err != nil {
		return nil, err
	}
	// An evolving array holds value.Value elements even where the checker narrows a
	// read to number, so the read is unboxed to the narrowed type when the two
	// disagree. `var a = []; a[0] = 1` is declared any[], so a builds a
	// value.Value-element array, but control-flow analysis types a[i] number, and an
	// arithmetic use of a.At(i) needs the AsNumber the narrowed type names. An array
	// whose element type is already static reads the primitive directly and takes no
	// unbox.
	unbox := func(read ast.Expr) ast.Expr {
		acc, ok := r.dynamicArrayElemUnbox(obj, n)
		if !ok {
			return read
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: read, Sel: ident(acc)}}
	}
	// A proven-integer loop index reads through AtI, which takes the index already
	// narrowed to a Go int, so the counter stays in a register and the float
	// truncation At runs on every access is dropped. A dynamic or fractional index
	// keeps At, which truncates the Number itself. Both bounds-check and read the
	// same element, so the choice is a speed one, not a semantic one.
	if r.intLoopIndex(idxNode) {
		idx, err := r.intIndexExpr(idxNode)
		if err != nil {
			return nil, err
		}
		return unbox(&ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("AtI")}, Args: []ast.Expr{idx}}), nil
	}
	idx, err := r.lowerExpr(idxNode)
	if err != nil {
		return nil, err
	}
	return unbox(&ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("At")}, Args: []ast.Expr{idx}}), nil
}

// typedArrayBoxedRead builds the boxed form of a typed-array element read a[i] whose
// result flows into a dynamic slot: recv.GetIndex(i), which answers the element
// boxed (a Number for the numeric family, a bigint for the bigint pair) for a
// canonical in-range index and undefined for an out-of-range or non-canonical one,
// the value.Value a dynamic consumer needs where the plain At would box a stand-in 0
// or 0n. It claims a read on any indexable typed array by a Number index with a
// side-effect-free identifier receiver, so re-evaluating the receiver here is free;
// every other read returns ok=false and boxes through the primitive path.
func (r *Renderer) typedArrayBoxedRead(src frontend.Node) (ast.Expr, bool, error) {
	if src.Kind() != frontend.NodeElementAccessExpression {
		return nil, false, nil
	}
	kids := r.prog.Children(src)
	if len(kids) != 2 {
		return nil, false, nil
	}
	obj, idxNode := kids[0], kids[1]
	if obj.Kind() != frontend.NodeIdentifier {
		return nil, false, nil
	}
	if !r.numericTypedArray(obj) && !r.bigintTypedArray(obj) {
		return nil, false, nil
	}
	if !r.isNumber(idxNode) {
		return nil, false, nil
	}
	recv, err := r.lowerExpr(obj)
	if err != nil {
		return nil, false, err
	}
	idx, err := r.lowerExpr(idxNode)
	if err != nil {
		return nil, false, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("GetIndex")}, Args: []ast.Expr{idx}}, true, nil
}

// dynamicArrayElemUnbox reports the accessor that unboxes an element read obj[i]
// whose node is n, for the case where the array holds value.Value elements but
// this read is narrowed to a static primitive. A TypeScript evolving array is
// declared any[], so its backing store is value.Value, yet control-flow analysis
// narrows a read to number, string, or boolean, and At hands back the bare box the
// narrowed use cannot consume. It reports the AsNumber, AsString, or AsBool the
// narrowed type names, and ok=false when the array's element type is already static
// (At returns the primitive itself) or the read stays dynamic (the box is what the
// use wants). The array's element type is read from its symbol, not the narrowed
// read node, since only the symbol carries the any[] the store was built at.
func (r *Renderer) dynamicArrayElemUnbox(obj, n frontend.Node) (string, bool) {
	acc, ok := dynAccessor(r.primitiveFlags(n))
	if !ok {
		return "", false
	}
	sym, ok := r.prog.SymbolAt(obj)
	if !ok {
		return "", false
	}
	elem, ok := r.prog.ElementType(r.prog.TypeOfSymbol(sym))
	if !ok {
		return "", false
	}
	if elem.Flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 {
		return "", false
	}
	return acc, true
}
