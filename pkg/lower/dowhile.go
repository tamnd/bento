package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers a do...while to the Go loop hand-written Go uses for the same
// shape, a for whose body runs once before the condition is tested. The frontend
// does not name a do node, so a do...while surfaces as an unclassified node whose
// text begins with do and whose children are the body block and the condition. The
// condition rides the for's post clause guarded by a flag seeded true, so the body
// runs once and then the loop re-tests the condition each time round:
//
//	for _bt0 := true; _bt0; _bt0 = cond {
//		body
//	}
//
// The post clause is the seam that makes continue correct. A continue inside the
// body jumps to the post, which re-evaluates the condition, so a do...while(false)
// with a continue in its body exits after one turn rather than spinning. A trailing
// `if !cond { break }` would sit past the continue and never run, which is the
// infinite loop this shape avoids.
//
// A do...while whose condition is not boolean, a truthy number for instance, hands
// back with the same reason lowerCondition reports for a while, so the two loops
// stay consistent on what they can lower.

// lowerDoWhile lowers a do...while, reporting false for any other unclassified node.
// The guard is the two children, a block first, with the text beginning at do, so a
// stray two-child node does not match.
func (r *Renderer) lowerDoWhile(n frontend.Node) (ast.Stmt, bool, error) {
	kids := r.prog.Children(n)
	if len(kids) != 2 || kids[0].Kind() != frontend.NodeBlock {
		return nil, false, nil
	}
	if !strings.HasPrefix(strings.TrimSpace(r.prog.Text(n)), "do") {
		return nil, false, nil
	}
	body, err := r.lowerBlock(kids[0])
	if err != nil {
		return nil, false, err
	}
	cond, err := r.lowerCondition(kids[1])
	if err != nil {
		return nil, false, err
	}
	flag := r.freshTemp()
	return &ast.ForStmt{
		Init: &ast.AssignStmt{Lhs: []ast.Expr{ident(flag)}, Tok: token.DEFINE, Rhs: []ast.Expr{ident("true")}},
		Cond: ident(flag),
		Post: &ast.AssignStmt{Lhs: []ast.Expr{ident(flag)}, Tok: token.ASSIGN, Rhs: []ast.Expr{cond}},
		Body: body,
	}, true, nil
}
