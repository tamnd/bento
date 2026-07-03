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
	// optLocals is the set of local names in the body currently being lowered that
	// are declared with an optional type (T | undefined, lowered to value.Opt[T]).
	// It is computed once per body by optLocalsOf and, like int32Locals, saved and
	// restored around each body. A read of one of these locals at a point the checker
	// narrowed to T (past a presence guard) unwraps with .Get(), the only place the
	// stored T is pulled out of the option; a read where the type is still optional
	// keeps the bare Opt value. A nil map (the default) unwraps nothing.
	optLocals map[string]bool
	// errorLocals is the set of catch-binding names in scope while a catch block is
	// lowered, each bound to the *value.Error the catch recovered. A read of the
	// binding's .message or .name lowers to the matching method on the error; the
	// binding used any other way hands back, since the runtime models a caught error
	// as a value.Error and not a general boxed value yet. It is set around a catch
	// block and cleared after, so a binding does not leak past its clause.
	errorLocals map[string]bool
	// usesThrow records that the program emitted a construct that can raise a
	// thrown value the runtime must report if it escapes: an explicit throw, or a
	// boundary crossing that range-checks (a go: int64 result). When set, the
	// assembled main defers value.ReportUncaught so an uncaught throw prints an
	// uncaught-error line and exits non-zero rather than crashing with a Go stack. A
	// program that cannot throw defers nothing, so its main and its imports are
	// unchanged.
	usesThrow bool
	// strBuilders is the list of reusable value.StrBuilder variables the current
	// body needs, one per template or number-interpolated concatenation site the
	// lowerer chose to build through a builder. A var declaration for each is hoisted
	// to the top of the body, above any loop, so the builder is reused across
	// iterations rather than recreated. Like int32Locals it is scoped to one body,
	// saved and restored around each so one function's builders do not leak into
	// another, and a nil slice (the default) hoists nothing.
	strBuilders []string
}

// NewRenderer builds a renderer over a checked program.
func NewRenderer(prog *frontend.Program) *Renderer {
	return &Renderer{prog: prog, decls: newDeclSet(), imports: map[string]bool{}, nodeImports: map[string]nodeBuiltin{}, goImports: map[string]goBuiltin{}, goNamespaces: map[string]string{}, goAliases: map[string]string{}, errorLocals: map[string]bool{}}
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

	case t.Flags&frontend.TypeObject != 0:
		// TypeObject covers both arrays and fixed-shape objects in the frontend
		// vocabulary. An element type means it is an array; otherwise it is an
		// object shape that lowers to a struct.
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
		return r.renderObject(t)

	case t.Flags&frontend.TypeUnion != 0:
		return r.renderUnion(t)

	case t.Flags&frontend.TypeLiteral != 0:
		// A lone literal type (a bare "circle" or 42 outside a union) lowers to
		// its widened base and so is caught by the primitive cases above, which
		// run first because a string literal also carries TypeString. Reaching
		// here means a literal with no base flag bento renders yet.
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "literal type with no lowerable base"}

	case t.Flags&frontend.TypeEnum != 0:
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "enum lowering lands in a later slice"}

	case t.Flags&frontend.TypeIntersection != 0:
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "intersection lowering (merged struct) lands in a later slice"}

	case t.Flags&frontend.TypeTypeParameter != 0:
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "type parameter needs monomorphization, a later slice"}

	default:
		return nil, &NotYetLowerable{Flags: t.Flags, Reason: "no lowering for this type"}
	}
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

// Decls returns the generated declarations the rendered types referred to, in a
// stable first-seen order, each a gofmt-clean Go declaration. A caller emits
// them once alongside the lowered functions that use them.
func (r *Renderer) Decls() []Decl { return r.decls.emit() }

// DeclNodes returns the same generated declarations as their go/ast nodes, in
// the same first-seen order, so the program assembler can splice them into the
// one file it prints rather than reparse the text Decls returns.
func (r *Renderer) DeclNodes() []ast.Decl { return r.decls.emitNodes() }
