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
// dynLocalsOf can drop a shadowed name, the same guard the union pre-pass
// keeps.
func (r *Renderer) collectDynDecls(n frontend.Node, out map[string]bool, declCount map[string]int) {
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
