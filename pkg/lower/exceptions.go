package lower

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the exception surface: constructing a built-in error and
// throwing it. A throw becomes a Go panic carrying a value.Error, and the
// assembled main defers value.ReportUncaught so a throw that escapes every catch
// prints an uncaught-error line and exits non-zero rather than crashing with a Go
// stack (10_value_model.md, and 16_go_interop.md section 7.7 for the boundary
// errors that share the same reporting). Recovering a throw into a catch binding
// is a later slice; this one covers raising and reporting, the half a program
// needs before try/catch can catch anything.

// errorCtors maps a built-in error constructor name to the value constructor its
// new expression lowers to. Only the errors bento's own lowerings raise (a plain
// Error, a TypeError from a failed guard, a RangeError from a range check, a
// SyntaxError from BigInt string parsing) are covered; the DOM and Node error
// subclasses are a later slice, and an unlisted name hands back so a user class
// named Error-something is never mistaken for a built-in.
var errorCtors = map[string]string{
	"Error":       "NewError",
	"TypeError":   "NewTypeError",
	"RangeError":  "NewRangeError",
	"SyntaxError": "NewSyntaxError",
}

// errorCtorValueNames is the set of built-in error constructors that lower to a
// value when named without new, the argument-and-comparison surface assert.throws
// drives. It is broader than errorCtors because boxing a constructor and reading
// its name work off the name alone, with no construction path, so it lists every
// standard error constructor plus URIError, matching the value package's own set.
var errorCtorValueNames = map[string]bool{
	"Error":          true,
	"TypeError":      true,
	"RangeError":     true,
	"SyntaxError":    true,
	"ReferenceError": true,
	"EvalError":      true,
	"URIError":       true,
	"AggregateError": true,
}

// errorConstructorRef reports the built-in error constructor an identifier names as
// a value, and ok=false for anything else. It gates on the ambient-global test so a
// user binding or class that shadows one of the names (class TypeError of one's own)
// is never mistaken for the built-in and keeps its own lowering. This is what the
// dynamic-boxing path checks to lower TypeError, passed as an argument or compared
// for identity, to the interned constructor value.
func (r *Renderer) errorConstructorRef(n frontend.Node) (string, bool) {
	if n.Kind() != frontend.NodeIdentifier {
		return "", false
	}
	name := r.prog.Text(n)
	if !errorCtorValueNames[name] {
		return "", false
	}
	if !r.isAmbientGlobal(n) {
		return "", false
	}
	return name, true
}

// newExpr lowers a new expression. Only the built-in error constructors are
// covered: new Error(message) and its TypeError and RangeError siblings lower to
// the matching value constructor, taking a single string message or none. A
// constructor bento does not recognize, or an error constructor called with a
// non-string or with more than one argument, hands back to the engine.
func (r *Renderer) newExpr(n frontend.Node) (ast.Expr, error) {
	kids := r.prog.Children(n)
	if len(kids) == 0 {
		return nil, &NotYetLowerable{Reason: "new expression did not expose a constructor"}
	}
	// A user class constructs through its generated NewX function. The name is
	// resolved through the constructor identifier's symbol, not its spelling, so a
	// local class named Map or Uint8Array still constructs as the class and never
	// falls through to a built-in with the same name.
	if kids[0].Kind() == frontend.NodeIdentifier {
		if info, ok := r.classNameRef(kids[0]); ok {
			return r.newClass(info, kids[1:])
		}
	}
	if r.prog.Text(kids[0]) == "Uint8Array" {
		return r.newTypedArray("Uint8Array", kids[1:])
	}
	if _, ok := typedArrayElemGo(r.prog.Text(kids[0])); ok {
		return r.newTypedArray(r.prog.Text(kids[0]), kids[1:])
	}
	if _, ok := bigintTypedArrayElemGo(r.prog.Text(kids[0])); ok {
		return r.newBigIntArray(r.prog.Text(kids[0]), kids[1:])
	}
	if r.prog.Text(kids[0]) == "ArrayBuffer" {
		return r.newArrayBuffer(kids[1:])
	}
	if r.prog.Text(kids[0]) == "SharedArrayBuffer" {
		return r.newSharedArrayBuffer(kids[1:])
	}
	if r.prog.Text(kids[0]) == "DataView" {
		return r.newDataView(kids[1:])
	}
	if r.prog.Text(kids[0]) == "WeakMap" {
		return r.newWeakMap(n, kids[1:])
	}
	if r.prog.Text(kids[0]) == "WeakRef" {
		return r.newWeakRef(n, kids[1:])
	}
	if r.prog.Text(kids[0]) == "FinalizationRegistry" {
		return r.newFinalizationRegistry(n, kids[1:])
	}
	if r.prog.Text(kids[0]) == "Map" {
		return r.newMap(n, kids[1:])
	}
	if r.prog.Text(kids[0]) == "WeakSet" {
		return r.newWeakSet(n, kids[1:])
	}
	if r.prog.Text(kids[0]) == "Set" {
		return r.newSet(n, kids[1:])
	}
	if r.prog.Text(kids[0]) == "Object" {
		return r.newObject(kids[1:])
	}
	if r.prog.Text(kids[0]) == "Proxy" {
		return r.newProxy(kids[1:])
	}
	// new Function("a", "return a") builds a function from source text at run time,
	// parsing the argument strings as a parameter list and a body. That is eval work,
	// phase 11, so it hands back with the reason that names where it belongs rather
	// than the generic constructor reason below.
	if r.prog.Text(kids[0]) == "Function" {
		return nil, &NotYetLowerable{Reason: "a Function built from a source string is eval, deferred to phase 11"}
	}
	if r.prog.Text(kids[0]) == "Promise" {
		return r.newPromise(n, kids[1:])
	}
	if r.prog.Text(kids[0]) == "RegExp" {
		return r.newRegExp(kids[1:])
	}
	ctor, ok := errorCtors[r.prog.Text(kids[0])]
	if !ok {
		return nil, &NotYetLowerable{Reason: "new of a constructor other than a built-in error is a later slice"}
	}
	args := kids[1:]
	if len(args) > 1 {
		return nil, &NotYetLowerable{Reason: "an error constructed with more than a message is a later slice"}
	}
	var message ast.Expr
	if len(args) == 0 {
		// new Error() with no argument carries an empty message, the same shape as
		// new Error(""), so it lowers to an empty bento string.
		message = r.bstrLit(nil)
	} else {
		if !r.isString(args[0]) {
			return nil, &NotYetLowerable{Reason: "an error constructed from a non-string message is a later slice"}
		}
		lowered, err := r.lowerExpr(args[0])
		if err != nil {
			return nil, err
		}
		message = lowered
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", ctor), Args: []ast.Expr{message}}, nil
}

// newObject lowers new Object() to the empty boxed object value.NewObject builds,
// a live property bag the dynamic Get and Set paths read and write. Only the
// zero-argument form is claimed: new Object(x) is the ToObject coercion, which
// returns x untouched when it is already an object and boxes a primitive into its
// wrapper, a distinct later slice, so it hands back.
func (r *Renderer) newObject(args []frontend.Node) (ast.Expr, error) {
	if len(args) != 0 {
		return nil, &NotYetLowerable{Reason: "new Object(value), the ToObject coercion, is a later slice"}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "NewObject")}, nil
}

