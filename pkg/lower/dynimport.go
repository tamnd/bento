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

// dynImportStaticReason marks a static-specifier dynamic import the compiler
// cannot lower in its call position: the specifier names a compiled sibling
// module, but neither the promise it returns nor the module namespace that
// promise settles to has a runtime value here (the exports are package-level Go
// declarations), so a call whose result is used as a value has nothing to hand
// back. The one form that does lower, const m = await import("./mod") where m is
// used only through its members, never reaches here: it is recognized at its
// declaration and lowers to the await suspension alone (dynamicImportNamespaceDecl),
// with m a compile-time namespace.
const dynImportStaticReason = "a dynamic import whose result is used as a value is a later slice"

// importMetaPropertyCallReason marks a call whose callee is an import
// meta-property, import.defer(...) or import.meta(...). bento lowers none of the
// import meta-properties yet, and the callee carries no value binding, so the
// call hands back here rather than fall through to a type query the checker
// panics on for an unsupported meta-property.
const importMetaPropertyCallReason = "a call on an import meta-property is a later slice"

// dynamicImportCall recognizes a dynamic import() call and routes it to the
// honest, clearly-reasoned outcome for its specifier form. It returns
// handled=false when the callee is not the import keyword, so the caller keeps
// its other callee paths. When the callee is an import keyword it always hands
// back for now, but with the reason that names which side of the static/computed
// line the call falls on, so a test reports honestly instead of miscompiling
// through the function-value path.
func (r *Renderer) dynamicImportCall(n frontend.Node, kids []frontend.Node) (ast.Expr, bool, error) {
	if len(kids) == 0 {
		return nil, false, nil
	}
	// An import meta-property callee, import.defer(...) or import.meta(...), is not a
	// dynamic import() and has no value binding. bento lowers none of the import
	// meta-properties, and asking the checker for the callee type panics on an
	// unsupported meta-property (the checker's checkMetaProperty only handles
	// import.meta and new.target), so intercept the callee here and hand back
	// cleanly before any type query runs.
	if isImportMetaPropertyCallee(r.prog, kids[0]) {
		return nil, true, &NotYetLowerable{Reason: importMetaPropertyCallReason}
	}
	if !isDynamicImportCallee(r.prog, kids[0]) {
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

// isImportMetaPropertyCallee reports whether a call's callee is an import
// meta-property, import.defer or import.meta, the shape import.<name>(...) takes.
// The frontend has no dedicated node for a meta-property, so it surfaces as the
// fallback kind carrying the whole meta-property text, import.defer. A value
// binding could never take a name that starts with the reserved import keyword
// followed by a dot, so the match is unambiguous. Whitespace inside the
// meta-property (import . defer) is normalized out before the prefix test.
func isImportMetaPropertyCallee(prog *frontend.Program, callee frontend.Node) bool {
	if callee.Kind() != frontend.NodeUnknown {
		return false
	}
	text := strings.Join(strings.Fields(prog.Text(callee)), "")
	return strings.HasPrefix(text, "import.")
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

// collectDynamicImportNamespaces records the local name of every binding
// introduced by an awaited static dynamic import, const m = await import("./mod"),
// whose specifier resolves to a composed sibling module. Such a binding names the
// same compile-time namespace a static import * as m does: the sibling's exports
// are package-level Go declarations, so a member call m.inc(1) or a member value
// read const f = m.inc resolves to the export's Go name with no runtime namespace
// value. The name joins internalNamespaces so member resolution reaches it, and
// dynImportNamespaces so its declaration lowers to the await suspension alone. A
// specifier that is not a composed sibling (a bare module, a declaration file, a
// runtime-computed specifier) is not recorded, so its call still hands back rather
// than resolve members against a module the composition never staged. The pass
// runs before any body lowers, the way the static import pre-pass does, so a
// member reference above the declaration still resolves.
func (r *Renderer) collectDynamicImportNamespaces(entry frontend.Node) {
	internal := r.internalImports(entry)
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		if n.Kind() == frontend.NodeVariableDeclaration {
			if name, ok := r.dynamicImportNamespaceBinding(n, internal); ok {
				r.internalNamespaces[name] = true
				r.dynImportNamespaces[name] = true
			}
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	walk(entry)
}

// dynamicImportNamespaceBinding reports whether a variable declaration is an
// awaited static dynamic import of a composed sibling, const m = await
// import("./mod"), and returns its local binding name. The binding must be a plain
// identifier (a destructuring target pulls named exports out and is its own slice)
// and the initializer must be await applied to a dynamic import whose static
// specifier is one of the internal siblings. Any other shape returns ok=false.
func (r *Renderer) dynamicImportNamespaceBinding(decl frontend.Node, internal map[string]bool) (string, bool) {
	kids := r.prog.Children(decl)
	if len(kids) < 2 {
		return "", false
	}
	name, ok := localName(r.prog.Text(kids[0]))
	if !ok {
		return "", false
	}
	spec, ok := r.awaitedStaticDynamicImportSpecifier(kids[len(kids)-1])
	if !ok || !internal[spec] {
		return "", false
	}
	return name, true
}

// awaitedStaticDynamicImportSpecifier returns the module specifier of an
// initializer of the form await import("<literal>"), unquoted, when the operand is
// a dynamic import with a static specifier. The await is required: a bare
// import("./mod") yields the promise itself, whose namespace consumption is a
// later slice, while awaiting it here settles to the namespace the declaration
// binds. A non-await initializer, an await of anything else, or a computed
// specifier returns ok=false.
func (r *Renderer) awaitedStaticDynamicImportSpecifier(init frontend.Node) (string, bool) {
	if init.Kind() != frontend.NodeAwaitExpression {
		return "", false
	}
	operands := r.prog.Children(init)
	if len(operands) == 0 {
		return "", false
	}
	call := operands[len(operands)-1]
	if call.Kind() != frontend.NodeCallExpression {
		return "", false
	}
	args := r.prog.Children(call)
	if len(args) < 2 || !isDynamicImportCallee(r.prog, args[0]) || !isStaticSpecifier(args[1]) {
		return "", false
	}
	return unquote(r.prog.Text(args[1])), true
}

// dynamicImportNamespaceDecl lowers the declaration of an awaited static dynamic
// import, const m = await import("./mod"), when the pre-pass recorded m as a
// compile-time namespace. The binding carries no runtime value, so the statement
// declares no Go var; it lowers to the await suspension alone, value.AwaitValue(
// _co, value.Undefined), which defers one microtask the way awaiting an
// already-settled promise does, preserving the ordering an await imposes while the
// member calls on m resolve to the sibling's package-level Go declarations. An
// await outside a lowered async body has no coroutine handle to park on, which the
// checker forbids, so it hands back rather than emit a park with no handle.
func (r *Renderer) dynamicImportNamespaceDecl(decls []frontend.Node) (ast.Stmt, bool, error) {
	if len(decls) != 1 {
		return nil, false, nil
	}
	kids := r.prog.Children(decls[0])
	if len(kids) < 2 {
		return nil, false, nil
	}
	name, ok := localName(r.prog.Text(kids[0]))
	if !ok || !r.dynImportNamespaces[name] {
		return nil, false, nil
	}
	if r.asyncCo == "" {
		return nil, true, &NotYetLowerable{Reason: "a dynamic import awaited outside a lowered async body is a later slice"}
	}
	r.requireImport(valuePkg)
	call := &ast.CallExpr{
		Fun:  index(sel("value", "AwaitValue"), sel("value", "Value")),
		Args: []ast.Expr{ident(r.asyncCo), sel("value", "Undefined")},
	}
	return &ast.ExprStmt{X: call}, true, nil
}
