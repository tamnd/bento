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

// TestRequireLoadsObjectExports pins slice G0.3b, the dynamic-value-into-static
// bridge: a module whose module.exports is an object flows into the caller's
// binding, which the checker types by the module's inferred exports shape. The
// binding lands in a boxed value.Value slot rather than the Go struct that shape
// would name, so the property reads off it resolve through the value model. Node
// prints the two fields the entry reads back off the required object.
func TestRequireLoadsObjectExports(t *testing.T) {
	got := buildAndRun(t, "main.js", map[string]string{
		"dep.js":  "module.exports = { x: 42, label: 'hi' };\n",
		"main.js": "const d = require('./dep');\nconsole.log(d.x, d.label);\n",
	})
	if want := "42 hi\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireCycle pins the partial-exports behaviour of a circular require through
// object properties. Module a requires b before it finishes exporting, and b
// requires back into a while a is still mid-body, so b sees a's exports as they
// stand at re-entry: done is still false because a sets it after the require. Node
// prints b's view of a captured during the cycle, then a's finished state.
func TestRequireCycle(t *testing.T) {
	got := buildAndRun(t, "main.js", map[string]string{
		"a.js": "exports.done = false;\n" +
			"const b = require('./b');\n" +
			"exports.done = true;\n" +
			"exports.bName = b.name;\n",
		"b.js": "const a = require('./a');\n" +
			"module.exports = { name: 'b', sawADone: a.done };\n",
		"main.js": "const a = require('./a');\n" +
			"console.log(a.done, a.bName);\n",
	})
	if want := "true b\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireExportsFunction pins slice G0.3c: a module whose body declares a
// top-level function and exports it lowers the function to a closure bound to a
// loader local, so it captures the loader's module and exports the way the source
// reads them. The entry calls the exported function through its boxed binding. Node
// prints the sum the required function returns.
func TestRequireExportsFunction(t *testing.T) {
	got := buildAndRun(t, "main.js", map[string]string{
		"dep.js":  "function add(a, b) { return a + b; }\nmodule.exports = add;\n",
		"main.js": "const add = require('./dep');\nconsole.log(add(2, 3));\n",
	})
	if want := "5\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireInternalHelper pins a module that declares two functions, one calling
// the other, and exports only the outer on an exports object. The inner helper is a
// loader local the outer closure captures, and a sibling reference resolves to it,
// so the composed call runs both. Node prints the result of the two nested calls.
func TestRequireInternalHelper(t *testing.T) {
	got := buildAndRun(t, "main.js", map[string]string{
		"dep.js":  "function sq(x) { return x * x; }\nfunction sumSq(a, b) { return sq(a) + sq(b); }\nmodule.exports = { sumSq };\n",
		"main.js": "const m = require('./dep');\nconsole.log(m.sumSq(3, 4));\n",
	})
	if want := "25\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireRecursiveExport pins a recursive exported function: the loader local
// is declared before it is assigned so the closure can call itself, the same
// two-step form a recursive named function expression takes. Node prints the
// factorial the required function computes.
func TestRequireRecursiveExport(t *testing.T) {
	got := buildAndRun(t, "main.js", map[string]string{
		"dep.js":  "function fac(n) { return n <= 1 ? 1 : n * fac(n - 1); }\nmodule.exports = fac;\n",
		"main.js": "console.log(require('./dep')(5));\n",
	})
	if want := "120\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRequireExportsClassInstance pins slice G0.3d: a required module that
// declares a top-level class, constructs an instance in its body, and exports a
// value read off it. The class registers in the shared pre-pass and emits as a
// package-level Go type the loader body constructs, and the primitive it exports
// crosses the require boundary the way any primitive export does. Node prints the
// sum the instance method returns.
func TestRequireExportsClassInstance(t *testing.T) {
	got := buildAndRun(t, "main.js", map[string]string{
		"dep.js": "class Point {\n" +
			"  x; y;\n" +
			"  constructor(x, y) { this.x = x; this.y = y; }\n" +
			"  sum() { return this.x + this.y; }\n" +
			"}\n" +
			"module.exports = new Point(3, 4).sum();\n",
		"main.js": "console.log(require('./dep'));\n",
	})
	if want := "7\n"; got != want {
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