// newTypedArray lowers a typed-array construction, the fixed-width numeric buffers
// of section 6.3, for the whole family by name. Two forms are covered. new
// Int32Array(n) allocates a zeroed buffer of length n, so a single Number argument
// lowers to value.NewInt32Array(n). new Int32Array([a, b, c]) fills a buffer from a
// list of values, so a single array-literal argument lowers to value.Int32ArrayOf(a,
// b, c) with each element coerced by the element kind's store rule. The two are told
// apart by the argument's syntax, an array literal versus anything else, so a length
// that happens to be a variable still takes the length form. The constructor names
// follow the New<Name> and <Name>Of scheme every family member shares, so the name
// alone selects them. Two more sources build a fresh buffer and copy into it: a
// number array value lowers to value.<Name>Of(src.Elems()...), and another typed
// array lowers to value.<Name>Of(src.Floats()...), each spreading the source's
// widened elements through the Of constructor's per-element coercion. The view over
// an ArrayBuffer with an offset and length takes newTypedArrayOverBuffer. A call
// with no argument or more than one non-buffer argument hands back.
func (r *Renderer) newTypedArray(name string, args []frontend.Node) (ast.Expr, error) {
	if len(args) >= 1 && r.isViewBuffer(args[0]) {
		return r.newTypedArrayOverBuffer(name, args)
	}
	if len(args) != 1 {
		return nil, &NotYetLowerable{Reason: "only new " + name + "(length) and new " + name + "([...]) are lowered yet"}
	}
	r.requireImport(valuePkg)
	if args[0].Kind() == frontend.NodeArrayLiteralExpression {
		elems := r.prog.Children(args[0])
		lowered := make([]ast.Expr, 0, len(elems))
		for _, e := range elems {
			if e.Kind() == frontend.NodeSpreadElement {
				return nil, &NotYetLowerable{Reason: "spread element in a " + name + " initializer is a later slice"}
			}
			if !r.isNumber(e) {
				return nil, &NotYetLowerable{Reason: "a " + name + " initialized from a non-number element is a later slice"}
			}
			v, err := r.lowerExpr(e)
			if err != nil {
				return nil, err
			}
			lowered = append(lowered, v)
		}
		return &ast.CallExpr{Fun: sel("value", name+"Of"), Args: lowered}, nil
	}
	// A number array value or another typed array copies into a fresh buffer,
	// spreading the source's elements through the Of constructor. A number array
	// reads its elements with Elems; a typed array widens each element with Floats.
	if r.isNumberArrayValue(args[0]) {
		return r.newTypedArrayFrom(name, args[0], "Elems")
	}
	if r.numericTypedArray(args[0]) {
		return r.newTypedArrayFrom(name, args[0], "Floats")
	}
	if !r.isNumber(args[0]) {
		return nil, &NotYetLowerable{Reason: "a " + name + " length that is not a number is a later slice"}
	}
	length, err := r.lowerExpr(args[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: sel("value", "New"+name), Args: []ast.Expr{length}}, nil
}

// newTypedArrayFrom lowers a typed array copied from a source value into a fresh
// buffer: value.<Name>Of(src.<read>()...), where read is Elems for a number array
// or Floats for another typed array, both returning the []float64 the Of
// constructor spreads through its per-element coercion. The Of constructor allocates
// its own buffer, so the copy does not alias the source.
func (r *Renderer) newTypedArrayFrom(name string, src frontend.Node, read string) (ast.Expr, error) {
	recv, err := r.lowerExpr(src)
	if err != nil {
		return nil, err
	}
	elems := &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(read)}}
	return &ast.CallExpr{
		Fun:      sel("value", name+"Of"),
		Args:     []ast.Expr{elems},
		Ellipsis: token.Pos(1),
	}, nil
}

// newBigIntArray lowers a bigint typed-array construction, the BigInt64Array and
// BigUint64Array of section 6.3 whose element is a bigint rather than a Number. It
// mirrors newTypedArray for the three covered forms: new BigInt64Array(n) lowers to
// value.NewBigInt64Array(n), new BigInt64Array([1n, 2n]) to value.BigInt64ArrayOf(1,
// 2) with each bigint element passed as the *big.Int it lowers to, and the view over
// an ArrayBuffer to value.BigInt64ArrayView through the shared newTypedArrayOverBuffer.
// The list form takes bigint elements rather than the numbers the numeric family
// takes, so a non-bigint element hands back. A copy from another typed array is a
// later slice.
func (r *Renderer) newBigIntArray(name string, args []frontend.Node) (ast.Expr, error) {
	if len(args) >= 1 && r.isViewBuffer(args[0]) {
		return r.newTypedArrayOverBuffer(name, args)
	}
	if len(args) != 1 {
		return nil, &NotYetLowerable{Reason: "only new " + name + "(length) and new " + name + "([...]) are lowered yet"}
	}
	r.requireImport(valuePkg)
	if args[0].Kind() == frontend.NodeArrayLiteralExpression {
		elems := r.prog.Children(args[0])
		lowered := make([]ast.Expr, 0, len(elems))
		for _, e := range elems {
			if e.Kind() == frontend.NodeSpreadElement {
				return nil, &NotYetLowerable{Reason: "spread element in a " + name + " initializer is a later slice"}
			}
			if !r.isBigInt(e) {
				return nil, &NotYetLowerable{Reason: "a " + name + " initialized from a non-bigint element is a later slice"}
			}
			v, err := r.lowerExpr(e)
			if err != nil {
				return nil, err
			}
			lowered = append(lowered, v)
		}
		return &ast.CallExpr{Fun: sel("value", name+"Of"), Args: lowered}, nil
	}
	if !r.isNumber(args[0]) {
		return nil, &NotYetLowerable{Reason: "a " + name + " length that is not a number is a later slice"}
	}
	length, err := r.lowerExpr(args[0])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: sel("value", "New"+name), Args: []ast.Expr{length}}, nil
}

// isNumberArrayValue reports whether a node's type is an array whose element type is
// a number, the from-an-array-like source a typed-array constructor copies. A typed
// array is not an ElementType array, so it does not match here and takes the
// typed-array-source path instead.
func (r *Renderer) isNumberArrayValue(n frontend.Node) bool {
	elem, ok := r.prog.ElementType(r.prog.TypeAt(n))
	if !ok {
		return false
	}
	return r.primitiveFlagsOfType(elem)&frontend.TypeNumber != 0
}

// newTypedArrayOverBuffer lowers a typed array constructed as a view over an
// existing ArrayBuffer, new Int32Array(buffer, byteOffset, length) and its shorter
// forms. The buffer is required, the byte offset defaults to zero when omitted, and
// the length runs to the end of the buffer when omitted, so the three overloads
// lower to value.<Name>View(buf), value.<Name>View(buf, offset), and
// value.<Name>View(buf, offset, length), the variadic length carrying the
// optionality. A byte offset or length that is not a number is a later slice and
// hands back, as does a call with more than three arguments.
func (r *Renderer) newTypedArrayOverBuffer(name string, args []frontend.Node) (ast.Expr, error) {
	if len(args) > 3 {
		return nil, &NotYetLowerable{Reason: "new " + name + " over a buffer takes at most a byte offset and a length"}
	}
	buf, err := r.lowerViewBuffer(args[0])
	if err != nil {
		return nil, err
	}
	callArgs := []ast.Expr{buf}
	if len(args) >= 2 {
		if !r.isNumber(args[1]) {
			return nil, &NotYetLowerable{Reason: "a " + name + " byte offset that is not a number is a later slice"}
		}
		offset, err := r.lowerExpr(args[1])
		if err != nil {
			return nil, err
		}
		callArgs = append(callArgs, offset)
	} else {
		callArgs = append(callArgs, &ast.BasicLit{Kind: token.FLOAT, Value: "0"})
	}
	if len(args) == 3 {
		if !r.isNumber(args[2]) {
			return nil, &NotYetLowerable{Reason: "a " + name + " view length that is not a number is a later slice"}
		}
		length, err := r.lowerExpr(args[2])
		if err != nil {
			return nil, err
		}
		callArgs = append(callArgs, length)
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", name+"View"), Args: callArgs}, nil
}

// newDataView lowers a DataView construction, the arbitrary-offset view over an
// ArrayBuffer (25 §25.3.2). new DataView(buffer), new DataView(buffer, byteOffset),
// and new DataView(buffer, byteOffset, byteLength) lower to value.NewDataView(buf),
// value.NewDataView(buf, offset), and value.NewDataView(buf, offset, length), the
// variadic length carrying the optionality the way the typed-array view path does.
// The first argument must be an ArrayBuffer or a SharedArrayBuffer, since a DataView
// has no from-a-length or from-an-array form; a SharedArrayBuffer is unwrapped to its
// underlying run so the view aliases the shared bytes. A byte offset or length that is
// not a number is a later slice and hands back, as does a call with more than three
// arguments.
func (r *Renderer) newDataView(args []frontend.Node) (ast.Expr, error) {
	if len(args) == 0 || !r.isViewBuffer(args[0]) {
		return nil, &NotYetLowerable{Reason: "new DataView takes an ArrayBuffer or a SharedArrayBuffer as its first argument"}
	}
	if len(args) > 3 {
		return nil, &NotYetLowerable{Reason: "new DataView takes at most a byte offset and a length"}
	}
	buf, err := r.lowerViewBuffer(args[0])
	if err != nil {
		return nil, err
	}
	callArgs := []ast.Expr{buf}
	if len(args) >= 2 {
		if !r.isNumber(args[1]) {
			return nil, &NotYetLowerable{Reason: "a DataView byte offset that is not a number is a later slice"}
		}
		offset, err := r.lowerExpr(args[1])
		if err != nil {
			return nil, err
		}
		callArgs = append(callArgs, offset)
	} else {
		callArgs = append(callArgs, &ast.BasicLit{Kind: token.FLOAT, Value: "0"})
	}
	if len(args) == 3 {
		if !r.isNumber(args[2]) {
			return nil, &NotYetLowerable{Reason: "a DataView byte length that is not a number is a later slice"}
		}
		length, err := r.lowerExpr(args[2])
		if err != nil {
			return nil, err
		}
		callArgs = append(callArgs, length)
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "NewDataView"), Args: callArgs}, nil
}

