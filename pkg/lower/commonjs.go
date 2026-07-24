package lower

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strconv"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the CommonJS module-wrapper globals. Node wraps every module
// in a function whose parameters are exports, require, module, __filename, and
// __dirname, so a module body reads those names as if they were globals. bento's
// AOT path admits a JavaScript entry (see the nodejs-compat roadmap), and a real
// Node file reaches for these names, so the lowerer models them rather than
// letting them fall through to a bare Go identifier that does not compile.
//
// This slice covers the two path globals, __dirname and __filename. Each denotes
// a string fixed for the life of the module: the directory holding the module
// file, and the module file's own absolute path. bento composes a whole program
// from its modules at compile time, so the path is known when the reference is
// lowered and needs no runtime lookup; the reference lowers to the string
// literal the module's file path yields, the same value.BStr a source string of
// that content would. module, exports, and require are later slices.

// dirnameLit lowers a __dirname reference to the directory of the module file the
// reference sits in. It reads the containing file's path off the node, so a
// reference resolves against its own module rather than the entry, which is what
// keeps the value right once more than one module composes into a program.
func (r *Renderer) dirnameLit(n frontend.Node) ast.Expr {
	return r.goStringValue(filepath.Dir(n.File().Path))
}

// filenameLit lowers a __filename reference to the absolute path of the module
// file the reference sits in, the value Node fills its wrapper's __filename with.
func (r *Renderer) filenameLit(n frontend.Node) ast.Expr {
	return r.goStringValue(n.File().Path)
}

// bentoModuleName and bentoExportsName are the Go identifiers the module object
// and its exports alias emit under. They are read by a bare module or exports
// reference and declared once as package-level vars, so a top-level function that
// closes over either names the same variable main does. A user binding that
// mangles to one of these names is rejected by the collision check in
// program.go rather than silently sharing the identifier.
const (
	bentoModuleName  = "bentoModule"
	bentoExportsName = "bentoExports"
)

// isCommonJSModuleGlobal reports whether n is a reference to the CommonJS module
// or exports wrapper global rather than a user binding that shares the name. The
// ambient any declarations (aot_ambient.go) do not settle this the way isGlobalRef
// settles Math, because the checker's CommonJS export inference synthesizes a
// separate symbol for exports and module whenever the module assigns to either,
// and that symbol's declarations are the assignment-target identifiers in the
// source file, not the ambient var. So isGlobalRef, which requires every
// declaration to live in a .d.ts, always says no here.
//
// The predicate instead accepts a symbol whose every declaration is either the
// ambient fallback var (a module that only reads the global resolves to it) or
// the source file node itself, which is how the checker models the CommonJS
// module symbol and its exports: the file is the module. A genuine user binding
// is excluded because it declares through a variable statement, a parameter, a
// function, or a class, none of which is the source file, so a local const
// exports keeps its own storage.
func (r *Renderer) isCommonJSModuleGlobal(n frontend.Node) bool {
	if n.Kind() != frontend.NodeIdentifier {
		return false
	}
	if t := r.prog.Text(n); t != "module" && t != "exports" {
		return false
	}
	sym, ok := r.prog.SymbolAt(n)
	if !ok {
		return false
	}
	decls := r.prog.Declarations(sym)
	if len(decls) == 0 {
		return false
	}
	for _, d := range decls {
		if d.File().Kind == frontend.FileDTS {
			continue
		}
		if d.Kind() == frontend.NodeSourceFile {
			continue
		}
		return false
	}
	return true
}

// moduleRef lowers a bare module reference to the package-level module object,
// flagging that the object must be emitted. The object is a value.Object with an
// exports property; a read of module.exports then lowers through the ordinary
// dynamic member path, since module is typed any.
func (r *Renderer) moduleRef() ast.Expr {
	r.usesCommonJSModule = true
	return ident(bentoModuleName)
}

// exportsRef lowers a bare exports reference to the exports alias, the same object
// module.exports starts at. A write exports.x = v mutates that object; a later
// module.exports = v reassigns module's property without moving this alias, which
// is the divergence Node's wrapper has and a program observes only if it does both.
func (r *Renderer) exportsRef() ast.Expr {
	r.usesCommonJSModule = true
	return ident(bentoExportsName)
}

// commonjsModuleDecls returns the package-level declarations that back the module
// and exports globals, or nil when the program named neither. The exports object
// is declared first and the module object holds it under the exports property, so
// the two names reach one object at program start; Go orders the two package vars
// by the dependency between them.
func (r *Renderer) commonjsModuleDecls() []ast.Decl {
	if !r.usesCommonJSModule {
		return nil
	}
	r.requireImport(valuePkg)
	exportsVar := &ast.GenDecl{
		Tok: token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{
			Names:  []*ast.Ident{ident(bentoExportsName)},
			Values: []ast.Expr{&ast.CallExpr{Fun: sel("value", "NewObject")}},
		}},
	}
	moduleVar := &ast.GenDecl{
		Tok: token.VAR,
		Specs: []ast.Spec{&ast.ValueSpec{
			Names: []*ast.Ident{ident(bentoModuleName)},
			Values: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{
					X:   &ast.CallExpr{Fun: sel("value", "NewObject")},
					Sel: ident("Set"),
				},
				Args: []ast.Expr{
					r.goStringValue("exports"),
					ident(bentoExportsName),
				},
			}},
		}},
	}
	return []ast.Decl{exportsVar, moduleVar}
}

// goStringValue builds the AST for the value.BStr of a Go string known at lower
// time, the representation a string-typed expression lowers to. It mirrors the
// non-surrogate branch of bstrLit, which a source string literal takes; a module
// path is always valid UTF-8 with no lone surrogate, so that branch is the whole
// of what a path needs.
func (r *Renderer) goStringValue(s string) ast.Expr {
	r.requireImport(valuePkg)
	return &ast.CallExpr{
		Fun:  sel("value", "FromGoString"),
		Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(s)}},
	}
}
