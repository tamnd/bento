package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers heterogeneous tuple types and their literals, reads, and
// destructuring binds (typed/05 decision T7). A tuple is a fixed sequence of
// positional element types, each of which may have its own type, so it lowers to
// an interned Go struct with one positional field per element, E0, E1, and so on:
// [string, number] becomes `type Tuple_str_num struct { E0 value.BStr; E1 float64 }`.
// The struct is a value type, not a pointer, since a tuple is a value the way an
// array position is, so a read t[0] is the field read t.E0 and a destructure
// [a, b] = t is the pair of field reads a, b := t.E0, t.E1.
//
// The interning key is deliberately label-agnostic. A labeled tuple
// [lo: number, hi: number] and its unlabeled twin [number, number] are mutually
// assignable in TypeScript, so emitting the labels as field names (Lo, Hi versus
// E0, E1) would make two Go structs that do not assign across, and a program that
// passes one where the other is expected would fail to compile. This slice keys
// and spells every tuple by position alone, E<i>, so structurally equal tuples
// share the one struct regardless of their labels. Labeled-field emission, and the
// optional-element (value.Opt[T]) and rest-tail ([]T) forms, are later sub-slices;
// each hands back here rather than emit a partial shape.

// internTuple returns the Go struct name for a tuple type, generating the struct
// declaration the first time it sees the shape and reusing the name after that. It
// mirrors internStruct: it dedupes by a structural signature so two checker type
// ids for the same tuple shape share one struct, reserves the name before rendering
// the fields, and rolls the reservation back on a handback so a later lowerable use
// of a shape that derives the same base name is not pushed to a numbered suffix. The
// signature is built from the positional elements rather than the type's object
// properties, which for a tuple are the inherited array members, not its shape.
func (d *declSet) internTuple(r *Renderer, t frontend.Type, elems []frontend.TupleElem) (string, error) {
	id := t.Identity()
	if name, ok := d.nameByIdentity[id]; ok {
		return name, nil
	}
	nodes := 0
	sig := tupleKey(r.prog, elems, map[int]int{}, &nodes)
	if nodes > maxStructKeyNodes {
		return "", &NotYetLowerable{Flags: t.Flags, Reason: "a tuple type too deeply nested to key structurally is a later slice"}
	}
	if name, ok := d.nameBySig[sig]; ok {
		d.nameByIdentity[id] = name
		return name, nil
	}
	base := tupleBaseName(r.prog, elems)
	name := d.reserve(base)
	d.nameByIdentity[id] = name
	d.nameBySig[sig] = name

	rollback := func() {
		delete(d.nameByIdentity, id)
		delete(d.nameBySig, sig)
		delete(d.used, name)
		d.order = d.order[:len(d.order)-1]
	}
	decl, err := renderTupleBody(r, name, elems)
	if err != nil {
		rollback()
		return "", err
	}
	body, err := printDecl(decl)
	if err != nil {
		rollback()
		return "", err
	}
	d.source[name] = body
	d.node[name] = decl
	return name, nil
}

// tupleKey builds a structural signature for a tuple from its positional elements,
// equal for two tuples with the same element sequence and different for two whose
// elements differ. It keys each element by its type through the shared
// structuralKeyN walk, prefixing an optional with ? and a rest with ..., and wraps
// the whole in tup(...) so a tuple never dedupes onto a plain object that happens to
// carry E0/E1 fields. It does not read the element labels, so a labeled tuple and
// its unlabeled structural twin key alike and share one struct, which keeps the two
// Go-assignable the way TypeScript keeps them assignable.
func tupleKey(prog *frontend.Program, elems []frontend.TupleElem, seen map[int]int, budget *int) string {
	parts := make([]string, len(elems))
	for i, e := range elems {
		flag := ""
		if e.Optional {
			flag = "?"
		}
		if e.Rest {
			flag = "..."
		}
		parts[i] = flag + structuralKeyN(prog, e.Type, seen, budget)
	}
	return "tup(" + strings.Join(parts, ",") + ")"
}

