package lower

import (
	"go/ast"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/goimport"
)

// This file lowers the small node:fs, node:os, and node:path surface a syscall
// workload uses (the readwrite benchmark) to the value package's host helpers.
// A node: import binds a set of local names to module functions; the frontend
// resolves each binding to an alias symbol, so a call to one of those names is
// not a call to a user function. The renderer records the bindings once from the
// entry module's import declarations, then routes a call to a bound name to the
// matching value helper (value.WriteFileSync and its siblings), which do the
// syscall directly through the Go standard library. The surface is exactly what
// aot_ambient.go declares and value/nodefs.go implements: a declared name is a
// lowerable one, and an import of anything outside this set hands back so the
// unit routes to the engine.

// nodeBuiltin names one imported node: builtin by its module and its original
// exported name, the pair the renderer dispatches on. The original name is kept
// rather than the local binding so an aliased import (import { join as j }) still
// dispatches on join, the function it actually names.
type nodeBuiltin struct {
	module string
	name   string
}

// nodeModuleExports lists, per supported node module, the exported names bento
// lowers. A named import of a listed name binds to the builtin; a named import of
// any other name from the module, or an import of an unsupported module, hands
// back. Keeping the set explicit here keeps it in lockstep with the ambient
// declarations and the value helpers, so the three never drift.
var nodeModuleExports = map[string]map[string]bool{
	"node:fs": {
		"mkdtempSync":   true,
		"writeFileSync": true,
		"readFileSync":  true,
		"rmSync":        true,
	},
	"node:os":   {"tmpdir": true},
	"node:path": {"join": true},
}

// collectNodeImports records every node: import binding in the entry module into
// r.nodeImports before any body is lowered. It walks the module's top-level
// import declarations, and for each one it recognizes a supported node module and
// its named imports, mapping each local binding to the builtin it names. An
// import bento does not lower (an unsupported module, a default or namespace
// import, an aliased-away shape, a listed module with an unlisted name) is a
// NotYetLowerable so the whole unit routes to the engine rather than compiling a
// call to a helper that does not exist.
func (r *Renderer) collectNodeImports(entry frontend.Node) error {
	internal := r.internalImports(entry)
	for _, stmt := range r.prog.Children(entry) {
		if stmt.Kind() != frontend.NodeUnknown {
			continue
		}
		if !strings.HasPrefix(strings.TrimSpace(r.prog.Text(stmt)), "import") {
			continue
		}
		if err := r.recordNodeImport(stmt, internal); err != nil {
			return err
		}
	}
	return nil
}

// internalImports returns the set of module specifiers this file statically
// imports that resolve to another source file in the composed program, the
// sibling modules the build staged and lowered alongside the entry. A binding
// imported from one of these resolves to that module's package-level Go
// declaration, so the import records nothing and each reference lowers to that
// name directly, the way a reference to a top-level function in the same file
// does. A relative specifier that resolves to a declaration file or to nothing is
// not one of these; it stays with the node: and go: paths, which decline it.
func (r *Renderer) internalImports(file frontend.Node) map[string]bool {
	out := map[string]bool{}
	for _, imp := range r.prog.Imports(file.File()) {
		if imp.Kind != frontend.ImportRelative {
			continue
		}
		if imp.Resolved.Path == "" || imp.Resolved.Kind == frontend.FileDTS {
			continue
		}
		out[imp.Specifier] = true
	}
	return out
}

// recordNodeImport parses one import declaration and records its bindings. The
// declaration's children are the import clause and the module specifier string
// literal; the specifier's text, unquoted, is the module. Only the supported
// modules lower, and within them only the named-import form: a default or
// namespace import has no named-imports node to walk, so it hands back. Each
// import specifier's identifier children are the exported name and, when the
// import is aliased, the local binding; the first is the export bento dispatches
// on and the last is the local name a call site uses.
func (r *Renderer) recordNodeImport(decl frontend.Node, internal map[string]bool) error {
	kids := r.prog.Children(decl)
	var module string
	var clause frontend.Node
	haveClause := false
	for _, k := range kids {
		switch k.Kind() {
		case frontend.NodeStringLiteral:
			module = unquote(r.prog.Text(k))
		case frontend.NodeUnknown:
			clause, haveClause = k, true
		}
	}
	// A sibling module the build composed into this unit exposes its exports as
	// package-level Go declarations, so an import of one binds each name to a Go
	// name already in scope and records nothing here, the way a reference to a
	// top-level function in the same file needs no recording.
	if internal[module] {
		return r.recordInternalImport(module, clause, haveClause)
	}
	// A go: specifier is a Go interop import, not a node: builtin, so it routes to
	// the interop recorder, which maps its bindings to direct Go calls. It is handled
	// here, in the one import walk, so a module mixing node: and go: imports records
	// both from a single pass.
	if strings.HasPrefix(module, goScheme) {
		return r.recordGoImport(module, clause, haveClause)
	}
	// The bento:go vocabulary module (section 5.2) declares the TypeScript types that
	// model Go concepts the language lacks. An import from it carries only compile-time
	// meaning: GoError names the class a catch narrows to with instanceof, which lowers
	// through the caught-error path by name rather than through a recorded binding, and
	// the rest are types with no runtime value. So the import records nothing and lowers
	// to nothing, the way a type-only import does; a bento:go runtime helper used as a
	// value still hands back at its call site until that helper lowers.
	if module == goimport.VocabularyModule {
		return nil
	}
	exports, ok := nodeModuleExports[module]
	if !ok {
		return &NotYetLowerable{Reason: "import of module " + module + " is a later slice"}
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
		if !exports[exported] {
			return &NotYetLowerable{Reason: "import of " + exported + " from " + module + " is a later slice"}
		}
		r.nodeImports[local] = nodeBuiltin{module: module, name: exported}
	}
	return nil
}

