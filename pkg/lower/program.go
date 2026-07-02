package lower

import (
	"go/ast"
	"go/format"
	"go/token"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file assembles the lowered pieces of one entry module into a single
// runnable Go program: a package main file holding the module's top-level
// functions as Go functions and its top-level statements as the body of main.
// It is the step that turns a checked .ts of real size into a .go with a main,
// the shape the native build compiles and links (doc 05 section 27, doc 17). Like
// every other lowering step it hands back a NotYetLowerable rather than emit
// partial or wrong Go, so a program the subset does not fully cover routes to the
// engine instead.

// Program is the assembled Go source for one compiled entry module. Source is a
// complete, gofmt-clean package main file the Go toolchain builds directly.
type Program struct {
	Source string
}

// RenderProgram lowers one entry source file to a runnable Go program. Top-level
// function declarations become package-level Go functions, and the remaining
// top-level statements become the body of main in source order, so the module's
// side effects run when the binary runs. A construct the statement subset does
// not cover, or a top-level form that is neither a function nor a lowerable
// statement (an import, an export, a class), hands back.
//
// The module's own top-level bindings are locals of main, so a top-level function
// that reads one is not yet supported: the function is a separate Go declaration
// that cannot see main's locals, which would fail the Go build rather than emit
// wrong output. Hoisting shared module bindings to package-level vars is a later
// slice; today a program whose functions are self-contained (the common shape of
// the compute workloads, which are a single top-level body) compiles.
func (r *Renderer) RenderProgram(entry frontend.Node) (Program, error) {
	var funcs []ast.Decl
	var mainBody []frontend.Node
	for _, stmt := range r.prog.Children(entry) {
		switch stmt.Kind() {
		case frontend.NodeFunctionDeclaration:
			fd, err := r.funcDecl(stmt)
			if err != nil {
				return Program{}, err
			}
			funcs = append(funcs, fd)
		case frontend.NodeInterfaceDeclaration, frontend.NodeTypeAliasDeclaration:
			// Type-level declarations carry no runtime code, so they emit nothing.
			continue
		case frontend.NodeUnknown:
			// The parser ends a source file with an empty end-of-file token bento
			// does not name; it is skipped. A non-empty unnamed node is a construct
			// the frontend did not classify, so it hands back rather than vanish.
			if strings.TrimSpace(r.prog.Text(stmt)) != "" {
				return Program{}, &NotYetLowerable{Reason: "unrecognized top-level construct is a later slice"}
			}
		default:
			mainBody = append(mainBody, stmt)
		}
	}

	stmts, err := r.lowerStatements(mainBody)
	if err != nil {
		return Program{}, err
	}

	mainDecl := &ast.FuncDecl{
		Name: ident("main"),
		Type: &ast.FuncType{Params: &ast.FieldList{}},
		Body: &ast.BlockStmt{List: stmts},
	}

	file := &ast.File{Name: ident("main")}
	if paths := r.Imports(); len(paths) > 0 {
		file.Decls = append(file.Decls, importDecl(paths))
	}
	file.Decls = append(file.Decls, funcs...)
	file.Decls = append(file.Decls, mainDecl)

	src, err := printFile(file)
	if err != nil {
		return Program{}, err
	}
	return Program{Source: src}, nil
}

// importDecl builds the import block for the assembled file. The parenthesized
// form is forced (a nonzero Lparen) so a single import prints as an import block
// like every other, which keeps the generated file's shape stable as more
// imports appear.
func importDecl(paths []string) ast.Decl {
	specs := make([]ast.Spec, 0, len(paths))
	for _, p := range paths {
		specs = append(specs, &ast.ImportSpec{
			Path: &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(p)},
		})
	}
	return &ast.GenDecl{Tok: token.IMPORT, Lparen: token.Pos(1), Specs: specs}
}

// printFile renders the assembled file to gofmt-clean Go source. A print failure
// means the file bento built is not valid Go, a lowering bug, so it surfaces as a
// NotYetLowerable rather than a panic, the same boundary printExpr and printDecl
// keep.
func printFile(f *ast.File) (string, error) {
	var b strings.Builder
	if err := format.Node(&b, token.NewFileSet(), f); err != nil {
		return "", &NotYetLowerable{Reason: "generated program did not print: " + err.Error()}
	}
	return b.String(), nil
}
