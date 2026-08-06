package lower

import (
	"errors"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// intLit renders a Go int literal, the arity NewCtor takes and the index value.Arg
// selects.
func intLit(i int) ast.Expr {
	return &ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(i)}
}

// The ES5 constructor function is how JavaScript wrote classes before it had them,
// and it is still how most of Node's own library and its whole test/common helper
// tree is written:
//
//	function ArrayStream() {}
//	Object.setPrototypeOf(ArrayStream.prototype, Stream.prototype);
//	ArrayStream.prototype.readable = true;
//	const s = new ArrayStream();
//
// Nothing in that shape survives bento's ordinary function lowering. A function
// declaration becomes a bare Go func, which has no .prototype to read, no property
// bag to write one onto, no receiver slot for the body's `this`, and nothing `new`
// can be applied to. So every one of those four lines handed the unit back, three of
// them under a reason that named an index signature, because Function.prototype is
// declared `any` in lib.es5.d.ts and the interned function shape carries no field of
// that name, so the read fell through to missingPropertyFold.
//
// A function the program uses that way lowers here instead to the runtime
// constructor value pkg/value/ctorfunc.go builds: one value.Value that is callable,
// constructible, and carries its own .prototype object. Everything about it is then
// dynamic, which is the point. The prototype is a real object, so a write onto it is
// an ordinary property write; new is value.Construct, which links the instance to
// whatever .prototype holds at that moment; and a member read on an instance climbs
// the chain the runtime already walks. None of that needs a new static shape,
// because a prototype chain is exactly the thing a static shape cannot describe.
//
// Only a function the program actually treats as a constructor takes this path.
// Every other function declaration lowers as it did, to a Go func, because a Go func
// is enormously better and the overwhelming majority of functions never see `new` or
// `.prototype`. What marks one is syntax, collected in a pre-pass over the entry and
// its dependencies: the callee of a `new`, or the object of a `.prototype` access.

// collectCtorFuncs records the top-level function declarations the program uses as
// ES5 constructors. It runs over the same file list the other pre-passes take, and
// like them it has to be complete before anything consults ctorFuncRef, since the
// boxed-signature pass asks about the same expressions the lowering later asks about.
//
// Only a top-level declaration is claimed. A function declared inside another
// function lowers to a Go closure local through the nested-function path, which has
// no package var to hold a constructor value, so marking it here would promise a
// lowering the declaration site does not perform; it keeps its old hand-back instead.
func (r *Renderer) collectCtorFuncs(files []frontend.Node) {
	if r.ctorFuncs == nil {
		r.ctorFuncs = map[frontend.Symbol]bool{}
	}
	top := map[frontend.Symbol]bool{}
	for _, f := range files {
		for _, stmt := range r.prog.Children(f) {
			if stmt.Kind() != frontend.NodeFunctionDeclaration {
				continue
			}
			if sym, ok := r.prog.SymbolAt(stmt); ok {
				top[sym] = true
			}
		}
	}
	for _, f := range files {
		r.walkCtorFuncs(f, top)
	}
}

func (r *Renderer) walkCtorFuncs(n frontend.Node, top map[frontend.Symbol]bool) {
	switch n.Kind() {
	case frontend.NodeNewExpression:
		if kids := r.prog.Children(n); len(kids) > 0 {
			r.markCtorFunc(kids[0], top)
		}
	case frontend.NodePropertyAccessExpression:
		// F.prototype is the signal, and it is the whole of it: a program that never
		// names the prototype and never constructs is not using the function as a
		// constructor, whatever else it does with it.
		if kids := r.prog.Children(n); len(kids) == 2 && r.prog.Text(kids[1]) == "prototype" {
			r.markCtorFunc(kids[0], top)
		}
	}
	for _, c := range r.prog.Children(n) {
		r.walkCtorFuncs(c, top)
	}
}

