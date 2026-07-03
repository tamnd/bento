package lower

import (
	"go/ast"
	"go/token"
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
	marshaled, err := r.marshalResultFromGo(info.Keyword, "", read)
	if err != nil {
		return nil, false, err
	}
	return marshaled, true, nil
}

// goErrorSentinelRef resolves an argument that names a go: sentinel error variable
// to the qualified Go variable read, so a caught error's is() compares against the
// real Go value (section 7.7). Two reference forms resolve: a direct binding (EOF
// imported by name from go:io) and a namespace member (io.EOF where io is a
// namespace import). In both the resolver must confirm the bound name is an error
// variable, so a constant or a function used in the argument position hands back
// and the is() call falls through to its not-lowerable path rather than emit an
// unsound comparison. A nil resolver (no Go toolchain wired) reports not found.
func (r *Renderer) goErrorSentinelRef(arg frontend.Node) (ast.Expr, bool) {
	if r.goErrorVars == nil {
		return nil, false
	}
	var importPath, name string
	switch arg.Kind() {
	case frontend.NodeIdentifier:
		b, ok := r.goImports[r.prog.Text(arg)]
		if !ok {
			return nil, false
		}
		importPath, name = b.importPath, b.name
	case frontend.NodePropertyAccessExpression:
		b, ok := r.namespaceGoCall(arg)
		if !ok {
			return nil, false
		}
		importPath, name = b.importPath, b.name
	default:
		return nil, false
	}
	if !isGoIdent(name) || !r.goErrorVars(importPath, name) {
		return nil, false
	}
	return sel(r.requireGoImport(importPath), name), true
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
		return r.goImportCallBySig(b, sig, call, argNodes)
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
// arguments marshal through marshalCallArgs, which enforces the count the signature
// wants (exact for a fixed signature, at-least-the-fixed-parameters for a variadic
// one), and each argument's and the result's marshaling must be one the crossing
// supports, or the call hands back.
func (r *Renderer) goImportCallBySig(b goBuiltin, sig goimport.FuncSig, call frontend.Node, argNodes []frontend.Node) (ast.Expr, error) {
	args, err := r.marshalCallArgs(b, sig, argNodes)
	if err != nil {
		return nil, err
	}
	alias := r.requireGoImport(b.importPath)
	goCall := &ast.CallExpr{Fun: sel(alias, b.name), Args: args}
	resultElem := ""
	if len(sig.ResultElem) > 0 {
		resultElem = sig.ResultElem[0]
	}
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
		if sig.Results[0] == "struct" {
			return r.guardStructResult(resultElem, must, call)
		}
		if sig.Results[0] == "structslice" {
			return r.guardStructSliceResult(resultElem, must, call)
		}
		marshaled, err := r.marshalResultFromGo(sig.Results[0], resultElem, r.stripResultBrand(sig, must))
		if err != nil {
			return nil, err
		}
		return r.guardExpr(marshaled, sig.Results[0], resultElem), nil
	}
	if len(sig.Results) == 0 {
		// A Go function with no value result lowers to the bare call, valid where the
		// TypeScript call is used as a statement, which is the only place a void call
		// type-checks.
		return r.guardVoid(goCall), nil
	}
	if sig.Results[0] == "struct" {
		return r.guardStructResult(resultElem, goCall, call)
	}
	if sig.Results[0] == "structslice" {
		return r.guardStructSliceResult(resultElem, goCall, call)
	}
	marshaled, err := r.marshalResultFromGo(sig.Results[0], resultElem, r.stripResultBrand(sig, goCall))
	if err != nil {
		return nil, err
	}
	return r.guardExpr(marshaled, sig.Results[0], resultElem), nil
}

