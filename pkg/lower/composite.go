package lower

import (
	"bytes"
	"go/ast"
	"go/printer"
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
	args, err := r.wrapArrayElems(args, kids, n)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: index(sel("value", "NewArray"), elemType), Args: args}, nil
}

// wrapArrayElems wraps each lowered element of an array construction into the arm
// constructor of a tagged-sum union element type, the construction a NewArray over
// a union needs. An array whose element type is a union like string | number stores
// union values, not the bare number or string an element lowers to, so each element
// is wrapped in the arm constructor its own type selects (NumOrStrOfNum, NumOrStrOfStr)
// the same way an assignment into a union slot is. When the element type is not a
// tagged-sum union every element passes through unchanged, so a homogeneous array is
// emitted exactly as before. elems are the element nodes in index order, the source
// types wrapToUnion reads to pick each arm; container is the whole literal or call,
// whose checker type carries the element type.
func (r *Renderer) wrapArrayElems(args []ast.Expr, elems []frontend.Node, container frontend.Node) ([]ast.Expr, error) {
	elemT, ok := r.prog.ElementType(r.prog.TypeAt(container))
	if !ok {
		return args, nil
	}
	for i, e := range args {
		w, err := r.wrapUnionElem(e, elems[i], elemT)
		if err != nil {
			return nil, err
		}
		args[i] = w
	}
	return args, nil
}

// wrapUnionElem wraps one lowered element into the arm constructor its source type
// selects when elemT is a tagged-sum union, and returns the element unchanged when
// it is not. It is the per-element step wrapArrayElems and the spread path share so a
// plain and a spread construction wrap an element the same way.
func (r *Renderer) wrapUnionElem(e ast.Expr, elem frontend.Node, elemT frontend.Type) (ast.Expr, error) {
	wrapped, ok, err := r.wrapToUnion(e, elem, elemT)
	if err != nil {
		return nil, err
	}
	if ok {
		return wrapped, nil
	}
	return e, nil
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
	elemT, hasElemT := r.prog.ElementType(r.prog.TypeAt(n))
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
			if hasElemT {
				if e, err = r.wrapUnionElem(e, k, elemT); err != nil {
					return nil, err
				}
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
			// A spread of a user iterable that is not an array walks the iterator
			// protocol: it is drained into a slice of its element type, then that slice
			// is spliced the same way an array's Elems is. The element types must lower
			// to the same Go type, so the drained values splice without a conversion.
			if shape, ok := r.symbolIteratorShape(r.prog.TypeAt(operand)); ok {
				iterElemType, err := r.typeExpr(shape.elem)
				if err != nil {
					return nil, err
				}
				same, err := sameGoType(elemType, iterElemType)
				if err != nil {
					return nil, err
				}
				if !same {
					return nil, &NotYetLowerable{Reason: "spread of an iterable with a different element type is a later slice"}
				}
				src, err := r.lowerExpr(operand)
				if err != nil {
					return nil, err
				}
				flush()
				if acc == nil {
					acc = &ast.CompositeLit{Type: seedType}
				}
				drained := r.iterableToSliceExpr(src, elemType, shape)
				acc = &ast.CallExpr{Fun: ident("append"), Args: []ast.Expr{acc, drained}, Ellipsis: token.Pos(1)}
				continue
			}
			// A spread of a generator drains its coroutine into a slice of its yielded
			// element type, then splices that slice the same way an array's Elems is. The
			// generator lowers to a *value.Gen the drain pulls with Next until done; an
			// iterator-helper shape (an IteratorObject, a *value.IterHelper) has a different
			// Next and stays out of this path, matching how for...of routes the two apart.
			// The yield type must lower to the same Go type as the target element, so the
			// drained values splice without a conversion.
			if r.isGeneratorIterable(operand) && !r.isIterHelperType(r.prog.TypeAt(operand)) {
				if yieldT, ok := r.generatorElemType(r.prog.TypeAt(operand)); ok {
					yieldGo, err := r.typeExpr(yieldT)
					if err != nil {
						return nil, err
					}
					same, err := sameGoType(elemType, yieldGo)
					if err != nil {
						return nil, err
					}
					if !same {
						return nil, &NotYetLowerable{Reason: "spread of a generator with a different element type is a later slice"}
					}
					src, err := r.lowerExpr(operand)
					if err != nil {
						return nil, err
					}
					flush()
					if acc == nil {
						acc = &ast.CompositeLit{Type: seedType}
					}
					drained := r.generatorToSliceExpr(src, elemType)
					acc = &ast.CallExpr{Fun: ident("append"), Args: []ast.Expr{acc, drained}, Ellipsis: token.Pos(1)}
					continue
				}
			}
			// A spread of a string splices its code points, each a one-code-point
			// string, the same walk for...of over a string takes. value.BStr.CodePoints
			// returns the []BStr the append splices, so the target must be a string
			// array; a different element type hands back rather than mixing kinds.
			if r.isString(operand) {
				same, err := sameGoType(elemType, sel("value", "BStr"))
				if err != nil {
					return nil, err
				}
				if !same {
					return nil, &NotYetLowerable{Reason: "spread of a string into a non-string array is a later slice"}
				}
				src, err := r.lowerExpr(operand)
				if err != nil {
					return nil, err
				}
				flush()
				if acc == nil {
					acc = &ast.CompositeLit{Type: seedType}
				}
				points := &ast.CallExpr{Fun: &ast.SelectorExpr{X: src, Sel: ident("CodePoints")}}
				acc = &ast.CallExpr{Fun: ident("append"), Args: []ast.Expr{acc, points}, Ellipsis: token.Pos(1)}
				continue
			}
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

// arrayStaticCall lowers a static call on the global Array constructor. It
// reports handled=false when the callee is not Array.<method> on the ambient
// global, so the caller falls through to the ordinary method-call dispatch; a
// call that is on Array but names a method this slice does not cover reports
// handled=true with a hand-back so it does not fall through to a misleading
// receiver-typed error. Array.of is covered; Array.from, the runtime-tag-needing
// Array.isArray, and the length-preallocating Array(n) wait on their own slices.
func (r *Renderer) arrayStaticCall(call, callee frontend.Node, argNodes []frontend.Node) (ast.Expr, bool, error) {
	kids := r.prog.Children(callee)
	if len(kids) != 2 {
		return nil, false, nil
	}
	recvNode, method := kids[0], r.prog.Text(kids[1])
	if !r.isGlobalRef(recvNode, "Array") {
		return nil, false, nil
	}
	switch method {
	case "of":
		expr, err := r.arrayOf(call, argNodes)
		return expr, true, err
	case "from":
		expr, err := r.arrayFrom(call, argNodes)
		return expr, true, err
	case "fromAsync":
		expr, err := r.arrayFromAsync(call, argNodes)
		return expr, true, err
	case "isArray":
		expr, err := r.arrayIsArray(argNodes)
		return expr, true, err
	default:
		return nil, true, &NotYetLowerable{Reason: "Array." + method + " is a later slice"}
	}
}

// arrayOf lowers Array.of(e0, e1, ...) to the same value.NewArray construction
// an array literal takes: Array.of builds an array from its arguments as
// elements, one to one, exactly what [e0, e1, ...] does (unlike Array(n), whose
// single number argument sets a length rather than an element). The element
// type comes from the checker's type for the whole call, not from the
// arguments, so a widened call spells the type the checker inferred; a call
// whose element type does not lower, such as the empty Array.of() the checker
// leaves unknown, hands back.
func (r *Renderer) arrayOf(call frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	elemType, ok := r.arrayElem(call)
	if !ok {
		return nil, &NotYetLowerable{Reason: "Array.of whose element type does not lower yet"}
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		e, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, e)
	}
	args, err := r.wrapArrayElems(args, argNodes, call)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: index(sel("value", "NewArray"), elemType), Args: args}, nil
}

