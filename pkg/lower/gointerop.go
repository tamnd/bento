package lower

import (
	"go/ast"
	"strconv"
	"strings"
	"unicode"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/goimport"
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
		// A namespace import (import * as zstd from "go:...") binds the package's whole
		// exported surface to one name, and a member call on it lowers the same way a
		// named import does. It has no named-imports node, so it is recognized by its
		// "* as name" clause and recorded as a namespace binding; a default import,
		// which a Go package has no export for, still hands back.
		if binding, ok := namespaceBinding(r.prog, clause); ok {
			r.goNamespaces[binding] = importPath
			return nil
		}
		return &NotYetLowerable{Reason: "default import of " + module + " is a later slice"}
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

// namespaceBinding returns the local name a namespace import binds, the name in
// import * as name from "go:...". The clause of a namespace import renders as
// "* as name" and holds a single identifier descendant, the binding; a named or
// default import does not start with the star, so it reports not found and the
// caller routes it elsewhere.
func namespaceBinding(prog *frontend.Program, clause frontend.Node) (string, bool) {
	if !strings.HasPrefix(strings.TrimSpace(prog.Text(clause)), "*") {
		return "", false
	}
	if id, ok := firstIdentifier(prog, clause); ok {
		return id, true
	}
	return "", false
}

// firstIdentifier returns the text of the first identifier in a subtree, in a
// pre-order walk. It pulls the binding name out of a namespace import clause, whose
// only identifier is the bound name.
func firstIdentifier(prog *frontend.Program, n frontend.Node) (string, bool) {
	if n.Kind() == frontend.NodeIdentifier {
		return prog.Text(n), true
	}
	for _, c := range prog.Children(n) {
		if id, ok := firstIdentifier(prog, c); ok {
			return id, true
		}
	}
	return "", false
}

// namespaceGoCall resolves a member callee against the namespace go: imports: when
// the callee is name.Member and name is a namespace binding, it returns the
// goBuiltin for Member in that package, the same shape a named import's binding
// carries. The property access's two identifier children are the namespace binding
// and the exported Go name; anything else (a deeper access, a non-identifier
// object) is not a namespace call and reports not found so the method-call path
// takes it.
func (r *Renderer) namespaceGoCall(access frontend.Node) (goBuiltin, bool) {
	kids := r.prog.Children(access)
	if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier || kids[1].Kind() != frontend.NodeIdentifier {
		return goBuiltin{}, false
	}
	importPath, ok := r.goNamespaces[r.prog.Text(kids[0])]
	if !ok {
		return goBuiltin{}, false
	}
	return goBuiltin{importPath: importPath, name: r.prog.Text(kids[1])}, true
}

// goConstRef lowers a reference to a name bound by a go: import that names an
// exported Go constant to the qualified constant read, marshaled to a bento value
// by the constant's Go type (section 6.10). It is consulted from the identifier
// path: a bound name used as a value rather than called is a constant reference
// when the const resolver knows its type keyword, and anything else (a function
// used as a value, an unwired resolver) reports not handled so the caller hands the
// reference back. A constant read cannot panic, so it needs no boundary guard.
func (r *Renderer) goConstRef(name string) (ast.Expr, bool, error) {
	b, ok := r.goImports[name]
	if !ok || r.goConsts == nil {
		return nil, false, nil
	}
	info, ok := r.goConsts(b.importPath, b.name)
	if !ok {
		return nil, false, nil
	}
	alias := r.requireGoImport(b.importPath)
	read := ast.Expr(sel(alias, b.name))
	if info.Defined {
		// A constant of a defined type over a basic (time.Second) is read as the branded
		// type; strip it to the underlying basic so the marshaling sees the plain Go type,
		// the same step a defined-type result takes (section 6.11).
		read = stripBrandToBasic(info.Keyword, read)
	}
	marshaled, err := r.marshalResultFromGo(info.Keyword, read)
	if err != nil {
		return nil, false, err
	}
	return marshaled, true, nil
}

// goImportCall lowers a call to a name bound by a go: import to a direct call into
// the Go package. When the Go signature is in hand (the build wires it), each
// argument and the result marshal by the Go type, so a number crosses with the
// right conversion and 64-bit range check (section 7.5). Without a signature it
// falls back to the crossings a TypeScript type settles on its own, the string and
// boolean marshaling, and hands a number back. Either way a crossing the slice
// does not cover hands the whole call back rather than emit an unsound call.
func (r *Renderer) goImportCall(b goBuiltin, call frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	if !isGoIdent(b.name) {
		return nil, &NotYetLowerable{Reason: "go: symbol " + b.name + " is not a Go identifier"}
	}
	if sig, ok := r.goSignature(b); ok {
		return r.goImportCallBySig(b, sig, argNodes)
	}
	return r.goImportCallByType(b, call, argNodes)
}

// goSignature returns the Go signature of a go: function when a resolver is wired
// and the signature is in a shape this slice marshals. A cleared OK (a variadic, an
// error return, an unsupported type) reports not-found so the caller falls back to
// the type-only path, which hands back on anything past string and boolean.
func (r *Renderer) goSignature(b goBuiltin) (goimport.FuncSig, bool) {
	if r.goSigs == nil {
		return goimport.FuncSig{}, false
	}
	sig, ok := r.goSigs(b.importPath, b.name)
	if !ok || !sig.OK {
		return goimport.FuncSig{}, false
	}
	return sig, true
}

// goImportCallBySig lowers a go: call against its Go signature, marshaling each
// argument and the result by the Go type keyword the signature carries. The
// argument count must match the signature, and each argument's marshaling must be
// one the crossing supports, or the call hands back.
func (r *Renderer) goImportCallBySig(b goBuiltin, sig goimport.FuncSig, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != len(sig.Params) {
		return nil, &NotYetLowerable{Reason: "go: call to " + b.name + " with a defaulted or spread argument is a later slice"}
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for i, a := range argNodes {
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		marshaled, err := r.marshalArgToGo(sig.Params[i], sig.ParamConv[i], lowered)
		if err != nil {
			return nil, err
		}
		args = append(args, marshaled)
	}
	alias := r.requireGoImport(b.importPath)
	goCall := &ast.CallExpr{Fun: sel(alias, b.name), Args: args}
	if sig.Throws {
		// A trailing error hoists to a throw. The Go call returns (T, error) or just
		// error, and Go's f(g()) call form passes both results straight into the bridge
		// helper: bridge.Must returns the value and raises on a non-nil error, and
		// bridge.Check is its no-result sibling. The raised GoError is a value.Thrown,
		// so recording usesThrow defers the top-level reporter, and a bento catch reads
		// it as an Error (section 7.7).
		r.usesThrow = true
		r.requireImport(bridgePkg)
		if len(sig.Results) == 0 {
			// error-only: bridge.Check(call), valid where the void call is a statement.
			return r.guardVoid(&ast.CallExpr{Fun: sel("bridge", "Check"), Args: []ast.Expr{goCall}}), nil
		}
		must := &ast.CallExpr{Fun: sel("bridge", "Must"), Args: []ast.Expr{goCall}}
		marshaled, err := r.marshalResultFromGo(sig.Results[0], r.stripResultBrand(sig, must))
		if err != nil {
			return nil, err
		}
		return r.guardExpr(marshaled, sig.Results[0]), nil
	}
	if len(sig.Results) == 0 {
		// A Go function with no value result lowers to the bare call, valid where the
		// TypeScript call is used as a statement, which is the only place a void call
		// type-checks.
		return r.guardVoid(goCall), nil
	}
	marshaled, err := r.marshalResultFromGo(sig.Results[0], r.stripResultBrand(sig, goCall))
	if err != nil {
		return nil, err
	}
	return r.guardExpr(marshaled, sig.Results[0]), nil
}

// stripResultBrand converts a defined-type result to its underlying basic before
// the result marshaling reads it, so the brand of section 6.11 is gone by the time
// the value crosses back. A plain-basic result is returned untouched.
func (r *Renderer) stripResultBrand(sig goimport.FuncSig, expr ast.Expr) ast.Expr {
	if !sig.ResultDefined || len(sig.Results) == 0 {
		return expr
	}
	return stripBrandToBasic(sig.Results[0], expr)
}

// stripBrandToBasic converts a branded value of a defined type over a basic to that
// basic, so the marshaling that reads it sees the plain Go type it expects. It is
// needed only for the keywords the marshaling hands straight to the bridge or returns
// unchanged (string, bool, float64) and the wide integers it forwards without a
// widening conversion (int64, uint64, uintptr): the remaining integer and float
// keywords already ride a conversion in the marshaling that strips the brand, so they
// pass through here untouched.
func stripBrandToBasic(keyword string, expr ast.Expr) ast.Expr {
	switch keyword {
	case "string", "bool", "float64", "int64", "uint64", "uintptr":
		return &ast.CallExpr{Fun: ident(keyword), Args: []ast.Expr{expr}}
	}
	return expr
}

// guardExpr wraps a lowered go: call result in the boundary recover of section
// 12.3, so a panic from the Go library the call entered becomes a catchable thrown
// GoError rather than an uncaught Go traceback. The guard closure returns the bento
// Go type the marshaled result evaluates to (a string result is a value.BStr, a
// boolean is a bool, every number is a float64), which the go: keyword names. A
// guarded call can raise, so this records usesThrow to defer the top-level uncaught
// reporter, which is what keeps the value model imported for the value.BStr result
// type.
func (r *Renderer) guardExpr(expr ast.Expr, goResult string) ast.Expr {
	r.usesThrow = true
	r.requireImport(bridgePkg)
	return &ast.CallExpr{
		Fun: sel("bridge", "Guard"),
		Args: []ast.Expr{
			&ast.FuncLit{
				Type: &ast.FuncType{
					Params:  &ast.FieldList{},
					Results: &ast.FieldList{List: []*ast.Field{{Type: r.bentoResultType(goResult)}}},
				},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{expr}}}},
			},
		},
	}
}

