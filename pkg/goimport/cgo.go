package goimport

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"sort"

	"golang.org/x/tools/go/packages"
)

// This file is the cgo detector of document 16 section 9.5. bento's zero-cgo
// promise (decision D2, decision D9) holds only for pure-Go libraries: a pure-Go
// go: import links into the emitted program and cross-compiles to every
// GOOS/GOARCH with CGO_ENABLED=0, exactly like the rest of the runtime. A go:
// import of a library that uses cgo, directly or through a dependency, drags cgo
// into the whole binary and forfeits that guarantee. The build must know before it
// runs the toolchain so it can tell the truth loudly rather than emit a cgo binary
// that quietly no longer cross-compiles, so this detector walks a reached package's
// import graph and reports the first cgo package it finds.

// CgoUse records that a go: import reached a cgo package. Import is the go: import
// path the program named, and Cgo is the package on that import's graph that
// carries the C directives, which is Import itself when the named library is the
// cgo one (the mattn/go-sqlite3 shape) or a transitive dependency when the named
// library is pure Go but pulls a cgo package in.
type CgoUse struct {
	Import string
	Cgo    string
}

// PureGoAlternative returns the import path of a pure-Go library that replaces a
// known cgo one, or the empty string when bento knows of none. It is the data
// behind the section 9.5 hint that points an author at the cgo-free option, the
// canonical case being modernc.org/sqlite in place of the C-wrapping
// mattn/go-sqlite3.
func PureGoAlternative(importPath string) string {
	switch importPath {
	case "github.com/mattn/go-sqlite3":
		return "modernc.org/sqlite"
	}
	return ""
}

// cgoLoadMode is the go/packages mode the detector needs: the package name, its
// Go file list (so a package's source files are named and can be scanned for the
// C import), and its imports and their dependencies so the whole reachable graph
// is available to walk. It deliberately omits NeedTypes and NeedSyntax, so the
// load never type-checks and never invokes a C compiler; go list enumerates the
// files from build metadata alone, which is what lets detection run on a host with
// no C toolchain.
const cgoLoadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedImports |
	packages.NeedDeps

// DetectCgo reports whether the Go package at importPath, or any package it
// imports transitively, is a cgo package. A package is cgo exactly when one of its
// Go files imports the pseudo-package "C", so the walk scans each package's files
// for that import. It loads the graph with CGO_ENABLED=1 in the load environment
// so a package's cgo files appear in its file list rather than being hidden behind
// the implicit cgo build constraint, and the load compiles nothing: go list names
// the files and the detector parses only their import clauses. A load failure is
// returned as an error so the caller can choose to degrade rather than block a
// build on a detector that could not run; a clean load with no cgo package returns
// ok false.
func DetectCgo(importPath string) (use CgoUse, ok bool, err error) {
	cfg := &packages.Config{
		Mode: cgoLoadMode,
		Env:  append(os.Environ(), "CGO_ENABLED=1"),
	}
	pkgs, err := packages.Load(cfg, importPath)
	if err != nil {
		return CgoUse{}, false, fmt.Errorf("detect cgo for %q: %w", importPath, err)
	}
	if len(pkgs) == 0 {
		return CgoUse{}, false, fmt.Errorf("no go package found for %q", importPath)
	}
	root := pkgs[0]
	if len(root.Errors) > 0 {
		return CgoUse{}, false, fmt.Errorf("go package %q has errors: %s", importPath, root.Errors[0].Msg)
	}

	cgoPkgs := map[string]bool{}
	seen := map[string]bool{}
	var walk func(p *packages.Package)
	walk = func(p *packages.Package) {
		if p == nil || seen[p.PkgPath] {
			return
		}
		seen[p.PkgPath] = true
		if packageUsesCgo(p) {
			cgoPkgs[p.PkgPath] = true
		}
		// Walk the imports in a fixed order so the chosen cgo package below is the
		// same across runs regardless of the map's iteration order.
		paths := make([]string, 0, len(p.Imports))
		for ip := range p.Imports {
			paths = append(paths, ip)
		}
		sort.Strings(paths)
		for _, ip := range paths {
			walk(p.Imports[ip])
		}
	}
	walk(root)

	if len(cgoPkgs) == 0 {
		return CgoUse{}, false, nil
	}
	// The named library being the cgo one is the direct and most useful thing to
	// report, so prefer the root; otherwise name the lexicographically first cgo
	// dependency, which keeps the diagnostic deterministic.
	pick := root.PkgPath
	if !cgoPkgs[pick] {
		pick = ""
		for p := range cgoPkgs {
			if pick == "" || p < pick {
				pick = p
			}
		}
	}
	return CgoUse{Import: importPath, Cgo: pick}, true, nil
}

// packageUsesCgo reports whether any of a package's Go files imports the
// pseudo-package "C", the exact definition of a cgo package. It parses only the
// import clause of each file, so it reads no bodies and runs no C tooling, and a
// file it cannot read or parse is skipped rather than treated as cgo, since a
// false cgo verdict would wrongly fail an honest pure-Go build.
func packageUsesCgo(p *packages.Package) bool {
	fset := token.NewFileSet()
	for _, path := range p.GoFiles {
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			continue
		}
		for _, imp := range file.Imports {
			if imp.Path != nil && imp.Path.Value == `"C"` {
				return true
			}
		}
	}
	return false
}