// arrayFrom lowers Array.from(source) where the source is iterable: a real
// array, whose backing slice is copied so the result aliases nothing; a string,
// whose code points become the elements; or a user iterable, drained through its
// Symbol.iterator the same pull-until-done walk a spread of that iterable takes.
// The result element type comes from the checker's type for the whole call and
// must match the source's Go element type so the collected values need no
// conversion. The array-like form (a plain object with a length and integer
// keys) and the optional map callback and thisArg are their own later slice and
// hand back.
func (r *Renderer) arrayFrom(call frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) == 0 {
		return nil, &NotYetLowerable{Reason: "Array.from with no source is a later slice"}
	}
	if len(argNodes) > 2 {
		return nil, &NotYetLowerable{Reason: "Array.from with a thisArg is a later slice"}
	}
	// A dynamic source, or a map callback, walks the source as an array-like value
	// at runtime, reading length and integer keys, and applies the optional map
	// callback there. This is the general form, producing a boxed array;
	// arrayFromBoxedResultCall keeps isDynamic in step so a read off the result
	// stays on the dynamic path.
	if r.arrayFromBoxedResultCall(call) {
		return r.arrayFromDynamic(argNodes)
	}
	if len(argNodes) > 1 {
		return nil, &NotYetLowerable{Reason: "Array.from with a map callback into a typed array is a later slice"}
	}
	elemType, ok := r.arrayElem(call)
	if !ok {
		return nil, &NotYetLowerable{Reason: "Array.from whose element type does not lower yet"}
	}
	src := argNodes[0]
	r.requireImport(valuePkg)
	// A real array copies its backing slice into a fresh []T, the same splice a
	// person writes with append, so the result shares storage with nothing.
	if opElemType, ok := r.arrayElem(src); ok {
		same, err := sameGoType(elemType, opElemType)
		if err != nil {
			return nil, err
		}
		if !same {
			return nil, &NotYetLowerable{Reason: "Array.from over an array with a different element type is a later slice"}
		}
		srcExpr, err := r.lowerExpr(src)
		if err != nil {
			return nil, err
		}
		elems := &ast.CallExpr{Fun: &ast.SelectorExpr{X: srcExpr, Sel: ident("Elems")}}
		copied := &ast.CallExpr{
			Fun:      ident("append"),
			Args:     []ast.Expr{&ast.CompositeLit{Type: &ast.ArrayType{Elt: elemType}}, elems},
			Ellipsis: token.Pos(1),
		}
		return &ast.CallExpr{Fun: sel("value", "ArrayFrom"), Args: []ast.Expr{copied}}, nil
	}
	// A user iterable is drained through its Symbol.iterator into a slice of its
	// element type, then handed to ArrayFrom the same way a spread splices it.
	if shape, ok := r.symbolIteratorShape(r.prog.TypeAt(src)); ok {
		iterElemType, err := r.typeExpr(shape.elem)
		if err != nil {
			return nil, err
		}
		same, err := sameGoType(elemType, iterElemType)
		if err != nil {
			return nil, err
		}
		if !same {
			return nil, &NotYetLowerable{Reason: "Array.from over an iterable with a different element type is a later slice"}
		}
		srcExpr, err := r.lowerExpr(src)
		if err != nil {
			return nil, err
		}
		drained := r.iterableToSliceExpr(srcExpr, elemType, shape)
		return &ast.CallExpr{Fun: sel("value", "ArrayFrom"), Args: []ast.Expr{drained}}, nil
	}
	// A string's elements are its code points, one substring per code point, the
	// same walk a for...of over a string takes.
	if r.isString(src) {
		srcExpr, err := r.lowerExpr(src)
		if err != nil {
			return nil, err
		}
		points := &ast.CallExpr{Fun: &ast.SelectorExpr{X: srcExpr, Sel: ident("CodePoints")}}
		return &ast.CallExpr{Fun: sel("value", "ArrayFrom"), Args: []ast.Expr{points}}, nil
	}
	return nil, &NotYetLowerable{Reason: "Array.from over an array-like object is a later slice"}
}

// arrayFromDynamic lowers Array.from into a boxed array by walking the source as
// an array-like value at runtime: value.ArrayFromArrayLike reads its length and
// integer keys and applies the optional map callback. The source and the callback
// box the same way a dynamic call's arguments do, and an absent callback passes
// value.Undefined so the runtime skips the map step. A thisArg is a later slice
// and has already handed back before here.
func (r *Renderer) arrayFromDynamic(argNodes []frontend.Node) (ast.Expr, error) {
	src, err := r.boxOperand(argNodes[0])
	if err != nil {
		return nil, err
	}
	mapFn := ast.Expr(sel("value", "Undefined"))
	if len(argNodes) == 2 {
		mapFn, err = r.boxOperand(argNodes[1])
		if err != nil {
			return nil, err
		}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "ArrayFromArrayLike"), Args: []ast.Expr{src, mapFn}}, nil
}

