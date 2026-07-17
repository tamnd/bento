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
// The target must be a plain identifier local or a fixed-shape property member whose Go
// representation is a widened primitive already matching the element's, so a bare Go
// assignment is valid with no coercion: the declared-binding path never runs the
// assignment machinery, and routing through it here for a refined or crossing target
// would need it. A destructuring target, a dynamic, optional, int32/int64/bigint-refined,
// or non-primitive target, an element whose primitive category differs, and a Map, Set,
// generator, or user iterator source all report handled=false so the caller keeps the
// existing hand-back.
func (r *Renderer) forOfAssignTarget(target, iterable, bodyNode frontend.Node) (ast.Stmt, bool, error) {
	// An array pattern head, `for ([a, b] of pairs)`, assigns each tuple position to an
	// existing binding rather than a single whole element, so it takes the pattern path.
	if target.Kind() == frontend.NodeArrayLiteralExpression {
		return r.forOfAssignPattern(target, iterable, bodyNode)
	}
	targetCat, ok := r.forOfAssignScalarCategory(target)
	if !ok {
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

// forOfAssignScalarCategory validates that target is a plain, assignable scalar slot a
// bare Go assignment can carry a ranged element into, and returns its primitive category.
// A plain-identifier local and a fixed-shape property member both qualify when their Go
// representation is a widened primitive; a representation-refined, dynamic, or
// non-primitive target, and any other target shape, report ok=false so the caller keeps
// the existing hand-back.
func (r *Renderer) forOfAssignScalarCategory(target frontend.Node) (string, bool) {
	switch target.Kind() {
	case frontend.NodeIdentifier:
		name, ok := localName(r.prog.Text(target))
		if !ok {
			return "", false
		}
		// A representation-refined or dynamic target holds a Go type the range element does
		// not, so assigning the element by name would not compile.
		if r.int32Locals[name] || r.int64Locals[name] || r.bigOwned[name] || r.isDynamic(target) {
			return "", false
		}
	case frontend.NodePropertyAccessExpression:
		// A member target `o.f of xs` assigns each element into a struct field. The receiver
		// must be repeatable, so naming it fresh each iteration is sound, and neither the
		// receiver nor the member may be dynamic, so the member lowers to a Go field selector
		// a bare assignment can target rather than a boxed read the assignment machinery would
		// need. The primitive-category check below still gates a refined or crossing member.
		kids := r.prog.Children(target)
		if len(kids) == 0 || !r.repeatableOperand(kids[0]) || r.isDynamic(kids[0]) || r.isDynamic(target) {
			return "", false
		}
	default:
		return "", false
	}
	cat := plainPrimCategory(r.primitiveFlags(target))
	if cat == "" {
		return "", false
	}
	return cat, true
}

// forOfAssignPattern lowers a for...of whose head is an array pattern that assigns each
// position to an existing binding, `for ([a, b] of pairs)`, the assignment-form sibling
// of `for (const [a, b] of pairs)`. It ranges the array's Elems into a per-iteration
// temporary and assigns each tuple field to its target at the top of the loop body, so
// each target carries its position the way a declared binding would, `a, b = _bt.E0,
// _bt.E1`.
//
// Only a plain-identifier pattern over an array whose element is a tuple is handled, the
// motivating `[K, V][]` pairs shape, and each target's Go type must match its tuple
// position's so the parallel assignment is valid with no coercion. A nested, defaulted,
// or rest pattern position, a member or refined or dynamic target, a target whose type
// crosses its position's, a pattern binding more names than the tuple has, and a non-tuple
// or non-array source all report handled=false so the caller keeps the existing hand-back.
func (r *Renderer) forOfAssignPattern(pattern, iterable, bodyNode frontend.Node) (ast.Stmt, bool, error) {
	targets := r.prog.Children(pattern)
	if len(targets) == 0 {
		return nil, false, nil
	}
	if !isArrayElem(r, iterable) {
		return nil, false, nil
	}
	elem, ok := r.prog.ElementType(r.prog.TypeAt(iterable))
	if !ok {
		return nil, false, nil
	}
	tupleElems, ok := r.prog.TupleElements(elem)
	if !ok || len(targets) > len(tupleElems) {
		return nil, false, nil
	}
	names := make([]ast.Expr, 0, len(targets))
	for i, tgt := range targets {
		if tgt.Kind() != frontend.NodeIdentifier {
			return nil, false, nil
		}
		name, ok := localName(r.prog.Text(tgt))
		if !ok {
			return nil, false, nil
		}
		// A representation-refined or dynamic target holds a Go type the tuple field does
		// not, so a bare assignment would not compile; only a plain widened primitive local
		// is handled, matching the identifier path's guard.
		if r.int32Locals[name] || r.int64Locals[name] || r.bigOwned[name] || r.isDynamic(tgt) {
			return nil, false, nil
		}
		if tupleElems[i].Optional || tupleElems[i].Rest {
			return nil, false, nil
		}
		tgtGo, err := r.typeExpr(r.prog.TypeAt(tgt))
		if err != nil {
			return nil, false, err
		}
		fieldGo, err := r.typeExpr(tupleElems[i].Type)
		if err != nil {
			return nil, false, err
		}
		if same, err := sameGoType(tgtGo, fieldGo); err != nil {
			return nil, false, err
		} else if !same {
			return nil, false, nil
		}
		names = append(names, ident(name))
	}
	// Intern the tuple struct so the field reads name a declared Go type, the same as the
	// assignment-form statement destructure does.
	if _, err := r.decls.internTuple(r, elem, tupleElems); err != nil {
		return nil, false, err
	}
	iter, err := r.lowerExpr(iterable)
	if err != nil {
		return nil, false, err
	}
	body, err := r.loopBody(bodyNode)
	if err != nil {
		return nil, false, err
	}
	tmp := r.freshTemp()
	values := make([]ast.Expr, 0, len(targets))
	for i := range targets {
		values = append(values, &ast.SelectorExpr{X: ident(tmp), Sel: ident("E" + itoa(i))})
	}
	assign := &ast.AssignStmt{Lhs: names, Tok: token.ASSIGN, Rhs: values}
	body.List = append([]ast.Stmt{assign}, body.List...)
	rng := &ast.RangeStmt{
		Key:   ident("_"),
		Value: ident(tmp),
		Tok:   token.DEFINE,
		X:     &ast.CallExpr{Fun: &ast.SelectorExpr{X: iter, Sel: ident("Elems")}},
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
