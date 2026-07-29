package lower

import (
	"strings"
	"testing"
)

// The node:path surface is wider than one function now, so these cases pin the
// two things the golden program cannot: that each export reaches its own helper
// whichever import form the program used, and that the shapes with no lowering
// hand back rather than compiling into a call that means something else.

// TestPathExportsLowerThroughEveryImportForm renders the same call three ways and
// requires all three to reach the same helper. A named import, a default import
// and a namespace import all name the same CommonJS module object in Node, so a
// program that picks one of them has not picked a different path module.
func TestPathExportsLowerThroughEveryImportForm(t *testing.T) {
	cases := []struct {
		call string
		want string
	}{
		{`join("a", "b")`, "value.PathJoin("},
		{`resolve("a", "b")`, "value.PathResolve("},
		{`normalize("a/b")`, "value.PathNormalize("},
		{`dirname("a/b")`, "value.PathDirname("},
		{`basename("a/b")`, "value.PathBasename("},
		{`basename("a/b", ".b")`, "value.PathBasenameSuffix("},
		{`extname("a/b.txt")`, "value.PathExtname("},
		{`isAbsolute("/a")`, "value.PathIsAbsolute("},
		{`relative("/a", "/b")`, "value.PathRelative("},
		{`toNamespacedPath("/a")`, "value.PathToNamespacedPath("},
	}
	for _, tc := range cases {
		t.Run(tc.call, func(t *testing.T) {
			name := tc.call[:strings.IndexByte(tc.call, '(')]
			forms := map[string]string{
				"named":     `import { ` + name + ` } from "node:path";` + "\nconst v: any = " + tc.call + ";",
				"default":   `import path from "node:path";` + "\nconst v: any = path." + tc.call + ";",
				"namespace": `import * as path from "node:path";` + "\nconst v: any = path." + tc.call + ";",
			}
			for form, src := range forms {
				got := renderProgram(t, src)
				if !strings.Contains(got, tc.want) {
					t.Errorf("%s import of %s emitted:\n%s\nwant a call to %s", form, name, got, tc.want)
				}
			}
		})
	}
}

// TestPathHandbacks pins the calls that still have no lowering. Each has to hand
// back: an argument count bento does not emit a helper for, or an argument whose
// type it cannot prove is a string, would otherwise compile into a helper call
// with the wrong shape.
func TestPathHandbacks(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"dirname with no argument",
			`import { dirname } from "node:path";
const v: any = dirname();`,
			"path.dirname with this argument count is a later slice",
		},
		{
			"basename with three arguments",
			`import { basename } from "node:path";
const v: any = basename("a", "b", "c");`,
			"path.basename with this argument count is a later slice",
		},
		{
			"relative with one argument",
			`import { relative } from "node:path";
const v: any = relative("a");`,
			"path.relative with this argument count is a later slice",
		},
		{
			"join of a value bento cannot prove is a string",
			`import { join } from "node:path";
const n: any = 1;
const v: any = join(n);`,
			"path.join with a non-string argument is a later slice",
		},
		{
			"resolve of a value bento cannot prove is a string",
			`import { resolve } from "node:path";
const n: any = 1;
const v: any = resolve("a", n);`,
			"path.resolve with a non-string argument is a later slice",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := compileTolerant(t, tc.src)
			r := NewRenderer(prog)
			_, err := r.RenderProgram(entryFile(t, prog))
			if err == nil {
				t.Fatal("lowered, want a hand back")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("hand back said %q, want it to mention %q", err.Error(), tc.want)
			}
		})
	}
}
