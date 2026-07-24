// Package lower is bento's ahead-of-time type lowering: it turns the resolved,
// flow-narrowed types the frontend hands it (pkg/frontend) into the Go source
// the Go toolchain then compiles. It implements 05_type_lowering.md.
//
// Lowering is a translation, not a type inference. The frontend already ran the
// real TypeScript checker, so every type that reaches here is settled, and the
// partitioner (pkg/partition, 06_compile_vs_interpret.md) already promised the
// unit is lowerable before handing it over. Lowering's contract in return is
// that it never emits unsound Go: when it meets a construct it does not render,
// it reports a NotYetLowerable error rather than guess, and the caller routes
// that unit to the engine (05_type_lowering.md section 30).
//
// This file owns the type renderer: the mapping from one frontend.Type to the
// Go type expression that represents it, together with the generated named
// declarations (structs today, tagged unions and vtables in later slices) that
// the expression refers to. The mapping table is 05_type_lowering.md section
// 31; each row is a case here or an explicit NotYetLowerable until its slice
// lands.
package lower

import (
	"fmt"
	"go/ast"
	"sort"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/goimport"
)

// valuePkg is the import path of the shared value model, the package that
// defines value.BStr and the other runtime value types the emitted Go refers
// to. Doc 05 section 2 fixes that emitted Go imports the runtime by real path.
const valuePkg = "github.com/tamnd/bento/pkg/value"

// NotYetLowerable is the reason lowering hands a unit back to the partitioner
// instead of emitting Go. It is not an internal failure: it is the honest edge
// of the compiled subset (05_type_lowering.md section 30), naming the construct
// that has no lowering yet so the partitioner can route the unit to the engine
// and a later slice can grow the covered set.
type NotYetLowerable struct {
	// Flags is the coarse classification of the type that could not be lowered,
	// enough for a diagnostic without holding the type across the boundary.
	Flags frontend.TypeFlags
	// Reason is a short human explanation of why this construct is not lowered
	// yet, phrased as the boundary it hit.
	Reason string
}

func (e *NotYetLowerable) Error() string {
	return fmt.Sprintf("not yet lowerable (flags %v): %s", e.Flags, e.Reason)
}

