package lower

import (
	"errors"
	"go/ast"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
	"github.com/tamnd/bento/pkg/goimport"
)

// This file lowers the small node:fs, node:os, node:path and node:util surface a
// syscall workload uses (the readwrite benchmark) to the value package's host helpers.
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
	"node:os": {
		"tmpdir":               true,
		"platform":             true,
		"arch":                 true,
		"type":                 true,
		"release":              true,
		"version":              true,
		"machine":              true,
		"hostname":             true,
		"homedir":              true,
		"endianness":           true,
		"totalmem":             true,
		"freemem":              true,
		"uptime":               true,
		"availableParallelism": true,
		"EOL":                  true,
		"devNull":              true,
	},
	"node:util": {
		"format":            true,
		"formatWithOptions": true,
		"inspect":           true,
	},
	"node:path": {
		"sep":              true,
		"delimiter":        true,
		"join":             true,
		"resolve":          true,
		"normalize":        true,
		"dirname":          true,
		"basename":         true,
		"extname":          true,
		"isAbsolute":       true,
		"relative":         true,
		"toNamespacedPath": true,
	},
}

// osHelpers maps a node:os export to the value helper that answers it. Every one
// of them takes no arguments and reports one fact about the machine, so the
// lowering is the same for all of them and only the name differs.
//
// The exports missing from this map are the ones that answer with an object
// rather than a string or a number: os.cpus, os.networkInterfaces, os.userInfo,
// os.loadavg and os.constants. An import of one of those hands back and the unit
// routes to the engine, which still answers it correctly.
var osHelpers = map[string]string{
	"tmpdir":               "Tmpdir",
	"platform":             "OSPlatform",
	"arch":                 "OSArch",
	"type":                 "OSType",
	"release":              "OSRelease",
	"version":              "OSVersion",
	"machine":              "OSMachine",
	"hostname":             "OSHostname",
	"homedir":              "OSHomedir",
	"endianness":           "OSEndianness",
	"totalmem":             "OSTotalmem",
	"freemem":              "OSFreemem",
	"uptime":               "OSUptime",
	"availableParallelism": "OSAvailableParallelism",
}

// pathHelpers maps a node:path export to the value helper that implements it, for
// the entries whose lowering differs only in the name called. The helpers are a
// port of Node's own path module rather than a wrapper over path/filepath, so the
// mapping is one to one and the argument order is Node's.
var pathHelpers = map[string]string{
	"join":             "PathJoin",
	"resolve":          "PathResolve",
	"normalize":        "PathNormalize",
	"dirname":          "PathDirname",
	"extname":          "PathExtname",
	"isAbsolute":       "PathIsAbsolute",
	"toNamespacedPath": "PathToNamespacedPath",
}

// nodeModuleConstants lists, per module, the exports that are values rather than
// functions, and the value helper each one reads through. They are the module's
// data members, path.sep and path.delimiter, os.EOL and os.devNull, and they are
// read rather than called, so they take their own path through the lowerer: a
// bare reference to one is the value itself, where a bare reference to an
// exported function is a function value bento has nothing to hand out for.
//
// Each is a helper call and not a compile-time literal, because the value is a
// fact about the platform the program is built for, which the compiler running on
// a Mac cannot decide for a Windows target. The helper is compiled into the
// target binary and answers there.
var nodeModuleConstants = map[string]map[string]string{
	"node:os": {
		"EOL":     "OSEOL",
		"devNull": "OSDevNull",
	},
	"node:path": {
		"sep":       "PathSep",
		"delimiter": "PathDelimiter",
	},
}

