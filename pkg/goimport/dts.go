package goimport

import (
	"go/types"
	"sort"
	"strings"
)

// This file is the .d.ts generator of document 16 section 5: it walks a Go
// package's exported surface and emits the TypeScript declarations that make the
// package feel like a typed module. The checker type-checks calls into Go, the
// editor autocompletes Go functions and methods, and the author never writes a
// type by hand. The type projection is the Mapper (section 6); this file is the
// walk over the scope and the shape of each declaration.

// DocLookup returns the documentation attached to a Go object, or "" when it has
// none. It is injected because the doc text comes from the AST, which the loader
// (section 4) holds, while this generator works from go/types; a nil lookup emits
// declarations with no TSDoc, the same as an undocumented npm typing.
type DocLookup func(obj types.Object) string

// GenOptions configures one declaration-file generation. ImportPath and Version
// stamp the header so a reader (and the cache, section 4.5) knows exactly which
// module version the declarations describe.
type GenOptions struct {
	// ImportPath is the Go import path the declarations are generated from, used
	// only in the header banner.
	ImportPath string
	// Version is the module version, used in the header banner; "" prints the path
	// without a version suffix.
	Version string
	// Docs resolves a Go object's doc comment to TSDoc text. Optional.
	Docs DocLookup
}

// Generate produces the full .d.ts text for a package's exported API. It emits a
// header banner, one import of exactly the bento:go helpers the file references,
// and a declaration for every exported function, type, constant, and variable, in
// a stable alphabetical order so the output is deterministic and the cache key is
// meaningful.
func Generate(pkg *types.Package, opts GenOptions) string {
	g := &generator{
		pkg:    pkg,
		mapper: NewMapper(pkg),
		docs:   opts.Docs,
	}
	body := g.walk()

	var out strings.Builder
	out.WriteString(g.header(opts))
	if imp := g.helperImport(); imp != "" {
		out.WriteString(imp)
		out.WriteString("\n")
	}
	out.WriteString("\n")
	out.WriteString(body)
	return out.String()
}

type generator struct {
	pkg    *types.Package
	mapper *Mapper
	docs   DocLookup
}

// header is the two-line banner that marks the file as generated and names the
// source module and version, so no one edits it by hand (section 5.2).
func (g *generator) header(opts GenOptions) string {
	src := opts.ImportPath
	if opts.Version != "" {
		src += "@" + opts.Version
	}
	if src == "" {
		src = g.pkg.Path()
	}
	return "// Generated from " + src + " by bento.\n" +
		"// Do not edit; regenerated on version or toolchain change.\n"
}

// helperImport builds the single `import type { ... } from "bento:go"` line for
// exactly the helpers the body referenced, or "" when the file uses none.
func (g *generator) helperImport() string {
	used := g.mapper.Used()
	if len(used) == 0 {
		return ""
	}
	names := make([]string, len(used))
	for i, h := range used {
		names[i] = string(h)
	}
	return "import type { " + strings.Join(names, ", ") + " } from \"bento:go\";\n"
}

// walk emits a declaration for every exported package-level object, sorted by
// name. It must run before helperImport is read, because emitting the bodies is
// what records which bento:go helpers the file uses.
func (g *generator) walk() string {
	scope := g.pkg.Scope()
	names := scope.Names()
	sort.Strings(names)

	var b strings.Builder
	first := true
	for _, name := range names {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		decl := g.declare(obj)
		if decl == "" {
			continue
		}
		if !first {
			b.WriteString("\n")
		}
		first = false
		b.WriteString(decl)
	}
	return b.String()
}

// declare emits one exported object's declaration, dispatching on its kind.
func (g *generator) declare(obj types.Object) string {
	switch o := obj.(type) {
	case *types.Func:
		return g.declareFunc(o)
	case *types.TypeName:
		return g.declareType(o)
	case *types.Const:
		return g.doc(obj) + "export const " + obj.Name() + ": " + g.mapper.Map(obj.Type()) + ";\n"
	case *types.Var:
		// Most exported Go variables are read-only sentinels (io.EOF), so const is
		// the honest projection (section 6.10).
		return g.doc(obj) + "export const " + obj.Name() + ": " + g.mapper.Map(obj.Type()) + ";\n"
	default:
		return ""
	}
}

// declareFunc emits a top-level function declaration, carrying its type
// parameters (section 11) and hoisting a trailing error to a throw (section 6.6).
func (g *generator) declareFunc(fn *types.Func) string {
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return ""
	}
	tp := g.mapper.TypeParams(sig.TypeParams())
	return g.doc(fn) + "export function " + fn.Name() + tp +
		"(" + g.mapper.ParamList(sig) + "): " + g.mapper.Results(sig) + ";\n"
}