// Renderer renders the types of one checked program to Go. It accumulates the
// named declarations its rendered expressions refer to, so a caller renders a
// set of types and then emits Decls once. A Renderer is scoped to a single
// program because it keys generated struct names on the program's structural
// type identity (05_type_lowering.md section 29), which is only stable within
// one program.
type Renderer struct {
	prog    *frontend.Program
	// programStrict is set when the entry module opens with a "use strict" directive
	// prologue, so a member store lowers to the throwing SetStrict rather than the
	// silent-drop Set, the way a strict script observes a failed assignment.
	programStrict bool
	decls   *declSet
	imports map[string]bool
	// nodeImports maps a local binding name introduced by a node: import to the
	// builtin it names, so a call to that binding lowers to the value helper the
	// builtin maps to rather than a user function. It is populated once from the
	// entry module's import declarations before any body is lowered, since a
	// function or a top-level statement may call an imported builtin.
	nodeImports map[string]nodeBuiltin
	// goImports maps a local binding name introduced by a go: import to the Go
	// symbol it names, so a call to that binding lowers to a direct call into the
	// real Go package rather than a user function. Like nodeImports it is populated
	// once from the entry module's import declarations before any body is lowered.
	goImports map[string]goBuiltin
	// goNamespaces maps a local binding introduced by a namespace go: import (import
	// * as zstd from "go:...") to the Go import path it names, so a member call on the
	// binding (zstd.NewReader(...)) lowers to a direct call into that package. It is
	// the namespace twin of goImports, which holds the named-import bindings; a call
	// site consults it when the callee is a property access on a namespace binding.
	goNamespaces map[string]string
	// internalNamespaces holds the local name of every namespace import of a composed
	// sibling module (import * as m from "./m"). A sibling's exports are package-level
	// Go declarations, so a member call on the binding (m.inc(1)) lowers to a direct
	// call to the export's Go name, the same name a named import of it would spell. A
	// call site consults this set to route such a call; a value read of the binding or
	// one of its members hands back, since a namespace bound as a first-class value
	// needs a struct the slice does not build.
	internalNamespaces map[string]bool
	// dynImportNamespaces holds the local names of bindings introduced by a static
	// dynamic import, const m = await import("./mod"), whose specifier resolves to a
	// composed sibling. Such a binding names the same compile-time namespace a static
	// import * as m does, so it also joins internalNamespaces for member resolution;
	// this set additionally marks its declaration so the statement lowers to the await
	// suspension alone, with no Go var, the way a namespace binding carries no runtime
	// value.
	dynImportNamespaces map[string]bool
	// goAliases maps a Go import path to the local alias the emitted file imports it
	// under, assigned on first call into the package so an imported-but-never-called
	// package emits no import. Every call into one package renders the same alias, so
	// the qualifier a call emits and the import spec the file carries agree.
	goAliases map[string]string
	// goSigs resolves the Go signature of a go: function so a call marshals numbers
	// by the Go type the TypeScript number cannot distinguish (int, int64, float64
	// all project to number). It is optional: with no resolver, a go: call lowers
	// only the crossings the TypeScript type settles on its own (string, boolean),
	// which keeps the renderer usable without the Go toolchain at hand. The build
	// wires it from the same package load the declaration generator ran.
	goSigs func(importPath, name string) (goimport.FuncSig, bool)
	// goConsts resolves the Go type keyword of a go: constant so a reference to one
	// marshals by the Go type the TypeScript number cannot distinguish, the same
	// disambiguation goSigs does for a call. It is optional and wired from the same
	// package load; with no resolver a reference to a go: binding used as a value
	// hands back, since the renderer cannot tell a constant from a function value.
	goConsts func(importPath, name string) (goimport.ConstInfo, bool)
	// goErrorVars reports whether a name exported by a go: package is a sentinel
	// error variable (io.EOF, sql.ErrNoRows), so a caught error's is() lowers to
	// errors.Is against the real Go variable and branches on identity the way Go
	// code does (section 7.7). It is optional and wired from the same package load;
	// with no resolver err.is against a go: sentinel hands back, since the renderer
	// cannot tell an error variable from any other exported binding.
	goErrorVars func(importPath, name string) bool
	// retType is the declared return type of the function whose body is currently
	// being lowered, so a return statement can coerce its value across the dynamic
	// boundary the way an assignment does. It is the zero type outside a function
	// body, and it is saved and restored around each body so a nested function does
	// not inherit the outer return type.
	retType frontend.Type
	// argsObjName is the Go name of the *value.Array[value.Value] local the current
	// body materialized to back its arguments object, or "" when the body does not
	// read arguments. It is set around a body whose arguments use is a supported
	// shape (see arguments.go), and reads of arguments in that body route through it:
	// arguments.length to its Len, arguments[i] to its At. Like retType it is saved
	// and restored around each body so a nested function does not inherit the outer
	// object.
	argsObjName string
	// argsWriteSafe reports whether a write to arguments[i] in the current body is
	// safe to lower against the materialized store. The store is a snapshot of the
	// parameters, so a write to it is the unmapped (strict) rule where arguments does
	// not alias the named parameters. That matches the mapped (sloppy) rule only when
	// no parameter is read by name in the body, since then the two rules are
	// indistinguishable. When a parameter is referenced by name the write is the
	// aliasing corner the snapshot cannot mirror, so it hands back. It is saved and
	// restored alongside argsObjName.
	argsWriteSafe bool
	// argsThreads memoizes whether a top-level function symbol threads the real
	// call-site arguments through a hidden trailing parameter (see argumentsthread.go).
	// The decision is a pure function of the symbol, consulted once at the declaration
	// to add the hidden parameter and again at every call site to pass the array, so it
	// is cached to keep the whole-program reference walk it runs off the hot path.
	argsThreads map[frontend.Symbol]bool
	// genCo is the Go name of the *value.GenCo handle the current generator body
	// yields through, or "" when the body being lowered is not a generator. It is set
	// around a generator function or method body, and a yield expression in that body
	// lowers to a call on it (genCo.Yield). Like retType it is saved and restored around
	// each body so a nested non-generator function does not inherit it.
	genCo string
	// genYieldType is the element type a generator body yields, the Y in *value.Gen[Y].
	// It fixes the type argument of the NewGen call the body is wrapped in and the type
	// a yield expression coerces its operand to. It is the zero type outside a generator
	// body, and it is saved and restored alongside genCo.
	genYieldType frontend.Type
	// asyncCo is the Go name of the *value.AsyncCo handle the current suspending async
	// body awaits through, or "" when the body being lowered is not an async body that
	// awaits (an await-free async body settles synchronously and needs no handle). An
	// await expression in that body lowers to a call on it (value.Await(asyncCo, p)). Like
	// genCo it is set around the body and saved and restored so a nested function does not
	// inherit it.
	asyncCo string
	// inAsyncGen marks that the body being lowered is an async generator, whose single
	// coroutine handle serves both yield and await: genCo and asyncCo both name it, and an
	// await routes to value.AsyncGenAwait(co, p) rather than value.Await so it parks the
	// async generator's driver instead of a plain async body's. It is set around the async
	// generator body and saved and restored alongside genCo and asyncCo.
	inAsyncGen bool
	// typeSubst maps a type parameter's identity to the concrete type it stands for in
	// the specialization currently being lowered, so typeExpr resolves a bare T to the
	// float64, value.BStr, or array type the call site fixed it to. It is set around one
	// monomorphized instantiation of a generic function (see mono.go) and, like retType,
	// saved and restored so one specialization's bindings do not leak into another. A nil
	// map (the default, outside a generic body) resolves nothing, so a non-generic body
	// lowers exactly as before.
	typeSubst map[int]frontend.Type
	// have been proven to hold only 32-bit integers and are therefore given a Go
	// int32 type rather than a float64. It is computed once per body by int32LocalsOf
	// and, like retType, saved and restored around each body so one function's
	// specialized locals do not leak into another. A nil map (the default) specializes
	// nothing, so a body with no eligible local lowers exactly as before.
	int32Locals map[string]bool
	// int64Locals is the set of local names in the body currently being lowered
	// whose values are proven to stay inside the safe-integer range while reaching
	// past 32 bits, and are therefore given a Go int64 type rather than a float64.
	// It is computed once per body by int64LocalsOf, after int32Locals and
	// counterIvl since the proof reads both, and, like int32Locals, saved and
	// restored around each body. A nil map (the default) specializes nothing.
	int64Locals map[string]bool
	// counterIvl maps each bounded increasing for-counter in the body currently being
	// lowered to the integer interval it ranges over, so an index built from a counter
	// can be proven to sit inside a fixed-length array. It is computed once per body by
	// counterIvlOf and, like int32Locals, saved and restored around each body. A nil
	// map proves no ranges, so a body with no such counter lowers exactly as before.
	counterIvl map[string]ivl
	// fixedTArr maps each fixed-length integer typed-array local in the body currently
	// being lowered to its length and Go element type, so an access at a proven-in-range
	// index reads or writes the backing slice directly rather than through the checked
	// At and SetAt. It is computed once per body by fixedTypedArraysOf and, like
	// int32Locals, saved and restored around each body. A nil map takes no native slice
	// path.
	fixedTArr map[string]typedArrInfo
	// constInt maps each const local in the body currently being lowered that is
	// initialized to an integer literal in the int32 range and never reassigned to
	// that literal value. The range analysis resolves such a name to its value where
	// it expects an integer literal, so an idiomatic const N = 4096 used as a typed
	// array length or a loop bound is recognized as the constant it is. It is computed
	// once per body by constIntsOf and, like int32Locals, saved and restored around
	// each body. A nil map resolves no names, so a body with no such const lowers
	// exactly as before.
	constInt map[string]int
	// optLocals is the set of local names in the body currently being lowered that
	// are declared with an optional type (T | undefined, lowered to value.Opt[T]).
	// It is computed once per body by optLocalsOf and, like int32Locals, saved and
	// restored around each body. A read of one of these locals at a point the checker
	// narrowed to T (past a presence guard) unwraps with .Get(), the only place the
	// stored T is pulled out of the option; a read where the type is still optional
	// keeps the bare Opt value. A nil map (the default) unwraps nothing.
	optLocals map[string]bool
	// definiteLocals is the set of local names in the body currently being lowered
	// that are declared with a non-optional, non-dynamic static type and no
	// initializer, and that no nested closure captures. Such a binding lowers to a
	// plain `var x T` at the declared Go type: the checker's strict definite-assignment
	// analysis rejects any direct read before the first assignment, so the Go zero value
	// is never observed as a JavaScript value, and the closure exclusion covers the one
	// read shape that analysis does not police. It is computed once per body by
	// definiteLocalsOf and, like optLocals, saved and restored around each body. A nil
	// map (the default) keeps every no-initializer typed binding on the handback path.
	definiteLocals map[string]bool
	// optParams is the set of parameter names in the body currently being lowered
	// that bind a bare optional (x?: T, no default, lowered to a value.Opt[T] field).
	// It is the parameter counterpart to optLocals, kept separate because optLocals is
	// recomputed per block in scopedBlockRange from the body's declarations, which do
	// not include the signature, so a param's optional-ness would be lost there. It is
	// built once per body next to dynLocals and saved and restored the same way; a read
	// of one of these params the checker narrowed to T unwraps with .Get() exactly like
	// an optional local. A nil map (the default) holds no optional params.
	optParams map[string]bool
	// dynLocals is the set of names in the body currently being lowered that are
	// bound as boxed dynamic values: a parameter or a local typed any or unknown,
	// which typeExpr lowers to value.Value. A read of one at a point the checker
	// narrowed to a single primitive, past a typeof guard, goes through the
	// matching accessor (AsString, AsNumber, AsBool) so the static expression it
	// flows into sees the unboxed Go value; a read still typed any keeps the bare
	// box, which is what the runtime helpers take. It is built per body next to
	// unionLocals and saved and restored the same way. A nil map unwraps nothing.
	dynLocals map[string]bool
	// dynBoundLocals is the set of names in the body currently being lowered that an
	// untyped destructuring pattern bound to a boxed value.Value even though the checker
	// gave them a non-any type: an object rest element, whose type is the fixed object
	// shape { the pattern did not name } rather than any. The runtime value is the plain
	// object ObjectRest built, so a read of a property off it must dispatch through the
	// dynamic Get and not fold to a fixed-shape miss. isDynamic consults this so such a
	// read routes the boxed way, and it rides the same per-body save and restore as
	// dynLocals so one function's rest bindings do not leak into another's reads. A nil
	// map (the default) marks nothing.
	dynBoundLocals map[string]bool
	// errorLocals is the set of catch-binding names in scope while a catch block is
	// lowered, each bound to the *value.Error the catch recovered. A read of the
	// binding's .message or .name lowers to the matching method on the error; the
	// binding used any other way hands back, since the runtime models a caught error
	// as a value.Error and not a general boxed value yet. It is set around a catch
	// block and cleared after, so a binding does not leak past its clause.
	errorLocals map[string]bool
	// funcExprSelf maps a named function expression's own name symbol to the Go local
	// variable its two-step lowering binds the closure to, so a recursive call inside
	// the body resolves to that local rather than to a top-level function name that
	// does not exist. It is set around the body of a named function expression and
	// cleared after, so the self name does not leak past the expression.
	funcExprSelf map[frontend.Symbol]string
	// paramAliases maps a constructor parameter symbol to a renamed Go name given when
	// the parameter's own Go name would shadow an outer binding a field initializer
	// reads. A field initializer runs outside the constructor's parameter scope, so its
	// reference to an outer const or var must reach that binding, not the parameter of
	// the same name the emitted constructor puts in scope; the parameter is renamed and
	// every reference to it, its signature, a parameter-property store, its body reads,
	// resolves through this map. It is set around the constructor whose parameters
	// collide and keyed by the unique parameter symbol, so an entry never leaks.
	paramAliases map[frontend.Symbol]string
	// scopeParams is the set of Go parameter names bound by the function whose body is
	// currently being lowered. A nested function declaration binds a Go local at the
	// top of that body, so its Go name must not collide with an enclosing parameter's;
	// the nested-function pass consults this set to hand back on a collision rather than
	// emit a Go redeclaration. A nil map also switches the nested-function pass off, so
	// a body type that does not populate it (a method, generator, async, or constructor)
	// keeps the older handback for a nested declaration rather than emit one this guard
	// did not vet. It is saved and restored around each body like retType.
	scopeParams map[string]bool
	// nestedFuncPlans holds the emission plan for each nested function declaration the
	// current block registered, keyed by its node, so lowerNestedFuncDecl emits the
	// closure the enterNestedFuncScope pass already vetted. An entry is added when a
	// block enters and removed when it restores, so a declaration outside the current
	// block is not in the map and falls to the older handback.
	nestedFuncPlans map[frontend.Node]*nestedFuncPlan
	// arrowDropDefaults marks an arrow-function node whose defaulted parameters lower
	// as plain Go fields with no default, because collectArrowDefaults proved the
	// const binding the arrow initializes never escapes as a value: every call to it is
	// a direct call the call site can fill the omitted default at, the same way a
	// top-level function's default is filled. Without this an arrow default hands back,
	// since a Go func value carries no optional parameter. It is filled once by
	// collectArrowDefaults before any body lowers.
	arrowDropDefaults map[frontend.Node]bool
	// arrowCallDefaults maps such an escape-safe arrow's binding symbol to its
	// parameter defaults, aligned to the parameter list with a nil where a parameter
	// has none. A direct call to the binding reads it to fill an omitted or
	// explicit-undefined trailing argument at the call site, exactly as calleeDefaults
	// serves a top-level function. It is filled alongside arrowDropDefaults.
	arrowCallDefaults map[frontend.Symbol][]frontend.Node
	// closurePadParams maps a function-literal argument node to the contextual target
	// signature's parameters when the literal declares fewer parameters than the func
	// type it flows into. JavaScript lets a callback ignore trailing arguments, so a
	// zero-parameter literal is assignable to a one-parameter callback type, but Go
	// requires the func value's type to match the slot exactly. closureParamFields
	// reads this to append the missing trailing parameters as blank-named fields of the
	// target's Go types, so the emitted literal's func type equals the slot's. It is set
	// by lowerArgAt around the literal's lowering and cleared after.
	closurePadParams map[frontend.Node][]frontend.Param
	// promiseSettleParams maps a Promise executor's resolve and reject parameter names
	// to how each settles the promise while that executor body lowers. A call to one
	// is not a plain function-value call: resolve carries a value of the element type
	// and reject an arbitrary boxed value, so the callee is intercepted in callExpr and
	// its argument bridged the settle way rather than through the callback's lib.d.ts
	// signature, whose unions and optionals do not render. It is set around the
	// executor body by newPromise and cleared after, so the names do not leak.
	promiseSettleParams map[string]promiseSettle
	// monoSpecs maps a generic top-level function's symbol to the distinct
	// monomorphizations the program's call sites ask for, one Go function emitted per
	// entry. It is filled once by collectMono in RenderProgram, before any body lowers,
	// so the declaration knows every specialization to emit and a call site can look up
	// the specialized Go name it resolves to. A symbol absent from the map is a generic
	// no call site monomorphizes, which hands back rather than emit an unspecialized func.
	monoSpecs map[frontend.Symbol][]monoSpec
	// monoMethodSpecs maps a generic method's declaration node to the distinct
	// monomorphizations its call sites ask for. A Go method cannot carry a type
	// parameter, so a generic method emits one mangled Go method per instantiation
	// (Wrap_num, Wrap_str) and each call rewrites to the one it resolves to. It is
	// filled once by collectMonoMethods in RenderProgram, before any body lowers, so
	// the class knows every specialization to emit and a call site agrees on the name.
	// A method absent from the map is a generic no call site monomorphizes, which
	// hands back rather than emit an unspecialized method.
	monoMethodSpecs map[frontend.Node][]monoSpec
	// tryRet tells a return statement how to leave the try construct it sits in.
	// The zero value is a plain function return. tryRetBody marks the body of the
	// escape closure a try with an escaping return compiles to, where a return
	// fills the closure's named results with `return x, true`. tryRetDefer marks
	// a catch or finally body, which runs inside a deferred function, where a
	// return assigns the named results and leaves with a bare return, the only
	// way a deferred function can set them. It is saved and restored around each
	// function body like retType so a nested function's returns stay its own.
	tryRet tryRetMode
	// usesThrow records that the program emitted a construct that can raise a
	// thrown value the runtime must report if it escapes: an explicit throw, or a
	// boundary crossing that range-checks (a go: int64 result). When set, the
	// assembled main defers value.ReportUncaught so an uncaught throw prints an
	// uncaught-error line and exits non-zero rather than crashing with a Go stack. A
	// program that cannot throw defers nothing, so its main and its imports are
	// unchanged.
	usesThrow bool
	// usesPromise records that the program minted a promise: an async method
	// lowered to a value.Promise, so the assembled main drains the microtask queue
	// once at its end (value.RunMicrotasks). That drain is doc 11's
	// run-to-completion point collapsed to the single turn a compiled test262 job
	// has: every promise 6a mints is already settled, so one end-of-main drain runs
	// each queued .then callback in order. A program that mints no promise drains
	// nothing, so its main is unchanged.
	usesPromise bool
	// usesCommonJSModule records that the program read the CommonJS module or exports
	// wrapper global, so the assembled program emits the package-level module object
	// and its exports alias (see commonjs.go). A module object is one value.Object with
	// an exports property; the exports alias holds the same object the property starts
	// at, so exports.x and module.exports.x reach one object, and a later
	// module.exports = v reassigns the property without moving the alias, the divergence
	// Node's wrapper has. A program that names neither global emits no such state.
	usesCommonJSModule bool
	// tmpSeq is a monotonic counter the lowerer draws generated temporary names from,
	// for the places a single source construct needs a Go local with no source name:
	// the element a destructuring for...of binds before it reads each position out of
	// it. It runs across the whole program so two temporaries never share a name, and
	// it advances in the deterministic lowering order so the emitted names are stable
	// from one render to the next.
	tmpSeq int
	// strBuilders is the list of reusable value.StrBuilder variables the current
	// body needs, one per template or number-interpolated concatenation site the
	// lowerer chose to build through a builder. A var declaration for each is hoisted
	// to the top of the body, above any loop, so the builder is reused across
	// iterations rather than recreated. Like int32Locals it is scoped to one body,
	// saved and restored around each so one function's builders do not leak into
	// another, and a nil slice (the default) hoists nothing.
	strBuilders []string
	// bigOwned is the set of bigint local names in the body currently being lowered
	// whose *big.Int is provably unshared, so a self-referential update like
	// acc = acc * i mutates in place with acc.Mul(acc, i) instead of allocating a
	// fresh big.Int per iteration. It is computed once per body by bigOwnedLocalsOf
	// and, like int32Locals, saved and restored around each body. A nil map (the
	// default) owns nothing, so every bigint write keeps the always-fresh form.
	bigOwned map[string]bool
	// bigLits interns the wide bigint literals of the program: a literal past int64
	// cannot be a big.NewInt call, so it becomes a package-level var parsed once at
	// init by value.BigIntMustParse, and every site that writes the same value reads
	// the same var. The map keys the literal's canonical decimal digits to the var
	// name; bigLitOrder remembers first-use order so the emitted var block is
	// deterministic. Program-scoped, not body-scoped: two functions naming the same
	// constant share one var.
	bigLits     map[string]string
	bigLitOrder []string
	// classes registers the module's top-level classes by source name, collected
	// in a pre-pass so a body can construct an instance of a class declared below
	// it; classOrder keeps source order so the emitted declarations are
	// deterministic. curClass and thisName are the body scope: inside a lowered
	// constructor or method they name the class and its receiver identifier so
	// this lowers to the receiver, and they are saved and restored around each
	// body the way retType is.
	classes    map[string]*classInfo
	classOrder []string
	curClass   *classInfo
	thisName   string
	// staticClass is the class whose static method body is being lowered, set
	// only inside a static function where there is no receiver, so super.m() in a
	// static method can resolve to the base class's static function while a bare
	// super or an instance-shaped super read still hands back through the
	// receiver gate the instance path uses.
	staticClass *classInfo
	// classTaken is the spoken-identifier set the class pre-pass built, kept
	// so the vtable machinery can check the names it mints at render time
	// (the vtable type and vars, the init functions) the same way
	// registration checks a constructor's name.
	classTaken map[string]bool
	// enums registers the module's top-level numeric enums by source name,
	// collected in a pre-pass so a member read A.B below the declaration still
	// resolves; enumOrder keeps source order so the emitted const blocks are
	// deterministic. A non-const enum emits a float64-backed const per member and
	// each member read lowers to that const; a const enum emits nothing and each
	// read inlines the member's numeric value.
	enums     map[string]*enumInfo
	enumOrder []string
	// unions interns the module's general (tagged-sum) unions in first-seen order,
	// each emitted as a package-level tag type, discriminant const block, sum
	// struct, and one wrapping constructor per arm. unionBySig maps a union's
	// structural signature to its interned descriptor so two structurally equal
	// unions share one Go type, the same shape-keyed dedup the struct interner uses.
	// Program-scoped, not body-scoped: a union named in two functions is one type.
	unions     []*unionInfo
	unionBySig map[string]*unionInfo
	// unionLocals is the set of local and parameter names in the body currently
	// being lowered whose declared type is an interned tagged-sum union, mapped to
	// that union's descriptor. A read of one at a point the checker narrowed to a
	// single arm reads that arm's field off the struct; a read where the type is
	// still the whole union keeps the bare struct. It is computed once per body and,
	// like optLocals, saved and restored around each body. A nil map reads nothing
	// out, so a body with no union binding lowers exactly as before.
	unionLocals map[string]*unionInfo
	// bindingUses counts, per binding symbol, how many identifiers in the module
	// resolve to it. A local declared and never read shows a count of one, the
	// declaration itself, and gets a trailing blank assignment so the emitted Go
	// does not trip the compiler's declared-and-not-used check while the
	// initializer still runs for its side effect, the way an unused `var x = e;`
	// still evaluates e in JavaScript. It is program-scoped and computed once in
	// RenderProgram, since a symbol is unique to its binding across the module.
	bindingUses map[frontend.Symbol]int
	// elidedUses counts, per binding symbol, the identifier references a fold drops
	// from the emitted Go. Object.keys(o) and its siblings read only o's static shape
	// and never lower o, and typeof x over a statically typed x folds to a constant
	// tag and drops x, so the source counts a read the emit does not make;
	// bindingUnused subtracts those dropped reads and still blanks a binding the fold
	// orphaned. It is filled alongside bindingUses in RenderProgram.
	elidedUses map[frontend.Symbol]int
	// writeUses counts, per binding symbol, the identifier references that write the
	// binding without reading it: the left side of a plain `x = e` assignment. Go
	// counts only reads toward a variable being used, so a local that is declared and
	// then only written (var x; x = 1) is declared-and-not-used in the emitted Go
	// even though the source names it more than once; bindingUnused subtracts these
	// write-only references so such a binding still gets its trailing blank. A
	// compound assignment (x += e) and a ++/-- read the binding first, so those are
	// not counted here. It is filled alongside bindingUses in RenderProgram.
	writeUses map[frontend.Symbol]int
	// bindingDecls counts, per binding symbol, the declaration name nodes that name
	// it. It is normally one, the single `var`/`let`/`const` that introduces the
	// binding, but JavaScript lets a `var` redeclare a name in the same scope, so
	// `{ var f; var f; }` is one binding with two declaration name nodes. bindingUnused
	// compares the surviving reference count against this so a redeclared-but-unread
	// binding is still recognized as unused and blanked, rather than its extra
	// declarations reading as though they were uses. It is filled alongside
	// bindingUses in RenderProgram.
	bindingDecls map[frontend.Symbol]int
	// storedCollIters records, per binding symbol, a `const it = m.keys()` and its
	// siblings whose Map or Set iterator the module stores in a local and then consumes
	// exactly once, as a for...of iterable or a spread operand. Such a binding's type is
	// the iterator object, which does not lower, so the declaration would hand the whole
	// unit back; instead the declaration emits nothing and the one consumer ranges the
	// receiver's snapshot the direct `for (const x of m.keys())` and `[...m.keys()]`
	// forms already lower to. It is filled once in RenderProgram and read at the
	// declaration (to suppress it) and at the consumer (to see through the identifier to
	// the receiver and accessor). A binding referenced more than once, or read anywhere
	// but that one consumer, is not recorded, so the stored iterator's single-use
	// semantics are preserved. It is program-scoped.
	storedCollIters map[frontend.Symbol]storedCollIter
	// collMutations records, per Map or Set binding symbol, the source positions of every
	// mutating call on it: add, set, delete, or clear. A manual drive of a stored Map or
	// Set iterator reads it to prove its receiver is not mutated after the iterator is
	// minted, since the drive walks a snapshot taken at mint time and a later mutation
	// would diverge from a live iterator's view. It is filled once in RenderProgram and
	// read at each iterator construction site. It is program-scoped.
	collMutations map[frontend.Symbol][]frontend.Pos
	// blockDeclared is a stack of the local names already given a Go declaration in
	// each open block, innermost last. A block is pushed when its statements start
	// lowering and popped when they finish, so the top frame names the bindings the
	// current Go block has already declared. JavaScript lets a `var` redeclare a name
	// in the same scope, but Go rejects a second `x :=` on a name already declared in
	// the block, so a redeclaration lowers to a plain assignment instead. The stack
	// is empty outside a block (a for-loop initializer, say), where no redeclaration
	// can arise, and that empty case declares normally.
	blockDeclared []map[string]bool
	// usingTopScope is a one-shot flag the two top-level statement lists, a function
	// body and the program main body, set right before they lower their statements, so
	// a `using` declaration among them lowers its scope-exit disposal to a Go defer,
	// whose function scope coincides with the JavaScript block scope there. It is
	// consumed at lowerStatements entry and cleared, so a nested block lowered from
	// within (an if body, a loop body, a switch case) sees it false and a `using` there
	// hands back rather than defer disposal to the wrong scope.
	usingTopScope bool
	// hoistedVars names the `var` bindings the active scope declared at its top
	// because a nested block writes one that another block reads. JavaScript scopes
	// a var to the whole function or module, not the block it sits in, so such a var
	// is one binding declared once at the scope top; the in-block `var name = e` then
	// lowers to a plain assignment. It is set per scope (the program body and each
	// function body) and restored on exit, so one scope's hoists do not leak into
	// another.
	hoistedVars map[string]bool
	// fwdHoisted names the callable-object bindings the active scope declared at
	// its top because a statement above the binding captures its name in a closure.
	// JavaScript scopes the const to the whole module or function, so the closure's
	// forward reference is legal there; Go needs the pointer declared first, so the
	// binding's own site lowers to a plain assignment. It is set per scope and
	// restored on exit, so one scope's forward hoists do not leak into another.
	fwdHoisted map[string]bool
	// fwdHoistedFunc names the plain function-valued bindings the active scope
	// declared at its top for the same reason fwdHoisted does, an earlier
	// statement's closure captures the name, but which lower to a bare Go func type
	// rather than a callable-object pointer. The pre-declaration is a `var name
	// func(...)...` and the binding's own site takes the ordinary var path, where
	// redeclaredVarAssign turns it into a plain assignment. Kept separate from
	// fwdHoisted because the two pre-declare different Go types and the callable
	// binding is intercepted earlier by flattenCallableBinding.
	fwdHoistedFunc map[string]bool
	// moduleAssignVars names the module-level bindings hoisted to a package-level var
	// whose initializer is not safe to evaluate at package-init time (a call, or an
	// expression over other module state). The binding is declared zero-valued at
	// package scope so a top-level function can read it, and its own statement stays
	// in main to run as an assignment at its source position, preserving the order
	// JavaScript evaluates the module top level. Like hoistedVars, membership turns
	// the in-place `name = e` from a fresh declaration into a plain assignment. It is
	// set once for the whole program, so it is not scoped or restored the way the
	// per-body hoist sets are.
	moduleAssignVars map[string]bool
	// typeDepth counts the nesting typeExpr is currently rendering, so a
	// self-referential type (a function whose return type reaches back to itself, an
	// object with a property of its own shape) hands back at a bounded depth rather
	// than recursing until the goroutine stack overflows. It is incremented on entry
	// to typeExpr and decremented on exit, so it returns to zero between top-level
	// renders.
	typeDepth int
	// typeNodes counts how many type nodes the current top-level typeExpr render
	// has already produced. Depth alone does not bound memory: a self-referential
	// type that reaches back through a union or a multi-property object branches, so
	// the node count grows exponentially in the depth and exhausts RAM long before
	// the depth ceiling trips (a lower-only pass climbed past 6 GB in seconds on
	// one such type). The node budget caps the total work of a single render
	// regardless of how it branches, so the worst case stays a handback rather than
	// an out-of-memory kill that can take the whole machine down. It is reset when a
	// render starts at depth zero.
	typeNodes int
	// notAssignSpans holds the byte spans of the program's code 2345 diagnostics, the
	// argument-not-assignable errors the front door tolerates. The bridge consults it
	// so a value hands back only where the checker actually reported the mismatch, not
	// wherever two static types happen to lower to different Go types (a shaped literal
	// bound through a legitimate structural coercion reaches the same bridge with no
	// 2345 against it). It is built once, lazily, on the first bridge that asks.
	notAssignSpans []frontend.Span
	// assign2322Spans holds the byte spans of the program's code 2322 diagnostics, the
	// assignment- and initializer-not-assignable errors the front door tolerates. A 2322
	// anchors on the target name (an initializer, an assignment, a property declaration)
	// or on the value (an array element), not always on the value the way 2345 does, so
	// the binding bridge matches both the source and the target node against these spans.
	// It is built lazily alongside notAssignSpans.
	assign2322Spans []frontend.Span
	// overload2769Spans holds the byte spans of the program's code 2769 diagnostics, the
	// "No overload matches this call" errors the front door tolerates. A 2769 anchors on
	// the call expression, so the overloaded-call path matches the call node against these
	// spans to mark a checker-rejected call it lowered as seen. It is built lazily
	// alongside notAssignSpans.
	overload2769Spans []frontend.Span
	// arithOperandSpans holds the byte spans of the program's code 2362 and 2363
	// diagnostics, the "left-hand side" and "right-hand side of an arithmetic operation
	// must be of type number, bigint, or any" errors. A string or boolean operand to a
	// numeric operator (1 * `x`) is one JavaScript runs by coercing through ToNumber, so
	// the emit path lowers it to a running program, but TypeScript rejects it. No lowering
	// can make that program one TypeScript accepts, so the mere presence of such a span
	// hands the whole unit back rather than emit Go for a program the checker refuses. A
	// .js source carries no such diagnostic, so its coercion is left alone. It is built
	// lazily alongside notAssignSpans.
	arithOperandSpans []frontend.Span
	// notAssignReady marks notAssignSpans, assign2322Spans, overload2769Spans, and
	// arithOperandSpans as built, so the empty slices are not rebuilt on every bridge. It
	// stays false until the first lookup collects the 2345, 2322, 2769, 2362, and 2363
	// spans.
	notAssignReady bool
	// seenAssign records the not-assignable spans (2345 and 2322) a guarded bridge
	// inspected, the argument, constructor, and binding sites where the representation
	// guard either lowered a safe value or handed back. A span left unseen at the end of
	// a render was lowered by a path with no guard (a builtin higher-order method
	// callback, a builtin element-slot argument, an assignment construct no bridge
	// reaches), which would emit Go the toolchain rejects, so the whole unit hands back
	// rather than ship it (see unguardedNotAssign). It is keyed by span since a span is a
	// comparable pair of offsets.
	seenAssign map[frontend.Span]bool
	// closureDepth is how many function expressions or arrow functions the current
	// render is nested inside, incremented on entry to each and restored on exit. A
	// closure lowers to an inline Go func literal, and the Go compiler's escape
	// analysis over deeply nested func literals is super-linear in the nesting depth:
	// past roughly twenty levels a single go build grows from hundreds of megabytes to
	// gigabytes and is killed for want of memory. maxClosureNestDepth caps the depth so
	// a pathologically nested source (test262's 32-deep IIFE stress tests) hands back
	// instead of emitting Go that OOMs the toolchain. It is a guard on emit shape, not a
	// semantic limit, the same reason the typeExpr node budget caps a branching type.
	closureDepth int
	// pendingArrowRet carries a contextual return type down to the very next arrow the
	// renderer lowers, so a concise arrow assigned to a function slot whose return type
	// is wider than the body's own (a union of the return types of a union-of-call-
	// signatures target) returns at the slot's type, coercing the body into it. arrowFunc
	// reads and clears it on entry so it applies to exactly the one arrow it was set for
	// and never leaks into a nested arrow in that arrow's body. It is nil for every arrow
	// lowered without a contextual function slot, the whole existing set.
	pendingArrowRet *frontend.Type
}