// arrayIsArray lowers Array.isArray(x), the runtime array-brand check. A dynamic
// value carries its kind at runtime, so the check dispatches through
// value.IsArray, which says false for an array-like object the way the spec's
// brand check does. A statically typed value's brand is known at compile time: a
// real array folds to true, and any other type, including an array-like object,
// folds to false.
func (r *Renderer) arrayIsArray(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "Array.isArray takes exactly one argument"}
	}
	arg := argNodes[0]
	if r.isDynamic(arg) {
		boxed, err := r.boxOperand(arg)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "IsArray"), Args: []ast.Expr{boxed}}, nil
	}
	if _, ok := r.arrayElem(arg); ok {
		return ident("true"), nil
	}
	return ident("false"), nil
}

// objectLiteral lowers an object literal { k: v, ... } to a composite literal
// that builds a pointer to the generated struct the object's shape interns to.
// The struct name comes from the same internStruct path a variable annotated
// with this shape takes, so a literal and a binding of the same shape produce
// the same Go type and structural assignability becomes Go assignability
// (05_type_lowering section 12). Each property lowers to a keyed field, so the
// literal's declaration order need not match the struct's field order.
//
// A spread member { ...src } copies another object's own fields into the struct:
// every property of the spread source becomes a keyed field reading src.Field,
// the same struct-field access a plain o.k read lowers to. Members apply left to
// right, so a key set by a later member wins over the same key from an earlier
// spread, matching JavaScript, and each field is emitted once at its first
// appearance. The spread source must be a plain identifier so reading its fields
// one by one never re-evaluates a side-effecting expression; a spread of a call
// or other expression is a later slice. A computed or string key, and a method
// or accessor member, still hand back, each its own later slice.
// computedKey returns the key expression of a computed-name member `[expr]: v`
// and reports whether the member has one. The frontend leaves a computed property
// name unclassified, so it is recognized by that unclassified kind together with
// the bracket its text opens on, and its single child is the expression the
// brackets wrap. A plain identifier, string, or numeric key is not a computed name
// and reports false.
func (r *Renderer) computedKey(member frontend.Node) (frontend.Node, bool) {
	kids := r.prog.Children(member)
	if len(kids) < 1 {
		return nil, false
	}
	key := kids[0]
	if key.Kind() != frontend.NodeUnknown {
		return nil, false
	}
	if !strings.HasPrefix(strings.TrimSpace(r.prog.Text(key)), "[") {
		return nil, false
	}
	inner := r.prog.Children(key)
	if len(inner) != 1 {
		return nil, false
	}
	return inner[0], true
}

// staticKeyLiteral reports whether a computed key's expression is a compile-time
// string or numeric constant, so the key it names is known and folds into a fixed
// shape the same way a plain key does. A parenthesized literal `(("b"))` unwraps to
// the literal inside, and a no-substitution template `\`b\“ is a constant string
// too. Any other expression, an identifier, a symbol, a call, is a runtime value
// whose key is not known until the literal runs.
func (r *Renderer) staticKeyLiteral(key frontend.Node) bool {
	for key.Kind() == frontend.NodeParenthesizedExpression {
		kids := r.prog.Children(key)
		if len(kids) != 1 {
			return false
		}
		key = kids[0]
	}
	switch key.Kind() {
	case frontend.NodeStringLiteral, frontend.NodeNumericLiteral, frontend.NodeNoSubstitutionTemplateLiteral:
		return true
	}
	return false
}

// objectLiteralNotFixed reports whether an object literal's set of property keys is
// unknown at compile time, so it cannot be named by a Go struct and must build as
// the dynamic bag. The only member that makes a shape non-fixed is a computed name
// `[expr]` whose expression is a runtime value: an identifier, a symbol, or any
// computed key. A computed name that brackets a string or numeric literal folds to
// that constant and keeps the shape fixed, the same closed key set a plain key
// gives, so a literal with only plain and constant-computed keys reports false.
func (r *Renderer) objectLiteralNotFixed(n frontend.Node) bool {
	for _, m := range r.prog.Children(n) {
		key, ok := r.computedKey(m)
		if !ok {
			continue
		}
		if !r.staticKeyLiteral(key) {
			return true
		}
	}
	return false
}

func (r *Renderer) objectLiteral(n frontend.Node) (ast.Expr, error) {
	// A literal whose shape is not statically fixed, one carrying a computed key that
	// names a runtime value, has no closed key set a Go struct could declare, so it
	// cannot lower on the struct path here. In a variable initializer such a literal is
	// intercepted upstream and boxed into the dynamic bag; anywhere else, an argument
	// or a nested value position, no boxing reaches it yet, so it hands back rather
	// than build a struct that would silently drop the computed member. This guards the
	// front door's tolerance of the 2464 computed-key diagnostic: admitting that code
	// lets such a literal reach the renderer, and this keeps the expr-position form an
	// honest handback instead of an empty struct.
	if r.objectLiteralNotFixed(n) {
		return nil, &NotYetLowerable{Reason: "object literal with a computed runtime key outside a variable initializer is a later slice"}
	}
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
	// Collect each field's final value keyed by its Go field name, and remember the
	// order fields first appear, so a plain literal emits its fields in source order
	// exactly as before while a spread's fields slot in where the spread sits. A
	// later member overwrites the value for a field an earlier one set, which is the
	// left-to-right override JavaScript's spread has.
	values := map[string]ast.Expr{}
	order := make([]string, 0)
	set := func(field string, val ast.Expr) {
		if _, seen := values[field]; !seen {
			order = append(order, field)
		}
		values[field] = val
	}
	for _, p := range r.prog.Children(n) {
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
			// its text opens with the spread token, so it routes to the spread copy.
			if strings.HasPrefix(strings.TrimSpace(r.prog.Text(p)), "...") {
				if err := r.objectSpread(kids[0], set); err != nil {
					return nil, err
				}
				continue
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
		set(field, val)
	}
	// Every field of the target struct must have been supplied by a member; if a
	// spread contributed fewer fields than its type names (a map-shaped source with
	// no static properties, say), a field would be left at its zero value, so hand
	// the whole literal back rather than emit a silently-incomplete struct.
	for _, tp := range r.prog.Properties(t) {
		field, ok := exportedField(tp.Name)
		if !ok {
			return nil, &NotYetLowerable{Reason: "object literal property name is not a Go identifier"}
		}
		if _, filled := values[field]; !filled {
			return nil, &NotYetLowerable{Reason: "object literal spread did not supply every field, a later slice"}
		}
	}
	elts := make([]ast.Expr, 0, len(order))
	for _, field := range order {
		elts = append(elts, &ast.KeyValueExpr{Key: ident(field), Value: values[field]})
	}
	return &ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(name), Elts: elts}}, nil
}

