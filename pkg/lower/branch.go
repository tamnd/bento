package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the loop-control statements, break and continue, to their Go
// counterparts. An unlabeled break or continue targets the innermost enclosing
// loop, and a break additionally targets an enclosing switch, in both languages;
// Go resolves an unlabeled branch exactly the way JavaScript does, so each maps
// straight across with no scope bookkeeping here. A labeled form, break outer,
// carries its target as an identifier child and lowers to the Go branch that names
// the same label; the label the branch targets is the same one lowerLabeled emits
// on the loop, so the two spellings stay paired.

// lowerBranch lowers a break or continue to a Go branch statement, reporting false
// for any other unclassified node so the caller keeps its existing hand-back. The
// frontend does not name a break or continue node, so each surfaces as an
// unclassified node whose text is the keyword. A bare form maps to an unlabeled
// branch; a labeled form carries its target as a single identifier child and maps
// to a branch that names that label.
func (r *Renderer) lowerBranch(n frontend.Node) (ast.Stmt, bool) {
	if n.Kind() != frontend.NodeUnknown {
		return nil, false
	}
	txt := strings.TrimSpace(r.prog.Text(n))
	switch txt {
	case "break", "break;":
		return &ast.BranchStmt{Tok: token.BREAK}, true
	case "continue", "continue;":
		return &ast.BranchStmt{Tok: token.CONTINUE}, true
	}
	// A labeled break or continue, break outer, keeps its target as the lone
	// identifier child. The keyword is the first word of the text, so the token
	// follows from the prefix and the label follows from the child.
	kids := r.prog.Children(n)
	if len(kids) == 1 && kids[0].Kind() == frontend.NodeIdentifier {
		label := strings.TrimSpace(r.prog.Text(kids[0]))
		switch {
		case strings.HasPrefix(txt, "break"):
			return &ast.BranchStmt{Tok: token.BREAK, Label: ident(label)}, true
		case strings.HasPrefix(txt, "continue"):
			return &ast.BranchStmt{Tok: token.CONTINUE, Label: ident(label)}, true
		}
	}
	return nil, false
}
