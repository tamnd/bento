package lower

import (
	"github.com/tamnd/bento/pkg/frontend"
)

// This file tracks the locals bound as boxed dynamic values (value.Value) so a
// read the checker narrowed to one primitive, past a typeof guard, unboxes
// through the matching accessor. Without it a narrowed read hands the bare box
// to a static path (a concat, a compare) that expects the Go primitive, which
// is Go that does not compile. The pre-pass mirrors unionLocalsOf: names come
// from the signature parameters and the body's declarations, and a name
// declared more than once is dropped since the set is keyed by name alone.

// dynLocalsOf collects the names bound as boxed dynamic values in a body: each
// parameter and each variable declaration typed any or unknown, the bindings
// that lower to value.Value. A shadowed name is dropped the way the union
// pre-pass drops it, and an empty result is nil so the common body with no
// dynamic binding pays nothing.
func (r *Renderer) dynLocalsOf(params []frontend.Param, body []frontend.Node) map[string]bool {
	out := map[string]bool{}
	declCount := map[string]int{}
	for _, p := range params {
		name, ok := localName(p.Name)
		if !ok {
			continue
		}
		declCount[name]++
		if p.Type.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
			out[name] = true
		}
	}
	for _, n := range body {
		r.collectDynDecls(n, out, declCount)
	}
	for name, c := range declCount {
		if c != 1 {
			delete(out, name)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// collectDynDecls walks one node, recording each variable declaration whose
// name is typed any or unknown, and recurses into its children so a binding in
// a nested block or loop is seen. It counts declarations per name alongside so
// dynLocalsOf can drop a name redeclared in this same scope. It stops at a
// nested function boundary: a binding inside a nested function belongs to that
// function's own dynLocals pass, so counting it here would treat a same-named
// local as a redeclaration and drop the outer binding. A prelude helper with a
// local `result` used to knock the top-level `result` out of the boxed-locals
// set this way, leaving its narrowed read a bare box that double-boxed and
// failed go build.
func (r *Renderer) collectDynDecls(n frontend.Node, out map[string]bool, declCount map[string]int) {
	if isFunctionScope(n.Kind()) {
		return
	}
	if n.Kind() == frontend.NodeVariableDeclaration {
		kids := r.prog.Children(n)
		if len(kids) > 0 && kids[0].Kind() == frontend.NodeIdentifier {
			if name, ok := localName(r.prog.Text(kids[0])); ok {
				declCount[name]++
				if r.prog.TypeAt(kids[0]).Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
					out[name] = true
				}
			}
		}
	}
	for _, c := range r.prog.Children(n) {
		r.collectDynDecls(c, out, declCount)
	}
}

// scopeDeclaredNames collects every binding name a function scope declares, its
// parameters and its variable declarations, stopping at a nested function the way
// collectDynDecls does. A closure inherits the enclosing dynamic locals so a captured
// outer binding stays tracked inside it, but a name the closure redeclares as its own
// local shadows that outer binding. The merge subtracts these names before overlaying
// the closure's own dynamic set, so an outer dynamic does not leak a value accessor
// onto a static local that reuses the name.
func (r *Renderer) scopeDeclaredNames(params []frontend.Param, body []frontend.Node) map[string]bool {
	out := map[string]bool{}
	for _, p := range params {
		if name, ok := localName(p.Name); ok {
			out[name] = true
		}
	}
	for _, n := range body {
		r.collectDeclaredNames(n, out)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// collectDeclaredNames walks one node recording every variable declaration name,
// whatever its type, and recurses into control-flow children while stopping at a
// nested function scope, the same boundary collectDynDecls keeps.
func (r *Renderer) collectDeclaredNames(n frontend.Node, out map[string]bool) {
	if isFunctionScope(n.Kind()) {
		return
	}
	if n.Kind() == frontend.NodeVariableDeclaration {
		kids := r.prog.Children(n)
		if len(kids) > 0 && kids[0].Kind() == frontend.NodeIdentifier {
			if name, ok := localName(r.prog.Text(kids[0])); ok {
				out[name] = true
			}
		}
	}
	for _, c := range r.prog.Children(n) {
		r.collectDeclaredNames(c, out)
	}
}

// isFunctionScope reports whether a node opens a new function scope, the boundary
// the dynamic-locals walk stops at so a nested function's bindings are left to that
// function's own pre-pass. It covers every form that carries its own parameter list
// and body: a function declaration or expression, an arrow, and a class member.
func isFunctionScope(k frontend.NodeKind) bool {
	switch k {
	case frontend.NodeFunctionDeclaration,
		frontend.NodeFunctionExpression,
		frontend.NodeArrowFunction,
		frontend.NodeMethodDeclaration,
		frontend.NodeGetAccessor,
		frontend.NodeSetAccessor,
		frontend.NodeConstructor:
		return true
	}
	return false
}

// dynAccessor names the Value accessor for a boxed read the checker narrowed to
// one primitive kind: the folded flags carry exactly one of string, number, or
// boolean and no longer the any or unknown that boxed the binding. The caller
// passes primitiveFlags, whose union fold matters here: typeof narrows any to
// boolean as the true | false union, which folds to the boolean facet this
// switch reads, so the union bit alone does not disqualify. Any other shape,
// including a still-dynamic read, a mixed union, and a narrow to bigint or a
// non-primitive the accessors do not cover, returns false so the read keeps the
// bare box and the dynamic paths (or a hand-back) take it from there.
func dynAccessor(f frontend.TypeFlags) (string, bool) {
	if f&(frontend.TypeAny|frontend.TypeUnknown) != 0 {
		return "", false
	}
	const prims = frontend.TypeString | frontend.TypeNumber | frontend.TypeBoolean | frontend.TypeBigInt
	switch f & prims {
	case frontend.TypeString:
		return "AsString", true
	case frontend.TypeNumber:
		return "AsNumber", true
	case frontend.TypeBoolean:
		return "AsBool", true
	}
	return "", false
}