// objectLiteralContextual lowers an object literal whose slot declares a
// fixed-shape object with an optional property, the one case where the literal's
// own fresh type, whose members are all required, interns a different struct than
// the slot and so cannot build at its own type (05_type_lowering section 17). It
// builds at the declared shape instead, the same contextual typing TypeScript
// applies, so a literal { x: 3 } assigned to { x: number; y?: number } fills the
// y field with the empty optional rather than leaving it off a differently-shaped
// struct.
//
// Each present member becomes a keyed field in source order, wrapped in value.Some
// when the field it fills is optional, so the evaluation order of side-effecting
// members is preserved. Each optional field the members omit is appended as
// value.None, which have no effect to order. A required field left unfilled, a
// spread member, and a computed or non-identifier key each hand back to a later
// slice, keeping this slice to the plain contextual build.
func (r *Renderer) objectLiteralContextual(n frontend.Node, shape frontend.Type) (ast.Expr, error) {
	name, err := r.decls.internStruct(r, shape)
	if err != nil {
		return nil, err
	}
	elts := make([]ast.Expr, 0)
	seen := map[string]bool{}
	for _, p := range r.prog.Children(n) {
		if p.Kind() != frontend.NodeUnknown {
			return nil, &NotYetLowerable{Reason: "object literal with a method or accessor member is a later slice"}
		}
		kids := r.prog.Children(p)
		var keyNode, valNode frontend.Node
		switch len(kids) {
		case 1:
			if strings.HasPrefix(strings.TrimSpace(r.prog.Text(p)), "...") {
				return nil, &NotYetLowerable{Reason: "object spread into a shape with an optional property is a later slice"}
			}
			keyNode, valNode = kids[0], kids[0]
		case 2:
			keyNode, valNode = kids[0], kids[1]
		default:
			return nil, &NotYetLowerable{Reason: "object literal member with an unexpected shape is a later slice"}
		}
		if keyNode.Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: "object literal with a non-identifier key is a later slice"}
		}
		srcName := r.prog.Text(keyNode)
		field, ok := exportedField(srcName)
		if !ok {
			return nil, &NotYetLowerable{Reason: "object literal property name is not a Go identifier"}
		}
		sp, hasProp := r.shapeProp(shape, srcName)
		// The field's declared shape is the type the member must build at, unwrapping
		// an optional T | undefined to T so a nested literal and a present optional
		// both target the concrete field type.
		fieldShape := frontend.Type{}
		haveFieldShape := false
		if hasProp {
			fieldShape = sp.Type
			if inner, isOpt := r.optionalInner(r.prog.UnionMembers(sp.Type)); isOpt {
				fieldShape = inner
			}
			haveFieldShape = true
		}
		// An explicit undefined filling an optional field is the empty optional,
		// value.None, the same value an omitted optional field takes; someWrap over
		// the lowered undefined would store the boxed Undefined in a typed slot,
		// which does not compile, so this case is handled before the general path.
		if hasProp && sp.Optional && r.prog.TypeAt(valNode).Flags == frontend.TypeUndefined {
			inner, ok := r.optionalInner(r.prog.UnionMembers(sp.Type))
			if !ok {
				return nil, &NotYetLowerable{Reason: "optional property outside the T | undefined shape is a later slice"}
			}
			none, err := r.noneOf(inner)
			if err != nil {
				return nil, err
			}
			elts = append(elts, &ast.KeyValueExpr{Key: ident(field), Value: none})
			seen[field] = true
			continue
		}
		var val ast.Expr
		if valNode.Kind() == frontend.NodeObjectLiteralExpression && haveFieldShape && r.isPlainShape(fieldShape) {
			// A nested object literal builds at the field's declared shape, not its own
			// fresh required shape, so an inner optional property interns the same
			// struct the field type does and the value lands in the field without a
			// shape mismatch.
			val, err = r.objectLiteralContextual(valNode, fieldShape)
		} else {
			val, err = r.lowerExpr(valNode)
			if err == nil && haveFieldShape && fieldShape.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
				// The field is a dynamic value.Value slot, which the inferred shape takes
				// when the value flows into an untyped destructured binding. A static
				// member value has to box into that slot the same way an argument crossing
				// into an any parameter does, so a { a: 1 } for a value.Value field emits
				// A: value.Number(1) rather than the untyped A: 1 that will not compile.
				val, err = r.coerceToType(val, valNode, fieldShape)
			}
		}
		if err != nil {
			return nil, err
		}
		// A member filling an optional field is wrapped in value.Some so it lands in
		// the value.Opt slot the field became, unless the member is already an optional
		// of that type, which passes through as the Opt it is.
		if hasProp && sp.Optional && !r.isOptional(valNode) {
			inner, ok := r.optionalInner(r.prog.UnionMembers(sp.Type))
			if !ok {
				return nil, &NotYetLowerable{Reason: "optional property outside the T | undefined shape is a later slice"}
			}
			val, err = r.someWrap(val, inner)
			if err != nil {
				return nil, err
			}
		}
		elts = append(elts, &ast.KeyValueExpr{Key: ident(field), Value: val})
		seen[field] = true
	}
	// Fill each field the members did not supply: an omitted optional becomes the
	// empty optional value.None, while an omitted required field has no value to
	// stand in, so the literal hands back rather than leave a field at its zero.
	for _, tp := range r.prog.Properties(shape) {
		field, ok := exportedField(tp.Name)
		if !ok {
			return nil, &NotYetLowerable{Reason: "object literal property name is not a Go identifier"}
		}
		if seen[field] {
			continue
		}
		if !tp.Optional {
			return nil, &NotYetLowerable{Reason: "object literal missing a required field is a later slice"}
		}
		inner, ok := r.optionalInner(r.prog.UnionMembers(tp.Type))
		if !ok {
			return nil, &NotYetLowerable{Reason: "optional property outside the T | undefined shape is a later slice"}
		}
		none, err := r.noneOf(inner)
		if err != nil {
			return nil, err
		}
		elts = append(elts, &ast.KeyValueExpr{Key: ident(field), Value: none})
	}
	return &ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(name), Elts: elts}}, nil
}

