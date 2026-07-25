package lower

// This file lowers TextEncoder and TextDecoder, the string-to-bytes codec Node and
// the web platform expose as globals. They are the layer Buffer's utf8 paths sit on
// top of, so a compiled .js entry reaches them before it reaches most of the module
// surface, and until now every one of those programs handed back at the new.
//
// Both lower to a Go type rather than to a boxed property bag, the way Map and Set do
// and unlike the Event pair. encode returns a Uint8Array, which lives in the typed
// world with no boxed kind of its own, so an encoder held in a value.Value could not
// hand one back; the compiler knows the receiver at every call site anyway, so the
// monomorphic form is also the direct one.

import (
	"go/ast"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// isTextEncoderType reports whether an object type is a TextEncoder. encodeInto is
// unique to it among the standard library's types, so its presence is the whole
// fingerprint, read the same way isMapType reads get/set/has/size.
func (r *Renderer) isTextEncoderType(t frontend.Type) bool {
	for _, p := range r.prog.Properties(t) {
		if p.Name == "encodeInto" {
			return true
		}
	}
	return false
}

// isTextEncoder reports whether the checker types a node as a TextEncoder, the
// receiver test the encoder lowerings share.
func (r *Renderer) isTextEncoder(n frontend.Node) bool {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	return r.isTextEncoderType(t)
}

// isTextDecoderType reports whether an object type is a TextDecoder. decode and
// ignoreBOM together are the fingerprint: ignoreBOM is unique to the decoder, and
// pairing it with decode keeps an options dictionary that happens to carry the flag
// from matching the type itself.
func (r *Renderer) isTextDecoderType(t frontend.Type) bool {
	var hasDecode, hasIgnoreBOM bool
	for _, p := range r.prog.Properties(t) {
		switch p.Name {
		case "decode":
			hasDecode = true
		case "ignoreBOM":
			hasIgnoreBOM = true
		}
	}
	return hasDecode && hasIgnoreBOM
}

// isTextDecoder reports whether the checker types a node as a TextDecoder, the
// receiver test the decoder lowerings share.
func (r *Renderer) isTextDecoder(n frontend.Node) bool {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	return r.isTextDecoderType(t)
}

// newTextEncoder lowers new TextEncoder() to value.NewTextEncoder(). The constructor
// takes no argument, so a call carrying one hands back.
func (r *Renderer) newTextEncoder(args []frontend.Node) (ast.Expr, error) {
	if len(r.namedArgs(args)) != 0 {
		return nil, &NotYetLowerable{Reason: "new TextEncoder takes no argument"}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "NewTextEncoder")}, nil
}

// newTextDecoder lowers new TextDecoder(label, options) to the matching runtime
// constructor. The bare and label-only forms take value.NewTextDecoder, whose label is
// variadic; the options form takes value.NewTextDecoderWithOptions with the two flags
// read off the dictionary. A label that is not a string, or an options argument that is
// not an object literal spelling fatal and ignoreBOM, hands back: the flags have to be
// known here to reach the constructor's parameters, and a dictionary that arrives
// through a binding is a later slice.
func (r *Renderer) newTextDecoder(args []frontend.Node) (ast.Expr, error) {
	valueArgs := r.namedArgs(args)
	if len(valueArgs) > 2 {
		return nil, &NotYetLowerable{Reason: "new TextDecoder takes a label and an options dictionary"}
	}
	r.requireImport(valuePkg)
	if len(valueArgs) == 0 {
		return &ast.CallExpr{Fun: sel("value", "NewTextDecoder")}, nil
	}
	if !r.isString(valueArgs[0]) {
		return nil, &NotYetLowerable{Reason: "new TextDecoder with a label that is not a string is a later slice"}
	}
	label, err := r.lowerExpr(valueArgs[0])
	if err != nil {
		return nil, err
	}
	if len(valueArgs) == 1 {
		return &ast.CallExpr{Fun: sel("value", "NewTextDecoder"), Args: []ast.Expr{label}}, nil
	}
	fatal, ignoreBOM, err := r.textDecoderOptions(valueArgs[1])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{
		Fun:  sel("value", "NewTextDecoderWithOptions"),
		Args: []ast.Expr{label, fatal, ignoreBOM},
	}, nil
}

