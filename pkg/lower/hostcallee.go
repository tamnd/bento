package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// nodehostPkg is the import path of the leaf package that holds the Go host
// functions a __bento_* callee lowers to. The emitted file imports it by its base
// name, nodehost, the same way it imports the value runtime.
const nodehostPkg = "github.com/tamnd/bento/pkg/nodehost"

// hostCallee lowers a call whose result the emitted Go builds by calling into the
// runtime. The bento Node builtins are authored for the interpreter, whose host
// layer answers a bare __bento_* callee with a Go function; the AOT path has no
// such binding, so a factory that reaches for one, os.js reading __bento_os_info
// for the platform snapshot, would hand back at the unresolved-callee gate. Each
// case here gives that callee a Go form the AOT binary calls directly, the same
// function the interpreter's host layer delegates to, so the two paths return the
// same value. It is a method switch rather than a package-level table because a
// case boxes an argument through the renderer's own lowering, which the call graph
// leads back to callExpr, and a var initialized from those methods would be an
// initialization cycle. The set grows one case at a time as the AOT path learns to
// emit each callee, so a name here is always one the emitted Go can stand behind,
// the same discipline the ambient declarations follow.
func (r *Renderer) hostCallee(name string) (func(r *Renderer, argNodes []frontend.Node) (ast.Expr, error), bool) {
	switch name {
	// __bento_os_info() returns the os module snapshot as a JSON string. It takes no
	// argument, so a call with one is a malformed factory rather than a program the
	// checker let through; the emitted Go calls nodehost.OSInfoJSON and wraps the Go
	// string in a value.BStr, the string the checker types the call as and the value
	// the JavaScript JSON.parse reads.
	case "__bento_os_info":
		return func(r *Renderer, argNodes []frontend.Node) (ast.Expr, error) {
			if len(argNodes) != 0 {
				return nil, &NotYetLowerable{Reason: "__bento_os_info takes no argument"}
			}
			r.requireImport(valuePkg)
			r.requireImport(nodehostPkg)
			return &ast.CallExpr{
				Fun:  sel("value", "FromGoString"),
				Args: []ast.Expr{&ast.CallExpr{Fun: sel("nodehost", "OSInfoJSON")}},
			}, nil
		}, true
	// __bento_inspect(value) renders a value the way the interpreter's prelude
	// inspector does, the compact object-and-array form util.inspect and the assert
	// message formatter read. The argument boxes to a value.Value the way any value
	// crossing into an any parameter does, and the emitted Go calls value.Inspect,
	// which returns the BStr the string-typed call reads. It routes to pkg/value
	// rather than pkg/nodehost because the inspector is pure value introspection with
	// no host detail behind it.
	case "__bento_inspect":
		return func(r *Renderer, argNodes []frontend.Node) (ast.Expr, error) {
			if len(argNodes) != 1 {
				return nil, &NotYetLowerable{Reason: "__bento_inspect takes one argument"}
			}
			arg, err := r.boxArgToValue(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{
				Fun:  sel("value", "Inspect"),
				Args: []ast.Expr{arg},
			}, nil
		}, true
	default:
		return nil, false
	}
}

// hostCalleeCall lowers a bare call to a __bento_* host callee, returning handled
// false when the callee name is not one the switch covers so the caller falls
// through to its own ambient-global handling. The callee must be an ambient global,
// the declaration the frontend resolves the name through, so a user binding that
// happened to take a __bento_ name keeps its own lowering rather than routing here.
func (r *Renderer) hostCalleeCall(calleeNode frontend.Node, argNodes []frontend.Node) (ast.Expr, bool, error) {
	if !r.isAmbientGlobal(calleeNode) {
		return nil, false, nil
	}
	fn, ok := r.hostCallee(r.prog.Text(calleeNode))
	if !ok {
		return nil, false, nil
	}
	expr, err := fn(r, argNodes)
	return expr, true, err
}