// objectSpread copies the fields of a { ...src } member into the collector: each
// own property of the spread source becomes a field read src.Field, the same
// struct-field access member.go lowers a plain src.k read to, keyed by the same
// exportedField name so the read and the struct field agree. The source must be a
// plain identifier so its fields can be read one at a time without re-evaluating
// a side-effecting expression, and it must be a fixed-shape object, not an array
// or a map-shaped object, which spread by copying elements rather than fields.
func (r *Renderer) objectSpread(srcNode frontend.Node, set func(string, ast.Expr)) error {
	if srcNode.Kind() != frontend.NodeIdentifier {
		return &NotYetLowerable{Reason: "object spread of an expression that is not a plain identifier is a later slice"}
	}
	srcType := r.prog.TypeAt(srcNode)
	if srcType.Flags&frontend.TypeObject == 0 {
		return &NotYetLowerable{Reason: "object spread of a non-object is a later slice"}
	}
	if _, isArray := r.prog.ElementType(srcType); isArray {
		return &NotYetLowerable{Reason: "object spread of an array is a later slice"}
	}
	// internStruct confirms the source lowers to a struct and registers it, so the
	// field reads below select declared fields; a source shape that does not lower
	// hands back here rather than reading a field that was never declared.
	if _, err := r.decls.internStruct(r, srcType); err != nil {
		return err
	}
	src, err := r.lowerExpr(srcNode)
	if err != nil {
		return err
	}
	props := r.prog.Properties(srcType)
	if len(props) == 0 {
		return &NotYetLowerable{Reason: "object spread of a source with no static fields is a later slice"}
	}
	for _, sp := range props {
		field, ok := exportedField(sp.Name)
		if !ok {
			return &NotYetLowerable{Reason: "object spread source has a field name that is not a Go identifier"}
		}
		set(field, &ast.SelectorExpr{X: src, Sel: ident(field)})
	}
	return nil
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
	case "reduceRight":
		return r.arrayReduceRight(recvNode, argNodes)
	case "concat":
		return r.arrayConcat(recvNode, argNodes)
	case "splice":
		return r.arraySplice(recvNode, argNodes)
	case "toSpliced":
		return r.arraySpliceMethod(recvNode, "toSpliced", "ToSplicedToEnd", "ToSpliced", argNodes)
	case "with":
		if len(argNodes) != 2 {
			return nil, &NotYetLowerable{Reason: "array with takes an index and a value"}
		}
		if !r.isNumber(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "array with a non-number index is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		idx, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		val, err := r.lowerExpr(argNodes[1])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("With")}, Args: []ast.Expr{idx, val}}, nil
	case "flat":
		return r.arrayFlat(recvNode, argNodes)
	case "flatMap":
		return r.arrayFlatMap(recvNode, argNodes)
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
	case "findLast":
		return r.arrayCallbackMethod(recvNode, "FindLast", argNodes)
	case "findLastIndex":
		return r.arrayCallbackMethod(recvNode, "FindLastIndex", argNodes)
	case "reverse":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "array reverse takes no arguments"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Reverse")}}, nil
	case "toReversed":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "array toReversed takes no arguments"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ToReversed")}}, nil
	case "sort":
		return r.arraySort(recvNode, argNodes)
	case "toSorted":
		return r.arraySortMethod(recvNode, "ToSorted", argNodes)
	case "pop":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "array pop takes no arguments"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Pop")}}, nil
	case "shift":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "array shift takes no arguments"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Shift")}}, nil
	case "unshift":
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
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Unshift")}, Args: args}, nil
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
	case "copyWithin":
		if len(argNodes) < 1 || len(argNodes) > 3 {
			return nil, &NotYetLowerable{Reason: "array copyWithin takes a target and up to two bounds"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		// The target and the optional start and end are all Numbers, the same shape
		// slice's bounds take, so each lowers straight through once it is a number.
		args := make([]ast.Expr, 0, len(argNodes))
		for _, b := range argNodes {
			if !r.isNumber(b) {
				return nil, &NotYetLowerable{Reason: "array copyWithin with a non-number bound is a later slice"}
			}
			lowered, err := r.lowerExpr(b)
			if err != nil {
				return nil, err
			}
			args = append(args, lowered)
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("CopyWithin")}, Args: args}, nil
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
	case "values":
		return r.arrayIterConstructor(recvNode, "ArrayIterValues", argNodes)
	case "keys":
		return r.arrayIterConstructor(recvNode, "ArrayIterKeys", argNodes)
	case "entries":
		return r.arrayIterConstructor(recvNode, "ArrayIterEntries", argNodes)
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
	case "forEach":
		return r.mapForEach(recvNode, argNodes)
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

// setMethodCall lowers a method call on a Set receiver to the matching value.Set
// method (section 6.5). Each method maps to its Go name with an exact argument
// count: add(v) inserts and returns the set, has(v) and delete(v) report membership,
// and clear() empties it. The checker has already typed each argument against the
// set's own member type, so the arguments lower straight through with no extra kind
// guard; a method or an argument count outside this set hands back. It mirrors
// mapMethodCall, minus the keyed get and set a Set has no analogue for.
func (r *Renderer) setMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	var goName string
	var want int
	switch method {
	case "add":
		goName, want = "Add", 1
	case "has":
		goName, want = "Has", 1
	case "delete":
		goName, want = "Delete", 1
	case "clear":
		goName, want = "Clear", 0
	case "forEach":
		return r.setForEach(recvNode, argNodes)
	case "union":
		return r.setAlgebraCall(recvNode, "Union", argNodes)
	case "intersection":
		return r.setAlgebraCall(recvNode, "Intersection", argNodes)
	case "difference":
		return r.setAlgebraCall(recvNode, "Difference", argNodes)
	case "symmetricDifference":
		return r.setAlgebraCall(recvNode, "SymmetricDifference", argNodes)
	case "isSubsetOf":
		return r.setAlgebraCall(recvNode, "IsSubsetOf", argNodes)
	case "isSupersetOf":
		return r.setAlgebraCall(recvNode, "IsSupersetOf", argNodes)
	case "isDisjointFrom":
		return r.setAlgebraCall(recvNode, "IsDisjointFrom", argNodes)
	default:
		return nil, &NotYetLowerable{Reason: "set method ." + method + " is a later slice"}
	}
	if len(argNodes) != want {
		return nil, &NotYetLowerable{Reason: "set method ." + method + " with this argument count is a later slice"}
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

// mapForEach lowers map.forEach(cb), the insertion-order traversal (section 6.5).
// Only an inline arrow is covered, the same restriction the array callback methods
// take: a one-parameter arrow reads the value and lowers to ForEachValue, and a
// two-parameter arrow reads the value then the key, the order forEach passes them,
// and lowers to ForEach. A callback passed by name, or one that also reads the map
// parameter, is a later slice. A thisArg is inert for an arrow's lexical this, so
// the two-argument forEach hands back rather than drop the argument's evaluation.
func (r *Renderer) mapForEach(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 || argNodes[0].Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "map forEach with a callback that is not a single inline arrow function is a later slice"}
	}
	var goName string
	switch r.arrowParamCount(argNodes[0]) {
	case 1:
		goName = "ForEachValue"
	case 2:
		goName = "ForEach"
	default:
		return nil, &NotYetLowerable{Reason: "map forEach with a callback that also reads the map parameter is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	fn, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}, Args: []ast.Expr{fn}}, nil
}

// setForEach lowers set.forEach(cb), the insertion-order traversal (section 6.5).
// Only a single inline one-parameter arrow is covered: the specification passes the
// member twice and then the set (value, value, set), and the common callback reads
// only the first parameter, which lowers to ForEach. A callback passed by name, one
// that reads the second value or the set parameter, or a thisArg argument (inert for
// an arrow's lexical this) is a later slice and hands back.
func (r *Renderer) setForEach(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 || argNodes[0].Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "set forEach with a callback that is not a single inline arrow function is a later slice"}
	}
	if r.arrowParamCount(argNodes[0]) != 1 {
		return nil, &NotYetLowerable{Reason: "set forEach with a callback that reads the second value or set parameter is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	fn, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ForEach")}, Args: []ast.Expr{fn}}, nil
}

// promiseMethodCall lowers a method on a Promise receiver to a value.Promise
// method. then(onFulfilled) schedules a callback on the fulfilled value,
// catch(onRejected) on the rejection reason, and finally(onFinally) on either
// settlement with no argument; all three run at the single microtask drain at the end
// of main, in settle order. Only an inline arrow with no result is covered: the value
// methods take a func with no return, so a callback that returns a value (promise
// chaining) hands back, as does a then with a second rejection handler, since 6a
// mints only settled promises and observes fulfillment through then, rejection
// through catch, and cleanup through finally, one callback each. Any other member
// keeps its own reason.
func (r *Renderer) promiseMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	var goName string
	wantParams := 1
	switch method {
	case "then":
		goName = "Then"
	case "catch":
		goName = "Catch"
	case "finally":
		goName = "Finally"
		wantParams = 0
	default:
		return nil, &NotYetLowerable{Reason: "a promise method ." + method + " is a later slice"}
	}
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "a promise ." + method + " with other than one callback is a later slice"}
	}
	cb := argNodes[0]
	if cb.Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "a promise ." + method + " callback that is not an inline arrow function is a later slice"}
	}
	if r.arrowParamCount(cb) != wantParams {
		return nil, &NotYetLowerable{Reason: "a promise ." + method + " callback with an unexpected parameter count is a later slice"}
	}
	rt, rtOK := r.arrowResultFrontendType(cb)
	// A then whose callback returns a value chains: the returned promise carries that
	// value, or adopts the state of a promise the callback returns. This is the
	// value-producing form, distinct from the void form the plain .Then method covers.
	if method == "then" && rtOK && !isVoidReturn(rt) {
		return r.promiseThenChain(recvNode, cb, rt)
	}
	if !rtOK || !isVoidReturn(rt) {
		return nil, &NotYetLowerable{Reason: "a promise ." + method + " callback that returns a value (chaining) is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	fn, err := r.lowerExpr(cb)
	if err != nil {
		return nil, err
	}
	// A then or catch queues a microtask, so main must drain the queue at its end.
	r.usesPromise = true
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}, Args: []ast.Expr{fn}}, nil
}