// guardVoid is the statement form of guardExpr, wrapping a go: call that returns
// nothing (a void Go function or the error-only bridge.Check) in bridge.Guard0 so a
// panic from the call is converted the same way. The result is itself a call
// expression, valid in the statement position a void go: call already occupies.
func (r *Renderer) guardVoid(call ast.Expr) ast.Expr {
	r.usesThrow = true
	r.requireImport(bridgePkg)
	return &ast.CallExpr{
		Fun: sel("bridge", "Guard0"),
		Args: []ast.Expr{
			&ast.FuncLit{
				Type: &ast.FuncType{Params: &ast.FieldList{}},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: call}}},
			},
		},
	}
}

// bentoResultType names the Go type a marshaled go: result evaluates to, the type
// the guard closure returns: a string result crosses to a value.BStr, a boolean to
// a bool, and every numeric result widens to a float64 bento number. The value
// model is already imported whenever a call is guarded, because guarding records
// usesThrow and the uncaught reporter lives in the value package.
func (r *Renderer) bentoResultType(goResult string) ast.Expr {
	switch goResult {
	case "string":
		return sel("value", "BStr")
	case "bool":
		return ident("bool")
	default:
		return ident("float64")
	}
}

// marshalArgToGo wraps a lowered argument in the crossing its Go parameter type
// needs: a string transcodes through the bridge, a boolean passes through, and a
// number converts to the Go numeric type (a bento number is a float64, so a Go int
// parameter takes an int conversion). A parameter type the slice does not cover
// hands back. When conv names a defined type over a basic (time.Duration over
// int64), the bento value converts straight to the named type qualified by its
// package, so the Go call receives a real time.Duration; the underlying-basic
// conversion the plain path applies is unnecessary because the named conversion
// narrows a bento number itself, and a string still transcodes through the bridge
// before it becomes the defined string (section 6.11).
func (r *Renderer) marshalArgToGo(goType string, conv goimport.DefinedConv, arg ast.Expr) (ast.Expr, error) {
	if conv.Name != "" {
		inner := arg
		if goType == "string" {
			r.requireImport(bridgePkg)
			inner = &ast.CallExpr{Fun: sel("bridge", "StringToGo"), Args: []ast.Expr{arg}}
		}
		alias := r.requireGoImport(conv.Path)
		return &ast.CallExpr{Fun: sel(alias, conv.Name), Args: []ast.Expr{inner}}, nil
	}
	switch goType {
	case "string":
		r.requireImport(bridgePkg)
		return &ast.CallExpr{Fun: sel("bridge", "StringToGo"), Args: []ast.Expr{arg}}, nil
	case "bool":
		return arg, nil
	case "float64":
		// A bento number is already a float64, so a float64 parameter needs no
		// conversion.
		return arg, nil
	default:
		if isGoNumeric(goType) {
			// A bento number is a float64; a narrower or integer Go parameter takes an
			// explicit conversion, which truncates toward zero exactly as a JavaScript
			// to-integer coercion does.
			return &ast.CallExpr{Fun: ident(goType), Args: []ast.Expr{arg}}, nil
		}
		return nil, &NotYetLowerable{Reason: "go: parameter of Go type " + goType + " is a later slice"}
	}
}