// markCtorFunc records the declaration an identifier in constructor position names,
// when that is a top-level function declaration of this program. A class is not one:
// a class has its own lowering, its own construction path, and a .prototype read on
// it is a separate question. Neither is an ambient global, whose Array.prototype and
// Object.prototype chains the static member paths own.
func (r *Renderer) markCtorFunc(n frontend.Node, top map[frontend.Symbol]bool) {
	if n.Kind() != frontend.NodeIdentifier {
		return
	}
	sym, ok := r.prog.SymbolAt(n)
	if !ok || !top[sym] {
		return
	}
	if sym.Flags&frontend.SymbolFunction == 0 {
		return
	}
	r.ctorFuncs[sym] = true
}

// ctorFuncRef reports whether n is an identifier naming a function declaration the
// program uses as an ES5 constructor, so the name reads as the runtime constructor
// value rather than as a Go func.
func (r *Renderer) ctorFuncRef(n frontend.Node) bool {
	if len(r.ctorFuncs) == 0 || n.Kind() != frontend.NodeIdentifier {
		return false
	}
	sym, ok := r.prog.SymbolAt(n)
	return ok && r.ctorFuncs[sym]
}

// ctorFuncDecl reports whether a function declaration is one of them, the question
// the top-level statement loop asks before it emits a Go func for the declaration.
func (r *Renderer) ctorFuncDecl(fn frontend.Node) bool {
	if len(r.ctorFuncs) == 0 || fn.Kind() != frontend.NodeFunctionDeclaration {
		return false
	}
	sym, ok := r.prog.SymbolAt(fn)
	return ok && r.ctorFuncs[sym]
}

// ctorValueRef reports whether an expression evaluates to a runtime constructor value
// rather than to something with a static Go shape. That is a function declaration this
// program uses as an ES5 constructor, and it is also any expression the lowerer already
// holds as a box, which is the form a constructor takes the moment it crosses a module
// boundary: `const ArrayStream = require('../common/arraystream')` binds a value, and
// nothing about that value's static type says it is constructible.
//
// Everything downstream of the question is a runtime check, so claiming a box that
// turns out not to be a constructor costs a TypeError at the point the language throws
// one rather than a wrong answer.
func (r *Renderer) ctorValueRef(n frontend.Node) bool {
	return r.ctorFuncRef(n) || r.isDynamic(n) || r.isBoxedChain(n)
}

// ctorFuncNew reports whether n is a new expression over a constructor function, so
// its result is the boxed instance value.Construct returns rather than a Go value of
// whatever shape the checker inferred for the constructor's instances.
func (r *Renderer) ctorFuncNew(n frontend.Node) bool {
	if n.Kind() != frontend.NodeNewExpression {
		return false
	}
	kids := r.prog.Children(n)
	return len(kids) > 0 && r.ctorFuncRef(kids[0])
}

// The receiver and the argument slice of a lowered constructor body. Both are
// spelled in the bento prefix the other generated bindings take, so a source name
// cannot reach them: mangleIdent passes an identifier through unchanged, so a plain
// name is the one thing a generated name must not look like.
const (
	bentoCtorThisName = "bentoThis"
	bentoCtorArgsName = "bentoArgs"
)

// registerCtorFunc reserves the package-level var a constructor function's name
// binds. It is a value.Value rather than a Go func, so a reference to the name, a
// call of it, and a new of it all read the same slot. Like the callable-object form
// it is a module-assign var, which lands its construction as a plain assignment at
// the top of main and keeps it off the storage-narrowing tiers.
func (r *Renderer) registerCtorFunc(fn frontend.Node) (ast.Decl, error) {
	name, err := r.ctorFuncName(fn)
	if err != nil {
		return nil, err
	}
	r.moduleAssignVars[name] = true
	r.requireImport(valuePkg)
	return &ast.GenDecl{
		Tok: token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{
			Names: []*ast.Ident{ident(name)},
			Type:  sel("value", "Value"),
		}},
	}, nil
}