// promiseThenChain lowers a then whose callback returns a value to the chaining runtime
// helper, value.ThenMap when the callback returns a plain value and value.ThenFlat when
// it returns a promise the chain adopts. Both infer their element types from the
// receiver and the callback, so the call needs no explicit type arguments and reads as
// value.ThenMap(recv, fn). The map-versus-adopt choice is the callback's result type: a
// result that lowers to a *value.Promise means the returned promise adopts that inner
// promise's state, and any other result is mapped through as the fulfilled value.
func (r *Renderer) promiseThenChain(recvNode, cb frontend.Node, rt frontend.Type) (ast.Expr, error) {
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	fn, err := r.lowerExpr(cb)
	if err != nil {
		return nil, err
	}
	goFn := "ThenMap"
	if _, ok := r.promiseElem(rt); ok {
		if rtGo, err := r.typeExpr(rt); err == nil && isPromiseGoType(rtGo) {
			goFn = "ThenFlat"
		}
	}
	r.usesPromise = true
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", goFn), Args: []ast.Expr{recv, fn}}, nil
}

// setAlgebraCall lowers the ES2025 set-algebra methods (union, intersection,
// difference, symmetricDifference, isSubsetOf, isSupersetOf, isDisjointFrom) to the
// matching value.Set method over a second set-like. JavaScript accepts any set-like
// as the argument, an object with a size, a has, and a keys iterator, and the two
// built-in set-likes are a Set and a Map (whose keys are its set-like members). The
// runtime method takes another *value.Set of the receiver's member type, so a Set
// argument passes straight through and a Map argument passes the Set of its keys,
// value.Map.KeySet, provided its key type lowers to the same Go type as the receiver's
// member. A set-like of a different member type, and the dynamic object-literal
// set-like a program builds by hand, hand back, the latter needing the dynamic has and
// keys protocol the typed path does not have. The combining methods return a new set
// and the predicates a boolean, which the method's own signature carries, so the call
// lowers to recv.Method(other) with no extra shaping.
func (r *Renderer) setAlgebraCall(recvNode frontend.Node, goName string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "set algebra ." + goName + " with other than one argument is a later slice"}
	}
	other := argNodes[0]
	recvElem, ok := r.setElem(r.prog.TypeAt(recvNode))
	if !ok {
		return nil, &NotYetLowerable{Reason: "set algebra ." + goName + " on a receiver whose member type did not lower"}
	}
	recvElemT, err := r.typeExpr(recvElem)
	if err != nil {
		return nil, err
	}
	// The argument is another Set, which passes through, or a Map, whose set-like view
	// is the Set of its keys. Its member (or key) Go type must match the receiver's
	// member so the runtime method's single type parameter is satisfied.
	var otherElem frontend.Type
	asKeySet := false
	switch {
	case r.isSet(other):
		otherElem, ok = r.setElem(r.prog.TypeAt(other))
	case r.isMap(other):
		otherElem, _, ok = r.mapKeyVal(r.prog.TypeAt(other))
		asKeySet = true
	default:
		return nil, &NotYetLowerable{Reason: "set algebra ." + goName + " with an arbitrary set-like other than a Set or a Map is a later slice"}
	}
	if !ok {
		return nil, &NotYetLowerable{Reason: "set algebra ." + goName + " with an argument whose member type did not lower"}
	}
	otherElemT, err := r.typeExpr(otherElem)
	if err != nil {
		return nil, err
	}
	same, err := sameGoType(recvElemT, otherElemT)
	if err != nil {
		return nil, err
	}
	if !same {
		return nil, &NotYetLowerable{Reason: "set algebra ." + goName + " between set-likes of different member types is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	arg, err := r.lowerExpr(other)
	if err != nil {
		return nil, err
	}
	// A Map argument presents its keys as the set-like the method reads.
	if asKeySet {
		arg = &ast.CallExpr{Fun: &ast.SelectorExpr{X: arg, Sel: ident("KeySet")}}
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}, Args: []ast.Expr{arg}}, nil
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

// arrayReduce lowers a reduce call, the left-to-right fold. It shares the fold
// machinery, naming the left-fold value function and method.
func (r *Renderer) arrayReduce(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	return r.arrayFold(recvNode, argNodes, "Reduce", "ReduceNoInit")
}

// arrayReduceRight lowers a reduceRight call, the right-to-left sibling of
// reduce. It shares the fold machinery, only naming the right-fold value
// function and method.
func (r *Renderer) arrayReduceRight(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	return r.arrayFold(recvNode, argNodes, "ReduceRight", "ReduceRightNoInit")
}

// arrayFold is the shared lowering for reduce and reduceRight. The initial-value
// form lowers to the free function named by freeFn because the accumulator type A
// may differ from the element type T, and a Go method cannot introduce the new
// type parameter A. The element type comes from the receiver and the accumulator
// type from the callback's result, the two type arguments the value function
// names. The no-init form delegates to arrayFoldNoInit.
//
// Only an inline two-parameter arrow is covered. A callback that also reads the
// index or array parameter needs those threaded through, so an arrow that is not
// exactly (accumulator, element) hands back, since the value function takes a
// two-parameter func.
func (r *Renderer) arrayFold(recvNode frontend.Node, argNodes []frontend.Node, freeFn, methodFn string) (ast.Expr, error) {
	if len(argNodes) == 1 {
		return r.arrayFoldNoInit(recvNode, argNodes[0], methodFn)
	}
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "array reduce with more than an initial value is a later slice"}
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
		Fun:  &ast.IndexListExpr{X: sel("value", freeFn), Indices: []ast.Expr{elemType, accType}},
		Args: []ast.Expr{recv, fn, init},
	}, nil
}

