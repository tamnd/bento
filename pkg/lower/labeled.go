package lower

import (
	"go/ast"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers a labeled statement, outer: for (...) { ... }, to its Go
// counterpart. The frontend does not name a labeled statement, so it surfaces as an
// unclassified node whose children are the label identifier and the statement it
// labels. Go spells the same construct with the label ahead of the statement, and a
// labeled break or continue inside the body targets it exactly the way JavaScript
// does, so the label maps straight across.
//
// The one difference is that Go rejects a label the body never targets while
// JavaScript accepts it. A label with no matching break or continue has no effect
// in either language, so the lowering keeps the label only when the body targets
// it and otherwise emits the bare statement.

// lowerLabeled lowers a labeled statement, reporting false for any other
// unclassified node. The guard is the two children, an identifier first, with the
// text beginning at the label followed by a colon.
func (r *Renderer) lowerLabeled(n frontend.Node) (ast.Stmt, bool, error) {
	kids := r.prog.Children(n)
	if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
		return nil, false, nil
	}
	label := strings.TrimSpace(r.prog.Text(kids[0]))
	if !strings.HasPrefix(strings.TrimSpace(r.prog.Text(n)), label+":") {
		return nil, false, nil
	}
	stmt, err := r.lowerStatement(kids[1])
	if err != nil {
		return nil, false, err
	}
	if !r.labelTargeted(kids[1], label) {
		return stmt, true, nil
	}
	return &ast.LabeledStmt{Label: ident(label), Stmt: stmt}, true, nil
}

// labelTargeted reports whether the subtree holds a labeled break or continue that
// names label, so the caller keeps a label Go would otherwise reject as unused. A
// labeled branch surfaces as an unclassified node with a single identifier child,
// the target, and text beginning at break or continue.
func (r *Renderer) labelTargeted(n frontend.Node, label string) bool {
	if n.Kind() == frontend.NodeUnknown {
		kids := r.prog.Children(n)
		if len(kids) == 1 && kids[0].Kind() == frontend.NodeIdentifier &&
			strings.TrimSpace(r.prog.Text(kids[0])) == label {
			txt := strings.TrimSpace(r.prog.Text(n))
			if strings.HasPrefix(txt, "break") || strings.HasPrefix(txt, "continue") {
				return true
			}
		}
	}
	for _, k := range r.prog.Children(n) {
		if r.labelTargeted(k, label) {
			return true
		}
	}
	return false
}
