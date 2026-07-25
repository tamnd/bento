package resolve

import (
	"errors"
	"testing"
)

// stubBuiltins is a fixed set of builtin names for tests.
type stubBuiltins map[string]bool

func (s stubBuiltins) Has(name string) bool { return s[name] }

func newTestResolver(fs FS, dev bool) *Resolver {
	return New(Options{
		FS:       fs,
		Builtins: stubBuiltins{"fs": true, "path": true, "os": true},
		Dev:      dev,
	})
}

func parentCJS(path string) *Module { return &Module{Path: path, Format: FormatCommonJS} }
func parentESM(path string) *Module { return &Module{Path: path, Format: FormatESM} }

func TestResolveRelativeWithExtensionSearch(t *testing.T) {
	fs := newMemFS().add("/app/index.ts", "").add("/app/util.ts", "")
	r := newTestResolver(fs, true)

	got, err := r.Resolve("./util", parentCJS("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/util.ts" {
		t.Errorf("path = %q, want /app/util.ts", got.Path)
	}
	if got.Kind != KindFile {
		t.Errorf("kind = %v, want file", got.Kind)
	}
}

func TestResolveTSExtensionRewrite(t *testing.T) {
	// An import of ./math.js from a .ts file means ./math.ts on disk.
	fs := newMemFS().add("/app/index.ts", "").add("/app/math.ts", "")
	r := newTestResolver(fs, true)

	got, err := r.Resolve("./math.js", parentESM("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/math.ts" {
		t.Errorf("path = %q, want /app/math.ts", got.Path)
	}
}

func TestResolveDirectoryIndex(t *testing.T) {
	fs := newMemFS().add("/app/index.ts", "").add("/app/lib/index.ts", "")
	r := newTestResolver(fs, true)

	got, err := r.Resolve("./lib", parentCJS("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/lib/index.ts" {
		t.Errorf("path = %q, want /app/lib/index.ts", got.Path)
	}
}

func TestResolveDirectoryIndexPrefersCJSForRequire(t *testing.T) {
	// A directory holding both index.js and index.mjs must resolve to index.js
	// for a CommonJS require and to index.mjs for an ESM import, matching Node:
	// require never picks the ESM-only file.
	fs := newMemFS().
		add("/app/index.js", "").
		add("/app/lib/index.js", "").
		add("/app/lib/index.mjs", "")
	r := newTestResolver(fs, true)

	cjs, err := r.Resolve("./lib", parentCJS("/app/index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if cjs.Path != "/app/lib/index.js" {
		t.Errorf("require path = %q, want /app/lib/index.js", cjs.Path)
	}

	esm, err := r.Resolve("./lib", parentESM("/app/index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if esm.Path != "/app/lib/index.mjs" {
		t.Errorf("import path = %q, want /app/lib/index.mjs", esm.Path)
	}
}

func TestResolveExtensionSearchPrefersCJSForRequire(t *testing.T) {
	// The same rule applies to extensionless file resolution: require('./util')
	// prefers util.js over util.mjs.
	fs := newMemFS().
		add("/app/index.js", "").
		add("/app/util.js", "").
		add("/app/util.mjs", "")
	r := newTestResolver(fs, true)

	got, err := r.Resolve("./util", parentCJS("/app/index.js"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/util.js" {
		t.Errorf("require path = %q, want /app/util.js", got.Path)
	}
}

func TestResolveStrictESMNoSearch(t *testing.T) {
	fs := newMemFS().add("/app/index.mjs", "").add("/app/util.js", "")
	r := newTestResolver(fs, false) // not dev: strict ESM

	_, err := r.Resolve("./util", parentESM("/app/index.mjs"))
	if err == nil {
		t.Fatal("strict ESM should not extension-search a bare relative import")
	}
	var re *ResolveError
	if !errors.As(err, &re) || re.Code != "ERR_MODULE_NOT_FOUND" {
		t.Errorf("want ERR_MODULE_NOT_FOUND, got %v", err)
	}
}

func TestResolveBuiltin(t *testing.T) {
	r := newTestResolver(newMemFS(), true)
	got, err := r.Resolve("node:fs", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindBuiltin || got.Path != "fs" || got.Format != FormatBuiltin {
		t.Errorf("unexpected builtin resolution: %+v", got)
	}
}

func TestResolveBareBuiltinUnshadowed(t *testing.T) {
	// A bare "fs" with no local package resolves to the builtin.
	r := newTestResolver(newMemFS(), true)
	got, err := r.Resolve("fs", parentCJS("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindBuiltin {
		t.Errorf("bare fs should resolve to builtin, got %v", got.Kind)
	}
}

func TestResolveBareBuiltinShadowed(t *testing.T) {
	// A local node_modules/fs package shadows the builtin name.
	fs := newMemFS().
		add("/app/index.ts", "").
		add("/app/node_modules/fs/package.json", `{"name":"fs","main":"main.js"}`).
		add("/app/node_modules/fs/main.js", "")
	r := newTestResolver(fs, true)

	got, err := r.Resolve("fs", parentCJS("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindFile || got.Path != "/app/node_modules/fs/main.js" {
		t.Errorf("local fs package should shadow builtin, got %+v", got)
	}
}

func TestResolveNodeModulesWalk(t *testing.T) {
	fs := newMemFS().
		add("/app/src/index.ts", "").
		add("/app/node_modules/left-pad/package.json", `{"name":"left-pad","main":"index.js"}`).
		add("/app/node_modules/left-pad/index.js", "")
	r := newTestResolver(fs, true)

	got, err := r.Resolve("left-pad", parentCJS("/app/src/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/node_modules/left-pad/index.js" {
		t.Errorf("path = %q, want the walked package main", got.Path)
	}
}

func TestResolveScopedPackageSubpath(t *testing.T) {
	fs := newMemFS().
		add("/app/index.ts", "").
		add("/app/node_modules/@scope/pkg/package.json", `{"name":"@scope/pkg"}`).
		add("/app/node_modules/@scope/pkg/sub.js", "")
	r := newTestResolver(fs, true)

	got, err := r.Resolve("@scope/pkg/sub", parentCJS("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/node_modules/@scope/pkg/sub.js" {
		t.Errorf("path = %q, want the scoped subpath", got.Path)
	}
}

func TestParseBare(t *testing.T) {
	cases := []struct {
		spec, name, subpath string
	}{
		{"lodash", "lodash", "."},
		{"lodash/fp", "lodash", "./fp"},
		{"@scope/pkg", "@scope/pkg", "."},
		{"@scope/pkg/sub/deep", "@scope/pkg", "./sub/deep"},
	}
	for _, c := range cases {
		name, subpath := parseBare(c.spec)
		if name != c.name || subpath != c.subpath {
			t.Errorf("parseBare(%q) = (%q, %q), want (%q, %q)", c.spec, name, subpath, c.name, c.subpath)
		}
	}
}

func TestResolveNotFoundListsSearched(t *testing.T) {
	fs := newMemFS().add("/app/index.ts", "")
	r := newTestResolver(fs, true)

	_, err := r.Resolve("missing-pkg", parentCJS("/app/index.ts"))
	var re *ResolveError
	if !errors.As(err, &re) {
		t.Fatalf("want *ResolveError, got %T", err)
	}
	if re.Code != "ERR_MODULE_NOT_FOUND" {
		t.Errorf("code = %q, want ERR_MODULE_NOT_FOUND", re.Code)
	}
	if len(re.Searched) == 0 {
		t.Error("not-found error should list searched node_modules dirs")
	}
}

func TestRealPathCanonicalizes(t *testing.T) {
	fs := newMemFS().
		add("/app/index.ts", "").
		add("/real/util.ts", "").
		link("/app/util.ts", "/real/util.ts")
	r := newTestResolver(fs, true)

	got, err := r.Resolve("./util.ts", parentCJS("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/real/util.ts" {
		t.Errorf("path = %q, want the symlink-resolved /real/util.ts", got.Path)
	}
}