// ctorFuncEagerRead reports the hand-back for a constructor function a sibling above
// its declaration already reads. A function declaration hoists in JavaScript, so such
// a read is legal there, but the binding here is assigned at the declaration's own
// position, so the read would come back as the zero value. Nothing in Node's helper
// tree is written that way; the constructor is always declared before the lines that
// set its prototype up and before anything constructs one.
func (r *Renderer) ctorFuncEagerRead(fn frontend.Node, siblings []frontend.Node, idx int) error {
	sym, ok := r.prog.SymbolAt(fn)
	if !ok {
		return nil
	}
	for _, prev := range siblings[:idx] {
		if r.subtreeReferencesSymbol(prev, sym) {
			return &NotYetLowerable{Reason: "a constructor function read above its own declaration is a later slice"}
		}
	}
	return nil
}

// lowerLocalCtorFunc emits a constructor function declared at the top level of a
// required module, where the binding is a loader local rather than a package var: the
// var is hoisted to the top of the loader body so every statement below can name it,
// and the construction runs at the declaration's own position. That is what carries
// the shape through require, which is how nearly every program actually meets it:
// Node's test/common/arraystream.js declares ArrayStream and every repl test reaches
// it through require.
func (r *Renderer) lowerLocalCtorFunc(fn frontend.Node) ([]ast.Stmt, error) {
	name, err := r.ctorFuncName(fn)
	if err != nil {
		return nil, err
	}
	lit, err := r.ctorFuncValue(fn, name)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	r.nestedFuncHoists = append(r.nestedFuncHoists, &ast.DeclStmt{Decl: &ast.GenDecl{
		Tok:   token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{Names: []*ast.Ident{ident(name)}, Type: sel("value", "Value")}},
	}})
	return []ast.Stmt{&ast.AssignStmt{
		Lhs: []ast.Expr{ident(name)},
		Tok: token.ASSIGN,
		Rhs: []ast.Expr{lit},
	}}, nil
}

// ctorFuncName is the Go name a constructor function's binding takes, its source
// name mangled the way any local is. It is deliberately not the exported spelling a
// plain function declaration takes: what the name holds is a variable, not a func,
// and every read of it goes through this same helper so the declaration and its
// references cannot drift.
func (r *Renderer) ctorFuncName(fn frontend.Node) (string, error) {
	sym, ok := r.prog.SymbolAt(fn)
	if !ok {
		return "", &NotYetLowerable{Reason: "a constructor function declaration with no symbol is a later slice"}
	}
	name, ok := localName(sym.Name)
	if !ok {
		return "", &NotYetLowerable{Reason: "a constructor function name is not a Go identifier"}
	}
	return name, nil
}

// buildCtorFuncCtors builds the construction statements for the constructor function
// declarations registered above, in declaration order. A function declaration hoists,
// so these run at the top of main, before any statement that writes onto a prototype
// or constructs an instance.
func (r *Renderer) buildCtorFuncCtors(fns []frontend.Node) ([]ast.Stmt, error) {
	stmts := make([]ast.Stmt, 0, len(fns))
	for _, fn := range fns {
		name, err := r.ctorFuncName(fn)
		if err != nil {
			return nil, err
		}
		lit, err := r.ctorFuncValue(fn, name)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, &ast.AssignStmt{
			Lhs: []ast.Expr{ident(name)},
			Tok: token.ASSIGN,
			Rhs: []ast.Expr{lit},
		})
	}
	return stmts, nil
}

