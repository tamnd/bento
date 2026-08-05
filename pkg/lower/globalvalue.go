package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/value"
)

// An ambient global named rather than called has no generated Go behind its name,
// so capitalizing the source name would emit an undefined symbol like Symbol or
// Atob. Before this file every such reference handed the unit back, which is what
// the two reasons "the ambient global X read as a value" and "the ambient global X
// used as a value" said.
//
// A program reaches for one constantly, and almost always for its identity:
//
//	assert.strictEqual(globalThis.atob, buffer.atob)
//	assert.notStrictEqual(sandbox.Symbol, Symbol)
//	traceCallback(setImmediate, ...)
//
// The globals bento hosts now have a value form (pkg/value/globalvalue.go): one
// interned value per name, reporting typeof "function", carrying .name, calling
// what the direct call lowers to, and comparing equal to itself wherever the
// program reaches it, including off the global object.
//
// What is hosted is what the runtime has built, and the list lives on the runtime
// side so the compiler cannot promise a name the runtime cannot answer for. A
// global bento has not built (crypto, WebSocket, ReadableStream, Atomics as an
// object, eval) is absent from it, so the handbacks below stay exactly as they
// were: refusing at compile time rather than handing the program a value that
// answers undefined for everything it asks.

// hostedGlobalRef reports whether n is a reference to an ambient global bento has a
// value form for, and so should lower to that value rather than hand back. It is
// the name test and the ambient test together, so a user's own `const Symbol = ...`
// is untouched and reads its own binding.
//
// A receiver is not a reference of that kind. Object.keys(o) and Symbol.iterator do
// not read the global as a value, they ask it for a member, and the static paths own
// every member bento lowers: they recognize the receiver by name and emit their own
// helper. A member those paths do not claim, Object.getOwnPropertyDescriptor say,
// hands the unit back today, and it must keep handing back. Answering the receiver
// with the value form would turn that honest compile-time refusal into a runtime read
// of a key the value does not carry, which is undefined rather than a wrong answer
// but is still a program that builds and then fails at the member instead of a
// program that never claimed to build.
func (r *Renderer) hostedGlobalRef(n frontend.Node) bool {
	if n.Kind() != frontend.NodeIdentifier {
		return false
	}
	// An entry point that never ran the pre-pass cannot tell a receiver from a value,
	// and answering yes there would take the static member paths away from a caller
	// that never asked for any of this. No set means no value form, which is what the
	// lowering did before this file existed.
	if r.memberReceivers == nil {
		return false
	}
	if !value.HostsGlobal(r.prog.Text(n)) {
		return false
	}
	if r.memberReceivers[n] {
		return false
	}
	return r.isAmbientGlobal(n)
}

// hostedGlobalValueRef reports whether n reads a hosted global as a value rather than
// calls it. A callee is excluded because the call paths own it: btoa(s) lowers to
// value.Btoa, a bento string and not a box, so treating the reference as a boxed
// value would make the whole call read as boxed and the string flow into a dynamic
// slot unwrapped. The value form still answers for a callee no call path claimed,
// which is what hostedGlobalRef above is for; this is only about what the call
// produces.
func (r *Renderer) hostedGlobalValueRef(n frontend.Node) bool {
	return r.hostedGlobalRef(n) && !r.callCallees[n]
}

// collectMemberReceivers records the two positions a reference to a global can sit in
// without being a read of its value: the receiver of a member access, which the static
// member paths own, and the callee of a call, which the call paths own. Both access
// forms count as a member access, since o.k and o[k] leave the receiver to the same
// paths.
func (r *Renderer) collectMemberReceivers(files []frontend.Node) {
	if r.memberReceivers == nil {
		r.memberReceivers = map[frontend.Node]bool{}
	}
	if r.callCallees == nil {
		r.callCallees = map[frontend.Node]bool{}
	}
	for _, f := range files {
		r.walkMemberReceivers(f)
	}
}

func (r *Renderer) walkMemberReceivers(n frontend.Node) {
	switch n.Kind() {
	case frontend.NodePropertyAccessExpression, frontend.NodeElementAccessExpression:
		if kids := r.prog.Children(n); len(kids) > 0 {
			r.memberReceivers[kids[0]] = true
		}
	case frontend.NodeCallExpression, frontend.NodeNewExpression:
		if kids := r.prog.Children(n); len(kids) > 0 {
			r.callCallees[kids[0]] = true
		}
	}
	for _, c := range r.prog.Children(n) {
		r.walkMemberReceivers(c)
	}
}

// hostedGlobalExpr lowers a hosted ambient global to its interned runtime value.
func (r *Renderer) hostedGlobalExpr(n frontend.Node) ast.Expr {
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun:  sel("value", "GlobalValue"),
		Args: []ast.Expr{stringLit(r.prog.Text(n))},
	}
}
