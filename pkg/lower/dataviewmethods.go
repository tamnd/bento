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

// dataViewBigIntGetters maps the 64-bit DataView getters to their runtime method.
// Each takes a byte offset and an optional littleEndian flag like the numeric getters,
// but its runtime method returns a *big.Int, since a 64-bit integer reads back as the
// bigint a Number cannot hold without loss (25 §25.3.4).
var dataViewBigIntGetters = map[string]string{
	"getBigInt64":  "GetBigInt64",
	"getBigUint64": "GetBigUint64",
}

// dataViewByteSetters maps the single-byte DataView setters to their runtime method.
// They take a byte offset and a value and carry no endianness, since one byte has no
// byte order (25 §25.3.4).
var dataViewByteSetters = map[string]string{
	"setInt8":  "SetInt8",
	"setUint8": "SetUint8",
}

// dataViewEndianSetters maps the multi-byte numeric DataView setters to their runtime
// method. Each takes a byte offset, a value, and an optional littleEndian flag that
// selects the byte order the write uses (25 §25.3.4).
var dataViewEndianSetters = map[string]string{
	"setInt16":   "SetInt16",
	"setUint16":  "SetUint16",
	"setInt32":   "SetInt32",
	"setUint32":  "SetUint32",
	"setFloat16": "SetFloat16",
	"setFloat32": "SetFloat32",
	"setFloat64": "SetFloat64",
}

// dataViewBigIntSetters maps the 64-bit DataView setters to their runtime method. Each
// takes a byte offset, a bigint value, and an optional littleEndian flag, its value a
// bigint rather than the Number the numeric setters take, since a 64-bit integer does
// not fit a Number without loss (25 §25.3.4).
var dataViewBigIntSetters = map[string]string{
	"setBigInt64":  "SetBigInt64",
	"setBigUint64": "SetBigUint64",
}

// dataViewMethodCall lowers a method call whose receiver is a DataView to the
// matching value.DataView getter or setter (25 §25.3). It runs after the typed-array
// and buffer paths in methodCall, since a DataView is its own view kind, and covers
// the getter and setter families here. A method the surface does not carry hands back
// with a reason naming it, so it reaches a clear handback rather than the generic
// receiver error.
func (r *Renderer) dataViewMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	if runtimeName, ok := dataViewByteGetters[method]; ok {
		return r.dataViewGet(recvNode, runtimeName, argNodes, false)
	}
	if runtimeName, ok := dataViewEndianGetters[method]; ok {
		return r.dataViewGet(recvNode, runtimeName, argNodes, true)
	}
	if runtimeName, ok := dataViewBigIntGetters[method]; ok {
		return r.dataViewGet(recvNode, runtimeName, argNodes, true)
	}
	if runtimeName, ok := dataViewByteSetters[method]; ok {
		return r.dataViewSet(recvNode, runtimeName, argNodes, false, false)
	}
	if runtimeName, ok := dataViewEndianSetters[method]; ok {
		return r.dataViewSet(recvNode, runtimeName, argNodes, true, false)
	}
	if runtimeName, ok := dataViewBigIntSetters[method]; ok {
		return r.dataViewSet(recvNode, runtimeName, argNodes, true, true)
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

// dataViewSet lowers a DataView setter call: recv.<Method>(offset, value) for the
// single-byte setters and recv.<Method>(offset, value, littleEndian?) for the
// multi-byte ones, the variadic bool carrying the endianness flag's optionality. The
// byte offset is a required Number; the value is a required Number for the numeric
// setters and a required bigint for the 64-bit ones; the endianness, where the method
// takes one, is an optional boolean. A wrong-typed offset, value, or endianness is a
// later slice and hands back, as does a byte setter given an endianness argument or
// any setter given too few or too many arguments.
func (r *Renderer) dataViewSet(recvNode frontend.Node, runtimeName string, argNodes []frontend.Node, endian, bigint bool) (ast.Expr, error) {
	maxArgs := 2
	if endian {
		maxArgs = 3
	}
	if len(argNodes) < 2 || len(argNodes) > maxArgs {
		return nil, &NotYetLowerable{Reason: "DataView " + runtimeName + " takes a byte offset, a value, and, where the width allows, an endianness flag"}
	}
	if !r.isNumber(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "a DataView byte offset that is not a number is a later slice"}
	}
	if bigint {
		if !r.isBigInt(argNodes[1]) {
			return nil, &NotYetLowerable{Reason: "a DataView bigint setter value that is not a bigint is a later slice"}
		}
	} else if !r.isNumber(argNodes[1]) {
		return nil, &NotYetLowerable{Reason: "a DataView numeric setter value that is not a number is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	offset, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	value, err := r.lowerExpr(argNodes[1])
	if err != nil {
		return nil, err
	}
	callArgs := []ast.Expr{offset, value}
	if len(argNodes) == 3 {
		if !r.isBool(argNodes[2]) {
			return nil, &NotYetLowerable{Reason: "a DataView endianness flag that is not a boolean is a later slice"}
		}
		le, err := r.lowerExpr(argNodes[2])
		if err != nil {
			return nil, err
		}
		callArgs = append(callArgs, le)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(runtimeName)}, Args: callArgs}, nil
}