// newArrayBuffer lowers an ArrayBuffer construction, the raw byte backing store of
// section 6.2. new ArrayBuffer(byteLength) lowers to value.NewArrayBuffer(n), a zeroed
// run of that many bytes. The resizable form new ArrayBuffer(n, { maxByteLength: m })
// lowers to value.NewResizableArrayBuffer(n, m), which records the maximum the buffer
// may grow to. Either way the byte length must be a Number; the options argument must
// be an object literal carrying the maxByteLength property, so any other second
// argument hands back.
func (r *Renderer) newArrayBuffer(args []frontend.Node) (ast.Expr, error) {
	if len(args) == 0 || len(args) > 2 {
		return nil, &NotYetLowerable{Reason: "only new ArrayBuffer(byteLength) and new ArrayBuffer(byteLength, { maxByteLength }) are lowered"}
	}
	if !r.isNumber(args[0]) {
		return nil, &NotYetLowerable{Reason: "an ArrayBuffer byte length that is not a number is a later slice"}
	}
	length, err := r.lowerExpr(args[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	if len(args) == 1 {
		return &ast.CallExpr{Fun: sel("value", "NewArrayBuffer"), Args: []ast.Expr{length}}, nil
	}
	max, err := r.arrayBufferMaxByteLength(args[1])
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: sel("value", "NewResizableArrayBuffer"), Args: []ast.Expr{length, max}}, nil
}

// arrayBufferMaxByteLength reads the maxByteLength option from an ArrayBuffer
// constructor's second argument. The argument must be an object literal whose one
// member is a plain maxByteLength property with a Number value, the shape a resizable
// buffer is built with; the value is lowered and returned so the runtime truncates it
// like ToIndex. A different shape (a spread, a computed key, a non-number value, an
// unknown or missing key) hands back, since the maximum then depends on runtime data
// this slice does not thread into the call.
func (r *Renderer) arrayBufferMaxByteLength(n frontend.Node) (ast.Expr, error) {
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return nil, &NotYetLowerable{Reason: "ArrayBuffer options that are not an object literal are a later slice"}
	}
	members := r.prog.Children(n)
	if len(members) != 1 {
		return nil, &NotYetLowerable{Reason: "ArrayBuffer options other than a lone maxByteLength are a later slice"}
	}
	member := members[0]
	if member.Kind() != frontend.NodeUnknown {
		return nil, &NotYetLowerable{Reason: "an ArrayBuffer option that is not a simple property is a later slice"}
	}
	kids := r.prog.Children(member)
	if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier || r.prog.Text(kids[0]) != "maxByteLength" {
		return nil, &NotYetLowerable{Reason: "an ArrayBuffer option other than maxByteLength is a later slice"}
	}
	if !r.isNumber(kids[1]) {
		return nil, &NotYetLowerable{Reason: "a maxByteLength that is not a number is a later slice"}
	}
	return r.lowerExpr(kids[1])
}

// newMap lowers a Map construction, the keyed collection of section 6.5. The empty
// new Map<K, V>() picks the value constructor for the key kind (a number, string,
// boolean, or object key each compares by its own SameValueZero) and instantiates
// it at the value type, so new Map<string, number>() lowers to
// value.NewStringMap[float64](). The key and value come from the map's own type at
// this node, read off its set signature the same way renderMap reads them, so the
// instantiation matches the type a binding of the map is declared with. The
// entries-argument form new Map([[k, v], ...]) fills the empty map from an array
// literal of pairs; any other iterable of pairs is a later slice and hands back.
func (r *Renderer) newMap(n frontend.Node, args []frontend.Node) (ast.Expr, error) {
	valueArgs := r.namedArgs(args)
	if len(valueArgs) > 1 {
		return nil, &NotYetLowerable{Reason: "new Map() with more than one argument is a later slice"}
	}
	k, v, ok := r.mapKeyVal(r.prog.TypeAt(n))
	if !ok {
		return nil, &NotYetLowerable{Reason: "new Map did not expose its key and value types"}
	}
	ctor, err := r.mapCtor(k, v)
	if err != nil {
		return nil, err
	}
	if len(valueArgs) == 0 {
		return ctor, nil
	}
	return r.mapFromPairs(n, ctor, valueArgs[0])
}

// namedArgs returns the value-argument children of a new expression, dropping the
// written type arguments. The children carry the type arguments (new Map<string,
// number>()) ahead of the value arguments, and the frontend leaves a type node
// unnamed so it reads as NodeUnknown; only a real value argument has a named kind,
// so the named children are exactly the value arguments.
func (r *Renderer) namedArgs(args []frontend.Node) []frontend.Node {
	var out []frontend.Node
	for _, a := range args {
		if a.Kind() != frontend.NodeUnknown {
			out = append(out, a)
		}
	}
	return out
}

// mapCtor builds the empty-Map constructor call for a key and value type: the value
// constructor for the key kind, instantiated at the value type. An object key
// compares by reference identity, and objects lower to Go struct pointers, so
// NewRefMap keys on the rendered pointer type; only a pointer render is comparable
// in Go, which arrays (a slice) and functions (a func) are not, so a key whose
// render is not a pointer hands back rather than emit a Go map keyed by an
// incomparable type.
func (r *Renderer) mapCtor(k, v frontend.Type) (ast.Expr, error) {
	vExpr, err := r.typeExpr(v)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	switch {
	case k.Flags&frontend.TypeNumber != 0:
		return &ast.CallExpr{Fun: &ast.IndexExpr{X: sel("value", "NewNumberMap"), Index: vExpr}}, nil
	case k.Flags&frontend.TypeString != 0:
		return &ast.CallExpr{Fun: &ast.IndexExpr{X: sel("value", "NewStringMap"), Index: vExpr}}, nil
	case k.Flags&frontend.TypeBoolean != 0:
		return &ast.CallExpr{Fun: &ast.IndexExpr{X: sel("value", "NewBoolMap"), Index: vExpr}}, nil
	case k.Flags&frontend.TypeObject != 0:
		kExpr, err := r.typeExpr(k)
		if err != nil {
			return nil, err
		}
		if _, ptr := kExpr.(*ast.StarExpr); !ptr {
			return nil, &NotYetLowerable{Reason: "a Map keyed by a reference type that is not a plain object is a later slice"}
		}
		return &ast.CallExpr{Fun: &ast.IndexListExpr{X: sel("value", "NewRefMap"), Indices: []ast.Expr{kExpr, vExpr}}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "a Map with a key that is not a number, string, boolean, or object is a later slice"}
	}
}

// mapFromPairs lowers new Map([[k, v], ...]) by filling the empty map ctor from an
// array literal of two-element array literals: each inner literal's two elements are
// the key and value, lowered directly, which sidesteps a general tuple lowering the
// frontend does not have yet. The fill runs inside a func literal so the whole
// construction stands where a value is expected, the same shape a spread's drain
// takes. A Map from any source other than an array literal of pairs hands back.
func (r *Renderer) mapFromPairs(n frontend.Node, ctor ast.Expr, arg frontend.Node) (ast.Expr, error) {
	if arg.Kind() != frontend.NodeArrayLiteralExpression {
		return nil, &NotYetLowerable{Reason: "new Map from a source that is not an array literal of pairs is a later slice"}
	}
	collType, err := r.renderMap(r.prog.TypeAt(n))
	if err != nil {
		return nil, err
	}
	tmp := r.freshTemp()
	stmts := []ast.Stmt{&ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{ctor}}}
	for _, pair := range r.prog.Children(arg) {
		if pair.Kind() != frontend.NodeArrayLiteralExpression {
			return nil, &NotYetLowerable{Reason: "new Map from an entry that is not a literal pair is a later slice"}
		}
		kv := r.prog.Children(pair)
		if len(kv) != 2 {
			return nil, &NotYetLowerable{Reason: "new Map from an entry that is not a two-element pair is a later slice"}
		}
		kExpr, err := r.lowerExpr(kv[0])
		if err != nil {
			return nil, err
		}
		vExpr, err := r.lowerExpr(kv[1])
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, &ast.ExprStmt{X: &ast.CallExpr{
			Fun:  &ast.SelectorExpr{X: ident(tmp), Sel: ident("Set")},
			Args: []ast.Expr{kExpr, vExpr},
		}})
	}
	stmts = append(stmts, &ast.ReturnStmt{Results: []ast.Expr{ident(tmp)}})
	return r.collectionIIFE(collType, stmts), nil
}

// collectionIIFE wraps the build-and-fill statements for a collection in a
// no-argument func literal returning collType and calls it, so a Map or Set built
// from an argument stands in expression position the way an empty constructor does.
func (r *Renderer) collectionIIFE(collType ast.Expr, body []ast.Stmt) ast.Expr {
	fn := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{},
			Results: &ast.FieldList{List: []*ast.Field{{Type: collType}}},
		},
		Body: &ast.BlockStmt{List: body},
	}
	return &ast.CallExpr{Fun: fn}
}

// mapStaticCall lowers a call on the global Map namespace, Map.<method>(...). The
// only static Map method the area tests reach is Map.groupBy, the ES2024 grouping
// constructor; any other static Map surface is its own later slice.
func (r *Renderer) mapStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "groupBy" {
		return nil, &NotYetLowerable{Reason: "a static Map." + method + " call is a later slice"}
	}
	return r.mapGroupBy(argNodes)
}

