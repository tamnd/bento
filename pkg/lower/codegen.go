package lower

import (
	"go/ast"
	"go/format"
	"go/token"
	"strings"
)

// This file is the one place bento turns lowered types into Go syntax. Every
// generated type expression and declaration is built as a go/ast node and then
// printed, never assembled by concatenating strings: an ast.Expr composes by
// nesting nodes (an array wraps its element node, a pointer wraps its pointee),
// so a malformed splice is a type error at construction rather than invalid Go
// discovered at format time. The printer, run in gofmt mode, is the only step
// that produces text, which keeps every generated declaration gofmt-clean as
// 05_type_lowering.md section 2 requires.

// ident is a bare identifier node, the leaf of most type expressions.
func ident(name string) *ast.Ident { return ast.NewIdent(name) }

// star is a pointer type node, *X. Objects, symbols, bigints, and arrays all
// lower to pointers, so this is the most reused constructor here.
func star(x ast.Expr) *ast.StarExpr { return &ast.StarExpr{X: x} }

// sel is a qualified name, pkg.Name, for the value-model and standard-library
// types the runtime provides (value.Array, value.Symbol, big.Int).
func sel(pkg, name string) *ast.SelectorExpr {
	return &ast.SelectorExpr{X: ident(pkg), Sel: ident(name)}
}

// index is a single type-argument instantiation, X[T], for the generic value
// header types such as value.Array[T].
func index(x, arg ast.Expr) *ast.IndexExpr {
	return &ast.IndexExpr{X: x, Index: arg}
}

// printExpr renders a type expression node to its Go source. It is the boundary
// between the ast world and the string world: callers above hold nodes, and only
// here do they become text. A print failure means the node bento built is not
// valid Go, a lowering bug rather than a source-driven boundary, so it surfaces
// as a NotYetLowerable rather than a panic.
func printExpr(e ast.Expr) (string, error) {
	var b strings.Builder
	if err := format.Node(&b, token.NewFileSet(), e); err != nil {
		return "", &NotYetLowerable{Reason: "generated type expression did not print: " + err.Error()}
	}
	return b.String(), nil
}

// printDecl renders a generated top-level declaration to gofmt-clean Go source
// with the trailing newline a file expects. format.Node prints a single decl
// without a final newline, so it is added to match the on-disk shape a developer
// reads in a stack trace.
func printDecl(d ast.Decl) (string, error) {
	var b strings.Builder
	if err := format.Node(&b, token.NewFileSet(), d); err != nil {
		return "", &NotYetLowerable{Reason: "generated declaration did not print: " + err.Error()}
	}
	b.WriteByte('\n')
	return b.String(), nil
}
