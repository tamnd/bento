package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the composite literals and their methods: array and object
// literals, the array method family (map, filter, indexOf, includes, join), and
// Map methods, with the closures they synthesize.

// arrayElem reports whether the checker types n as an array, returning the
// lowered Go element type when so. TypeObject covers both arrays and fixed-shape
// objects in the frontend vocabulary, so an element type is what distinguishes
// the two, the same test typeExpr uses to route an array type to renderArray. A
// hand-back on the element type (an element that does not lower yet) reads here
// as "not a lowerable array", so the caller hands the whole construct back.
func (r *Renderer) arrayElem(n frontend.Node) (ast.Expr, bool) {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return nil, false
	}
	elem, ok := r.prog.ElementType(t)
	if !ok {
		return nil, false
	}
	e, err := r.typeExpr(elem)
	if err != nil {
		return nil, false
	}
	return e, true
}

// arrayLiteral lowers an array literal [e0, e1, ...] to a value.NewArray call
// instantiated at the element type. The element type is taken from the checker's
// type for the whole literal, not guessed from the elements, so a widened or
// empty literal is spelled the way the checker sees it and NewArray's type
// argument is explicit rather than inferred from a possibly empty argument list.
// A literal that splices in a spread element takes the arraySpread path instead;
// an elided hole still hands back to a later slice.
func (r *Renderer) arrayLiteral(n frontend.Node) (ast.Expr, error) {
	elemType, ok := r.arrayElem(n)
	if !ok {
		return nil, &NotYetLowerable{Reason: "array literal whose element type does not lower yet"}
	}
	kids := r.prog.Children(n)
	for _, k := range kids {
		if k.Kind() == frontend.NodeSpreadElement {
			return r.arraySpread(n, elemType, kids)
		}
	}
	args := make([]ast.Expr, 0, len(kids))
	for _, k := range kids {
		e, err := r.lowerExpr(k)
		if err != nil {
			return nil, err
		}
		args = append(args, e)
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: index(sel("value", "NewArray"), elemType), Args: args}, nil
}

// arraySpread lowers an array literal that splices in one or more spread
// elements, [a, ...b, c], to a []T built with append and wrapped once by
// value.ArrayFrom. The backing slice starts as a fresh []T literal, so the
// result aliases none of the spread sources: a run of plain elements folds into
// one append with those elements as arguments (or into the seed literal when it
// leads), and each spread appends its source's backing slice with append's
// variadic form. A person splicing arrays in Go writes this same append chain,
// and ArrayFrom then takes the finished slice without a second copy.
//
// Only a spread of another array whose element Go type matches the literal's is
// covered, since that is what append's variadic form takes; a spread of a string
// or another iterable, or of an array with a different element type, hands back.
func (r *Renderer) arraySpread(n frontend.Node, elemType ast.Expr, kids []frontend.Node) (ast.Expr, error) {
	seedType := &ast.ArrayType{Elt: elemType}
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
	for _, k := range kids {
		if k.Kind() != frontend.NodeSpreadElement {
			e, err := r.lowerExpr(k)
			if err != nil {
				return nil, err
			}
			pending = append(pending, e)
			continue
		}
		operands := r.prog.Children(k)
		if len(operands) != 1 {
			return nil, &NotYetLowerable{Reason: "spread element with an unexpected shape is a later slice"}
		}
		operand := operands[0]
		opElemType, ok := r.arrayElem(operand)
		if !ok {
			return nil, &NotYetLowerable{Reason: "spread of a non-array value in an array literal is a later slice"}
		}
		same, err := sameGoType(elemType, opElemType)
		if err != nil {
			return nil, err
		}
		if !same {
			return nil, &NotYetLowerable{Reason: "spread of an array with a different element type is a later slice"}
		}
		spreadVal, err := r.lowerExpr(operand)
		if err != nil {
			return nil, err
		}
		flush()
		if acc == nil {
			acc = &ast.CompositeLit{Type: seedType}
		}
		elems := &ast.CallExpr{Fun: &ast.SelectorExpr{X: spreadVal, Sel: ident("Elems")}}
		acc = &ast.CallExpr{Fun: ident("append"), Args: []ast.Expr{acc, elems}, Ellipsis: token.Pos(1)}
	}
	flush()
	if acc == nil {
		acc = &ast.CompositeLit{Type: seedType}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "ArrayFrom"), Args: []ast.Expr{acc}}, nil
}