// mapGroupBy lowers Map.groupBy(items, cb) to a Map<K, T[]> that groups the items
// of an array by the key the callback returns, in first-seen key order. It builds
// the map inline in a func literal: the callback lowers to a Go func value, the
// source's elements range in order, and each item appends into the slice its key
// already holds or starts a new one. The key type is the callback's result type, so
// the empty-map constructor is chosen the same way new Map() chooses it, and the
// value type is a slice of the source's element type.
//
// Only an array source and an inline arrow taking the item, or the item and its
// index, are covered. A non-array iterable needs the general iterable drain, and a
// callback passed as a named reference needs the reference resolved to a func value
// first, so each hands back rather than mislower the grouping.
func (r *Renderer) mapGroupBy(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "Map.groupBy with other than an items source and a callback is a later slice"}
	}
	itemsNode, cb := argNodes[0], argNodes[1]
	itemsType := r.prog.TypeAt(itemsNode)
	elemFT, ok := r.prog.ElementType(itemsType)
	if itemsType.Flags&frontend.TypeObject == 0 || !ok {
		return nil, &NotYetLowerable{Reason: "Map.groupBy over a source that is not an array is a later slice"}
	}
	elemExpr, err := r.typeExpr(elemFT)
	if err != nil {
		return nil, err
	}
	if cb.Kind() != frontend.NodeArrowFunction {
		return nil, &NotYetLowerable{Reason: "Map.groupBy with a callback that is not an inline arrow function is a later slice"}
	}
	params := r.arrowParamCount(cb)
	if params != 1 && params != 2 {
		return nil, &NotYetLowerable{Reason: "Map.groupBy with a callback that reads more than the item and its index is a later slice"}
	}
	keyType, ok := r.arrowResultFrontendType(cb)
	if !ok {
		return nil, &NotYetLowerable{Reason: "Map.groupBy callback with a block body has no key type"}
	}
	// A callback that returns a literal, or a union of literals, types the key as
	// that literal type ("even" | "odd"), which carries no primitive-kind flag. The
	// key of the built map is the primitive the literals belong to, so widening picks
	// the map constructor and spells the key type the same way a string key would.
	keyType = r.prog.Widen(keyType)
	keyExpr, err := r.typeExpr(keyType)
	if err != nil {
		return nil, err
	}
	// The value type is T[], which lowers to *value.Array[T], so each group is a
	// value array the same shape a T[] binding takes. arrType spells it freshly at
	// each use so no AST node is shared across the emitted tree.
	arrType := func() ast.Expr { return star(index(sel("value", "Array"), elemExpr)) }
	ctor, err := r.groupCtor(keyType, keyExpr, arrType())
	if err != nil {
		return nil, err
	}
	fn, err := r.lowerExpr(cb)
	if err != nil {
		return nil, err
	}
	items, err := r.lowerExpr(itemsNode)
	if err != nil {
		return nil, err
	}
	cbName, mName, itName := r.freshTemp(), r.freshTemp(), r.freshTemp()
	keyName, groupName := r.freshTemp(), r.freshTemp()
	// _cb(_it) for a one-parameter callback, _cb(_it, float64(_i)) when it also reads
	// the index. The index binding names the range key only in that case, so a
	// one-parameter loop ranges the value alone and drops the index to blank.
	idxExpr := ident("_")
	callArgs := []ast.Expr{ident(itName)}
	if params == 2 {
		iName := r.freshTemp()
		idxExpr = ident(iName)
		callArgs = append(callArgs, &ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{ident(iName)}})
	}
	getCall := &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(mName), Sel: ident("Get")}, Args: []ast.Expr{ident(keyName)}}
	loopBody := []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ident(keyName)}, Tok: token.DEFINE, Rhs: []ast.Expr{
			&ast.CallExpr{Fun: ident(cbName), Args: callArgs},
		}},
		// if _g := _m.Get(_k); !_g.IsUndefined() { _g.Get().Push(_it) } else {
		// _m.Set(_k, value.NewArray[T](_it)) } appends into the key's existing group
		// or starts a one-element group the first time the key is seen. The stored
		// group is a *value.Array, so Push mutates the group the map already holds.
		&ast.IfStmt{
			Init: &ast.AssignStmt{Lhs: []ast.Expr{ident(groupName)}, Tok: token.DEFINE, Rhs: []ast.Expr{getCall}},
			Cond: &ast.UnaryExpr{Op: token.NOT, X: &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(groupName), Sel: ident("IsUndefined")}}},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(groupName), Sel: ident("Get")}},
					Sel: ident("Push"),
				},
				Args: []ast.Expr{ident(itName)},
			}}}},
			Else: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: &ast.CallExpr{
				Fun: &ast.SelectorExpr{X: ident(mName), Sel: ident("Set")},
				Args: []ast.Expr{ident(keyName), &ast.CallExpr{
					Fun:  index(sel("value", "NewArray"), elemExpr),
					Args: []ast.Expr{ident(itName)},
				}},
			}}}},
		},
	}
	body := []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ident(cbName)}, Tok: token.DEFINE, Rhs: []ast.Expr{fn}},
		&ast.AssignStmt{Lhs: []ast.Expr{ident(mName)}, Tok: token.DEFINE, Rhs: []ast.Expr{ctor}},
		&ast.RangeStmt{
			Key:   idxExpr,
			Value: ident(itName),
			Tok:   token.DEFINE,
			X:     &ast.CallExpr{Fun: &ast.SelectorExpr{X: items, Sel: ident("Elems")}},
			Body:  &ast.BlockStmt{List: loopBody},
		},
		&ast.ReturnStmt{Results: []ast.Expr{ident(mName)}},
	}
	collType := star(&ast.IndexListExpr{X: sel("value", "Map"), Indices: []ast.Expr{keyExpr, arrType()}})
	return r.collectionIIFE(collType, body), nil
}

// groupCtor builds the empty Map<K, T[]> constructor for Map.groupBy: the value
// constructor for the key kind, instantiated at the group value type. It mirrors
// mapCtor's key-kind switch and reference-key pointer guard, but the value type is
// an already-lowered array expression rather than a checker type, since groupBy
// synthesizes it from the source's element type.
func (r *Renderer) groupCtor(keyType frontend.Type, keyExpr, valExpr ast.Expr) (ast.Expr, error) {
	r.requireImport(valuePkg)
	switch {
	case keyType.Flags&frontend.TypeNumber != 0:
		return &ast.CallExpr{Fun: &ast.IndexExpr{X: sel("value", "NewNumberMap"), Index: valExpr}}, nil
	case keyType.Flags&frontend.TypeString != 0:
		return &ast.CallExpr{Fun: &ast.IndexExpr{X: sel("value", "NewStringMap"), Index: valExpr}}, nil
	case keyType.Flags&frontend.TypeBoolean != 0:
		return &ast.CallExpr{Fun: &ast.IndexExpr{X: sel("value", "NewBoolMap"), Index: valExpr}}, nil
	case keyType.Flags&frontend.TypeObject != 0:
		if _, ptr := keyExpr.(*ast.StarExpr); !ptr {
			return nil, &NotYetLowerable{Reason: "Map.groupBy keyed by a reference type that is not a plain object is a later slice"}
		}
		return &ast.CallExpr{Fun: &ast.IndexListExpr{X: sel("value", "NewRefMap"), Indices: []ast.Expr{keyExpr, valExpr}}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Map.groupBy with a key that is not a number, string, boolean, or object is a later slice"}
	}
}

// newSet lowers a Set construction, the collection of unique members of section
// 6.5. The empty new Set<T>() picks the value constructor for the member kind (a
// number, string, boolean, or object member each compares by its own
// SameValueZero), so new Set<string>() lowers to value.NewStringSet(). Unlike a Map,
// whose value type is free and so needs a type argument, the member type fully
// determines the primitive Set constructor, so the call carries no instantiation.
// The member type comes from the set's own type at this node, read off its add
// signature the same way renderSet reads it. The iterable-argument form new
// Set(iterable) fills the empty set from an array or a user iterable; a source that
// is neither hands back.
func (r *Renderer) newSet(n frontend.Node, args []frontend.Node) (ast.Expr, error) {
	valueArgs := r.namedArgs(args)
	if len(valueArgs) > 1 {
		return nil, &NotYetLowerable{Reason: "new Set() with more than one argument is a later slice"}
	}
	elem, ok := r.setElem(r.prog.TypeAt(n))
	if !ok {
		return nil, &NotYetLowerable{Reason: "new Set did not expose its member type"}
	}
	ctor, err := r.setCtor(elem)
	if err != nil {
		return nil, err
	}
	if len(valueArgs) == 0 {
		return ctor, nil
	}
	return r.setFromIterable(n, ctor, valueArgs[0])
}

// setCtor builds the empty-Set constructor call for a member type. An object member
// compares by reference identity, and objects lower to Go struct pointers, so
// NewRefSet keys on the rendered pointer type; only a pointer render is comparable
// in Go, which arrays and functions are not, so a member whose render is not a
// pointer hands back rather than emit a Set backed by an incomparable member type.
func (r *Renderer) setCtor(elem frontend.Type) (ast.Expr, error) {
	r.requireImport(valuePkg)
	switch {
	case elem.Flags&frontend.TypeNumber != 0:
		return &ast.CallExpr{Fun: sel("value", "NewNumberSet")}, nil
	case elem.Flags&frontend.TypeString != 0:
		return &ast.CallExpr{Fun: sel("value", "NewStringSet")}, nil
	case elem.Flags&frontend.TypeBoolean != 0:
		return &ast.CallExpr{Fun: sel("value", "NewBoolSet")}, nil
	case elem.Flags&frontend.TypeObject != 0:
		elemExpr, err := r.typeExpr(elem)
		if err != nil {
			return nil, err
		}
		if _, ptr := elemExpr.(*ast.StarExpr); !ptr {
			return nil, &NotYetLowerable{Reason: "a Set of a reference type that is not a plain object is a later slice"}
		}
		return &ast.CallExpr{Fun: &ast.IndexExpr{X: sel("value", "NewRefSet"), Index: elemExpr}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "a Set with a member that is not a number, string, boolean, or object is a later slice"}
	}
}

// setFromIterable lowers new Set(iterable) by filling the empty set ctor from the
// source's elements, ranged inside a func literal so the whole construction stands
// where a value is expected. The elements come from iterableElems, which ranges an
// array's backing slice or drains a user iterable through the iterator protocol; the
// checker has typed the source as Iterable<T> for the set's member T, so each ranged
// element adds with no conversion.
func (r *Renderer) setFromIterable(n frontend.Node, ctor ast.Expr, arg frontend.Node) (ast.Expr, error) {
	collType, err := r.renderSet(r.prog.TypeAt(n))
	if err != nil {
		return nil, err
	}
	elems, err := r.iterableElems(arg)
	if err != nil {
		return nil, err
	}
	tmp := r.freshTemp()
	member := r.freshTemp()
	add := &ast.ExprStmt{X: &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ident(tmp), Sel: ident("Add")},
		Args: []ast.Expr{ident(member)},
	}}
	stmts := []ast.Stmt{
		&ast.AssignStmt{Lhs: []ast.Expr{ident(tmp)}, Tok: token.DEFINE, Rhs: []ast.Expr{ctor}},
		&ast.RangeStmt{
			Key:   ident("_"),
			Value: ident(member),
			Tok:   token.DEFINE,
			X:     elems,
			Body:  &ast.BlockStmt{List: []ast.Stmt{add}},
		},
		&ast.ReturnStmt{Results: []ast.Expr{ident(tmp)}},
	}
	return r.collectionIIFE(collType, stmts), nil
}

