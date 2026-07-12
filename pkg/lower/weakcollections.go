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

// isWeakSetType reports whether an object type is a JavaScript WeakSet. The standard
// library types a WeakSet with add, has, and delete but no size and no get: add
// separates it from a WeakMap (which has get, not add), and the absent size separates
// it from a Set, whose fingerprint requires size. The three method names together
// with no size are the fingerprint.
func (r *Renderer) isWeakSetType(t frontend.Type) bool {
	var hasAdd, hasHas, hasDelete, hasSize bool
	for _, p := range r.prog.Properties(t) {
		switch p.Name {
		case "add":
			hasAdd = true
		case "has":
			hasHas = true
		case "delete":
			hasDelete = true
		case "size":
			hasSize = true
		}
	}
	return hasAdd && hasHas && hasDelete && !hasSize
}

// isWeakSet reports whether the checker types a node as a WeakSet, the receiver test
// the WeakSet lowerings share. It is the node-level companion to isWeakSetType, first
// ruling out an array so an array is never mistaken for a weak set.
func (r *Renderer) isWeakSet(n frontend.Node) bool {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	return r.isWeakSetType(t)
}

// renderWeakSet lowers a WeakSet<E> type to a pointer to the generic value.WeakSet
// header. The runtime holds the object pointee, so it is generic over T where the
// member renders to *T. It reads E off the add signature the same way renderSet does,
// then strips the star from the member render to name the pointee. A member whose
// render is not a pointer is not a weak-collection member, so it hands back.
func (r *Renderer) renderWeakSet(t frontend.Type) (ast.Expr, error) {
	elem, ok := r.setElem(t)
	if !ok {
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "WeakSet type did not expose its member through an add signature"}
	}
	pointee, err := r.weakKeyPointee(elem)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return star(index(sel("value", "WeakSet"), pointee)), nil
}

// newWeakSet lowers a WeakSet construction. The empty new WeakSet<E>() picks the value
// constructor over the member pointee, so new WeakSet<Foo>() lowers to
// value.NewWeakSet[Foo](). The iterable-argument form new WeakSet([a, b]) needs each
// member built as an object first, which the iterable-drain slice brings, so it hands
// back for now rather than mislower it.
func (r *Renderer) newWeakSet(n frontend.Node, args []frontend.Node) (ast.Expr, error) {
	valueArgs := r.namedArgs(args)
	if len(valueArgs) > 0 {
		return nil, &NotYetLowerable{Reason: "new WeakSet from an iterable of members is a later slice"}
	}
	elem, ok := r.setElem(r.prog.TypeAt(n))
	if !ok {
		return nil, &NotYetLowerable{Reason: "new WeakSet did not expose its member type"}
	}
	pointee, err := r.weakKeyPointee(elem)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: &ast.IndexExpr{X: sel("value", "NewWeakSet"), Index: pointee}}, nil
}

// weakSetMethodCall lowers a method call on a WeakSet receiver to the matching
// value.WeakSet method. Each maps to its Go name with an exact argument count: add(k)
// inserts and returns the set, has(k) and delete(k) report membership. A WeakSet has
// no clear, no size, and no iteration, so only these three are covered.
func (r *Renderer) weakSetMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	var goName string
	var want int
	switch method {
	case "add":
		goName, want = "Add", 1
	case "has":
		goName, want = "Has", 1
	case "delete":
		goName, want = "Delete", 1
	default:
		return nil, &NotYetLowerable{Reason: "WeakSet method ." + method + " is a later slice"}
	}
	return r.weakCall(recvNode, goName, want, argNodes, "WeakSet")
}

// isWeakRefType reports whether an object type is a JavaScript WeakRef. Its deref
// method is unique to WeakRef among the collections, so the presence of deref with no
// get, set, or add is the fingerprint.
func (r *Renderer) isWeakRefType(t frontend.Type) bool {
	var hasDeref, hasGet, hasSet, hasAdd bool
	for _, p := range r.prog.Properties(t) {
		switch p.Name {
		case "deref":
			hasDeref = true
		case "get":
			hasGet = true
		case "set":
			hasSet = true
		case "add":
			hasAdd = true
		}
	}
	return hasDeref && !hasGet && !hasSet && !hasAdd
}

