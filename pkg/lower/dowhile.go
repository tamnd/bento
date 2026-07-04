package lower

import (
	"go/ast"
	"go/token"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers a do...while to the Go loop hand-written Go uses for the same
// shape, a bare for whose body runs once before the condition is tested. The
// frontend does not name a do node, so a do...while surfaces as an unclassified
// node whose text begins with do and whose children are the body block and the
// condition. The loop keeps running while the condition holds, so the lowering
// runs the body and then breaks once the condition no longer holds:
//
//	for {
//		body
//		if !cond {
//			break
//		}
//	}
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
	body.List = append(body.List, &ast.IfStmt{
		Cond: negateCond(cond),
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.BranchStmt{Tok: token.BREAK}}},
	})
	return &ast.ForStmt{Body: body}, true, nil
}

// negateCond returns the logical negation of a boolean condition. A comparison
// flips to its opposite, s < 3 to s >= 3, which reads the way hand-written Go names
// the break condition; a double negation cancels; anything else takes a leading !.
func negateCond(cond ast.Expr) ast.Expr {
	switch c := cond.(type) {
	case *ast.BinaryExpr:
		if inv, ok := invertComparison(c.Op); ok {
			return &ast.BinaryExpr{X: c.X, Op: inv, Y: c.Y}
		}
	case *ast.UnaryExpr:
		if c.Op == token.NOT {
			return c.X
		}
	}
	return &ast.UnaryExpr{Op: token.NOT, X: cond}
}

// invertComparison maps a comparison operator to its negation, reporting false for
// any operator that is not a comparison, where flipping would change the structure
// rather than negate it.
func invertComparison(op token.Token) (token.Token, bool) {
	switch op {
	case token.LSS:
		return token.GEQ, true
	case token.LEQ:
		return token.GTR, true
	case token.GTR:
		return token.LEQ, true
	case token.GEQ:
		return token.LSS, true
	case token.EQL:
		return token.NEQ, true
	case token.NEQ:
		return token.EQL, true
	default:
		return op, false
	}
}
