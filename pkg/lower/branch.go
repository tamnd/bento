package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the bare loop-control statements, break and continue, to their
// Go counterparts. An unlabeled break or continue targets the innermost enclosing
// loop, and a break additionally targets an enclosing switch, in both languages;
// Go resolves an unlabeled branch exactly the way JavaScript does, so each maps
// straight across with no scope bookkeeping here. A labeled form, break outer,
// carries a target and is a later slice, so it does not match and hands back.

// lowerBranch lowers a bare break or continue to a Go branch statement, reporting
// false for any other unclassified node so the caller keeps its existing hand-back.
// The frontend does not name a break or continue node, so each surfaces as an
// unclassified node whose text is the keyword; a labeled form carries its target in
// the text and does not match, staying deferred rather than lowering to an
// unlabeled branch that would target the wrong loop.
func (r *Renderer) lowerBranch(n frontend.Node) (ast.Stmt, bool) {
	if n.Kind() != frontend.NodeUnknown {
		return nil, false
	}
	switch strings.TrimSpace(r.prog.Text(n)) {
	case "break", "break;":
		return &ast.BranchStmt{Tok: token.BREAK}, true
	case "continue", "continue;":
		return &ast.BranchStmt{Tok: token.CONTINUE}, true
	default:
		return nil, false
	}
}