// tupleBaseName derives the deterministic base name of a tuple struct from a short
// mnemonic of each element type, Tuple_str_num for [string, number], matching the
// Tuple_ + per-element-token convention typed/05 uses in its examples. The mnemonic
// is a category token, not the full structural key, so the name stays readable; two
// tuples whose names collide but whose shapes differ are told apart by reserve's
// numbered suffix, since interning keys on the full structural signature, not the
// name. Labels do not enter the name, the same label-agnostic rule the key follows.
func tupleBaseName(prog *frontend.Program, elems []frontend.TupleElem) string {
	parts := make([]string, len(elems))
	for i, e := range elems {
		parts[i] = tupleElemMnemonic(prog, e.Type)
	}
	return "Tuple_" + strings.Join(parts, "_")
}

// tupleElemMnemonic fingerprints a tuple element type by its category into a short
// Go-identifier-safe token for the struct name. It bottoms out at once on every
// type, so a nested object or tuple element contributes obj or tup rather than its
// whole shape, which keeps the name bounded; the full structural distinction lives
// in tupleKey, not the name.
func tupleElemMnemonic(prog *frontend.Program, t frontend.Type) string {
	switch {
	case t.Flags == 0:
		return "void"
	case t.Flags&frontend.TypeNumber != 0:
		return "num"
	case t.Flags&frontend.TypeString != 0:
		return "str"
	case t.Flags&frontend.TypeBoolean != 0:
		return "bool"
	case t.Flags&frontend.TypeBigInt != 0:
		return "big"
	case t.Flags&frontend.TypeSymbol != 0:
		return "sym"
	case t.Flags&frontend.TypeUndefined != 0:
		return "undef"
	case t.Flags&frontend.TypeNull != 0:
		return "null"
	case t.Flags&frontend.TypeObject != 0:
		if _, ok := prog.ElementType(t); ok {
			return "arr"
		}
		if _, ok := prog.TupleElements(t); ok {
			return "tup"
		}
		return "obj"
	case t.Flags&frontend.TypeUnion != 0:
		return "union"
	default:
		return "val"
	}
}

// renderTupleBody builds one tuple struct declaration as go/ast nodes: a positional
// field E<i> per element, in element order, whose type is the node typeExpr builds
// for the element. The struct carries no json tags, since a tuple's positions are
// not named properties a reflection walk recovers by key, and it is a value type,
// not a pointer, so a tuple flows by value the way an array position does. An
// optional element (value.Opt[T]) and a rest tail (a slice field) are later
// sub-slices, so each hands back here, which rolls internTuple's reservation back
// and leaves the whole tuple a clean handback rather than a partial struct.
// contextualTupleElems reports the declared tuple type and elements an array literal in
// this slot must build at, and true, only when the declared type is a tuple that carries an
// optional element. That is the one case where the literal's own all-required type interns a
// different struct than the slot declares (a value.Opt field versus a bare one), so the
// literal must build at the declared tuple the way a contextual object literal builds at its
// declared shape. A required-only tuple, a non-tuple, or an optional-wrapped tuple slot
// reports false, leaving the literal to build at its own type unchanged.
func (r *Renderer) contextualTupleElems(declared frontend.Type) (frontend.Type, []frontend.TupleElem, bool) {
	elems, ok := r.prog.TupleElements(declared)
	if !ok {
		return frontend.Type{}, nil, false
	}
	for _, e := range elems {
		if e.Optional {
			return declared, elems, true
		}
	}
	return frontend.Type{}, nil, false
}

