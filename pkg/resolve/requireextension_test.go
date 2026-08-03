package resolve

import "testing"

// A require never resolves to .mjs. Node's CommonJS resolver searches .js,
// .json and .node and stops, and .mjs means ESM by definition, so picking one
// for a require is not a near miss, it is the wrong module. A package that
// ships both index.js and index.mjs is shipping a CommonJS entry and an ESM
// entry side by side, and require means the first one.
//
// Node's own test suite does exactly this: node/test/common/ holds index.js
// next to index.mjs and fixtures.js next to fixtures.mjs, and almost every test
// in the suite opens with require('../common'). Resolving that to the ESM file
// put a fifth of the suite behind a single line of extension order.

func TestRequirePrefersTheCommonJSSiblingOverTheESMOne(t *testing.T) {
	fs := newMemFS().add("/app/main.js", "").add("/app/leaf.js", "").add("/app/leaf.mjs", "")
	r := newTestResolver(fs, true)

	got, err := r.Resolve("./leaf", parentCJS("/app/main.js"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/leaf.js" {
		t.Errorf("path = %q, want /app/leaf.js; a require took the ESM sibling", got.Path)
	}
}

func TestRequireOfADirectoryTakesIndexJSOverIndexMJS(t *testing.T) {
	// This is test/common exactly: a directory holding both index files, reached
	// by a bare directory specifier.
	fs := newMemFS().add("/app/main.js", "").add("/app/common/index.js", "").add("/app/common/index.mjs", "")
	r := newTestResolver(fs, true)

	got, err := r.Resolve("./common", parentCJS("/app/main.js"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/common/index.js" {
		t.Errorf("path = %q, want /app/common/index.js", got.Path)
	}
}

func TestAnImportStillTakesTheESMSibling(t *testing.T) {
	// The rule is about the context asking, not about .mjs being unwelcome. An
	// ESM importer that writes an extensionless specifier still means the ESM
	// file, and that ordering is unchanged.
	fs := newMemFS().add("/app/main.mjs", "").add("/app/leaf.js", "").add("/app/leaf.mjs", "")
	r := newTestResolver(fs, true)

	got, err := r.Resolve("./leaf", parentESM("/app/main.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/leaf.mjs" {
		t.Errorf("path = %q, want /app/leaf.mjs", got.Path)
	}
}

func TestRequireStillFindsAnMJSAskedForByName(t *testing.T) {
	// Dropping .mjs from the search order is not a ban on the file. A specifier
	// that names the extension is an exact path and resolves as written; what
	// the loader then does with an ESM file is the loader's business.
	fs := newMemFS().add("/app/main.js", "").add("/app/leaf.mjs", "")
	r := newTestResolver(fs, true)

	got, err := r.Resolve("./leaf.mjs", parentCJS("/app/main.js"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/leaf.mjs" {
		t.Errorf("path = %q, want /app/leaf.mjs", got.Path)
	}
}

func TestRequireWithNoCommonJSSiblingDoesNotFallBackToMJS(t *testing.T) {
	// The order matters less than the exclusion. If .mjs were merely demoted, a
	// require of a name with only an ESM file on disk would still find it and
	// hand the CommonJS loader a module it cannot stage. Not found is the honest
	// answer, and it is Node's answer too.
	fs := newMemFS().add("/app/main.js", "").add("/app/leaf.mjs", "")
	r := newTestResolver(fs, true)

	if got, err := r.Resolve("./leaf", parentCJS("/app/main.js")); err == nil {
		t.Errorf("resolved to %q; a require should not reach an .mjs by extension search", got.Path)
	}
}

func TestTheSameSpecifierResolvesTwoWaysFromTwoContexts(t *testing.T) {
	// One resolver answers both questions, and the answers differ, so the cache
	// has to keep them apart. Before the context was part of the key, whichever
	// importer asked first decided for the other.
	fs := newMemFS().
		add("/app/main.js", "").add("/app/main.mjs", "").
		add("/app/leaf.js", "").add("/app/leaf.mjs", "")
	r := newTestResolver(fs, true)

	first, err := r.Resolve("./leaf", parentCJS("/app/main.js"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.Resolve("./leaf", parentESM("/app/main.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if first.Path != "/app/leaf.js" {
		t.Errorf("require got %q, want /app/leaf.js", first.Path)
	}
	if second.Path != "/app/leaf.mjs" {
		t.Errorf("import got %q, want /app/leaf.mjs; the cache answered with the require's result", second.Path)
	}
}

func TestABarePackageRequireTakesTheCommonJSIndex(t *testing.T) {
	// The same rule has to hold down the node_modules path, which resolves its
	// own index and subpath files rather than going through resolveFile.
	fs := newMemFS().
		add("/app/main.js", "").
		add("/app/node_modules/dual/index.js", "").
		add("/app/node_modules/dual/index.mjs", "").
		add("/app/node_modules/dual/sub.js", "").
		add("/app/node_modules/dual/sub.mjs", "")
	r := newTestResolver(fs, true)

	root, err := r.Resolve("dual", parentCJS("/app/main.js"))
	if err != nil {
		t.Fatal(err)
	}
	if root.Path != "/app/node_modules/dual/index.js" {
		t.Errorf("package root = %q, want the .js index", root.Path)
	}
	sub, err := r.Resolve("dual/sub", parentCJS("/app/main.js"))
	if err != nil {
		t.Fatal(err)
	}
	if sub.Path != "/app/node_modules/dual/sub.js" {
		t.Errorf("package subpath = %q, want the .js file", sub.Path)
	}
}