// iterableElems returns a Go slice expression of a source's elements, for a
// collection constructor to range and add. An array (or array literal) ranges its
// backing slice through Elems; a user iterable that defines [Symbol.iterator] is
// drained through the iterator protocol into a slice of its element type. A source
// that is neither hands back, since a built-in Set, Map, or string iterable needs
// its own walk a later slice brings.
func (r *Renderer) iterableElems(arg frontend.Node) (ast.Expr, error) {
	if isArrayElem(r, arg) {
		src, err := r.lowerExpr(arg)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: src, Sel: ident("Elems")}}, nil
	}
	if shape, ok := r.symbolIteratorShape(r.prog.TypeAt(arg)); ok {
		elemType, err := r.typeExpr(shape.elem)
		if err != nil {
			return nil, err
		}
		src, err := r.lowerExpr(arg)
		if err != nil {
			return nil, err
		}
		return r.iterableToSliceExpr(src, elemType, shape), nil
	}
	return nil, &NotYetLowerable{Reason: "a collection from a source that is not an array or a user iterable is a later slice"}
}

// errorInstanceof lowers `e instanceof Error` on a caught error, the guard a
// catch narrows its binding with before reading the error. The checker types a
// catch binding unknown, so the only way a catch reads .message or .name is to
// narrow first, and instanceof is that narrowing (04_frontend_typescript_go.md,
// the narrowing table). The left operand must be a catch binding in scope and the
// right a built-in error constructor; the test lowers to the IsA name check, since
// the runtime models the error family as one type with a name rather than
// distinct Go types. It reports handled=false when the left is not a caught error,
// so a general instanceof still hands back, and an error against a non-error
// constructor hands back explicitly.
func (r *Renderer) errorInstanceof(left, right frontend.Node) (ast.Expr, bool, error) {
	if left.Kind() != frontend.NodeIdentifier {
		return nil, false, nil
	}
	name, ok := localName(r.prog.Text(left))
	if !ok || !r.errorLocals[name] {
		return nil, false, nil
	}
	ctor := r.prog.Text(right)
	// e instanceof GoError narrows a catch binding to the Go-error surface of section
	// 7.7, the guard an author writes before reading err.is. It lowers to the
	// *value.Error IsGoError test, which is true when the caught error carries a Go
	// error behind it: a caught go: failure narrows through, a program-thrown error
	// does not. GoError is the bento:go vocabulary class, not a built-in error, so it
	// routes before the errorCtors check below.
	if ctor == "GoError" {
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("IsGoError")}}, true, nil
	}
	if _, isErr := errorCtors[ctor]; !isErr {
		return nil, false, &NotYetLowerable{Reason: "instanceof a class other than a built-in error on a caught error is a later slice"}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun:  &ast.SelectorExpr{X: ident(name), Sel: ident("IsA")},
		Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(ctor)}},
	}, true, nil
}

// errorMethodCall lowers a method call on a caught error, the error-identity
// surface of section 7.7. Only is() is covered here: err.is(sentinel) lowers to
// the *value.Error Is method against the Go error the caught error carries, so the
// comparison is errors.Is against the original error and a caught go: failure
// branches on identity the way Go code does. The argument must be a reference to a
// go: sentinel error variable (io.EOF and its kind); anything else hands back,
// since a comparison against a value that is not a Go error would be unsound. as()
// waits on the concrete-struct projection it unwraps into, and any other method on
// a caught error is its own later slice.
func (r *Renderer) errorMethodCall(name, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "is":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "err.is takes exactly one sentinel argument"}
		}
		sentinel, ok := r.goErrorSentinelRef(argNodes[0])
		if !ok {
			return nil, &NotYetLowerable{Reason: "err.is against a value that is not a go: sentinel error is a later slice"}
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("Is")}, Args: []ast.Expr{sentinel}}, nil
	case "as":
		return nil, &NotYetLowerable{Reason: "err.as waits on concrete Go error type projection"}
	case "toString":
		// err.toString() is Error.prototype.toString, the "Name: message" form the
		// *value.Error produces through ToBStr, the same coercion String(err) and a
		// template substitution take. It carries no argument, so a call with one hands
		// back rather than silently ignore it. The result boxes into a string value:
		// e.toString() is any-typed (the binding is unknown to the checker), so it must
		// flow as a value.Value the way a dynamic receiver's toString does, not a bare
		// BStr, so a console.log or concatenation coerces it through the dynamic path.
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "a caught error's toString takes no argument"}
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{
			Fun:  sel("value", "StringValue"),
			Args: []ast.Expr{&ast.CallExpr{Fun: &ast.SelectorExpr{X: ident(name), Sel: ident("ToBStr")}}},
		}, nil
	default:
		return nil, &NotYetLowerable{Reason: "a caught error's ." + method + "() is a later slice"}
	}
}

// lowerThrow lowers a throw statement to a panic carrying the thrown error, the
// raise half of the exception model. Only throwing a built-in error is covered
// (throw new Error("..."), or throwing a variable bound to one), because the
// thrown value must be a value.Error for the panic to round-trip through the
// runtime's recover; throwing a bare string or number is a later slice that boxes
// the operand. Emitting the throw records that the program can raise, so the
// assembled main defers the uncaught-error reporter.
func (r *Renderer) lowerThrow(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) != 1 {
		return nil, &NotYetLowerable{Reason: "throw did not expose a single operand"}
	}
	operand, err := r.thrownOperand(kids[0])
	if err != nil {
		return nil, err
	}
	return &ast.ExprStmt{X: &ast.CallExpr{Fun: sel("value", "Throw"), Args: []ast.Expr{operand}}}, nil
}

// thrownOperand lowers a throw's operand to the Go expression that carries the
// runtime's Thrown surface, the payload value.Throw and a generator's throw(e) both
// raise. It is the shared conversion behind a throw statement and a generator throw:
// a caught error re-raises the exact *value.Error it bound, a new built-in error or a
// registered error class throws the instance itself, and a thrown string wraps in
// value.ThrownString. Any other operand hands back until arbitrary thrown values box.
func (r *Renderer) thrownOperand(node frontend.Node) (ast.Expr, error) {
	// Re-throwing a caught error is the rethrow half of the exception model:
	// catch (err) { ...; throw err } passes the recovered value straight back up.
	// The binding is already a *value.Error, which carries the runtime's Thrown
	// surface, so the throw re-raises that exact value with its identity intact.
	// Reading the binding any other way still hands back at expr.go, so this is
	// spelled out here rather than routed through lowerExpr, which would decline.
	if node.Kind() == frontend.NodeIdentifier {
		if name, ok := localName(r.prog.Text(node)); ok && r.errorLocals[name] {
			r.usesThrow = true
			return ident(name), nil
		}
	}
	if !r.isThrowable(node) {
		// A new expression of a registered class whose instances carry a string
		// message throws too: the instance itself is the panic payload, and the
		// class gains the ErrorName and ErrorMessage methods (renderClasses) that
		// satisfy the runtime's Thrown surface, so a catch that recovers it binds
		// an error named after the class carrying the instance's message. A
		// thrown string wraps in the runtime's ThrownString, which carries the
		// same surface with the string as the name.
		if info, ok := r.thrownClassOf(node); ok {
			info.thrownAsError = true
		} else if r.isString(node) {
			operand, err := r.lowerExpr(node)
			if err != nil {
				return nil, err
			}
			r.usesThrow = true
			return &ast.CallExpr{Fun: sel("value", "ThrownString"), Args: []ast.Expr{operand}}, nil
		} else {
			return nil, &NotYetLowerable{Reason: "throwing a value that is not a built-in error is a later slice"}
		}
	}
	operand, err := r.lowerExpr(node)
	if err != nil {
		return nil, err
	}
	r.usesThrow = true
	return operand, nil
}