// nodeConstantRead lowers a read of one of a node module's data exports to the
// value helper that answers it, and reports whether the name is one. Both the
// member read (path.sep) and the bare read of a named import (import { sep })
// come through here, so the two forms cannot answer differently.
func (r *Renderer) nodeConstantRead(module, name string) (ast.Expr, bool) {
	helper, ok := nodeModuleConstants[module][name]
	if !ok {
		return nil, false
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", helper)}, true
}

// collectNodeImports records every node: import binding in the entry module into
// r.nodeImports before any body is lowered. It walks the module's top-level
// import declarations, and for each one it recognizes a supported node module and
// its named imports, mapping each local binding to the builtin it names. An
// import bento does not lower (an unsupported module, a listed module with an
// unlisted name, a bare side-effect import) is a NotYetLowerable so the whole unit
// routes to the engine rather than compiling a call to a helper that does not
// exist.
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
// modules lower, in all three of the forms that bind something: a named list binds
// each name to the builtin it names, and a default or namespace clause binds one
// name to the whole module, which a member call resolves through. Each import
// specifier's identifier children are the exported name and, when the import is
// aliased, the local binding; the first is the export bento dispatches on and the
// last is the local name a call site uses.
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
	// A default or namespace import binds the whole module rather than picking names
	// out of it. Both bind the same thing here: node's core modules are CommonJS, so
	// the default export is the module object, which is what import * as binds too.
	// Recording either as a namespace lets a member call on the binding reach the
	// same dispatch a named import's binding reaches, which is what makes the form
	// most Node programs are actually written in compile.
	//
	// A mixed clause, import path, { join } from "node:path", carries both, so the
	// module-object binding is recorded before the named list is walked rather than
	// instead of it.
	bindings := moduleObjectBindings(r.prog, clause)
	for _, binding := range bindings {
		r.nodeNamespaces[binding] = module
	}
	named, ok := namedImportsNode(r.prog, clause)
	if !ok {
		if len(bindings) > 0 {
			return nil
		}
		return &NotYetLowerable{Reason: "import of " + module + " in this form is a later slice"}
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
// into the same unit. A named, aliased, or default import binds each name to the
// sibling's package-level Go declaration, which carries the same Go spelling the
// binding takes there, so the import records nothing and each reference lowers to
// that name directly: the call path and the value-read path both deref the import
// alias to the export it names, so a named import spells the exported name, an
// aliased import (import { area as f }) recovers the same exported Go name from the
// alias rather than the local f, and a default import the Default name bento gives a
// sibling's default export. A non-function export a reference cannot spell this way,
// a const or class, hands back at the export side (its top-level statement is not
// composed), so the reference never reaches a successful build; recording nothing
// here is safe for every named form. Only a bare side-effect import, whose module
// evaluation order the composed unit would have to preserve, hands back, and a
// namespace import records its binding for member resolution.
func (r *Renderer) recordInternalImport(module string, clause frontend.Node, haveClause bool) error {
	if !haveClause {
		return &NotYetLowerable{Reason: "a bare side-effect import of a sibling module is a later slice"}
	}
	// A namespace import binds the sibling's whole exported surface to one name. Each
	// export is already a package-level Go declaration, so the binding needs no runtime
	// struct: the name is recorded here, and a member call on it (m.inc(1)) or a member
	// value read (const f = m.inc) resolves to the export's Go name at its site. The
	// binding used as a first-class value still hands back at its site, since a
	// namespace passed around as a whole would need the struct this slice does not build.
	if binding, ok := namespaceBinding(r.prog, clause); ok {
		r.internalNamespaces[binding] = true
		return nil
	}
	// A named, aliased, default, or mixed import records nothing; each reference
	// resolves to the export's Go name by deref-ing the import alias at its site.
	return nil
}

// namespaceNodeCall resolves a member callee against the module-object bindings of
// the node: imports: when the callee is name.member and name stands for a whole
// node module, it returns the nodeBuiltin for member in that module, the same pair
// a named import of member binds. So path.join(a, b) and the join(a, b) of import
// { join } meet at one dispatch and cannot drift apart.
//
// A member the module does not export, or one bento does not lower yet, is
// reported as an error rather than as not-found: the binding is known to name a
// node module, so falling through to the method-call path would blame a receiver
// that is not there and lose the real reason.
func (r *Renderer) namespaceNodeCall(access frontend.Node) (nodeBuiltin, bool, error) {
	kids := r.prog.Children(access)
	if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier || kids[1].Kind() != frontend.NodeIdentifier {
		return nodeBuiltin{}, false, nil
	}
	module, ok := r.nodeNamespaces[r.prog.Text(kids[0])]
	if !ok {
		return nodeBuiltin{}, false, nil
	}
	name := r.prog.Text(kids[1])
	if !nodeModuleExports[module][name] {
		return nodeBuiltin{}, false, &NotYetLowerable{Reason: "call of " + name + " on " + module + " is a later slice"}
	}
	return nodeBuiltin{module: module, name: name}, true, nil
}

// moduleObjectBindings returns the local names in an import clause that stand for
// the whole module object rather than for one of its exports: the binding of a
// namespace import (import * as path), the binding of a default import (import
// path), and both when one clause carries a default and a namespace at once. A
// clause that only picks names out of the module returns none.
//
// A namespace clause is recognized by its leading star, which is what
// namespaceBinding matches. Otherwise the default binding is the identifier
// directly under the clause: a named list is an Unknown brace node whose
// identifiers sit two levels down inside its specifiers, so it cannot be mistaken
// for one, and the star form in a mixed clause is matched by its own text.
func moduleObjectBindings(prog *frontend.Program, clause frontend.Node) []string {
	if b, ok := namespaceBinding(prog, clause); ok {
		return []string{b}
	}
	var out []string
	for _, c := range prog.Children(clause) {
		if c.Kind() == frontend.NodeIdentifier {
			out = append(out, prog.Text(c))
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(prog.Text(c)), "*") {
			if id, ok := firstIdentifier(prog, c); ok {
				out = append(out, id)
			}
		}
	}
	return out
}

// namedImportsNode descends an import clause to its named-imports node, the brace
// list whose children are the import specifiers. The clause of a named import
// wraps a single named-imports child; a default or namespace import has a
// different shape with no such child, which is reported as not found so the caller
// takes it as a module-object binding instead.
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
	// A data export of the module is not callable. The checker rejects calling one
	// in a well-typed program, but the lowerer sees programs whose diagnostics were
	// tolerated, so the shape is named here rather than falling through to the
	// default, which would report it as an unimplemented function.
	if _, ok := nodeModuleConstants[b.module][b.name]; ok {
		return nil, &NotYetLowerable{Reason: b.module + "." + b.name + " is a value, not a function"}
	}
	// Every node:os function bento lowers asks one question about the machine and
	// takes nothing, so they all lower the same way and differ only in the helper
	// named. An argument passed to one is not a call bento understands, so it hands
	// back rather than dropping the argument on the floor.
	if helper, ok := osHelpers[b.name]; ok && b.module == "node:os" {
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "os." + b.name + " takes no arguments"}
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", helper)}, nil
	}
	switch b.module + "." + b.name {
	case "node:path.join", "node:path.resolve":
		// join and resolve are the two variadic ones. Neither has a lower bound on
		// its argument count: path.join() is "." and path.resolve() is the working
		// directory, and the helpers answer both.
		args, err := r.stringArgs("path."+b.name, argNodes)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", pathHelpers[b.name]), Args: args}, nil

	case "node:path.normalize", "node:path.dirname", "node:path.extname",
		"node:path.isAbsolute", "node:path.toNamespacedPath":
		args, err := r.stringArgsN("path."+b.name, argNodes, 1)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", pathHelpers[b.name]), Args: args}, nil

	case "node:path.relative":
		args, err := r.stringArgsN("path.relative", argNodes, 2)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PathRelative"), Args: args}, nil

	case "node:path.basename":
		// basename takes an optional suffix to strip, and the two arities are
		// different helpers rather than one with a default, because a BStr has no
		// spelling for "no suffix" that is not also a real empty suffix.
		if len(argNodes) != 1 && len(argNodes) != 2 {
			return nil, &NotYetLowerable{Reason: "path.basename with this argument count is a later slice"}
		}
		args, err := r.stringArgs("path.basename", argNodes)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		if len(args) == 2 {
			return &ast.CallExpr{Fun: sel("value", "PathBasenameSuffix"), Args: args}, nil
		}
		return &ast.CallExpr{Fun: sel("value", "PathBasename"), Args: args}, nil

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

	case "node:util.format", "node:util.inspect":
		// util's two renderers take values of any type, so every argument boxes rather
		// than being checked against a shape. The helpers are variadic and read their
		// own argument list, format for the specifiers in its first argument and inspect
		// for the options in its second, so the lowering passes the arguments through
		// unchanged and does no arity checking of its own.
		args, err := r.boxedArgs("util."+b.name, argNodes)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		helper := "NodeFormat"
		if b.name == "inspect" {
			helper = "NodeInspectArgs"
		}
		return &ast.CallExpr{Fun: sel("value", helper), Args: args}, nil

	case "node:util.formatWithOptions":
		// formatWithOptions takes the options object first and the format arguments
		// after, which the helper spells as one required parameter and a variadic rest.
		// A call with no arguments at all cannot name that shape, so the options
		// argument is emitted as undefined, which is what the helper rejects with
		// Node's ERR_INVALID_ARG_TYPE rather than formatting nothing.
		args, err := r.boxedArgs("util.formatWithOptions", argNodes)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		if len(args) == 0 {
			args = []ast.Expr{sel("value", "Undefined")}
		}
		return &ast.CallExpr{Fun: sel("value", "NodeFormatWithOptions"), Args: args}, nil

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

// boxedArgs lowers a variadic list of arguments of any type into boxed values, the
// shape util.format and util.inspect take. Where a path helper needs each argument
// to be a string, these render whatever they are given, so each argument boxes into
// a value.Value instead. An argument bento cannot box yet (an array of objects, a
// Map) hands the whole call back, since a util call that dropped one of its
// arguments would print something other than what the program asked for.
func (r *Renderer) boxedArgs(what string, argNodes []frontend.Node) ([]ast.Expr, error) {
	args := make([]ast.Expr, 0, len(argNodes))
	for _, a := range argNodes {
		boxed, err := r.boxOperand(a)
		if err != nil {
			var nyl *NotYetLowerable
			if errors.As(err, &nyl) {
				return nil, &NotYetLowerable{Reason: what + " with an argument that does not box yet (" + nyl.Reason + ")"}
			}
			return nil, err
		}
		args = append(args, boxed)
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
