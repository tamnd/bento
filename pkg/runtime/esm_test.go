package runtime

import (
	"strings"
	"testing"

	// Pull in the default engine backend for the end-to-end tests.
	_ "github.com/tamnd/bento/pkg/engine/quickjs"
)

func TestTopLevelAwaitEntry(t *testing.T) {
	// A program that awaits at the top level cannot run as CommonJS; it must take
	// the native ES module path. The ordering must match Node's.
	out := runFile(t, "tla.mjs", map[string]string{
		"tla.mjs": `console.log("start");
const all = await Promise.all([Promise.resolve("x"), Promise.resolve("y"), Promise.resolve("z")]);
console.log("all", all.join(""));
const v = await Promise.resolve(41);
console.log("awaited", v + 1);
console.log("end");`,
	})
	want := "start\nall xyz\nawaited 42\nend\n"
	if out != want {
		t.Errorf("stdout = %q, want %q", out, want)
	}
}

func TestTopLevelAwaitWithTimer(t *testing.T) {
	// The event loop must pump timers so a top-level await on a timer-backed
	// promise resolves and the program settles.
	out := runFile(t, "timer.mjs", map[string]string{
		"timer.mjs": `const x = await new Promise((r) => setTimeout(() => r("done"), 5));
console.log(x);`,
	})
	if out != "done\n" {
		t.Errorf("stdout = %q, want done", out)
	}
}

func TestESMImportsBuiltin(t *testing.T) {
	// A native ES module importing a core builtin, both named and default, runs
	// through the re-export shim over require.
	out := runFile(t, "app.mjs", map[string]string{
		"app.mjs": `import path, { join } from "node:path";
const p = await Promise.resolve(join("a", "b", "c"));
console.log(p);
console.log(typeof path.dirname);`,
	})
	if out != "a/b/c\nfunction\n" {
		t.Errorf("stdout = %q, want the joined path and a function", out)
	}
}

func TestESMImportsLocalESM(t *testing.T) {
	out := runFile(t, "app.mjs", map[string]string{
		"app.mjs": `import lib, { twice } from "./lib.mjs";
console.log(await Promise.resolve(twice(21)), lib);`,
		"lib.mjs": `export const twice = (n) => n * 2;
export default "lib-default";`,
	})
	if out != "42 lib-default\n" {
		t.Errorf("stdout = %q, want 42 lib-default", out)
	}
}

func TestESMImportsCommonJSNamed(t *testing.T) {
	// Named and default imports of a CommonJS dependency must both work from an
	// ES module, which is the interop the shim provides.
	out := runFile(t, "app.mjs", map[string]string{
		"app.mjs": `import dep, { hello, VERSION } from "./dep.cjs";
console.log(await Promise.resolve(hello("x")), VERSION, typeof dep);`,
		"dep.cjs": `module.exports = { hello: (n) => "hi " + n, VERSION: "1.0" };`,
	})
	if out != "hi x 1.0 object\n" {
		t.Errorf("stdout = %q, want hi x 1.0 object", out)
	}
}

func TestESMImportsJSON(t *testing.T) {
	out := runFile(t, "app.mjs", map[string]string{
		"app.mjs": `import cfg from "./data.json";
console.log(await Promise.resolve(cfg.port));`,
		"data.json": `{"port": 8080}`,
	})
	if out != "8080\n" {
		t.Errorf("stdout = %q, want 8080", out)
	}
}

func TestNonAwaitEntryStaysCommonJS(t *testing.T) {
	// A module without top-level await keeps the CommonJS path even when it uses
	// import syntax, so the common case never pays for native module linking.
	out := runFile(t, "app.ts", map[string]string{
		"app.ts": `import { join } from "node:path";
console.log(join("p", "q"));`,
	})
	if out != "p/q\n" {
		t.Errorf("stdout = %q, want p/q", out)
	}
}

func TestTopLevelAwaitReExportChain(t *testing.T) {
	// An ES module re-exporting from another ES module must link through the
	// resolver, and a top-level await in the entry gates the output.
	out := runFile(t, "app.mjs", map[string]string{
		"app.mjs": `import { sum } from "./a.mjs";
console.log(await Promise.resolve(sum(1, 2, 3)));`,
		"a.mjs": `export { sum } from "./b.mjs";`,
		"b.mjs": `export const sum = (...ns) => ns.reduce((a, b) => a + b, 0);`,
	})
	if strings.TrimSpace(out) != "6" {
		t.Errorf("stdout = %q, want 6", out)
	}
}