// guardStructResult lowers a go: call whose Go result is a struct into a read-only
// object box (sections 6.7 and 7.4). The struct crosses to the SAME interned Go
// struct the interface shape interns to, so property access on the bound variable
// resolves against one Go type. A closure binds the Go result to v and returns a
// pointer to a freshly built interned struct, reading each exported Go field and
// marshaling it by its keyword. The whole thing wraps in the boundary recover so a
// panic in the Go call reports as a thrown value.
func (r *Renderer) guardStructResult(elem string, goResult ast.Expr, call frontend.Node) (ast.Expr, error) {
	_, _, fields := goimport.SplitStructElem(elem)
	rt := r.prog.TypeAt(call)
	interned, err := r.decls.internStruct(r, rt)
	if err != nil {
		return nil, err
	}
	elts, err := r.structBoxElts(fields, "v")
	if err != nil {
		return nil, err
	}
	r.usesThrow = true
	r.requireImport(bridgePkg)
	retType := star(ident(interned))
	body := []ast.Stmt{
		&ast.AssignStmt{
			Lhs: []ast.Expr{ident("v")},
			Tok: token.DEFINE,
			Rhs: []ast.Expr{goResult},
		},
		&ast.ReturnStmt{Results: []ast.Expr{
			&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(interned), Elts: elts}},
		}},
	}
	return &ast.CallExpr{
		Fun: sel("bridge", "Guard"),
		Args: []ast.Expr{&ast.FuncLit{
			Type: &ast.FuncType{
				Params:  &ast.FieldList{},
				Results: &ast.FieldList{List: []*ast.Field{{Type: retType}}},
			},
			Body: &ast.BlockStmt{List: body},
		}},
	}, nil
}

// structBoxElts builds the composite-literal elements a struct crossing fills its
// interned box with: one keyed entry per exported field, reading the field off a Go
// struct value named recvName and marshaling it back to a bento value by its keyword
// (sections 6.7, 7.4). It is the field loop shared by the single-struct result
// (guardStructResult reads a bound Go result) and the struct-slice result
// (guardStructSliceResult reads each element of the slice), so both box a struct the
// same way. A field whose Go name is not a Go identifier hands back.
func (r *Renderer) structBoxElts(fields []goimport.StructField, recvName string) ([]ast.Expr, error) {
	elts := make([]ast.Expr, 0, len(fields))
	for _, f := range fields {
		read := &ast.SelectorExpr{X: ident(recvName), Sel: ident(f.Name)}
		val, err := r.marshalResultFromGo(f.Keyword, "", read)
		if err != nil {
			return nil, err
		}
		field, ok := exportedField(f.Name)
		if !ok {
			return nil, &NotYetLowerable{Reason: "go: struct field " + f.Name + " is not a Go identifier"}
		}
		elts = append(elts, &ast.KeyValueExpr{Key: ident(field), Value: val})
	}
	return elts, nil
}

// guardStructSliceResult lowers a go: call whose Go result is a slice of a struct,
// the []Point shape, into a bento array of read-only object boxes (sections 6.4,
// 6.7, 7.4). Each element crosses exactly as a single struct result does: a per
// element closure binds one Go struct value and returns a pointer to the interned
// struct the array's element type interns to, so property access on an element
// resolves against one Go type, the same type a lone struct result would intern to.
// bridge.SliceFromGo runs that closure over the slice and builds the bento array, and
// the whole thing wraps in the boundary recover so a panic in the Go call, or a
// per-element range check that throws, reports as a thrown value. This is the result
// direction: a []struct argument is a later slice.
func (r *Renderer) guardStructSliceResult(elem string, goSlice ast.Expr, call frontend.Node) (ast.Expr, error) {
	path, typeName, fields := goimport.SplitStructElem(elem)
	et, ok := r.prog.ElementType(r.prog.TypeAt(call))
	if !ok {
		return nil, &NotYetLowerable{Reason: "go: a []struct result must have an array element type"}
	}
	interned, err := r.decls.internStruct(r, et)
	if err != nil {
		return nil, err
	}
	elts, err := r.structBoxElts(fields, "v")
	if err != nil {
		return nil, err
	}
	r.usesThrow = true
	r.requireImport(bridgePkg)
	r.requireImport(valuePkg)
	alias := r.requireGoImport(path)
	internedPtr := star(ident(interned))
	// conv := func(v alias.Type) *interned { return &interned{...fields...} }
	conv := &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("v")}, Type: sel(alias, typeName)}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: internedPtr}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
			&ast.UnaryExpr{Op: token.AND, X: &ast.CompositeLit{Type: ident(interned), Elts: elts}},
		}}}},
	}
	fromGo := &ast.CallExpr{Fun: sel("bridge", "SliceFromGo"), Args: []ast.Expr{goSlice, conv}}
	arrType := star(index(sel("value", "Array"), internedPtr))
	return &ast.CallExpr{
		Fun: sel("bridge", "Guard"),
		Args: []ast.Expr{&ast.FuncLit{
			Type: &ast.FuncType{
				Params:  &ast.FieldList{},
				Results: &ast.FieldList{List: []*ast.Field{{Type: arrType}}},
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{fromGo}}}},
		}},
	}, nil
}