// tupleShapeMismatch reports whether a value of tuple type src flows into a tuple slot
// target that carries an optional element and whose structural signature differs, the cross
// that would assign one interned tuple struct to a Go variable of another. Enabling the
// optional-element struct made such a target nameable, so a required-only literal (its own
// all-required twin) flowing into an optional slot at a site the contextual build does not
// cover, an argument, a return, a reassignment, must hand back rather than emit a mismatched
// struct assignment. Two tuples with the same signature share one struct and do not mismatch,
// and a target with no optional element keeps its pre-optional behavior untouched.
func (r *Renderer) tupleShapeMismatch(src, target frontend.Type) bool {
	tElems, tOk := r.prog.TupleElements(target)
	if !tOk {
		return false
	}
	sElems, sOk := r.prog.TupleElements(src)
	if !sOk {
		return false
	}
	hasOpt := false
	for _, e := range tElems {
		if e.Optional {
			hasOpt = true
			break
		}
	}
	if !hasOpt {
		return false
	}
	n1, n2 := 0, 0
	return tupleKey(r.prog, tElems, map[int]int{}, &n1) != tupleKey(r.prog, sElems, map[int]int{}, &n2)
}

// tupleElemInner is the value type an optional tuple element's value.Opt wraps: the
// checker may already widen an optional element to T | undefined, so a two-member
// optional element type unwraps to its present member, and a bare element type is its
// own inner. It is the T a value.Some[T]/value.None[T]/value.Opt[T] the optional element
// lowers through is parameterized on.
func (r *Renderer) tupleElemInner(e frontend.TupleElem) frontend.Type {
	if inner, ok := r.optionalInner(r.prog.UnionMembers(e.Type)); ok {
		return inner
	}
	return e.Type
}

func renderTupleBody(r *Renderer, name string, elems []frontend.TupleElem) (*ast.GenDecl, error) {
	fields := &ast.FieldList{}
	for i, e := range elems {
		if e.Rest {
			return nil, &NotYetLowerable{Flags: e.Type.Flags, Reason: "a tuple rest tail lowers to a slice field, a later slice"}
		}
		// An optional element holds a value.Opt[T] the same way a T | undefined binding or
		// object field does, so a position the literal supplies boxes to value.Some and one
		// it omits fills value.None, and a t[i] read hands back the Opt the presence-test and
		// narrowed-read machinery already unwraps.
		if e.Optional {
			inner := r.tupleElemInner(e)
			innerGo, err := r.typeExpr(inner)
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			fields.List = append(fields.List, &ast.Field{
				Names: []*ast.Ident{ident("E" + itoa(i))},
				Type:  index(sel("value", "Opt"), innerGo),
			})
			continue
		}
		goType, err := r.typeExpr(e.Type)
		if err != nil {
			return nil, err
		}
		fields.List = append(fields.List, &ast.Field{
			Names: []*ast.Ident{ident("E" + itoa(i))},
			Type:  goType,
		})
	}
	return &ast.GenDecl{
		Tok: token.TYPE,
		Specs: []ast.Spec{&ast.TypeSpec{
			Name: ident(name),
			Type: &ast.StructType{Fields: fields},
		}},
	}, nil
}

// tupleLiteral lowers a tuple-typed array literal [e0, e1, ...] to a composite of
// the interned tuple struct, one keyed field per element, Tuple_str_num{E0: ..., E1:
// ...}. The literal only reaches here when the checker typed it as a tuple, a
// contextually-typed [x, y]; a literal with no tuple context is an ordinary array
// and takes the array path. Each element value is coerced to its field's declared
// type, so a literal element that flows across the static/dynamic boundary or into a
// union field is bridged the same way an array element or an assignment is. A spread
// or a hole in the literal hands back, since its arity no longer lines up one-to-one
// with the tuple positions. A literal shorter than the arity is admitted only when every
// missing position is a trailing optional element: each supplied position boxes to
// value.Some if its element is optional, and each omitted trailing optional fills
// value.None, the same present/absent boxing a contextual object literal applies.
func (r *Renderer) tupleLiteral(n frontend.Node, elems []frontend.TupleElem) (ast.Expr, error) {
	return r.tupleLiteralAt(n, r.prog.TypeAt(n), elems)
}

