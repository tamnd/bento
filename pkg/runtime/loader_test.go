package runtime

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	// Pull in the default engine backend for the end-to-end tests.
	_ "github.com/tamnd/bento/pkg/engine/quickjs"
)

// runFile writes the given files into a temp dir, runs entry through a real
// runtime, and returns stdout. Files map a relative path to its contents.
func runFile(t *testing.T, entry string, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		full := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var out, errb bytes.Buffer
	rt, err := New(Config{
		Argv:         []string{"bento", entry},
		BentoVersion: "test",
		Stdout:       &out,
		Stderr:       &errb,
	})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	defer func() { _ = rt.Close() }()

	if err := rt.RunFile(filepath.Join(dir, entry)); err != nil {
		t.Fatalf("run %s: %v\nstderr: %s", entry, err, errb.String())
	}
	return out.String()
}

func TestRequireRelativeModule(t *testing.T) {
	out := runFile(t, "index.ts", map[string]string{
		"index.ts": `import { add } from "./math";
console.log(add(2, 3));`,
		"math.ts": `export function add(a: number, b: number): number { return a + b; }`,
	})
	if out != "5\n" {
		t.Errorf("stdout = %q, want 5", out)
	}
}

func TestRequireTSExtensionRewrite(t *testing.T) {
	// Importing ./util.js from TypeScript must find ./util.ts on disk.
	out := runFile(t, "index.ts", map[string]string{
		"index.ts": `import { name } from "./util.js";
console.log(name);`,
		"util.ts": `export const name = "rewritten";`,
	})
	if out != "rewritten\n" {
		t.Errorf("stdout = %q, want rewritten", out)
	}
}

func TestRequireJSONModule(t *testing.T) {
	out := runFile(t, "index.ts", map[string]string{
		"index.ts": `const cfg = require("./config.json");
console.log(cfg.port, cfg.name);`,
		"config.json": `{"port": 8080, "name": "bento"}`,
	})
	if out != "8080 bento\n" {
		t.Errorf("stdout = %q, want 8080 bento", out)
	}
}

func TestRequireNodeModulesPackage(t *testing.T) {
	out := runFile(t, "index.ts", map[string]string{
		"index.ts": `import greet from "greeter";
console.log(greet("world"));`,
		"node_modules/greeter/package.json": `{"name":"greeter","main":"index.js"}`,
		"node_modules/greeter/index.js":     `module.exports = function (who) { return "hi " + who; };`,
	})
	if out != "hi world\n" {
		t.Errorf("stdout = %q, want hi world", out)
	}
}

func TestRequirePackageExports(t *testing.T) {
	out := runFile(t, "index.ts", map[string]string{
		"index.ts": `import { tag } from "modern";
console.log(tag);`,
		"node_modules/modern/package.json": `{"name":"modern","exports":{".":{"bento":"./bento.js","default":"./default.js"}}}`,
		"node_modules/modern/bento.js":     `export const tag = "bento-build";`,
		"node_modules/modern/default.js":   `export const tag = "default-build";`,
	})
	if out != "bento-build\n" {
		t.Errorf("stdout = %q, want bento-build (the bento condition should win)", out)
	}
}

func TestModuleCachedOnce(t *testing.T) {
	// The side-effecting module must run once even when required twice.
	out := runFile(t, "index.ts", map[string]string{
		"index.ts": `import "./side";
import "./side";
const a = require("./counter");
const b = require("./counter");
console.log(a === b, a.value);`,
		"side.ts":    `console.log("side effect");`,
		"counter.ts": `console.log("counter init"); export const value = 1; export default { value };`,
	})
	// "side effect" and "counter init" should each appear exactly once.
	if got := bytes.Count([]byte(out), []byte("side effect")); got != 1 {
		t.Errorf("side effect ran %d times, want 1\n%s", got, out)
	}
	if got := bytes.Count([]byte(out), []byte("counter init")); got != 1 {
		t.Errorf("counter init ran %d times, want 1\n%s", got, out)
	}
}

func TestRequireCoreModuleWinsOverPackage(t *testing.T) {
	// A local node_modules/path package must not shadow the core path module,
	// matching Node: core modules always resolve first for bare names.
	out := runFile(t, "index.ts", map[string]string{
		"index.ts": `import { join } from "path";
console.log(join("a", "b"));`,
		"node_modules/path/package.json": `{"name":"path","main":"index.js"}`,
		"node_modules/path/index.js":     `module.exports = { join: function () { return "IMPOSTER"; } };`,
	})
	if out != "a/b\n" {
		t.Errorf("stdout = %q, want a/b from core path", out)
	}
}

func TestRequireMissingThrowsFromModule(t *testing.T) {
	dir := t.TempDir()
	entry := filepath.Join(dir, "index.ts")
	if err := os.WriteFile(entry, []byte(`require("./does-not-exist");`), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	rt, err := New(Config{Argv: []string{"bento", entry}, Stdout: &out, Stderr: &errb})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer func() { _ = rt.Close() }()
	if err := rt.RunFile(entry); err == nil {
		t.Fatal("requiring a missing relative module should throw")
	}
}
