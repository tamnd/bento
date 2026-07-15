package lower

import (
	"go/ast"

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
	// A closed string-literal union would lower to a small integer tag enum, but the
	// value conversions that enum needs, a string literal into its tag and a tag back
	// to its string for a comparison, a print, a template, or a coercion, are a later
	// slice. The enum type alone carries no value without them: emitting it would leave
	// a binding the source assigns a string to holding a Go integer type no string
	// enters or leaves, so the union hands back and the partitioner routes the unit to
	// the interpreter until those conversions land.
	return nil, &NotYetLowerable{Flags: t.Flags, Reason: "a closed string-literal union lowers to a tag enum, whose value conversions are a later slice"}
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

