package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// buildAndRun compiles the entry of a CommonJS program written under a fresh temp
// directory and returns its combined output, failing the test on a build or run
// error. Each file in files is written relative to that directory, so a test names
// its modules by path and its require specifiers resolve between them.
func buildAndRun(t *testing.T, entry string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	bin := filepath.Join(dir, "prog")
	if err := Build(Options{Entry: filepath.Join(dir, entry), Output: bin}); err != nil {
		t.Fatalf("build %s: %v", entry, err)
	}
	got, err := exec.Command(bin).CombinedOutput()
	if err != nil {
		t.Fatalf("run %s: %v (%s)", entry, err, got)
	}
	return string(got)
}

// TestRequireLoadsModuleExports pins slice G0.3a: a require of a sibling module
// resolves the specifier to that module's loader, runs its body, and returns
// whatever the body left on module.exports. The entry logs the returned value, so
// it crossed the require boundary rather than throwing the way an unresolved
// require does. The module exports a primitive: an object export flows into a
// statically typed binding the caller must destructure, which needs the
// dynamic-value-into-static-type bridge a later slice adds, so the module system
// itself is pinned here on a value the boundary already carries.
func TestRequireLoadsModuleExports(t *testing.T) {
	got := buildAndRun(t, "main.js", map[string]string{
		"dep.js":  "module.exports = 42;\n",
		"main.js": "console.log(require('./dep'));\n",
	})
	if want := "42\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireRunsBodyOnce pins the module cache: a module required more than once
// runs its body a single time, and every require returns the one exports value. The
// module logs from its body, so a body run twice would print twice; Node prints the
// body line once and the caller line after it.
func TestRequireRunsBodyOnce(t *testing.T) {
	got := buildAndRun(t, "main.js", map[string]string{
		"dep.js":  "console.log('dep body');\nmodule.exports = 1;\n",
		"main.js": "require('./dep');\nrequire('./dep');\nconsole.log('main');\n",
	})
	if want := "dep body\nmain\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireTransitive pins a require chain: the entry requires a module that
// itself requires a third, so the loader of the middle module calls the loader of
// the leaf and folds its exports into its own. The composed value reaches the entry
// through the two hops.
func TestRequireTransitive(t *testing.T) {
	got := buildAndRun(t, "main.js", map[string]string{
		"b.js":    "module.exports = 'b';\n",
		"a.js":    "const b = require('./b');\nmodule.exports = 'a+' + b;\n",
		"main.js": "console.log(require('./a'));\n",
	})
	if want := "a+b\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireDiamond pins the module cache across more than one requirer: two
// modules both require a shared third, so its loader is called twice but its body
// runs once, and each requirer folds the one exports value into its own. The shared
// module logs from its body, so a second run would print twice; Node prints it once
// and then the two composed values.
func TestRequireDiamond(t *testing.T) {
	got := buildAndRun(t, "main.js", map[string]string{
		"c.js":    "console.log('c body');\nmodule.exports = 'c';\n",
		"a.js":    "const c = require('./c');\nmodule.exports = 'a' + c;\n",
		"b.js":    "const c = require('./c');\nmodule.exports = 'b' + c;\n",
		"main.js": "console.log(require('./a'), require('./b'));\n",
	})
	if want := "c body\nac bc\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
