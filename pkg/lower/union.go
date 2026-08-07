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

	// A reference type beside null lowers to that type's bare pointer, since the
	// pointer already carries nil as its null. It runs before the optional and
	// tagged-sum paths so the pointer, not a wrapper, is what a `next: Node | null`
	// field and every value that flows through it hold (nullableref.go).
	if inner, ok := r.nullableRef(t); ok {
		return r.typeExpr(inner)
	}

	// The optional shape T | undefined lowers to value.Opt[T'] rather than the
	// tagged sum, because undefined is not another type to discriminate but the
	// one missing value: a present flag beside a T slot captures it with no
	// boxing. This is the two-member union where one member is exactly undefined;
	// the other is lowered as the element type. A null member is not this shape
	// (null is a distinct value, not absence), so it falls through to the general
	// paths and hands back until the tagged sum lands.
	if inner, ok := r.optionalInner(members); ok {
		// An inner that itself lowers to the dynamic value.Value box already carries
		// undefined as a first-class value, so the optional collapses to the bare box
		// rather than value.Opt[value.Value]: wrapping it would leave a redundant present
		// flag and force every read to unwrap the Opt before the box, which the dynamic
		// member and element paths do not do. The empty object top type { } is the
		// motivating case, as { } | undefined.
		if inner.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 || r.isObjectTopType(inner) || r.isStringIndexDict(inner) {
			r.requireImport(valuePkg)
			return sel("value", "Value"), nil
		}
		elem, err := r.typeExpr(inner)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return index(sel("value", "Opt"), elem), nil
	}

	// A union of object shapes that all carry the same key set has nothing to
	// discriminate on and nothing to gain from a tag: the checker already answers a
	// read of any key with the union of what the members hold there, so one struct
	// whose fields are those unions is exactly the type, not an approximation. It
	// routes ahead of the tagged sum below, which would otherwise refuse for want of
	// a discriminant. The motivating shape is the ternary that picks between two
	// literals of the same keys, `typeof ms === "bigint" ? { two: 2n } : { two: 2 }`,
	// which node's own test harness opens with.
	if merged, ok, err := r.mergedObjectUnion(t, members); ok || err != nil {
		return merged, err
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
	// A closed string-literal union is a plain string at run time: its value is always
	// one of a fixed set of strings, so value.BStr carries it and every operation the
	// source writes, a compare against a member, a print, a template, a coercion, reads
	// it through the ordinary string machinery. typeExpr already folds this union to
	// value.BStr through primitiveFlagsOfType, so it reaches here only if that fold
	// missed the shape; returning the same BStr keeps the two paths in agreement.
	r.requireImport(valuePkg)
	return sel("value", "BStr"), nil
}

// mergedObjectUnion renders a union of like-keyed object shapes as the single struct
// their merge describes, and reports false for a union that is not that shape so the
// caller falls through to the tagged sum.
//
// The merge is sound because the key sets agree. TypeScript answers `u.k` on a union
// with the union of each member's `k`, which is exactly the field the merged struct
// carries, so a read is typed the same either way. The one thing the merge admits
// that the union does not is a combination across members, a value whose `two` came
// from one arm and whose `four` came from another, and nothing can build one: a
// value only ever enters the slot as a whole member, so every field of it comes from
// the same arm. Correlation is not lost either, because there was none to lose:
// TypeScript narrows a union of objects only through a discriminant property, which
// the caller has already looked for and not found.
//
// A differing key set is a different matter and stays with the tagged sum. Merging
// `{a} | {a, b}` would grow a `b` field on a value that does not have one, and the
// `in` operator, which is how the language tells those two apart, would then answer
// for a key the shape carries but the value never set.
//
// Every member has to be a plain record, the same bar the nullable-object arm sets:
// a class instance, an array, or a method bundle carries behavior a struct of data
// fields would drop.
func (r *Renderer) mergedObjectUnion(t frontend.Type, members []frontend.Type) (ast.Expr, bool, error) {
	if !r.likeKeyedObjectUnion(members) {
		return nil, false, nil
	}
	// The union's own properties are the merged fields, so the struct is interned from
	// the union type itself rather than assembled member by member.
	e, err := r.renderObject(t)
	if err != nil {
		return nil, false, err
	}
	return e, true, nil
}

// isMergedObjectUnion reports whether a type is a union that lowers to the merged
// struct, so the paths that work on a fixed shape, a member read, a field write, a
// struct literal, can treat it as the shape it lowers to. It answers only for a
// union; a plain object type is already a shape and takes those paths on its own.
func (r *Renderer) isMergedObjectUnion(t frontend.Type) bool {
	if t.Flags&frontend.TypeUnion == 0 {
		return false
	}
	return r.likeKeyedObjectUnion(r.prog.UnionMembers(t))
}

// mergedUnionSlot reports whether a name reads a slot whose declared type is a union
// of like-keyed object shapes, whatever the checker has narrowed it to at this use.
// The Go value in that slot is the merged struct for the whole of its life, since the
// narrowing is a fact about the checker's view and not about the representation, so a
// path that picks Go types from the narrowed type would pick a member struct the value
// never had. It answers the question about the slot, not the expression, which is why
// it asks the declared type rather than TypeAt.
func (r *Renderer) mergedUnionSlot(n frontend.Node) bool {
	_, ok := r.mergedUnionOf(n)
	return ok
}

