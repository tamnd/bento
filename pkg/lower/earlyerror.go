package lower

import (
	"fmt"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file catches one JavaScript early error the TypeScript checker does not
// report, so bento rejects the program up front instead of lowering it to Go
// that does not compile. The case is the ES2015 Block early error (spec 13.2.1
// Static Semantics: Early Errors): it is a Syntax Error if any element of the
// LexicallyDeclaredNames of a block's StatementList also occurs in that block's
// VarDeclaredNames. A block-level FunctionDeclaration is a LexicallyDeclaredName
// of the block, and a `var` in the same block (or in a nested plain block that
// is not a function boundary) is one of its VarDeclaredNames, so the two names
// collide and node throws SyntaxError at parse time. TypeScript permits the
// function/var merge and reports nothing, so without this check bento's front
// end passes the program to the lowerer, which emits `f := func(){}; var f ...`
// into one Go block and the Go toolchain rejects it as a redeclaration, turning
// a program the language rejects into a build that does not link.
//
// The check is deliberately narrow: it covers only the function-decl-vs-var
// collision the two block-scope/syntax/redeclaration cases exercise, not the
// full LexicallyDeclaredNames matrix (let/const/class shadowing, duplicate
// lexical names, Annex-B sloppy-mode subtleties). Widening it risks rejecting a
// program node accepts, so the other redeclaration shapes are left to hand back
// (safe) until their own slice. The detection is on AST shape alone, so it is
// mode-independent and covers a case run under both strict and sloppy
// composition.

// CheckBlockScopeEarlyErrors walks the given source roots for the block-scoped
// function-declaration-versus-var collision and returns a plain error naming the
// clashing binding on the first one it finds, or nil when none is present. The
// error is intentionally not a *NotYetLowerable: it is a real early-error
// rejection the build surfaces the way it surfaces a checker diagnostic, so a
// parse-phase negative test scores a pass rather than a handback.
func (r *Renderer) CheckBlockScopeEarlyErrors(roots ...frontend.Node) error {
	for _, root := range roots {
		if root == nil {
			continue
		}
		if err := r.checkBlockRedecl(root, false); err != nil {
			return err
		}
	}
	return nil
}

// checkBlockRedecl walks a subtree, running the collision check on each
// standalone Block it finds. parentFuncLike says whether the node's parent opens
// a function scope, which is how a function body is told apart from a plain
// block: a Block whose parent is function-like is a FunctionBody, and at a
// function body (like a script or module top level) a function and a var of one
// name merge legally, so the 13.2.1 rule does not apply there. Only a Block that
// is not a function body lexically scopes its top-level function declarations, so
// only such a block can carry the collision.
func (r *Renderer) checkBlockRedecl(n frontend.Node, parentFuncLike bool) error {
	if n.Kind() == frontend.NodeBlock && !parentFuncLike {
		if name, ok := r.blockFnVarCollision(n); ok {
			return fmt.Errorf("SyntaxError: redeclaration of block-scoped function '%s' as a var in the same block", name)
		}
	}
	funcLike := isFunctionLike(n.Kind())
	for _, c := range r.prog.Children(n) {
		if err := r.checkBlockRedecl(c, funcLike); err != nil {
			return err
		}
	}
	return nil
}

// blockFnVarCollision reports the name of a direct-child function declaration of
// the block that also occurs among the block's var-declared names, and whether
// there was one. The block's lexically-declared function names are its direct
// statements only: a function nested inside a deeper block belongs to that block,
// not this one. The block's var-declared names descend through nested plain
// blocks and statements but stop at any function boundary, since a function
// starts a new var scope and its inner `var` is not this block's. A name in both
// sets is the early error.
func (r *Renderer) blockFnVarCollision(block frontend.Node) (string, bool) {
	fnNames := map[string]bool{}
	for _, s := range r.prog.Children(block) {
		if s.Kind() != frontend.NodeFunctionDeclaration {
			continue
		}
		if nameNode, ok := r.funcExprNameNode(s); ok {
			fnNames[r.prog.Text(nameNode)] = true
		}
	}
	if len(fnNames) == 0 {
		return "", false
	}
	var hit string
	r.eachBlockVarName(block, func(name string) bool {
		if fnNames[name] {
			hit = name
			return true
		}
		return false
	})
	if hit != "" {
		return hit, true
	}
	return "", false
}

// eachBlockVarName calls visit with each `var`-declared name in the block's var
// scope until visit returns true. It descends through nested plain blocks and
// statements, which share the enclosing block's var scope, but skips any
// function-like subtree, whose vars belong to its own scope. Only a `var`
// statement contributes; a let or const is lexical and is not a var-declared
// name. A for-loop's own `var` init is not walked here, since the two cases this
// check targets use plain `var` statements and covering the loop init would
// widen the rule past what is measured; missing it only leaves such a case a
// handback, never a false rejection.
func (r *Renderer) eachBlockVarName(block frontend.Node, visit func(string) bool) {
	var walk func(n frontend.Node) bool
	walk = func(n frontend.Node) bool {
		for _, c := range r.prog.Children(n) {
			if isFunctionLike(c.Kind()) {
				continue
			}
			if r.isVarStatement(c) {
				for _, nn := range r.varNameNodes(c) {
					if visit(r.prog.Text(nn)) {
						return true
					}
				}
			}
			if walk(c) {
				return true
			}
		}
		return false
	}
	walk(block)
}