// objectLiteral lowers an object literal { k: v, ... } to a composite literal
// that builds a pointer to the generated struct the object's shape interns to.
// The struct name comes from the same internStruct path a variable annotated
// with this shape takes, so a literal and a binding of the same shape produce
// the same Go type and structural assignability becomes Go assignability
// (05_type_lowering section 12). Each property lowers to a keyed field, so the
// literal's declaration order need not match the struct's field order. Only the
// plain identifier-keyed forms are covered: a computed or string key belongs in
// the object side table, a spread copies another object's own fields, and a
// method or accessor is a function member, each its own later slice, so any of
// them hands back rather than emit a wrong or partial struct.
func (r *Renderer) objectLiteral(n frontend.Node) (ast.Expr, error) {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "object literal whose type is not an object shape is a later slice"}
	}
	if _, ok := r.prog.ElementType(t); ok {
		return nil, &NotYetLowerable{Reason: "object literal typed as an array is a later slice"}
	}
	// internStruct registers the struct and reports the name; a shape that does
	// not lower (an optional property, a non-identifier field) hands back here, so
	// the literal never builds a struct the type side would refuse to declare.
	name, err := r.decls.internStruct(r, t)
	if err != nil {
		return nil, err
	}
	props := r.prog.Children(n)
	elts := make([]ast.Expr, 0, len(props))
	for _, p := range props {
		if p.Kind() != frontend.NodeUnknown {
			// A method, getter, or setter member is a function property, which the
			// frontend names its own kind rather than a property assignment.
			return nil, &NotYetLowerable{Reason: "object literal with a method or accessor member is a later slice"}
		}
		kids := r.prog.Children(p)
		var keyNode, valNode frontend.Node
		switch len(kids) {
		case 1:
			// A shorthand { x } is { x: x }: the single child is both the key and the
			// value reference. A spread { ...o } is also a single-child member, but
			// its text opens with the spread token, so it routes to the handback.
			if strings.HasPrefix(strings.TrimSpace(r.prog.Text(p)), "...") {
				return nil, &NotYetLowerable{Reason: "object spread in a literal is a later slice"}
			}
			keyNode, valNode = kids[0], kids[0]
		case 2:
			keyNode, valNode = kids[0], kids[1]
		default:
			return nil, &NotYetLowerable{Reason: "object literal member with an unexpected shape is a later slice"}
		}
		if keyNode.Kind() != frontend.NodeIdentifier {
			// A computed [k] key or a string/numeric key does not become a struct
			// field; it belongs in the object side table, a later slice.
			return nil, &NotYetLowerable{Reason: "object literal with a non-identifier key is a later slice"}
		}
		field, ok := exportedField(r.prog.Text(keyNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "object literal property name is not a Go identifier"}
		}
		val, err := r.lowerExpr(valNode)
		if err != nil {
			return nil, err
		}
		elts = append(elts, &ast.KeyValueExpr{Key: ident(field), Value: val})
	}
	return &ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(name), Elts: elts}}, nil
}

