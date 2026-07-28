package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the nullable reference, `T | null`, where T is a type whose
// Go form is already a pointer: a class instance (*C), an array
// (*value.Array[E]), and anything else that lands on a pointer.
//
// Such a T has a null of its own, nil. That makes the tagged sum the general
// object union needs (tagunion.go) pure overhead here, and worse, it would break
// identity: a value of the union would no longer be the same pointer an ordinary
// T binding holds. So this shape keeps the bare pointer. The union's Go type is
// T's, the null literal in such a slot is nil, and a `x != null` guard is
// `x != nil`. Narrowing needs no unwrap at all, because the narrowed T and the
// union T | null share one Go representation.
//
// This is what lets a linked structure lower, which the ES5-era benchmarks and
// most hand-written data structures are built from: a `next: Node | null` field,
// a `head: Node | null` root, and a walk that stops on null.

// nullableRef returns the non-null member of a union that is exactly one
// pointer-lowered type beside one or more null and undefined sentinels. A union
// with no such member, with two of them, with any other non-sentinel member, or
// with no null member at all is not this shape: `T | undefined` stays with the
// optional pre-pass and its value.Opt, so requiring null keeps this path off
// shapes that already lower.
func (r *Renderer) nullableRef(t frontend.Type) (frontend.Type, bool) {
	if t.Flags&frontend.TypeUnion == 0 {
		return frontend.Type{}, false
	}
	members := r.prog.UnionMembers(t)
	if len(members) < 2 {
		return frontend.Type{}, false
	}
	var inner frontend.Type
	found := false
	hasNull := false
	for _, m := range members {
		if m.Flags&frontend.TypeNull != 0 {
			hasNull = true
			continue
		}
		if m.Flags&frontend.TypeUndefined != 0 {
			continue
		}
		if found || !r.lowersToPointer(m) {
			return frontend.Type{}, false
		}
		inner, found = m, true
	}
	if !found || !hasNull {
		return frontend.Type{}, false
	}
	return inner, true
}

// lowersToPointer reports whether a type's Go form is a pointer, which is what
// makes nil available as its null. The test is the rendered type itself rather
// than a list of type flags, so every shape that already lowers to a pointer (a
// registered class, an array) is admitted on the same footing and a shape that
// does not (a string, a number, a struct-shaped union) is not. A type that hands
// back has no Go form at all and is not this case.
func (r *Renderer) lowersToPointer(t frontend.Type) bool {
	rendered, err := r.RenderType(t)
	if err != nil {
		return false
	}
	return strings.HasPrefix(rendered, "*")
}

// nullableRefAt reports whether a node's type is the nullable reference shape,
// the form the expression paths ask in: a comparison against null, a truthiness
// test, a null literal flowing into a slot. It reads the declared type as well
// as the narrowed one, since a narrowed T and the union T | null lower to the
// same pointer either way.
func (r *Renderer) nullableRefAt(n frontend.Node) bool {
	if declared, _, ok := r.prog.DeclaredTypeAt(n); ok {
		if _, ok := r.nullableRef(declared); ok {
			return true
		}
	}
	_, ok := r.nullableRef(r.prog.TypeAt(n))
	return ok
}

// conditionalRef lowers a ternary whose result is a reference, `cond ? node :
// null` and `cond ? a : b` over two class instances alike. Neither needs the
// tagged sum the general union path builds: the whole expression's Go type is
// already a pointer, so the IIFE returns that pointer and a null branch is nil.
// The branches are lowered by the caller and pass through wrapToUnion, which is
// what turns the null literal into nil and leaves a real reference alone.
//
// It reports handled=false when the result type is not a pointer, which leaves
// the tagged-sum path below it untouched. The condition and both branches are
// already lowered, so answering false costs nothing but the type test.
func (r *Renderer) conditionalRef(cond, whenTrue ast.Expr, trueNode frontend.Node, whenFalse ast.Expr, falseNode frontend.Node, target frontend.Type) (ast.Expr, bool, error) {
	if !r.lowersToPointer(target) {
		return nil, false, nil
	}
	retType, err := r.typeExpr(target)
	if err != nil {
		return nil, false, nil
	}
	trueWrapped, _, err := r.wrapToUnion(whenTrue, trueNode, target)
	if err != nil {
		return nil, false, err
	}
	falseWrapped, _, err := r.wrapToUnion(whenFalse, falseNode, target)
	if err != nil {
		return nil, false, err
	}
	lit := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: retType}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{
			&ast.IfStmt{
				Cond: cond,
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{trueWrapped}}}},
			},
			&ast.ReturnStmt{Results: []ast.Expr{falseWrapped}},
		}},
	}
	return &ast.CallExpr{Fun: lit}, true, nil
}

// nullableRefNullCompare lowers an equality between a nullable reference and the
// null literal to the Go nil comparison, `p != nil`. All four operators reduce
// to the same test: === and !== against null are an identity check, and == and
// != against null admit undefined too, which a bare pointer can never be, so
// they agree. It reports handled=false when the operator is not an equality,
// when neither side is the null literal, or when the other side is not a
// nullable reference, which leaves the boxed LooseEquals path and the operator
// table untouched.
func (r *Renderer) nullableRefNullCompare(opText string, left, right frontend.Node) (ast.Expr, bool, error) {
	var goOp token.Token
	switch opText {
	case "===", "==":
		goOp = token.EQL
	case "!==", "!=":
		goOp = token.NEQ
	default:
		return nil, false, nil
	}
	ref, null := left, right
	if !r.isNullOrUndefined(null) {
		ref, null = right, left
	}
	if !r.isNullOrUndefined(null) || r.isNullOrUndefined(ref) {
		return nil, false, nil
	}
	if !r.nullableRefAt(ref) {
		return nil, false, nil
	}
	e, err := r.lowerExpr(ref)
	if err != nil {
		return nil, false, err
	}
	return &ast.BinaryExpr{X: e, Op: goOp, Y: ident("nil")}, true, nil
}
