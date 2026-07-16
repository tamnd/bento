package partition

import "github.com/tamnd/bento/pkg/frontend"

// lowerable reports whether a type is in the set the lowering pass can render as
// Go (05_type_lowering.md). It is expressed entirely in frontend queries so the
// partitioner never re-derives a type or touches typescript-go. The predicate is
// conservative: a type it does not yet understand is not lowerable, which sends
// the unit toward interpretation or speculation rather than a wrong compile.
//
// The set covered here is the base of section 5.1: the primitives, fixed-shape
// objects and their arrays, and unions whose every member is itself lowerable.
// Generic type parameters, intersections, and enums are deferred to later
// lowering slices and report false for now, so a unit that depends on them stays
// off the compiled path until lowering learns to render them.
func lowerable(p *frontend.Program, t frontend.Type) bool {
	return lowerableRec(p, t, map[int]bool{})
}

func lowerableRec(p *frontend.Program, t frontend.Type, visited map[int]bool) bool {
	// A zero Type is the frontend's "no answer": a statement position, a void
	// expression. Nothing to lower and nothing to block, so it does not fail the
	// unit on its own.
	if t.Flags == 0 {
		return true
	}

	f := t.Flags

	// any and unknown are never lowerable. They are handled as untyped values at
	// the use site, but a declared any parameter or field also lands here.
	if f&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
		return false
	}

	// The primitives and the trivial return-position types render directly.
	const primitives = frontend.TypeNumber | frontend.TypeString | frontend.TypeBoolean |
		frontend.TypeBigInt | frontend.TypeNull | frontend.TypeUndefined | frontend.TypeSymbol |
		frontend.TypeVoid | frontend.TypeNever | frontend.TypeLiteral
	if f&primitives != 0 {
		return true
	}

	// Break cycles: a recursive object type (a node whose field points back at
	// its own type) would otherwise recurse forever. Identity is stable within
	// this one program traversal.
	id := t.Identity()
	if visited[id] {
		return true
	}
	visited[id] = true

	if f&frontend.TypeUnion != 0 {
		for _, m := range p.UnionMembers(t) {
			if !lowerableRec(p, m, visited) {
				return false
			}
		}
		return true
	}

	if f&frontend.TypeObject != 0 {
		// A tuple lowers when every positional element type does. Recognize it
		// before the array and the fixed-shape object cases: a tuple answers
		// TupleElements and refuses ElementType, and its own object properties are
		// the inherited array members, not its positional shape, so the object path
		// below would judge it on the wrong facts. Keeping the tuple on the compiled
		// path is what stops Pass A routing a tuple-typed unit to the engine
		// (typed/05, delivery slice 1).
		if elems, ok := p.TupleElements(t); ok {
			for _, e := range elems {
				if !lowerableRec(p, e.Type, visited) {
					return false
				}
			}
			return true
		}
		// An array lowers when its element type does.
		if elem, ok := p.ElementType(t); ok {
			return lowerableRec(p, elem, visited)
		}
		// A fixed-shape object lowers when every property type does.
		for _, prop := range p.Properties(t) {
			if !lowerableRec(p, prop.Type, visited) {
				return false
			}
		}
		return true
	}

	// Type parameters, intersections, enums, and anything else are not rendered
	// yet.
	return false
}
