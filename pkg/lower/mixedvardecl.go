package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// A variable statement declares one binding per comma, and a destructuring pattern is
// one of those bindings like any other. `const [p, q] = arr, extra = 9` binds p, q and
// extra, in that order, and each of them is an ordinary binding of the enclosing scope.
//
// The destructuring lowering never saw such a statement. Both its entry points take the
// statement whole and report no interest unless it holds exactly one declaration, so a
// pattern beside a plain name fell to the plain declaration path, which spells a binding
// by asking localName for a Go identifier. A pattern's text is `[p, q]`, and localName
// mangles that into an identifier rather than refusing it, so the emitted Go declared
// U5B_pU2C_U20_qU5D_ and nothing named p:
//
//	./main.go:10:3: declared and not used: U5B_pU2C_U20_qU5D_
//	./main.go:13:41: undefined: p
//
// With a function reading one of the names it did not even get that far. The hoist gives
// every name of such a statement a package var, the plain declarator then lowers as an
// assignment into its own, and the pattern declarator does not, so the statement looked
// like a mix of a redeclared name and a new one and handed back.
//
// This file is the statement's own lowering: declaration by declaration in source order,
// each one through the path that already knows how to lower it.

// flattenMixedDeclarators lowers a variable statement that declares a destructuring
// pattern alongside another declaration, taking each declaration on its own and
// concatenating the results in source order. That order is the evaluation order the
// statement already has, so a later declaration reading a name an earlier one bound,
// `const [p, q] = arr, sum = p + q`, reads it after it is bound.
//
// A pattern goes through the same per-declaration destructure cores a single-declaration
// statement and a for loop's initializer go through, so every shape they lower it lowers
// here and every shape they hand back on it hands back on here. A plain declaration goes
// through varDeclStmt as a statement of one, which is what makes the hoisted case work:
// a name whose package var this statement assigns into is the whole of its own group, so
// it is a redeclaration of all of it rather than a mix.
//
// It reports ok=false for a statement with one declaration, which the callers above it
// already lower, and for one with no pattern at all, which the plain path lowers.
func (r *Renderer) flattenMixedDeclarators(n frontend.Node, hoisted map[string]bool) ([]ast.Stmt, bool, error) {
	if n.Kind() != frontend.NodeVariableStatement {
		return nil, false, nil
	}
	var decls []frontend.Node
	collectVarDecls(r.prog, n, &decls)
	if len(decls) < 2 {
		return nil, false, nil
	}
	pattern := false
	for _, d := range decls {
		if r.declIsPattern(d) {
			pattern = true
			break
		}
	}
	if !pattern {
		return nil, false, nil
	}
	// A `using` or `await using` declaration's scope-exit disposal is a later slice
	// wherever the declaration sits. The single-declaration path refuses it inside
	// lowerVarStatement, which this path does not run, so the test is repeated here
	// rather than left to a declaration that would lower as a plain binding and drop
	// the dispose call.
	if kw, ok := r.usingKeyword(n); ok {
		return nil, true, &NotYetLowerable{Reason: "the " + kw + " declaration's scope-exit disposal is a later slice"}
	}
	isVar := r.isVarStatement(n)
	// Which plain names this statement is the first Go declaration of, read before any
	// of it lowers, for the same reason the single-statement path reads it first.
	fresh := r.freshBindingMask(decls, isVar)
	var out []ast.Stmt
	for _, d := range decls {
		if !r.declIsPattern(d) {
			s, err := r.varDeclStmt([]frontend.Node{d}, isVar)
			if err != nil {
				return nil, true, err
			}
			out = append(out, s)
			continue
		}
		stmts, err := r.patternDeclStmts(d, hoisted)
		if err != nil {
			return nil, true, err
		}
		out = append(out, stmts...)
	}
	return append(out, r.unusedBindingBlanks(decls, fresh)...), true, nil
}

// patternDeclStmts lowers one destructuring declaration of a variable statement, an
// array pattern or an object one, and retargets its binds at the package vars its leaves
// hoisted to when they did. It is the per-declaration half of what flattenArrayDestructure
// and flattenObjectDestructure do for a statement holding nothing else.
//
// The declaration is a pattern, so a shape neither core recognizes is a hand-back rather
// than a fall-through to the plain path, which would spell the pattern as a mangled Go
// identifier and emit a program that does not build.
func (r *Renderer) patternDeclStmts(d frontend.Node, hoisted map[string]bool) ([]ast.Stmt, error) {
	if stmts, ok, err := r.arrayDestructureDecl(d); err != nil {
		return nil, err
	} else if ok {
		return storeIntoHoistedPattern(stmts, hoisted)
	}
	if stmts, ok, err := r.objectDestructureDecl(d); err != nil {
		return nil, err
	} else if ok {
		return storeIntoHoistedPattern(stmts, hoisted)
	}
	return nil, &NotYetLowerable{Reason: "a destructuring declaration beside another binding has a pattern shape neither destructure path lowers"}
}