// recordInternalImport handles an import from a sibling module the build composed
// into the same unit. A named import binds each name to the sibling's
// package-level Go declaration, which carries the same Go spelling the binding
// takes there, so the import records nothing and each reference lowers to that
// name directly. The forms this slice does not lower hand back so the whole unit
// routes to the engine rather than emit a reference with no target: a bare
// side-effect import (whose module evaluation order the composed unit would have
// to preserve), a default or namespace import, and an aliased import (whose local
// name differs from the exported one, so the reference would not spell the
// declaration's Go name).
func (r *Renderer) recordInternalImport(module string, clause frontend.Node, haveClause bool) error {
	if !haveClause {
		return &NotYetLowerable{Reason: "a bare side-effect import of a sibling module is a later slice"}
	}
	named, ok := namedImportsNode(r.prog, clause)
	if !ok {
		return &NotYetLowerable{Reason: "a default or namespace import of a sibling module is a later slice"}
	}
	for _, spec := range r.prog.Children(named) {
		names := identChildren(r.prog, spec)
		if len(names) >= 2 && names[0] != names[len(names)-1] {
			return &NotYetLowerable{Reason: "an aliased import of a sibling module is a later slice"}
		}
	}
	return nil
}

// namedImportsNode descends an import clause to its named-imports node, the brace
// list whose children are the import specifiers. The clause of a named import
// wraps a single named-imports child; a default or namespace import has a
// different shape with no such child, which is reported as not found so the
// caller hands back.
func namedImportsNode(prog *frontend.Program, clause frontend.Node) (frontend.Node, bool) {
	// The clause node and the named-imports node both render as "{ ... }"; the
	// named-imports node is the descendant whose own children are the specifiers.
	// A default binding is an identifier child of the clause, and a namespace
	// binding is a "* as name" child, neither of which is an Unknown brace node, so
	// walking to the first Unknown child that itself has Unknown children lands on
	// the specifier list precisely.
	for _, c := range prog.Children(clause) {
		if c.Kind() != frontend.NodeUnknown {
			continue
		}
		specs := prog.Children(c)
		if len(specs) == 0 {
			continue
		}
		allSpecs := true
		for _, s := range specs {
			if s.Kind() != frontend.NodeUnknown {
				allSpecs = false
				break
			}
		}
		if allSpecs {
			return c, true
		}
	}
	return nil, false
}

// identChildren returns the identifier names directly under a node, the exported
// name and optional local alias of an import specifier in source order.
func identChildren(prog *frontend.Program, n frontend.Node) []string {
	var out []string
	for _, c := range prog.Children(n) {
		if c.Kind() == frontend.NodeIdentifier {
			out = append(out, prog.Text(c))
		}
	}
	return out
}

// unquote strips one layer of surrounding quotes from a string-literal's source
// text, turning "node:fs" into node:fs. The checker guarantees the literal is
// well formed, so a simple trim of the matching first and last byte is enough;
// the module names bento matches contain no escapes.
func unquote(s string) string {
	if len(s) >= 2 {
		q := s[0]
		if (q == '"' || q == '\'' || q == '`') && s[len(s)-1] == q {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// nodeBuiltinCall lowers a call to an imported node: builtin to its value helper.
// Each builtin has a fixed shape the ambient declaration pins, so the arguments
// are checked against that shape before lowering: a string path and data for a
// write, a variadic list of string segments for a join, an options object read
// at compile time for a remove. A shape the helper does not accept hands back
// rather than emitting a mistyped call.
func (r *Renderer) nodeBuiltinCall(b nodeBuiltin, argNodes []frontend.Node) (ast.Expr, error) {
	switch b.module + "." + b.name {
	case "node:os.tmpdir":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "os.tmpdir takes no arguments"}
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "Tmpdir")}, nil

	case "node:path.join":
		args, err := r.stringArgs("path.join", argNodes)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PathJoin"), Args: args}, nil

	case "node:fs.mkdtempSync":
		args, err := r.stringArgsN("fs.mkdtempSync", argNodes, 1)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "Mkdtemp"), Args: args}, nil

	case "node:fs.writeFileSync":
		args, err := r.stringArgsN("fs.writeFileSync", argNodes, 2)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "WriteFileSync"), Args: args}, nil

	case "node:fs.readFileSync":
		// The encoding argument is fixed to "utf8" by the ambient declaration, so the
		// checker has already proven the second argument is that literal; the helper
		// bakes the encoding in, so only the path is lowered and the encoding argument
		// is dropped.
		if len(argNodes) != 2 {
			return nil, &NotYetLowerable{Reason: "fs.readFileSync with this argument count is a later slice"}
		}
		if !r.isString(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "fs.readFileSync of a non-string path is a later slice"}
		}
		path, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "ReadFileSyncUTF8"), Args: []ast.Expr{path}}, nil

	case "node:fs.rmSync":
		return r.rmSyncCall(argNodes)

	default:
		return nil, &NotYetLowerable{Reason: b.module + "." + b.name + " is a later slice"}
	}
}