// mergedUnionOf returns the merged object union a name's slot holds, whether or not
// the checker has narrowed the reference at this use, and reports false for anything
// else. The declared type is asked first because it is the one that names the Go
// struct; the narrowed type answers for an expression that is not a name and so has no
// declaration to ask.
func (r *Renderer) mergedUnionOf(n frontend.Node) (frontend.Type, bool) {
	if declared, _, ok := r.prog.DeclaredTypeAt(n); ok && r.isMergedObjectUnion(declared) {
		return declared, true
	}
	if t := r.prog.TypeAt(n); r.isMergedObjectUnion(t) {
		return t, true
	}
	return frontend.Type{}, false
}

// likeKeyedObjectUnion is the merge's condition on its own, with no interning, so it
// can be asked as a question.
func (r *Renderer) likeKeyedObjectUnion(members []frontend.Type) bool {
	if len(members) < 2 {
		return false
	}
	for _, m := range members {
		if m.Flags&frontend.TypeObject == 0 || m.Flags&frontend.TypeUnion != 0 {
			return false
		}
		if !r.isPlainRecordType(m) {
			return false
		}
	}
	// A discriminated union keeps its tag: narrowing on the discriminant is real
	// information the merge would throw away.
	if _, _, ok := r.discriminant(members); ok {
		return false
	}
	return sameKeySet(r, members)
}

// conditionalMergedObject lowers a ternary whose whole-expression type is a union of
// like-keyed object shapes. There is nothing to tag, so the IIFE returns the merged
// struct pointer and each branch builds at the merged shape rather than at its own
// member type, which is what keeps the two returns the same Go type. Building at the
// merged shape is also what preserves identity: the object is created once, as the
// thing the slot holds, rather than created at a member shape and copied into
// another.
//
// A branch that is not an object literal has nothing to rebuild. Converting an
// existing value from a member shape to the merged one would have to copy it field
// by field, and a copy is a different object from the one the source named, so that
// branch hands back instead.
func (r *Renderer) conditionalMergedObject(cond ast.Expr, trueNode, falseNode frontend.Node, target frontend.Type) (ast.Expr, bool, error) {
	if !r.isMergedObjectUnion(target) {
		return nil, false, nil
	}
	gt, err := r.typeExpr(target)
	if err != nil {
		return nil, false, err
	}
	branch := func(n frontend.Node) (ast.Expr, error) {
		lit := r.unwrapParens(n)
		if lit.Kind() != frontend.NodeObjectLiteralExpression {
			return nil, &NotYetLowerable{Reason: "a branch of a like-keyed object union that is not an object literal is a later slice"}
		}
		return r.objectLiteralContextual(lit, target)
	}
	whenTrue, err := branch(trueNode)
	if err != nil {
		return nil, false, err
	}
	whenFalse, err := branch(falseNode)
	if err != nil {
		return nil, false, err
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: gt}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.IfStmt{
				Cond: cond,
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{whenTrue}}}},
			},
			&ast.ReturnStmt{Results: []ast.Expr{whenFalse}},
		}},
	}
	return &ast.CallExpr{Fun: lit}, true, nil
}

// sameKeySet reports whether every member of an object union declares the same
// property names, the condition the merged struct rests on. Order is not part of it,
// since the checker lists properties in declaration order and two members of a union
// need not have been written the same way.
func sameKeySet(r *Renderer, members []frontend.Type) bool {
	first := keySet(r, members[0])
	for _, m := range members[1:] {
		ks := keySet(r, m)
		if len(ks) != len(first) {
			return false
		}
		for k := range ks {
			if !first[k] {
				return false
			}
		}
	}
	return true
}

// keySet collects a type's declared property names.
func keySet(r *Renderer, t frontend.Type) map[string]bool {
	out := map[string]bool{}
	for _, p := range r.prog.Properties(t) {
		out[p.Name] = true
	}
	return out
}

// optionalInner reports whether members are the optional shape T | undefined and
// returns the non-undefined member T if so. That shape is exactly one bare
// undefined member alongside the inner; a undefined member is recognized by its
// flags being exactly TypeUndefined, not by a undefined constituent of a wider
// type. A union without an undefined member (for example T | null), or with more
// than one, is not this shape and returns false, so the caller falls through to
// the string-literal and hand-back paths.
//
// The inner is usually a single member, T | undefined. The one exception is
// boolean: the checker spells boolean as the pair true | false, so boolean |
// undefined arrives as the three members true | false | undefined. When every
// non-undefined member carries the boolean facet, they widen to boolean, whose Go
// slot is bool, the same folding primitiveFlagsOfType applies to a bare true |
// false, so the inner is one of those boolean members and typeExpr renders it as
// bool. Any other multi-member remainder (a genuine string | number | undefined)
// is not an optional over a single inner and returns false.
func (r *Renderer) optionalInner(members []frontend.Type) (frontend.Type, bool) {
	undefIdx := -1
	rest := make([]frontend.Type, 0, len(members))
	for i, m := range members {
		if m.Flags == frontend.TypeUndefined {
			if undefIdx != -1 {
				return frontend.Type{}, false
			}
			undefIdx = i
			continue
		}
		rest = append(rest, m)
	}
	if undefIdx == -1 || len(rest) == 0 {
		return frontend.Type{}, false
	}
	if len(rest) == 1 {
		return rest[0], true
	}
	for _, m := range rest {
		if m.Flags&frontend.TypeBoolean == 0 {
			return frontend.Type{}, false
		}
	}
	return rest[0], true
}
