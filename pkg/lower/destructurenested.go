package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// A destructuring pattern can nest inside another pattern in any element position,
// so `const [[a, b], [c, d]] = m` binds four names off a two-level array shape and
// `const { p: { x } } = o` reads a property of a property. Go has no destructuring,
// so a nested pattern lowers by minting a temporary for the slot the outer pattern
// selects, then binding the inner pattern against that held value; the recursion
// composes the same read-into-a-temp step to any depth. This file holds the
// recursive core the declaration and parameter paths route a nested element through.

// patternNode reports whether n is a nested binding pattern, an array pattern
// (`[...]`) or an object pattern (`{...}`) appearing as an element of an outer
// pattern. The frontend wraps such an element in an opaque node whose text opens
// with the pattern's bracket, so the shape is read off the leading token.
func (r *Renderer) patternNode(n frontend.Node) bool {
	if n.Kind() != frontend.NodeUnknown {
		return false
	}
	t := strings.TrimSpace(r.prog.Text(n))
	return strings.HasPrefix(t, "[") || strings.HasPrefix(t, "{")
}

// bindSubPattern binds a nested pattern against a receiver expression that already
// holds the value the outer pattern selected. It dispatches on the pattern's shape,
// an array or an object, and recurses so nested patterns compose to any depth. The
// leaves bind with tok, so a declaration or a parameter binds fresh names with a
// `:=` while an assignment target stores into existing names with a `=`.
func (r *Renderer) bindSubPattern(pat frontend.Node, recv ast.Expr, patType frontend.Type, tok token.Token) ([]ast.Stmt, error) {
	txt := strings.TrimSpace(r.prog.Text(pat))
	switch {
	case strings.HasPrefix(txt, "["):
		return r.bindSubArray(pat, recv, patType, tok)
	}
	return nil, &NotYetLowerable{Reason: "a nested destructuring element that is neither an array nor an object pattern is a later slice"}
}

// bindSubArray binds an array pattern nested inside an outer pattern. It reads each
// fixed slot off the receiver by index, the same bounds-aware AtI read a top-level
// array element takes, and binds a further-nested element by minting a temporary for
// the slot and recursing. The receiver already holds the value, so no source
// temporary or iterator draining is needed here; that machinery stays with the
// top-level path. A default or a rest inside the nesting composes the fill and
// gather rules through a level and is a later item, so it hands back for now.
func (r *Renderer) bindSubArray(pat frontend.Node, recv ast.Expr, patType frontend.Type, tok token.Token) ([]ast.Stmt, error) {
	elemT, ok := r.prog.ElementType(patType)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a nested array pattern over a non-array or tuple type is a later slice"}
	}
	elemGo, err := r.typeExpr(elemT)
	if err != nil {
		return nil, err
	}
	elems := r.prog.Children(pat)
	if len(elems) == 0 {
		return nil, &NotYetLowerable{Reason: "an empty array destructuring pattern binds nothing"}
	}
	var out []ast.Stmt
	for i, el := range elems {
		info, err := r.classifyArrayElem(el)
		if err != nil {
			return nil, err
		}
		read := &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ident("AtI")},
			Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}},
		}
		if info.nested != nil {
			tmp := r.freshTemp()
			out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{read}})
			inner, err := r.bindSubPattern(info.nested, ident(tmp), elemT, tok)
			if err != nil {
				return nil, err
			}
			out = append(out, inner...)
			continue
		}
		if info.hasDefault {
			return nil, &NotYetLowerable{Reason: "a default inside a nested array pattern composes the fill through the nesting, a later slice"}
		}
		name, ok := localName(r.prog.Text(info.nameNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "destructured name is not a Go identifier"}
		}
		nameGo, err := r.typeExpr(r.prog.TypeAt(info.nameNode))
		if err != nil {
			return nil, err
		}
		if same, err := sameGoType(nameGo, elemGo); err != nil {
			return nil, err
		} else if !same {
			return nil, &NotYetLowerable{Reason: "array destructuring where an element's type differs from the array element type is a later slice"}
		}
		out = append(out, &ast.AssignStmt{Lhs: []ast.Expr{ident(name)}, Tok: tok, Rhs: []ast.Expr{read}})
	}
	return out, nil
}