// marshalResultFromGo wraps a Go call's result in the crossing back to a bento
// value: a string transcodes through the bridge, a boolean passes through, a
// float-width number widens to a bento number, and a 64-bit integer goes through
// the bridge range check that turns a silent precision loss into a RangeError
// (section 7.5). A result type the slice does not cover hands back.
func (r *Renderer) marshalResultFromGo(goType string, goCall ast.Expr) (ast.Expr, error) {
	switch goType {
	case "string":
		r.requireImport(bridgePkg)
		return &ast.CallExpr{Fun: sel("bridge", "StringFromGo"), Args: []ast.Expr{goCall}}, nil
	case "bool":
		return goCall, nil
	case "float64":
		return goCall, nil
	case "float32", "int8", "int16", "int32", "uint8", "uint16", "uint32":
		// Every value of these types fits a float64 exactly, so the widening is a plain
		// conversion with no range check (section 6.2).
		return &ast.CallExpr{Fun: ident("float64"), Args: []ast.Expr{goCall}}, nil
	case "int64":
		r.requireImport(bridgePkg)
		return &ast.CallExpr{Fun: sel("bridge", "Int64ToNumber"), Args: []ast.Expr{goCall}}, nil
	case "uint64", "uintptr":
		r.requireImport(bridgePkg)
		return &ast.CallExpr{Fun: sel("bridge", "Uint64ToNumber"), Args: []ast.Expr{goCall}}, nil
	case "int":
		// int is 64-bit on the targets bento builds for, so it takes the same range
		// check as int64 after a widening conversion to the checked type.
		r.requireImport(bridgePkg)
		return &ast.CallExpr{
			Fun:  sel("bridge", "Int64ToNumber"),
			Args: []ast.Expr{&ast.CallExpr{Fun: ident("int64"), Args: []ast.Expr{goCall}}},
		}, nil
	case "uint":
		r.requireImport(bridgePkg)
		return &ast.CallExpr{
			Fun:  sel("bridge", "Uint64ToNumber"),
			Args: []ast.Expr{&ast.CallExpr{Fun: ident("uint64"), Args: []ast.Expr{goCall}}},
		}, nil
	default:
		return nil, &NotYetLowerable{Reason: "go: result of Go type " + goType + " is a later slice"}
	}
}

// goImportCallByType lowers a go: call from the TypeScript types alone, the path
// taken when no Go signature is wired. It covers the crossings a TypeScript type
// fixes without the Go type: a string through the bridge and a boolean unchanged.
// A number hands back, because the TypeScript number does not say whether the Go
// side wants an int, an int64, or a float64, and only the signature-driven path
// can marshal it soundly.
func (r *Renderer) goImportCallByType(b goBuiltin, call frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
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
		fromGo := &ast.CallExpr{Fun: sel("bridge", "StringFromGo"), Args: []ast.Expr{goCall}}
		return r.guardExpr(fromGo, "string"), nil
	case rt.Flags&frontend.TypeBoolean != 0:
		return r.guardExpr(goCall, "bool"), nil
	default:
		return nil, &NotYetLowerable{Reason: "go: call result of this type is a later slice"}
	}
}

// isGoNumeric reports whether a Go type keyword names a numeric basic type, the
// set marshalArgToGo converts a bento number into. It excludes float64, which a
// bento number already is, so the caller handles that with no conversion.
func isGoNumeric(goType string) bool {
	switch goType {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64", "uintptr",
		"float32", "float64":
		return true
	default:
		return false
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
