package partition

import "github.com/tamnd/bento/pkg/frontend"

// Boxing is the result of escape analysis: the set of bindings whose values must
// be represented as a boxed canonical Value rather than a monomorphic Go
// representation, because each reaches a dynamic sink where interpreted code or a
// reflective operation observes or mutates it (06_compile_vs_interpret.md section
// 13). A binding not in the set can stay in its cheap monomorphic representation
// for its whole lifetime.
//
// Boxing is the second lattice Pass B propagates (section 4.2). This slice
// computes a sound lower bound on it: every binding reported here genuinely
// escapes, so the caller can trust an Escapes result, while the transitive
// data-flow closure that catches values which escape by flowing into another
// escaping value (section 13.2) is a later slice. Nothing lowers against this
// result yet, so reporting a lower bound is safe; when lowering consumes it, the
// transitive closure must be in place first, because under-boxing an escaping
// value would be unsound.
type Boxing struct {
	// Escaping holds every binding symbol that reaches a dynamic sink directly.
	Escaping map[frontend.Symbol]bool
}

// Escapes reports whether a binding must be boxed. A false result from this slice
// means "not known to escape by a direct sink," not yet "provably monomorphic";
// the latter waits on the transitive closure.
func (b Boxing) Escapes(sym frontend.Symbol) bool { return b.Escaping[sym] }

// Boxing runs escape analysis over the whole program and returns the boxing
// lattice. It walks every unit body, since a binding can reach a sink in any unit
// that can see it, and marks the binding behind every value that flows straight
// into a dynamic sink.
//
// The sink this slice detects is the reflective walk: JSON.stringify reads its
// argument's shape dynamically (section 13.1), so a typed object handed to it can
// no longer be assumed to keep a fixed Go layout and must be boxed. This is the
// sink of the section 13.5 worked example, where audit's JSON.stringify(o) is
// exactly what forces o to box while a value only ever passed to summarize stays
// monomorphic. Primitives are immutable and cross by value with no identity
// concern (section 9.3), so only object-shaped arguments escape here.
func (pt *Partitioner) Boxing() Boxing {
	b := Boxing{Escaping: map[frontend.Symbol]bool{}}
	for _, u := range pt.Units() {
		pt.scanEscapes(u.Root, &b)
	}
	return b
}

// scanEscapes walks one unit's body, stopping at nested function boundaries the
// way the other Pass B walks do, and records the binding behind every dynamic
// sink argument it finds. Each call is visited once, under the unit that owns it,
// which is enough because a sink marks the binding it names regardless of which
// unit the sink sits in.
func (pt *Partitioner) scanEscapes(node frontend.Node, b *Boxing) {
	for _, child := range pt.prog.Children(node) {
		if child.Kind() == frontend.NodeCallExpression {
			pt.inspectSink(child, b)
		}
		if functionLike(child.Kind()) {
			continue
		}
		pt.scanEscapes(child, b)
	}
}

// inspectSink marks the binding behind a JSON.stringify argument as escaping. A
// stringify of an object-typed reference to a binding forces that binding to box;
// a stringify of a primitive, a literal, or a fresh expression marks nothing,
// because a value with no binding and no identity has nothing to keep monomorphic.
func (pt *Partitioner) inspectSink(call frontend.Node, b *Boxing) {
	kids := pt.prog.Children(call)
	if len(kids) < 2 {
		return
	}
	if !pt.isJSONStringify(kids[0]) {
		return
	}
	arg := kids[1]
	if !isBoxable(pt.prog.TypeAt(arg)) {
		return
	}
	if sym, ok := pt.bindingOf(arg); ok {
		b.Escaping[sym] = true
	}
}

// isJSONStringify reports whether a call's callee is the global JSON.stringify. It
// is a property access whose receiver resolves to the global JSON namespace and
// whose member name is stringify, the same shape the lowerer recognizes before it
// emits the value serializer.
func (pt *Partitioner) isJSONStringify(callee frontend.Node) bool {
	if callee.Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	parts := pt.prog.Children(callee)
	if len(parts) < 2 {
		return false
	}
	recv, name := parts[0], parts[1]
	if sym, ok := pt.prog.SymbolAt(recv); !ok || sym.Name != "JSON" {
		return false
	}
	return pt.prog.Text(name) == "stringify"
}

// bindingOf returns the symbol a bare identifier names, the binding whose stored
// value the sink observes. An argument that is not a plain identifier, a literal,
// a call, a property access, names no single binding to box, so it returns false.
func (pt *Partitioner) bindingOf(arg frontend.Node) (frontend.Symbol, bool) {
	if arg.Kind() != frontend.NodeIdentifier {
		return frontend.Symbol{}, false
	}
	sym, ok := pt.prog.SymbolAt(arg)
	if !ok {
		return frontend.Symbol{}, false
	}
	return pt.prog.Aliased(sym), true
}

// isBoxable reports whether a type is one whose values carry mutable identity, an
// object, array, class, or function shape, as opposed to an immutable primitive.
// Only these need boxing when they reach a sink; primitives cross by value.
func isBoxable(t frontend.Type) bool {
	return t.Flags&frontend.TypeObject != 0
}