// tupleLiteralAt lowers a tuple literal to build at a given tuple type and its elements,
// rather than the literal's own inferred type. The plain tupleLiteral passes the literal's
// checker type; a contextual slot whose declared tuple carries an optional element passes
// the declared type instead, so `const a: [number, string?] = [1, "x"]` builds the
// value.Opt-carrying declared struct the binding's Go type expects rather than the literal's
// all-required twin, the same contextual build objectLiteralContextual applies to objects.
func (r *Renderer) tupleLiteralAt(n frontend.Node, t frontend.Type, elems []frontend.TupleElem) (ast.Expr, error) {
	name, err := r.decls.internTuple(r, t, elems)
	if err != nil {
		return nil, err
	}
	kids := r.prog.Children(n)
	if len(kids) > len(elems) {
		return nil, &NotYetLowerable{Reason: "a tuple literal whose element count does not match the tuple arity (a spread or a hole) is a later slice"}
	}
	// A shorter literal is only sound when every position past its end is an optional
	// element value.None can fill; a missing required or rest position hands back.
	for i := len(kids); i < len(elems); i++ {
		if !elems[i].Optional {
			return nil, &NotYetLowerable{Reason: "a tuple literal shorter than the tuple arity that omits a required or rest position is a later slice"}
		}
	}
	elts := make([]ast.Expr, 0, len(elems))
	for i, k := range kids {
		if k.Kind() == frontend.NodeSpreadElement {
			return nil, &NotYetLowerable{Reason: "a tuple literal spread element is a later slice"}
		}
		inner := elems[i].Type
		if elems[i].Optional {
			inner = r.tupleElemInner(elems[i])
		}
		v, err := r.lowerExpr(k)
		if err != nil {
			return nil, err
		}
		v, err = r.coerceToType(v, k, inner)
		if err != nil {
			return nil, err
		}
		if elems[i].Optional {
			v, err = r.someWrap(v, inner)
			if err != nil {
				return nil, err
			}
		}
		elts = append(elts, &ast.KeyValueExpr{Key: ident("E" + itoa(i)), Value: v})
	}
	// Each omitted trailing optional position carries an explicit value.None so the
	// composite names every field and the struct value is fully initialized.
	for i := len(kids); i < len(elems); i++ {
		none, err := r.noneOf(r.tupleElemInner(elems[i]))
		if err != nil {
			return nil, err
		}
		elts = append(elts, &ast.KeyValueExpr{Key: ident("E" + itoa(i)), Value: none})
	}
	return &ast.CompositeLit{Type: ident(name), Elts: elts}, nil
}

// tupleArrayMethodCall lowers an array method borrowed on a tuple receiver. A tuple
// is an array subtype in TypeScript, so numTuple.map(f) is legal even though the
// tuple's Go representation is a positional struct that carries no such method. The
// tuple is materialized as a value.Array over its element union (the checker's
// numeric index type) and the array method dispatches on that materialized array.
// handled is false for a method this slice does not cover, so the caller falls
// through to its existing dispatch rather than mislower a method the tuple path does
// not host.
func (r *Renderer) tupleArrayMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node, elems []frontend.TupleElem) (ast.Expr, bool, error) {
	switch method {
	case "map":
		e, err := r.tupleMapCall(recvNode, argNodes, elems)
		return e, true, err
	default:
		return nil, false, nil
	}
}

// tupleMapCall lowers tuple.map(cb) to value.MapArray over the array the tuple
// materializes to. map always builds a fresh array whose element type is the
// callback's result, so it takes the free-function form value.MapArray[T, U] the way
// a type-changing array map does, with T the tuple's element union and U the
// callback's result. Only an inline arrow callback is covered; a named callback needs
// the reference resolved to a func value first, a later slice.
func (r *Renderer) tupleMapCall(recvNode frontend.Node, argNodes []frontend.Node, elems []frontend.TupleElem) (ast.Expr, error) {
	if len(argNodes) != 1 || argNodes[0].Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "a tuple map whose callback is not an inline arrow function is a later slice"}
	}
	arr, elemGo, err := r.materializeTupleArray(recvNode, elems)
	if err != nil {
		return nil, err
	}
	bodyType, err := r.arrowResultType(argNodes[0])
	if err != nil {
		return nil, err
	}
	fn, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun:  &ast.IndexListExpr{X: sel("value", "MapArray"), Indices: []ast.Expr{elemGo, bodyType}},
		Args: []ast.Expr{arr, fn},
	}, nil
}

