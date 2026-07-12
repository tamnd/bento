package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

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
