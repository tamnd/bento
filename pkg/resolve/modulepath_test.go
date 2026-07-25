package resolve

import (
	"strings"
	"testing"
)

// The resolver speaks module paths: slash-separated on every platform, which is
// what the resolution algorithm is specified over, what a package.json carries,
// and what an import specifier is. Only the FS implementation converts.
//
// The hard part of testing that is that on Unix path and filepath agree on
// nearly everything, so a resolver written against filepath passes almost any
// test a Unix machine can run. These cases are chosen for the places the two
// disagree even on Unix, so a regression is caught here rather than on Windows:
//
//   - filepath.IsAbs("C:/app/main.ts") is false on Unix, so the body of a
//     Windows file: URL would be joined against the importer's directory instead
//     of used as it stands.
//   - filepath.Dir("C:/main.ts") is "C:" on Unix, which names the working
//     directory on that drive rather than its root, so the node_modules walk
//     would start somewhere that does not exist.

// TestResolvesUnderADOSRoot walks a whole tree rooted at a volume: a relative
// import, a bare package through node_modules, and the realpath that comes back
// as the module's identity. A resolver holding OS paths gets none of these right
// on Windows, and gets the first two wrong even here.
func TestResolvesUnderADOSRoot(t *testing.T) {
	fs := newMemFS().
		add("C:/app/main.ts", "").
		add("C:/app/util.ts", "").
		add("C:/app/node_modules/dep/package.json", `{"main":"lib/index.js"}`).
		add("C:/app/node_modules/dep/lib/index.js", "")
	r := New(Options{FS: fs})
	parent := &Module{Path: "C:/app/main.ts", Dir: "C:/app", Format: FormatESM}

	for _, c := range []struct{ specifier, want string }{
		{"./util.ts", "C:/app/util.ts"},
		// A Windows file: URL, which classify unwraps to a DOS-rooted path. This is
		// the only way an absolute Windows path reaches the resolver as a specifier;
		// a bare "C:/app/util.ts" is not a module specifier and Node rejects it too.
		{"file:///C:/app/util.ts", "C:/app/util.ts"},
		{"dep", "C:/app/node_modules/dep/lib/index.js"},
	} {
		got, err := r.Resolve(c.specifier, parent)
		if err != nil {
			t.Errorf("Resolve(%q) failed: %v", c.specifier, err)
			continue
		}
		if got.Path != c.want {
			t.Errorf("Resolve(%q) = %q, want %q", c.specifier, got.Path, c.want)
		}
	}
}

// TestParentDirUnderADOSRoot pins the directory the walk starts from when a
// caller gives a path and no Dir. filepath.Dir would answer "C:" on Unix, which
// is a drive-relative path and not a place, so every lookup beneath it misses.
func TestParentDirUnderADOSRoot(t *testing.T) {
	for _, c := range []struct{ path, want string }{
		{"C:/app/main.ts", "C:/app"},
		{"C:/main.ts", "C:/"},
		{"/app/main.ts", "/app"},
	} {
		if got := parentDir(&Module{Path: c.path}); got != c.want {
			t.Errorf("parentDir(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

// TestResolvedPathsCarryNoSeparator sweeps the resolver's answers for a
// backslash. Every one of these is a path that reaches a caller and, through the
// frontend, the TypeScript checker, whose file map panics on a key in the wrong
// spelling.
func TestResolvedPathsCarryNoSeparator(t *testing.T) {
	fs := newMemFS().
		add("/app/main.ts", "").
		add("/app/sub/deep/mod.ts", "").
		add("/app/package.json", `{"imports":{"#alias":"./sub/deep/mod.ts"}}`).
		add("/app/node_modules/@scope/pkg/package.json", `{"exports":{"./x":"./x.js"}}`).
		add("/app/node_modules/@scope/pkg/x.js", "")
	r := New(Options{FS: fs})
	parent := &Module{Path: "/app/main.ts", Dir: "/app", Format: FormatESM}

	for _, specifier := range []string{"./sub/deep/mod.ts", "#alias", "@scope/pkg/x"} {
		got, err := r.Resolve(specifier, parent)
		if err != nil {
			t.Errorf("Resolve(%q) failed: %v", specifier, err)
			continue
		}
		if strings.Contains(got.Path, `\`) {
			t.Errorf("Resolve(%q) = %q, which carries an operating system separator", specifier, got.Path)
		}
	}
}

// TestFileURLResolvesToItsPath pins the unwrap. classify has always turned a
// file: URL into the path inside it, and Resolve has always passed the raw
// specifier on instead, so a file: import never resolved on any platform. Only
// the classification was tested, which is why nobody noticed. The Resolved keeps
// the specifier the importer wrote, so a later diagnostic still names the URL.
func TestFileURLResolvesToItsPath(t *testing.T) {
	fs := newMemFS().add("/app/main.ts", "").add("/app/util.ts", "")
	r := New(Options{FS: fs})
	parent := &Module{Path: "/app/main.ts", Dir: "/app", Format: FormatESM}

	got, err := r.Resolve("file:///app/util.ts", parent)
	if err != nil {
		t.Fatalf("Resolve(file:///app/util.ts) failed: %v", err)
	}
	if got.Path != "/app/util.ts" {
		t.Errorf("Resolve(file:///app/util.ts).Path = %q, want %q", got.Path, "/app/util.ts")
	}
	if got.Specifier != "file:///app/util.ts" {
		t.Errorf("Resolved.Specifier = %q, want the URL the importer wrote", got.Specifier)
	}
}

// TestNodeModulesWalkStaysUnderTheVolume pins the walk's stopping point. It has
// to end at the volume root and not climb past it into a path with no volume at
// all, which is what a POSIX-only Dir does with "C:/".
func TestNodeModulesWalkStaysUnderTheVolume(t *testing.T) {
	dirs := nodeModulesDirs("C:/app/sub")
	want := []string{"C:/app/sub/node_modules", "C:/app/node_modules", "C:/node_modules"}
	if len(dirs) != len(want) {
		t.Fatalf("nodeModulesDirs(C:/app/sub) = %q, want %q", dirs, want)
	}
	for i := range want {
		if dirs[i] != want[i] {
			t.Errorf("nodeModulesDirs[%d] = %q, want %q", i, dirs[i], want[i])
		}
	}
}