// materializeTupleArray builds the value.Array a tuple materializes to when an array
// method is borrowed on it, value.NewArray[T](t.E0, t.E1, ...), and returns the array
// expression and its element Go type. T is the checker's numeric index type for the
// tuple, the union of its positions viewed as an array, so a heterogeneous tuple
// wraps each field into the arm its position selects and a homogeneous one passes its
// fields through. The receiver must be repeatable, since each field read re-evaluates
// it; a side-effecting source hands back rather than run its effect once per element.
func (r *Renderer) materializeTupleArray(recvNode frontend.Node, elems []frontend.TupleElem) (ast.Expr, ast.Expr, error) {
	if !r.repeatableOperand(recvNode) {
		return nil, nil, &NotYetLowerable{Reason: "materializing a side-effecting tuple source as an array is a later slice"}
	}
	elemT, ok := r.prog.NumberIndexType(r.prog.TypeAt(recvNode))
	if !ok {
		return nil, nil, &NotYetLowerable{Reason: "a tuple whose array element type the checker does not expose is a later slice"}
	}
	elemGo, err := r.typeExpr(elemT)
	if err != nil {
		return nil, nil, err
	}
	info, isUnion := r.unionInfoOrIntern(elemT)
	elts := make([]ast.Expr, 0, len(elems))
	for i, e := range elems {
		if e.Optional || e.Rest {
			return nil, nil, &NotYetLowerable{Reason: "materializing a tuple with an optional or rest element as an array is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, nil, err
		}
		field := &ast.SelectorExpr{X: recv, Sel: ident("E" + itoa(i))}
		switch {
		case isUnion:
			// A heterogeneous tuple materializes to a tagged-sum array, so each field is
			// wrapped into the arm its own position type selects, the same construction an
			// array literal of a union element type takes.
			arm, ok := info.armForFlags(e.Type.Flags)
			if !ok {
				return nil, nil, &NotYetLowerable{Reason: "a tuple element whose type does not select a union arm is a later slice"}
			}
			elts = append(elts, &ast.CallExpr{Fun: ident(info.ctorName(arm)), Args: []ast.Expr{field}})
		case elemT.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0:
			// An any[]/unknown[] element type stores boxed values, so a concrete field is
			// boxed into value.Value the way an element of a dynamic array is.
			boxed, err := r.boxStaticToDynamicFlags(field, e.Type.Flags)
			if err != nil {
				return nil, nil, err
			}
			elts = append(elts, boxed)
		default:
			elts = append(elts, field)
		}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: index(sel("value", "NewArray"), elemGo), Args: elts}, elemGo, nil
}

// tupleLiteralIndex reads a tuple element access index t[i] as a compile-time
// position, returning the integer and true only when i is a numeric literal whose
// value is a non-negative integer inside the tuple's arity. A non-literal index (a
// variable, an expression), a fractional or negative literal, and an out-of-range
// one each report false, so the caller hands the read back rather than select a
// field the struct does not carry. The bound is what keeps the emitted E<idx> a real
// field of the interned struct.
func (r *Renderer) tupleLiteralIndex(idxNode frontend.Node, arity int) (int, bool) {
	if idxNode.Kind() != frontend.NodeNumericLiteral {
		return 0, false
	}
	v, ok := numericLiteralValue(r.prog.Text(idxNode))
	if !ok {
		return 0, false
	}
	i := int(v)
	if float64(i) != v || i < 0 || i >= arity {
		return 0, false
	}
	return i, true
}

// tupleElementRead lowers a tuple element access t[i] with a literal index to the Go
// field read t.E<i>. A non-literal or out-of-range index, and an access on a rest
// element whose slice field this slice does not emit, each hand back; an optional
// element reads its value.Opt field, unwrapping with .Get() where the checker narrowed
// the access past a presence guard. It is the tuple counterpart of the array At read:
// where an array reads a[i] through a bounds-checked At because its length is dynamic,
// a tuple's positions are fixed and
// typed, so the read is a plain, statically sound struct selector.
func (r *Renderer) tupleElementRead(n, obj, idxNode frontend.Node, elems []frontend.TupleElem) (ast.Expr, error) {
	idx, ok := r.tupleLiteralIndex(idxNode, len(elems))
	if !ok {
		return nil, &NotYetLowerable{Reason: "a tuple element access with a non-literal or out-of-range index is a later slice"}
	}
	if elems[idx].Rest {
		return nil, &NotYetLowerable{Reason: "a tuple element access on a rest element is a later slice"}
	}
	recv, err := r.lowerExpr(obj)
	if err != nil {
		return nil, err
	}
	field := &ast.SelectorExpr{X: recv, Sel: ident("E" + itoa(idx))}
	// An optional position stores value.Opt[T]. A read the checker narrowed to T,
	// past a presence guard like t[i] !== undefined, unwraps with .Get(); a read
	// where the type still carries undefined keeps the bare Opt, which is what the
	// presence test and an Opt-to-Opt assignment want. This mirrors the identifier
	// read side's isOptBinding && !isOptional unwrap.
	if elems[idx].Optional && !r.isOptional(n) {
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: field, Sel: ident("Get")}}, nil
	}
	return field, nil
}

