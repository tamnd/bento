package lower

import (
	"go/ast"
	"go/token"
	"sort"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers union types (05_type_lowering.md sections 9 and 10). It lands
// the closed string-literal union first: a union whose every member is a string
// literal is a closed, compile-time-known set of strings, so it lowers to a
// small integer tag enum rather than carrying a full bstr, and the comparisons
// the source writes against those literals become integer compares (section 10).
// The general tagged sum struct of section 9, for unions of unlike member types,
// is a later slice, so a union with any non-string-literal member hands back.

// renderUnion lowers a union type to the Go type that represents it. Today it
// covers only the closed string-literal union; every other union hands back so
// the partitioner routes the unit to the engine rather than get a wrong Go type.
func (r *Renderer) renderUnion(t frontend.Type) (ast.Expr, error) {
	members := r.prog.UnionMembers(t)

	// The optional shape T | undefined lowers to value.Opt[T'] rather than the
	// tagged sum, because undefined is not another type to discriminate but the
	// one missing value: a present flag beside a T slot captures it with no
	// boxing. This is the two-member union where one member is exactly undefined;
	// the other is lowered as the element type. A null member is not this shape
	// (null is a distinct value, not absence), so it falls through to the general
	// paths and hands back until the tagged sum lands.
	if inner, ok := r.optionalInner(members); ok {
		elem, err := r.typeExpr(inner)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return index(sel("value", "Opt"), elem), nil
	}

	values := make([]string, 0, len(members))
	allStringLiterals := true
	for _, m := range members {
		lit, ok := r.prog.LiteralValue(m)
		if !ok || lit.Kind != frontend.LiteralString {
			allStringLiterals = false
			break
		}
		values = append(values, lit.Str)
	}
	// A union that is not a closed set of string literals routes to the general
	// tagged-sum struct: a union of unlike primitive arms lowers to a discriminant
	// tag plus one inline field per arm (tagunion.go). internUnion hands back for a
	// union outside the primitive-arm subset, so a shape it cannot represent still
	// defers to the interpreter rather than emit a wrong Go type.
	if !allStringLiterals {
		info, err := r.internUnion(t)
		if err != nil {
			return nil, err
		}
		return ident(info.goName), nil
	}
	if len(values) == 0 {
		// A union the checker reports with no members is degenerate (never), which
		// has no value representation to render.
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "union with no members has no lowering"}
	}
	name, err := r.decls.internStringEnum(t.Identity(), values)
	if err != nil {
		return nil, err
	}
	return ident(name), nil
}

// optionalInner reports whether members are the optional shape T | undefined and
// returns the non-undefined member T if so. That shape is exactly two members
// where one is the bare undefined type; a undefined member is recognized by its
// flags being exactly TypeUndefined, not by a undefined constituent of a wider
// type. A union with more than two members, or a two-member union without an
// undefined member (for example T | null), is not this shape and returns false,
// so the caller falls through to the string-literal and hand-back paths.
func (r *Renderer) optionalInner(members []frontend.Type) (frontend.Type, bool) {
	if len(members) != 2 {
		return frontend.Type{}, false
	}
	a, b := members[0], members[1]
	switch {
	case a.Flags == frontend.TypeUndefined && b.Flags != frontend.TypeUndefined:
		return b, true
	case b.Flags == frontend.TypeUndefined && a.Flags != frontend.TypeUndefined:
		return a, true
	default:
		return frontend.Type{}, false
	}
}

// internStringEnum returns the Go enum type name for a closed string-literal
// union, generating the type and its tag constants the first time it sees the
// set and reusing the name after. Like the struct interner it keys on the
// union's structural identity so the same set of literals shares one enum, and
// it sorts the member values so the tag assignment (the iota order) and the
// generated name are independent of the order the checker lists the union.
func (d *declSet) internStringEnum(id int, values []string) (string, error) {
	if name, ok := d.nameByIdentity[id]; ok {
		return name, nil
	}

	sorted := append([]string(nil), values...)
	sort.Strings(sorted)

	variants, err := enumVariantNames(sorted)
	if err != nil {
		return "", err
	}

	name := d.reserve("Lit" + strings.Join(variants, ""))
	d.nameByIdentity[id] = name

	body, err := renderStringEnum(name, variants)
	if err != nil {
		delete(d.nameByIdentity, id)
		delete(d.used, name)
		d.order = d.order[:len(d.order)-1]
		return "", err
	}
	d.source[name] = body
	return name, nil
}

// enumVariantNames turns each string-literal value into the exported Go name of
// its tag constant. A value that is not spelled as a Go identifier (a space, a
// leading digit, punctuation) has no clean constant name; the mangled name table
// that would carry it is a later slice, so such a set hands back rather than
// invent a name. Two values that capitalize to the same identifier get a numeric
// suffix so every tag in one enum stays distinct.
func enumVariantNames(values []string) ([]string, error) {
	out := make([]string, 0, len(values))
	used := map[string]bool{}
	for _, v := range values {
		variant, ok := exportedField(v)
		if !ok {
			return nil, &NotYetLowerable{Reason: "string-literal union member is not an identifier; the mangled name table is a later slice"}
		}
		base := variant
		for n := 2; used[variant]; n++ {
			variant = base + itoa(n)
		}
		used[variant] = true
		out = append(out, variant)
	}
	return out, nil
}

// renderStringEnum builds the enum declaration as go/ast nodes: an unsigned
// integer tag type and the const block that names each tag, the first at iota so
// the rest follow. The tag names are the enum type name concatenated with each
// variant, which keeps them unique across enums without a package-level
// collision. Both declarations are printed through the gofmt-mode printer, so the
// result is gofmt-clean, matching the struct path.
func renderStringEnum(name string, variants []string) (string, error) {
	typeDecl := &ast.GenDecl{
		Tok:   token.TYPE,
		Specs: []ast.Spec{&ast.TypeSpec{Name: ident(name), Type: ident("uint8")}},
	}

	// Lparen is set so the const block always prints in its parenthesized group
	// form, even for a single-member enum, matching the const ( ... ) shape.
	constDecl := &ast.GenDecl{Tok: token.CONST, Lparen: token.Pos(1), Rparen: token.Pos(1)}
	for i, variant := range variants {
		spec := &ast.ValueSpec{Names: []*ast.Ident{ident(name + variant)}}
		if i == 0 {
			// The first tag pins the underlying type and starts the iota run that
			// the rest of the block inherits by omitting a value.
			spec.Type = ident(name)
			spec.Values = []ast.Expr{ident("iota")}
		}
		constDecl.Specs = append(constDecl.Specs, spec)
	}

	typeSrc, err := printDecl(typeDecl)
	if err != nil {
		return "", err
	}
	constSrc, err := printDecl(constDecl)
	if err != nil {
		return "", err
	}
	// A blank line separates the tag type from its const block, the shape a
	// developer expects to read.
	return strings.TrimRight(typeSrc, "\n") + "\n\n" + constSrc, nil
}