// ctorFuncValue lowers a constructor function declaration to the value.NewCtor call
// that builds its runtime value. The body rides inside as an ordinary closure over
// the declaration's own parameters, the same closure a function expression lowers
// to, immediately applied to the boxed arguments the constructor call handed in:
//
//	value.NewCtor("F", 1, func(bentoThis value.Value, bentoArgs []value.Value) value.Value {
//		func(a value.Value) { ... }(value.Arg(bentoArgs, 0))
//		return value.Undefined
//	})
//
// Reusing the closure is what keeps the body on every path an ordinary function body
// already takes: its parameters bind as parameters, its arguments-object plan, its
// dynamic-locals set and its nested declarations are all the existing machinery.
// Only the receiver is new, and it is a plain Go capture the body's `this` reads.
func (r *Renderer) ctorFuncValue(fn frontend.Node, name string) (ast.Expr, error) {
	lit, sig, err := r.receiverFuncLit(fn, "constructor function")
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun: sel("value", "NewCtor"),
		Args: []ast.Expr{
			stringLit(r.prog.Text(fnNameNode(r, fn))),
			intLit(sig.MinArgs),
			lit,
		},
	}, nil
}

// receiverFuncLit lowers a function whose body reads a receiver the call supplies to
// the Go closure the runtime's ctorFn is: it takes that receiver and the boxed
// arguments, and it answers a boxed result.
//
//	func(bentoThis value.Value, bentoArgs []value.Value) value.Value {
//		func(a value.Value) { ... }(value.Arg(bentoArgs, 0))
//		return value.Undefined
//	}
//
// The body rides inside as an ordinary closure over the function's own parameters,
// the same closure a function expression lowers to, immediately applied to the
// arguments the call handed in. Reusing that closure is what keeps the body on every
// path an ordinary function body already takes: its parameters bind as parameters, its
// arguments-object plan, its dynamic-locals set and its nested declarations are all
// the existing machinery. Only the receiver is new, and it is a plain Go capture the
// body's `this` reads.
//
// what names the shape in a hand-back, since the same lowering serves a constructor
// function and a method written onto an object.
func (r *Renderer) receiverFuncLit(fn frontend.Node, what string) (*ast.FuncLit, frontend.Signature, error) {
	var none frontend.Signature
	if r.isAsyncFunc(fn) {
		return nil, none, &NotYetLowerable{Reason: "an async " + what + " is a later slice"}
	}
	if r.isGeneratorFunc(fn) {
		return nil, none, &NotYetLowerable{Reason: "a generator " + what + " is a later slice"}
	}
	sig, ok := r.prog.SignatureAt(fn)
	if !ok {
		return nil, none, &NotYetLowerable{Reason: "a " + what + " with no call signature is a later slice"}
	}
	if sig.RestParam != nil {
		return nil, none, &NotYetLowerable{Reason: "a " + what + " with a rest parameter is a later slice"}
	}
	// A parameter the boxed-signature pass decided holds a box reads as any from here
	// on, the same overlay a function expression's signature takes.
	sig = r.boxedSig(fn, sig)
	fields, err := r.closureParamFields(fn, sig, what)
	if err != nil {
		return nil, none, err
	}

	restore := r.pushCtorThis(fn)
	body, err := r.blockBodyArrow(fn, fields)
	restore()
	if err != nil {
		return nil, none, err
	}

	// The arguments arrive boxed, so each one coerces into the static type the closure's
	// parameter declares, the same crossing an argument makes at any dynamic boundary. A
	// parameter typed any, or one the callback pass forced dynamic, already reads a
	// value.Value and takes the box straight through.
	paramNodes := r.funcParamNodes(fn)
	args := make([]ast.Expr, 0, len(sig.Params))
	for i, p := range sig.Params {
		at := &ast.CallExpr{
			Fun:  sel("value", "Arg"),
			Args: []ast.Expr{ident(bentoCtorArgsName), intLit(i)},
		}
		if p.Type.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0 || r.paramForcedDyn(paramNodes, i) {
			args = append(args, at)
			continue
		}
		coerced, err := r.coerceDynamicToStaticFlags(at, p.Type.Flags)
		if err != nil {
			return nil, none, err
		}
		args = append(args, coerced)
	}
	apply := &ast.CallExpr{Fun: body, Args: args}

	// A body that falls off the end yields undefined, which Construct reads as "keep the
	// object I made" and a method call reads as the undefined the language gives. A body
	// that returns a value hands it on: a dynamic result rides straight out, and a
	// concrete one boxes the way any static result crossing into the value model does.
	var inner []ast.Stmt
	switch ret := sig.Return; {
	case ret.Flags&(frontend.TypeVoid|frontend.TypeUndefined|frontend.TypeNever) != 0:
		inner = []ast.Stmt{
			&ast.ExprStmt{X: apply},
			&ast.ReturnStmt{Results: []ast.Expr{sel("value", "Undefined")}},
		}
	case r.isDynamicType(ret) || r.boxedReturnFns[fn]:
		inner = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{apply}}}
	default:
		boxed, err := r.boxStaticResultToDynamic(apply, ret)
		if err != nil {
			return nil, none, err
		}
		inner = []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{boxed}}}
	}

	r.requireImport(valuePkg)
	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params: &ast.FieldList{List: []*ast.Field{
				{Names: []*ast.Ident{ident(bentoCtorThisName)}, Type: sel("value", "Value")},
				{Names: []*ast.Ident{ident(bentoCtorArgsName)}, Type: &ast.ArrayType{Elt: sel("value", "Value")}},
			}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: sel("value", "Value")}}},
		},
		Body: &ast.BlockStmt{List: inner},
	}, sig, nil
}