// tupleDestructure lowers a variable-declaration array destructure whose source is a
// tuple, const [a, b] = pair, to the pair of field reads a, b := pair.E0, pair.E1. A
// non-identifier source is evaluated once into a temporary the reads select off, the
// same destructureSource the array path uses, so a call source const [lo, hi] =
// minmax(xs) runs the call a single time. This slice binds plain, positional names
// only: a nested pattern, a default, a rest element, a pattern binding more names
// than the tuple has, an optional or rest tuple position, and a binding whose
// declared type differs from the tuple element's Go type each hand back, deferring
// those forms to a later sub-slice rather than mislower.
// bindingDeclaredType returns the un-narrowed declared type of a destructured
// binding name, falling back to the type at the node when no declared type is
// available. The declared type carries a widening the narrowed initializer type
// drops, let [x]: [string | number] = [1] declaring x the union though the field
// holds only the number the literal minted.
func (r *Renderer) bindingDeclaredType(nameNode frontend.Node) frontend.Type {
	if decl, _, ok := r.prog.DeclaredTypeAt(nameNode); ok {
		return decl
	}
	return r.prog.TypeAt(nameNode)
}

// initElemSource returns the i-th element node of an array-literal initializer,
// the source whose type a widened field read coerces from. A non-array-literal
// initializer, or an index past its elements, has no such node, so the widening
// stays a handback rather than a guess.
func (r *Renderer) initElemSource(initNode frontend.Node, i int) (frontend.Node, bool) {
	if initNode == nil || initNode.Kind() != frontend.NodeArrayLiteralExpression {
		return nil, false
	}
	kids := r.prog.Children(initNode)
	if i < 0 || i >= len(kids) {
		return nil, false
	}
	return kids[i], true
}

