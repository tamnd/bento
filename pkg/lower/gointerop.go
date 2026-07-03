package lower

import (
	"go/ast"
	"strconv"
	"strings"
	"unicode"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers a go: import to a direct call into the real Go package it
// names (16_go_interop.md section 9.1). The resolver classifies a go: specifier
// as a Go interop import and the checker binds the imported names against the
// package's generated declarations, so by the time lowering runs a call to one of
// those names is known-good: the emitted code imports the Go package under a
// local alias and calls the function directly, with the value crossings handled
// by the runtime bridge (pkg/goimport/bridge) rather than any interpreter hop.
//
// This first slice lowers the crossings a TypeScript type settles on its own: a
// string parameter transcodes through bridge.StringToGo and a string result back
// through bridge.StringFromGo, a boolean crosses unchanged, and any other type on
// a parameter or result hands back so the unit routes to the engine. The number
// crossings need the Go signature to tell int from int64 from float64 (section
// 7.5), which the TypeScript type alone does not carry, so they wait for the
// signature-driven marshaler in a later slice.

// bridgePkg is the import path of the interop runtime helper package the emitted
// Go calls to marshal a value across the boundary. Section 9.4 fixes that the
// generated code reaches the bridge by real import path, the same way it reaches
// the value model.
const bridgePkg = "github.com/tamnd/bento/pkg/goimport/bridge"

// goScheme is the specifier prefix that marks a go: interop import, the scheme the
// resolver classifies and the record step routes on.
const goScheme = "go:"

// goBuiltin names one imported go: symbol by the Go package it comes from and its
// exported name, the pair a call to the local binding dispatches on. The exported
// name is kept rather than the local binding so an aliased import (import { Sum as
// S }) still calls the function it actually names, exactly as nodeBuiltin does for
// the node: surface.
type goBuiltin struct {
	importPath string
	name       string
}

// recordGoImport records the bindings of one go: import declaration into
// r.goImports, so a call to a bound name lowers to a direct Go call. Only the
// named-import form lowers: a default or namespace import has no named-imports
// node to walk, so it hands back. Each specifier's identifier children are the
// exported Go name and, when the import is aliased, the local binding; the first
// is the Go symbol a call emits and the last is the local name a call site uses.
// The checker has already proven each name is a real export of the package, so
// the record step does not re-validate the surface, it only maps the bindings.
func (r *Renderer) recordGoImport(module string, clause frontend.Node, haveClause bool) error {
	importPath := goImportPath(module)
	if importPath == "" {
		return &NotYetLowerable{Reason: "go: import with an empty package path is a later slice"}
	}
	if !haveClause {
		return &NotYetLowerable{Reason: "bare import of " + module + " has no bindings to lower"}
	}
	named, ok := namedImportsNode(r.prog, clause)
	if !ok {
		return &NotYetLowerable{Reason: "default or namespace import of " + module + " is a later slice"}
	}
	for _, spec := range r.prog.Children(named) {
		names := identChildren(r.prog, spec)
		if len(names) == 0 {
			return &NotYetLowerable{Reason: "import specifier of " + module + " exposed no name"}
		}
		exported := names[0]
		local := names[len(names)-1]
		r.goImports[local] = goBuiltin{importPath: importPath, name: exported}
	}
	return nil
}

// goImportCall lowers a call to a name bound by a go: import to a direct call into
// the Go package. Each argument marshals by its TypeScript type: a string through
// bridge.StringToGo, a boolean unchanged. The result marshals back by the call's
// type: a string through bridge.StringFromGo, a boolean unchanged. A parameter or
// a result of any other type hands back, because the marshaling of a number, a
// byte slice, a struct, or an error needs the Go signature this slice does not yet
// read, and emitting a bare call for one would cross the boundary unsound.
func (r *Renderer) goImportCall(b goBuiltin, call frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if !isGoIdent(b.name) {
		return nil, &NotYetLowerable{Reason: "go: symbol " + b.name + " is not a Go identifier"}
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		switch {
		case r.isString(a):
			r.requireImport(bridgePkg)
			lowered = &ast.CallExpr{Fun: sel("bridge", "StringToGo"), Args: []ast.Expr{lowered}}
		case r.isBool(a):
			// A boolean is the one crossing that needs no transcoding: a bento boolean
			// and a Go bool are the same value, so it passes through unchanged (section
			// 7.1).
		default:
			return nil, &NotYetLowerable{Reason: "go: call argument of this type is a later slice"}
		}
		args = append(args, lowered)
	}
	alias := r.requireGoImport(b.importPath)
	goCall := &ast.CallExpr{Fun: sel(alias, b.name), Args: args}
	rt := r.prog.TypeAt(call)
	switch {
	case rt.Flags&frontend.TypeString != 0:
		r.requireImport(bridgePkg)
		return &ast.CallExpr{Fun: sel("bridge", "StringFromGo"), Args: []ast.Expr{goCall}}, nil
	case rt.Flags&frontend.TypeBoolean != 0:
		return goCall, nil
	default:
		return nil, &NotYetLowerable{Reason: "go: call result of this type is a later slice"}
	}
}

// requireGoImport records that the emitted Go imports a package under a local
// alias and returns that alias, assigning it on first use so a package imported
// but never called emits no import (which Go would reject as unused). The alias is
// stable for the rest of the lowering, so every call into the same package renders
// the same qualifier.
func (r *Renderer) requireGoImport(importPath string) string {
	if a, ok := r.goAliases[importPath]; ok {
		return a
	}
	alias := r.uniqueGoAlias(importPath)
	r.goAliases[importPath] = alias
	return alias
}

// uniqueGoAlias picks a local alias for a Go package: the last path segment,
// sanitized to a Go identifier, which matches the package's own name for the
// common case (crypto/sha256 aliases sha256) and reads the way the spec's examples
// do. A collision with another aliased package or a reserved runtime name takes a
// numeric suffix, so two packages that share a last segment still get distinct
// aliases.
func (r *Renderer) uniqueGoAlias(importPath string) string {
	base := goAliasBase(importPath)
	alias := base
	for n := 2; r.goAliasTaken(alias); n++ {
		alias = base + strconv.Itoa(n)
	}
	return alias
}

// goAliasTaken reports whether an alias is already spoken for, either by another
// aliased Go package or by a name the emitted file gives its own runtime imports,
// so uniqueGoAlias never shadows the value model or the bridge.
func (r *Renderer) goAliasTaken(alias string) bool {
	switch alias {
	case "value", "bridge", "main":
		return true
	}
	for _, a := range r.goAliases {
		if a == alias {
			return true
		}
	}
	return false
}

// goImportPath strips the go: scheme and any pinned @version from a specifier,
// leaving the bare Go import path the emitted code imports. A Go import path never
// contains an @, so splitting on the first one recovers the path cleanly.
func goImportPath(module string) string {
	p := strings.TrimPrefix(module, goScheme)
	if at := strings.IndexByte(p, '@'); at >= 0 {
		p = p[:at]
	}
	return p
}

// goAliasBase is the un-suffixed alias for an import path: its last segment,
// sanitized so every character is legal in a Go identifier. An empty result (a
// path that is only separators) falls back to a fixed name so the alias is always
// a legal identifier.
func goAliasBase(importPath string) string {
	seg := importPath
	if slash := strings.LastIndexByte(seg, '/'); slash >= 0 {
		seg = seg[slash+1:]
	}
	base := sanitizeGoIdent(seg)
	if base == "" {
		base = "gopkg"
	}
	return base
}

// sanitizeGoIdent rewrites a string to a legal Go identifier: a letter, digit, or
// underscore is kept, anything else becomes an underscore, and a leading digit
// takes an underscore prefix so the result never starts with one. It is used only
// for import aliases, whose exact spelling is bento's to choose, so a lossy
// rewrite is fine as long as it is a valid identifier.
func sanitizeGoIdent(s string) string {
	var b strings.Builder
	for i, r := range s {
		switch {
		case r == '_' || unicode.IsLetter(r):
			b.WriteRune(r)
		case unicode.IsDigit(r):
			if i == 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