// textDecoderOptions reads the fatal and ignoreBOM flags off a TextDecoder options
// dictionary, returning the Go expression for each with an absent flag defaulting to
// false, the specification's default for both. Only an object literal is read: the
// flags land in the constructor's parameter list, so they have to be expressions here,
// and a dictionary that arrives through a binding would need a runtime read the
// monomorphic constructor has no slot for.
func (r *Renderer) textDecoderOptions(n frontend.Node) (fatal, ignoreBOM ast.Expr, err error) {
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return nil, nil, &NotYetLowerable{Reason: "new TextDecoder with options that are not an object literal is a later slice"}
	}
	fatal, ignoreBOM = ident("false"), ident("false")
	for _, m := range r.prog.Children(n) {
		kids := r.prog.Children(m)
		if len(kids) != 2 || strings.HasPrefix(strings.TrimSpace(r.prog.Text(m)), "...") {
			return nil, nil, &NotYetLowerable{Reason: "a TextDecoder options member that is not a plain property is a later slice"}
		}
		name, ok := r.memberName(kids[0])
		if !ok {
			return nil, nil, &NotYetLowerable{Reason: "a TextDecoder options member with a computed name is a later slice"}
		}
		if name != "fatal" && name != "ignoreBOM" {
			return nil, nil, &NotYetLowerable{Reason: "the TextDecoder option " + name + " is a later slice"}
		}
		if !r.isBool(kids[1]) {
			return nil, nil, &NotYetLowerable{Reason: "a TextDecoder option that is not a boolean needs coercion, a later slice"}
		}
		lowered, err := r.lowerExpr(kids[1])
		if err != nil {
			return nil, nil, err
		}
		if name == "fatal" {
			fatal = lowered
		} else {
			ignoreBOM = lowered
		}
	}
	return fatal, ignoreBOM, nil
}

// textEncoderMethodCall lowers a method call on a TextEncoder receiver. encode(input)
// is the whole of the covered surface: it takes a string and gives the Uint8Array of
// its UTF-8 bytes, and an absent argument is the empty string, which the specification
// makes the default. encodeInto writes into a caller's buffer and reports a read and
// written pair, an object shape this slice does not build, so it hands back.
func (r *Renderer) textEncoderMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "encode" {
		return nil, &NotYetLowerable{Reason: "TextEncoder method ." + method + " is a later slice"}
	}
	args := r.namedArgs(argNodes)
	if len(args) > 1 {
		return nil, &NotYetLowerable{Reason: "encoder.encode takes at most one argument"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	input := ast.Expr(r.bstrLit(nil))
	if len(args) == 1 {
		if !r.isString(args[0]) {
			return nil, &NotYetLowerable{Reason: "encoder.encode of a non-string needs coercion, a later slice"}
		}
		if input, err = r.lowerExpr(args[0]); err != nil {
			return nil, err
		}
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Encode")}, Args: []ast.Expr{input}}, nil
}

// textDecoderMethodCall lowers a method call on a TextDecoder receiver. decode(input)
// is the whole of the covered surface. The input must be a Uint8Array, the view the
// runtime reads bytes off; the wider BufferSource the standard library declares also
// admits an ArrayBuffer and the other views, each of which needs its own byte read,
// so those hand back. An absent argument is the empty string, which the runtime gives
// for a nil view.
func (r *Renderer) textDecoderMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "decode" {
		return nil, &NotYetLowerable{Reason: "TextDecoder method ." + method + " is a later slice"}
	}
	args := r.namedArgs(argNodes)
	if len(args) > 1 {
		return nil, &NotYetLowerable{Reason: "decoder.decode takes at most one argument"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	if len(args) == 0 {
		r.requireImport(valuePkg)
		return &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: recv, Sel: ident("Decode")},
			Args: []ast.Expr{ident("nil")},
		}, nil
	}
	if name, ok := r.typedArrayName(r.prog.TypeAt(args[0])); !ok || name != "Uint8Array" {
		return nil, &NotYetLowerable{Reason: "decoder.decode of a view other than a Uint8Array is a later slice"}
	}
	input, err := r.lowerExpr(args[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Decode")}, Args: []ast.Expr{input}}, nil
}

// textCodecGetter lowers a getter read on a TextEncoder or TextDecoder receiver. Each
// is an accessor in the source and a method on the runtime type, the same shape
// map.size takes, so it routes before the struct-field path would try to intern the
// name as a field of a shape neither type has. It reports false for any other read so
// the caller falls through.
func (r *Renderer) textCodecGetter(obj frontend.Node, prop string) (ast.Expr, bool, error) {
	var goName string
	switch {
	case r.isTextEncoder(obj) && prop == "encoding":
		goName = "Encoding"
	case r.isTextDecoder(obj):
		switch prop {
		case "encoding":
			goName = "Encoding"
		case "fatal":
			goName = "Fatal"
		case "ignoreBOM":
			goName = "IgnoreBOM"
		}
	}
	if goName == "" {
		return nil, false, nil
	}
	recv, err := r.lowerExpr(obj)
	if err != nil {
		return nil, false, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}}, true, nil
}