// arrayMethodCall lowers a method call on an array receiver to a value.Array
// method. Only push is covered so far: it appends its arguments and returns the
// new length. The checker has already verified each argument against the element
// type, so the arguments lower directly with no per-argument kind guard the way
// the string methods need, since here the element type, not a fixed argument
// kind, is what the checker enforced. The reading, higher-order, and other
// pop is a later slice, waiting on the optional machinery for its undefined
// result. The higher-order map and filter are covered here, over a concise-body
// arrow callback that takes the element; slice, which returns a fresh array over
// a copied range; the search methods indexOf and includes, over a synthesized
// element-equality closure; and join, over a synthesized per-element ToString
// closure.
func (r *Renderer) arrayMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "push":
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		args := make([]ast.Expr, 0, len(argNodes))
		for _, a := range argNodes {
			lowered, err := r.lowerExpr(a)
			if err != nil {
				return nil, err
			}
			args = append(args, lowered)
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Push")}, Args: args}, nil
	case "map":
		return r.arrayMapFilter(recvNode, "Map", argNodes, true)
	case "filter":
		return r.arrayMapFilter(recvNode, "Filter", argNodes, false)
	case "reduce":
		return r.arrayReduce(recvNode, argNodes)
	case "indexOf":
		return r.arrayIndexOfIncludes(recvNode, "IndexOf", argNodes, false)
	case "lastIndexOf":
		return r.arrayIndexOfIncludes(recvNode, "LastIndexOf", argNodes, false)
	case "includes":
		return r.arrayIndexOfIncludes(recvNode, "Includes", argNodes, true)
	case "join":
		return r.arrayJoin(recvNode, argNodes)
	case "some":
		return r.arrayCallbackMethod(recvNode, "Some", argNodes)
	case "every":
		return r.arrayCallbackMethod(recvNode, "Every", argNodes)
	case "forEach":
		return r.arrayCallbackMethod(recvNode, "ForEach", argNodes)
	case "find":
		return r.arrayCallbackMethod(recvNode, "Find", argNodes)
	case "findIndex":
		return r.arrayCallbackMethod(recvNode, "FindIndex", argNodes)
	case "reverse":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "array reverse takes no arguments"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Reverse")}}, nil
	case "sort":
		return r.arraySort(recvNode, argNodes)
	case "pop":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "array pop takes no arguments"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Pop")}}, nil
	case "slice":
		if len(argNodes) > 2 {
			return nil, &NotYetLowerable{Reason: "array slice with more than two arguments is not valid"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		args := make([]ast.Expr, 0, len(argNodes))
		for _, a := range argNodes {
			if !r.isNumber(a) {
				return nil, &NotYetLowerable{Reason: "array slice with a non-number bound is a later slice"}
			}
			lowered, err := r.lowerExpr(a)
			if err != nil {
				return nil, err
			}
			args = append(args, lowered)
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Slice")}, Args: args}, nil
	case "at":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "array at takes exactly one argument"}
		}
		if !r.isNumber(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "array at with a non-number index is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("AtOpt")}, Args: []ast.Expr{arg}}, nil
	case "fill":
		if len(argNodes) < 1 || len(argNodes) > 3 {
			return nil, &NotYetLowerable{Reason: "array fill takes a value and up to two bounds"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		// The first argument is the fill value, which the checker has already typed
		// against the element type, so it lowers straight through. The remaining
		// arguments are the optional start and end bounds, each a Number.
		val, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		args := []ast.Expr{val}
		for _, b := range argNodes[1:] {
			if !r.isNumber(b) {
				return nil, &NotYetLowerable{Reason: "array fill with a non-number bound is a later slice"}
			}
			lowered, err := r.lowerExpr(b)
			if err != nil {
				return nil, err
			}
			args = append(args, lowered)
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Fill")}, Args: args}, nil
	default:
		return nil, &NotYetLowerable{Reason: "array method ." + method + " is a later slice"}
	}
}

// mapMethodCall lowers a method call on a Map receiver to the matching value.Map
// method (section 6.5). Each method maps to its Go name with an exact argument
// count: get(k) reads an entry as an Opt the same narrowing and nullish paths any
// optional takes, set(k, v) writes and returns the map, has(k) and delete(k) report
// membership, and clear() empties it. The checker has already typed each argument
// against the map's own K and V, so the arguments lower straight through with no
// extra kind guard; a method or an argument count outside this set hands back.
func (r *Renderer) mapMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	var goName string
	var want int
	switch method {
	case "get":
		goName, want = "Get", 1
	case "set":
		goName, want = "Set", 2
	case "has":
		goName, want = "Has", 1
	case "delete":
		goName, want = "Delete", 1
	case "clear":
		goName, want = "Clear", 0
	default:
		return nil, &NotYetLowerable{Reason: "map method ." + method + " is a later slice"}
	}
	if len(argNodes) != want {
		return nil, &NotYetLowerable{Reason: "map method ." + method + " with this argument count is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	args := make([]ast.Expr, 0, want)
	for _, a := range argNodes {
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}, Args: args}, nil
}

// arrayMapFilter lowers a map or filter call to the matching value.Array method
// over a lowered callback. Only a single arrow-function argument is covered, the
// shape these almost always take; a callback passed as a named reference is a
// later slice, since it needs the reference resolved to a function value first.
// map handles both the same-type and the type-changing form. A Go method cannot
// introduce a new type parameter, so the value.Array.Map method can only return
// the element type; when the callback's result type matches the element the map
// lowers to that method, and when it differs (number[].map(n => n.toString()) is
// string[]) it lowers to the free function value.MapArray[T, U] with both type
// arguments spelled out. The result type is read straight off the arrow's body,
// which the checker has already inferred, compared against the array's element
// type. filter has no such split because its callback is always element to
// boolean, so it always uses the method.
func (r *Renderer) arrayMapFilter(recvNode frontend.Node, goMethod string, argNodes []frontend.Node, restrictToElem bool) (ast.Expr, error) {
	if len(argNodes) != 1 || argNodes[0].Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "array ." + goMethod + " with a callback that is not an inline arrow function is a later slice"}
	}
	if restrictToElem {
		elemType, ok := r.arrayElem(recvNode)
		if !ok {
			return nil, &NotYetLowerable{Reason: "array map on a receiver whose element type did not lower"}
		}
		arrow := argNodes[0]
		bodyType, err := r.arrowResultType(arrow)
		if err != nil {
			return nil, err
		}
		same, err := sameGoType(elemType, bodyType)
		if err != nil {
			return nil, err
		}
		if !same {
			// A type-changing map cannot use the method, so it lowers to the free
			// function value.MapArray[T, U](recv, fn), the one place the element and
			// result Go types are both named in the emitted call.
			recv, err := r.lowerExpr(recvNode)
			if err != nil {
				return nil, err
			}
			fn, err := r.lowerExpr(arrow)
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{
				Fun:  &ast.IndexListExpr{X: sel("value", "MapArray"), Indices: []ast.Expr{elemType, bodyType}},
				Args: []ast.Expr{recv, fn},
			}, nil
		}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	fn, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goMethod)}, Args: []ast.Expr{fn}}, nil
}

