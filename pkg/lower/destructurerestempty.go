package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// A destructuring rest gathers what the pattern did not name, and sometimes that is
// nothing. `const { a, ...rest } = { a: 1 }` leaves rest the empty object type, and the
// empty object type is not a fixed shape anywhere else in the lowerer: it is the
// structural top type, so typeExpr gives it a value.Value slot and isDynamic routes every
// read of it through the runtime, which is why `const e = {}` builds a value.NewObject
// and answers Object.keys(e).length with 0.
//
// The rest gather did not consult that rule. It interned the shape whatever it held and
// copied field by field, so an empty one bound a struct that has no fields and no methods
// while every read of the name expected the box:
//
//	./main.go:17:100: rest.OwnEnumerableKeys undefined (type *ObjEmpty has no field or
//	method OwnEnumerableKeys)
//
// A leaf a function reads showed the same disagreement from the other side, since the
// package var it hoists to is spelled by typeExpr:
//
//	cannot use &ObjEmpty{} (value of type *ObjEmpty) as value.Value value in assignment
//
// So the gather asks isObjectTopType the way typeExpr and isDynamic do, and an empty
// rest builds the same runtime object the empty literal builds. The declaration form and
// the assignment form share these two, so both answer alike.

// restGatherStruct reports the interned struct a rest gathers into, or the empty string
// when the rest is the empty object type and gathers into a runtime object instead. It is
// the classify-time half: interning is what refuses a shape that does not lower, and an
// empty shape has nothing to intern.
func (r *Renderer) restGatherStruct(restType frontend.Type) (string, error) {
	if r.isObjectTopType(restType) {
		return "", nil
	}
	return r.decls.internStruct(r, restType)
}

// restGatherExpr builds the value a rest element binds: the struct literal copying each
// remaining field off the receiver, or value.NewObject() for a rest with nothing left in
// it. An empty gather reads no field, so it does not touch the receiver at all, which is
// what leaves the source temp for blankUnusedRecvTemp to blank.
func (r *Renderer) restGatherExpr(restType frontend.Type, structName string, recv ast.Expr, what string) (ast.Expr, error) {
	if structName == "" {
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "NewObject")}, nil
	}
	props := r.prog.Properties(restType)
	elts := make([]ast.Expr, 0, len(props))
	for _, pr := range props {
		field, ok := exportedField(pr.Name)
		if !ok {
			return nil, &NotYetLowerable{Reason: "an object " + what + " rest property is not a Go field name"}
		}
		elts = append(elts, &ast.KeyValueExpr{Key: ident(field), Value: &ast.SelectorExpr{X: recv, Sel: ident(field)}})
	}
	return &ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(structName), Elts: elts}}, nil
}