// maxClosureNestDepth is the deepest run of nested function expressions or arrow
// functions the renderer will emit as inline Go func literals. Beyond it the Go
// compiler's memory over the nested closures blows up super-linearly (measured: depth
// 16 builds in ~70MB, 20 in ~310MB, 22 in ~1.2GB, 24 in ~4.3GB, 26+ is OOM-killed), so
// a deeper nest hands back rather than ship Go the toolchain cannot build. Real code
// never nests closures this deep; only a stress test does.
const maxClosureNestDepth = 16

// maxTypeDepth bounds how deep typeExpr renders a nested type before it hands
// back. A real annotation nests only a handful of levels, so the ceiling sits far
// above any genuine type and only ever trips on a self-referential one, which has
// no finite Go shape and must route to the interpreter instead of crashing.
const maxTypeDepth = 256

// maxTypeNodes bounds how many type nodes a single top-level typeExpr render may
// produce before it hands back. A branching self-referential type multiplies its
// node count at every level, so a depth cap alone lets it allocate gigabytes; the
// node budget stops that in bounded memory. A genuine annotation renders a few
// dozen nodes, so the ceiling sits far above any real type and only trips on a
// pathological one.
const maxTypeNodes = 20000

// NewRenderer builds a renderer over a checked program.
func NewRenderer(prog *frontend.Program) *Renderer {
	return &Renderer{prog: prog, decls: newDeclSet(), imports: map[string]bool{}, nodeImports: map[string]nodeBuiltin{}, goImports: map[string]goBuiltin{}, goNamespaces: map[string]string{}, internalNamespaces: map[string]bool{}, dynImportNamespaces: map[string]bool{}, goAliases: map[string]string{}, errorLocals: map[string]bool{}, funcExprSelf: map[frontend.Symbol]string{}, argsThreads: map[frontend.Symbol]bool{}, paramAliases: map[frontend.Symbol]string{}, arrowDropDefaults: map[frontend.Node]bool{}, arrowCallDefaults: map[frontend.Symbol][]frontend.Node{}, closurePadParams: map[frontend.Node][]frontend.Param{}, promiseSettleParams: map[string]promiseSettle{}, monoSpecs: map[frontend.Symbol][]monoSpec{}, monoMethodSpecs: map[frontend.Node][]monoSpec{}, bigLits: map[string]string{}, classes: map[string]*classInfo{}, enums: map[string]*enumInfo{}, unionBySig: map[string]*unionInfo{}, seenAssign: map[frontend.Span]bool{}}
}

