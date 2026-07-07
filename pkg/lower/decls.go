package lower

import (
	"go/ast"
	"go/token"
	"sort"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file owns the set of generated Go declarations a render pass produces,
// today the structs that object shapes lower to (05_type_lowering.md section
// 12). Two rules from the spec drive its design. First, a distinct structural
// shape lowers to exactly one Go struct, interned so that every object literal
// with the same fields shares the type and structural assignability becomes Go
// assignability (section 12). Second, the generated name is derived
// deterministically from the shape, so the same shape yields the same name
// across compilation units and builds (section 29).

// Decl is one generated Go declaration: a name and its gofmt-clean source.
type Decl struct {
	Name   string
	Source string
}

// declSet accumulates generated declarations during a render pass and interns
// object structs by structural identity. It keys on frontend.Type.Identity,
// which is the checker's structural identity surfaced through the frontend: two
// object types with the same shape share an identity, so they share a struct.
// The identity is only stable within one program, which is why a Renderer, and
// so a declSet, is scoped to one program.
type declSet struct {
	// nameByIdentity maps a checker type id to the struct name it was assigned. It
	// is a fast path and the cycle breaker: an id is recorded before a shape's
	// fields are rendered, so a field that refers back to the same type object
	// resolves to the reserved name instead of recursing forever.
	nameByIdentity map[int]string
	// nameBySig maps a shape's structural signature to its struct name. The checker
	// hands out a distinct type id for each object type object, including a fresh
	// literal type that is structurally identical to the widened type of the
	// binding it initializes, so keying on the id alone would emit two structs for
	// one shape and a literal of type ObjXY_2 would not assign to a binding of type
	// ObjXY. Keying on the structural signature collapses those to one struct,
	// which is what makes structural assignability become Go assignability.
	nameBySig map[string]string
	// source holds each generated declaration's gofmt-clean text, keyed by name.
	// A name maps to an empty string while its struct is still being built,
	// which is how a self-referential shape is broken: the name is reserved
	// before its fields are rendered.
	source map[string]string
	// node holds each generated declaration as its go/ast node, keyed by name, so
	// the program assembler can splice the struct types into the one ast.File it
	// prints rather than reparse their text. It carries the same declarations
	// source does, in the same order the order slice records.
	node map[string]ast.Decl
	// order is the names in first-seen order, so emission is stable.
	order []string
	// used records every assigned name so a second shape that derives the same
	// base name gets a deterministic numbered suffix instead of colliding.
	used map[string]bool
}

func newDeclSet() *declSet {
	return &declSet{
		nameByIdentity: map[int]string{},
		nameBySig:      map[string]string{},
		source:         map[string]string{},
		node:           map[string]ast.Decl{},
		used:           map[string]bool{},
	}
}

// internStruct returns the Go struct name for an object type, generating the
// struct declaration the first time it sees the shape and reusing the name after
// that. It reserves the name before rendering the fields so a field whose type
// refers back to this same object (a recursive shape) resolves to the reserved
// name instead of recursing forever.
func (d *declSet) internStruct(r *Renderer, t frontend.Type) (string, error) {
	id := t.Identity()
	if name, ok := d.nameByIdentity[id]; ok {
		return name, nil
	}
	// Dedupe by structural signature before reserving a name, so a fresh object
	// literal type and the widened binding type it initializes, which the checker
	// gives distinct ids, share the one struct. The signature is computed from the
	// shape without reserving a name, so a shape seen a second time under a new id
	// reuses the first struct rather than emitting a numbered twin.
	sig := structuralKey(r.prog, t, map[int]int{})
	if name, ok := d.nameBySig[sig]; ok {
		d.nameByIdentity[id] = name
		return name, nil
	}

	props := r.prog.Properties(t)
	// A callable object (one call signature plus properties) interns to a struct
	// with a reserved Call field. Its name comes from the interface's declared name
	// when it has one, so the emitted type reads like the source (Assert, not a
	// property-derived ObjSameValue), and falls back to the property-derived name
	// for an anonymous callable shape.
	call, construct := r.prog.Signatures(t)
	// A callable object with more than one call signature (an overload set) or with
	// a construct signature has no single Go func field to stand in for it, so the
	// whole shape hands back rather than dropping the extra signatures on the floor.
	if len(call) > 1 || len(construct) > 0 {
		return "", &NotYetLowerable{Flags: t.Flags, Reason: "an overloaded or constructable callable object is a later slice"}
	}
	var callSig *frontend.Signature
	base := structBaseName(props)
	if len(call) == 1 {
		callSig = &call[0]
		// A named interface (Assert) carries its name on the type symbol, so the
		// emitted struct reads like the source rather than a property-derived twin. An
		// anonymous type literal has the checker's internal "__type" symbol instead,
		// which is not a name a reader wrote, so those keep the property-derived base.
		if sym, ok := r.prog.TypeSymbol(t); ok && !strings.HasPrefix(sym.Name, "__") {
			if nm, ok := exportedField(sym.Name); ok {
				base = nm
			}
		}
	}
	name := d.reserve(base)
	d.nameByIdentity[id] = name
	d.nameBySig[sig] = name

	decl, err := renderStructBody(r, name, props, callSig)
	if err != nil {
		// Roll the reservation back so a later, lowerable use of a shape that
		// happens to share this base name is not pushed to a suffix by a failure.
		delete(d.nameByIdentity, id)
		delete(d.nameBySig, sig)
		delete(d.used, name)
		d.order = d.order[:len(d.order)-1]
		return "", err
	}
	body, err := printDecl(decl)
	if err != nil {
		delete(d.nameByIdentity, id)
		delete(d.nameBySig, sig)
		delete(d.used, name)
		d.order = d.order[:len(d.order)-1]
		return "", err
	}
	d.source[name] = body
	d.node[name] = decl
	return name, nil
}

// structuralKey builds a string that is equal for two types with the same
// structure and different for two types with a different structure, so the decl
// set can intern object structs by shape rather than by the checker's per type
// object id. It walks the type tree, keying a primitive by its flag, an array by
// its element, an object by its sorted property name and value keys, and a union
// by its sorted member keys. A type it does not break down structurally keys by
// its id, so two such types never collide (each stays its own struct) at the cost
// of not sharing one when they happen to match. The seen map records the depth at
// which each type id was entered, so a cycle keys by that relative depth and two
// isomorphic recursive shapes still produce equal keys.
func structuralKey(prog *frontend.Program, t frontend.Type, seen map[int]int) string {
	if t.Flags == 0 {
		return "void"
	}
	switch {
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
		id := t.Identity()
		if depth, ok := seen[id]; ok {
			return "@" + itoa(len(seen)-depth)
		}
		if elem, ok := prog.ElementType(t); ok {
			return "[]" + structuralKey(prog, elem, seen)
		}
		seen[id] = len(seen)
		props := prog.Properties(t)
		parts := make([]string, 0, len(props))
		for _, p := range props {
			opt := ""
			if p.Optional {
				opt = "?"
			}
			parts = append(parts, p.Name+opt+":"+structuralKey(prog, p.Type, seen))
		}
		// A call signature keys the shape too, so a callable object never dedupes
		// onto a plain object that happens to carry the same properties: the two
		// intern to different Go structs, one with a Call field and one without.
		if call, _ := prog.Signatures(t); len(call) == 1 {
			parts = append(parts, "()"+signatureKey(prog, call[0]))
		}
		sort.Strings(parts)
		delete(seen, id)
		return "{" + strings.Join(parts, ";") + "}"
	case t.Flags&frontend.TypeUnion != 0:
		members := prog.UnionMembers(t)
		parts := make([]string, 0, len(members))
		for _, m := range members {
			parts = append(parts, structuralKey(prog, m, seen))
		}
		sort.Strings(parts)
		return "(" + strings.Join(parts, "|") + ")"
	default:
		return "#" + itoa(t.Identity())
	}
}

// signatureKey builds a structural key for one call signature, so two callable
// objects with the same call shape key alike and one with a different arity or
// parameter kind keys apart. It keys each parameter by a shallow fingerprint of
// its type (the category, not the full nested structure), marking an optional
// with a trailing ?, and appends the return type's fingerprint after an arrow.
//
// The fingerprint is deliberately shallow. A callable object's own methods are
// themselves function-typed properties, and their parameters and returns can be
// further callable objects (a typed array's methods return typed arrays with the
// same methods, and the checker hands back a fresh type id at each step), so
// descending into signature types with structuralKey would recurse without
// bound because the cycle guard keys by id and never sees a repeat. The category
// fingerprint bottoms out at once, which distinguishes the shapes that actually
// differ (arity, a primitive versus an object parameter, the return category)
// while a rare pair that matches on the fingerprint but differs deeper simply
// shares a struct, the same structural approximation the object walk already
// accepts for a type it cannot break down.
func signatureKey(prog *frontend.Program, sig frontend.Signature) string {
	parts := make([]string, 0, len(sig.Params))
	for _, p := range sig.Params {
		opt := ""
		if p.Optional {
			opt = "?"
		}
		parts = append(parts, opt+shallowTypeKey(prog, p.Type))
	}
	return "(" + strings.Join(parts, ",") + ")->" + shallowTypeKey(prog, sig.Return)
}

// shallowTypeKey fingerprints a type by its category without descending into a
// nested shape, so a key built from it terminates at once. It is the bounded
// counterpart to structuralKey, used where a full structural walk would recurse
// without bound (a call signature's parameter and return types).
func shallowTypeKey(prog *frontend.Program, t frontend.Type) string {
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
		return "obj"
	case t.Flags&frontend.TypeUnion != 0:
		return "union"
	default:
		return "#" + itoa(t.Identity())
	}
}

