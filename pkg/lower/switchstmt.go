package lower

import (
	"go/ast"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers a switch statement to a Go switch. JavaScript and Go switch
// differ in one fundamental way: a JavaScript case falls through into the next
// unless it breaks, while a Go case never falls through. This lowering targets the
// break-terminated subset, where the two line up exactly. A case whose body ends
// in break (or returns, or throws) becomes a Go case with the break dropped, since
// Go breaks for it. A run of empty cases that share the following body merges into
// one Go case with several expressions, the "case 2: case 3: body" share form. A
// body that runs off its end into the next case is genuine fall-through, a
// different control shape, and hands the unit back rather than emit a Go switch
// that would silently break where the source meant to continue.

// lowerSwitch lowers a switch over a number, which includes a numeric enum since an
// enum value is a float64, to a Go switch on the same value. The discriminant must
// be a number so the tag and the case constants compare as Go float64s; a string,
// boolean, or dynamic discriminant compares differently and is a later slice, as is
// a case label that is not a number.
func (r *Renderer) lowerSwitch(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 2 {
		return nil, &NotYetLowerable{Reason: "switch statement did not expose a discriminant and a case block"}
	}
	disc, caseBlock := kids[0], kids[1]
	// A switch over the discriminant of an object union, switch (s.kind), lowers to a
	// Go switch on the tag, switch s.tag, with each string-literal case label mapped
	// to the arm's tag constant: the same one-integer narrowing the if form takes,
	// extended to the multi-way shape (tagunion.go). Inside each case the union local
	// is narrowed to that arm, so a read of it selects the arm's field.
	if name, info, ok := r.discriminantRead(disc); ok {
		return r.lowerDiscriminantSwitch(name, info, caseBlock)
	}
	if !r.isNumber(disc) {
		return nil, &NotYetLowerable{Reason: "a switch on a non-number discriminant is a later slice"}
	}
	tag, err := r.lowerExpr(disc)
	if err != nil {
		return nil, err
	}

	clauses := r.prog.Children(caseBlock)
	body, err := r.switchClauses(clauses, func(e frontend.Node) (ast.Expr, error) {
		if !r.isNumber(e) {
			return nil, &NotYetLowerable{Reason: "a switch case label that is not a number is a later slice"}
		}
		return r.lowerExpr(e)
	})
	if err != nil {
		return nil, err
	}
	return &ast.SwitchStmt{Tag: tag, Body: body}, nil
}

// lowerDiscriminantSwitch lowers a switch over an object union's discriminant to a
// Go switch on the tag, mapping each string-literal case label to the arm's tag
// constant. A case label that is not a string literal, or names no arm, hands back.
func (r *Renderer) lowerDiscriminantSwitch(name string, info *unionInfo, caseBlock frontend.Node) (ast.Stmt, error) {
	clauses := r.prog.Children(caseBlock)
	body, err := r.switchClauses(clauses, func(e frontend.Node) (ast.Expr, error) {
		lit, ok := r.stringLiteralValue(e)
		if !ok {
			return nil, &NotYetLowerable{Reason: "a discriminant switch case label that is not a string literal is a later slice"}
		}
		arm, ok := info.armByDisc(lit)
		if !ok {
			return nil, &NotYetLowerable{Reason: "a discriminant switch case naming no union arm is a later slice"}
		}
		return ident(info.tagConst(arm)), nil
	})
	if err != nil {
		return nil, err
	}
	tag := &ast.SelectorExpr{X: ident(name), Sel: ident("tag")}
	return &ast.SwitchStmt{Tag: tag, Body: body}, nil
}

// switchClauses builds the Go case-clause list shared by the numeric and
// discriminant switch lowerings, applying lowerLabel to each case label so the
// caller controls whether a label is a number, a tag constant, or another form. It
// carries the JavaScript-to-Go fall-through rules: a break-terminated case drops its
// break, a run of empty cases merges its labels into the next body, and a body that
// runs off its end into the next case hands back.
func (r *Renderer) switchClauses(clauses []frontend.Node, lowerLabel func(frontend.Node) (ast.Expr, error)) (*ast.BlockStmt, error) {
	body := &ast.BlockStmt{}
	// pending holds the case labels of empty case clauses waiting to merge into the
	// next clause that carries a body, the JavaScript "case 2: case 3: body" share
	// form, which Go writes as one case with several expressions.
	var pending []ast.Expr
	for i, clause := range clauses {
		isDefault := r.isDefaultClause(clause)
		exprNodes, stmtNodes := r.splitCaseClause(clause, isDefault)
		isLast := i == len(clauses)-1

		var labels []ast.Expr
		for _, e := range exprNodes {
			label, err := lowerLabel(e)
			if err != nil {
				return nil, err
			}
			labels = append(labels, label)
		}

		stmtNodes, brokeOut := r.stripTrailingBreak(stmtNodes)
		terminated := brokeOut || r.bodyTerminates(stmtNodes)

		if isDefault {
			// A default that runs off its end into a following clause is fall-through,
			// which includes an empty default in the middle, so a non-terminated default
			// that is not last hands back. A default cannot carry the labels of empty
			// cases that precede it, so a fall-through from a case into default does too.
			if !terminated && !isLast {
				return nil, &NotYetLowerable{Reason: "a switch default that falls through into the next case is a later slice"}
			}
			if len(pending) > 0 {
				return nil, &NotYetLowerable{Reason: "a switch case that falls through into default is a later slice"}
			}
		} else {
			// An empty case with no body of its own shares the next clause's body, so its
			// label is held and merged forward. As the last clause it has nothing to fall
			// into, so it stays an empty Go case that matches and does nothing.
			if len(stmtNodes) == 0 && !isLast {
				pending = append(pending, labels...)
				continue
			}
			if len(stmtNodes) > 0 && !terminated && !isLast {
				return nil, &NotYetLowerable{Reason: "a switch case that falls through into the next is a later slice"}
			}
		}

		lowered, err := r.lowerStatements(stmtNodes)
		if err != nil {
			return nil, err
		}
		clauseOut := &ast.CaseClause{Body: lowered}
		if !isDefault {
			clauseOut.List = append(pending, labels...)
		}
		pending = nil
		body.List = append(body.List, clauseOut)
	}
	// Empty cases trailing the last body have no clause to merge into, so they become
	// one empty Go case that matches and does nothing, the JavaScript behavior.
	if len(pending) > 0 {
		body.List = append(body.List, &ast.CaseClause{List: pending})
	}
	return body, nil
}

// isDefaultClause reports whether a clause of a case block is the default clause,
// read from its leading keyword. A case clause carries a label expression before
// its body; the default clause carries only a body, so the two are told apart by
// the keyword the clause opens with.
func (r *Renderer) isDefaultClause(clause frontend.Node) bool {
	return strings.HasPrefix(strings.TrimSpace(r.prog.Text(clause)), "default")
}

// splitCaseClause separates a clause into its label expressions and its body
// statements. A case clause opens with a single label expression, its first child,
// and the rest are the body; a default clause has no label, so every child is a
// body statement.
func (r *Renderer) splitCaseClause(clause frontend.Node, isDefault bool) (exprs, stmts []frontend.Node) {
	kids := r.prog.Children(clause)
	if isDefault {
		return nil, kids
	}
	if len(kids) == 0 {
		return nil, nil
	}
	return kids[:1], kids[1:]
}

// stripTrailingBreak drops a case body's trailing break, the JavaScript terminator
// a Go case does not need, and reports whether it dropped one. Only the top-level
// break is dropped, which is where the idiomatic case terminator sits; a break
// nested inside a block stays where it is and hands back through the statement
// lowering rather than be silently removed.
func (r *Renderer) stripTrailingBreak(stmts []frontend.Node) ([]frontend.Node, bool) {
	if len(stmts) == 0 {
		return stmts, false
	}
	if r.isBreakStatement(stmts[len(stmts)-1]) {
		return stmts[:len(stmts)-1], true
	}
	return stmts, false
}

// bodyTerminates reports whether a case body cannot run off its end, so control
// cannot fall through into the following clause. A trailing return or throw
// terminates, and so does a trailing block whose own last statement terminates,
// which is the "case N: { ...; return x; }" braced form. A trailing break or
// continue terminates too: a break leaves the switch and a continue jumps to the
// enclosing loop, so neither reaches the next clause (a trailing break is normally
// stripped before this, but a continue is not, so this is what keeps a
// "case N: continue;" from reading as a fall-through). Anything else can reach the
// end, so the caller treats it as a fall-through when a clause follows.
func (r *Renderer) bodyTerminates(stmts []frontend.Node) bool {
	if len(stmts) == 0 {
		return false
	}
	last := stmts[len(stmts)-1]
	if _, ok := r.lowerBranch(last); ok {
		return true
	}
	switch last.Kind() {
	case frontend.NodeReturnStatement, frontend.NodeThrowStatement:
		return true
	case frontend.NodeBlock:
		return r.bodyTerminates(r.prog.Children(last))
	default:
		return false
	}
}

// isBreakStatement reports whether a statement is a bare break. The frontend does
// not name a break node, so it surfaces as an unclassified node whose text is the
// keyword; a labeled break carries a target and does not match, so it stays
// unlowered rather than be mistaken for the plain switch terminator.
func (r *Renderer) isBreakStatement(n frontend.Node) bool {
	if n.Kind() != frontend.NodeUnknown {
		return false
	}
	text := strings.TrimSpace(r.prog.Text(n))
	return text == "break;" || text == "break"
}