// freshTemp returns a generated Go local name unique across the program, for a
// lowering that needs a variable with no source spelling. The name carries a prefix
// no mangled source identifier takes, so it cannot collide with a user binding, and
// the monotonic counter keeps two temporaries apart even in nested scopes.
func (r *Renderer) freshTemp() string {
	name := "_bt" + strconv.Itoa(r.tmpSeq)
	r.tmpSeq++
	return name
}

// SetGoSignatures wires the resolver a go: call marshals numbers against, so a Go
// int, int64, and float64 (all one TypeScript number) each cross the boundary with
// the right conversion and range check. The build sets it from the Go package load
// the declaration generator already ran; a renderer with no resolver lowers only
// the string and boolean crossings a TypeScript type settles on its own.
func (r *Renderer) SetGoSignatures(resolve func(importPath, name string) (goimport.FuncSig, bool)) {
	r.goSigs = resolve
}

// SetGoConstants wires the resolver a go: constant reference marshals against, the
// companion to SetGoSignatures for a binding used as a value rather than called. It
// is set from the same Go package load; a renderer with no resolver hands a
// reference to a go: binding back rather than guess whether it is a constant.
func (r *Renderer) SetGoConstants(resolve func(importPath, name string) (goimport.ConstInfo, bool)) {
	r.goConsts = resolve
}

// SetGoErrorVars wires the resolver a caught error's is() checks a go: sentinel
// against, so err.is(EOF) lowers to errors.Is against io.EOF. It is set from the
// same Go package load; a renderer with no resolver hands err.is back rather than
// guess whether a bound name is an error variable.
func (r *Renderer) SetGoErrorVars(resolve func(importPath, name string) bool) {
	r.goErrorVars = resolve
}

// requireImport records that the Go the renderer has emitted refers to a
// package, so a caller assembling a file adds the import. Lowering emits
// qualified names (math.Mod, and later value.*) as it goes, and the set of
// packages those names need is only known after the body is lowered, so it is
// gathered here rather than declared up front.
func (r *Renderer) requireImport(path string) {
	r.imports[path] = true
}

