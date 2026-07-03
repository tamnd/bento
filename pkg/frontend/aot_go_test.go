package frontend

import (
	"strings"
	"testing"
)

// TestGoDeclPathRoundTrips checks that a go: import's virtual declaration path
// encodes the import path and version losslessly, so the overlay can recover which
// package to generate from the path alone.
func TestGoDeclPathRoundTrips(t *testing.T) {
	cases := []struct {
		importPath string
		version    string
	}{
		{"crypto/sha256", ""},
		{"crypto/sha256", "v1.0.0"},
		{"github.com/klauspost/compress/zstd", "v1.17.9"},
		{"io", ""},
	}
	for _, c := range cases {
		path := goDeclPath(c.importPath, c.version)
		if !strings.HasSuffix(path, ".d.ts") {
			t.Errorf("goDeclPath(%q, %q) = %q, want a .d.ts file", c.importPath, c.version, path)
		}
		gotPath, gotVer, ok := goImportForDeclPath(path)
		if !ok {
			t.Errorf("goImportForDeclPath(%q) reported not a go declaration path", path)
			continue
		}
		if gotPath != c.importPath || gotVer != c.version {
			t.Errorf("round trip of %q got (%q, %q), want (%q, %q)", path, gotPath, gotVer, c.importPath, c.version)
		}
	}
}

// TestGoImportForDeclPathRejectsOtherPaths holds the boundary: an ordinary file
// path is not a go declaration path, so the overlay leaves it to the base FS.
func TestGoImportForDeclPathRejectsOtherPaths(t *testing.T) {
	for _, path := range []string{
		"/app/main.ts",
		"/__bento_ambient__.d.ts",
		"/__bento_go__/",
		"crypto/sha256.d.ts",
	} {
		if _, _, ok := goImportForDeclPath(path); ok {
			t.Errorf("goImportForDeclPath(%q) claimed a non-go path", path)
		}
	}
}

// TestWrapGoModuleNamesTheSpecifier proves the wrapper produces an ambient module
// declaration named for the exact go: specifier, which is what the checker resolves
// the import against, and drops a redundant declare modifier the body might carry.
func TestWrapGoModuleNamesTheSpecifier(t *testing.T) {
	body := "export declare function Do(): void;\n"
	got := wrapGoModule("go:crypto/sha256", body)
	if !strings.HasPrefix(got, `declare module "go:crypto/sha256" {`) {
		t.Errorf("wrapped module does not open with the go: specifier:\n%s", got)
	}
	if !strings.HasSuffix(got, "}\n") {
		t.Errorf("wrapped module is not closed:\n%s", got)
	}
	if strings.Contains(got, "export declare ") {
		t.Errorf("wrapped module leaves a redundant declare modifier in place:\n%s", got)
	}
	if !strings.Contains(got, "export function Do(): void;") {
		t.Errorf("wrapped module dropped the body declaration:\n%s", got)
	}
}

// TestGoSpecifierRebuildsImport checks the specifier the ambient module is named
// for matches how the program wrote the import, with and without a pinned version.
func TestGoSpecifierRebuildsImport(t *testing.T) {
	if got := goSpecifier("crypto/sha256", ""); got != "go:crypto/sha256" {
		t.Errorf("goSpecifier unpinned = %q, want go:crypto/sha256", got)
	}
	if got := goSpecifier("crypto/sha256", "v1.2.3"); got != "go:crypto/sha256@v1.2.3" {
		t.Errorf("goSpecifier pinned = %q, want go:crypto/sha256@v1.2.3", got)
	}
}
