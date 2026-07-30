package lower

import "github.com/tamnd/bento/pkg/frontend"

// This file routes a plain object binding to the dynamic bag when the program uses
// it as the receiver of an Object static that only a runtime object can answer:
// Object.seal, freeze, preventExtensions, setPrototypeOf, getPrototypeOf,
// getOwnPropertyDescriptor and their kin. Each of those lowerings already emits the
// matching runtime call when its receiver isDynamic, and hands back when the receiver
// is a fixed-shape Go struct, which carries no descriptor bag, no prototype slot, and
// no integrity state. Boxing the binding from its literal gives every one of them the
// runtime object they need. Object.defineProperty is deliberately not in the routed
// set; see objectDynShapeMethods for why.
//
// The routing is sound the same way the undeclared-write routing is: the operation
// hands back today on a struct receiver, so the whole unit already hands back at that
// call. Boxing the binding can only move the unit forward, to a full lowering, or
// leave it handing back at some other use of the binding that the dynamic path does
// not yet cover. It never turns a passing unit into a failing one, because a unit
// that reaches one of these ops was not passing. Coercing the boxed binding into a
// static shape still hands the unit back, so no wrong Go is emitted.

// objectDynShapeMethods is the set of Object statics whose lowering requires a
// runtime object and hands back on a fixed-shape struct receiver. The integrity
// predicates isSealed, isFrozen, and isExtensible are left out on purpose: a fixed
// shape answers them by a constant fold today, so a binding used only through them
// is already passing and must not be re-routed to the bag.
//
// defineProperty and defineProperties are left out too, for a subtler reason. Boxing a
// binding does not only move the Object static forward; it flips the binding to the
// dynamic representation for every one of its uses. An array-like bound object that a
// test hands to Object.defineProperty is nearly always then driven through
// Array.prototype.<m>.call(obj, ...), and the array generic on a dynamic bag lowers to a
// path that does not honor an accessor descriptor mutating length mid-iteration. That
// path does not hand back, it runs and gives the wrong count, so boxing turns a handback
// into a fail. The other statics here carry no such downstream array-generic pairing in
// the corpus, so they box safely; defineProperty waits for the array-generic-on-bag path
// to model accessor side effects.
var objectDynShapeMethods = map[string]bool{
	"seal":                      true,
	"freeze":                    true,
	"preventExtensions":         true,
	"setPrototypeOf":            true,
	"getPrototypeOf":            true,
	"getOwnPropertyDescriptor":  true,
	"getOwnPropertyDescriptors": true,
	"getOwnPropertySymbols":     true,
}

// bindingObjectLiteralNeedsDynamicShape reports whether an object-literal initializer
// bound to nameNode is later handed to an Object static that only a runtime object can
// answer. The init-boxing site asks it so the binding's literal boxes to a bag from
// creation, giving the later Object.defineProperty or Object.seal the descriptor bag,
// prototype slot, and integrity state it needs rather than handing back on a struct.
// The literal must be a fixed object shape the struct path would otherwise take, so a
// computed-key or spread literal that already boxes on its own is not double-routed.
func (r *Renderer) bindingObjectLiteralNeedsDynamicShape(nameNode, initNode frontend.Node) bool {
	if initNode.Kind() != frontend.NodeObjectLiteralExpression {
		return false
	}
	sym, ok := r.prog.SymbolAt(nameNode)
	if !ok {
		return false
	}
	return r.symIsDynObjectReceiver(sym) && r.isFixedObjectShape(r.prog.TypeAt(initNode))
}

// symIsDynObjectReceiver reports whether an object binding is passed as the receiver
// of an Object static that needs a runtime object. It scans every source file once, on
// the first query, and answers from the recorded set, so a read of the binding and the
// binding's own declaration ask the routing question the same way and land the object
// on one representation.
func (r *Renderer) symIsDynObjectReceiver(sym frontend.Symbol) bool {
	if !r.dynObjectReceiverScanned {
		r.scanDynObjectReceivers()
	}
	return r.dynObjectReceiverSyms[sym]
}

// scanDynObjectReceivers walks every source file for a call Object.<m>(o, ...) whose
// method needs a runtime object and whose first argument is a plain identifier bound to
// a fixed object shape, and records that binding symbol. The scan runs once; the result
// is a closed set the routing predicates read without walking again.
func (r *Renderer) scanDynObjectReceivers() {
	r.dynObjectReceiverScanned = true
	set := map[frontend.Symbol]bool{}
	var walk func(n frontend.Node)
	walk = func(n frontend.Node) {
		for _, sym := range r.dynObjectReceivers(n) {
			set[sym] = true
		}
		for _, c := range r.prog.Children(n) {
			walk(c)
		}
	}
	for _, sf := range r.prog.SourceFiles() {
		walk(sf)
	}
	r.dynObjectReceiverSyms = set
}

// dynObjectReceivers returns the binding symbols a node hands to an Object static that
// needs a runtime object, when the node is a call Object.<m>(o, ...) whose method is in
// objectDynShapeMethods. The receiver (the first argument) is reported for every such
// method. Object.setPrototypeOf(o, proto) also reports proto: the prototype the runtime
// stores must be the same boxed object a later Object.getPrototypeOf reads back, so a
// struct-shaped proto boxed fresh at the call site would fail the === identity the
// prototype statics preserve. Each reported argument must be a plain identifier bound to
// a fixed object shape; a receiver that is already dynamic, an array, or any
// non-fixed-object type is left to the paths that own it.
func (r *Renderer) dynObjectReceivers(n frontend.Node) []frontend.Symbol {
	if n.Kind() != frontend.NodeCallExpression {
		return nil
	}
	kids := r.prog.Children(n)
	if len(kids) < 2 {
		return nil
	}
	callee := kids[0]
	if callee.Kind() != frontend.NodePropertyAccessExpression {
		return nil
	}
	cParts := r.prog.Children(callee)
	if len(cParts) != 2 || !r.isGlobalRef(cParts[0], "Object") {
		return nil
	}
	method := r.prog.Text(cParts[1])
	if !objectDynShapeMethods[method] {
		return nil
	}
	args := kids[1:]
	var out []frontend.Symbol
	if sym, ok := r.fixedObjectBindingSym(args[0]); ok {
		out = append(out, sym)
	}
	// setPrototypeOf stores its second argument as the receiver's prototype, and a later
	// getPrototypeOf reads that same object back, so the prototype source must box to a
	// stable bag rather than a fresh one per use.
	if method == "setPrototypeOf" && len(args) >= 2 {
		if sym, ok := r.fixedObjectBindingSym(args[1]); ok {
			out = append(out, sym)
		}
	}
	return out
}

// fixedObjectBindingSym returns the binding symbol of an argument that is a plain
// identifier bound to a fixed object shape, the only argument form this routing boxes.
func (r *Renderer) fixedObjectBindingSym(arg frontend.Node) (frontend.Symbol, bool) {
	var zero frontend.Symbol
	if arg.Kind() != frontend.NodeIdentifier {
		return zero, false
	}
	if !r.isFixedObjectShape(r.prog.TypeAt(arg)) {
		return zero, false
	}
	return r.prog.SymbolAt(arg)
}