// isWeakRef reports whether the checker types a node as a WeakRef, the receiver test
// the WeakRef deref lowering uses.
func (r *Renderer) isWeakRef(n frontend.Node) bool {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	return r.isWeakRefType(t)
}

// weakRefTarget returns the pointee of a WeakRef type's target, the T in the *T the
// target renders to. It reads T off the deref signature, whose result is T | undefined:
// the object member of that union is the target type, which strips to its pointee the
// same way a weak key does. A WeakRef whose target render is not a pointer hands back.
func (r *Renderer) weakRefTarget(t frontend.Type) (ast.Expr, error) {
	var derefType frontend.Type
	found := false
	for _, p := range r.prog.Properties(t) {
		if p.Name == "deref" {
			derefType, found = p.Type, true
			break
		}
	}
	if !found {
		return nil, &NotYetLowerable{Reason: "WeakRef type did not expose its target through a deref signature"}
	}
	call, _ := r.prog.Signatures(derefType)
	if len(call) == 0 {
		return nil, &NotYetLowerable{Reason: "WeakRef deref did not expose a signature"}
	}
	members := r.prog.UnionMembers(call[0].Return)
	if len(members) == 0 {
		members = []frontend.Type{call[0].Return}
	}
	for _, m := range members {
		if m.Flags&frontend.TypeObject != 0 {
			return r.weakKeyPointee(m)
		}
	}
	return nil, &NotYetLowerable{Reason: "WeakRef over a target that is not an object is a later slice"}
}

// renderWeakRef lowers a WeakRef<T> type to a pointer to the generic value.WeakRef
// header over the target pointee.
func (r *Renderer) renderWeakRef(t frontend.Type) (ast.Expr, error) {
	pointee, err := r.weakRefTarget(t)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return star(index(sel("value", "WeakRef"), pointee)), nil
}

// newWeakRef lowers new WeakRef(target) to value.NewWeakRef[Pointee](target). The
// pointee comes from the WeakRef type at this node, and the single target argument
// lowers straight through as the object pointer it renders to.
func (r *Renderer) newWeakRef(n frontend.Node, args []frontend.Node) (ast.Expr, error) {
	valueArgs := r.namedArgs(args)
	if len(valueArgs) != 1 {
		return nil, &NotYetLowerable{Reason: "new WeakRef takes exactly one target argument"}
	}
	pointee, err := r.weakRefTarget(r.prog.TypeAt(n))
	if err != nil {
		return nil, err
	}
	target, err := r.lowerExpr(valueArgs[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: &ast.IndexExpr{X: sel("value", "NewWeakRef"), Index: pointee}, Args: []ast.Expr{target}}, nil
}

// weakRefMethodCall lowers weakRef.deref() to recv.Deref(), the only method a WeakRef
// carries. It returns an Opt the same narrowing and nullish paths any optional takes.
func (r *Renderer) weakRefMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "deref" {
		return nil, &NotYetLowerable{Reason: "WeakRef method ." + method + " is a later slice"}
	}
	return r.weakCall(recvNode, "Deref", 0, argNodes, "WeakRef")
}

// isFinalizationRegistryType reports whether an object type is a JavaScript
// FinalizationRegistry. Its register and unregister methods together are unique to it
// among the collections, so their presence is the fingerprint.
func (r *Renderer) isFinalizationRegistryType(t frontend.Type) bool {
	var hasRegister, hasUnregister bool
	for _, p := range r.prog.Properties(t) {
		switch p.Name {
		case "register":
			hasRegister = true
		case "unregister":
			hasUnregister = true
		}
	}
	return hasRegister && hasUnregister
}

// isFinalizationRegistry reports whether the checker types a node as a
// FinalizationRegistry, the receiver test the register and unregister lowerings share.
func (r *Renderer) isFinalizationRegistry(n frontend.Node) bool {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	return r.isFinalizationRegistryType(t)
}

// finalizationHeldType returns the held-value type T of a FinalizationRegistry, read
// off register's second parameter (target, heldValue: T, token?). It reports false for
// a type with no such register signature.
func (r *Renderer) finalizationHeldType(t frontend.Type) (frontend.Type, bool) {
	var regType frontend.Type
	found := false
	for _, p := range r.prog.Properties(t) {
		if p.Name == "register" {
			regType, found = p.Type, true
			break
		}
	}
	if !found {
		return frontend.Type{}, false
	}
	call, _ := r.prog.Signatures(regType)
	if len(call) == 0 || len(call[0].Params) < 2 {
		return frontend.Type{}, false
	}
	return call[0].Params[1].Type, true
}