// rmSyncCall lowers fs.rmSync(path, options?). The path is a string, and the
// options are read at compile time: the second argument, when present, must be an
// object literal whose recursive and force properties are boolean literals, the
// shape the ambient declaration types and a cleanup call passes. The two flags
// become plain boolean arguments to value.RmSync, so the recursion and the
// force-missing behavior are decided at lower time and cost nothing at runtime.
func (r *Renderer) rmSyncCall(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) < 1 || len(argNodes) > 2 {
		return nil, &NotYetLowerable{Reason: "fs.rmSync with this argument count is a later slice"}
	}
	if !r.isString(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "fs.rmSync of a non-string path is a later slice"}
	}
	path, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	recursive, force := false, false
	if len(argNodes) == 2 {
		recursive, force, err = r.rmOptions(argNodes[1])
		if err != nil {
			return nil, err
		}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun:  sel("value", "RmSync"),
		Args: []ast.Expr{path, boolLit(recursive), boolLit(force)},
	}, nil
}

// rmOptions reads the recursive and force flags from an rmSync options object at
// compile time. The argument must be an object literal, and each of its members
// must be a plain recursive or force property whose value is a boolean literal;
// anything else (a spread, a computed key, a non-literal value, an unknown key)
// hands back, because the flag then depends on runtime data this slice does not
// thread into the call.
func (r *Renderer) rmOptions(n frontend.Node) (recursive, force bool, err error) {
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return false, false, &NotYetLowerable{Reason: "fs.rmSync options that are not an object literal are a later slice"}
	}
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return false, false, &NotYetLowerable{Reason: "fs.rmSync options with a non-property member are a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return false, false, &NotYetLowerable{Reason: "fs.rmSync option that is not a simple property is a later slice"}
		}
		val, ok := boolLiteralValue(kids[1])
		if !ok {
			return false, false, &NotYetLowerable{Reason: "fs.rmSync option value that is not a boolean literal is a later slice"}
		}
		switch r.prog.Text(kids[0]) {
		case "recursive":
			recursive = val
		case "force":
			force = val
		default:
			return false, false, &NotYetLowerable{Reason: "fs.rmSync option " + r.prog.Text(kids[0]) + " is a later slice"}
		}
	}
	return recursive, force, nil
}

// stringArgs lowers a variadic list of string arguments, the shape path.join
// takes. Every argument must type as a string, so a number or an object argument
// hands back rather than lowering to a value.PathJoin call whose Go signature
// would reject it.
func (r *Renderer) stringArgs(what string, argNodes []frontend.Node) ([]ast.Expr, error) {
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		if !r.isString(a) {
			return nil, &NotYetLowerable{Reason: what + " with a non-string argument is a later slice"}
		}
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		args = append(args, lowered)
	}
	return args, nil
}

// stringArgsN lowers exactly n string arguments, the shape the fixed-arity fs
// helpers take. A different count, or a non-string argument, hands back.
func (r *Renderer) stringArgsN(what string, argNodes []frontend.Node, n int) ([]ast.Expr, error) {
	if len(argNodes) != n {
		return nil, &NotYetLowerable{Reason: what + " with this argument count is a later slice"}
	}
	return r.stringArgs(what, argNodes)
}

// boolLiteralValue returns the value of a boolean-literal node, and whether the
// node is one. It is the compile-time read the options objects rely on, so a
// runtime boolean expression is not one and the caller hands back.
func boolLiteralValue(n frontend.Node) (bool, bool) {
	switch n.Kind() {
	case frontend.NodeTrueKeyword:
		return true, true
	case frontend.NodeFalseKeyword:
		return false, true
	default:
		return false, false
	}
}

// boolLit builds the Go true or false identifier for a compile-time-known flag.
func boolLit(v bool) ast.Expr {
	if v {
		return ident("true")
	}
	return ident("false")
}
