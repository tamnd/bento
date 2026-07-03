package goimport

import "testing"

// TestDetectCgoDirect proves a package that is itself cgo is reported as a cgo use
// naming that package. runtime/cgo is the canonical always-cgo standard-library
// package, so it is a stable positive that needs no C toolchain, since detection
// reads file metadata and parses only import clauses.
func TestDetectCgoDirect(t *testing.T) {
	use, ok, err := DetectCgo("runtime/cgo")
	if err != nil {
		t.Fatalf("DetectCgo(runtime/cgo) errored: %v", err)
	}
	if !ok {
		t.Fatal("runtime/cgo not detected as cgo, want a cgo use")
	}
	if use.Import != "runtime/cgo" || use.Cgo != "runtime/cgo" {
		t.Errorf("use = %+v, want import and cgo both runtime/cgo", use)
	}
}

// TestDetectCgoTransitive proves the detector walks the import graph: a pure-Go
// package that imports a cgo one is reported as a cgo use whose import is the
// package asked for and whose cgo library is the dependency that carries the C
// import, the shape a real library pulling in a cgo driver takes.
func TestDetectCgoTransitive(t *testing.T) {
	const path = "github.com/tamnd/bento/pkg/goimport/cgodepfixture"
	use, ok, err := DetectCgo(path)
	if err != nil {
		t.Fatalf("DetectCgo(%s) errored: %v", path, err)
	}
	if !ok {
		t.Fatal("a package importing runtime/cgo not detected as cgo, want a cgo use")
	}
	if use.Import != path {
		t.Errorf("use.Import = %q, want the package asked for %q", use.Import, path)
	}
	if use.Cgo != "runtime/cgo" {
		t.Errorf("use.Cgo = %q, want the transitive cgo dependency runtime/cgo", use.Cgo)
	}
}

// TestDetectCgoPureGo proves a pure-Go package and its pure-Go dependencies are not
// reported as cgo, so the detector never fails an honest zero-cgo build. strings
// pulls in a handful of standard-library packages and none use cgo.
func TestDetectCgoPureGo(t *testing.T) {
	use, ok, err := DetectCgo("strings")
	if err != nil {
		t.Fatalf("DetectCgo(strings) errored: %v", err)
	}
	if ok {
		t.Errorf("strings reported as cgo (%+v), want a pure-Go verdict", use)
	}
}

// TestPureGoAlternative proves the section 9.5 hint knows the canonical cgo-to-pure
// swap and answers empty for a package it has no alternative for, so the diagnostic
// points at modernc.org/sqlite for go-sqlite3 and stays silent otherwise.
func TestPureGoAlternative(t *testing.T) {
	if got := PureGoAlternative("github.com/mattn/go-sqlite3"); got != "modernc.org/sqlite" {
		t.Errorf("PureGoAlternative(go-sqlite3) = %q, want modernc.org/sqlite", got)
	}
	if got := PureGoAlternative("github.com/klauspost/compress/zstd"); got != "" {
		t.Errorf("PureGoAlternative(zstd) = %q, want empty for a pure-Go library", got)
	}
}
