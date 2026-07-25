package build

import (
	"os"
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/cpath"
)

// The build package is where an operating system path becomes a checker path: the
// entry named on the command line goes through cpath.Abs, and canonicalPath walks
// its symlinks on disk and converts the answer back. These tests pin the second
// half, which is the one that would silently regress, because on Unix the two
// spellings are the same string and nothing complains.

// TestCanonicalPathAnswersInCheckerPaths pins that the symlink walk gives back a
// checker path and not whatever the disk said. Without the conversion back, a
// Windows build would compare the loader's "C:/x/main.ts" against EvalSymlinks's
// "C:\x\main.ts", find no entry among the program's own source files, and hand the
// whole unit back with a message about an ambiguous entry.
func TestCanonicalPathAnswersInCheckerPaths(t *testing.T) {
	dir := t.TempDir()
	file := cpath.Join(cpath.FromOS(dir), "main.ts")
	if err := os.WriteFile(cpath.ToOS(file), []byte("export {};\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := canonicalPath(file)
	if strings.Contains(got, `\`) {
		t.Errorf("canonicalPath(%q) = %q, which carries an operating system separator", file, got)
	}
	if !cpath.IsAbs(got) {
		t.Errorf("canonicalPath(%q) = %q, which is not absolute", file, got)
	}
	if again := canonicalPath(got); again != got {
		t.Errorf("canonicalPath is not idempotent: %q then %q", got, again)
	}
}

// TestCanonicalPathKeepsAPathItCannotResolve pins the fallback: a virtual file a
// test feeds through an in-memory FS has no on-disk form, so EvalSymlinks fails,
// and the path still has to come back as a checker path rather than untouched.
func TestCanonicalPathKeepsAPathItCannotResolve(t *testing.T) {
	for _, in := range []string{"/__bento_ambient__.d.ts", "/no/such/dir/main.ts"} {
		if got := canonicalPath(in); got != in {
			t.Errorf("canonicalPath(%q) = %q, want it unchanged", in, got)
		}
	}
}

// TestIsJavaScriptReadsTheExtension pins that the entry-kind test reads a checker
// path. path.Ext, which cpath.Ext is, cannot be fooled by a backslash on Unix the
// way filepath.Ext can, so a path that reached here half-converted is still
// classified by its real extension.
func TestIsJavaScriptReadsTheExtension(t *testing.T) {
	js := []string{"/a/b.js", "/a/b.mjs", "/a/b.cjs", "/a/b.jsx", "C:/a/b.js"}
	ts := []string{"/a/b.ts", "/a/b.d.ts", "C:/a/b.ts", "/a/b"}
	for _, p := range js {
		if !isJavaScript(p) {
			t.Errorf("isJavaScript(%q) = false, want true", p)
		}
	}
	for _, p := range ts {
		if isJavaScript(p) {
			t.Errorf("isJavaScript(%q) = true, want false", p)
		}
	}
}
