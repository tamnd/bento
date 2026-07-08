package lower

import (
	"go/ast"
	"go/token"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers member access: property reads, element access, and the Math
// and Number constants a property read can name.

// propertyAccess lowers a member expression. Two members are covered: .length on
// a string, which is the code-unit count and lowers to the value.BStr Length
// method, a float64 that matches the number type the checker gives .length; and a
// numeric constant on the Math or Number namespace (Math.PI, Number.EPSILON, and
// their siblings), which is a property read on a global rather than a method call,
// so it lowers to the matching value-package constant. Every other property (a
// field of a lowered object, a method call, .length on an array) is its own later
// slice and hands back.
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
		key := &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(prop)}}}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Get")}, Args: []ast.Expr{key}}, nil
	}
	// A static read A.total lowers to the package var the static field became.
	// The receiver here is the class name itself, whose type shares the class
	// symbol an instance type walks to, so this routes before the instance path
	// below or the read would resolve against the instance fields.
	if obj.Kind() == frontend.NodeIdentifier {
		if info, ok := r.classNameRef(obj); ok {
			if f, ok := info.staticByName(prop); ok {
				return ident(f.goName), nil
			}
			if _, isMethod := info.staticMethodByName(prop); isMethod {
				return nil, &NotYetLowerable{Reason: "a static method of class " + info.name + " read as a value is a later slice"}
			}
			return nil, &NotYetLowerable{Reason: "class " + info.name + " has no static ." + prop + " this slice lowers"}
		}
	}
	// A field read on a class instance, this.x inside a class body or p.x on an
	// instance, lowers to the Go struct field the class declared, and a getter
	// read to the method call the accessor became. It routes before the length,
	// size, and interned-shape paths so a class whose fields happen to spell one
	// of those fingerprints is still read as the class it is. A method read
	// without a call is a bound-function value, a later slice.
	if info, ok := r.classReceiver(obj); ok {
		f, isField := info.lookupField(prop)
		g, isGetter := info.lookupGetter(prop)
		if !isField && !isGetter {
			if _, isMethod := info.lookupMethod(prop); isMethod {
				return nil, &NotYetLowerable{Reason: "a method of class " + info.name + " read as a value is a later slice"}
			}
			if _, isSetter := info.lookupSetter(prop); isSetter {
				return nil, &NotYetLowerable{Reason: "reading the write-only accessor ." + prop + " of class " + info.name + " is a later slice"}
			}
			return nil, &NotYetLowerable{Reason: "class " + info.name + " has no property ." + prop + " this slice lowers"}
		}
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
		}
		if isGetter {
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(g.goName)}}, nil
		}
		return &ast.SelectorExpr{X: recv, Sel: ident(f.goName)}, nil
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
		if isArray || r.numericTypedArray(obj) {
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
			field, ok := exportedField(prop)
			if !ok {
				return nil, &NotYetLowerable{Reason: "property name ." + prop + " is not a Go identifier"}
			}
			// An optional field is a value.Opt, so a read the checker has narrowed to
			// the bare element type (inside an x !== undefined guard) would need the
			// Get unwrap to match its float64 or string slot. That narrowed read is a
			// later slice, so it hands back rather than emit an Opt where T is wanted;
			// an unnarrowed read stays the Opt the field holds and lowers straight to
			// the selector below.
			if sp, ok := r.shapeProp(objType, prop); ok && sp.Optional && !r.isOptionalType(r.prog.TypeAt(n)) {
				return nil, &NotYetLowerable{Reason: "a narrowed read of the optional property ." + prop + " needs the Get unwrap, a later slice"}
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
	return nil, &NotYetLowerable{Reason: "property access ." + prop + " on this type is a later slice"}
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
	if n.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return false
	}
	obj, nameNode := kids[0], kids[1]
	objType := r.prog.TypeAt(obj)
	if objType.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(objType); isArray {
		return false
	}
	_, present := r.shapeProp(objType, r.prog.Text(nameNode))
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
	// o["k"] with a string-literal key on a fixed-shape object is the struct-field
	// read o.k spelled with brackets, so it lowers to the same selector through the
	// same exportedField and internStruct the dotted read uses, and a read and its
	// value agree on the field. Only a string-literal key takes this path: a dynamic
	// key has no static field to select and is its own later slice. An array or typed
	// array, which is also a TypeObject, is excluded so its numeric index still
	// routes to the At read below.
	if key, ok := r.stringLiteralKey(idxNode); ok {
		objType := r.prog.TypeAt(obj)
		if objType.Flags&frontend.TypeObject != 0 && !r.isTypedArray(obj) {
			if _, isArray := r.prog.ElementType(objType); !isArray {
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
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("GetIndex")}, Args: []ast.Expr{idx}}, nil
		case r.isDynamic(idxNode):
			idx, err := r.lowerExpr(idxNode)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("GetElem")}, Args: []ast.Expr{idx}}, nil
		case r.isString(idxNode):
			idx, err := r.lowerExpr(idxNode)
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Get")}, Args: []ast.Expr{idx}}, nil
		default:
			return nil, &NotYetLowerable{Reason: "a dynamic element access with a non-number, non-string index is a later slice"}
		}
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