// structArgConv builds the closure a struct argument crosses through: func(o
// *interned) alias.Type { return alias.Type{...fields...} }, reading each exported
// field off the boxed object and marshaling it by its keyword into the matching Go
// field (section 6.7). The box is the interned struct the object's shape interns
// to, named by boxType, so the field the closure reads and the Go field it fills
// line up by name. The single-struct argument calls this closure directly on the
// box; the []struct argument hands it to bridge.SliceToGo to run over each element,
// so both marshal a struct the same way.
func (r *Renderer) structArgConv(elem string, boxType frontend.Type) (*ast.FuncLit, error) {
	path, typeName, fields := goimport.SplitStructElem(elem)
	interned, err := r.decls.internStruct(r, boxType)
	if err != nil {
		return nil, err
	}
	alias := r.requireGoImport(path)
	elts := make([]ast.Expr, 0, len(fields))
	for _, f := range fields {
		boxField, ok := exportedField(f.Name)
		if !ok {
			return nil, &NotYetLowerable{Reason: "go: struct field " + f.Name + " is not a Go identifier"}
		}
		read := &ast.SelectorExpr{X: ident("o"), Sel: ident(boxField)}
		val, err := r.marshalArgToGo(f.Keyword, "", goimport.DefinedConv{}, read, nil)
		if err != nil {
			return nil, err
		}
		elts = append(elts, &ast.KeyValueExpr{Key: ident(f.Name), Value: val})
	}
	goStruct := sel(alias, typeName)
	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("o")}, Type: star(ident(interned))}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: goStruct}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
			&ast.CompositeLit{Type: goStruct, Elts: elts},
		}}}},
	}, nil
}