// Imports returns the import paths the emitted Go refers to, sorted so the
// output is deterministic. A caller writes one import per path into the file it
// assembles around the rendered declarations.
func (r *Renderer) Imports() []string {
	paths := make([]string, 0, len(r.imports))
	for p := range r.imports {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// GoImportPaths returns the Go import paths the rendered program reaches through a
// go: interop call, sorted, with no duplicates. It reads the alias map, which is
// populated only as a package is actually called into, so a go: import that is
// declared but never called is not listed. The build consults this to detect
// whether any reached package pulls in cgo before it runs the toolchain
// (document 16 section 9.5), the one caller that needs the interop paths apart
// from the import block importSpecs already assembles.
func (r *Renderer) GoImportPaths() []string {
	paths := make([]string, 0, len(r.goAliases))
	for p := range r.goAliases {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}

// RenderType returns the Go type expression that represents t, registering any
// named declarations it needs (a struct for an object shape) into the renderer.
// It returns a NotYetLowerable error for a construct whose slice has not landed,
// which is the section 30 handoff, never a silent wrong answer.
func (r *Renderer) RenderType(t frontend.Type) (string, error) {
	expr, err := r.typeExpr(t)
	if err != nil {
		return "", err
	}
	return printExpr(expr)
}

// typeExpr is the mapping table itself: one frontend.Type to the go/ast node for
// the Go type that represents it. It composes nodes rather than text, so an array
// wraps its already-built element node and a pointer wraps its pointee, and it
// returns a NotYetLowerable for a construct whose slice has not landed, which is
// the section 30 handoff, never a silent wrong answer. RenderType prints whatever
// it returns.
func (r *Renderer) typeExpr(t frontend.Type) (ast.Expr, error) {
	// The zero Type carries no flags. It stands for a position with no value,
	// such as a statement or a void return, and has no slot representation, so
	// asking for its type expression is a caller error, not a lowering gap.
	if t.Flags == 0 {
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "no type at this position (void or statement)"}
	}

	// Inside a monomorphized generic body a bare type parameter resolves to the
	// concrete type the call site fixed it to, so typeExpr renders the float64 or
	// value.BStr the specialization uses rather than reaching the type-parameter
	// handback below. The substitution is set only around a specialization (mono.go);
	// outside one it is nil and this is a no-op.
	if t.Flags&frontend.TypeTypeParameter != 0 && r.typeSubst != nil {
		if conc, ok := r.typeSubst[t.Identity()]; ok {
			return r.typeExpr(conc)
		}
	}

	// The polymorphic this type, how the checker types a `return this` and the
	// self-returning [Symbol.iterator]() of a class that is its own iterator, is a
	// type parameter whose symbol is the enclosing class. Inside that class's body
	// it renders as a pointer to the class's Go struct, the receiver's own type,
	// rather than reaching the bare type-parameter handback below.
	if t.Flags&frontend.TypeTypeParameter != 0 && r.curClass != nil {
		if sym, ok := r.prog.TypeSymbol(t); ok && sym.Flags&frontend.SymbolClass != 0 && sym.Name == r.curClass.name {
			return star(ident(r.curClass.goName)), nil
		}
	}

	// A self-referential type recurses through typeExpr without end (a function type
	// whose return renders another function of the same shape, an object with a
	// property of its own type), so a bounded depth converts that into a handback
	// rather than a stack overflow that would crash the worker and stall the run.
	r.typeDepth++
	defer func() { r.typeDepth-- }()
	if r.typeDepth == 1 {
		r.typeNodes = 0
	}
	r.typeNodes++
	if r.typeDepth > maxTypeDepth || r.typeNodes > maxTypeNodes {
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "a self-referential or excessively nested type is a later slice"}
	}

	// A union whose members all share a primitive facet (a numeric-literal union
	// like 1 | 2 | 3 is a number, true | false is a boolean, a closed string-literal
	// union "a" | "b" is a string) widens to that primitive's Go type, the same type
	// the predicates fold it to, so a function returning such a union gets a float64,
	// bool, or value.BStr result rather than routing to the tagged-sum machinery a
	// real object union needs. A closed string-literal union is a plain string at run
	// time, so value.BStr carries it and every string operation reads it directly. A
	// union that folds nothing (a mixed or optional union) keeps its own flags and
	// falls through to the union handling.
	if t.Flags&frontend.TypeUnion != 0 {
		if pf := r.primitiveFlagsOfType(t); pf != t.Flags {
			switch {
			case pf&frontend.TypeNumber != 0:
				return ident("float64"), nil
			case pf&frontend.TypeBoolean != 0:
				return ident("bool"), nil
			case pf&frontend.TypeString != 0:
				r.requireImport(valuePkg)
				return sel("value", "BStr"), nil
			}
		}
		// The IteratorResult union a generator's next/return/throw hand back lowers to
		// the value.IterResult struct, its { value, done } read as fields. It routes
		// before the general tagged-sum union path below, which declines this union
		// because its discriminant `done` is a boolean literal, not a string one.
		if r.isIteratorResult(t) {
			r.requireImport(valuePkg)
			return sel("value", "IterResult"), nil
		}
	}

	switch {
	case t.Flags&(frontend.TypeAny|frontend.TypeUnknown) != 0:
		// any and unknown have no static shape, so a value of one is self-describing
		// at runtime: it is the boxed value.Value the dynamic world uses (section 30,
		// the value model's boxing boundary). A dynamic value carries its own kind,
		// so the operations on it (a property read, a coercion, the + operator)
		// dispatch on that kind at runtime through the value package rather than on a
		// Go static type here.
		r.requireImport(valuePkg)
		return sel("value", "Value"), nil

	case t.Flags&frontend.TypeNumber != 0:
		// number is IEEE-754 double, so float64 (D14, section 3). Integer
		// refinement to int32/int64 is a later slice and only ever narrows the
		// representation, never the observable number type.
		return ident("float64"), nil

	case t.Flags&frontend.TypeBigInt != 0:
		// bigint is arbitrary precision, so *big.Int; no fixed-width Go integer
		// is correct (section 4).
		r.requireImport("math/big")
		return star(sel("big", "Int")), nil

	case t.Flags&frontend.TypeString != 0:
		// string is a sequence of UTF-16 code units, so the bento string type
		// value.BStr, never Go string, which would be UTF-8 (section 5). Doc 05
		// writes it as a bare bstr, but that is shorthand: the type lives in the
		// value package and is referenced by its real import path (section 2).
		r.requireImport(valuePkg)
		return sel("value", "BStr"), nil

	case t.Flags&frontend.TypeBoolean != 0:
		// The one clean mapping (section 6).
		return ident("bool"), nil

	case t.Flags&frontend.TypeSymbol != 0:
		// A symbol is a unique opaque value whose whole purpose is identity, so it
		// rides the boxed value.Value the dynamic world uses, its identity the
		// identity of the value's backing symbol. Boxing it uniformly is what lets a
		// symbol flow into a property bag as a computed key beside the string keys,
		// where GetElem and SetElem dispatch on the boxed kind (section 8).
		r.requireImport(valuePkg)
		return sel("value", "Value"), nil

	case t.Flags&frontend.TypeEnum != 0:
		// An enum-typed slot (a parameter, return, or annotated binding typed as
		// the enum) carries the enum flag beside a union of its members, which would
		// otherwise route to renderUnion. A registered numeric enum is float64-backed
		// and a string enum is value.BStr-backed, the same types their member reads
		// already resolve to, so the slot takes that primitive. An enum this file
		// declined to register (a heterogeneous or computed one) has no lowering here
		// and hands back.
		if info, ok := r.enumOfType(t); ok {
			if info.isString {
				r.requireImport(valuePkg)
				return sel("value", "BStr"), nil
			}
			return ident("float64"), nil
		}
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "enum lowering lands in a later slice"}

	case t.Flags&frontend.TypeObject != 0:
		// TypeObject covers both arrays and fixed-shape objects in the frontend
		// vocabulary. An element type means it is an array; otherwise it is an
		// object shape that lowers to a struct.
		if info, ok := r.classOfType(t); ok {
			// A class instance is the named struct the class lowered to, held by
			// pointer so methods mutate it and identity is preserved, and it routes
			// first so a class whose fields spell an array-like or Map-like shape is
			// never re-derived structurally.
			return star(ident(info.goName)), nil
		}
		if elems, ok := r.prog.TupleElements(t); ok {
			// A tuple is a fixed sequence of positional element types, not a struct
			// shape and not an array with a single element type, so it lowers to an
			// interned positional struct, Tuple_str_num{E0, E1} (typed/05 T7), held by
			// value the way an array position is rather than by pointer. It routes here
			// before renderObject, which would otherwise intern the tuple's inherited
			// array members as struct fields and miscompile the shape. An optional or
			// rest element, whose field this slice does not yet emit, hands back through
			// internTuple rather than emit a partial struct.
			name, err := r.decls.internTuple(r, t, elems)
			if err != nil {
				return nil, err
			}
			return ident(name), nil
		}
		// An ArrayIterator, the object arr.values(), arr.keys(), and arr.entries() hand
		// back, is the *value.ArrayIter the runtime walks. It routes before the generator
		// family and the structural object path, which would otherwise expand the
		// iterator's next() result into the IteratorResult union and hand back. This is
		// the slot a `const it = arr.values()` binding takes.
		if r.isArrayIteratorType(t) {
			r.requireImport(valuePkg)
			return star(sel("value", "ArrayIter")), nil
		}
		// A MapIterator or SetIterator, the object m.keys(), m.values(), s.keys(), and
		// s.values() hand back, is the *value.ArrayIter the runtime walks over the
		// receiver's insertion-ordered snapshot. Its keys() and values() are the only
		// sources of these types that lower: entries() hands back at construction, so any
		// surviving MapIterator or SetIterator is a single-value drive the ArrayIter slot
		// holds. It routes before the structural object path, which would otherwise expand
		// the iterator's next() result into the IteratorResult union and hand back. This is
		// the slot a `const it = m.values()` binding takes.
		if r.isCollIteratorType(t) {
			r.requireImport(valuePkg)
			return star(sel("value", "ArrayIter")), nil
		}
		// An IteratorObject, the result of Iterator.from and of every lazy helper
		// (map, filter, take, drop, flatMap), is the *value.IterHelper the runtime
		// pulls. It routes before the generator family, which shares the wider iterator
		// type names but drives a *value.Gen[Y] whose Next takes a sent value the
		// helper's no-argument Next does not, so it must be recognized first. This is
		// the slot a `const m = it.map(f)` binding takes.
		if r.isIterHelperType(t) {
			r.requireImport(valuePkg)
			return star(sel("value", "IterHelper")), nil
		}
		// A Generator (or the wider iterator family) is the *value.Gen[Y] coroutine the
		// runtime drives, its yielded element type read off the generic's first type
		// argument. It routes before renderFuncType and the structural array and object
		// paths, which would otherwise expand the generator's next() result into the
		// IteratorResult union and hand back. This is the return type a generator
		// function value spells, the slot a `const it = g()` binding takes.
		if elem, ok := r.generatorElemType(t); ok {
			inner, err := r.typeExpr(elem)
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return star(index(sel("value", "Gen"), inner)), nil
		}
		// An AsyncGenerator (or the wider async iterator family) is the *value.AsyncGen[Y]
		// coroutine the runtime drives, the async mirror of the Generator slot above: it is
		// the return type an async generator function value spells and the slot a
		// `const g = ag()` binding takes, routed before the structural paths that would
		// expand its next() promise into a Promise of the IteratorResult union and hand back.
		if elem, ok := r.asyncGeneratorElemType(t); ok {
			inner, err := r.typeExpr(elem)
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return star(index(sel("value", "AsyncGen"), inner)), nil
		}
		if ft, ok, err := r.renderFuncType(t); err != nil {
			return nil, err
		} else if ok {
			// A function type (an arrow or function value, the type of a callback
			// parameter) lowers to a Go func type. It routes before renderObject, which
			// would otherwise intern its call signature away and leave an empty struct.
			return ft, nil
		}
		if elem, ok := r.prog.ElementType(t); ok {
			return r.renderArray(elem)
		}
		if r.isGoOpaqueType(t) {
			// A go: opaque handle (section 6.13) projects to a GoOpaque object, but it is a
			// token bento never reads, not a shape: a local that holds one is typed as the
			// uniform bridge.Opaque the crossing produces, not a struct of its phantom brand.
			r.requireImport(bridgePkg)
			return sel("bridge", "Opaque"), nil
		}
		if r.arrayBufferType(t) {
			// An ArrayBuffer (section 6.2) is the value model's raw byte backing store,
			// spelled as a pointer to value.ArrayBuffer. A typed array or a DataView views
			// it, so it is a distinct type from any view. It is not a struct shape, so it
			// routes here before renderObject would intern its interface as fields.
			r.requireImport(valuePkg)
			return star(sel("value", "ArrayBuffer")), nil
		}
		if r.sharedArrayBufferType(t) {
			// A SharedArrayBuffer (25 §25.2) is the value model's shared backing store,
			// spelled as a pointer to value.SharedArrayBuffer. It carries the same bytes an
			// ArrayBuffer does but a distinct grow-and-slice surface, so it is its own type. It
			// is not a struct shape, so it routes here before renderObject would intern its
			// interface as fields.
			r.requireImport(valuePkg)
			return star(sel("value", "SharedArrayBuffer")), nil
		}
		if r.dataViewType(t) {
			// A DataView (section 25.3) is the value model's arbitrary-offset view over an
			// ArrayBuffer, spelled as a pointer to value.DataView. Like the buffer it is not a
			// struct shape, so it routes here before renderObject would intern its getInt8
			// family as fields.
			r.requireImport(valuePkg)
			return star(sel("value", "DataView")), nil
		}
		if name, ok := r.typedArrayName(t); ok {
			// A typed array (section 6.3) is the value model's fixed-width numeric buffer,
			// spelled as a pointer the same way a typed Array is. Uint8Array carries its own
			// []byte representation for the go: boundary (section 7.3); the rest of the
			// numeric family share the generic value.TypedArray[T]. It is not a struct shape,
			// so it routes here before renderObject would intern its interface as fields.
			return r.renderTypedArray(name)
		}
		if r.isWeakMapType(t) {
			// A WeakMap<K, V> (25 §24.3) is the value model's weakly keyed collection,
			// spelled as a pointer to the generic value.WeakMap header. Its fingerprint has
			// no size, so it routes before the Map check, whose fingerprint requires size,
			// and before renderObject would intern its method interface as fields.
			return r.renderWeakMap(t)
		}
		if r.isMapType(t) {
			// A Map<K, V> (section 6.5) is the value model's keyed collection, spelled as a
			// pointer to the generic value.Map header, and the type a Go map[K]V projects to
			// across the boundary. It is not a struct shape, so it routes here before
			// renderObject would intern its method interface as fields.
			return r.renderMap(t)
		}
		if r.isWeakSetType(t) {
			// A WeakSet<E> (25 §24.4) is the value model's weakly held collection of
			// objects, spelled as a pointer to the generic value.WeakSet header. Its
			// fingerprint has no size, so it routes before the Set check, whose fingerprint
			// requires size, and before renderObject would intern its method interface.
			return r.renderWeakSet(t)
		}
		if r.isSetType(t) {
			// A Set<T> (section 6.5) is the value model's collection of unique members,
			// spelled as a pointer to the generic value.Set header. Like Map it is not a
			// struct shape, so it routes here before renderObject would intern its method
			// interface as fields. Its fingerprint is disjoint from a Map's, so the order
			// against the Map check above does not matter.
			return r.renderSet(t)
		}
		if r.isPromiseType(t) {
			// A Promise<T> (the async slice) is the value model's settled promise,
			// spelled as a pointer to the generic value.Promise header, the result type
			// an async method's Go signature carries. Like Map and Set it is not a struct
			// shape, so it routes here before renderObject would intern its then/catch
			// interface as fields.
			return r.renderPromise(t)
		}
		if r.isWeakRefType(t) {
			// A WeakRef<T> (25 §26.1) is the value model's single weak reference, spelled as
			// a pointer to the generic value.WeakRef header. It is not a struct shape, so it
			// routes here before renderObject would intern its deref interface as a field.
			return r.renderWeakRef(t)
		}
		if r.isFinalizationRegistryType(t) {
			// A FinalizationRegistry<T> (25 §26.2) is the value model's post-collection
			// callback registry, spelled as a pointer to the generic value.FinalizationRegistry
			// header. It is not a struct shape, so it routes before renderObject would intern
			// its register interface as fields.
			return r.renderFinalizationRegistry(t)
		}
		if r.regExpType(t) {
			// A RegExp (22 §22.2) is the value model's compiled pattern, spelled as a
			// pointer to value.RegExp. It is not a struct shape, so it routes here before
			// renderObject would intern its exec and test interface as fields.
			r.requireImport(valuePkg)
			return star(sel("value", "RegExp")), nil
		}
		if r.plainDateType(t) {
			// A Temporal.PlainDate (Temporal §3) is the value model's ISO calendar date,
			// spelled as a pointer to value.PlainDate. It is not a struct shape, so it
			// routes here before renderObject would intern its getter interface as fields.
			r.requireImport(valuePkg)
			return star(sel("value", "PlainDate")), nil
		}
		if r.plainTimeType(t) {
			// A Temporal.PlainTime (Temporal §4) is the value model's wall-clock time,
			// spelled as a pointer to value.PlainTime, routed here for the same reason as
			// PlainDate: it is a host type, not a struct shape.
			r.requireImport(valuePkg)
			return star(sel("value", "PlainTime")), nil
		}
		if r.plainDateTimeType(t) {
			// A Temporal.PlainDateTime (Temporal §5) is the value model's date paired with
			// a wall-clock time, spelled as a pointer to value.PlainDateTime, routed here for
			// the same reason as PlainDate and PlainTime: it is a host type, not a struct shape.
			r.requireImport(valuePkg)
			return star(sel("value", "PlainDateTime")), nil
		}
		if r.durationType(t) {
			// A Temporal.Duration (Temporal §7) is the value model's span of time, spelled as
			// a pointer to value.Duration, routed here for the same reason as the plain types:
			// it is a host type, not a struct shape.
			r.requireImport(valuePkg)
			return star(sel("value", "Duration")), nil
		}
		if r.plainYearMonthType(t) {
			// A Temporal.PlainYearMonth (Temporal §9) is the value model's year-and-month,
			// spelled as a pointer to value.PlainYearMonth, routed here for the same reason as
			// the other plain types: it is a host type, not a struct shape.
			r.requireImport(valuePkg)
			return star(sel("value", "PlainYearMonth")), nil
		}
		if r.plainMonthDayType(t) {
			// A Temporal.PlainMonthDay (Temporal §10) is the value model's month-and-day,
			// spelled as a pointer to value.PlainMonthDay, routed here for the same reason as
			// the other plain types: it is a host type, not a struct shape.
			r.requireImport(valuePkg)
			return star(sel("value", "PlainMonthDay")), nil
		}
		if r.instantType(t) {
			// A Temporal.Instant (Temporal §8) is the value model's exact time as a nanosecond
			// count, spelled as a pointer to value.Instant, routed here for the same reason as
			// the other Temporal types: it is a host type, not a struct shape.
			r.requireImport(valuePkg)
			return star(sel("value", "Instant")), nil
		}
		if r.zonedDateTimeType(t) {
			// A Temporal.ZonedDateTime (Temporal §7) is the value model's exact time paired with
			// a zone, spelled as a pointer to value.ZonedDateTime, routed here for the same reason
			// as the other Temporal types: it is a host type, not a struct shape.
			r.requireImport(valuePkg)
			return star(sel("value", "ZonedDateTime")), nil
		}
		if r.isStringIndexDict(t) {
			// A pure string-index dictionary, { [x: string]: string } with no declared
			// members, is a bag keyed by arbitrary strings, not a fixed shape. Interning
			// it to a struct would drop the signature and leave an empty struct no other
			// object assigns to, so it lowers to the dynamic value.Value the property bag
			// uses: any object flows into it by boxing, and a keyed read or write
			// dispatches on the boxed kind at runtime. A shape that carries declared
			// members beside its index signature keeps the struct below, where its known
			// fields still lower.
			r.requireImport(valuePkg)
			return sel("value", "Value"), nil
		}
		if r.isEmptyObjectTopType(t) {
			// The empty object type { } carries no declared members and no index
			// signature, so it is not a shape: structurally it is the top type that
			// accepts any non-null value, and a user type guard narrows it to a shape
			// the empty struct could never hold. Interning it to an empty struct drops
			// the value and leaves narrowed member reads dangling, so it lowers to the
			// dynamic value.Value box, where the value survives narrowing and a keyed
			// read dispatches on the boxed kind at runtime.
			r.requireImport(valuePkg)
			return sel("value", "Value"), nil
		}
		return r.renderObject(t)

	case t.Flags&frontend.TypeUnion != 0:
		return r.renderUnion(t)

	case t.Flags&frontend.TypeLiteral != 0:
		// A lone literal type (a bare "circle" or 42 outside a union) lowers to
		// its widened base and so is caught by the primitive cases above, which
		// run first because a string literal also carries TypeString. Reaching
		// here means a literal with no base flag bento renders yet.
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "literal type with no lowerable base"}

	case t.Flags&frontend.TypeIntersection != 0:
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "intersection lowering (merged struct) lands in a later slice"}

	case t.Flags&frontend.TypeTypeParameter != 0:
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "type parameter needs monomorphization, a later slice"}

	case t.Flags&(frontend.TypeUndefined|frontend.TypeNull) != 0:
		// A slot typed exactly undefined or null holds only that one sentinel, and
		// bento already lowers both literals to the value.Undefined and value.Null
		// singletons, which are value.Value. So the slot's Go type is value.Value,
		// the boxed representation the singleton fits, agreeing with the literal
		// lowering wherever the value flows in: a struct field, a tuple or array
		// element, or a function-type parameter typed null or undefined. This runs
		// after the union case, so a T | undefined union still routes to renderUnion;
		// only a bare undefined or null reaches here.
		r.requireImport(valuePkg)
		return sel("value", "Value"), nil

	case t.Flags&frontend.TypeVoid != 0:
		// A slot typed void carries no meaningful value, and a void expression
		// evaluates to undefined at run time, so it takes the same boxed value.Value
		// the undefined sentinel does. A void return is handled at the signature
		// level, which drops it before it reaches here; this covers a void in a value
		// position, such as a field or element typed void.
		r.requireImport(valuePkg)
		return sel("value", "Value"), nil

	case t.Flags&frontend.TypeNever != 0:
		// never is uninhabited, so no value ever reaches a never-typed slot at run
		// time, but a Go declaration still needs a type for it: a never[] element
		// (the type of an empty array literal), a field, or a function-type parameter
		// no caller can supply. value.Value is the boxed placeholder that renders
		// each of these without inventing a representation a real value would need.
		r.requireImport(valuePkg)
		return sel("value", "Value"), nil

	default:
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "no lowering for this type"}
	}
}

