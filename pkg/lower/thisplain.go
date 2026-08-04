package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// A plain function's `this` is not the object it happens to sit near in the source.
// It is whatever the call supplies, and a call that supplies nothing leaves it
// undefined once the code is strict. bento compiles a plain function to a Go closure
// with no receiver slot, so nothing bento emits ever supplies one, and undefined is
// the honest answer everywhere in a lowered program rather than a stand-in for one.
//
// That is also the answer Node's own suite wants. `common.mustCall(function (err,
// ...args) { fn.apply(this, args) })` reads `this` only to hand it straight back out
// as the receiver of another call, and undefined is what Node's callback invocation
// puts there too.
//
// Sloppy mode says the global object instead, and bento has no value for that, so a
// non-strict function that reads `this` keeps handing back. Every file in Node's
// suite opens with "use strict", and every ES module is strict whether it says so or
// not, so the strict half is nearly the whole of it.
//
// What is left is the one syntactic place JavaScript does supply a receiver to a
// plain function: method-call syntax over an object that holds it. bento fills a
// struct field with the closure and `o.m()` calls that field with nothing bound, so
// a function value written straight into a property refuses to lower a body that
// reads `this`. That is receiverPosition below, and it covers an object literal's
// property, a class field's initializer, and a store into a property. A function
// value that reaches a property by a longer road, bound to a name first, is the same
// reach a boxed object method already refuses in coerce.go.

// strictAt reports whether the source file a node sits in runs strict, which decides
// what `this` means in a plain function there. An ES module is always strict, and a
// script is strict when it opens with a "use strict" directive prologue. The answer
// is per file rather than per program because a required CommonJS module carries its
// own directive and the entry carries its own, and a plain function is lowered in
// whichever file declared it.
//
// A function that carries its own "use strict" inside a sloppy file is strict too,
// and so is anything nested in one. Neither is read here: answering false for them
// keeps the hand-back, which is the safe direction, and no file in Node's suite is
// in that shape.
func (r *Renderer) strictAt(n frontend.Node) bool {
	f := n.File()
	if r.prog.IsModule(f) {
		return true
	}
	if r.fileStrict == nil {
		r.fileStrict = map[string]bool{}
		for _, root := range r.prog.SourceFiles() {
			r.fileStrict[root.File().Path] = r.hasUseStrictPrologue(root)
		}
	}
	return r.fileStrict[f.Path]
}

// plainThisScope enters the body of a plain function: a declaration or an expression,
// never an arrow and never a method. It answers a restore to defer and the reason the
// function does not lower, asked once at the top of the lowering so a refusal names
// the function rather than surfacing from somewhere inside its body.
//
// A body with no `this` in it needs no scope at all and gets a no-op restore, which
// keeps every function bento already lowered on exactly the path it was on. A body
// that does read `this` clears the class scope for the same reason boxMethodClosure
// does: an enclosing method's receiver is not this function's, and a `this.x` inside
// it must not resolve against the class the method belongs to. The receiver-position
// flag clears too, since it describes where the function value is going rather than
// what its body may do.
func (r *Renderer) plainThisScope(fn frontend.Node, what string) (func(), error) {
	if err := r.plainThisRefusal(fn, what); err != nil {
		return nil, err
	}
	return r.pushPlainThis(fn), nil
}

// plainThisRefusal is the gate half, split out because a nested function declaration
// is decided in one pass and lowered in another: the decision pass reads the refusal
// and the lowering pass pushes the scope.
func (r *Renderer) plainThisRefusal(fn frontend.Node, what string) error {
	if !subtreeHasKind(r.prog, fn, frontend.NodeThisKeyword) {
		return nil
	}
	if r.recvPos {
		return &NotYetLowerable{Reason: "a " + what + " stored straight into a property is called with that object as its this, a later slice"}
	}
	if !r.strictAt(fn) {
		return &NotYetLowerable{Reason: "this in a non-strict " + what + " is the global object, a later slice"}
	}
	return nil
}

// pushPlainThis is the scope half, and it answers a no-op for a body with no `this`
// so every function bento already lowered stays on exactly the path it was on.
func (r *Renderer) pushPlainThis(fn frontend.Node) func() {
	if !subtreeHasKind(r.prog, fn, frontend.NodeThisKeyword) {
		return func() {}
	}
	prevClass, prevThis, prevStatic := r.curClass, r.thisName, r.staticClass
	prevPlain, prevRecv := r.thisPlain, r.recvPos
	r.curClass, r.thisName, r.staticClass = nil, "", nil
	r.thisPlain, r.recvPos = true, false
	return func() {
		r.curClass, r.thisName, r.staticClass = prevClass, prevThis, prevStatic
		r.thisPlain, r.recvPos = prevPlain, prevRecv
	}
}

// pushReceiverPosition marks the expression about to lower as one whose value is
// written straight into a property, where a plain function would be called with the
// owning object as its `this`. It is cleared rather than set by anything nested that
// makes its own scope, so only the value itself is in the position, not the bodies
// inside it.
func (r *Renderer) pushReceiverPosition() func() {
	prev := r.recvPos
	r.recvPos = true
	return func() { r.recvPos = prev }
}

// plainThisValue lowers a `this` that no lowered class body binds. Inside a plain
// function it is the undefined a receiver-free call leaves, and anywhere else it is
// still the hand-back it was: a static method's `this` is the class constructor and a
// module's top-level `this` is the exports object, and neither has a value here yet.
func (r *Renderer) plainThisValue() (ast.Expr, error) {
	if !r.thisPlain {
		return nil, &NotYetLowerable{Reason: "this outside a lowered class body is a later slice"}
	}
	r.requireImport(valuePkg)
	return sel("value", "Undefined"), nil
}

// funcValueReadsThis reports whether the function an identifier names has a body that
// reads `this`. It is the question `f.call(obj)` has to ask: bento's plain function
// takes no receiver, so a call that supplies one supplies it to nothing, and a body
// that would have read it gets undefined instead of the object the source named.
func (r *Renderer) funcValueReadsThis(n frontend.Node) bool {
	sym, ok := r.prog.SymbolAt(n)
	if !ok {
		return false
	}
	for _, d := range r.prog.Declarations(sym) {
		if subtreeHasKind(r.prog, d, frontend.NodeThisKeyword) {
			return true
		}
	}
	return false
}

// nullishThisArg reports whether the this argument of a .call or .apply is one the
// callee would read as undefined anyway. Strict mode passes null and undefined
// through untouched rather than substituting the global object, so a body that reads
// `this` sees the same undefined bento gives it, and the argument drops with nothing
// lost. Any other receiver is a real one and hands back.
func (r *Renderer) nullishThisArg(n frontend.Node) bool {
	return n.Kind() == frontend.NodeNullKeyword || r.isUndefinedLiteral(n)
}