// arrayCallbackMethod lowers a predicate-or-effect array method whose only
// argument is a single-element callback, some, every, and forEach, to the
// matching value.Array method over a lowered arrow. some and every take a
// func(T) bool and short-circuit, forEach takes a func(T) and runs for effect,
// so each is the receiver method applied to the lowered callback with no result
// juggling map's type-changing form needs. Only an inline arrow taking exactly
// the element is covered: a callback passed as a named reference is a later
// slice, since it needs the reference resolved to a func value first, and a
// callback that also reads the index or array parameter needs those threaded
// through, a later slice too, so a two-parameter arrow hands back rather than
// emit a call the value method's one-parameter func could not take.
func (r *Renderer) arrayCallbackMethod(recvNode frontend.Node, goMethod string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 || argNodes[0].Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "array ." + goMethod + " with a callback that is not an inline arrow function is a later slice"}
	}
	if r.arrowParamCount(argNodes[0]) != 1 {
		return nil, &NotYetLowerable{Reason: "array ." + goMethod + " with a callback that reads the index or array parameter is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	fn, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goMethod)}, Args: []ast.Expr{fn}}, nil
}

// arrowParamCount reports how many parameters an arrow declares, the count the
// higher-order methods check to keep a callback to the single-element shape the
// value methods take. It counts the parameter children rather than reading the
// signature, so it needs no checker query.
func (r *Renderer) arrowParamCount(arrow frontend.Node) int {
	n := 0
	for _, k := range r.prog.Children(arrow) {
		if k.Kind() == frontend.NodeParameter {
			n++
		}
	}
	return n
}

// arrayReduce lowers a reduce call with an initial value to the free function
// value.Reduce[T, A](recv, fn, init). It is the free function rather than a
// method for the same reason the type-changing map is: the accumulator type A
// may differ from the element type T, and a Go method cannot introduce the new
// type parameter A. The element type comes from the receiver and the
// accumulator type from the callback's result, the two the value function names
// as its type arguments, so numbers.reduce((acc, n) => acc + n, 0) spells
// value.Reduce[float64, float64] and a string accumulator spells its own A.
//
// Only the two-argument form over an inline two-parameter arrow is covered.
// reduce without an initial value seeds the accumulator with the first element
// and throws on an empty array, a different shape that is its own later slice,
// so a one-argument call hands back. A callback that also reads the index or
// array parameter needs those threaded through, so an arrow that is not exactly
// (accumulator, element) hands back too, since the value function takes a
// two-parameter func.
func (r *Renderer) arrayReduce(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "array reduce without an initial value is a later slice"}
	}
	arrow := argNodes[0]
	if arrow.Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "array reduce with a callback that is not an inline arrow function is a later slice"}
	}
	if r.arrowParamCount(arrow) != 2 {
		return nil, &NotYetLowerable{Reason: "array reduce with a callback that reads the index or array parameter is a later slice"}
	}
	elemType, ok := r.arrayElem(recvNode)
	if !ok {
		return nil, &NotYetLowerable{Reason: "array reduce on a receiver whose element type did not lower"}
	}
	accType, err := r.arrowResultType(arrow)
	if err != nil {
		return nil, err
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	fn, err := r.lowerExpr(arrow)
	if err != nil {
		return nil, err
	}
	init, err := r.lowerExpr(argNodes[1])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun:  &ast.IndexListExpr{X: sel("value", "Reduce"), Indices: []ast.Expr{elemType, accType}},
		Args: []ast.Expr{recv, fn, init},
	}, nil
}