// isThrowable reports whether a throw operand lowers to a value.Error the runtime
// can raise and recover. Only a new expression for a built-in error qualifies here;
// re-throwing a caught error is recognized earlier in lowerThrow, where the binding
// is known to be a *value.Error. A locally constructed error variable still hands
// back until its type flows through, and any other operand hands back until
// arbitrary thrown values are boxed.
func (r *Renderer) isThrowable(n frontend.Node) bool {
	if n.Kind() == frontend.NodeNewExpression {
		kids := r.prog.Children(n)
		if len(kids) == 0 {
			return false
		}
		_, ok := errorCtors[r.prog.Text(kids[0])]
		return ok
	}
	return false
}

// thrownClassOf reports the registered class a throw's new expression
// constructs, when its instances can ride the runtime's throw path: the class
// declares its own string message field, so the Thrown surface the panic
// payload needs (a name and a message) reads straight off the instance. A
// class without one hands back rather than throw a value a catch could not
// read.
func (r *Renderer) thrownClassOf(n frontend.Node) (*classInfo, bool) {
	if n.Kind() != frontend.NodeNewExpression {
		return nil, false
	}
	kids := r.prog.Children(n)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier {
		return nil, false
	}
	info, ok := r.classNameRef(kids[0])
	if !ok {
		return nil, false
	}
	for _, f := range info.fields {
		if f.prop == "message" && r.prog.TypeAt(f.ident).Flags&frontend.TypeString != 0 {
			return info, true
		}
	}
	return nil, false
}

// tryRetMode is how a return statement leaves the try construct it sits in;
// the modes are documented on the Renderer's tryRet field. tryRetDeferPlain is
// the deferred-handler mode of the always-returning form, whose closure carries
// only the value: the assignment fills ret alone, with no done to raise.
type tryRetMode int

const (
	tryRetNone tryRetMode = iota
	tryRetBody
	tryRetDefer
	tryRetDeferPlain
)

// lowerTry lowers a try/catch/finally to a Go closure over panic and recover. A
// throw inside the try body is a panic, so the try body runs inside an immediately
// invoked function whose deferred functions handle it: a catch is a deferred
// recover that binds the caught error and runs the catch body, and a finally is a
// deferred call that runs whether or not the body threw. The two are deferred in
// order so the catch runs first and the finally last, matching the language:
// finally runs after the catch, and after a normal completion too.
//
// A try none of whose bodies return stays that plain closure. A body that
// returns compiles to the escape form instead: the closure takes named results
// (ret T, done bool), a return in the try body fills them with `return x,
// true`, a return in a catch or finally assigns them from inside the deferred
// function, and the call site turns done back into the enclosing function's
// return. A break or continue inside a body still hands back at the statement
// lowering.
func (r *Renderer) lowerTry(n frontend.Node) (ast.Stmt, error) {
	kids := r.prog.Children(n)
	if len(kids) == 0 || kids[0].Kind() != frontend.NodeBlock {
		return nil, &NotYetLowerable{Reason: "try did not expose a block body"}
	}
	tryBlock := kids[0]

	var catchClause, finallyBlock frontend.Node
	var hasCatch, hasFinally bool
	for _, k := range kids[1:] {
		if k.Kind() == frontend.NodeBlock {
			finallyBlock, hasFinally = k, true
		} else {
			catchClause, hasCatch = k, true
		}
	}
	if !hasCatch && !hasFinally {
		return nil, &NotYetLowerable{Reason: "a try with neither catch nor finally is a later slice"}
	}
	// A try, catch, and finally body all lower inside a Go closure, and Go's
	// break and continue cannot leave the function they sit in. A break or
	// continue in one of those bodies that targets a loop or switch enclosing
	// the whole try would compile to a branch with no loop around it, so it
	// hands back rather than emit Go the compiler rejects. A branch captured by
	// a loop, switch, or label inside the body stays put and lowers normally.
	if r.branchEscapesClosure(tryBlock) ||
		(hasCatch && r.branchEscapesClosure(catchClause)) ||
		(hasFinally && r.branchEscapesClosure(finallyBlock)) {
		return nil, &NotYetLowerable{Reason: "a break or continue leaving the try's closure to an enclosing loop is a later slice"}
	}
	escapes := r.blockReturns(tryBlock) ||
		(hasCatch && r.blockReturns(catchClause)) ||
		(hasFinally && r.blockReturns(finallyBlock))
	if escapes {
		return r.lowerTryEscape(n, tryBlock, catchClause, finallyBlock, hasCatch, hasFinally)
	}

	var closureBody []ast.Stmt

	// The finally defer is added first so it runs last, after the catch and after a
	// normal completion, the language's ordering.
	if hasFinally {
		finStmts, err := r.lowerBlock(finallyBlock)
		if err != nil {
			return nil, err
		}
		closureBody = append(closureBody, &ast.DeferStmt{Call: callClosure(finStmts.List)})
	}

	if hasCatch {
		catchDefer, err := r.catchDefer(catchClause, false)
		if err != nil {
			return nil, err
		}
		closureBody = append(closureBody, catchDefer)
	}

	tryStmts, err := r.lowerBlock(tryBlock)
	if err != nil {
		return nil, err
	}
	closureBody = append(closureBody, tryStmts.List...)

	return &ast.ExprStmt{X: callClosure(closureBody)}, nil
}

// lowerTryEscape lowers a try one of whose bodies returns. Two forms cover it,
// both closures whose named results a deferred catch or finally can fill.
//
// When every path through the try body and the catch ends in a return or a
// throw, the construct as a whole always returns, so it lowers to returning
// the closure directly, and the enclosing function needs nothing after it:
//
//	return func() (ret T) { ... }()
//
// A return in that try body stays a plain Go return; a return in the catch or
// finally assigns ret and leaves the handler with a bare return.
//
// Otherwise control can run past the construct, so the closure carries a
// second result saying whether a body returned, and the call site turns it
// back into a real return:
//
//	if ret, done := func() (ret T, done bool) { ... }(); done {
//		return ret
//	}
//
// There a return in the try body is `return x, true`, and a return in a catch
// or finally assigns both named results. A function returning nothing always
// takes the done form with ret dropped, since it has no missing-return
// obligation the always form would be needed for.
func (r *Renderer) lowerTryEscape(n, tryBlock, catchClause, finallyBlock frontend.Node, hasCatch, hasFinally bool) (ast.Stmt, error) {
	if r.tryRet == tryRetDefer || r.tryRet == tryRetDeferPlain {
		return nil, &NotYetLowerable{Reason: "a try with an escaping return nested in a catch or finally is a later slice"}
	}
	// The closure's named results live in the same scope as the bodies, so a
	// source binding or reference named ret or done inside the construct would
	// resolve to them rather than to the source's own variable; that shadowing
	// hazard hands back rather than miscompile.
	if r.mentionsName(n, "ret") || r.mentionsName(n, "done") {
		return nil, &NotYetLowerable{Reason: "a name colliding with the try escape results (ret, done) is a later slice"}
	}
	valued := !isVoidReturn(r.retType)
	always := valued && r.alwaysAbrupt(r.prog.Children(tryBlock)) &&
		(!hasCatch || r.alwaysAbrupt(r.prog.Children(r.catchBlockOf(catchClause))))

	bodyMode, deferMode := tryRetBody, tryRetDefer
	if always {
		// The always form's closure returns the plain value, so returns in the
		// try body need no rewriting at all, and the handlers fill only ret.
		bodyMode, deferMode = tryRetNone, tryRetDeferPlain
	}

	var closureBody []ast.Stmt
	if hasFinally {
		prev := r.tryRet
		r.tryRet = deferMode
		finStmts, err := r.lowerBlock(finallyBlock)
		r.tryRet = prev
		if err != nil {
			return nil, err
		}
		closureBody = append(closureBody, &ast.DeferStmt{Call: callClosure(finStmts.List)})
	}
	if hasCatch {
		prev := r.tryRet
		r.tryRet = deferMode
		catchDefer, err := r.catchDefer(catchClause, true)
		r.tryRet = prev
		if err != nil {
			return nil, err
		}
		closureBody = append(closureBody, catchDefer)
	}
	prev := r.tryRet
	r.tryRet = bodyMode
	tryStmts, err := r.lowerBlock(tryBlock)
	r.tryRet = prev
	if err != nil {
		return nil, err
	}
	closureBody = append(closureBody, tryStmts.List...)
	// The closure's tail return is the fall-off path; a body already ending in
	// a Go return needs none, and in the always form a throw ends the body
	// instead, whose panic the tail return sits after only syntactically.
	if len(closureBody) == 0 || !isGoReturn(closureBody[len(closureBody)-1]) {
		closureBody = append(closureBody, &ast.ReturnStmt{})
	}

	results := &ast.FieldList{}
	if valued {
		rt, err := r.typeExpr(r.retType)
		if err != nil {
			return nil, err
		}
		results.List = append(results.List, &ast.Field{Names: []*ast.Ident{ident("ret")}, Type: rt})
	}
	if !always {
		results.List = append(results.List, &ast.Field{Names: []*ast.Ident{ident("done")}, Type: ident("bool")})
	}
	closure := &ast.CallExpr{Fun: &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}, Results: results},
		Body: &ast.BlockStmt{List: closureBody},
	}}

	if always {
		// Inside an enclosing escape closure this return itself raises that
		// closure's done.
		if r.tryRet == tryRetBody {
			return &ast.ReturnStmt{Results: []ast.Expr{closure, ident("true")}}, nil
		}
		return &ast.ReturnStmt{Results: []ast.Expr{closure}}, nil
	}

	// The done form's call site: bind the results and return when a body did.
	// Inside an enclosing escape closure the emitted return fills that closure's
	// named results; the if-init's ret shadows the outer one only for the
	// return's operand, which is exactly the value being handed up.
	var lhs []ast.Expr
	var thenRet *ast.ReturnStmt
	if valued {
		lhs = []ast.Expr{ident("ret"), ident("done")}
		if r.tryRet == tryRetBody {
			thenRet = &ast.ReturnStmt{Results: []ast.Expr{ident("ret"), ident("true")}}
		} else {
			thenRet = &ast.ReturnStmt{Results: []ast.Expr{ident("ret")}}
		}
	} else {
		lhs = []ast.Expr{ident("done")}
		if r.tryRet == tryRetBody {
			thenRet = &ast.ReturnStmt{Results: []ast.Expr{ident("true")}}
		} else {
			thenRet = &ast.ReturnStmt{}
		}
	}
	return &ast.IfStmt{
		Init: &ast.AssignStmt{Lhs: lhs, Tok: token.DEFINE, Rhs: []ast.Expr{closure}},
		Cond: ident("done"),
		Body: &ast.BlockStmt{List: []ast.Stmt{thenRet}},
	}, nil
}