// methodFuncBox reports whether a function value crossing into a dynamic slot has to
// carry a receiver, which is the case when it is a function expression whose body
// reads `this`:
//
//	A.prototype.who = function () { return "A:" + this.tag; };
//
// That is the other half of the ES5 constructor function, and the half a program
// spends most of its lines on: the constructor sets up the instance's own fields and
// the prototype holds the methods, every one of which reads `this`. Boxed as an
// ordinary callable the body would read undefined there, so it takes the method box
// instead.
//
// An arrow is never one: its `this` is the enclosing scope's and rebinding it to the
// receiver of whatever call finds it would be wrong. A sloppy-mode body is not one
// either, since a receiver-free call there is the global object bento has no value
// for, which plainThisRefusal already hands back.
func (r *Renderer) methodFuncBox(n frontend.Node) bool {
	if n.Kind() != frontend.NodeFunctionExpression {
		return false
	}
	if !subtreeHasKind(r.prog, n, frontend.NodeThisKeyword) {
		return false
	}
	return r.strictAt(n)
}

// methodFuncValue lowers such a function expression to the runtime method value. The
// closure is the receiver-taking one a constructor body rides in, so a method and the
// constructor it hangs off read `this` through exactly the same binding. The name the
// box carries is the one the ordinary box would have recorded, so f.name is unchanged.
//
// A shape the receiver-taking closure cannot spell reports ok=false rather than
// handing the unit back, and the caller boxes it the way it always did. That keeps
// every function already lowering on its old path: a callback with a rest parameter
// that reads `this` only to hand it straight to an apply, the shape Node's mustCall
// wrapper takes, still lowers with the undefined a receiver-free call leaves, which is
// the receiver Node's own invocation puts there too.
func (r *Renderer) methodFuncValue(n frontend.Node) (ast.Expr, bool, error) {
	if _, named := r.funcExprNameNode(n); named {
		// A named function expression's own name is in scope inside its body, which takes
		// the self-reference two-step the plain closure form here does not build.
		return nil, false, nil
	}
	lit, _, err := r.receiverFuncLit(n, "function expression reading this")
	if err != nil {
		var nyl *NotYetLowerable
		if errors.As(err, &nyl) {
			return nil, false, nil
		}
		return nil, false, err
	}
	r.requireImport(valuePkg)
	var boxed ast.Expr = &ast.CallExpr{Fun: sel("value", "NewMethod"), Args: []ast.Expr{lit}}
	if name := r.boxedFuncName(n); name != "" {
		boxed = &ast.CallExpr{Fun: sel("value", "WithName"), Args: []ast.Expr{boxed, stringLit(name)}}
	}
	return boxed, true, nil
}

