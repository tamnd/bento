package goimport

import "testing"

// TestParseThirdPartyModule pins the common case: a module path plus a package
// path inside it parses to that import path with no version.
func TestParseThirdPartyModule(t *testing.T) {
	s, err := ParseSpecifier("go:github.com/klauspost/compress/zstd")
	if err != nil {
		t.Fatalf("ParseSpecifier returned error: %v", err)
	}
	if s.ImportPath != "github.com/klauspost/compress/zstd" {
		t.Errorf("import path = %q, want the verbatim Go path", s.ImportPath)
	}
	if s.Pinned() {
		t.Errorf("specifier reported pinned, want unpinned; version = %q", s.Version)
	}
}

// TestParseStandardLibrary pins that a standard-library import path parses, with no
// dotted host required. This is the case the old resolver rejected: section 3
// imports "crypto/sha256" the same way it imports a third-party module.
func TestParseStandardLibrary(t *testing.T) {
	for _, path := range []string{"crypto/sha256", "io", "encoding/json"} {
		s, err := ParseSpecifier("go:" + path)
		if err != nil {
			t.Errorf("ParseSpecifier(go:%s) returned error: %v", path, err)
			continue
		}
		if s.ImportPath != path {
			t.Errorf("import path = %q, want %q", s.ImportPath, path)
		}
	}
}

// TestParsePinnedVersion pins that an inline @version is split off the import path
// and surfaced as the version, not left dangling on the path (section 3.2).
func TestParsePinnedVersion(t *testing.T) {
	s, err := ParseSpecifier("go:github.com/klauspost/compress/zstd@v1.17.9")
	if err != nil {
		t.Fatalf("ParseSpecifier returned error: %v", err)
	}
	if s.ImportPath != "github.com/klauspost/compress/zstd" {
		t.Errorf("import path = %q, want the path with the version removed", s.ImportPath)
	}
	if s.Version != "v1.17.9" {
		t.Errorf("version = %q, want v1.17.9", s.Version)
	}
	if !s.Pinned() {
		t.Errorf("specifier reported unpinned for a pinned import")
	}
}

// TestParsePseudoVersion pins that a Go pseudo-version, which is what an untagged
// commit resolves to, is accepted as a version.
func TestParsePseudoVersion(t *testing.T) {
	s, err := ParseSpecifier("go:example.com/mod@v0.0.0-20210101000000-abcdefabcdef")
	if err != nil {
		t.Fatalf("ParseSpecifier returned error: %v", err)
	}
	if s.Version != "v0.0.0-20210101000000-abcdefabcdef" {
		t.Errorf("version = %q, want the pseudo-version", s.Version)
	}
}

// TestParseRejectsNonScheme pins that a specifier without the go: scheme is not a
// Go import and is refused.
func TestParseRejectsNonScheme(t *testing.T) {
	if IsGoImport("github.com/klauspost/compress/zstd") {
		t.Errorf("a bare path was treated as a go: import")
	}
	if _, err := ParseSpecifier("github.com/klauspost/compress/zstd"); err == nil {
		t.Errorf("ParseSpecifier accepted a specifier with no go: scheme")
	}
}

// TestParseRejectsMalformedPaths pins that a path with an empty segment, a leading
// or trailing slash, a relative segment, or a stray character is refused rather
// than passed through to break a later stage.
func TestParseRejectsMalformedPaths(t *testing.T) {
	for _, bad := range []string{
		"go:",
		"go:/leading",
		"go:trailing/",
		"go:double//slash",
		"go:has space/pkg",
		"go:../relative/pkg",
		"go:pkg/./here",
		"go:bad$char/pkg",
	} {
		if _, err := ParseSpecifier(bad); err == nil {
			t.Errorf("ParseSpecifier(%q) accepted a malformed path", bad)
		}
	}
}

// TestParseRejectsMalformedVersion pins that a pin that is not a Go module version
// is refused, so a typo like a missing v is caught at parse time.
func TestParseRejectsMalformedVersion(t *testing.T) {
	for _, bad := range []string{
		"go:example.com/mod@",
		"go:example.com/mod@1.2.3",
		"go:example.com/mod@latest",
		"go:example.com/mod@v1 2",
	} {
		if _, err := ParseSpecifier(bad); err == nil {
			t.Errorf("ParseSpecifier(%q) accepted a malformed version", bad)
		}
	}
}
