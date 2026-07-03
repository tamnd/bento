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
			default:
				return nil, &NotYetLowerable{Reason: "a caught error's ." + prop + " is a later slice"}
			}
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
		key := &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(prop)}}}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Get")}, Args: []ast.Expr{key}}, nil
	}
	// A field read on a class instance, this.x inside a class body or p.x on an
	// instance, lowers to the Go struct field the class declared. It routes before
	// the length, size, and interned-shape paths so a class whose fields happen to
	// spell one of those fingerprints is still read as the class it is. A method
	// read without a call is a bound-function value, a later slice.
	if info, ok := r.classReceiver(obj); ok {
		f, ok := info.fieldByName(prop)
		if !ok {
			if _, isMethod := info.methodByName(prop); isMethod {
				return nil, &NotYetLowerable{Reason: "a method of class " + info.name + " read as a value is a later slice"}
			}
			return nil, &NotYetLowerable{Reason: "class " + info.name + " has no property ." + prop + " this slice lowers"}
		}
		recv, err := r.lowerExpr(obj)
		if err != nil {
			return nil, err
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
		if isArray || r.isBytes(obj) {
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
			field, ok := exportedField(prop)
			if !ok {
				return nil, &NotYetLowerable{Reason: "property name ." + prop + " is not a Go identifier"}
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

// elementAccess lowers an index expression a[i] to the array's At method. Only an
// array receiver is covered: arrayElem confirms the checker types the receiver as
// an array whose element type lowers, and the index must be a Number, the JS array
// index. An object property read spelled o["k"] and a string character read s[i]
// have different runtime meanings and hand back to their own later slices. The
// element type is carried by the receiver, so At needs no type argument here; it
// returns the element the checker already typed the whole access as.
func (r *Renderer) elementAccess(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return nil, &NotYetLowerable{Reason: "element access did not expose an object and an index"}
	}
	obj, idxNode := kids[0], kids[1]
	// A Uint8Array read a[i] returns a byte as a Number through the buffer's own At,
	// the same method name a typed Array indexes through, so the two receivers share
	// this shape and differ only in which value type carries At. A byte buffer is not
	// an array in the checker's vocabulary (it has no element type), so it is tested
	// here explicitly rather than falling through arrayElem.
	if !r.isBytes(obj) {
		if _, ok := r.arrayElem(obj); !ok {
			return nil, &NotYetLowerable{Reason: "element access on a non-array receiver is a later slice"}
		}
	}
	if !r.isNumber(idxNode) {
		return nil, &NotYetLowerable{Reason: "array element access with a non-number index is a later slice"}
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