// catchBlockOf returns the block body of a catch clause, or the clause itself
// when it has none, which only makes the abruptness analysis answer false.
func (r *Renderer) catchBlockOf(catchClause frontend.Node) frontend.Node {
	for _, k := range r.prog.Children(catchClause) {
		if k.Kind() == frontend.NodeBlock {
			return k
		}
	}
	return catchClause
}

// alwaysAbrupt reports whether a statement list cannot complete normally: its
// last statement returns, throws, or is an if-else (or block) whose every arm
// does. It is deliberately conservative; a shape it cannot see answers false
// and only costs the try escape its done result.
func (r *Renderer) alwaysAbrupt(stmts []frontend.Node) bool {
	if len(stmts) == 0 {
		return false
	}
	return r.stmtAbrupt(stmts[len(stmts)-1])
}

// stmtAbrupt reports whether one statement cannot complete normally, the
// per-statement half of alwaysAbrupt.
func (r *Renderer) stmtAbrupt(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeReturnStatement, frontend.NodeThrowStatement:
		return true
	case frontend.NodeBlock:
		return r.alwaysAbrupt(r.prog.Children(n))
	case frontend.NodeIfStatement:
		kids := r.prog.Children(n)
		return len(kids) >= 3 && r.stmtAbrupt(kids[1]) && r.stmtAbrupt(kids[2])
	}
	return false
}

// isGoReturn reports whether a lowered statement is a Go return, the check the
// try escape uses to skip its redundant tail return.
func isGoReturn(s ast.Stmt) bool {
	_, ok := s.(*ast.ReturnStmt)
	return ok
}

