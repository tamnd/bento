package lower

import (
	"go/ast"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file classifies a dynamic import() call. A dynamic import is a call
// expression whose callee is the `import` keyword rather than a value, so the
// frontend surfaces it as an unnamed callee node whose text is exactly "import".
// The callExpr dispatch would otherwise treat that callee as a function value
// and hand back with a misleading reason; this pass intercepts it first and
// reports the honest split the checklist (test262/11_modules_eval_realms.md
// Group 4) asks for: a static string-literal specifier names a compiled module
// the compiler could resolve, while a runtime-computed specifier names a target
// only known at run time, which a static ahead-of-time compiler cannot load.

// dynImportComputedReason is the honest ceiling for a dynamic import whose
// specifier is computed at run time: the target is not knowable at compile time,
// so there is no compiled module to resolve it to. This is a deliberate
// hand-back, not a missing slice.
const dynImportComputedReason = "a dynamic import with a runtime-computed specifier cannot resolve its target at compile time"

// dynImportStaticReason marks the static-specifier form as a later slice: the
// specifier names a compiled sibling module, so the call resolves to a load of
// that module's namespace returning a resolved promise, but modeling the
// namespace value and its promise wrapper on top of the module-composition and
// async runtime is its own arc.
const dynImportStaticReason = "a dynamic import with a static specifier resolving to a compiled-module load is a later slice"

// dynamicImportCall recognizes a dynamic import() call and routes it to the
// honest, clearly-reasoned outcome for its specifier form. It returns
// handled=false when the callee is not the import keyword, so the caller keeps
// its other callee paths. When the callee is an import keyword it always hands
// back for now, but with the reason that names which side of the static/computed
// line the call falls on, so a test reports honestly instead of miscompiling
// through the function-value path.
func (r *Renderer) dynamicImportCall(n frontend.Node, kids []frontend.Node) (ast.Expr, bool, error) {
	if len(kids) == 0 || !isDynamicImportCallee(r.prog, kids[0]) {
		return nil, false, nil
	}
	// A specifier that is a string literal or a template with no substitutions is
	// statically knowable, so it names a compiled module; anything else (an
	// identifier, a concatenation, a call result) is computed at run time.
	if len(kids) >= 2 && isStaticSpecifier(kids[1]) {
		return nil, true, &NotYetLowerable{Reason: dynImportStaticReason}
	}
	return nil, true, &NotYetLowerable{Reason: dynImportComputedReason}
}

// isDynamicImportCallee reports whether a call's callee is the import keyword,
// the shape a dynamic import() takes. The frontend has no dedicated node for the
// keyword, so it surfaces as the fallback kind carrying the exact text "import";
// a value binding could never take that reserved word as its name, so the match
// is unambiguous.
func isDynamicImportCallee(prog *frontend.Program, callee frontend.Node) bool {
	return callee.Kind() == frontend.NodeUnknown && strings.TrimSpace(prog.Text(callee)) == "import"
}

// isStaticSpecifier reports whether a dynamic import's specifier argument is
// statically knowable: a string literal or a template literal with no
// substitutions. A parenthesized literal, a concatenation, or any expression the
// checker only resolves at run time is computed.
func isStaticSpecifier(arg frontend.Node) bool {
	switch arg.Kind() {
	case frontend.NodeStringLiteral, frontend.NodeNoSubstitutionTemplateLiteral:
		return true
	default:
		return false
	}
}