// fnNameNode returns the identifier a function declaration binds, so the runtime
// value carries the name the source gave it and f.name answers with it.
func fnNameNode(r *Renderer, fn frontend.Node) frontend.Node {
	kids := r.prog.Children(fn)
	if len(kids) > 0 && kids[0].Kind() == frontend.NodeIdentifier {
		return kids[0]
	}
	return fn
}

// pushCtorThis enters a constructor body, where `this` is the receiver [[Construct]]
// bound rather than the undefined a receiver-free call leaves. An enclosing class
// scope is cleared for the same reason pushPlainThis clears it: a method's receiver
// is not this constructor's, and a this.x inside must not resolve against it.
func (r *Renderer) pushCtorThis(fn frontend.Node) func() {
	prevClass, prevThis, prevStatic := r.curClass, r.thisName, r.staticClass
	prevPlain, prevRecv, prevCtor := r.thisPlain, r.recvPos, r.ctorThisName
	r.curClass, r.thisName, r.staticClass = nil, "", nil
	r.thisPlain, r.recvPos = true, false
	r.ctorThisName = bentoCtorThisName
	return func() {
		r.curClass, r.thisName, r.staticClass = prevClass, prevThis, prevStatic
		r.thisPlain, r.recvPos, r.ctorThisName = prevPlain, prevRecv, prevCtor
	}
}

// newCtorFunc lowers `new F(args)` over a constructor value to the runtime
// [[Construct]]. Each argument boxes, the way it does for any dynamic call, since
// the body reads its parameters out of a value slice.
func (r *Renderer) newCtorFunc(callee frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	fnExpr, err := r.lowerExpr(callee)
	if err != nil {
		return nil, err
	}
	args := make([]ast.Expr, 0, len(argNodes)+1)
	args = append(args, fnExpr)
	for _, a := range argNodes {
		if a.Kind() == frontend.NodeSpreadElement {
			return nil, &NotYetLowerable{Reason: "a spread argument in a constructor call is a later slice"}
		}
		boxed, err := r.boxOperand(a)
		if err != nil {
			return nil, err
		}
		args = append(args, boxed)
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "Construct"), Args: args}, nil
}

// ctorFuncInstanceof lowers `v instanceof F` over a constructor function to the
// runtime chain walk. The result is a Go bool, the same static shape every other
// lowered comparison yields, so a condition reads it directly.
func (r *Renderer) ctorFuncInstanceof(left, right frontend.Node) (ast.Expr, error) {
	obj, err := r.boxOperand(left)
	if err != nil {
		return nil, err
	}
	fn, err := r.lowerExpr(right)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "InstanceOf"), Args: []ast.Expr{obj, fn}}, nil
}

// ctorFuncMethodCall lowers F.call(recv, args...) over a constructor value to the
// runtime call that honors the receiver. That is constructor chaining, how ES5 code
// calls a base constructor from a derived one:
//
//	function ArrayStream() { Stream.call(this); }
//
// apply gathers its arguments into an array the callee then reads positionally, which
// is a different bridge, so it hands back under its own reason rather than being
// folded in here.
func (r *Renderer) ctorFuncMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, bool, error) {
	if method != "call" {
		return nil, false, &NotYetLowerable{Reason: "apply on a constructor value is a later slice"}
	}
	fn, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, false, err
	}
	args := make([]ast.Expr, 0, len(argNodes)+1)
	args = append(args, fn)
	if len(argNodes) == 0 {
		// F.call() with nothing at all binds undefined, the receiver a bare call leaves.
		args = append(args, sel("value", "Undefined"))
	}
	for _, a := range argNodes {
		if a.Kind() == frontend.NodeSpreadElement {
			return nil, false, &NotYetLowerable{Reason: "a spread argument in a constructor function call is a later slice"}
		}
		boxed, err := r.boxOperand(a)
		if err != nil {
			return nil, false, err
		}
		args = append(args, boxed)
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "CallWithThis"), Args: args}, true, nil
}
