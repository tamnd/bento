package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// A rest element gathers the elements a pattern's fixed slots did not take into a
// fresh array. JavaScript's array rest copies the tail past the last named slot, so
// this file lowers it to Slice from that slot, the same tail copy the array model's
// Slice method makes. The rest must be the last element, and its target must be an
// array whose element type matches the source's, since a heterogeneous tail is a
// later slice. The declaration, assignment, and parameter forms share this shape.

// arrayRestElem returns the identifier a trailing array-pattern rest element binds,
// `...rest`, and whether the element is a rest at all. A rest reads by its leading
// `...` token, so the element text is checked for the spread prefix; the bound name
// is the element's identifier child. A rest of a nested pattern, `...[x, y]`, has no
// identifier child, so it reports false and hands back through the ordinary path.
func (r *Renderer) arrayRestElem(el frontend.Node) (frontend.Node, bool) {
	if !strings.HasPrefix(strings.TrimSpace(r.prog.Text(el)), "...") {
		return nil, false
	}
	for _, c := range r.prog.Children(el) {
		if c.Kind() == frontend.NodeIdentifier {
			return c, true
		}
	}
	return nil, false
}

// splitArrayRest separates a pattern's trailing rest element from the fixed elements
// before it. It returns the fixed elements, the rest's bound identifier, and whether
// a rest is present. A rest anywhere but the last position is invalid JavaScript, so
// a rest among the fixed elements is reported through err, handing the unit back
// rather than mislowering.
func (r *Renderer) splitArrayRest(elems []frontend.Node) (fixed []frontend.Node, restNode frontend.Node, hasRest bool, err error) {
	fixed = elems
	if len(elems) > 0 {
		if n, ok := r.arrayRestElem(elems[len(elems)-1]); ok {
			restNode, hasRest = n, true
			fixed = elems[:len(elems)-1]
		}
	}
	for _, el := range fixed {
		if _, ok := r.arrayRestElem(el); ok {
			return nil, nil, false, &NotYetLowerable{Reason: "an array destructuring rest element must be the last element"}
		}
	}
	return fixed, restNode, hasRest, nil
}

// arrayRestBinding builds the statement that binds a rest target to the tail of the
// source past the fixed slots, `rest := recv.Slice(from)`. The rest target must be
// an array whose element type matches the source element type, the same match a
// fixed element needs; a heterogeneous tail, whose Slice would not carry the target's
// declared element type, hands back. The tok selects `:=` for a declaration or `=`
// for an assignment into an already-declared target.
func (r *Renderer) arrayRestBinding(restNode frontend.Node, elemT frontend.Type, recv ast.Expr, from int, tok token.Token) (ast.Stmt, error) {
	name, ok := localName(r.prog.Text(restNode))
	if !ok {
		return nil, &NotYetLowerable{Reason: "an array destructuring rest name is not a Go identifier"}
	}
	restGo, err := r.typeExpr(r.prog.TypeAt(restNode))
	if err != nil {
		return nil, err
	}
	arrGo, err := r.renderArray(elemT)
	if err != nil {
		return nil, err
	}
	if same, err := sameGoType(restGo, arrGo); err != nil {
		return nil, err
	} else if !same {
		return nil, &NotYetLowerable{Reason: "an array destructuring rest whose element type differs from the source element type is a later slice"}
	}
	return &ast.AssignStmt{
		Lhs: []ast.Expr{ident(name)},
		Tok: tok,
		Rhs: []ast.Expr{&ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ident("Slice")},
			Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(from)}},
		}},
	}, nil
}