func (r *Renderer) tupleDestructure(patNode, initNode frontend.Node, elems []frontend.TupleElem) ([]ast.Stmt, error) {
	patElems := r.prog.Children(patNode)
	if len(patElems) == 0 {
		return nil, &NotYetLowerable{Reason: "an empty array destructuring pattern binds nothing"}
	}
	fixedElems, _, hasRest, err := r.splitArrayRest(patElems)
	if err != nil {
		return nil, err
	}
	if hasRest {
		return nil, &NotYetLowerable{Reason: "a tuple destructuring rest element is a later slice"}
	}
	if len(fixedElems) > len(elems) {
		return nil, &NotYetLowerable{Reason: "a tuple destructuring pattern that binds more elements than the tuple has is a later slice"}
	}
	infos := make([]arrayDefaultElem, len(fixedElems))
	coerceSrc := make([]frontend.Node, len(fixedElems))
	defaultVal := make([]ast.Expr, len(fixedElems))
	for i, el := range fixedElems {
		info, err := r.classifyArrayElem(el)
		if err != nil {
			return nil, err
		}
		if info.nested != nil {
			return nil, &NotYetLowerable{Reason: "a tuple destructuring nested pattern is a later slice"}
		}
		if info.hasDefault {
			// A defaulted element whose field is statically undefined always takes the
			// default, let [z = ""]: [string | undefined] = [undefined] binding z the "" the
			// annotation strips the undefined off of, so the read is dead and the fill folds to
			// the default alone. The default lowers to the binding's declared type. A defaulted
			// element over a field that can carry a present value still hands back, since that
			// needs the runtime undefined test this always-default fold skips.
			if elems[i].Type.Flags != frontend.TypeUndefined {
				return nil, &NotYetLowerable{Reason: "a tuple destructuring defaulted element over a possibly-present field is a later slice"}
			}
			name, ok := localName(r.prog.Text(info.nameNode))
			if !ok {
				return nil, &NotYetLowerable{Reason: "destructured name is not a Go identifier"}
			}
			def, err := r.lowerExpr(info.defNode)
			if err != nil {
				return nil, err
			}
			def, err = r.coerceToType(def, info.defNode, r.bindingDeclaredType(info.nameNode))
			if err != nil {
				return nil, err
			}
			info.name = name
			infos[i] = info
			defaultVal[i] = def
			continue
		}
		if elems[i].Optional || elems[i].Rest {
			return nil, &NotYetLowerable{Reason: "a tuple destructuring bind of an optional or rest element is a later slice"}
		}
		name, ok := localName(r.prog.Text(info.nameNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "destructured name is not a Go identifier"}
		}
		// The binding takes its declared type, which the read must render into. In the
		// common case the checker types the binding off the tuple element and the two
		// Go types agree, so a := pair.E0 is a bare assignment. Where the declared type is
		// wider than the field the initializer built, let [x]: [string | number] = [1]
		// declaring x the union while the field carries only the number the literal minted,
		// the read wraps into the binding's type through the initializer element as the
		// coercion source. A non-literal initializer has no element node to read the source
		// type from, so a genuine widening there stays a handback rather than a guess.
		nameGo, err := r.typeExpr(r.bindingDeclaredType(info.nameNode))
		if err != nil {
			return nil, err
		}
		fieldGo, err := r.typeExpr(elems[i].Type)
		if err != nil {
			return nil, err
		}
		if same, err := sameGoType(nameGo, fieldGo); err != nil {
			return nil, err
		} else if !same {
			src, ok := r.initElemSource(initNode, i)
			if !ok {
				return nil, &NotYetLowerable{Reason: "a tuple destructuring where a binding's type differs from the tuple element type is a later slice"}
			}
			coerceSrc[i] = src
		}
		info.name = name
		infos[i] = info
	}
	anyRead := false
	for i := range infos {
		if defaultVal[i] == nil {
			anyRead = true
			break
		}
	}
	// Emit the struct so the field reads name a declared Go type, even where the
	// tuple type reached the renderer only through this binding.
	if _, err := r.decls.internTuple(r, r.prog.TypeAt(initNode), elems); err != nil {
		return nil, err
	}
	var stmts []ast.Stmt
	recv := func() (ast.Expr, error) { return nil, &NotYetLowerable{Reason: "a tuple destructuring drew its source with no read"} }
	if anyRead {
		prefix, r2, err := r.destructureSource(initNode)
		if err != nil {
			return nil, err
		}
		stmts = prefix
		recv = r2
	} else if initNode.Kind() != frontend.NodeIdentifier {
		// Every position took its default, so no field is read, yet the source may hold
		// side effects, let [z = ""] = [f()] still calling f. Evaluate it once to the blank
		// so the effect survives without a declared temp Go would flag unused.
		lowered, err := r.lowerExpr(initNode)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ident("_")},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{lowered},
		})
	}
	for i, info := range infos {
		if defaultVal[i] != nil {
			// The always-undefined field makes the read dead, so the source struct is not
			// drawn for this position and the binding takes the default directly.
			stmts = append(stmts, &ast.AssignStmt{
				Lhs: []ast.Expr{ident(info.name)},
				Tok: token.DEFINE,
				Rhs: []ast.Expr{defaultVal[i]},
			})
			stmts = r.blankUnusedParamBinding(stmts, info.nameNode, info.name)
			continue
		}
		rc, err := recv()
		if err != nil {
			return nil, err
		}
		var read ast.Expr = &ast.SelectorExpr{X: rc, Sel: ident("E" + itoa(i))}
		if coerceSrc[i] != nil {
			read, err = r.coerceToType(read, coerceSrc[i], r.bindingDeclaredType(info.nameNode))
			if err != nil {
				return nil, err
			}
		}
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(info.name)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{read},
		})
		// The binding is a fresh := that the source may never read, const [x] = pair
		// with x unused, and Go rejects an unused local. The object-pattern path already
		// blanks such a member; the tuple path did the same read without the blank, so
		// an unread element compiled to a declared-and-not-used error. Reuse the shared
		// blank, which appends _ = name only when the binding's own use count shows no
		// read survives.
		stmts = r.blankUnusedParamBinding(stmts, info.nameNode, info.name)
	}
	return stmts, nil
}