// arrayFoldNoInit lowers a reduce or reduceRight call with no initial value to
// the value.Array method named by methodFn over a lowered arrow. With no init the
// accumulator seeds from an end element, so its type is the element type and the
// callback is func(T, T) T, which is why this is a plain method rather than the
// free function the initial-value form needs for a differing accumulator type.
// An empty array throws at runtime, so no compile-time handling is needed here.
// Only an inline two-parameter arrow is covered, the same (accumulator, element)
// shape the initial-value form requires; a named callback or one that reads the
// index or array parameter hands back.
func (r *Renderer) arrayFoldNoInit(recvNode frontend.Node, arrow frontend.Node, methodFn string) (ast.Expr, error) {
	if arrow.Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "array reduce with a callback that is not an inline arrow function is a later slice"}
	}
	if r.arrowParamCount(arrow) != 2 {
		return nil, &NotYetLowerable{Reason: "array reduce with a callback that reads the index or array parameter is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	fn, err := r.lowerExpr(arrow)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(methodFn)}, Args: []ast.Expr{fn}}, nil
}

// arrayConcat lowers a concat call to the value.Array Concat method, which takes
// the argument arrays to splice onto a copy of the receiver. JavaScript's concat
// spreads an array argument one level and appends a non-array argument as a
// single element, so each argument is classified by its checker type: an argument
// that is an array of the receiver's element type passes through as an array to
// spread, and an argument that is the element type is wrapped in a one-element
// array so the runtime method sees a uniform list of arrays.
//
// The classification leans on the argument's lowered Go type, so an argument
// that is neither the element type nor an array of it, such as a concat that
// mixes element types or a spread of a non-array iterable, hands back. A concat
// with no arguments lowers to a bare Concat call, which returns a shallow copy of
// the receiver the way a.concat() does in JavaScript.
func (r *Renderer) arrayConcat(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	recvType := r.prog.TypeAt(recvNode)
	elem, ok := r.prog.ElementType(recvType)
	if !ok {
		return nil, &NotYetLowerable{Reason: "array concat on a receiver whose element type did not lower"}
	}
	elemType, err := r.typeExpr(elem)
	if err != nil {
		return nil, err
	}
	elemStr := goTypeString(elemType)
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	// An argument is classified by its lowered Go type rather than checker
	// identity, so a number literal argument reads as float64 the same way the
	// element does. An array of that Go type spreads, a bare value of it wraps in
	// a one-element array, and anything else hands back.
	args := make([]ast.Expr, 0, len(argNodes))
	for _, an := range argNodes {
		lowered, err := r.lowerExpr(an)
		if err != nil {
			return nil, err
		}
		at := r.prog.TypeAt(an)
		if ae, ok := r.prog.ElementType(at); ok {
			if aeType, err := r.typeExpr(ae); err == nil && goTypeString(aeType) == elemStr {
				args = append(args, lowered)
				continue
			}
		}
		if atType, err := r.typeExpr(at); err == nil && goTypeString(atType) == elemStr {
			wrapType, err := r.typeExpr(elem)
			if err != nil {
				return nil, err
			}
			args = append(args, &ast.CallExpr{Fun: index(sel("value", "NewArray"), wrapType), Args: []ast.Expr{lowered}})
			continue
		}
		return nil, &NotYetLowerable{Reason: "array concat with an argument that is neither the element type nor an array of it is a later slice"}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Concat")}, Args: args}, nil
}

// arraySplice lowers a splice call to the value.Array method that matches its
// argument shape. The one-argument splice(start) form, where the delete count is
// omitted and defaults to the rest of the array, lowers to SpliceToEnd. The form
// with a delete count lowers to Splice, passing the start, the count, and any
// items to insert. The start and count are Numbers and lower straight through
// once they are numbers; the items are typed against the element type by the
// checker, so they lower straight through as the method's variadic tail.
func (r *Renderer) arraySplice(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	return r.arraySpliceMethod(recvNode, "splice", "SpliceToEnd", "Splice", argNodes)
}

// arraySpliceMethod lowers splice and its copying sibling toSpliced, which share
// an argument shape: a required Number start, an optional Number delete count,
// and any number of items typed against the element type. The one-argument form
// lowers to the toEnd method and the longer form to the full method, so the two
// Go method names are passed in along with the source method name used in the
// hand-back messages. splice returns the removed elements and mutates while
// toSpliced returns the edited array and copies, but that difference lives in the
// runtime methods, not in how the call lowers.
func (r *Renderer) arraySpliceMethod(recvNode frontend.Node, name, toEndMethod, fullMethod string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) == 0 {
		return nil, &NotYetLowerable{Reason: "array " + name + " needs at least a start index"}
	}
	if !r.isNumber(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "array " + name + " with a non-number start is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	start, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	if len(argNodes) == 1 {
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(toEndMethod)}, Args: []ast.Expr{start}}, nil
	}
	if !r.isNumber(argNodes[1]) {
		return nil, &NotYetLowerable{Reason: "array " + name + " with a non-number delete count is a later slice"}
	}
	count, err := r.lowerExpr(argNodes[1])
	if err != nil {
		return nil, err
	}
	args := []ast.Expr{start, count}
	for _, item := range argNodes[2:] {
		lowered, err := r.lowerExpr(item)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(fullMethod)}, Args: args}, nil
}