// renderFuncType lowers a function type to a Go func type, so a callback
// parameter or a local holding an arrow reads with a callable Go type rather
// than the empty struct renderObject would give a type it sees as a shape with
// no fields. It reports ok=false when the type is not a plain function value, so
// typeExpr falls through to its object handling: a callable object that also
// carries properties, an overloaded type with more than one call signature, a
// constructor type, or a generic or rest-or-optional-parameter signature are
// each their own later slice. Once the type is a plain function value, a
// parameter or return that has no lowering hands the unit back rather than
// falling through to a wrong shape.
func (r *Renderer) renderFuncType(t frontend.Type) (ast.Expr, bool, error) {
	call, construct := r.prog.Signatures(t)
	if len(call) == 0 {
		// Not a function value (a plain object, an array, or a constructor-only
		// type); let typeExpr's object handling take it.
		return nil, false, nil
	}
	// The type is callable, so it is a function value and must not fall through to
	// the empty-struct object lowering, which would be unsound. An overload set
	// (more than one call signature) or a construct signature is still a later
	// slice; the reason names what actually remains now that properties lower.
	if len(call) != 1 || len(construct) != 0 {
		return nil, true, &NotYetLowerable{Flags: t.Flags, Reason: "overloaded or constructor function type is a later slice"}
	}
	// A callable object carries its own properties on top of the call signature, so
	// it is not a bare Go func: it lowers to a named struct whose fields are the
	// properties plus one reserved Call field for the call itself, the same struct
	// internStruct mints for a fixed-shape object. It routes here, before the plain
	// func type below, so its properties are not interned away.
	if len(r.prog.Properties(t)) != 0 {
		obj, err := r.renderObject(t)
		if err != nil {
			return nil, true, err
		}
		return obj, true, nil
	}
	ft, err := r.funcTypeOf(call[0])
	if err != nil {
		return nil, true, err
	}
	return ft, true, nil
}

// funcTypeOf builds the Go func type for one call signature: a field per
// parameter and, unless the return is void or never, a single result. A dynamic
// optional parameter lowers to its plain type, since undefined lives inside the
// value box natively and the call site fills an omitted argument with
// value.Undefined, matching how paramFields lowers the function body parameter;
// a static optional has no such room and hands back, as does a generic
// signature. A rest parameter lowers to a trailing *value.Array[T] field, the
// same array header a rest parameter of a function body takes, so it is a plain
// (non-variadic) Go func value. It is shared by renderFuncType's plain function
// value and the Call field of a callable-object struct.
func (r *Renderer) funcTypeOf(sig frontend.Signature) (*ast.FuncType, error) {
	if len(sig.TypeParams) != 0 {
		return nil, &NotYetLowerable{Reason: "generic function type is a later slice"}
	}
	var params []*ast.Field
	for _, p := range sig.Params {
		if p.Optional && p.Type.Flags&(frontend.TypeAny|frontend.TypeUnknown) == 0 {
			return nil, &NotYetLowerable{Reason: "function type with a static optional parameter is a later slice"}
		}
		pt, err := r.typeExpr(p.Type)
		if err != nil {
			return nil, err
		}
		params = append(params, &ast.Field{Type: pt})
	}
	// A rest parameter gathers the trailing arguments into the same *value.Array[T] a
	// rest parameter of a function body takes (restParamField), so a callback typed
	// with `...args: T[]` reads as one array field here and the call site packs the
	// trailing arguments into an array, exactly as a call of a rest-parameter function
	// does. The Go func value is not variadic: bento models rest through the array
	// header, not Go's `...`, so a func value and the function body agree on the shape.
	if sig.RestParam != nil {
		rt, err := r.typeExpr(sig.RestParam.Type)
		if err != nil {
			return nil, err
		}
		params = append(params, &ast.Field{Type: rt})
	}
	ft := &ast.FuncType{Params: &ast.FieldList{List: params}}
	if sig.Return.Flags != 0 && sig.Return.Flags&(frontend.TypeVoid|frontend.TypeNever) == 0 {
		rt, err := r.typeExpr(sig.Return)
		if err != nil {
			return nil, err
		}
		ft.Results = &ast.FieldList{List: []*ast.Field{{Type: rt}}}
	}
	return ft, nil
}

// isCallableObject reports whether a type is a callable object: a fixed-shape
// object that carries exactly one call signature alongside one or more
// properties. It is the shape the test262 assert prelude spells, and it lowers
// to a struct with a reserved Call field rather than to a bare Go func or a plain
// struct.
func (r *Renderer) isCallableObject(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	call, _ := r.prog.Signatures(t)
	return len(call) == 1 && len(r.prog.Properties(t)) > 0
}