// renderFinalizationRegistry lowers a FinalizationRegistry<T> type to a pointer to the
// generic value.FinalizationRegistry header over the held-value type T.
func (r *Renderer) renderFinalizationRegistry(t frontend.Type) (ast.Expr, error) {
	held, ok := r.finalizationHeldType(t)
	if !ok {
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "FinalizationRegistry type did not expose its held value through a register signature"}
	}
	hExpr, err := r.typeExpr(held)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return star(index(sel("value", "FinalizationRegistry"), hExpr)), nil
}

// newFinalizationRegistry lowers new FinalizationRegistry(cb) to
// value.NewFinalizationRegistry[T](cb): it reads the held type off the registry type at
// this node and lowers the single cleanup-callback argument straight through as the
// func(T) it renders to.
func (r *Renderer) newFinalizationRegistry(n frontend.Node, args []frontend.Node) (ast.Expr, error) {
	valueArgs := r.namedArgs(args)
	if len(valueArgs) != 1 {
		return nil, &NotYetLowerable{Reason: "new FinalizationRegistry takes exactly one cleanup-callback argument"}
	}
	held, ok := r.finalizationHeldType(r.prog.TypeAt(n))
	if !ok {
		return nil, &NotYetLowerable{Reason: "new FinalizationRegistry did not expose its held value type"}
	}
	hExpr, err := r.typeExpr(held)
	if err != nil {
		return nil, err
	}
	cb, err := r.lowerExpr(valueArgs[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: &ast.IndexExpr{X: sel("value", "NewFinalizationRegistry"), Index: hExpr}, Args: []ast.Expr{cb}}, nil
}

// finalizationRegistryMethodCall lowers register and unregister on a
// FinalizationRegistry receiver. unregister(token) is a plain method returning a
// boolean. register(target, held, token?) lowers to the free function
// value.FinalizationRegister[Target, T], because the call is generic over the target's
// own type, which the registry type alone does not carry: the target pointee and the
// held type T are the type arguments, and a missing unregister token passes nil.
func (r *Renderer) finalizationRegistryMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "unregister":
		return r.weakCall(recvNode, "Unregister", 1, argNodes, "FinalizationRegistry")
	case "register":
		return r.finalizationRegister(recvNode, argNodes)
	default:
		return nil, &NotYetLowerable{Reason: "FinalizationRegistry method ." + method + " is a later slice"}
	}
}

// finalizationRegister lowers registry.register(target, held, token?) to the free
// function value.FinalizationRegister[Target, T](registry, target, held, token). The
// target's pointee is its rendered object type, T is the registry's held type, and a
// two-argument register with no token passes a nil token that no unregister matches.
func (r *Renderer) finalizationRegister(recvNode frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 2 && len(argNodes) != 3 {
		return nil, &NotYetLowerable{Reason: "FinalizationRegistry register takes a target, a held value, and an optional token"}
	}
	held, ok := r.finalizationHeldType(r.prog.TypeAt(recvNode))
	if !ok {
		return nil, &NotYetLowerable{Reason: "FinalizationRegistry register did not expose its held value type"}
	}
	hExpr, err := r.typeExpr(held)
	if err != nil {
		return nil, err
	}
	targetPointee, err := r.weakKeyPointee(r.prog.TypeAt(argNodes[0]))
	if err != nil {
		return nil, err
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	target, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	heldExpr, err := r.lowerExpr(argNodes[1])
	if err != nil {
		return nil, err
	}
	var tokenExpr ast.Expr = ident("nil")
	if len(argNodes) == 3 {
		tokenExpr, err = r.lowerExpr(argNodes[2])
		if err != nil {
			return nil, err
		}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun:  &ast.IndexListExpr{X: sel("value", "FinalizationRegister"), Indices: []ast.Expr{targetPointee, hExpr}},
		Args: []ast.Expr{recv, target, heldExpr, tokenExpr},
	}, nil
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