// tupleDestructureValues lowers the right side of an assignment-form array destructure
// whose source is a tuple variable, [a, b] = pair, into one field read per target,
// pair.E0, pair.E1, ready for the parallel assignment. It is the assignment-form
// sibling of tupleDestructure, which binds fresh names with :=; here the targets are
// already-declared locals the caller assigns with =. The source is a plain identifier,
// so each per-index read re-evaluates it harmlessly the same way the array AtI path
// does. This is a pure read of the tuple, so it is sound under the value-struct model:
// nothing is written back through the source. A pattern that binds more elements than
// the tuple has, an optional or rest tuple position, and a target whose Go type differs
// from the tuple element's each hand back, matching tupleDestructure's guards.
func (r *Renderer) tupleDestructureValues(targets []frontend.Node, rhs frontend.Node, elems []frontend.TupleElem) ([]ast.Expr, error) {
	if len(targets) > len(elems) {
		return nil, &NotYetLowerable{Reason: "an assignment-form tuple destructure that binds more elements than the tuple has is a later slice"}
	}
	// Emit the struct so the field reads name a declared Go type, even where the tuple
	// type reached the renderer only through this assignment.
	if _, err := r.decls.internTuple(r, r.prog.TypeAt(rhs), elems); err != nil {
		return nil, err
	}
	values := make([]ast.Expr, 0, len(targets))
	for i, tgt := range targets {
		if elems[i].Optional || elems[i].Rest {
			return nil, &NotYetLowerable{Reason: "an assignment-form tuple destructure of an optional or rest element is a later slice"}
		}
		tgtGo, err := r.typeExpr(r.prog.TypeAt(tgt))
		if err != nil {
			return nil, err
		}
		fieldGo, err := r.typeExpr(elems[i].Type)
		if err != nil {
			return nil, err
		}
		if same, err := sameGoType(tgtGo, fieldGo); err != nil {
			return nil, err
		} else if !same {
			return nil, &NotYetLowerable{Reason: "an assignment-form tuple destructure where a target's type differs from the tuple element type is a later slice"}
		}
		recv, err := r.lowerExpr(rhs)
		if err != nil {
			return nil, err
		}
		values = append(values, &ast.SelectorExpr{X: recv, Sel: ident("E" + itoa(i))})
	}
	return values, nil
}
