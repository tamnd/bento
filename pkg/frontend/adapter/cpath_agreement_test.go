package adapter

import (
	"strings"
	"testing"

	"github.com/microsoft/typescript-go/shim"

	"github.com/tamnd/bento/pkg/cpath"
)

// pkg/cpath cannot import typescript-go: only this package may, so that a compiler
// version bump has a one-package blast radius, and cpath is imported by pkg/build,
// pkg/frontend and pkg/resolve. So cpath writes the path normalization out itself,
// and these tests are what make that safe. They run bento's normalizer and the
// compiler's over the same inputs and fail on any disagreement, here in the one
// package allowed to ask the compiler what it thinks.
//
// A disagreement is not cosmetic. The compiler's virtual file system panics on a
// key it does not consider normalized, so a path bento normalized differently is a
// crash on Windows, and one that no Unix machine would ever reproduce.

// checkerShapes are the paths bento actually produces: absolute POSIX, absolute
// DOS, and the relative forms that appear before a path has been made absolute.
var checkerShapes = []string{
	"/",
	"/home/x/main.ts",
	"/home/x/./main.ts",
	"/home/x/../y/main.ts",
	"/home/x/",
	"/home//x/main.ts",
	"/../x/main.ts",
	"/home/x/../../../main.ts",
	"/__bento_ambient__.d.ts",
	"/__bento_go__/crypto/sha256@1.24.d.ts",
	`C:\`,
	"C:/",
	`C:\Users\x\main.ts`,
	"C:/Users/x/main.ts",
	`C:\Users\x\..\y\main.ts`,
	`C:\Users\x\.\main.ts`,
	`C:\Users\x\`,
	`C:\..\x\main.ts`,
	"C:/__bento_ambient__.d.ts",
	"c:/users/x/main.ts",
	`\\server\share\x\main.ts`,
	"//server/share/x/main.ts",
	"//server/share/x/../y/main.ts",
	"main.ts",
	"./main.ts",
	"../main.ts",
	"a/b/../c/main.ts",
	"a/./b/main.ts",
	"",
}

// TestNormalizeAgreesWithTheCompiler is the guard against drift. Every path bento
// hands the checker goes through cpath.FromOS, and the compiler will panic on
// anything it does not consider normalized, so the two must give the same answer
// for every shape bento produces.
func TestNormalizeAgreesWithTheCompiler(t *testing.T) {
	for _, in := range checkerShapes {
		want := shim.NormalizePath(in)
		got := cpath.FromOS(in)
		if got != want {
			t.Errorf("FromOS(%q) = %q, compiler says %q", in, got, want)
		}
	}
}

// TestNormalizedPathsSatisfyTheFileMapPredicate runs the exact check the compiler's
// vfstest.FromMap makes on the keys of a file map before it panics, over what
// bento's own normalizer produces. This is the assertion closest to the actual
// Windows failure: a key that is not rooted, or that does not normalize to itself,
// takes down the build.
func TestNormalizedPathsSatisfyTheFileMapPredicate(t *testing.T) {
	for _, in := range checkerShapes {
		p := cpath.RemoveTrailingSeparator(cpath.FromOS(in))
		if !cpath.IsAbs(p) {
			continue // A relative input stays relative; only rooted paths reach the map.
		}
		// A path that is only a root names a directory, never a file, so it is not a
		// key any file map carries. The two functions differ there on purpose: the
		// compiler's RemoveTrailingDirectorySeparator takes "/" to "" and "C:/" to
		// "C:", while bento's keeps the slash, because bento uses the result as a
		// directory it will join against and neither "" nor "C:" is one.
		if cpath.IsRoot(p) {
			continue
		}
		if !shim.IsRootedDiskPath(p) {
			t.Errorf("%q normalized to %q, which the compiler does not consider rooted", in, p)
		}
		if again := shim.RemoveTrailingDirectorySeparator(shim.NormalizePath(p)); again != p {
			t.Errorf("%q normalized to %q, which the compiler renormalizes to %q", in, p, again)
		}
	}
}

// TestRootedAgreesWithTheCompiler pins IsAbs against the compiler's own notion,
// with one deliberate difference recorded: a bare volume like "c:" is rooted to
// the compiler and is not absolute to bento, because it names the working
// directory on that drive rather than a place, and bento may not hold one.
func TestRootedAgreesWithTheCompiler(t *testing.T) {
	for _, in := range checkerShapes {
		p := cpath.FromOS(in)
		got := cpath.IsAbs(p)
		want := shim.IsRootedDiskPath(p)
		if got != want && (len(p) != 2 || !strings.HasSuffix(p, ":")) {
			t.Errorf("IsAbs(%q) = %v, compiler's IsRootedDiskPath says %v", p, got, want)
		}
	}
}

// TestVirtualPathsDoNotMixStyles pins the second panic, the one that fires when a
// file map carries a POSIX-style key beside a Windows-style one. bento's two
// synthetic paths are written POSIX-rooted, so on a Windows program they have to
// take the program's volume before they reach the map.
func TestVirtualPathsDoNotMixStyles(t *testing.T) {
	root := cpath.FromOS(`C:\Users\x\main.ts`)
	for _, virtual := range []string{"/__bento_ambient__.d.ts", "/__bento_go__/fmt.d.ts"} {
		p := cpath.Virtual(virtual, root)
		if strings.HasPrefix(p, "/") {
			t.Errorf("Virtual(%q, %q) = %q, still POSIX-style beside a Windows root", virtual, root, p)
		}
		if cpath.Volume(p) != cpath.Volume(root) {
			t.Errorf("Virtual(%q, %q) = %q, on a different volume from the root", virtual, root, p)
		}
		if norm := shim.NormalizePath(p); norm != p {
			t.Errorf("Virtual produced %q, which the compiler normalizes to %q", p, norm)
		}
	}
}
