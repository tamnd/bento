package resolve

import (
	"errors"
	"testing"
)

func resolverWithConditions(fs FS, conditions []string) *Resolver {
	return New(Options{FS: fs, Conditions: conditions, Dev: true})
}

func TestExportsConditionSelection(t *testing.T) {
	pkg := `{
		"name": "dual",
		"exports": {
			".": {
				"import": "./esm/index.js",
				"require": "./cjs/index.js",
				"default": "./cjs/index.js"
			}
		}
	}`
	fs := newMemFS().
		add("/app/index.ts", "").
		add("/app/node_modules/dual/package.json", pkg).
		add("/app/node_modules/dual/esm/index.js", "").
		add("/app/node_modules/dual/cjs/index.js", "")

	importR := resolverWithConditions(fs, DefaultImportConditions)
	got, err := importR.Resolve("dual", parentESM("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/node_modules/dual/esm/index.js" {
		t.Errorf("import condition path = %q, want the esm build", got.Path)
	}

	requireR := resolverWithConditions(fs, DefaultRequireConditions)
	got, err = requireR.Resolve("dual", parentCJS("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/node_modules/dual/cjs/index.js" {
		t.Errorf("require condition path = %q, want the cjs build", got.Path)
	}
}

func TestExportsConditionOrderWins(t *testing.T) {
	// When both node and default could match, the first in author order wins.
	pkg := `{
		"name": "ordered",
		"exports": {
			"node": "./node.js",
			"default": "./default.js"
		}
	}`
	fs := newMemFS().
		add("/app/index.ts", "").
		add("/app/node_modules/ordered/package.json", pkg).
		add("/app/node_modules/ordered/node.js", "").
		add("/app/node_modules/ordered/default.js", "")

	r := resolverWithConditions(fs, DefaultImportConditions)
	got, err := r.Resolve("ordered", parentESM("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/node_modules/ordered/node.js" {
		t.Errorf("path = %q, want node.js since node comes first", got.Path)
	}
}

func TestExportsSubpathPattern(t *testing.T) {
	pkg := `{
		"name": "patterned",
		"exports": {
			"./features/*": "./src/features/*.js",
			"./features/special": "./src/special.js"
		}
	}`
	fs := newMemFS().
		add("/app/index.ts", "").
		add("/app/node_modules/patterned/package.json", pkg).
		add("/app/node_modules/patterned/src/features/a.js", "").
		add("/app/node_modules/patterned/src/special.js", "")

	r := resolverWithConditions(fs, DefaultImportConditions)

	got, err := r.Resolve("patterned/features/a", parentESM("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/node_modules/patterned/src/features/a.js" {
		t.Errorf("pattern path = %q, want the star substitution", got.Path)
	}

	// The exact key must beat the pattern.
	got, err = r.Resolve("patterned/features/special", parentESM("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/node_modules/patterned/src/special.js" {
		t.Errorf("exact path = %q, want special.js beating the pattern", got.Path)
	}
}

func TestExportsPathNotExported(t *testing.T) {
	pkg := `{"name":"closed","exports":{".":"./index.js"}}`
	fs := newMemFS().
		add("/app/index.ts", "").
		add("/app/node_modules/closed/package.json", pkg).
		add("/app/node_modules/closed/index.js", "").
		add("/app/node_modules/closed/secret.js", "")

	r := resolverWithConditions(fs, DefaultImportConditions)
	_, err := r.Resolve("closed/secret", parentESM("/app/index.ts"))
	var re *ResolveError
	if !errors.As(err, &re) || re.Code != "ERR_PACKAGE_PATH_NOT_EXPORTED" {
		t.Errorf("want ERR_PACKAGE_PATH_NOT_EXPORTED, got %v", err)
	}
}

func TestExportsStringShorthand(t *testing.T) {
	pkg := `{"name":"simple","exports":"./main.js"}`
	fs := newMemFS().
		add("/app/index.ts", "").
		add("/app/node_modules/simple/package.json", pkg).
		add("/app/node_modules/simple/main.js", "")

	r := resolverWithConditions(fs, DefaultImportConditions)
	got, err := r.Resolve("simple", parentESM("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/node_modules/simple/main.js" {
		t.Errorf("path = %q, want the string-shorthand main", got.Path)
	}
}

func TestLegacyMainAndModule(t *testing.T) {
	pkg := `{"name":"legacy","main":"./cjs.js","module":"./esm.js"}`
	fs := newMemFS().
		add("/app/index.ts", "").
		add("/app/node_modules/legacy/package.json", pkg).
		add("/app/node_modules/legacy/cjs.js", "").
		add("/app/node_modules/legacy/esm.js", "")

	importR := resolverWithConditions(fs, DefaultImportConditions)
	got, err := importR.Resolve("legacy", parentESM("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/node_modules/legacy/esm.js" {
		t.Errorf("import path = %q, want the module field", got.Path)
	}

	requireR := resolverWithConditions(fs, DefaultRequireConditions)
	got, err = requireR.Resolve("legacy", parentCJS("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/node_modules/legacy/cjs.js" {
		t.Errorf("require path = %q, want the main field", got.Path)
	}
}

func TestExportsOverridesLegacy(t *testing.T) {
	// When exports is present, the main/module fields are dead.
	pkg := `{"name":"e","main":"./legacy.js","exports":{".":"./modern.js"}}`
	fs := newMemFS().
		add("/app/index.ts", "").
		add("/app/node_modules/e/package.json", pkg).
		add("/app/node_modules/e/legacy.js", "").
		add("/app/node_modules/e/modern.js", "")

	r := resolverWithConditions(fs, DefaultImportConditions)
	got, err := r.Resolve("e", parentESM("/app/index.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/node_modules/e/modern.js" {
		t.Errorf("path = %q, want exports to win over main", got.Path)
	}
}

func TestExportsKeyOrderPreserved(t *testing.T) {
	// Parsed directly from source so author key order survives.
	node, err := parseExportsNode([]byte(`{"import":"./a.js","node":"./b.js","default":"./c.js"}`))
	if err != nil {
		t.Fatal(err)
	}
	if node.kind != nodeMap || len(node.entries) != 3 {
		t.Fatalf("unexpected node: %+v", node)
	}
	wantKeys := []string{"import", "node", "default"}
	for i, e := range node.entries {
		if e.key != wantKeys[i] {
			t.Errorf("entry %d key = %q, want %q", i, e.key, wantKeys[i])
		}
	}
}
