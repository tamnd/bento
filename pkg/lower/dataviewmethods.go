package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// dataViewByteGetters maps the single-byte DataView getters to their runtime method.
// They take only a byte offset and carry no endianness, since one byte has no byte
// order (25 §25.3.4).
var dataViewByteGetters = map[string]string{
	"getInt8":  "GetInt8",
	"getUint8": "GetUint8",
}

// dataViewEndianGetters maps the multi-byte DataView getters to their runtime method.
// Each takes a byte offset and an optional littleEndian flag that selects the byte
// order the read uses (25 §25.3.4).
var dataViewEndianGetters = map[string]string{
	"getInt16":   "GetInt16",
	"getUint16":  "GetUint16",
	"getInt32":   "GetInt32",
	"getUint32":  "GetUint32",
	"getFloat16": "GetFloat16",
	"getFloat32": "GetFloat32",
	"getFloat64": "GetFloat64",
}

// dataViewMethodCall lowers a method call whose receiver is a DataView to the
// matching value.DataView getter or setter (25 §25.3). It runs after the typed-array
// and buffer paths in methodCall, since a DataView is its own view kind, and covers
// the getter family here; the float, bigint, and setter families land in later
// commits under the same group. A method the surface does not carry hands back with a
// reason naming it, so it reaches a clear handback rather than the generic receiver
// error.
func (r *Renderer) dataViewMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	if runtimeName, ok := dataViewByteGetters[method]; ok {
		return r.dataViewGet(recvNode, runtimeName, argNodes, false)
	}
	if runtimeName, ok := dataViewEndianGetters[method]; ok {
		return r.dataViewGet(recvNode, runtimeName, argNodes, true)
	}
	return nil, &NotYetLowerable{Reason: "DataView method ." + method + " is a later slice"}
}

// dataViewGet lowers a DataView getter call: recv.<Method>(offset) for the single-byte
// getters and recv.<Method>(offset, littleEndian?) for the multi-byte ones, the
// variadic bool carrying the endianness flag's optionality. The byte offset is a
// required Number; the endianness, where the method takes one, is an optional boolean.
// A non-number offset or non-boolean endianness is a later slice and hands back, as
// does a byte getter given an endianness argument or any getter given too many
// arguments.
func (r *Renderer) dataViewGet(recvNode frontend.Node, runtimeName string, argNodes []frontend.Node, endian bool) (ast.Expr, error) {
	maxArgs := 1
	if endian {
		maxArgs = 2
	}
	if len(argNodes) == 0 || len(argNodes) > maxArgs {
		return nil, &NotYetLowerable{Reason: "DataView " + runtimeName + " takes a byte offset and, where the width allows, an endianness flag"}
	}
	if !r.isNumber(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "a DataView byte offset that is not a number is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	offset, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	callArgs := []ast.Expr{offset}
	if len(argNodes) == 2 {
		if !r.isBool(argNodes[1]) {
			return nil, &NotYetLowerable{Reason: "a DataView endianness flag that is not a boolean is a later slice"}
		}
		le, err := r.lowerExpr(argNodes[1])
		if err != nil {
			return nil, err
		}
		callArgs = append(callArgs, le)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(runtimeName)}, Args: callArgs}, nil
}
