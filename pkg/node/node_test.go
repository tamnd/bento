package node

import (
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/engine"
	_ "github.com/tamnd/bento/pkg/engine/quickjs"
)

// harness builds an engine with the minimal prelude hooks the node layer needs
// (__bento_defineModule, require, __bento_inspect) and installs the node layer.
func harness(t *testing.T) engine.Engine {
	t.Helper()
	eng, err := engine.New("")
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	t.Cleanup(func() { _ = eng.Close() })

	// A stand-in prelude: just the module registry the node factories plug into.
	const mini = `
	globalThis.__bento_inspect = function (v) { try { return JSON.stringify(v); } catch (e) { return String(v); } };
	(function () {
	  const resolved = new Map();
	  const factories = new Map();
	  function alias(name, fn) { fn(name); if (name.indexOf("node:") === 0) fn(name.slice(5)); else fn("node:" + name); }
	  globalThis.__bento_registerModule = function (n, e) { alias(n, (x) => resolved.set(x, e)); };
	  globalThis.__bento_defineModule = function (n, f) { alias(n, (x) => factories.set(x, f)); };
	  function load(spec) {
	    if (resolved.has(spec)) return resolved.get(spec);
	    const f = factories.get(spec);
	    if (!f) return undefined;
	    const m = { exports: {} };
	    alias(spec, (x) => resolved.set(x, m.exports));
	    f(m, m.exports, globalThis.require);
	    alias(spec, (x) => resolved.set(x, m.exports));
	    return m.exports;
	  }
	  globalThis.require = function (spec) {
	    const found = load(spec);
	    if (found !== undefined) return found;
	    throw new Error("Cannot find module '" + spec + "'");
	  };
	})();
	`
	if _, err := eng.Eval("<mini-prelude>", mini); err != nil {
		t.Fatalf("mini prelude: %v", err)
	}
	if err := Install(eng); err != nil {
		t.Fatalf("install: %v", err)
	}
	return eng
}

// evalString runs an expression and returns its string result.
func evalString(t *testing.T, eng engine.Engine, expr string) string {
	t.Helper()
	v, err := eng.Eval("<test>", "String("+expr+")")
	if err != nil {
		t.Fatalf("eval %q: %v", expr, err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("eval %q: want string, got %T", expr, v)
	}
	return s
}

func TestPathModule(t *testing.T) {
	eng := harness(t)
	cases := map[string]string{
		`require("path").join("a", "b", "c")`:      "a/b/c",
		`require("path").basename("/x/y/z.ts")`:    "z.ts",
		`require("path").extname("/x/y/z.ts")`:     ".ts",
		`require("path").dirname("/x/y/z.ts")`:     "/x/y",
		`require("path").normalize("/a/./b/../c")`: "/a/c",
		`require("node:path").isAbsolute("/x")`:    "true",
		`require("path").posix.join("a", "b")`:     "a/b",
		`require("path").win32.join("a", "b")`:     "a\\b",
	}
	for expr, want := range cases {
		if got := evalString(t, eng, expr); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
}

func TestEventsModule(t *testing.T) {
	eng := harness(t)
	got := evalString(t, eng, `(function () {
		const EE = require("events");
		const e = new EE();
		let sum = 0;
		e.on("add", (n) => { sum += n; });
		e.emit("add", 2);
		e.emit("add", 3);
		let onceCount = 0;
		e.once("boot", () => { onceCount++; });
		e.emit("boot");
		e.emit("boot");
		return sum + ":" + onceCount + ":" + e.listenerCount("add");
	})()`)
	if got != "5:1:1" {
		t.Errorf("events = %q, want 5:1:1", got)
	}
}

func TestBufferModule(t *testing.T) {
	eng := harness(t)
	cases := map[string]string{
		`require("buffer").Buffer.from("hi").toString("hex")`:                                                                    "6869",
		`require("buffer").Buffer.from("6869", "hex").toString()`:                                                                "hi",
		`require("buffer").Buffer.from("hello").toString("base64")`:                                                              "aGVsbG8=",
		`require("buffer").Buffer.from("aGVsbG8=", "base64").toString()`:                                                         "hello",
		`require("buffer").Buffer.concat([require("buffer").Buffer.from("ab"), require("buffer").Buffer.from("cd")]).toString()`: "abcd",
		`require("buffer").Buffer.alloc(3).length`:                                                                               "3",
		`require("buffer").Buffer.isBuffer(require("buffer").Buffer.from("x"))`:                                                  "true",
	}
	for expr, want := range cases {
		if got := evalString(t, eng, expr); got != want {
			t.Errorf("%s = %q, want %q", expr, got, want)
		}
	}
}

func TestUtilModule(t *testing.T) {
	eng := harness(t)
	if got := evalString(t, eng, `require("util").format("%s=%d", "x", 5)`); got != "x=5" {
		t.Errorf("util.format = %q", got)
	}
}

func TestAssertModule(t *testing.T) {
	eng := harness(t)
	got := evalString(t, eng, `(function () {
		const assert = require("assert");
		assert.strictEqual(1 + 1, 2);
		let threw = false;
		try { assert.strictEqual(1, 2); } catch (e) { threw = e.code === "ERR_ASSERTION"; }
		return threw;
	})()`)
	if got != "true" {
		t.Errorf("assert = %q, want true", got)
	}
}

func TestOSModule(t *testing.T) {
	eng := harness(t)
	if got := evalString(t, eng, `typeof require("os").platform()`); got != "string" {
		t.Errorf("os.platform type = %q", got)
	}
	if got := evalString(t, eng, `require("os").arch().length > 0`); got != "true" {
		t.Errorf("os.arch = %q", got)
	}
}

func TestSourceIsStable(t *testing.T) {
	a, err := Source()
	if err != nil {
		t.Fatalf("source: %v", err)
	}
	b, _ := Source()
	if a != b {
		t.Error("Source() is not deterministic")
	}
	if !strings.Contains(a, "__bento_defineModule") {
		t.Error("bundle missing module definitions")
	}
}
