package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// The Proxy ceiling, stated plainly (10_advanced group 4, item 8).
//
// Every trap that routes through an operation the value model already performs
// lowers: get, set, has, deleteProperty, ownKeys, getOwnPropertyDescriptor,
// defineProperty, getPrototypeOf, setPrototypeOf, isExtensible, preventExtensions,
// and apply. Each forwards to the target through the same runtime path the
// equivalent operator drives and enforces its non-configurable and
// non-extensible invariants against the target's own descriptors, so the proxy
// inherits those semantics rather than growing a second copy.
//
// Two corners stay a handback because they need live reflection the static model
// does not carry, and phase 11 owns them:
//
//   - The construct trap is unreachable from lowered code. new over a proxy is a
//     new expression, and bento's class path models neither the [[Construct]]
//     slot nor the newTarget it threads to the base of a chain, so new of a
//     non-builtin constructor hands back exactly as Reflect.construct does. The
//     runtime construct method exists and is unit-tested, but nothing lowered
//     reaches it yet.
//   - The apply trap sees an undefined thisArg. bento's plain functions ignore
//     the receiver at their declaration, so a lowerable target provably does not
//     read thisArg and dropping it is correct rather than lossy; a target that
//     did observe its receiver is the same later slice the call protocol defers.
//
// The invariant checks themselves lower, but they compare against the target's
// descriptor model, so a proxy whose target is an exotic object whose internals
// bento does not fully carry inherits that gap. That is a value-model ceiling on
// exotic internals, not a Proxy gap, and it is recorded so the coverage claim
// stays exact.

// newProxy lowers new Proxy(target, handler) to value.NewProxy over the two boxed
// operands, the exotic object whose internal methods route through the handler
// before they reach the target (10_advanced group 4). Both operands box into
// dynamic values, since a proxy holds its target and handler as live objects the
// runtime reads traps off; a call that does not pass exactly the target and the
// handler is not a Proxy construction and hands back.
func (r *Renderer) newProxy(args []frontend.Node) (ast.Expr, error) {
	if len(args) != 2 {
		return nil, &NotYetLowerable{Reason: "new Proxy takes exactly a target and a handler"}
	}
	// A fixed-shape target (a RegExp, a function, and other intrinsics) boxes to a
	// value.Value that carries no runtime property bag, the same limitation that
	// hands back a bare Reflect.has on such a value. A proxy over it would still box
	// it and forward [[HasProperty]], [[Get]], and [[OwnPropertyKeys]] to that bagless
	// box, which reports every inherited or own property absent and answers has, in,
	// and Object.create lookups with a wrong false. Decline the construction so the
	// program hands back rather than running to a wrong answer; a plain-object target
	// is dynamic and keeps its bag, so it lowers as before. A property view over a
	// boxed intrinsic target is a later slice.
	if !r.isDynamic(args[0]) {
		return nil, &NotYetLowerable{Reason: "new Proxy over a fixed-shape target, which boxes to a value with no runtime property bag to forward a trap to, is a later slice"}
	}
	target, err := r.boxOperand(args[0])
	if err != nil {
		return nil, err
	}
	handler, err := r.boxOperand(args[1])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "NewProxy"), Args: []ast.Expr{target, handler}}, nil
}

// isProxyConstruction reports whether a node is a new Proxy(target, handler)
// expression, the initializer a var binding must store as a boxed value.Value and
// mark dynamic. The checker types the result typeof target, a fixed shape, but the
// value is the exotic value.NewProxy builds, so a named member read p.attr off the
// binding must route through the runtime Get that dispatches the get trap rather
// than an interned Go struct selector the box does not carry. Only a two-argument
// construction off the ambient Proxy global is one; Proxy.revocable is a call, not
// a new, and a local class named Proxy resolves through classNameRef before here.
func (r *Renderer) isProxyConstruction(n frontend.Node) bool {
	if n.Kind() != frontend.NodeNewExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 || r.prog.Text(kids[0]) != "Proxy" {
		return false
	}
	if _, ok := r.classNameRef(kids[0]); ok {
		return false
	}
	return len(kids[1:]) == 2
}

// markProxyTargetLocals scans a block's statements for a new Proxy(ident, handler)
// whose target is a plain identifier and records each such name, so the target's own
// binding boxes to a shared value.Value the proxy can alias rather than a detached
// copy. It walks the statement subtrees so a proxy built inside an initializer or an
// expression is seen. The scan is additive and cheap: with no Proxy construction in
// the block it marks nothing and the proxy paths behave exactly as before.
func (r *Renderer) markProxyTargetLocals(nodes []frontend.Node) {
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		if r.isProxyConstruction(n) {
			target := r.prog.Children(n)[1]
			if target.Kind() == frontend.NodeIdentifier {
				if name, ok := localName(r.prog.Text(target)); ok {
					if r.proxyTargetLocals == nil {
						r.proxyTargetLocals = map[string]bool{}
					}
					r.proxyTargetLocals[name] = true
				}
			}
		}
		for _, kid := range r.prog.Children(n) {
			walk(kid)
		}
	}
	for _, n := range nodes {
		walk(n)
	}
}

// isProxyTargetLocal reports whether a binding name was marked a proxy target by the
// block pre-scan, the cue for its initializer to box to a shared value.Value.
func (r *Renderer) isProxyTargetLocal(nameNode frontend.Node) bool {
	name, ok := localName(r.prog.Text(nameNode))
	return ok && r.proxyTargetLocals[name]
}

// proxyStaticCall lowers a static call on the ambient Proxy global. Only
// Proxy.revocable(target, handler) is covered: it lowers to value.ProxyRevocable over
// the two boxed operands, which builds the proxy and pairs it with a revoke function
// as a { proxy, revoke } object (10_advanced group 4). Any other Proxy static, or the
// wrong arity, hands back.
func (r *Renderer) proxyStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "revocable" {
		return nil, &NotYetLowerable{Reason: "Proxy." + method + " is a later slice"}
	}
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "Proxy.revocable takes exactly a target and a handler"}
	}
	target, err := r.boxOperand(argNodes[0])
	if err != nil {
		return nil, err
	}
	handler, err := r.boxOperand(argNodes[1])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "ProxyRevocable"), Args: []ast.Expr{target, handler}}, nil
}
