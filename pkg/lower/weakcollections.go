package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the weak collections: WeakMap and WeakSet (this slice), and later
// WeakRef and FinalizationRegistry. Each maps to a value runtime type that holds its
// object targets weakly, so the operational surface lowers exactly while the exact
// turn a collected target's slot is reclaimed stays the garbage-collection-timing
// ceiling the milestone names. A weak collection is a TypeObject like a Map or Set,
// so it is detected by the same property-name fingerprint the keyed collections use
// and routed ahead of renderObject, which would otherwise intern its methods as a
// struct shape.

// isWeakMapType reports whether an object type is a JavaScript WeakMap. The standard
// library types a WeakMap with get, set, has, and delete but no size: get and set
// separate it from a WeakSet (which has add, not get), and the absent size separates
// it from a Map, whose fingerprint requires size. The four method names together
// with no size are the fingerprint, read the same way isMapType reads its own.
func (r *Renderer) isWeakMapType(t frontend.Type) bool {
	var hasGet, hasSet, hasHas, hasDelete, hasSize bool
	for _, p := range r.prog.Properties(t) {
		switch p.Name {
		case "get":
			hasGet = true
		case "set":
			hasSet = true
		case "has":
			hasHas = true
		case "delete":
			hasDelete = true
		case "size":
			hasSize = true
		}
	}
	return hasGet && hasSet && hasHas && hasDelete && !hasSize
}

// isWeakMap reports whether the checker types a node as a WeakMap, the receiver test
// the WeakMap lowerings share. It is the node-level companion to isWeakMapType: it
// reads the node's type and applies the same fingerprint, first ruling out an array
// so an array is never mistaken for a weak map.
func (r *Renderer) isWeakMap(n frontend.Node) bool {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	return r.isWeakMapType(t)
}

// renderWeakMap lowers a WeakMap<K, V> type to a pointer to the generic value.WeakMap
// header. The runtime keys on the object pointee, so it is generic over T where the
// key renders to *T and V is the value type. It reads K and V off the set signature
// the same way renderMap does, then strips the star from the key render to name the
// pointee. A key whose render is not a pointer is not a weak-collection key, so it
// hands back rather than emit a WeakMap over an incomparable key.
func (r *Renderer) renderWeakMap(t frontend.Type) (ast.Expr, error) {
	k, v, ok := r.mapKeyVal(t)
	if !ok {
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "WeakMap type did not expose its key and value through a set signature"}
	}
	pointee, err := r.weakKeyPointee(k)
	if err != nil {
		return nil, err
	}
	vExpr, err := r.typeExpr(v)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return star(&ast.IndexListExpr{X: sel("value", "WeakMap"), Indices: []ast.Expr{pointee, vExpr}}), nil
}

// weakKeyPointee renders a weak-collection key type and returns its pointee, the T in
// the *T a key object renders to. A WeakMap or WeakSet holds objects, which lower to
// Go struct pointers, and the runtime is generic over the pointee, so the star is
// stripped here. A key whose render is not a pointer (an array is a slice, a function
// a func) is not a weak key and hands back, upholding the zero-fail rule rather than
// emit a weak collection over a type the runtime cannot hold.
func (r *Renderer) weakKeyPointee(k frontend.Type) (ast.Expr, error) {
	if k.Flags&frontend.TypeObject == 0 {
		return nil, &NotYetLowerable{Reason: "a weak collection keyed by a non-object type is not lowerable"}
	}
	kExpr, err := r.typeExpr(k)
	if err != nil {
		return nil, err
	}
	star, ok := kExpr.(*ast.StarExpr)
	if !ok {
		return nil, &NotYetLowerable{Reason: "a weak collection keyed by a reference type that is not a plain object is a later slice"}
	}
	return star.X, nil
}

// newWeakMap lowers a WeakMap construction. The empty new WeakMap<K, V>() picks the
// value constructor over the key pointee and the value type, so new WeakMap<Foo,
// number>() lowers to value.NewWeakMap[Foo, float64](). The entries-argument form
// new WeakMap([[k, v], ...]) is a later slice: a WeakMap's keys are objects with
// reference identity, so the fill needs each key to be an already-built object, which
// the iterable-drain slice brings, so it hands back for now rather than mislower it.
func (r *Renderer) newWeakMap(n frontend.Node, args []frontend.Node) (ast.Expr, error) {
	valueArgs := r.namedArgs(args)
	if len(valueArgs) > 0 {
		return nil, &NotYetLowerable{Reason: "new WeakMap from an iterable of entry pairs is a later slice"}
	}
	k, v, ok := r.mapKeyVal(r.prog.TypeAt(n))
	if !ok {
		return nil, &NotYetLowerable{Reason: "new WeakMap did not expose its key and value types"}
	}
	pointee, err := r.weakKeyPointee(k)
	if err != nil {
		return nil, err
	}
	vExpr, err := r.typeExpr(v)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: &ast.IndexListExpr{X: sel("value", "NewWeakMap"), Indices: []ast.Expr{pointee, vExpr}}}, nil
}

// weakMapMethodCall lowers a method call on a WeakMap receiver to the matching
// value.WeakMap method. Each maps to its Go name with an exact argument count: get(k)
// reads an entry as an Opt, set(k, v) writes and returns the map, has(k) and
// delete(k) report membership. A WeakMap has no clear, no size, and no iteration, so
// only these four are covered and anything else hands back.
func (r *Renderer) weakMapMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	var goName string
	var want int
	switch method {
	case "get":
		goName, want = "Get", 1
	case "set":
		goName, want = "Set", 2
	case "has":
		goName, want = "Has", 1
	case "delete":
		goName, want = "Delete", 1
	default:
		return nil, &NotYetLowerable{Reason: "WeakMap method ." + method + " is a later slice"}
	}
	return r.weakCall(recvNode, goName, want, argNodes, "WeakMap")
}

// weakCall lowers a fixed-arity weak-collection method to recv.GoName(args...): it
// checks the argument count, lowers the receiver and each argument straight through
// (the checker has already typed them against the collection's own key or member
// type), and builds the call. It is the shared tail of the WeakMap and WeakSet method
// dispatch, kind naming which collection so the hand-back reason reads right.
func (r *Renderer) weakCall(recvNode frontend.Node, goName string, want int, argNodes []frontend.Node, kind string) (ast.Expr, error) {
	if len(argNodes) != want {
		return nil, &NotYetLowerable{Reason: kind + " method with this argument count is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	args := make([]ast.Expr, 0, want)
	for _, a := range argNodes {
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}, Args: args}, nil
}
