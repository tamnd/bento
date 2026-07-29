package node

import (
	"os"
	"strconv"
	"testing"
)

// The engine's node:path is held to the same table of real Node output that the
// ahead-of-time port in pkg/value is held to. That is the whole point of the
// table: a program has to get the same answer whether it was run or compiled, and
// the only way to be sure of that is to check both against the runtime they are
// both imitating rather than against each other.
//
// The table lives next to the port because that is where it was written; a second
// copy here would be a second thing to keep current.
const nodePathRefPath = "../value/testdata/nodepath_node24.json"

// pathTableScript drives every case in the reference through one variant and
// returns the mismatches, so a failure names the call rather than just the count.
// It runs inside the engine, because the module under test is JavaScript and the
// engine is the thing that has to run it.
const pathTableScript = `
(function (variantName, cwd, refText) {
  const ref = JSON.parse(refText)[variantName];
  const impl = require("path")[variantName];
  process.cwd = function () { return cwd; };
  // The win32 variant reads the working directory Windows keeps per drive out of
  // the environment. The reference was generated with none of those set, so the
  // table runs against none either.
  process.env = {};
  const bad = [];
  function eq(what, got, want) {
    if (got !== want) bad.push(what + " = " + JSON.stringify(got) + ", node says " + JSON.stringify(want));
  }
  for (const p of Object.keys(ref.unary)) {
    const want = ref.unary[p];
    eq("normalize(" + JSON.stringify(p) + ")", impl.normalize(p), want.normalize);
    eq("dirname(" + JSON.stringify(p) + ")", impl.dirname(p), want.dirname);
    eq("basename(" + JSON.stringify(p) + ")", impl.basename(p), want.basename);
    eq("extname(" + JSON.stringify(p) + ")", impl.extname(p), want.extname);
    eq("isAbsolute(" + JSON.stringify(p) + ")", impl.isAbsolute(p), want.isAbsolute);
    eq("toNamespacedPath(" + JSON.stringify(p) + ")", impl.toNamespacedPath(p), want.toNamespacedPath);
  }
  for (const key of Object.keys(ref.basenameExt)) {
    const a = JSON.parse(key);
    eq("basename" + key, impl.basename(a[0], a[1]), ref.basenameExt[key]);
  }
  for (const key of Object.keys(ref.join)) {
    eq("join" + key, impl.join.apply(null, JSON.parse(key)), ref.join[key]);
  }
  for (const key of Object.keys(ref.relative)) {
    eq("relative" + key, impl.relative.apply(null, JSON.parse(key)), ref.relative[key]);
  }
  for (const key of Object.keys(ref.resolve)) {
    eq("resolve" + key, impl.resolve.apply(null, JSON.parse(key)), ref.resolve[key]);
  }
  eq("sep", impl.sep, ref.sep);
  eq("delimiter", impl.delimiter, ref.delimiter);
  return bad.join("\n");
})`

// TestPathModuleMatchesNode runs the whole reference table through the engine's
// path module, both variants, on whatever platform the test runs on.
func TestPathModuleMatchesNode(t *testing.T) {
	ref, err := os.ReadFile(nodePathRefPath)
	if err != nil {
		t.Fatalf("read reference: %v", err)
	}
	for _, variant := range []struct{ name, cwd string }{
		{"posix", "/cwd"},
		{"win32", `C:\cwd`},
	} {
		t.Run(variant.name, func(t *testing.T) {
			eng := harness(t)
			call := pathTableScript + "(" + strconv.Quote(variant.name) + ", " +
				strconv.Quote(variant.cwd) + ", " + strconv.Quote(string(ref)) + ")"
			got, err := eng.Eval("<path-table>", call)
			if err != nil {
				t.Fatalf("run table: %v", err)
			}
			s, ok := got.(string)
			if !ok {
				t.Fatalf("table returned %T, want a string", got)
			}
			if s != "" {
				t.Errorf("engine path.%s disagrees with node:\n%s", variant.name, s)
			}
		})
	}
}

// TestPathModuleFollowsThePlatform pins which variant a bare require gets, which
// is the one thing the table cannot say: it drives the variants by name, and a
// program that writes require("path").join gets whichever one the platform calls
// for.
func TestPathModuleFollowsThePlatform(t *testing.T) {
	eng := harness(t)
	want := "a/b"
	if nodePlatform() == "win32" {
		want = `a\b`
	}
	if got := evalString(t, eng, `require("path").join("a", "b")`); got != want {
		t.Errorf(`require("path").join("a", "b") = %q, want %q on %s`, got, want, nodePlatform())
	}
	// Node's path is its own posix and win32 property, both ways round, so code
	// that reaches for path.posix.win32 finds what it expects.
	for _, expr := range []string{
		`require("path").posix.posix.sep === "/"`,
		`require("path").posix.win32.sep === "\\"`,
		`require("path").win32.posix.sep === "/"`,
		`require("path").win32.win32.sep === "\\"`,
	} {
		if got := evalString(t, eng, expr); got != "true" {
			t.Errorf("%s = %q, want true", expr, got)
		}
	}
}