// declareType emits a named type. A struct or interface projects to a TypeScript
// interface carrying its exported fields and methods; a defined type over a basic
// projects to a branded alias so Go's named-type distinction has teeth in the
// checker (section 6.11); any other underlying projects to a plain type alias.
func (g *generator) declareType(tn *types.TypeName) string {
	named, ok := tn.Type().(*types.Named)
	if !ok {
		// An alias to a non-named type (type Byte = byte); project the target.
		return g.doc(tn) + "export type " + tn.Name() + " = " + g.mapper.Map(tn.Type()) + ";\n"
	}
	tp := g.mapper.TypeParams(named.TypeParams())
	switch u := named.Underlying().(type) {
	case *types.Struct:
		if !structHasExportedSurface(named, u) {
			// A struct with no exported fields and no exported methods has no shape the
			// author can read or call, so it projects as an opaque token rather than an
			// empty interface: it is a handle received from one call and passed to another
			// (section 6.13), which is exactly how the runtime crosses it.
			g.mapper.mark(HelperOpaque)
			return g.doc(tn) + "export type " + tn.Name() + tp +
				" = GoOpaque<\"" + g.pkg.Name() + "." + tn.Name() + "\">;\n"
		}
		return g.declareStruct(tn, named, tp, u)
	case *types.Interface:
		return g.declareInterface(tn, tp, u)
	case *types.Basic:
		return g.declareBranded(tn, tp, u)
	default:
		return g.doc(tn) + "export type " + tn.Name() + tp + " = " + g.mapper.Map(u) + ";\n"
	}
}

// structHasExportedSurface reports whether a struct type has any exported field or
// any exported method, the shape an author can read or call. A struct with neither
// is an opaque token, not a class, so it projects as GoOpaque (section 6.13).
func structHasExportedSurface(named *types.Named, st *types.Struct) bool {
	for f := range st.Fields() {
		if f.Exported() {
			return true
		}
	}
	for fn := range named.Methods() {
		if fn.Exported() {
			return true
		}
	}
	return false
}

// declareStruct emits a struct as an interface: exported fields become properties
// and exported methods become method members, with the receiver implicit the same
// way this is implicit in TypeScript (section 6.7).
func (g *generator) declareStruct(tn *types.TypeName, named *types.Named, tp string, st *types.Struct) string {
	var b strings.Builder
	b.WriteString(g.doc(tn))
	b.WriteString("export interface " + tn.Name() + tp + " {\n")
	for f := range st.Fields() {
		if !f.Exported() {
			continue
		}
		b.WriteString(g.indentDoc(f))
		b.WriteString("  " + f.Name() + ": " + g.mapper.Map(f.Type()) + ";\n")
	}
	b.WriteString(g.methods(named))
	b.WriteString("}\n")
	return b.String()
}

// declareInterface emits a named interface as a TypeScript interface of its
// methods, which is a faithful projection because both languages type interfaces
// structurally (section 6.8).
func (g *generator) declareInterface(tn *types.TypeName, tp string, it *types.Interface) string {
	var b strings.Builder
	b.WriteString(g.doc(tn))
	b.WriteString("export interface " + tn.Name() + tp + " {\n")
	for fn := range it.Methods() {
		if !fn.Exported() {
			continue
		}
		b.WriteString(g.method(fn))
	}
	b.WriteString("}\n")
	return b.String()
}

// declareBranded emits a defined type over a basic as a branded alias: the
// runtime value is the underlying type, and the phantom brand keeps the checker
// from silently accepting an unrelated value of the same underlying type
// (section 6.11).
func (g *generator) declareBranded(tn *types.TypeName, tp string, b *types.Basic) string {
	brand := g.pkg.Name() + "." + tn.Name()
	underlying := g.mapper.Map(b)
	return g.doc(tn) + "export type " + tn.Name() + tp + " = " + underlying +
		" & { readonly __brand: \"" + brand + "\" };\n"
}

// methods emits the exported methods declared on a named type, each as an
// interface member. Only methods declared directly on the type are emitted;
// promoted methods from an embedded field are left for a later pass.
func (g *generator) methods(named *types.Named) string {
	var b strings.Builder
	for fn := range named.Methods() {
		if !fn.Exported() {
			continue
		}
		b.WriteString(g.method(fn))
	}
	return b.String()
}

// method emits one method as an interface member: its name, parameters, and the
// throw-mode result, indented one level inside the interface body.
func (g *generator) method(fn *types.Func) string {
	sig, ok := fn.Type().(*types.Signature)
	if !ok {
		return ""
	}
	return g.indentDoc(fn) + "  " + fn.Name() +
		"(" + g.mapper.ParamList(sig) + "): " + g.mapper.Results(sig) + ";\n"
}

// doc renders an object's documentation as a top-level TSDoc block, or "" when
// there is none. A single line becomes a one-line block; multiple lines become a
// starred block, so the library author's own words show on hover (section 5.3).
func (g *generator) doc(obj types.Object) string {
	return renderDoc(g.lookupDoc(obj), "")
}

// indentDoc is doc for a member, indented two spaces to sit inside an interface.
func (g *generator) indentDoc(obj types.Object) string {
	return renderDoc(g.lookupDoc(obj), "  ")
}

func (g *generator) lookupDoc(obj types.Object) string {
	if g.docs == nil {
		return ""
	}
	return g.docs(obj)
}

func renderDoc(text, indent string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) == 1 {
		return indent + "/** " + lines[0] + " */\n"
	}
	var b strings.Builder
	b.WriteString(indent + "/**\n")
	for _, line := range lines {
		b.WriteString(indent + " * " + strings.TrimRight(line, " \t") + "\n")
	}
	b.WriteString(indent + " */\n")
	return b.String()
}
