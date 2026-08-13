package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// globalThis is the name JavaScript gives the global scope as an object, and a
// Node program reads it three ways. It reaches for a global unambiguously,
// `globalThis.process`, which is line 23 of test/common/index.js and so the first
// thing 665 of Node's tests do. It keys the object at run time, `globalThis[name]`,
// the shape a leak check over a list of names takes. And it writes its own entries
// in, `globalThis.foo = 1`, which is how a test makes a value visible to a module
// it loads later.
//
// The checker types the name as `typeof globalThis`, a tree of hundreds of
// properties whose types are more objects still, so before this file every one of
// those reads asked for a Go type for that shape and the structural-key walk gave
// up on it. That was the biggest family in the suite and it named the walk rather
// than the thing the program was doing.
//
// What the name means here is an object: the value.Object GlobalThisValue builds
// and caches, holding the globals bento models as non-enumerable properties. The
// reference lowers to that object and every read, write, and enumeration over it
// goes down the dynamic path the same way a read off `process` or `module.exports`
// already does. globalThis.process is the same object the bare name reads, since
// both reach the one object ProcessValue caches.
//
// The one thing that does not go to the object is a read of a global bento does
// not model. `globalThis.crypto` is a real object in Node, and answering the
// object's undefined for it would be a wrong answer rather than a refusal, so a
// member whose name resolves to an ambient global is lowered as the bare name is,
// which either hosts it or hands back naming it. A member that resolves to no
// global at all, `globalThis.foo`, is a property of the object and nothing more,
// so it reads and writes as one.
//
// Node gives the same object a second name, `global`, and its own test suite uses
// both within a few lines of each other. It is the same object here too: the name
// lowers to the same package-level var, so `global === globalThis` holds and a
// property written through one is read through the other.

// bentoGlobalThisName is the Go identifier the global object emits under, the one
// package-level var every reference to globalThis in the program reads.
const bentoGlobalThisName = "bentoGlobalThis"

// isGlobalThisRef reports whether n is a reference to the global object rather than
// a user binding that shares one of its two names.
//
// globalThis is the standard one. The checker gives it a symbol with no declarations
// at all, since no file declares it, which is what separates it from a
// `const globalThis = ...` a program wrote for itself: that one is a variable with a
// declaration. isAmbientGlobal cannot settle this because it wants every declaration
// to be in a .d.ts and there are none to look at.
//
// global is Node's own older name for the same object, and `global === globalThis`
// is true in Node. bento declares it in its ambient library (aot_ambient.go) rather
// than inheriting it, so it does have a declaration and the ambient test is the one
// that settles it, which keeps a program's own `const global = {}` reading its own
// binding. Both names answer here so every path built on this one, the member read
// that resolves an unmodeled global by name and the run-time keyed read among them,
// treats the two spellings as the one object they are.
func (r *Renderer) isGlobalThisRef(n frontend.Node) bool {
	if n.Kind() != frontend.NodeIdentifier {
		return false
	}
	switch r.prog.Text(n) {
	case "globalThis":
		sym, ok := r.prog.SymbolAt(n)
		if !ok {
			return false
		}
		return len(r.prog.Declarations(sym)) == 0
	case "global":
		return r.isAmbientGlobal(n)
	}
	return false
}

// globalThisRef lowers a globalThis reference to the package-level global object,
// flagging that it must be emitted. Every read off it (a member, an element, a
// for...in) lowers through the ordinary dynamic paths from the object this
// returns, because isDynamic routes the reference there.
func (r *Renderer) globalThisRef() ast.Expr {
	r.usesGlobalThis = true
	return ident(bentoGlobalThisName)
}

// globalThisMember lowers a read of a named member off globalThis whose name is an
// ambient global, by lowering it as the bare name. That is what keeps the answer
// honest either way: globalThis.process reaches the process object the bare name
// reaches, and globalThis.crypto hands back naming crypto rather than reading the
// undefined the global object holds for a surface bento has not built. A member
// that names no global at all is not this case and reads off the object.
func (r *Renderer) globalThisMember(obj, nameNode frontend.Node) (ast.Expr, bool, error) {
	if !r.isGlobalThisRef(obj) || !r.isAmbientGlobal(nameNode) {
		return nil, false, nil
	}
	expr, err := r.lowerExpr(nameNode)
	if err != nil {
		return nil, false, err
	}
	return expr, true, nil
}

// globalThisDecls returns the package-level declaration backing the global object,
// or nil when the program never named globalThis. It is emitted alongside the
// CommonJS and process globals, so a program that names none of them pays for none
// of them.
func (r *Renderer) globalThisDecls() []ast.Decl {
	if !r.usesGlobalThis {
		return nil
	}
	r.requireImport(valuePkg)
	return []ast.Decl{&ast.GenDecl{
		Tok: token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{
			Names:  []*ast.Ident{ident(bentoGlobalThisName)},
			Values: []ast.Expr{&ast.CallExpr{Fun: sel("value", "GlobalThisValue")}},
		}},
	}}
}