// catchDefer builds the deferred recover that runs a catch clause. It recovers the
// panic, and when something was thrown it converts the payload to the *value.Error
// the binding names (value.Caught re-panics a Go runtime bug so a genuine crash is
// not swallowed) and runs the catch body. A catch with no binding still recovers
// and converts, so a Go runtime panic is re-raised rather than silently caught, but
// discards the error. allowReturns is set by the escape form, whose named results
// a catch return can fill; the plain form keeps its hand-back on a returning
// catch, since its closure has nowhere to carry the value.
func (r *Renderer) catchDefer(catchClause frontend.Node, allowReturns bool) (ast.Stmt, error) {
	var binding, bindingJS string
	var catchPattern frontend.Node
	var catchBlock frontend.Node
	for _, k := range r.prog.Children(catchClause) {
		switch k.Kind() {
		case frontend.NodeBlock:
			catchBlock = k
		case frontend.NodeVariableDeclaration:
			// The binding is the declaration's leading identifier; a type
			// annotation (catch (err: any), the strict-checker spelling) rides
			// along as a trailing child and changes nothing, since the binding
			// is the *value.Error the recover converts regardless of what the
			// author annotated. A binding pattern binds through the dynamic path.
			vk := r.prog.Children(k)
			if len(vk) == 0 || vk[0].Kind() != frontend.NodeIdentifier {
				// The caught value has no static type, so a pattern over it is a
				// dynamic-sourced destructuring: each element reads off the boxed
				// value.Value through the dynamic Get and GetIndex protocol, the same
				// as an untyped pattern anywhere else. The caught error boxes to that
				// value through value.Caught(rec).ToValue(), and the pattern binds
				// against it below. Only an object or array pattern binds this way; a
				// stranger shape hands back.
				if len(vk) == 0 {
					return nil, &NotYetLowerable{Reason: "a catch clause exposed no binding"}
				}
				txt := strings.TrimSpace(r.prog.Text(vk[0]))
				if !strings.HasPrefix(txt, "{") && !strings.HasPrefix(txt, "[") {
					return nil, &NotYetLowerable{Reason: "an unusual catch binding is a later slice"}
				}
				catchPattern = vk[0]
				continue
			}
			name, ok := localName(r.prog.Text(vk[0]))
			if !ok {
				return nil, &NotYetLowerable{Reason: "catch binding is not a Go identifier"}
			}
			binding = name
			bindingJS = strings.TrimSpace(r.prog.Text(vk[0]))
		default:
			return nil, &NotYetLowerable{Reason: "an unusual catch clause is a later slice"}
		}
	}
	if catchBlock == nil {
		return nil, &NotYetLowerable{Reason: "catch clause did not expose a block body"}
	}
	if !allowReturns && r.blockReturns(catchBlock) {
		return nil, &NotYetLowerable{Reason: "a return that escapes a try, catch, or finally is a later slice"}
	}
	// The binding recovers as a *value.Error and the catch body reads it as that
	// error for its whole extent. Reassigning it to another value (a = 4) would
	// store into the *value.Error local, and shadowing it with a nested declaration
	// (let a = 3) would still read the inner name through the error's methods, both
	// of which emit Go that does not build. Model neither yet, so hand back when the
	// binding is rebound or shadowed anywhere in the body. block-scope/shadowing/
	// catch-parameter-shadowing-let-declaration and let-declaration-shadowing-catch-parameter
	// hit these.
	if bindingJS != "" && r.nameReboundOrShadowed(catchBlock, bindingJS) {
		return nil, &NotYetLowerable{Reason: "a catch binding reassigned or shadowed inside its body is a later slice"}
	}

	// The binding is in scope only while the catch block is lowered, so a read of its
	// .message or .name resolves to the error and a use elsewhere hands back.
	if binding != "" {
		prev := r.errorLocals[binding]
		r.errorLocals[binding] = true
		defer func() { r.errorLocals[binding] = prev }()
	}
	r.requireImport(valuePkg)
	// bind is `e := value.Caught(rec)` when the clause names the error, or a bare
	// `value.Caught(rec)` call when it does not, so a Go runtime panic re-raises
	// either way. A named binding is also assigned to blank so a catch that never
	// reads the error still compiles.
	caught := &ast.CallExpr{Fun: sel("value", "Caught"), Args: []ast.Expr{ident("rec")}}

	// A destructured catch binding reads its elements off the caught value's boxed form:
	// value.Caught(rec).ToValue() gives the thrown object or primitive as a value.Value,
	// held once in a temporary the pattern binds against through the same dynamic protocol
	// an untyped pattern uses everywhere else. The pattern binds before the block lowers so
	// every name it introduces is marked dynamic: the checker gives a catch-destructured
	// name a concrete type (a number, inferred off the throw), yet its Go slot is a boxed
	// value.Value, so its reads must route the dynamic way or a typed coercion would meet a
	// box. The set rides a save and restore, so the bindings do not leak past the clause.
	var catchIntro []ast.Stmt
	if catchPattern != nil {
		tmp := r.freshTemp()
		recvExpr := &ast.CallExpr{Fun: &ast.SelectorExpr{X: caught, Sel: ident("ToValue")}}
		binds, err := r.bindDynamicPattern(catchPattern, ident(tmp), token.DEFINE)
		if err != nil {
			return nil, err
		}
		catchIntro = append(catchIntro, define(tmp, recvExpr))
		catchIntro = append(catchIntro, binds...)
		prevDyn := r.dynBoundLocals
		m := map[string]bool{}
		for name := range prevDyn {
			m[name] = true
		}
		r.collectAssignedNames(binds, m)
		r.dynBoundLocals = m
		defer func() { r.dynBoundLocals = prevDyn }()
	}
	catchStmts, err := r.lowerBlock(catchBlock)
	if err != nil {
		return nil, err
	}

	var body []ast.Stmt
	switch {
	case catchPattern != nil:
		body = append(body, catchIntro...)
	case binding == "":
		body = append(body, &ast.ExprStmt{X: caught})
	default:
		body = append(body,
			&ast.AssignStmt{Lhs: []ast.Expr{ident(binding)}, Tok: token.DEFINE, Rhs: []ast.Expr{caught}},
			&ast.AssignStmt{Lhs: []ast.Expr{ident("_")}, Tok: token.ASSIGN, Rhs: []ast.Expr{ident(binding)}},
		)
	}
	body = append(body, catchStmts.List...)

	// if rec := recover(); rec != nil { <body> }
	guard := &ast.IfStmt{
		Init: &ast.AssignStmt{
			Lhs: []ast.Expr{ident("rec")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{&ast.CallExpr{Fun: ident("recover")}},
		},
		Cond: &ast.BinaryExpr{X: ident("rec"), Op: token.NEQ, Y: ident("nil")},
		Body: &ast.BlockStmt{List: body},
	}
	return &ast.DeferStmt{Call: callClosure([]ast.Stmt{guard})}, nil
}

// nameReboundOrShadowed reports whether the name is assigned to or re-declared
// anywhere in the subtree. The catch lowering uses it to decide a catch binding is
// no longer the fixed *value.Error the body reads: an assignment target of the name
// stores a new value into the error local, and a variable declaration of the name
// shadows the binding with a fresh local the body then reads through the error's
// methods. Either shape emits Go that does not build, so the caller hands back.
func (r *Renderer) nameReboundOrShadowed(n frontend.Node, name string) bool {
	switch n.Kind() {
	case frontend.NodeBinaryExpression:
		kids := r.prog.Children(n)
		if len(kids) == 3 && isAssignOp(r.prog.Text(kids[1])) &&
			kids[0].Kind() == frontend.NodeIdentifier &&
			strings.TrimSpace(r.prog.Text(kids[0])) == name {
			return true
		}
	case frontend.NodeVariableDeclaration:
		kids := r.prog.Children(n)
		if len(kids) > 0 && kids[0].Kind() == frontend.NodeIdentifier &&
			strings.TrimSpace(r.prog.Text(kids[0])) == name {
			return true
		}
	}
	for _, k := range r.prog.Children(n) {
		if r.nameReboundOrShadowed(k, name) {
			return true
		}
	}
	return false
}

// isAssignOp reports whether the operator text assigns to its left operand: the
// plain "=" and every compound form. It excludes the comparison operators, which
// also end in "=", so a read like a === b is not mistaken for a store.
func isAssignOp(op string) bool {
	switch op {
	case "=", "+=", "-=", "*=", "/=", "%=", "**=",
		"<<=", ">>=", ">>>=", "&=", "|=", "^=", "&&=", "||=", "??=":
		return true
	}
	return false
}

// blockReturns reports whether a statement subtree contains a return that would
// complete out of it, descending through nested statements but not into a nested
// function, whose own return is its own. It is the guard the try lowering uses to
// keep an escaping return out of the closure it emits, where a Go return would
// leave the closure rather than the enclosing function.
func (r *Renderer) blockReturns(n frontend.Node) bool {
	switch n.Kind() {
	case frontend.NodeFunctionDeclaration, frontend.NodeFunctionExpression, frontend.NodeArrowFunction,
		frontend.NodeMethodDeclaration, frontend.NodeGetAccessor, frontend.NodeSetAccessor, frontend.NodeConstructor:
		return false
	case frontend.NodeReturnStatement:
		return true
	}
	for _, k := range r.prog.Children(n) {
		if r.blockReturns(k) {
			return true
		}
	}
	return false
}

// branchEscapesClosure reports whether a break or continue inside n targets a
// loop or switch outside n. lowerTry lowers the try, catch, and finally bodies
// inside a Go closure, and a branch that leaves the loop enclosing the try would
// compile to a break or continue with no loop around it, which Go rejects. The
// walk stops at a nested function, whose branches target its own loops, counts
// the loops and switches declared inside n, and tracks the labels those loops
// carry, so it reports an escape only for a branch whose target sits outside n:
// an unlabeled break with no enclosing loop or switch, an unlabeled continue
// with no enclosing loop, or a labeled branch whose label is not declared
// inside n.
func (r *Renderer) branchEscapesClosure(n frontend.Node) bool {
	return r.branchEscapes(n, 0, 0, nil)
}

func (r *Renderer) branchEscapes(n frontend.Node, loopDepth, switchDepth int, labels map[string]bool) bool {
	switch n.Kind() {
	case frontend.NodeFunctionDeclaration, frontend.NodeFunctionExpression, frontend.NodeArrowFunction,
		frontend.NodeMethodDeclaration, frontend.NodeGetAccessor, frontend.NodeSetAccessor, frontend.NodeConstructor:
		return false
	case frontend.NodeForStatement, frontend.NodeForOfStatement, frontend.NodeForInStatement, frontend.NodeWhileStatement:
		return r.branchEscapesChildren(n, loopDepth+1, switchDepth, labels)
	case frontend.NodeSwitchStatement:
		return r.branchEscapesChildren(n, loopDepth, switchDepth+1, labels)
	case frontend.NodeUnknown:
		txt := strings.TrimSpace(r.prog.Text(n))
		// A break or continue surfaces unclassified with the keyword leading its
		// text; the labeled form keeps its target as a lone identifier child. The
		// keyword can be followed straight away by a semicolon (break;), so match
		// the leading word rather than splitting on whitespace.
		if word := branchKeyword(txt); word == "break" || word == "continue" {
			kids := r.prog.Children(n)
			if len(kids) == 1 && kids[0].Kind() == frontend.NodeIdentifier {
				return !labels[strings.TrimSpace(r.prog.Text(kids[0]))]
			}
			if word == "break" {
				return loopDepth == 0 && switchDepth == 0
			}
			return loopDepth == 0
		}
		kids := r.prog.Children(n)
		// A do...while surfaces unclassified with a body block, a condition, and
		// text beginning with do; it counts as a loop the way the other loops do.
		if len(kids) == 2 && kids[0].Kind() == frontend.NodeBlock && strings.HasPrefix(txt, "do") {
			return r.branchEscapesChildren(n, loopDepth+1, switchDepth, labels)
		}
		// A labeled statement surfaces unclassified with the label identifier
		// first and the statement it labels second; the label is in scope for that
		// statement, so a branch naming it stays inside n.
		if len(kids) == 2 && kids[0].Kind() == frontend.NodeIdentifier {
			label := strings.TrimSpace(r.prog.Text(kids[0]))
			if strings.HasPrefix(txt, label+":") {
				next := make(map[string]bool, len(labels)+1)
				for k := range labels {
					next[k] = true
				}
				next[label] = true
				return r.branchEscapes(kids[1], loopDepth, switchDepth, next)
			}
		}
	}
	return r.branchEscapesChildren(n, loopDepth, switchDepth, labels)
}

// branchKeyword returns "break" or "continue" when txt is a branch statement led
// by that keyword, and "" otherwise. It matches the keyword only when what
// follows is not another identifier character, so a break; with no space is
// caught while an identifier that merely starts with the letters (breakfast) is
// not.
func branchKeyword(txt string) string {
	for _, kw := range [...]string{"break", "continue"} {
		if strings.HasPrefix(txt, kw) {
			if rest := txt[len(kw):]; rest == "" || !isIdentPart(rest[0]) {
				return kw
			}
		}
	}
	return ""
}

// isIdentPart reports whether c can continue a JavaScript identifier, the test
// that tells break; from a name like breakfast.
func isIdentPart(c byte) bool {
	return c == '_' || c == '$' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func (r *Renderer) branchEscapesChildren(n frontend.Node, loopDepth, switchDepth int, labels map[string]bool) bool {
	for _, k := range r.prog.Children(n) {
		if r.branchEscapes(k, loopDepth, switchDepth, labels) {
			return true
		}
	}
	return false
}

// callClosure wraps statements in an immediately invoked function with no
// parameters or results, the form a try body and its deferred handlers take so a
// recover can run inside a deferred function.
func callClosure(stmts []ast.Stmt) *ast.CallExpr {
	return &ast.CallExpr{Fun: &ast.FuncLit{
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: stmts},
	}}
}
