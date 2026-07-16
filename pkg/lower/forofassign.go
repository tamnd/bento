package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// forOfAssignTarget lowers a for...of whose head assigns each element to an existing
// binding, `for (x of it)`, rather than declaring a fresh one. It ranges the same
// backing slice the single-declaration range path ranges, an array's Elems, a string's
// CodePoints, or a numeric typed array's Floats, into a per-iteration temporary, and
// assigns that temporary to the target at the top of the loop body, so the target
// carries each element the way a declared binding would.
//
// The target must be a plain identifier bound to a widened primitive local whose Go
// representation already matches the element's, so a bare Go assignment is valid with no
// coercion: the declared-binding path never runs the assignment machinery, and routing
// through it here for a refined or crossing target would need it. A member or
// destructuring target, a dynamic, optional, int32/int64/bigint-refined, or non-primitive
// target, an element whose primitive category differs, and a Map, Set, generator, or user
// iterator source all report handled=false so the caller keeps the existing hand-back.
func (r *Renderer) forOfAssignTarget(target, iterable, bodyNode frontend.Node) (ast.Stmt, bool, error) {
	if target.Kind() != frontend.NodeIdentifier {
		return nil, false, nil
	}
	name, ok := localName(r.prog.Text(target))
	if !ok {
		return nil, false, nil
	}
	// A representation-refined or dynamic target holds a Go type the range element does not,
	// so assigning the element by name would not compile. Only a plain widened primitive
	// local is handled; every other shape hands back to the existing later-slice reason.
	if r.int32Locals[name] || r.int64Locals[name] || r.bigOwned[name] || r.isDynamic(target) {
		return nil, false, nil
	}
	targetCat := plainPrimCategory(r.primitiveFlags(target))
	if targetCat == "" {
		return nil, false, nil
	}

	var elemsMethod string
	switch {
	case isArrayElem(r, iterable):
		elem, ok := r.prog.ElementType(r.prog.TypeAt(iterable))
		if !ok || plainPrimCategory(r.primitiveFlagsOfType(elem)) != targetCat {
			return nil, false, nil
		}
		elemsMethod = "Elems"
	case r.numericTypedArray(iterable):
		if targetCat != "number" {
			return nil, false, nil
		}
		elemsMethod = "Floats"
	case r.isString(iterable):
		if targetCat != "string" {
			return nil, false, nil
		}
		elemsMethod = "CodePoints"
	default:
		// A Map, Set, generator, iterator helper, or user iterator source with an assignment
		// target stays a later slice: those forms drive a pull rather than range a backing
		// slice, and threading an assignment target through each is its own work.
		return nil, false, nil
	}

	iter, err := r.lowerExpr(iterable)
	if err != nil {
		return nil, false, err
	}
	lhs, err := r.lowerExpr(target)
	if err != nil {
		return nil, false, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, false, err
	}
	// Range into a temporary and assign it to the target at the top of the body, so the
	// element is named into the existing binding before the body reads it. The temporary is
	// always used, so no unused-range-value handling the declared path needs applies here.
	tmp := r.freshTemp()
	assign := &ast.AssignStmt{
		Lhs: []ast.Expr{lhs},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{ident(tmp)},
	}
	body.List = append([]ast.Stmt{assign}, body.List...)
	rng := &ast.RangeStmt{
		Key:   ident("_"),
		Value: ident(tmp),
		Tok:   token.DEFINE,
		X:     &ast.CallExpr{Fun: &ast.SelectorExpr{X: iter, Sel: ident(elemsMethod)}},
		Body:  body,
	}
	return rng, true, nil
}

// plainPrimCategory reports the primitive category of a type that lowers to a plain
// widened Go representation, "number" for float64, "string" for value.BStr, and
// "boolean" for bool, or "" for anything else. The flags passed in must already be
// folded by primitiveFlags / primitiveFlagsOfType, which widens a numeric-literal
// union (1 | 2 | 3) to number and the true | false union the checker often spells
// boolean as to boolean, so a boolean local and a boolean[] element both land on
// their plain Go repr here.
//
// A type carrying dynamic, null, undefined, bigint, or object facets has no single
// plain primitive representation and reports "", so an optional (string | undefined),
// a mixed union (number | boolean, which folds no facet), or a dynamic target keeps
// the hand-back. A folded union bit is deliberately tolerated: the category is read
// from the primitive facet, not from the shape of the type.
func plainPrimCategory(flags frontend.TypeFlags) string {
	const disqualify = frontend.TypeAny | frontend.TypeUnknown | frontend.TypeNull |
		frontend.TypeUndefined | frontend.TypeBigInt | frontend.TypeObject
	if flags&disqualify != 0 {
		return ""
	}
	switch {
	case flags&frontend.TypeNumber != 0 && flags&(frontend.TypeString|frontend.TypeBoolean) == 0:
		return "number"
	case flags&frontend.TypeString != 0 && flags&(frontend.TypeNumber|frontend.TypeBoolean) == 0:
		return "string"
	case flags&frontend.TypeBoolean != 0 && flags&(frontend.TypeNumber|frontend.TypeString) == 0:
		return "boolean"
	}
	return ""
}