// reserve picks a unique name from a base, appending _2, _3, and so on when the
// base is already taken by a different shape, and records it as used and in
// emission order.
func (d *declSet) reserve(base string) string {
	name := d.reserveName(base)
	d.order = append(d.order, name)
	return name
}

// reserveName claims a unique package-level name from a base and records it as
// used, but does not enter it in the struct/enum emission order the way reserve
// does. It is for declarations that emit through their own channel (the tagged
// union types render as a group in renderUnions), yet must still not collide with
// an interned struct or enum name, so they draw from the same used set.
func (d *declSet) reserveName(base string) string {
	name := base
	for n := 2; d.used[name]; n++ {
		name = base + "_" + itoa(n)
	}
	d.used[name] = true
	return name
}

// emit returns the declarations in first-seen order.
func (d *declSet) emit() []Decl {
	out := make([]Decl, 0, len(d.order))
	for _, name := range d.order {
		out = append(out, Decl{Name: name, Source: d.source[name]})
	}
	return out
}

// emitNodes returns the generated declarations as their go/ast nodes in
// first-seen order, so the program assembler can splice them into the single
// ast.File it prints alongside the lowered functions that refer to them.
func (d *declSet) emitNodes() []ast.Decl {
	out := make([]ast.Decl, 0, len(d.order))
	for _, name := range d.order {
		out = append(out, d.node[name])
	}
	return out
}