// arrayIndexOfIncludes lowers an indexOf or includes call to the matching
// value.Array method, passing the target and a synthesized equality closure. The
// closure is what lets the value method stay type-agnostic: it cannot compare
// two values of its type parameter, so the lowerer, which knows the element
// type, builds the comparison. The equality differs by method and element type.
// For a number element, indexOf uses strict equality, which is Go ==, so a NaN
// target is never found, while includes uses SameValueZero, so its closure also
// treats two NaNs as equal. For a string element the comparison is
// value.BStr.Equal either way, since strict equality and SameValueZero agree on
// strings, and for a boolean it is Go ==. An element type outside those, whose
// equality would be reference identity or a deep compare, hands back. The
// optional fromIndex argument is a later slice, so more than one argument hands
// back.
func (r *Renderer) arrayIndexOfIncludes(recvNode frontend.Node, goMethod string, argNodes []frontend.Node, sameValueZero bool) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "array ." + goMethod + " with a fromIndex argument is a later slice"}
	}
	elemGo, ok := r.arrayElem(recvNode)
	if !ok {
		return nil, &NotYetLowerable{Reason: "array ." + goMethod + " on a receiver whose element type did not lower"}
	}
	elem, ok := r.prog.ElementType(r.prog.TypeAt(recvNode))
	if !ok {
		return nil, &NotYetLowerable{Reason: "array ." + goMethod + " could not read its element type"}
	}
	eq, err := r.equalityClosure(elem, elemGo, sameValueZero)
	if err != nil {
		return nil, err
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	target, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goMethod)}, Args: []ast.Expr{target, eq}}, nil
}

// equalityClosure builds the func(T, T) bool the array search methods take,
// spelling out the JavaScript equality for the element type. The parameters are
// named a and b and typed at the element's Go type. A number compares with ==,
// and under SameValueZero also matches two NaNs (a != a && b != b), the one place
// includes and indexOf diverge. A string compares with value.BStr.Equal, a
// boolean with ==. Any other element type hands back, since its equality is not
// one of these value comparisons.
func (r *Renderer) equalityClosure(elem frontend.Type, elemGo ast.Expr, sameValueZero bool) (ast.Expr, error) {
	var body ast.Expr
	switch {
	case elem.Flags&frontend.TypeNumber != 0:
		body = &ast.BinaryExpr{X: ident("a"), Op: token.EQL, Y: ident("b")}
		if sameValueZero {
			// a == b || a != a && b != b, so NaN matches NaN under SameValueZero.
			nanA := &ast.BinaryExpr{X: ident("a"), Op: token.NEQ, Y: ident("a")}
			nanB := &ast.BinaryExpr{X: ident("b"), Op: token.NEQ, Y: ident("b")}
			body = &ast.BinaryExpr{X: body, Op: token.LOR, Y: &ast.BinaryExpr{X: nanA, Op: token.LAND, Y: nanB}}
		}
	case elem.Flags&frontend.TypeString != 0:
		body = &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident("a"), Sel: ident("Equal")}, Args: []ast.Expr{ident("b")}}
	case elem.Flags&frontend.TypeBoolean != 0:
		body = &ast.BinaryExpr{X: ident("a"), Op: token.EQL, Y: ident("b")}
	default:
		return nil, &NotYetLowerable{Reason: "array search on an element type without a value equality is a later slice"}
	}
	params := &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("a"), ident("b")}, Type: elemGo}}}
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: &ast.FieldList{List: []*ast.Field{{Type: ident("bool")}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{body}}}},
	}, nil
}