// renderArray lowers an array type. The default is the Array[T] header from the
// value model (section 11), which carries the JavaScript array semantics a bare
// Go slice lacks: an assignable .length, holes, and the sparse-array edges. The
// bare []T fast path is an optimization the partitioner unlocks only when it has
// proven the array dense and in-range, which is a later slice, so the correct
// default here is the header.
func (r *Renderer) renderArray(elem frontend.Type) (ast.Expr, error) {
	inner, err := r.typeExpr(elem)
	if err != nil {
		return nil, err
	}
	return star(index(sel("value", "Array"), inner)), nil
}

// renderMap lowers a Map<K, V> type to a pointer to the generic value.Map header
// (section 6.5). It reads the key and value types off the map's set signature and
// renders each through typeExpr, so a Map<string, number> becomes a
// *value.Map[value.BStr, float64] and the element types carry whatever their own
// lowering is. A key or value type that has no lowering yet hands back through the
// same NotYetLowerable typeExpr returns for it.
func (r *Renderer) renderMap(t frontend.Type) (ast.Expr, error) {
	k, v, ok := r.mapKeyVal(t)
	if !ok {
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "Map type did not expose its key and value through a set signature"}
	}
	kExpr, err := r.typeExpr(k)
	if err != nil {
		return nil, err
	}
	vExpr, err := r.typeExpr(v)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return star(&ast.IndexListExpr{X: sel("value", "Map"), Indices: []ast.Expr{kExpr, vExpr}}), nil
}

// renderObject lowers a fixed-shape object type to a pointer to a generated Go
// struct (section 12). Objects are pointers because JavaScript objects have
// reference identity and === is reference equality, which Go pointer identity
// gives exactly. The struct itself is registered in the decl set, interned by
// the type's structural identity so the same shape yields the same Go type.
// isStringIndexDict reports whether a type is a pure string-index dictionary, an
// object with a string index signature and no declared members, the shape typeExpr
// lowers to a dynamic value.Value rather than a fixed struct. The no-declared-members
// gate keeps every other object out: an array, a tuple, a Map, a class, and a
// record with named fields all carry properties, so only a bare { [x: string]: T }
// matches, and the empty object { } (no index signature) does not. isDynamic and
// typeExpr both consult it so the "is a boxed value.Value" decision and the Go type
// they emit for the slot never disagree.
func (r *Renderer) isStringIndexDict(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if len(r.prog.Properties(t)) != 0 {
		return false
	}
	_, ok := r.prog.StringIndexType(t)
	return ok
}

// isEmptyObjectTopType reports whether a type is the empty object type { }, an object
// with no declared members, no string index signature, and no call signature. This is
// the structural top type that accepts any non-null value, not a fixed shape, so typeExpr
// lowers it to a dynamic value.Value rather than an empty interned struct. It excludes a
// string-index dictionary (which isStringIndexDict already routes to value.Value), and it
// excludes an array, a tuple, and a callable, which have their own lowerings. isDynamic
// and typeExpr both consult it so the "is a boxed value.Value" decision and the emitted Go
// type for the slot never disagree.
func (r *Renderer) isEmptyObjectTopType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if len(r.prog.Properties(t)) != 0 {
		return false
	}
	if _, ok := r.prog.StringIndexType(t); ok {
		return false
	}
	if _, ok := r.classOfType(t); ok {
		return false
	}
	if _, ok := r.prog.TupleElements(t); ok {
		return false
	}
	if _, ok := r.prog.ElementType(t); ok {
		return false
	}
	// A construct-signature type { new(): T } carries no declared property either, but
	// it is a constructor value, not the { } top type, so a call or construct signature
	// of any kind keeps it off the dynamic-box path and on its own lowering.
	if calls, constructs := r.prog.Signatures(t); len(calls) > 0 || len(constructs) > 0 {
		return false
	}
	return true
}

// isNarrowableBoxType reports whether a type lowers to the dynamic value.Value box and
// so survives a user type guard narrowing it to a shape at a use site: the empty object
// top type { }, a string-index dictionary, or an optional over either ({ } | undefined
// being the motivating case). It deliberately excludes bare any and unknown, whose
// narrowing already has its own established lowering, keeping this path to the structural
// box types the { } work introduced. isDynamic consults it on a receiver's declared type
// so a narrowed member read still dispatches through the box the Go variable holds.
func (r *Renderer) isNarrowableBoxType(t frontend.Type) bool {
	if r.isEmptyObjectTopType(t) || r.isStringIndexDict(t) {
		return true
	}
	if t.Flags&frontend.TypeUnion != 0 {
		if inner, ok := r.optionalInner(r.prog.UnionMembers(t)); ok {
			return r.isNarrowableBoxType(inner)
		}
	}
	return false
}

func (r *Renderer) renderObject(t frontend.Type) (ast.Expr, error) {
	name, err := r.decls.internStruct(r, t)
	if err != nil {
		return nil, err
	}
	return star(ident(name)), nil
}

// isFixedObjectShape reports whether a type's Go representation is a plain interned
// object struct, the { x: string } a binding or a return holds by pointer, so a value
// of it can be boxed into a value.Object by copying its fields. It requires the type to
// carry declared members and to lower to a bare pointer-to-struct, and it excludes a
// class instance (also a pointer to a struct, but with identity and methods a field
// copy would drop), a tuple, an array, and a callable, whose own paths lower them to
// something other than an object struct. The special runtime object types (Map, Set,
// Promise, a typed array) lower to a value.X pointer, a selector rather than a bare
// identifier, so the final shape check rules them out too.
func (r *Renderer) isFixedObjectShape(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if len(r.prog.Properties(t)) == 0 {
		return false
	}
	if _, ok := r.classOfType(t); ok {
		return false
	}
	if _, ok := r.prog.TupleElements(t); ok {
		return false
	}
	if _, ok := r.prog.ElementType(t); ok {
		return false
	}
	if calls, _ := r.prog.Signatures(t); len(calls) > 0 {
		return false
	}
	ge, err := r.typeExpr(t)
	if err != nil {
		return false
	}
	st, ok := ge.(*ast.StarExpr)
	if !ok {
		return false
	}
	_, ok = st.X.(*ast.Ident)
	return ok
}

// isGoOpaqueType reports whether an object type is a go: opaque handle, the
// GoOpaque<Tag> the bridge projects for a Go type it does not express (section
// 6.13). GoOpaque is { readonly __goOpaque: Tag }, so the phantom brand property is
// its fingerprint: a real object shape never carries it. The check is by property
// name alone, so it matches whatever the tag is without reading it.
func (r *Renderer) isGoOpaqueType(t frontend.Type) bool {
	for _, p := range r.prog.Properties(t) {
		if p.Name == "__goOpaque" {
			return true
		}
	}
	return false
}

// typedArrayName reports whether an object type is a member of the typed-array
// family and returns its name (Uint8Array, Int32Array, Float64Array, and the
// rest). The standard library types the family with a BYTES_PER_ELEMENT constant
// no plain object or the generic Array carries, so that property is the
// fingerprint, and the type's own symbol names which member it is, so Int8Array is
// told from Float64Array by name rather than left ambiguous. A caller must have
// already ruled the type out as an array (its ElementType is not an array
// element), so this runs only for the non-array object shapes.
// arrayBufferType reports whether an object type is a JavaScript ArrayBuffer, the
// raw byte backing store bento maps to value.ArrayBuffer (section 6.2). An
// ArrayBuffer carries no BYTES_PER_ELEMENT, so typedArrayName excludes it; it is
// named by its type symbol the same way a typed array's element name is read. A
// SharedArrayBuffer is a distinct symbol name and does not match, so it stays a
// later slice. A caller reaching this has already ruled the type out as an array.
func (r *Renderer) arrayBufferType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "ArrayBuffer"
}

// isArrayBuffer reports whether a node's type is an ArrayBuffer, the receiver test
// the .byteLength read shares with the constructor lowering.
func (r *Renderer) isArrayBuffer(n frontend.Node) bool {
	return r.arrayBufferType(r.prog.TypeAt(n))
}

// sharedArrayBufferType reports whether an object type is a JavaScript
// SharedArrayBuffer, the shared backing store bento maps to value.SharedArrayBuffer
// (25 §25.2). Like ArrayBuffer it carries no BYTES_PER_ELEMENT and is named by its
// type symbol; the distinct symbol name is what tells it from an ArrayBuffer, which
// otherwise shares the byteLength and slice surface. A caller reaching this has
// already ruled the type out as an array.
func (r *Renderer) sharedArrayBufferType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "SharedArrayBuffer"
}

// isSharedArrayBuffer reports whether a node's type is a SharedArrayBuffer, the
// receiver test the byteLength, grow, and slice lowerings share with the constructor
// lowering.
func (r *Renderer) isSharedArrayBuffer(n frontend.Node) bool {
	return r.sharedArrayBufferType(r.prog.TypeAt(n))
}

// dataViewType reports whether a type is a DataView, the arbitrary-offset view over
// an ArrayBuffer (25 §25.3). Like ArrayBuffer it is named by its type symbol: a
// DataView carries no BYTES_PER_ELEMENT, so it is not a typed array, and its getInt8
// family of methods separates it from every other view. A caller reaching this has
// already ruled the type out as an array.
func (r *Renderer) dataViewType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "DataView"
}

// isDataView reports whether a node's type is a DataView, the receiver test the
// getter, setter, and geometry-read lowerings share with the constructor lowering.
func (r *Renderer) isDataView(n frontend.Node) bool {
	return r.dataViewType(r.prog.TypeAt(n))
}

func (r *Renderer) typedArrayName(t frontend.Type) (string, bool) {
	if t.Flags&frontend.TypeObject == 0 {
		return "", false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return "", false
	}
	hasBytesPerElement := false
	for _, p := range r.prog.Properties(t) {
		if p.Name == "BYTES_PER_ELEMENT" {
			hasBytesPerElement = true
			break
		}
	}
	if !hasBytesPerElement {
		return "", false
	}
	sym, ok := r.prog.TypeSymbol(t)
	if !ok {
		return "", false
	}
	return sym.Name, true
}

// renderTypedArray maps a typed-array name to its Go value type: Uint8Array to the
// *value.Uint8Array byte buffer, each numeric-family member to a
// *value.TypedArray[T] over the element's Go type, and the bigint pair
// (BigInt64Array, BigUint64Array) to a *value.BigIntArray[T] over int64 or uint64.
func (r *Renderer) renderTypedArray(name string) (ast.Expr, error) {
	r.requireImport(valuePkg)
	if name == "Uint8Array" {
		return star(sel("value", "Uint8Array")), nil
	}
	if elem, ok := bigintTypedArrayElemGo(name); ok {
		return star(index(sel("value", "BigIntArray"), ident(elem))), nil
	}
	elem, ok := typedArrayElemGo(name)
	if !ok {
		return nil, &NotYetLowerable{Reason: "the " + name + " typed array is a later slice"}
	}
	return star(index(sel("value", "TypedArray"), ident(elem))), nil
}

// typedArrayElemGo maps a numeric typed-array name to the Go element type of its
// value.TypedArray representation, and ok=false for a name outside that family:
// Uint8Array has its own []byte representation, and the bigint-element arrays take
// bigintTypedArrayElemGo and a distinct value.BigIntArray. Uint8ClampedArray stores
// a uint8 like Uint8Array but through the generic buffer with the clamp coercion, so
// the two are distinct Go types.
func typedArrayElemGo(name string) (string, bool) {
	switch name {
	case "Int8Array":
		return "int8", true
	case "Uint8ClampedArray":
		return "uint8", true
	case "Int16Array":
		return "int16", true
	case "Uint16Array":
		return "uint16", true
	case "Int32Array":
		return "int32", true
	case "Uint32Array":
		return "uint32", true
	case "Float32Array":
		return "float32", true
	case "Float64Array":
		return "float64", true
	default:
		return "", false
	}
}

