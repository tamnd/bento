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
func renderTupleBody(r *Renderer, name string, elems []frontend.TupleElem) (*ast.GenDecl, error) {
	fields := &ast.FieldList{}
	for i, e := range elems {
		if e.Optional {
			return nil, &NotYetLowerable{Flags: e.Type.Flags, Reason: "an optional tuple element lowers through value.Opt, a later slice"}
		}
		if e.Rest {
			return nil, &NotYetLowerable{Flags: e.Type.Flags, Reason: "a tuple rest tail lowers to a slice field, a later slice"}
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
// with the tuple positions.
func (r *Renderer) tupleLiteral(n frontend.Node, elems []frontend.TupleElem) (ast.Expr, error) {
	t := r.prog.TypeAt(n)
	name, err := r.decls.internTuple(r, t, elems)
	if err != nil {
		return nil, err
	}
	kids := r.prog.Children(n)
	if len(kids) != len(elems) {
		return nil, &NotYetLowerable{Reason: "a tuple literal whose element count does not match the tuple arity (a spread or a hole) is a later slice"}
	}
	elts := make([]ast.Expr, 0, len(kids))
	for i, k := range kids {
		if k.Kind() == frontend.NodeSpreadElement {
			return nil, &NotYetLowerable{Reason: "a tuple literal spread element is a later slice"}
		}
		v, err := r.lowerExpr(k)
		if err != nil {
			return nil, err
		}
		v, err = r.coerceToType(v, k, elems[i].Type)
		if err != nil {
			return nil, err
		}
		elts = append(elts, &ast.KeyValueExpr{Key: ident("E" + itoa(i)), Value: v})
	}
	return &ast.CompositeLit{Type: ident(name), Elts: elts}, nil
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
// field read t.E<i>. A non-literal or out-of-range index, and an access on an
// optional or rest element whose field this slice does not emit, each hand back. It
// is the tuple counterpart of the array At read: where an array reads a[i] through a
// bounds-checked At because its length is dynamic, a tuple's positions are fixed and
// typed, so the read is a plain, statically sound struct selector.
func (r *Renderer) tupleElementRead(obj, idxNode frontend.Node, elems []frontend.TupleElem) (ast.Expr, error) {
	idx, ok := r.tupleLiteralIndex(idxNode, len(elems))
	if !ok {
		return nil, &NotYetLowerable{Reason: "a tuple element access with a non-literal or out-of-range index is a later slice"}
	}
	if elems[idx].Optional || elems[idx].Rest {
		return nil, &NotYetLowerable{Reason: "a tuple element access on an optional or rest element is a later slice"}
	}
	recv, err := r.lowerExpr(obj)
	if err != nil {
		return nil, err
	}
	return &ast.SelectorExpr{X: recv, Sel: ident("E" + itoa(idx))}, nil
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
	for i, el := range fixedElems {
		info, err := r.classifyArrayElem(el)
		if err != nil {
			return nil, err
		}
		if info.nested != nil || info.hasDefault {
			return nil, &NotYetLowerable{Reason: "a tuple destructuring nested pattern or defaulted element is a later slice"}
		}
		if elems[i].Optional || elems[i].Rest {
			return nil, &NotYetLowerable{Reason: "a tuple destructuring bind of an optional or rest element is a later slice"}
		}
		name, ok := localName(r.prog.Text(info.nameNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "destructured name is not a Go identifier"}
		}
		// The binding's declared type must render to the same Go type as the tuple
		// field it reads, so a := pair.E0 is a well-typed assignment. They agree in
		// the common case, since the checker types the binding off the tuple element;
		// a case where they diverge (a widening the field does not carry) hands back
		// rather than emit a mismatched read.
		nameGo, err := r.typeExpr(r.prog.TypeAt(info.nameNode))
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
			return nil, &NotYetLowerable{Reason: "a tuple destructuring where a binding's type differs from the tuple element type is a later slice"}
		}
		info.name = name
		infos[i] = info
	}
	// Emit the struct so the field reads name a declared Go type, even where the
	// tuple type reached the renderer only through this binding.
	if _, err := r.decls.internTuple(r, r.prog.TypeAt(initNode), elems); err != nil {
		return nil, err
	}
	prefix, recv, err := r.destructureSource(initNode)
	if err != nil {
		return nil, err
	}
	stmts := prefix
	for i, info := range infos {
		rc, err := recv()
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(info.name)},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.SelectorExpr{X: rc, Sel: ident("E" + itoa(i))}},
		})
	}
	return stmts, nil
}
