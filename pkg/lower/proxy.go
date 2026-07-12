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
