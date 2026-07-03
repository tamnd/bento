package goimport

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"
)

// This file is the loader of document 16 section 5.1: it reads a Go package's
// exported API without compiling it to machine code, by running go/packages in a
// mode that type-checks the source and keeps the syntax. The type view drives the
// declaration generator (dts.go), and the syntax view supplies the doc comments
// that become TSDoc (section 5.3). Working from go/types rather than source text
// is what resolves aliases, embeddings, and cross-package types the way a real Go
// compiler sees them.

// LoadMode is the go/packages mode the loader needs: the package name, its
// type information, and its syntax, plus the same for dependencies so a
// cross-package type in a signature resolves. It is exported so a caller that
// loads packages itself can match the mode the generator expects.
const LoadMode = packages.NeedName |
	packages.NeedTypes |
	packages.NeedTypesInfo |
	packages.NeedSyntax |
	packages.NeedDeps |
	packages.NeedImports

// Load type-checks the Go package at importPath and generates its .d.ts. It is
// the front door for the common case: hand it an import path and a version to
// stamp in the header, and it returns the declaration text. The version is not
// verified here; it is the value the resolver already reconciled against go.mod
// (section 4.3), carried through only for the banner and the cache key.
func Load(importPath, version string) (string, error) {
	pkg, err := loadPackage(importPath)
	if err != nil {
		return "", err
	}
	return Generate(pkg.Types, GenOptions{
		ImportPath: importPath,
		Version:    version,
		Docs:       docLookup(pkg),
	}), nil
}

// loadPackage loads exactly one package in the generator's mode and returns it,
// turning the several ways go/packages can report trouble (no package, too many,
// type errors) into one error the caller can surface at resolve time.
func loadPackage(importPath string) (*packages.Package, error) {
	cfg := &packages.Config{Mode: LoadMode}
	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return nil, fmt.Errorf("load go package %q: %w", importPath, err)
	}
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no go package found for %q", importPath)
	}
	if len(pkgs) > 1 {
		return nil, fmt.Errorf("import path %q matched %d packages, want one", importPath, len(pkgs))
	}
	pkg := pkgs[0]
	if len(pkg.Errors) > 0 {
		return nil, fmt.Errorf("go package %q has errors: %w", importPath, firstPackageError(pkg))
	}
	if pkg.Types == nil {
		return nil, fmt.Errorf("go package %q loaded without type information", importPath)
	}
	return pkg, nil
}

func firstPackageError(pkg *packages.Package) error {
	if len(pkg.Errors) == 0 {
		return errors.New("unknown package error")
	}
	return errors.New(pkg.Errors[0].Msg)
}

// docLookup builds a DocLookup over a loaded package by indexing the doc comment
// of every documentable declaration by the position of its defining identifier,
// then resolving a types.Object to its doc through that object's Pos. This is how
// the generator, which works from go/types, reaches the doc text that lives in the
// AST (section 5.3). A Go doc comment conventionally starts with the identifier
// name; the index keeps the text verbatim and the emitter renders it as TSDoc.
func docLookup(pkg *packages.Package) DocLookup {
	index := map[token.Pos]string{}
	for _, file := range pkg.Syntax {
		indexFileDocs(file, index)
	}
	if len(index) == 0 {
		return nil
	}
	return func(obj types.Object) string {
		return index[obj.Pos()]
	}
}

// indexFileDocs walks one file's top-level declarations, recording the doc
// comment for each function, type, constant, variable, struct field, and
// interface method against the position of its name.
func indexFileDocs(file *ast.File, index map[token.Pos]string) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if text := commentText(d.Doc); text != "" {
				index[d.Name.Pos()] = text
			}
		case *ast.GenDecl:
			indexGenDecl(d, index)
		}
	}
}

// indexGenDecl indexes a type, const, or var declaration. A grouped declaration
// can carry its doc on the group or on the individual spec, so a spec with no doc
// of its own inherits the group's, matching how go/doc attributes documentation.
func indexGenDecl(d *ast.GenDecl, index map[token.Pos]string) {
	groupDoc := commentText(d.Doc)
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			text := commentText(s.Doc)
			if text == "" {
				text = groupDoc
			}
			if text != "" {
				index[s.Name.Pos()] = text
			}
			indexTypeMembers(s.Type, index)
		case *ast.ValueSpec:
			text := commentText(s.Doc)
			if text == "" {
				text = groupDoc
			}
			if text == "" {
				continue
			}
			for _, name := range s.Names {
				index[name.Pos()] = text
			}
		}
	}
}

// indexTypeMembers indexes the doc comments on a struct's fields or an interface's
// methods, so member declarations carry the library author's own documentation
// the same way top-level declarations do.
func indexTypeMembers(expr ast.Expr, index map[token.Pos]string) {
	switch t := expr.(type) {
	case *ast.StructType:
		if t.Fields == nil {
			return
		}
		for _, field := range t.Fields.List {
			indexField(field, index)
		}
	case *ast.InterfaceType:
		if t.Methods == nil {
			return
		}
		for _, field := range t.Methods.List {
			indexField(field, index)
		}
	}
}

// indexField records a field or method's doc against each name it declares,
// preferring the leading doc comment and falling back to a trailing line comment,
// which is where a one-line field note usually sits.
func indexField(field *ast.Field, index map[token.Pos]string) {
	text := commentText(field.Doc)
	if text == "" {
		text = commentText(field.Comment)
	}
	if text == "" {
		return
	}
	for _, name := range field.Names {
		index[name.Pos()] = text
	}
}

// commentText flattens a comment group to its text with the markers stripped, or
// "" when there is no comment. It trims trailing space so a rendered TSDoc block
// carries no ragged whitespace.
func commentText(group *ast.CommentGroup) string {
	if group == nil {
		return ""
	}
	return strings.TrimRight(group.Text(), "\n")
}