// renderStructBody builds one struct declaration as go/ast nodes: a field per
// property, in the source declaration order the frontend preserves so layout is
// stable (section 29). Each field's type is the node typeExpr already built for
// it, nested directly rather than spliced as text. It returns the declaration
// node itself so the caller can both print it (for the decl-source table) and
// splice it into the assembled program file, keeping one gofmt-clean node as the
// single source of truth (section 2).
func renderStructBody(r *Renderer, name string, props []frontend.Property, callSig *frontend.Signature) (*ast.GenDecl, error) {
	fields := &ast.FieldList{}
	// A callable object reserves the leading Call field for its call signature, so
	// the struct reads like the function bundle a Go developer hand-writes: a Call
	// closure plus the member closures and data fields. The field is the func type
	// the call signature lowers to; a call signature that does not lower (a static
	// optional, a rest parameter) hands the whole shape back.
	if callSig != nil {
		ft, err := r.funcTypeOf(*callSig)
		if err != nil {
			return nil, err
		}
		fields.List = append(fields.List, &ast.Field{
			Names: []*ast.Ident{ident("Call")},
			Type:  ft,
		})
	}
	for _, p := range props {
		if p.Optional && !r.isOptionalType(p.Type) {
			// An optional property x?: T types as T | undefined, which lowers to a
			// value.Opt[T'] field below through the ordinary typeExpr, with the Opt's
			// zero value standing for the absent member (05_type_lowering section 17).
			// An optional whose type is not that two-member shape (x?: number | string
			// adds a third member) needs the tagged sum and stays a later slice.
			return nil, &NotYetLowerable{Flags: p.Type.Flags, Reason: "optional property outside the T | undefined shape needs the tagged sum, a later slice"}
		}
		field, ok := exportedField(p.Name)
		if !ok {
			// A non-identifier property name (a space, a numeric key) belongs in
			// the object's symbol/string side table, not a struct field, which
			// is a later slice.
			return nil, &NotYetLowerable{Flags: p.Type.Flags, Reason: "non-identifier property name belongs in the object side table"}
		}
		if callSig != nil && field == "Call" {
			// The Call field is reserved for the call signature, so a callable object
			// with a property that also exports to Call collides; it hands back with
			// an honest reason rather than shadowing the call slot.
			return nil, &NotYetLowerable{Flags: p.Type.Flags, Reason: "a callable object with a property named call collides with the reserved Call field, a later slice"}
		}
		goType, err := r.typeExpr(p.Type)
		if err != nil {
			return nil, err
		}
		// The field carries the original JavaScript property name in a json struct
		// tag, so a reflection walk (JSON.stringify) recovers the exact key rather
		// than guessing it back from the exported Go name, which capitalizes the
		// first letter and so cannot tell "name" from "Name". The tag is the one
		// place the source key survives onto the Go type.
		fields.List = append(fields.List, &ast.Field{
			Names: []*ast.Ident{ident(field)},
			Type:  goType,
			Tag: &ast.BasicLit{
				Kind:  token.STRING,
				Value: "`json:\"" + p.Name + "\"`",
			},
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

// structBaseName derives the deterministic base name of an object struct from
// its property names, matching the Obj + capitalized-property-names convention
// the spec uses in its examples (ObjName, ObjIncGet). A property whose name is
// not a Go identifier is skipped for the purpose of the name, because it does
// not become a field; renderStructBody rejects such a shape anyway, so the name
// is only ever used for shapes made entirely of identifier fields. An empty
// object is ObjEmpty so it still has a legal, stable name.
func structBaseName(props []frontend.Property) string {
	names := make([]string, 0, len(props))
	for _, p := range props {
		if field, ok := exportedField(p.Name); ok {
			names = append(names, field)
		}
	}
	if len(names) == 0 {
		return "ObjEmpty"
	}
	// Sort so the name is independent of property order, since two shapes that
	// differ only in declaration order are the same structural type and must
	// share a name.
	sort.Strings(names)
	return "Obj" + strings.Join(names, "")
}

// itoa is a tiny non-allocating-enough integer to string for suffixing, kept
// local so the file has no strconv dependency for one use.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
