package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// An object rest gathers the own properties a pattern did not name, and a pattern can sit
// anywhere another pattern puts one. `const { o: { a, ...inner } } = deep` nests one a
// level down, and `for (const { a, ...r } of rows)` binds one against each element of the
// loop, which is the same nesting: both bind a sub-pattern against a receiver that already
// holds the value, through bindSubObject.
//
// bindSubObject had no rest arm, so a rest element fell to classifyObjectElem, which
// refuses one outright:
//
//	an object destructuring rest property gathers the remaining own properties into an
//	object, which needs the object model of phase 7
//
// That reason was true of the whole capability once and is no longer: a top-level
// `const { a, ...rest } = o` gathers a fixed-shape rest into its own interned struct, and
// an empty one into the runtime object. A nested rest is that same gather off a receiver
// the outer level already selected, so it needs nothing the top level does not have. The
// array sibling was already there, since bindSubArray splits a trailing rest and calls
// arrayRestBinding.
//
// So the gather moves into one helper both levels call, and the nested object paths get
// their rest arm. Neither of them mints a source temp, since the receiver already holds
// the value, so the classify and emit halves the top-level path keeps apart are one step
// here.

// objectRestBinding builds the statements that bind an object pattern's rest target
// against a receiver already holding the source value: the gather, and the blank an
// unread name needs. The tok selects `:=` for a declaration or a loop binding and `=` for
// an assignment into a target that already exists, so the nested declaration and
// assignment paths share it. what names the form in a hand-back reason.
func (r *Renderer) objectRestBinding(restIdent frontend.Node, recv ast.Expr, tok token.Token, what string) ([]ast.Stmt, error) {
	name, ok := localName(r.prog.Text(restIdent))
	if !ok {
		return nil, &NotYetLowerable{Reason: "an object " + what + " rest target is not a Go identifier"}
	}
	restType := r.prog.TypeAt(restIdent)
	if restType.Flags&frontend.TypeObject == 0 {
		return nil, &NotYetLowerable{Reason: "an object " + what + " rest whose type is not a fixed-shape object is a later slice"}
	}
	if _, isArray := r.prog.ElementType(restType); isArray {
		return nil, &NotYetLowerable{Reason: "an object " + what + " rest typed as an array is a later slice"}
	}
	structName, err := r.restGatherStruct(restType)
	if err != nil {
		return nil, err
	}
	gather, err := r.restGatherExpr(restType, structName, recv, what)
	if err != nil {
		return nil, err
	}
	out := []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: tok, Rhs: []ast.Expr{gather}}}
	// A `:=` gather nothing reads would trip Go's declared-and-unused rule, and gathering
	// a rest only to discard it is an ordinary way to write `everything but a`. An
	// assignment gather writes a target that already exists, so it needs no blank.
	if tok == token.DEFINE {
		out = r.blankUnusedParamBinding(out, restIdent, name)
	}
	return out, nil
}