// bigintTypedArrayElemGo maps a bigint typed-array name to the Go element type of
// its value.BigIntArray representation, int64 for BigInt64Array and uint64 for
// BigUint64Array, and ok=false for any other name. The bigint pair reads and writes
// through *big.Int rather than the float64 the numeric family rides, so it is a
// distinct Go type with its own element mapping.
func bigintTypedArrayElemGo(name string) (string, bool) {
	switch name {
	case "BigInt64Array":
		return "int64", true
	case "BigUint64Array":
		return "uint64", true
	default:
		return "", false
	}
}

// bigintTypedArray reports whether a node's type is one of the bigint typed arrays,
// the receiver test the bigint index read, index write, geometry, and length
// lowerings share. It is the bigint counterpart of numericTypedArray, kept separate
// because a bigint element is a *big.Int, not the Number the numeric buffers hand
// out.
func (r *Renderer) bigintTypedArray(n frontend.Node) bool {
	name, ok := r.typedArrayName(r.prog.TypeAt(n))
	if !ok {
		return false
	}
	_, ok = bigintTypedArrayElemGo(name)
	return ok
}

// bytesPerElement maps a typed-array name to its element width in bytes, the
// BYTES_PER_ELEMENT constant, and ok=false for any other name. It spans the whole
// family, the bigint pair included, because the constant is a pure property of the
// element kind and reads the same whether or not that member's construction lowers.
func bytesPerElement(name string) (int, bool) {
	switch name {
	case "Int8Array", "Uint8Array", "Uint8ClampedArray":
		return 1, true
	case "Int16Array", "Uint16Array":
		return 2, true
	case "Int32Array", "Uint32Array", "Float32Array":
		return 4, true
	case "Float64Array", "BigInt64Array", "BigUint64Array":
		return 8, true
	default:
		return 0, false
	}
}

// numericTypedArray reports whether a node's type is a typed array bento lowers to
// an indexable numeric buffer, Uint8Array or a numeric-family member, the receiver
// test the index read, index write, and .length lowerings share. A bigint-element
// array is excluded: its elements are not Numbers, so it does not index through the
// float64 At and SetAt the numeric buffers share.
func (r *Renderer) numericTypedArray(n frontend.Node) bool {
	name, ok := r.typedArrayName(r.prog.TypeAt(n))
	if !ok {
		return false
	}
	if name == "Uint8Array" {
		return true
	}
	_, ok = typedArrayElemGo(name)
	return ok
}

// isTypedArray reports whether a node's type is any typed-array family member, the
// exclusion test the struct-field read and write paths use to keep a typed array,
// which is also a TypeObject, from being mistaken for a fixed-shape struct.
func (r *Renderer) isTypedArray(n frontend.Node) bool {
	_, ok := r.typedArrayName(r.prog.TypeAt(n))
	return ok
}

// isMapType reports whether an object type is a JavaScript Map, the keyed
// collection bento maps to value.Map (section 6.5). The standard library types a
// Map with get, set, has, and size together: get and set separate it from a Set
// (which has add, not get) and an Array, size separates it from a WeakMap (which
// has no size), so the four names together are the fingerprint, read the same way
// typedArrayName reads BYTES_PER_ELEMENT. A caller must have already ruled the type
// out as an array, so this runs only for the non-array object shapes.
func (r *Renderer) isMapType(t frontend.Type) bool {
	var hasGet, hasSet, hasHas, hasSize bool
	for _, p := range r.prog.Properties(t) {
		switch p.Name {
		case "get":
			hasGet = true
		case "set":
			hasSet = true
		case "has":
			hasHas = true
		case "size":
			hasSize = true
		}
	}
	return hasGet && hasSet && hasHas && hasSize
}

// isMap reports whether the checker types a node as a Map, the receiver test the
// map lowerings share (a new expression's target, a .get or .set call, a .size
// read). It is the node-level companion to isMapType: it reads the node's type and
// applies the same fingerprint, first ruling out an array so an array is never
// mistaken for a map.
func (r *Renderer) isMap(n frontend.Node) bool {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	return r.isMapType(t)
}

// mapKeyVal returns the key and value types of a Map type, read off its set method
// whose signature is set(key: K, value: V): this. The standard library declares set
// on every Map, so its first two parameters carry K and V exactly, which is more
// direct than reconstructing them from get's V | undefined result. It reports false
// for a type with no such set signature, which a non-map object is.
func (r *Renderer) mapKeyVal(t frontend.Type) (key, val frontend.Type, ok bool) {
	var setType frontend.Type
	found := false
	for _, p := range r.prog.Properties(t) {
		if p.Name == "set" {
			setType, found = p.Type, true
			break
		}
	}
	if !found {
		return frontend.Type{}, frontend.Type{}, false
	}
	call, _ := r.prog.Signatures(setType)
	if len(call) == 0 || len(call[0].Params) < 2 {
		return frontend.Type{}, frontend.Type{}, false
	}
	return call[0].Params[0].Type, call[0].Params[1].Type, true
}

// renderSet lowers a Set<T> type to a pointer to the generic value.Set header
// (section 6.5). It reads the member type off the set's add signature and renders
// it through typeExpr, so a Set<string> becomes a *value.Set[value.BStr] and the
// member type carries whatever its own lowering is. A member type that has no
// lowering yet hands back through the same NotYetLowerable typeExpr returns for it.
func (r *Renderer) renderSet(t frontend.Type) (ast.Expr, error) {
	elem, ok := r.setElem(t)
	if !ok {
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "Set type did not expose its member through an add signature"}
	}
	eExpr, err := r.typeExpr(elem)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return star(index(sel("value", "Set"), eExpr)), nil
}

// isSetType reports whether an object type is a JavaScript Set, the collection of
// unique members bento maps to value.Set (section 6.5). The standard library types
// a Set with add, has, and size together: add separates it from a Map (which has
// get and set, not add) and from an Array, size separates it from a WeakSet (which
// has no size), so the three names together are the fingerprint, read the same way
// isMapType reads its own. A caller must have already ruled the type out as an
// array, so this runs only for the non-array object shapes.
func (r *Renderer) isSetType(t frontend.Type) bool {
	var hasAdd, hasHas, hasSize bool
	for _, p := range r.prog.Properties(t) {
		switch p.Name {
		case "add":
			hasAdd = true
		case "has":
			hasHas = true
		case "size":
			hasSize = true
		}
	}
	return hasAdd && hasHas && hasSize
}

// isSet reports whether the checker types a node as a Set, the receiver test the
// set lowerings share (a new expression's target, a .add or .has call, a .size
// read). It is the node-level companion to isSetType: it reads the node's type and
// applies the same fingerprint, first ruling out an array so an array is never
// mistaken for a set.
func (r *Renderer) isSet(n frontend.Node) bool {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	return r.isSetType(t)
}

// setElem returns the member type of a Set type, read off its add method whose
// signature is add(value: T): this. The standard library declares add on every Set,
// so its first parameter carries T exactly, which is more direct than reconstructing
// it from the values iterator. It reports false for a type with no such add
// signature, which a non-set object is.
func (r *Renderer) setElem(t frontend.Type) (elem frontend.Type, ok bool) {
	var addType frontend.Type
	found := false
	for _, p := range r.prog.Properties(t) {
		if p.Name == "add" {
			addType, found = p.Type, true
			break
		}
	}
	if !found {
		return frontend.Type{}, false
	}
	call, _ := r.prog.Signatures(addType)
	if len(call) == 0 || len(call[0].Params) < 1 {
		return frontend.Type{}, false
	}
	return call[0].Params[0].Type, true
}

// renderPromise lowers a Promise<T> type to a pointer to the generic value.Promise
// header, the result type an async method's Go signature carries. It reads the
// element type off the promise's then callback and renders it through typeExpr, so
// a Promise<number> becomes a *value.Promise[float64]. A Promise<void> carries no
// value, so it lowers to the unit promise *value.Promise[value.Unit], the concrete
// Go type a void async body's settled promise takes. An element type that has no
// lowering yet hands back through the same NotYetLowerable typeExpr returns for it.
func (r *Renderer) renderPromise(t frontend.Type) (ast.Expr, error) {
	elem, ok := r.promiseElem(t)
	if !ok {
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "Promise type did not expose its value through a then signature"}
	}
	r.requireImport(valuePkg)
	if isVoidReturn(elem) {
		return star(index(sel("value", "Promise"), sel("value", "Unit"))), nil
	}
	eExpr, err := r.typeExpr(elem)
	if err != nil {
		return nil, err
	}
	return star(index(sel("value", "Promise"), eExpr)), nil
}

// isPromiseType reports whether an object type is a JavaScript Promise, the async
// result bento maps to value.Promise. The standard library types a promise with
// then, catch, and finally together: then and catch are the two a thenable spells,
// and finally separates a real Promise from a bare thenable or a user object that
// happens to carry a then, so the three names together are the fingerprint, read
// the same way isMapType reads its own. A caller must have already ruled the type
// out as an array, so this runs only for the non-array object shapes.
func (r *Renderer) isPromiseType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	var hasThen, hasCatch, hasFinally bool
	for _, p := range r.prog.Properties(t) {
		switch p.Name {
		case "then":
			hasThen = true
		case "catch":
			hasCatch = true
		case "finally":
			hasFinally = true
		}
	}
	return hasThen && hasCatch && hasFinally
}

// isPromise reports whether the checker types a node as a Promise, the receiver
// test the promise method lowerings share (a .then or .catch call). It is the
// node-level companion to isPromiseType: it reads the node's type and applies the
// same fingerprint, first ruling out an array so an array is never mistaken for a
// promise.
func (r *Renderer) isPromise(n frontend.Node) bool {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	return r.isPromiseType(t)
}

// promiseElem returns the value type of a Promise type, read off its then method
// whose first parameter is the fulfillment callback (value: T) => ... . The
// standard library declares then on every promise, and the checker resolves that
// callback's value parameter to T, so it carries the element type exactly. The
// callback parameter is optional, so its declared type is a union with undefined
// and null; the function member of that union carries the signature whose first
// parameter is T. It reports false for a type with no such then signature, which a
// non-promise object is.
func (r *Renderer) promiseElem(t frontend.Type) (elem frontend.Type, ok bool) {
	var thenType frontend.Type
	found := false
	for _, p := range r.prog.Properties(t) {
		if p.Name == "then" {
			thenType, found = p.Type, true
			break
		}
	}
	if !found {
		return frontend.Type{}, false
	}
	call, _ := r.prog.Signatures(thenType)
	if len(call) == 0 || len(call[0].Params) == 0 {
		return frontend.Type{}, false
	}
	onFulfilled := call[0].Params[0].Type
	// The callback parameter is optional, so its type is the callback function in a
	// union with undefined and null. Read the function member out of that union; a
	// non-union type is the function itself.
	candidates := r.prog.UnionMembers(onFulfilled)
	if len(candidates) == 0 {
		candidates = []frontend.Type{onFulfilled}
	}
	for _, c := range candidates {
		cbCall, _ := r.prog.Signatures(c)
		if len(cbCall) > 0 && len(cbCall[0].Params) > 0 {
			return cbCall[0].Params[0].Type, true
		}
	}
	return frontend.Type{}, false
}

// Decls returns the generated declarations the rendered types referred to, in a
// stable first-seen order, each a gofmt-clean Go declaration. A caller emits
// them once alongside the lowered functions that use them.
func (r *Renderer) Decls() []Decl { return r.decls.emit() }

// DeclNodes returns the same generated declarations as their go/ast nodes, in
// the same first-seen order, so the program assembler can splice them into the
// one file it prints rather than reparse the text Decls returns.
func (r *Renderer) DeclNodes() []ast.Decl { return r.decls.emitNodes() }
