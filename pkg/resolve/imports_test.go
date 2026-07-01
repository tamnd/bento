package resolve

import (
	"errors"
	"testing"
)

func TestImportsExactRelativeTarget(t *testing.T) {
	pkg := `{
		"name": "app",
		"imports": {
			"#config": "./src/config.js"
		}
	}`
	fs := newMemFS().
		add("/app/package.json", pkg).
		add("/app/src/config.js", "").
		add("/app/src/main.ts", "")

	r := resolverWithConditions(fs, DefaultImportConditions)
	got, err := r.Resolve("#config", parentESM("/app/src/main.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/src/config.js" {
		t.Errorf("path = %q, want the mapped config file", got.Path)
	}
}

func TestImportsConditionSelection(t *testing.T) {
	// An internal import can carry conditions, so a package ships a bento variant
	// of a private alias. The first active condition in author order wins.
	pkg := `{
		"name": "app",
		"imports": {
			"#env": {
				"bento": "./src/env.bento.js",
				"node": "./src/env.node.js",
				"default": "./src/env.js"
			}
		}
	}`
	fs := newMemFS().
		add("/app/package.json", pkg).
		add("/app/src/env.bento.js", "").
		add("/app/src/env.node.js", "").
		add("/app/src/env.js", "").
		add("/app/src/main.ts", "")

	r := resolverWithConditions(fs, DefaultImportConditions)
	got, err := r.Resolve("#env", parentESM("/app/src/main.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/src/env.bento.js" {
		t.Errorf("path = %q, want the bento variant", got.Path)
	}
}

func TestImportsStarPattern(t *testing.T) {
	pkg := `{
		"name": "app",
		"imports": {
			"#lib/*": "./src/lib/*.js"
		}
	}`
	fs := newMemFS().
		add("/app/package.json", pkg).
		add("/app/src/lib/math.js", "").
		add("/app/src/main.ts", "")

	r := resolverWithConditions(fs, DefaultImportConditions)
	got, err := r.Resolve("#lib/math", parentESM("/app/src/main.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/src/lib/math.js" {
		t.Errorf("path = %q, want the star-substituted file", got.Path)
	}
}

func TestImportsLongestPrefixWins(t *testing.T) {
	// Two patterns match; the one with the longer literal prefix wins.
	pkg := `{
		"name": "app",
		"imports": {
			"#lib/*": "./src/generic/*.js",
			"#lib/http/*": "./src/http/*.js"
		}
	}`
	fs := newMemFS().
		add("/app/package.json", pkg).
		add("/app/src/generic/http/client.js", "").
		add("/app/src/http/client.js", "").
		add("/app/src/main.ts", "")

	r := resolverWithConditions(fs, DefaultImportConditions)
	got, err := r.Resolve("#lib/http/client", parentESM("/app/src/main.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/src/http/client.js" {
		t.Errorf("path = %q, want the longer-prefix pattern to win", got.Path)
	}
}

func TestImportsBareTarget(t *testing.T) {
	// An internal import mapping to a bare specifier resolves through the normal
	// node_modules walk, so "#pkg" is a private alias for a real dependency.
	pkg := `{
		"name": "app",
		"imports": {
			"#logger": "pino"
		}
	}`
	dep := `{ "name": "pino", "main": "./index.js" }`
	fs := newMemFS().
		add("/app/package.json", pkg).
		add("/app/src/main.ts", "").
		add("/app/node_modules/pino/package.json", dep).
		add("/app/node_modules/pino/index.js", "")

	r := resolverWithConditions(fs, DefaultImportConditions)
	got, err := r.Resolve("#logger", parentESM("/app/src/main.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != "/app/node_modules/pino/index.js" {
		t.Errorf("path = %q, want the resolved dependency entry", got.Path)
	}
}

func TestImportsNotDefined(t *testing.T) {
	pkg := `{
		"name": "app",
		"imports": {
			"#config": "./src/config.js"
		}
	}`
	fs := newMemFS().
		add("/app/package.json", pkg).
		add("/app/src/main.ts", "")

	r := resolverWithConditions(fs, DefaultImportConditions)
	_, err := r.Resolve("#missing", parentESM("/app/src/main.ts"))
	if err == nil {
		t.Fatal("expected an import-not-defined error")
	}
	var re *ResolveError
	if !errors.As(err, &re) || re.Code != "ERR_PACKAGE_IMPORT_NOT_DEFINED" {
		t.Errorf("error = %v, want ERR_PACKAGE_IMPORT_NOT_DEFINED", err)
	}
}

func TestImportsInvalidSpecifier(t *testing.T) {
	fs := newMemFS().
		add("/app/package.json", `{"name":"app","imports":{"#x":"./x.js"}}`).
		add("/app/src/main.ts", "")

	r := resolverWithConditions(fs, DefaultImportConditions)
	for _, spec := range []string{"#", "#/foo"} {
		_, err := r.Resolve(spec, parentESM("/app/src/main.ts"))
		if err == nil {
			t.Errorf("%q: expected an invalid-specifier error", spec)
			continue
		}
		var re *ResolveError
		if !errors.As(err, &re) || re.Code != "ERR_INVALID_MODULE_SPECIFIER" {
			t.Errorf("%q: error = %v, want ERR_INVALID_MODULE_SPECIFIER", spec, err)
		}
	}
}

func TestImportsNoImportsField(t *testing.T) {
	fs := newMemFS().
		add("/app/package.json", `{"name":"app"}`).
		add("/app/src/main.ts", "")

	r := resolverWithConditions(fs, DefaultImportConditions)
	_, err := r.Resolve("#anything", parentESM("/app/src/main.ts"))
	if err == nil {
		t.Fatal("expected an error when no imports field exists")
	}
	var re *ResolveError
	if !errors.As(err, &re) || re.Code != "ERR_PACKAGE_IMPORT_NOT_DEFINED" {
		t.Errorf("error = %v, want ERR_PACKAGE_IMPORT_NOT_DEFINED", err)
	}
}