// arrayJoin lowers a join call to the value.Array Join method, passing the
// separator and a synthesized per-element ToString closure. The separator is the
// lowered string argument, or the JavaScript default comma when the call has
// none; an argument that is not a string, or more than one, hands back, since
// only the string-separator form is covered. The stringify closure is built the
// same way the search-method equality is, off the element type, because the
// value method cannot run the element-type-specific ToString on its type
// parameter.
func (r *Renderer) arrayJoin(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) > 1 {
		return nil, &NotYetLowerable{Reason: "array join with more than one argument is not valid"}
	}
	elemGo, ok := r.arrayElem(recvNode)
	if !ok {
		return nil, &NotYetLowerable{Reason: "array join on a receiver whose element type did not lower"}
	}
	elem, ok := r.prog.ElementType(r.prog.TypeAt(recvNode))
	if !ok {
		return nil, &NotYetLowerable{Reason: "array join could not read its element type"}
	}
	str, err := r.stringifyClosure(elem, elemGo)
	if err != nil {
		return nil, err
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	var sep ast.Expr
	if len(argNodes) == 1 {
		if !r.isString(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "array join with a non-string separator is a later slice"}
		}
		sep, err = r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
	} else {
		r.requireImport(valuePkg)
		sep = &ast.CallExpr{Fun: sel("value", "FromGoString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `","`}}}
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Join")}, Args: []ast.Expr{sep, str}}, nil
}

// arraySort lowers a sort call to the value.Array Sort method over a lowered
// comparator. Only the comparator form is covered, and only when the comparator
// is a single inline two-parameter arrow, the shape a sort almost always takes:
// sort in place ordering by a compare function. sort() with no comparator is the
// default lexicographic order, which coerces every element to a string and
// compares by UTF-16 code unit ([1, 10, 2] sorts as "1", "10", "2"), a different
// element-to-string path that is its own later slice, so a zero-argument call
// hands back. A comparator that is not an inline arrow, or an arrow whose
// parameter count is not two, also hands back: the value method's comparator is
// func(T, T) float64, so a callback that reads a different arity would not fit
// its signature. The two-parameter count is checked inline here rather than
// through a shared helper so this slice stays independent of the predicate
// family.
func (r *Renderer) arraySort(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) == 0 {
		return nil, &NotYetLowerable{Reason: "array sort without a comparator needs the default string-order sort, a later slice"}
	}
	if len(argNodes) != 1 || argNodes[0].Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "array sort with a comparator that is not an inline arrow function is a later slice"}
	}
	arrow := argNodes[0]
	params := 0
	for _, k := range r.prog.Children(arrow) {
		if k.Kind() == frontend.NodeParameter {
			params++
		}
	}
	if params != 2 {
		return nil, &NotYetLowerable{Reason: "array sort comparator that does not take exactly two parameters is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	cmp, err := r.lowerExpr(arrow)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Sort")}, Args: []ast.Expr{cmp}}, nil
}

// stringifyClosure builds the func(T) value.BStr the join method takes, spelling
// out the element-type ToString. It mirrors stringify but over a synthesized
// parameter rather than a node: a number goes through value.NumberToString, a
// boolean through value.BoolToString, and a string is returned as is. Any other
// element type, whose ToString would run user code, hands back.
func (r *Renderer) stringifyClosure(elem frontend.Type, elemGo ast.Expr) (ast.Expr, error) {
	var body ast.Expr
	switch {
	case elem.Flags&frontend.TypeString != 0:
		body = ident("x")
	case elem.Flags&frontend.TypeNumber != 0:
		r.requireImport(valuePkg)
		body = &ast.CallExpr{Fun: sel("value", "NumberToString"), Args: []ast.Expr{ident("x")}}
	case elem.Flags&frontend.TypeBoolean != 0:
		r.requireImport(valuePkg)
		body = &ast.CallExpr{Fun: sel("value", "BoolToString"), Args: []ast.Expr{ident("x")}}
	default:
		return nil, &NotYetLowerable{Reason: "array join on an element type without a value ToString is a later slice"}
	}
	params := &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("x")}, Type: elemGo}}}
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "BStr")}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{body}}}},
	}, nil
}
