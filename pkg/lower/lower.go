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
	// int32Locals is the set of local names in the body currently being lowered that
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
	// dynLocals is the set of names in the body currently being lowered that are
	// bound as boxed dynamic values: a parameter or a local typed any or unknown,
	// which typeExpr lowers to value.Value. A read of one at a point the checker
	// narrowed to a single primitive, past a typeof guard, goes through the
	// matching accessor (AsString, AsNumber, AsBool) so the static expression it
	// flows into sees the unboxed Go value; a read still typed any keeps the bare
	// box, which is what the runtime helpers take. It is built per body next to
	// unionLocals and saved and restored the same way. A nil map unwraps nothing.
	dynLocals map[string]bool
	// errorLocals is the set of catch-binding names in scope while a catch block is
	// lowered, each bound to the *value.Error the catch recovered. A read of the
	// binding's .message or .name lowers to the matching method on the error; the
	// binding used any other way hands back, since the runtime models a caught error
	// as a value.Error and not a general boxed value yet. It is set around a catch
	// block and cleared after, so a binding does not leak past its clause.
	errorLocals map[string]bool
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
}

// NewRenderer builds a renderer over a checked program.
func NewRenderer(prog *frontend.Program) *Renderer {
	return &Renderer{prog: prog, decls: newDeclSet(), imports: map[string]bool{}, nodeImports: map[string]nodeBuiltin{}, goImports: map[string]goBuiltin{}, goNamespaces: map[string]string{}, goAliases: map[string]string{}, errorLocals: map[string]bool{}, bigLits: map[string]string{}, classes: map[string]*classInfo{}, enums: map[string]*enumInfo{}, unionBySig: map[string]*unionInfo{}}
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

	// A union whose members all share a primitive facet (a numeric-literal union
	// like 1 | 2 | 3 is a number, true | false is a boolean) widens to that
	// primitive's Go type, the same type the predicates fold it to, so a function
	// returning such a union gets a float64 or bool result rather than routing to
	// the tagged-sum machinery a real object union needs. A closed string-literal
	// union is deliberately left out: it lowers to an integer tag enum below, not a
	// primitive. A union that folds nothing (a mixed or optional union) keeps its
	// own flags and falls through to the union handling.
	if t.Flags&frontend.TypeUnion != 0 {
		if pf := r.primitiveFlagsOfType(t); pf != t.Flags {
			switch {
			case pf&frontend.TypeNumber != 0:
				return ident("float64"), nil
			case pf&frontend.TypeBoolean != 0:
				return ident("bool"), nil
			}
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
		// A symbol is a unique opaque value whose whole purpose is identity, so
		// a pointer whose identity is the symbol's identity (section 8).
		return star(sel("value", "Symbol")), nil

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
		if name, ok := r.typedArrayName(t); ok {
			// A typed array (section 6.3) is the value model's fixed-width numeric buffer,
			// spelled as a pointer the same way a typed Array is. Uint8Array carries its own
			// []byte representation for the go: boundary (section 7.3); the rest of the
			// numeric family share the generic value.TypedArray[T]. It is not a struct shape,
			// so it routes here before renderObject would intern its interface as fields.
			return r.renderTypedArray(name)
		}
		if r.isMapType(t) {
			// A Map<K, V> (section 6.5) is the value model's keyed collection, spelled as a
			// pointer to the generic value.Map header, and the type a Go map[K]V projects to
			// across the boundary. It is not a struct shape, so it routes here before
			// renderObject would intern its method interface as fields.
			return r.renderMap(t)
		}
		if r.isSetType(t) {
			// A Set<T> (section 6.5) is the value model's collection of unique members,
			// spelled as a pointer to the generic value.Set header. Like Map it is not a
			// struct shape, so it routes here before renderObject would intern its method
			// interface as fields. Its fingerprint is disjoint from a Map's, so the order
			// against the Map check above does not matter.
			return r.renderSet(t)
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
	// the empty-struct object lowering, which would be unsound. Only a single plain
	// call signature lowers here; an overload set (more than one call signature), a
	// callable object that also carries a construct signature or its own properties,
	// or a generic or rest-or-optional-parameter signature is a later slice and
	// hands the unit back rather than emit a wrong shape.
	if len(call) != 1 || len(construct) != 0 || len(r.prog.Properties(t)) != 0 {
		return nil, true, &NotYetLowerable{Flags: t.Flags, Reason: "function type with overloads, properties, or a construct signature is a later slice"}
	}
	sig := call[0]
	if len(sig.TypeParams) != 0 || sig.RestParam != nil {
		return nil, true, &NotYetLowerable{Flags: t.Flags, Reason: "generic or rest-parameter function type is a later slice"}
	}
	var params []*ast.Field
	for _, p := range sig.Params {
		if p.Optional {
			return nil, true, &NotYetLowerable{Flags: t.Flags, Reason: "function type with an optional parameter is a later slice"}
		}
		pt, err := r.typeExpr(p.Type)
		if err != nil {
			return nil, true, err
		}
		params = append(params, &ast.Field{Type: pt})
	}
	ft := &ast.FuncType{Params: &ast.FieldList{List: params}}
	// A void or never return (or the zero type at a position with no value) is a
	// Go func with no results; any other return lowers to the single Go result
	// the signature carries.
	if sig.Return.Flags != 0 && sig.Return.Flags&(frontend.TypeVoid|frontend.TypeNever) == 0 {
		rt, err := r.typeExpr(sig.Return)
		if err != nil {
			return nil, true, err
		}
		ft.Results = &ast.FieldList{List: []*ast.Field{{Type: rt}}}
	}
	return ft, true, nil
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
func (r *Renderer) renderObject(t frontend.Type) (ast.Expr, error) {
	name, err := r.decls.internStruct(r, t)
	if err != nil {
		return nil, err
	}
	return star(ident(name)), nil
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
// *value.Uint8Array byte buffer, and each numeric-family member to a
// *value.TypedArray[T] over the element's Go type. A bigint-element array
// (BigInt64Array, BigUint64Array) has no lowering yet and hands back.
func (r *Renderer) renderTypedArray(name string) (ast.Expr, error) {
	r.requireImport(valuePkg)
	if name == "Uint8Array" {
		return star(sel("value", "Uint8Array")), nil
	}
	elem, ok := typedArrayElemGo(name)
	if !ok {
		return nil, &NotYetLowerable{Reason: "the " + name + " typed array is a later slice"}
	}
	return star(index(sel("value", "TypedArray"), ident(elem))), nil
}

// typedArrayElemGo maps a numeric typed-array name to the Go element type of its
// value.TypedArray representation, and ok=false for a name outside that family:
// Uint8Array has its own []byte representation, and the bigint-element arrays are
// a later slice. Uint8ClampedArray stores a uint8 like Uint8Array but through the
// generic buffer with the clamp coercion, so the two are distinct Go types.
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

// Decls returns the generated declarations the rendered types referred to, in a
// stable first-seen order, each a gofmt-clean Go declaration. A caller emits
// them once alongside the lowered functions that use them.
func (r *Renderer) Decls() []Decl { return r.decls.emit() }

// DeclNodes returns the same generated declarations as their go/ast nodes, in
// the same first-seen order, so the program assembler can splice them into the
// one file it prints rather than reparse the text Decls returns.
func (r *Renderer) DeclNodes() []ast.Decl { return r.decls.emitNodes() }