// marshalCallArgs lowers and marshals each argument of a go: call against the Go
// signature, returning the positional Go arguments the call passes. A fixed
// signature demands an exact argument count. A variadic signature (section 6.9)
// requires at least the fixed parameters and marshals every argument past them by
// the element type the trailing entry carries, so a ...string call marshals each
// tail argument as one string and Go reassembles the slice; passing zero tail
// arguments is allowed and emits the call with only its fixed arguments.
func (r *Renderer) marshalCallArgs(b goBuiltin, sig goimport.FuncSig, argNodes []frontend.Node) ([]ast.Expr, error) {
	last := len(sig.Params) - 1
	if sig.Variadic {
		if len(argNodes) < last {
			return nil, &NotYetLowerable{Reason: "go: variadic call to " + b.name + " with fewer arguments than its required parameters"}
		}
	} else if len(argNodes) != len(sig.Params) {
		return nil, &NotYetLowerable{Reason: "go: call to " + b.name + " with a defaulted or spread argument is a later slice"}
	}
	args := make([]ast.Expr, 0, len(argNodes))
	for i, a := range argNodes {
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		// An argument in or past the variadic slot marshals by the trailing element entry;
		// a fixed argument marshals by its own parameter entry.
		p := i
		if sig.Variadic && i >= last {
			p = last
		}
		marshaled, err := r.marshalArgToGo(sig.Params[p], sig.ParamElem[p], sig.ParamConv[p], lowered, a)
		if err != nil {
			return nil, err
		}
		args = append(args, marshaled)
	}
	return args, nil
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
func (r *Renderer) guardExpr(expr ast.Expr, goResult, elem string) ast.Expr {
	r.usesThrow = true
	r.requireImport(bridgePkg)
	return &ast.CallExpr{
		Fun: sel("bridge", "Guard"),
		Args: []ast.Expr{
			&ast.FuncLit{
				Type: &ast.FuncType{
					Params:  &ast.FieldList{},
					Results: &ast.FieldList{List: []*ast.Field{{Type: r.bentoResultType(goResult, elem)}}},
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
func (r *Renderer) bentoResultType(goResult, elem string) ast.Expr {
	switch goResult {
	case "string":
		return sel("value", "BStr")
	case "bool":
		return ident("bool")
	case "slice":
		// A slice result crosses to a bento array whose element is the bento type its own
		// keyword names, the same header the array model uses for a TypeScript array
		// (section 6.4).
		r.requireImport(valuePkg)
		return star(index(sel("value", "Array"), r.bentoResultType(elem, "")))
	case "opaque":
		// An opaque handle is held as the uniform token type, so the guard closure returns
		// a bridge.Opaque whatever the foreign Go type is; the concrete type is named only
		// where the token crosses back into Go (section 6.13).
		r.requireImport(bridgePkg)
		return sel("bridge", "Opaque")
	case "any":
		// A Go any result crosses back to the boxed value.Value the dynamic world uses, so
		// the guard closure returns a value.Value; the unknown projection holds it directly
		// (section 6.12).
		r.requireImport(valuePkg)
		return sel("value", "Value")
	case "bytes":
		// A []byte result crosses to the value model's byte buffer, spelled as the
		// pointer a Uint8Array local is spelled (section 7.3), so the guard closure
		// returns a *value.Uint8Array.
		r.requireImport(valuePkg)
		return star(sel("value", "Uint8Array"))
	case "map":
		// A map result crosses to the value model's Map, spelled as the pointer a map
		// local is spelled, instantiated at the bento key and value types the crossing's
		// own keywords name (section 6.5).
		r.requireImport(valuePkg)
		keyKw, valKw := goimport.SplitMapElem(elem)
		return star(&ast.IndexListExpr{
			X:       sel("value", "Map"),
			Indices: []ast.Expr{r.bentoResultType(keyKw, ""), r.bentoResultType(valKw, "")},
		})
	default:
		return ident("float64")
	}
}

// bentoMapCtor builds the empty bento Map a map crossing back fills: the constructor
// its key kind fixes (a string key takes NewStringMap, a boolean NewBoolMap, and any
// numeric key NewNumberMap), instantiated at the bento value type the value keyword
// names. The bento Map carries a per-kind key equality the bridge cannot pick from
// the Go types alone, so the lowerer selects the constructor here and hands the empty
// map to bridge.MapFromGo to fill (section 6.5).
func (r *Renderer) bentoMapCtor(keyKw, valKw string) ast.Expr {
	r.requireImport(valuePkg)
	ctor := "NewNumberMap"
	switch keyKw {
	case "string":
		ctor = "NewStringMap"
	case "bool":
		ctor = "NewBoolMap"
	}
	return &ast.CallExpr{Fun: &ast.IndexExpr{X: sel("value", ctor), Index: r.bentoResultType(valKw, "")}}
}

// elemConv builds the per-element conversion closure a slice crossing hands to
// bridge.SliceToGo or bridge.SliceFromGo: a one-parameter function named over x that
// returns body, with the parameter and result types the crossing direction fixes
// (section 6.4). The body is the element's own scalar crossing applied to x, so a
// slice reuses the exact marshaling a single element would take.
func (r *Renderer) elemConv(paramType, resultType, body ast.Expr) *ast.FuncLit {
	return &ast.FuncLit{
		Type: &ast.FuncType{
			Params:  &ast.FieldList{List: []*ast.Field{{Names: []*ast.Ident{ident("x")}, Type: paramType}}},
			Results: &ast.FieldList{List: []*ast.Field{{Type: resultType}}},
		},
		Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{body}}}},
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
func (r *Renderer) marshalArgToGo(goType, elem string, conv goimport.DefinedConv, arg ast.Expr, argNode frontend.Node) (ast.Expr, error) {
	if goType == "slice" {
		// A bento array crosses to a Go slice element by element: the emitted closure
		// applies the element's own crossing to each element, and bridge.SliceToGo runs
		// it over the array (section 6.4). The closure parameter is the bento element
		// type and its result the Go element type, the inverse of the result path. A
		// slice element is always a plain basic, never an any, so the element crossing
		// needs no argument node.
		r.requireImport(bridgePkg)
		body, err := r.marshalArgToGo(elem, "", goimport.DefinedConv{}, ident("x"), nil)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{
			Fun:  sel("bridge", "SliceToGo"),
			Args: []ast.Expr{arg, r.elemConv(r.bentoResultType(elem, ""), ident(elem), body)},
		}, nil
	}
	if goType == "map" {
		// A bento Map crosses to a Go map entry by entry: bridge.MapToGo iterates the map
		// once and applies a key crossing and a value crossing to each entry, each the
		// same scalar crossing a single key or value would take (section 6.5). The two
		// closures take the bento key and value types and return the Go key and value
		// types, the inverse of the result path. A map key and value are always plain
		// basics in this slice, never an any, so neither crossing needs an argument node.
		r.requireImport(bridgePkg)
		keyKw, valKw := goimport.SplitMapElem(elem)
		keyBody, err := r.marshalArgToGo(keyKw, "", goimport.DefinedConv{}, ident("x"), nil)
		if err != nil {
			return nil, err
		}
		valBody, err := r.marshalArgToGo(valKw, "", goimport.DefinedConv{}, ident("x"), nil)
		if err != nil {
			return nil, err
		}
		keyConv := r.elemConv(r.bentoResultType(keyKw, ""), ident(keyKw), keyBody)
		valConv := r.elemConv(r.bentoResultType(valKw, ""), ident(valKw), valBody)
		return &ast.CallExpr{Fun: sel("bridge", "MapToGo"), Args: []ast.Expr{arg, keyConv, valConv}}, nil
	}
	if goType == "struct" {
		// A bento object crosses into a Go struct parameter field by field: a closure
		// binds the boxed object and returns a Go struct literal, reading each exported
		// field off the box and marshaling it by its keyword into the matching Go field
		// (section 6.7). The box is the same interned struct the object's shape interns
		// to, so its field the closure reads and the Go field it fills line up by name.
		// The struct crosses only as a top-level argument, never inside a composite, so a
		// missing argument node (which only a recursive composite element passes) hands
		// back.
		if argNode == nil {
			return nil, &NotYetLowerable{Reason: "go: a struct argument must be a top-level argument"}
		}
		conv, err := r.structArgConv(elem, r.prog.TypeAt(argNode))
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: conv, Args: []ast.Expr{arg}}, nil
	}
	if goType == "structslice" {
		// A bento array of objects crosses into a Go slice of structs element by element,
		// the inverse of the []struct result: bridge.SliceToGo runs the same per-object
		// closure the single-struct argument builds over the array and collects the Go
		// structs into a fresh slice (sections 6.4, 6.7). The element boxes are the same
		// interned struct the array's element type interns to, so the closure reads and
		// fills fields by name exactly as a lone struct argument would. The array crosses
		// only as a top-level argument, so a missing argument node hands back.
		if argNode == nil {
			return nil, &NotYetLowerable{Reason: "go: a []struct argument must be a top-level argument"}
		}
		et, ok := r.prog.ElementType(r.prog.TypeAt(argNode))
		if !ok {
			return nil, &NotYetLowerable{Reason: "go: a []struct argument must have an array element type"}
		}
		conv, err := r.structArgConv(elem, et)
		if err != nil {
			return nil, err
		}
		r.requireImport(bridgePkg)
		return &ast.CallExpr{Fun: sel("bridge", "SliceToGo"), Args: []ast.Expr{arg, conv}}, nil
	}
	if goType == "any" {
		// A Go any parameter takes a boxed bento value: bridge.AnyToGo unwraps a scalar to
		// its Go native and passes a reference value through as its value.Value box
		// (section 6.12). The argument's own static type, not the any parameter, fixes the
		// lowered Go type, so a statically typed argument (a number literal into an any
		// slot) is boxed to a value.Value first, while an already-dynamic argument is a
		// value.Value and passes straight in.
		r.requireImport(bridgePkg)
		boxed := arg
		if argNode != nil && !r.isDynamic(argNode) {
			b, err := r.boxStaticToDynamic(arg, argNode)
			if err != nil {
				return nil, err
			}
			boxed = b
		}
		return &ast.CallExpr{Fun: sel("bridge", "AnyToGo"), Args: []ast.Expr{boxed}}, nil
	}
	if goType == "bytes" {
		// A Uint8Array crosses into a Go []byte parameter through bridge.BytesToGo, which
		// copies the buffer into a fresh Go slice so a callee that retains or mutates the
		// slice past the call can never alias bento's buffer (section 7.3). The zero-copy
		// BytesToGoShared is sound only for a callee proven to read the bytes within the
		// call, a proof a later slice supplies; the safe copying form is the default here.
		r.requireImport(bridgePkg)
		return &ast.CallExpr{Fun: sel("bridge", "BytesToGo"), Args: []ast.Expr{arg}}, nil
	}
	if goType == "opaque" {
		// An opaque handle crosses back into Go by recovering the real value the token
		// holds: bento held it as a bridge.Opaque, and OpaqueToGo asserts it back to the
		// concrete foreign type the emitted call names as its type argument (section 6.13).
		// The element string packs that type's import path and Go name.
		r.requireImport(bridgePkg)
		path, name := goimport.SplitOpaqueElem(elem)
		return &ast.CallExpr{
			Fun:  &ast.IndexExpr{X: sel("bridge", "OpaqueToGo"), Index: sel(r.requireGoImport(path), name)},
			Args: []ast.Expr{arg},
		}, nil
	}
	if goType == "func" {
		// A bento function crosses into a Go func parameter as a wrapper closure the Go
		// library calls: the wrapper takes the Go parameter types, marshals each Go
		// argument to the bento value the callback expects, invokes the bento function,
		// and marshals its result back to the Go return type (section 7.6). The callback
		// runs inline on the call, which is the synchronous case a Go API like a Map
		// helper or filepath.Walk takes; the cross-goroutine hand-off onto the loop is a
		// later slice tied to the event loop. The bento function is the lowered argument
		// itself, called as a literal, so a callback argument must be a real function
		// expression and hands back if it arrives inside a composite.
		if argNode == nil {
			return nil, &NotYetLowerable{Reason: "go: a callback argument must be a top-level argument"}
		}
		resultKw, paramKws := goimport.SplitFuncElem(elem)
		fields := make([]*ast.Field, 0, len(paramKws))
		callArgs := make([]ast.Expr, 0, len(paramKws))
		for i, pk := range paramKws {
			name := "p" + strconv.Itoa(i)
			fields = append(fields, &ast.Field{Names: []*ast.Ident{ident(name)}, Type: ident(pk)})
			// The Go argument crosses into the bento value the callback's parameter holds,
			// the same Go-to-bento crossing a result takes, so the callback body sees a
			// bento number, string, or boolean exactly as it would from any other source.
			bento, err := r.marshalResultFromGo(pk, "", ident(name))
			if err != nil {
				return nil, err
			}
			callArgs = append(callArgs, bento)
		}
		inner := &ast.CallExpr{Fun: arg, Args: callArgs}
		if resultKw == "" {
			// A void callback is called for its effect: the wrapper has no result and the
			// bento call stands as a statement.
			return &ast.FuncLit{
				Type: &ast.FuncType{Params: &ast.FieldList{List: fields}},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: inner}}},
			}, nil
		}
		if resultKw == "error" {
			// A throwing callback returns error to Go: the bento callback returns nothing,
			// so the wrapper runs it inside bridge.CallbackError, which returns nil when it
			// returns normally and the thrown value converted to a Go error when it throws
			// (section 7.6). The Go library calling the wrapper sees the failure as its own
			// error return, the inverse of the (T, error) result crossing.
			r.requireImport(bridgePkg)
			runner := &ast.FuncLit{
				Type: &ast.FuncType{Params: &ast.FieldList{}},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ExprStmt{X: inner}}},
			}
			return &ast.FuncLit{
				Type: &ast.FuncType{
					Params:  &ast.FieldList{List: fields},
					Results: &ast.FieldList{List: []*ast.Field{{Type: ident("error")}}},
				},
				Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{
					&ast.CallExpr{Fun: sel("bridge", "CallbackError"), Args: []ast.Expr{runner}},
				}}}},
			}, nil
		}
		// The bento result crosses back to the Go return type, the same bento-to-Go
		// crossing an argument takes, so a Go caller sees the return type it declared.
		goRet, err := r.marshalArgToGo(resultKw, "", goimport.DefinedConv{}, inner, nil)
		if err != nil {
			return nil, err
		}
		return &ast.FuncLit{
			Type: &ast.FuncType{
				Params:  &ast.FieldList{List: fields},
				Results: &ast.FieldList{List: []*ast.Field{{Type: ident(resultKw)}}},
			},
			Body: &ast.BlockStmt{List: []ast.Stmt{&ast.ReturnStmt{Results: []ast.Expr{goRet}}}},
		}, nil
	}
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
func (r *Renderer) marshalResultFromGo(goType, elem string, goCall ast.Expr) (ast.Expr, error) {
	if goType == "slice" {
		// A Go slice crosses back to a bento array element by element: the emitted
		// closure applies the element's own crossing to each element, and
		// bridge.SliceFromGo runs it over the slice (section 6.4). The closure parameter
		// is the Go element type and its result the bento element type.
		r.requireImport(bridgePkg)
		body, err := r.marshalResultFromGo(elem, "", ident("x"))
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{
			Fun:  sel("bridge", "SliceFromGo"),
			Args: []ast.Expr{goCall, r.elemConv(ident(elem), r.bentoResultType(elem, ""), body)},
		}, nil
	}
	if goType == "map" {
		// A Go map crosses back to a bento Map entry by entry: bridge.MapFromGo ranges the
		// Go map and applies a key crossing and a value crossing to each entry, filling the
		// empty bento Map its key kind fixes (section 6.5). The two closures take the Go key
		// and value types and return the bento key and value types. Go map iteration order
		// is unspecified, so the bento Map's order after the crossing is unspecified too,
		// which the Map contract allows.
		r.requireImport(bridgePkg)
		keyKw, valKw := goimport.SplitMapElem(elem)
		keyBody, err := r.marshalResultFromGo(keyKw, "", ident("x"))
		if err != nil {
			return nil, err
		}
		valBody, err := r.marshalResultFromGo(valKw, "", ident("x"))
		if err != nil {
			return nil, err
		}
		keyConv := r.elemConv(ident(keyKw), r.bentoResultType(keyKw, ""), keyBody)
		valConv := r.elemConv(ident(valKw), r.bentoResultType(valKw, ""), valBody)
		return &ast.CallExpr{
			Fun:  sel("bridge", "MapFromGo"),
			Args: []ast.Expr{goCall, r.bentoMapCtor(keyKw, valKw), keyConv, valConv},
		}, nil
	}
	if goType == "bytes" {
		// A Go []byte result crosses back through bridge.BytesFromGo, which copies the Go
		// slice into a fresh bento buffer so a callee that keeps the slice and mutates it
		// after return cannot change bytes the bento program now owns (section 7.3). The
		// zero-copy BytesFromGoShared is the fast path a later slice reaches for once it
		// can prove Go will not mutate the slice; the safe copying form is the default.
		r.requireImport(bridgePkg)
		return &ast.CallExpr{Fun: sel("bridge", "BytesFromGo"), Args: []ast.Expr{goCall}}, nil
	}
	if goType == "opaque" {
		// An opaque handle crosses back as a token: bento never inspects it, so the Go
		// result is boxed into a bridge.Opaque that holds the value and keeps it alive
		// while the bento program holds the token (section 6.13). The token has one Go
		// type whatever the foreign type is, so a local that holds it needs no per-type
		// declaration.
		r.requireImport(bridgePkg)
		return &ast.CallExpr{Fun: sel("bridge", "OpaqueFromGo"), Args: []ast.Expr{goCall}}, nil
	}
	if goType == "any" {
		// A Go any result crosses back to a boxed bento value: bridge.AnyFromGo unboxes a
		// value the value model represents to its bento kind and passes a value.Value that
		// round-tripped through a Go container back unchanged (section 6.12). The result
		// projects to unknown, so the bento side holds it as the value.Value the dynamic
		// world already uses.
		r.requireImport(bridgePkg)
		return &ast.CallExpr{Fun: sel("bridge", "AnyFromGo"), Args: []ast.Expr{goCall}}, nil
	}
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
		return r.guardExpr(fromGo, "string", ""), nil
	case rt.Flags&frontend.TypeBoolean != 0:
		return r.guardExpr(goCall, "bool", ""), nil
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