// arrayFlat lowers a flat call at the default depth of one to the value.Flat free
// function instantiated at the inner element type. The receiver must be an array
// whose element type is itself an array, so that flattening one level yields an
// array of that inner element type. A depth argument is a later slice, since a
// depth other than one needs a different nesting of the flatten, so a flat with
// any argument hands back. A receiver whose element type is not an array also
// hands back, since there is nothing to flatten.
func (r *Renderer) arrayFlat(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 0 {
		return nil, &NotYetLowerable{Reason: "array flat with an explicit depth is a later slice"}
	}
	recvType := r.prog.TypeAt(recvNode)
	elem, ok := r.prog.ElementType(recvType)
	if !ok {
		return nil, &NotYetLowerable{Reason: "array flat on a receiver whose element type did not lower"}
	}
	innerElem, ok := r.prog.ElementType(elem)
	if !ok {
		return nil, &NotYetLowerable{Reason: "array flat on an array whose elements are not arrays is a later slice"}
	}
	innerType, err := r.typeExpr(innerElem)
	if err != nil {
		return nil, err
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: index(sel("value", "Flat"), innerType), Args: []ast.Expr{recv}}, nil
}

// arrayFlatMap lowers a flatMap call to the value.FlatMap free function, which
// maps each element to an array and concatenates the results one level. The
// function is instantiated at the element type T and the callback's inner result
// type U, so numbers.flatMap((n) => [n, -n]) spells value.FlatMap[float64,
// float64]. The callback must be an inline one-parameter arrow that returns an
// array, since that is the shape FlatMap flattens; an arrow that also reads the
// index or array parameter, or one that returns a bare value rather than an
// array, is a later slice and hands back.
func (r *Renderer) arrayFlatMap(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "array flatMap takes a single callback"}
	}
	arrow := argNodes[0]
	if arrow.Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "array flatMap with a callback that is not an inline arrow function is a later slice"}
	}
	if r.arrowParamCount(arrow) != 1 {
		return nil, &NotYetLowerable{Reason: "array flatMap with a callback that reads the index or array parameter is a later slice"}
	}
	elemType, ok := r.arrayElem(recvNode)
	if !ok {
		return nil, &NotYetLowerable{Reason: "array flatMap on a receiver whose element type did not lower"}
	}
	resultType, ok := r.arrowResultFrontendType(arrow)
	if !ok {
		return nil, &NotYetLowerable{Reason: "array flatMap with a block-bodied callback that has no call signature is a later slice"}
	}
	innerElem, ok := r.prog.ElementType(resultType)
	if !ok {
		return nil, &NotYetLowerable{Reason: "array flatMap with a callback that returns a value rather than an array is a later slice"}
	}
	innerType, err := r.typeExpr(innerElem)
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
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun:  &ast.IndexListExpr{X: sel("value", "FlatMap"), Indices: []ast.Expr{elemType, innerType}},
		Args: []ast.Expr{recv, fn},
	}, nil
}

// goTypeString renders a lowered Go type expression to its source form so two
// types can be compared by spelling. It is used where checker type identity is
// too strict, such as telling a number literal argument apart from the element
// type it widens to, since both render to the same Go type.
func goTypeString(e ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, token.NewFileSet(), e); err != nil {
		return ""
	}
	return buf.String()
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
	return r.arraySortMethod(recvNode, "Sort", argNodes)
}

// arraySortMethod lowers sort and its copying sibling toSorted, which share a
// comparator shape: both take one inline arrow of two parameters and lower to a
// method of the same name on the array receiver. Only the Go method name and the
// hand-back messages differ, so the method name is passed in. A missing
// comparator needs the default string-order sort, and a comparator that is not
// an inline two-parameter arrow is a later slice, the same limits sort had.
func (r *Renderer) arraySortMethod(recvNode frontend.Node, goMethod string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) == 0 {
		return nil, &NotYetLowerable{Reason: "array " + goMethod + " without a comparator needs the default string-order sort, a later slice"}
	}
	if len(argNodes) != 1 || argNodes[0].Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "array " + goMethod + " with a comparator that is not an inline arrow function is a later slice"}
	}
	arrow := argNodes[0]
	params := 0
	for _, k := range r.prog.Children(arrow) {
		if k.Kind() == frontend.NodeParameter {
			params++
		}
	}
	if params != 2 {
		return nil, &NotYetLowerable{Reason: "array " + goMethod + " comparator that does not take exactly two parameters is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	cmp, err := r.lowerExpr(arrow)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goMethod)}, Args: []ast.Expr{cmp}}, nil
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
	case elem.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0:
		// A dynamic element is a boxed value.Value, so join runs the abstract
		// ToString on each one at runtime, with join's own rule that undefined and
		// null become the empty string. This is the shape the assert prelude's
		// compareArray.format reaches through Array.prototype.map.call(...).join.
		r.requireImport(valuePkg)
		body = &ast.CallExpr{Fun: sel("value", "JoinString"), Args: []ast.Expr{ident("x")}}
	default:
		return nil, &NotYetLowerable{Reason: "array join on an element type without a value ToString is a later slice"}
	}
	params := &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("x")}, Type: elemGo}}}
	return &ast.FuncLit{
		Type: &ast.FuncType{Params: params, Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "BStr")}}}},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{body}}}},
	}, nil
}
