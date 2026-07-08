package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"unicode/utf8"

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
	// A switch over a string compares by code unit, so it lowers to a Go switch
	// on the UTF-8 view with each string-literal label spelled as a Go string.
	// The two agree exactly when every label transcodes cleanly, which the label
	// guard checks; the discriminant itself needs no purity guard because a Go
	// switch evaluates its tag once. The string test rides condBranchType so a
	// typeof discriminant and a ternary over strings, the assert._toString
	// shape, count too: both lower to a BStr even where the checker's type on
	// the node itself does not say string.
	if _, kind, ok := r.condBranchType(disc); ok && kind == "string" {
		return r.lowerStringSwitch(disc, caseBlock)
	}
	// A switch over a boolean is the case-chain idiom, switch (true) { case a: case
	// b: }, where each label is a boolean test and the first true one runs. JavaScript
	// matches a label with strict equality, disc === label, and for two booleans that
	// is Go's ==, which is exactly what a Go switch on a bool tag computes, so the
	// discriminant lowers to a Go bool tag and each label to a Go bool expression. The
	// bool test rides condBranchType so a typeof-style boolean and a ternary over
	// booleans count too.
	if _, kind, ok := r.condBranchType(disc); ok && kind == "bool" {
		return r.lowerBooleanSwitch(disc, caseBlock)
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

// lowerStringSwitch lowers a switch over a string discriminant to a Go switch on
// the discriminant's UTF-8 view, disc.ToGoString(), with each case label spelled
// as a Go string literal. Code-unit equality and Go string equality agree here
// because a label that survives the guard transcodes to UTF-8 losslessly: only a
// lone surrogate transcodes lossily, into U+FFFD, so a label carrying U+FFFD is
// the one spelling a lossy discriminant could falsely match, and it hands back.
func (r *Renderer) lowerStringSwitch(disc, caseBlock frontend.Node) (ast.Stmt, error) {
	tag, err := r.lowerExpr(disc)
	if err != nil {
		return nil, err
	}
	clauses := r.prog.Children(caseBlock)
	body, err := r.switchClauses(clauses, func(e frontend.Node) (ast.Expr, error) {
		lit, ok := r.stringLiteralValue(e)
		if !ok {
			return nil, &NotYetLowerable{Reason: "a string switch case label that is not a string literal is a later slice"}
		}
		if strings.ContainsRune(lit, utf8.RuneError) {
			return nil, &NotYetLowerable{Reason: "a string switch case label carrying a replacement character cannot compare by code unit"}
		}
		return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit)}, nil
	})
	if err != nil {
		return nil, err
	}
	goTag := &ast.CallExpr{Fun: &ast.SelectorExpr{X: tag, Sel: ident("ToGoString")}}
	return &ast.SwitchStmt{Tag: goTag, Body: body}, nil
}

// lowerBooleanSwitch lowers a switch over a boolean discriminant to a Go switch on
// the discriminant with each case label spelled as a Go bool expression. The tag
// needs no conversion because a boolean already lowers to a Go bool, and boolean
// strict equality is Go's ==, so switch tag { case label: } matches exactly where
// disc === label does. A case label that is not itself a boolean hands back: a
// number or string label can never strictly equal a boolean, so it would only match
// in JavaScript by never matching, and Go would reject the mixed-type comparison.
func (r *Renderer) lowerBooleanSwitch(disc, caseBlock frontend.Node) (ast.Stmt, error) {
	tag, err := r.lowerExpr(disc)
	if err != nil {
		return nil, err
	}
	clauses := r.prog.Children(caseBlock)
	body, err := r.switchClauses(clauses, func(e frontend.Node) (ast.Expr, error) {
		if !r.isBool(e) {
			return nil, &NotYetLowerable{Reason: "a boolean switch case label that is not a boolean is a later slice"}
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
	covered := map[string]bool{}
	body, err := r.switchClauses(clauses, func(e frontend.Node) (ast.Expr, error) {
		lit, ok := r.stringLiteralValue(e)
		if !ok {
			return nil, &NotYetLowerable{Reason: "a discriminant switch case label that is not a string literal is a later slice"}
		}
		arm, ok := info.armByDisc(lit)
		if !ok {
			return nil, &NotYetLowerable{Reason: "a discriminant switch case naming no union arm is a later slice"}
		}
		covered[lit] = true
		return ident(info.tagConst(arm)), nil
	})
	if err != nil {
		return nil, err
	}
	// An exhaustive switch, one with a case for every arm and no default, is a
	// terminating statement in TypeScript: the checker types the fall-out as never, so
	// a function may end in it with no trailing return. Go does not treat a
	// default-less switch as terminating, so a Go default that panics is synthesized to
	// carry that guarantee across; it is unreachable in well-typed code and only fires
	// on a corrupted tag. A switch that already has a default, or that leaves an arm
	// uncovered, keeps its own fall-out to the code after it.
	if !r.hasDefaultClause(clauses) && len(covered) == len(info.arms) {
		body.List = append(body.List, &ast.CaseClause{Body: []ast.Stmt{unreachablePanic()}})
	}
	tag := &ast.SelectorExpr{X: ident(name), Sel: ident("tag")}
	return &ast.SwitchStmt{Tag: tag, Body: body}, nil
}

// hasDefaultClause reports whether any clause of a case block is the default clause,
// so an exhaustive switch that already spells its own fall-out is left alone rather
// than given a second default.
func (r *Renderer) hasDefaultClause(clauses []frontend.Node) bool {
	for _, c := range clauses {
		if r.isDefaultClause(c) {
			return true
		}
	}
	return false
}

// fallthroughStmt builds the explicit fallthrough a case whose body runs off its
// end takes, the Go spelling of JavaScript's implicit fall-through.
func fallthroughStmt() ast.Stmt {
	return &ast.BranchStmt{Tok: token.FALLTHROUGH}
}

// unreachablePanic builds the panic statement of a synthesized exhaustive-switch
// default, the marker that a tag outside the union's arms reached code the checker
// proved unreachable.
func unreachablePanic() ast.Stmt {
	return &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  ident("panic"),
		Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `"unreachable"`}},
	}}
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
			// A default clause cannot carry the labels of empty cases that precede
			// it, so those labels become their own Go case whose body is a single
			// fallthrough: the label matches, transfers into the default body, and
			// no label re-evaluates, exactly the JavaScript share.
			if len(pending) > 0 {
				body.List = append(body.List, &ast.CaseClause{List: pending, Body: []ast.Stmt{fallthroughStmt()}})
				pending = nil
			}
		} else {
			// An empty case with no body of its own shares the next clause's body, so its
			// label is held and merged forward. As the last clause it has nothing to fall
			// into, so it stays an empty Go case that matches and does nothing.
			if len(stmtNodes) == 0 && !isLast {
				pending = append(pending, labels...)
				continue
			}
		}

		lowered, err := r.lowerStatements(stmtNodes)
		if err != nil {
			return nil, err
		}
		// A body that can run off its end into the following clause is genuine
		// fall-through, which Go spells with an explicit trailing fallthrough:
		// control enters the next clause's body without testing its label, the
		// same transfer JavaScript makes. The last clause has nothing to fall
		// into in either language, so it keeps its plain fall-out.
		if !terminated && !isLast {
			lowered = append(lowered, fallthroughStmt())
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
